package services

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	m "github.com/keighl/mandrill"
)

// Mailer abstrai o envio de e-mails para permitir testes sem chamadas externas.
type Mailer interface {
	Send(toEmail, toName, subject, html string) error
}

// Mail é a instância global usada pelos controllers. É trocada em tempo de
// execução quando o administrador salva outro provedor em Configurações →
// E-mail (ConfigureMailer); nil significa "envio desabilitado".
var Mail Mailer

var mailMu sync.RWMutex

// SetMailer troca o remetente ativo (concorrência segura).
func SetMailer(mm Mailer) {
	mailMu.Lock()
	Mail = mm
	mailMu.Unlock()
}

// Remetente devolve o mailer ativo (nil = desabilitado).
func Remetente() Mailer {
	mailMu.RLock()
	defer mailMu.RUnlock()
	return Mail
}

// -----------------------------------------------------------------------------
// Resolução do provedor: configuração do cliente (banco) > ambiente (.env)
// -----------------------------------------------------------------------------

// ConfigurarRemetente lê a configuração de e-mail do banco (quando houver)
// e instala o mailer correspondente. Chamado no boot e sempre que a
// configuração é salva.
func ConfigurarRemetente(cfg *models.EmailSettings) (string, error) {
	if cfg == nil || cfg.Provider == "" || cfg.Provider == models.EmailProviderPlatform {
		return configurarPadraoDoAmbiente()
	}
	mm, descricao, err := NovoMailerDeConfig(cfg)
	if err != nil {
		return "", err
	}
	SetMailer(mm)
	return descricao, nil
}

func configurarPadraoDoAmbiente() (string, error) {
	c := utils.Cfg
	switch {
	case c.MandrillChave != "" && c.FromEmail != "":
		SetMailer(&MandrillMailer{
			client: m.ClientWithKey(c.MandrillChave), from: c.FromEmail, fromName: c.FromName,
			env: c.Ambiente, emailDev: c.EmailDev,
		})
		return "Mandrill (API) com o remetente da plataforma", nil
	case c.SMTPHost != "" && c.FromEmail != "":
		SetMailer(&SMTPMailer{
			Host: c.SMTPHost, Port: c.SMTPPort, User: c.SMTPUser, Pass: c.SMTPPass,
			From: c.FromEmail, FromName: c.FromName, env: c.Ambiente, emailDev: c.EmailDev,
		})
		return "SMTP da plataforma (" + c.SMTPHost + ")", nil
	}
	SetMailer(nil)
	return "", errors.New("nenhum provedor de e-mail configurado: escolha um em Configurações → E-mail")
}

// NovoMailerDeConfig constrói o mailer para a configuração do cliente.
func NovoMailerDeConfig(cfg *models.EmailSettings) (Mailer, string, error) {
	if cfg.FromEmail == "" {
		return nil, "", errors.New("informe o e-mail do remetente")
	}
	senha, err := utils.Decifrar(cfg.SecretCifrado)
	if err != nil {
		return nil, "", err
	}
	nome := cfg.FromName
	if nome == "" {
		nome = utils.Cfg.TenantNome
	}
	switch cfg.Provider {
	case models.EmailProviderMandrillAPI:
		if senha == "" {
			return nil, "", errors.New("informe a chave de API do Mandrill")
		}
		return &MandrillMailer{client: m.ClientWithKey(senha), from: cfg.FromEmail, fromName: nome, replyTo: cfg.ReplyTo,
			env: utils.Cfg.Ambiente, emailDev: utils.Cfg.EmailDev}, "Mandrill (API)", nil
	case models.EmailProviderMandrillSMTP, models.EmailProviderMailerooSMTP, models.EmailProviderSMTP:
		host, porta, usuario := cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser
		preset := models.EmailPresets[cfg.Provider]
		if preset.Host != "" {
			host, porta = preset.Host, preset.Port
		}
		if host == "" {
			return nil, "", errors.New("informe o servidor SMTP")
		}
		if porta == 0 {
			porta = 587
		}
		if usuario == "" && cfg.Provider == models.EmailProviderMandrillSMTP {
			usuario = cfg.FromEmail // o Mandrill aceita qualquer usuário; a senha é a chave
		}
		if senha == "" {
			return nil, "", errors.New("informe a senha (ou a chave) do SMTP")
		}
		return &SMTPMailer{Host: host, Port: porta, User: usuario, Pass: senha, From: cfg.FromEmail, FromName: nome,
			ReplyTo: cfg.ReplyTo, env: utils.Cfg.Ambiente, emailDev: utils.Cfg.EmailDev}, preset.Nome + " (SMTP " + host + ")", nil
	}
	return nil, "", fmt.Errorf("provedor desconhecido: %s", cfg.Provider)
}

