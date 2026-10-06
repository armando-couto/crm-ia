package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/armando-couto/goutils"
	m "github.com/keighl/mandrill"
)

// Mailer abstrai o envio de e-mails para permitir testes sem chamadas externas.
type Mailer interface {
	Send(toEmail, toName, subject, html string) error
}

// Mail é a instância global usada pelos controllers; inicializada no application.go.
var Mail Mailer

// MandrillMailer envia e-mails transacionais via Mandrill, mesmo padrão do send-email.
type MandrillMailer struct {
	client   *m.Client
	from     string
	fromName string
	env      string
	emailDev string
}

func NewMandrillMailer() (*MandrillMailer, error) {
	key := goutils.Godotenv("mandrill")
	if key == "" {
		return nil, errors.New("chave mandrill não encontrada no .env")
	}
	from := goutils.Godotenv("from")
	if from == "" {
		return nil, errors.New("chave from não encontrada no .env")
	}
	return &MandrillMailer{
		client:   m.ClientWithKey(key),
		from:     from,
		fromName: "CRM IA",
		env:      strings.ToLower(goutils.Godotenv("env")),
		emailDev: strings.ToLower(goutils.Godotenv("email_dev")),
	}, nil
}

// Send envia o e-mail. Fora de produção, redireciona para email_dev (padrão da casa).
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

// emailLayout aplica o layout padrão do CRM IA aos e-mails transacionais.
func emailLayout(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<body style="margin:0;padding:0;background-color:#F5F3F8;font-family:Arial,Helvetica,sans-serif;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#F5F3F8;padding:32px 0;">
    <tr><td align="center">
      <table role="presentation" width="560" cellpadding="0" cellspacing="0" style="background:#FFFFFF;border-radius:12px;overflow:hidden;">
        <tr><td style="background:#9B52DF;padding:24px 32px;">
          <span style="color:#FFFFFF;font-size:20px;font-weight:bold;">CRM IA</span>
        </td></tr>
        <tr><td style="padding:32px;">
          <h2 style="margin:0 0 16px;color:#222222;font-size:18px;">%s</h2>
          <div style="color:#464646;font-size:14px;line-height:1.6;">%s</div>
        </td></tr>
        <tr><td style="padding:16px 32px;background:#F2E8FA;color:#8E8E8E;font-size:12px;">
          CRM IA &middot; e-mail automático, não responda.
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, title, body)
}

// WelcomeEmail monta o e-mail de boas-vindas com a senha temporária do novo usuário.
func WelcomeEmail(name, tempPassword, appURL string) (subject, html string) {
	subject = "Bem-vindo ao CRM IA"
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>!</p>
		<p>Sua conta no CRM IA foi criada. Acesse com seu e-mail e a senha temporária abaixo:</p>
		<p style="font-size:18px;font-weight:bold;letter-spacing:1px;color:#9B52DF;">%s</p>
		<p>Recomendamos alterar a senha no primeiro acesso.</p>
		<p><a href="%s" style="display:inline-block;background:#9B52DF;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">Acessar o CRM IA</a></p>`,
		name, tempPassword, appURL)
	return subject, emailLayout(subject, body)
}

// ResetPasswordEmail monta o e-mail de redefinição de senha.
func ResetPasswordEmail(name, token, appURL string) (subject, html string) {
	subject = "Redefinição de senha - CRM IA"
	link := fmt.Sprintf("%s/redefinir-senha?token=%s", strings.TrimRight(appURL, "/"), token)
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>.</p>
		<p>Recebemos um pedido para redefinir sua senha no CRM IA. O link abaixo é válido por 2 horas:</p>
		<p><a href="%s" style="display:inline-block;background:#9B52DF;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">Redefinir senha</a></p>
		<p>Se você não solicitou, ignore este e-mail.</p>`, name, link)
	return subject, emailLayout(subject, body)
}
