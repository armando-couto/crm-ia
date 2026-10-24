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

// -----------------------------------------------------------------------------
// Inteligência de empresas: fontes de enriquecimento (Receita Federal grátis,
// BigDataCorp e Serasa pagos). Segredos cifrados; a API devolve has_secret.
// -----------------------------------------------------------------------------

const SettingEnrichment = "enrichment"

// Códigos das fontes.
const (
	ProvedorReceita     = "receita"
	ProvedorBigDataCorp = "bigdatacorp"
	ProvedorSerasa      = "serasa"
)

// BigDataCorpDatasets são os conjuntos de dados vendidos por consulta, com o
// preço de referência da 1ª faixa em centavos (só para estimativa).
var BigDataCorpDatasets = []struct {
	Codigo, Nome, Descricao string
	PrecoCentavos           int
}{
	{"basic_data", "Dados básicos", "razão social, situação, abertura, natureza e CNAE", 3},
	{"registration_data", "Dados cadastrais", "endereço, telefone e e-mail cadastrais", 14},
	{"activity_indicators", "Indicadores de atividade", "faixa de funcionários, faturamento presumido, nível de atividade, domínio/SSL/e-mail corporativo", 6},
	{"relationships", "Sócios e vínculos", "quadro societário com nível e datas", 4},
	{"phones_extended", "Telefones validados", "telefones encontrados com validação", 6},
	{"emails_extended", "E-mails validados", "e-mails encontrados com validação", 6},
	{"addresses_extended", "Endereços", "endereços adicionais", 6},
	{"domains_extended", "Domínios", "domínios da empresa, SSL e atividade", 6},
	{"processes", "Processos", "processos judiciais (resumo e últimos)", 8},
	{"kyc", "KYC e sanções", "pessoas politicamente expostas e listas de sanções", 6},
	{"media_profile_and_exposure", "Mídia", "exposição na imprensa e notícias", 10},
}

// BigDataCorpDatasetsPadrao é o conjunto sugerido (≈ R$ 0,27).
var BigDataCorpDatasetsPadrao = []string{"basic_data", "registration_data", "activity_indicators", "relationships"}

// PrecosBigDataCorpPadrao devolve a tabela de preços de referência.
func PrecosBigDataCorpPadrao() map[string]int {
	m := map[string]int{}
	for _, d := range BigDataCorpDatasets {
		m[d.Codigo] = d.PrecoCentavos
	}
	return m
}

// BigDataCorpDatasetValido diz se o código é um dataset conhecido.
func BigDataCorpDatasetValido(codigo string) bool {
	for _, d := range BigDataCorpDatasets {
		if d.Codigo == codigo {
			return true
		}
	}
	return false
}

// BigDataCorpSettings é a configuração do provedor BigDataCorp.
type BigDataCorpSettings struct {
	Habilitado bool   `json:"habilitado"`
	Demo       bool   `json:"demo"`
	TokenID    string `json:"token_id"`
	// AccessToken é só de entrada (PUT): vazio mantém o já gravado.
	AccessToken        string         `json:"access_token,omitempty"`
	AccessTokenCifrado string         `json:"access_token_cifrado,omitempty"`
	HasSecret          bool           `json:"has_secret"`
	DatasetsPadrao     []string       `json:"datasets_padrao"`
	PrecosCentavos     map[string]int `json:"precos_centavos"`
	// BaseURL permite apontar para outro endereço (homologação, testes).
	BaseURL string `json:"base_url,omitempty"`
}

// SerasaSettings é a configuração do provedor Serasa Experian.
type SerasaSettings struct {
	Habilitado bool   `json:"habilitado"`
	Demo       bool   `json:"demo"`
	Ambiente   string `json:"ambiente"` // uat|prod
	ClientID   string `json:"client_id"`
	// ClientSecret é só de entrada (PUT): vazio mantém o já gravado.
	ClientSecret        string `json:"client_secret,omitempty"`
	ClientSecretCifrado string `json:"client_secret_cifrado,omitempty"`
	HasSecret           bool   `json:"has_secret"`
	ReportName          string `json:"report_name"`
	OptionalFeatures    string `json:"optional_features"`
	PrecoCentavos       int    `json:"preco_centavos"`
	// BaseURL substitui o endereço do ambiente; ReportPath é o template do
	// caminho do relatório ({report}, {features}, {cnpj}) — o parâmetro exato
	// do documento não é público e pode ser ajustado sem mexer no código.
	BaseURL    string `json:"base_url,omitempty"`
	ReportPath string `json:"report_path,omitempty"`
}