// -----------------------------------------------------------------------------
// Mandrill (API HTTPS)
// -----------------------------------------------------------------------------

// MandrillMailer envia e-mails transacionais pela API do Mandrill.
type MandrillMailer struct {
	client   *m.Client
	from     string
	fromName string
	replyTo  string
	env      string
	emailDev string
}

// NewMandrillMailer mantém o construtor antigo (lê o ambiente).
func NewMandrillMailer() (*MandrillMailer, error) {
	c := utils.Cfg
	if c.MandrillChave == "" {
		return nil, errors.New("chave MANDRILL_API_KEY não encontrada")
	}
	if c.FromEmail == "" {
		return nil, errors.New("remetente (FROM_EMAIL) não encontrado")
	}
	return &MandrillMailer{client: m.ClientWithKey(c.MandrillChave), from: c.FromEmail, fromName: c.FromName,
		env: strings.ToLower(c.Ambiente), emailDev: c.EmailDev}, nil
}

// Send envia o e-mail. Fora de produção, redireciona para EMAIL_DEV.
func (mm *MandrillMailer) Send(toEmail, toName, subject, html string) error {
	if mm.env != "production" && mm.emailDev != "" {
		toEmail = mm.emailDev
	}
	message := &m.Message{}
	message.AddRecipient(toEmail, toName, "to")
	message.FromEmail = mm.from
	message.FromName = mm.fromName
	message.Subject = subject
	message.HTML = html
	if mm.replyTo != "" {
		message.Headers = map[string]string{"Reply-To": mm.replyTo}
	}
	responses, err := mm.client.MessagesSend(message)
	if err != nil {
		return err
	}
	for _, r := range responses {
		if r.Status == "rejected" || r.Status == "invalid" {
			return fmt.Errorf("e-mail %s: status %s (%s)", r.Email, r.Status, r.RejectionReason)
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// SMTP (Mandrill, Maileroo ou qualquer servidor com STARTTLS/TLS)
// -----------------------------------------------------------------------------

// SMTPMailer envia por SMTP autenticado. Porta 465 usa TLS implícito; as
// demais abrem em claro e exigem STARTTLS antes de autenticar — a senha nunca
// trafega sem criptografia.
type SMTPMailer struct {
	Host     string
	Port     int
	User     string
	Pass     string
	From     string
	FromName string
	ReplyTo  string
	env      string
	emailDev string
	// Timeout da conexão (padrão 20s).
	Timeout time.Duration
}

func (s *SMTPMailer) Send(toEmail, toName, subject, html string) error {
	if s.env != "production" && s.emailDev != "" {
		toEmail = s.emailDev
	}
	if s.Timeout == 0 {
		s.Timeout = 20 * time.Second
	}
	endereco := net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
	tlsCfg := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	disc := &net.Dialer{Timeout: s.Timeout}
	if s.Port == 465 {
		conn, err = tls.DialWithDialer(disc, "tcp", endereco, tlsCfg)
	} else {
		conn, err = disc.Dial("tcp", endereco)
	}
	if err != nil {
		return fmt.Errorf("conectando ao SMTP %s: %w", endereco, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.Timeout))

	cli, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("SMTP %s: %w", endereco, err)
	}
	defer cli.Close()

	if s.Port != 465 {
		if ok, _ := cli.Extension("STARTTLS"); !ok {
			return errors.New("o servidor SMTP não oferece STARTTLS; a senha não será enviada em claro")
		}
		if err := cli.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if s.User != "" || s.Pass != "" {
		if err := cli.Auth(smtp.PlainAuth("", s.User, s.Pass, s.Host)); err != nil {
			return fmt.Errorf("autenticação SMTP recusada: %w", err)
		}
	}
	if err := cli.Mail(s.From); err != nil {
		return fmt.Errorf("remetente recusado: %w", err)
	}
	if err := cli.Rcpt(toEmail); err != nil {
		return fmt.Errorf("destinatário recusado: %w", err)
	}
	w, err := cli.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(montarMensagem(s.From, s.FromName, toEmail, toName, s.ReplyTo, subject, html))); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cli.Quit()
}

// montarMensagem escreve o RFC 5322 mínimo: cabeçalhos codificados em UTF-8 e
// corpo HTML em quoted-printable-free (8bit) — os servidores atuais aceitam.
func montarMensagem(from, fromName, to, toName, replyTo, subject, html string) string {
	var b strings.Builder
	codificar := func(s string) string { return mime.QEncoding.Encode("utf-8", s) }
	b.WriteString("From: " + enderecoCom(from, fromName, codificar) + "\r\n")
	b.WriteString("To: " + enderecoCom(to, toName, codificar) + "\r\n")
	if replyTo != "" {
		b.WriteString("Reply-To: " + replyTo + "\r\n")
	}
	b.WriteString("Subject: " + codificar(subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + fmt.Sprintf("%d.%s", time.Now().UnixNano(), dominioDe(from)) + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("X-Mailer: CRM IA\r\n\r\n")
	b.WriteString(strings.ReplaceAll(html, "\n", "\r\n"))
	b.WriteString("\r\n")
	return b.String()
}

func enderecoCom(email, nome string, codificar func(string) string) string {
	if strings.TrimSpace(nome) == "" {
		return email
	}
	return codificar(nome) + " <" + email + ">"
}

func dominioDe(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return email[i+1:]
	}
	return "crmia.local"
}

// -----------------------------------------------------------------------------
// Modelos transacionais da própria plataforma
// -----------------------------------------------------------------------------

// nomeDoProduto é o que aparece nos e-mails do sistema: o nome da empresa do
// cliente, nunca a marca da infraestrutura por trás.
func nomeDoProduto() string {
	if utils.Cfg.TenantNome != "" {
		return utils.Cfg.TenantNome
	}
	return "CRM IA"
}

func corDoProduto() string {
	if c := utils.Cfg.CorPrimaria; len(c) == 7 && c[0] == '#' {
		return strings.ToUpper(c)
	}
	return "#9B52DF"
}

// emailLayout aplica o layout padrão do CRM IA aos e-mails transacionais.
func emailLayout(title, body string) string {
	cor := corDoProduto()
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<body style="margin:0;padding:0;background-color:#F5F3F8;font-family:Arial,Helvetica,sans-serif;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#F5F3F8;padding:32px 0;">
    <tr><td align="center">
      <table role="presentation" width="560" cellpadding="0" cellspacing="0" style="background:#FFFFFF;border-radius:12px;overflow:hidden;">
        <tr><td style="background:%s;padding:24px 32px;">
          <span style="color:#FFFFFF;font-size:20px;font-weight:bold;">%s</span>
        </td></tr>
        <tr><td style="padding:32px;">
          <h2 style="margin:0 0 16px;color:#222222;font-size:18px;">%s</h2>
          <div style="color:#464646;font-size:14px;line-height:1.6;">%s</div>
        </td></tr>
        <tr><td style="padding:16px 32px;background:#F2E8FA;color:#8E8E8E;font-size:12px;">
          %s &middot; e-mail automático, não responda.
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, cor, nomeDoProduto(), title, body, nomeDoProduto())
}

func botao(url, texto string) string {
	return fmt.Sprintf(`<p><a href="%s" style="display:inline-block;background:%s;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">%s</a></p>`, url, corDoProduto(), texto)
}

// WelcomeEmail monta o e-mail de boas-vindas com a senha temporária do novo usuário.
func WelcomeEmail(name, tempPassword, appURL string) (subject, html string) {
	produto := nomeDoProduto()
	subject = "Bem-vindo ao " + produto
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>!</p>
		<p>Sua conta no %s foi criada. Acesse com seu e-mail e a senha temporária abaixo:</p>
		<p style="font-size:18px;font-weight:bold;letter-spacing:1px;color:%s;">%s</p>
		<p>No primeiro acesso você define uma senha definitiva.</p>%s`,
		name, produto, corDoProduto(), tempPassword, botao(appURL, "Acessar o "+produto))
	return subject, emailLayout(subject, body)
}

// ResetPasswordEmail monta o e-mail de redefinição de senha.
func ResetPasswordEmail(name, token, appURL string) (subject, html string) {
	produto := nomeDoProduto()
	subject = "Redefinição de senha - " + produto
	link := fmt.Sprintf("%s/redefinir-senha?token=%s", strings.TrimRight(appURL, "/"), token)
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>.</p>
		<p>Recebemos um pedido para redefinir sua senha no %s. O link abaixo é válido por 2 horas:</p>%s
		<p>Se você não solicitou, ignore este e-mail.</p>`, name, produto, botao(link, "Redefinir senha"))
	return subject, emailLayout(subject, body)
}

// TestEmail é a mensagem do botão "enviar teste" das configurações de e-mail.
func TestEmail(provedor string) (subject, html string) {
	subject = "Teste de envio - " + nomeDoProduto()
	body := fmt.Sprintf(`<p>Se você está lendo isto, o envio de e-mails do seu CRM está funcionando.</p>
		<p>Provedor ativo: <strong>%s</strong>.</p>
		<p>Enviado em %s.</p>`, provedor, time.Now().Format("02/01/2006 15:04:05"))
	return subject, emailLayout(subject, body)
}

// LogEnvioFalhou padroniza o log de falha de envio (nunca interrompe o fluxo).
func LogEnvioFalhou(contexto string, err error) {
	log.Printf("e-mail (%s): falha no envio: %v", contexto, err)
}
