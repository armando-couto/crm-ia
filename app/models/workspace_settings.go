package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// Configurações do ambiente que o cliente define no setup inicial e pode
// mudar depois em Configurações. Tudo em app_settings (chave → JSON).
// -----------------------------------------------------------------------------

const (
	SettingWorkspace = "workspace"
	SettingEmail     = "email"
	SettingSending   = "sending"
	SettingSetup     = "setup"
)

// WorkspaceSettings é a identidade da empresa dentro do CRM.
type WorkspaceSettings struct {
	Name     string `json:"name"`
	Segment  string `json:"segment"`
	Color    string `json:"color"`
	LogoURL  string `json:"logo_url"`
	Website  string `json:"website"`
	Timezone string `json:"timezone"`
}

func LoadWorkspaceSettings(db *sql.DB, padraoNome, padraoCor, padraoLogo string) WorkspaceSettings {
	w := WorkspaceSettings{Name: padraoNome, Color: padraoCor, LogoURL: padraoLogo, Timezone: "America/Sao_Paulo"}
	_, _ = GetSetting(db, SettingWorkspace, &w)
	if w.Name == "" {
		w.Name = padraoNome
	}
	if w.Color == "" {
		w.Color = padraoCor
	}
	return w
}

// ValidateWorkspace confere nome e cor.
func (w *WorkspaceSettings) Validate() error {
	w.Name = strings.TrimSpace(w.Name)
	w.Color = strings.ToLower(strings.TrimSpace(w.Color))
	if w.Name == "" {
		return fmt.Errorf("informe o nome da empresa")
	}
	if len(w.Name) > 80 {
		return fmt.Errorf("o nome da empresa é longo demais")
	}
	if w.Color != "" && !corHexValida(w.Color) {
		return fmt.Errorf("cor inválida: use o formato #rrggbb")
	}
	if w.LogoURL != "" && !strings.HasPrefix(w.LogoURL, "https://") && !strings.HasPrefix(w.LogoURL, "http://") {
		return fmt.Errorf("a URL da logo precisa começar com http:// ou https://")
	}
	return nil
}