// SerasaReportPathPadrao é o template padrão do relatório.
const SerasaReportPathPadrao = "/credit-services/business-information-report/v1/reports?reportName={report}&optionalFeatures={features}&documento={cnpj}"

// EnrichmentSettings é a configuração de enriquecimento (chave "enrichment").
type EnrichmentSettings struct {
	AutoReceita          bool                `json:"auto_receita"`
	Sobrescrever         string              `json:"sobrescrever"` // vazios|sempre
	LimiteMensalCentavos int                 `json:"limite_mensal_centavos"`
	PrazoCacheDias       int                 `json:"prazo_cache_dias"`
	BigDataCorp          BigDataCorpSettings `json:"bigdatacorp"`
	Serasa               SerasaSettings      `json:"serasa"`
}

func DefaultEnrichmentSettings() EnrichmentSettings {
	return EnrichmentSettings{
		AutoReceita:          true,
		Sobrescrever:         "vazios",
		LimiteMensalCentavos: 20000,
		PrazoCacheDias:       30,
		BigDataCorp: BigDataCorpSettings{
			DatasetsPadrao: append([]string{}, BigDataCorpDatasetsPadrao...),
			PrecosCentavos: PrecosBigDataCorpPadrao(),
		},
		Serasa: SerasaSettings{
			Ambiente:      "uat",
			ReportName:    "RELATORIO_AVANCADO_PJ_PME",
			PrecoCentavos: 1500,
		},
	}
}

// LoadEnrichmentSettings lê a configuração, completando o que faltar com o padrão.
func LoadEnrichmentSettings(db *sql.DB) EnrichmentSettings {
	s := DefaultEnrichmentSettings()
	if db != nil {
		_, _ = GetSetting(db, SettingEnrichment, &s)
	}
	s.completar()
	return s
}

func (s *EnrichmentSettings) completar() {
	if s.Sobrescrever != "sempre" {
		s.Sobrescrever = "vazios"
	}
	if s.PrazoCacheDias <= 0 {
		s.PrazoCacheDias = 30
	}
	if s.LimiteMensalCentavos < 0 {
		s.LimiteMensalCentavos = 0
	}
	if len(s.BigDataCorp.DatasetsPadrao) == 0 {
		s.BigDataCorp.DatasetsPadrao = append([]string{}, BigDataCorpDatasetsPadrao...)
	}
	precos := PrecosBigDataCorpPadrao()
	for k, v := range s.BigDataCorp.PrecosCentavos {
		if v >= 0 {
			precos[k] = v
		}
	}
	s.BigDataCorp.PrecosCentavos = precos
	s.BigDataCorp.HasSecret = s.BigDataCorp.AccessTokenCifrado != ""
	if s.Serasa.Ambiente != "prod" {
		s.Serasa.Ambiente = "uat"
	}
	if s.Serasa.ReportName == "" {
		s.Serasa.ReportName = "RELATORIO_AVANCADO_PJ_PME"
	}
	if s.Serasa.PrecoCentavos < 0 {
		s.Serasa.PrecoCentavos = 0
	}
	s.Serasa.HasSecret = s.Serasa.ClientSecretCifrado != ""
}

