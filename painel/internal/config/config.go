// Package config lê o ambiente do painel administrativo do CRM IA.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Ambiente string
	Porta    string
	BasePath string
	Dominio  string
	// SiteURL: para onde a marca do painel leva (a landing, na raiz do domínio).
	SiteURL string

	URLBanco    string
	ChaveSessao string
	ChaveCSRF   string

	AdminEmail string
	AdminSenha string

	// DiasAtrasoSuspensao: tolerância depois do vencimento antes de suspender.
	DiasAtrasoSuspensao int

	ProvisionadorURL   string
	ProvisionadorToken string
	// TenantAPIMolde: como alcançar a API de um cliente pela rede interna (%s = slug).
	TenantAPIMolde string

	// Cobrança recorrente (Fix Pay Sign). As credenciais são da PRÓPRIA CRM
	// IA como estabelecimento; o cliente final nunca as vê.
	SignTokenAPI string // FIXPAY_TOKEN_API
	SignApisURL  string // FIXPAY_APIS_URL   (apis.fixpay.com.br)
	SignGateway  string // FIXPAY_SIGN_URL   (signpayments.fixpay.com.br — checkout por integração)
	SignGrupoID  int64  // FIXPAY_SIGN_GRUPO_ID
	SignSimulado bool   // FIXPAY_SIMULADO

	// E-mail transacional do painel (avisos de acesso e de pagamento).
	MandrillChave, FromEmail, FromNome string

	// Nome comercial e contatos exibidos nas páginas públicas (checkout).
	NomeProduto     string
	EmailSuporte    string
	WhatsAppSuporte string
}

func (c Config) EmDesenvolvimento() bool { return c.Ambiente == "development" }

// URLPublica monta um endereço absoluto do painel.
func (c Config) URLPublica(caminho string) string {
	esquema := "https://"
	if strings.HasPrefix(c.Dominio, "http://") || strings.HasPrefix(c.Dominio, "https://") {
		esquema = ""
	}
	if !strings.HasPrefix(caminho, "/") {
		caminho = "/" + caminho
	}
	return esquema + strings.TrimRight(c.Dominio, "/") + c.BasePath + caminho
}

func Carregar() (Config, error) {
	c := Config{
		Ambiente: texto("APP_ENV", "production"),
		Porta:    texto("PORT", "7894"),
		BasePath: texto("BASE_PATH", "/painel"),
		Dominio:  texto("DOMAIN", "localhost"),
		SiteURL:  texto("SITE_URL", "/"),

		URLBanco:    os.Getenv("DATABASE_URL"),
		ChaveSessao: texto("SESSION_KEY", os.Getenv("PAINEL_SESSION_KEY")),
		ChaveCSRF:   texto("CSRF_KEY", os.Getenv("PAINEL_CSRF_KEY")),

		AdminEmail: texto("ADMIN_EMAIL", texto("PAINEL_ADMIN_EMAIL", "admin@crmia.local")),
		AdminSenha: texto("ADMIN_SENHA", os.Getenv("PAINEL_ADMIN_SENHA")),

		DiasAtrasoSuspensao: inteiro("DIAS_ATRASO_SUSPENSAO", 10),

		ProvisionadorURL:   texto("PROVISIONER_URL", "http://provisioner:7898"),
		ProvisionadorToken: os.Getenv("PROVISIONER_TOKEN"),
		TenantAPIMolde:     texto("TENANT_API_MOLDE", "http://ci-%s-app:8080/api"),

		SignTokenAPI: os.Getenv("FIXPAY_TOKEN_API"),
		SignApisURL:  texto("FIXPAY_APIS_URL", "https://apis.fixpay.com.br"),
		SignGateway:  texto("FIXPAY_SIGN_URL", "https://signpayments.fixpay.com.br"),
		SignGrupoID:  int64(inteiro("FIXPAY_SIGN_GRUPO_ID", 0)),
		SignSimulado: booleano("FIXPAY_SIMULADO", false),

		MandrillChave: os.Getenv("MANDRILL_API_KEY"),
		FromEmail:     texto("MANDRILL_FROM_EMAIL", "nao-responda@crmia.local"),
		FromNome:      texto("MANDRILL_FROM_NOME", "CRM IA"),

		NomeProduto:     texto("NOME_PRODUTO", "CRM IA"),
		EmailSuporte:    texto("EMAIL_COMERCIAL", "contato@crmia.local"),
		WhatsAppSuporte: os.Getenv("WHATSAPP_COMERCIAL"),
	}
	if c.URLBanco == "" {
		return c, errors.New("DATABASE_URL não definido")
	}
	if len(c.ChaveSessao) < 32 || len(c.ChaveCSRF) < 32 {
		return c, errors.New("SESSION_KEY e CSRF_KEY precisam ter ao menos 32 caracteres (openssl rand -hex 32)")
	}
	if c.ProvisionadorToken == "" {
		return c, errors.New("PROVISIONER_TOKEN não definido")
	}
	if c.BasePath == "/" {
		c.BasePath = ""
	}
	c.BasePath = strings.TrimRight(c.BasePath, "/")
	return c, nil
}

func texto(nome, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		return v
	}
	return padrao
}

func inteiro(nome string, padrao int) int {
	if v := os.Getenv(nome); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return padrao
}

func booleano(nome string, padrao bool) bool {
	if v := os.Getenv(nome); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return padrao
}