func corHexValida(c string) bool {
	if len(c) != 7 || c[0] != '#' {
		return false
	}
	for _, r := range c[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// Provedores de e-mail que o cliente pode escolher.
const (
	EmailProviderPlatform     = "plataforma"    // o padrão do ambiente (sem configuração própria)
	EmailProviderMandrillAPI  = "mandrill_api"  // Mandrill pela API HTTPS
	EmailProviderMandrillSMTP = "mandrill_smtp" // smtp.mandrillapp.com
	EmailProviderMailerooSMTP = "maileroo_smtp" // smtp.maileroo.com
	EmailProviderSMTP         = "smtp"          // qualquer servidor SMTP
)

// EmailPreset são os dados fixos de cada provedor conhecido.
type EmailPreset struct {
	Nome string
	Host string
	Port int
	// Dica mostrada na tela sobre onde pegar a credencial.
	Dica string
}

var EmailPresets = map[string]EmailPreset{
	EmailProviderMandrillAPI:  {Nome: "Mandrill", Dica: "Mandrill → Settings → SMTP & API Info → New API Key. O domínio do remetente precisa ter SPF e DKIM verificados."},
	EmailProviderMandrillSMTP: {Nome: "Mandrill", Host: "smtp.mandrillapp.com", Port: 587, Dica: "Usuário: qualquer texto (use o e-mail do remetente). Senha: uma chave de API do Mandrill."},
	EmailProviderMailerooSMTP: {Nome: "Maileroo", Host: "smtp.maileroo.com", Port: 587, Dica: "Maileroo → Sending Domains → SMTP credentials. Usuário e senha SMTP do domínio verificado."},
	EmailProviderSMTP:         {Nome: "SMTP", Dica: "Servidor com STARTTLS (porta 587) ou TLS (porta 465)."},
}

func ValidEmailProvider(p string) bool {
	switch p {
	case EmailProviderPlatform, EmailProviderMandrillAPI, EmailProviderMandrillSMTP, EmailProviderMailerooSMTP, EmailProviderSMTP:
		return true
	}
	return false
}

// EmailSettings é a configuração de envio do cliente. O segredo (chave de API
// ou senha SMTP) fica cifrado no banco e nunca volta para a tela.
type EmailSettings struct {
	Provider  string `json:"provider"`
	FromEmail string `json:"from_email"`
	FromName  string `json:"from_name"`
	ReplyTo   string `json:"reply_to"`
	SMTPHost  string `json:"smtp_host,omitempty"`
	SMTPPort  int    `json:"smtp_port,omitempty"`
	SMTPUser  string `json:"smtp_user,omitempty"`
	// SecretCifrado é a senha/chave em AES-GCM; na resposta da API vai só
	// HasSecret.
	SecretCifrado string `json:"secret_cifrado,omitempty"`
	HasSecret     bool   `json:"has_secret"`
	// Resultado do último teste.
	TestedAt   *time.Time `json:"tested_at,omitempty"`
	TestResult string     `json:"test_result,omitempty"`
}

func LoadEmailSettings(db *sql.DB) *EmailSettings {
	var e EmailSettings
	found, err := GetSetting(db, SettingEmail, &e)
	if err != nil || !found {
		return nil
	}
	e.HasSecret = e.SecretCifrado != ""
	return &e
}

// Publica devolve a cópia segura para a API (sem o segredo).
func (e EmailSettings) Publica() EmailSettings {
	e.HasSecret = e.SecretCifrado != ""
	e.SecretCifrado = ""
	return e
}

func (e *EmailSettings) Validate() error {
	e.Provider = strings.TrimSpace(e.Provider)
	e.FromEmail = NormalizeEmail(e.FromEmail)
	e.ReplyTo = NormalizeEmail(e.ReplyTo)
	e.FromName = strings.TrimSpace(e.FromName)
	e.SMTPHost = strings.TrimSpace(e.SMTPHost)
	e.SMTPUser = strings.TrimSpace(e.SMTPUser)
	if !ValidEmailProvider(e.Provider) {
		return fmt.Errorf("provedor de e-mail inválido")
	}
	if e.Provider == EmailProviderPlatform {
		return nil
	}
	if e.FromEmail == "" || !strings.Contains(e.FromEmail, "@") {
		return fmt.Errorf("informe um e-mail de remetente válido")
	}
	if e.Provider == EmailProviderSMTP {
		if e.SMTPHost == "" {
			return fmt.Errorf("informe o servidor SMTP")
		}
		if e.SMTPPort <= 0 || e.SMTPPort > 65535 {
			return fmt.Errorf("porta SMTP inválida")
		}
	}
	return nil
}

// SendingSettings são as regras de disparo automático.
type SendingSettings struct {
	// DailyLimit 0 = sem teto.
	DailyLimit int `json:"daily_limit"`
	// Janela em horas cheias (0–23) no fuso do ambiente; Start == End = dia inteiro.
	WindowStart  int    `json:"window_start"`
	WindowEnd    int    `json:"window_end"`
	WeekdaysOnly bool   `json:"weekdays_only"`
	Paused       bool   `json:"paused"`
	Signature    string `json:"signature"`
	TrackOpens   bool   `json:"track_opens"`
	TrackClicks  bool   `json:"track_clicks"`
	Timezone     string `json:"timezone"`
}

func DefaultSendingSettings() SendingSettings {
	return SendingSettings{DailyLimit: 500, WindowStart: 8, WindowEnd: 19, WeekdaysOnly: true,
		TrackOpens: true, TrackClicks: true, Timezone: "America/Sao_Paulo"}
}

func LoadSendingSettings(db *sql.DB) SendingSettings {
	s := DefaultSendingSettings()
	if db != nil {
		_, _ = GetSetting(db, SettingSending, &s)
	}
	return s
}

func (s *SendingSettings) Validate() error {
	if s.DailyLimit < 0 || s.DailyLimit > 100000 {
		return fmt.Errorf("teto diário inválido")
	}
	if s.WindowStart < 0 || s.WindowStart > 23 || s.WindowEnd < 0 || s.WindowEnd > 23 {
		return fmt.Errorf("a janela de horário usa horas de 0 a 23")
	}
	if len(s.Signature) > 2000 {
		return fmt.Errorf("a assinatura é longa demais")
	}
	if s.Timezone == "" {
		s.Timezone = "America/Sao_Paulo"
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("fuso horário desconhecido")
	}
	return nil
}

// DentroDaJanela diz se o instante cai na janela de disparo.
func (s SendingSettings) DentroDaJanela(agora time.Time) bool {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		loc = time.Local
	}
	t := agora.In(loc)
	if s.WeekdaysOnly && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
		return false
	}
	if s.WindowStart == s.WindowEnd {
		return true
	}
	h := t.Hour()
	if s.WindowStart < s.WindowEnd {
		return h >= s.WindowStart && h < s.WindowEnd
	}
	// Janela que cruza a meia-noite (ex.: 20 → 6).
	return h >= s.WindowStart || h < s.WindowEnd
}

// SetupState registra o andamento do assistente de configuração inicial.
type SetupState struct {
	Completed   bool       `json:"completed"`
	Template    string     `json:"template"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Steps       []string   `json:"steps"` // passos já concluídos
}

func LoadSetupState(db *sql.DB) SetupState {
	var s SetupState
	_, _ = GetSetting(db, SettingSetup, &s)
	if s.Steps == nil {
		s.Steps = []string{}
	}
	return s
}

// MarcarPasso acrescenta um passo concluído (idempotente).
func (s *SetupState) MarcarPasso(passo string) {
	for _, p := range s.Steps {
		if p == passo {
			return
		}
	}
	s.Steps = append(s.Steps, passo)
}

// CountActiveUsers conta os acessos ativos: é o número que a licença limita.
func CountActiveUsers(db *sql.DB) (int, error) {
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE active`).Scan(&total)
	return total, err
}