// Validate normaliza e confere a configuração recebida da tela.
func (s *EnrichmentSettings) Validate() error {
	s.Sobrescrever = strings.ToLower(strings.TrimSpace(s.Sobrescrever))
	if s.Sobrescrever == "" {
		s.Sobrescrever = "vazios"
	}
	if s.Sobrescrever != "vazios" && s.Sobrescrever != "sempre" {
		return fmt.Errorf("regra de sobrescrita inválida (vazios ou sempre)")
	}
	if s.LimiteMensalCentavos < 0 {
		return fmt.Errorf("o limite mensal não pode ser negativo")
	}
	if s.PrazoCacheDias < 1 || s.PrazoCacheDias > 365 {
		return fmt.Errorf("o prazo do cache deve ficar entre 1 e 365 dias")
	}
	s.BigDataCorp.TokenID = strings.TrimSpace(s.BigDataCorp.TokenID)
	s.BigDataCorp.BaseURL = strings.TrimRight(strings.TrimSpace(s.BigDataCorp.BaseURL), "/")
	ds := []string{}
	for _, d := range s.BigDataCorp.DatasetsPadrao {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if !BigDataCorpDatasetValido(d) {
			return fmt.Errorf("conjunto de dados desconhecido: %s", d)
		}
		ds = append(ds, d)
	}
	s.BigDataCorp.DatasetsPadrao = ds
	for k, v := range s.BigDataCorp.PrecosCentavos {
		if !BigDataCorpDatasetValido(k) {
			return fmt.Errorf("conjunto de dados desconhecido na tabela de preços: %s", k)
		}
		if v < 0 {
			return fmt.Errorf("preço inválido para %s", k)
		}
	}
	s.Serasa.Ambiente = strings.ToLower(strings.TrimSpace(s.Serasa.Ambiente))
	if s.Serasa.Ambiente == "" {
		s.Serasa.Ambiente = "uat"
	}
	if s.Serasa.Ambiente != "uat" && s.Serasa.Ambiente != "prod" {
		return fmt.Errorf("ambiente Serasa inválido (uat ou prod)")
	}
	s.Serasa.ClientID = strings.TrimSpace(s.Serasa.ClientID)
	s.Serasa.ReportName = strings.TrimSpace(s.Serasa.ReportName)
	s.Serasa.OptionalFeatures = strings.TrimSpace(s.Serasa.OptionalFeatures)
	s.Serasa.BaseURL = strings.TrimRight(strings.TrimSpace(s.Serasa.BaseURL), "/")
	s.Serasa.ReportPath = strings.TrimSpace(s.Serasa.ReportPath)
	if s.Serasa.PrecoCentavos < 0 {
		return fmt.Errorf("o preço da consulta Serasa não pode ser negativo")
	}
	for _, u := range []string{s.BigDataCorp.BaseURL, s.Serasa.BaseURL} {
		if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			return fmt.Errorf("endereço base inválido: %s", u)
		}
	}
	s.completar()
	return nil
}

// Publica devolve a cópia segura para a API: sem segredos, só has_secret.
func (s EnrichmentSettings) Publica() EnrichmentSettings {
	s.completar()
	s.BigDataCorp.AccessToken = ""
	s.BigDataCorp.AccessTokenCifrado = ""
	s.Serasa.ClientSecret = ""
	s.Serasa.ClientSecretCifrado = ""
	return s
}

// ParaGravar limpa os campos só de entrada antes de ir ao banco.
func (s EnrichmentSettings) ParaGravar() EnrichmentSettings {
	s.BigDataCorp.AccessToken = ""
	s.Serasa.ClientSecret = ""
	s.completar()
	return s
}

// CustoBigDataCorp estima o custo (centavos) dos datasets pedidos.
func (s EnrichmentSettings) CustoBigDataCorp(datasets []string) int {
	total := 0
	for _, d := range datasets {
		total += s.BigDataCorp.PrecosCentavos[d]
	}
	return total
}

// BigDataCorpPronto diz se o provedor pode ser usado (credenciais ou demo).
func (s EnrichmentSettings) BigDataCorpPronto() bool {
	b := s.BigDataCorp
	return b.Habilitado && (b.Demo || (b.TokenID != "" && b.AccessTokenCifrado != ""))
}

// SerasaPronto diz se o provedor pode ser usado (credenciais ou demo).
func (s EnrichmentSettings) SerasaPronto() bool {
	r := s.Serasa
	return r.Habilitado && (r.Demo || (r.ClientID != "" && r.ClientSecretCifrado != ""))
}

// NomeProvedor devolve o nome amigável da fonte.
func NomeProvedor(codigo string) string {
	switch codigo {
	case ProvedorReceita:
		return "Receita Federal"
	case ProvedorBigDataCorp:
		return "BigDataCorp"
	case ProvedorSerasa:
		return "Serasa Experian"
	}
	return codigo
}
