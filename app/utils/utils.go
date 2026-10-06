// Package utils concentra a configuração do ambiente de um cliente do CRM IA.
//
// Uma imagem serve todos os clientes: tudo que distingue um ambiente do outro
// (banco, Redis, segredos, slug, nome, tema, licença) chega por variáveis de
// ambiente, injetadas pelo provisionador na stack do cliente. Em
// desenvolvimento um arquivo .env (ou .env.production no Linux) é carregado
// uma vez; o ambiente do processo sempre tem precedência.
package utils

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// DB é a conexão global com o PostgreSQL do cliente, aberta no application.go.
var DB *sql.DB

// JWTSecret, AppURL e as chaves do webhook são lidos no boot (LoadConfig) e
// usados pelos controllers; ficam como variáveis de pacote por compatibilidade.
var (
	JWTSecret          string
	AppURL             string
	MandrillWebhookKey string
	MandrillWebhookURL string
)

// Config é a leitura completa do ambiente.
type Config struct {
	Ambiente string // production | development | homologation
	Porta    string
	// AppURL é a raiz pública do ambiente (https://dominio/slug): links de
	// e-mail, rastreio de abertura/clique e formulários públicos saem daqui.
	AppURL string
	// BasePath é o prefixo do ambiente no domínio ("/slug"). O Traefik o
	// remove antes de chegar aqui; o SPA precisa dele para montar as rotas.
	BasePath string

	TenantSlug string
	TenantNome string
	TenantCNPJ string
	Versao     string

	DatabaseURL string
	RedisURL    string

	JWTSecret   string
	ChaveCripto string // AES-256-GCM (64 hex): cifra segredos guardados no banco (senha SMTP)
	// TokenInterno autentica o painel da plataforma nas rotas /api/interno.
	TokenInterno string
	// PainelURL: endereço interno do painel (rede do Docker) — "Meu plano".
	PainelURL string

	UsuariosMax int

	AdminNome  string
	AdminEmail string
	AdminSenha string

	CorPrimaria string
	LogoURL     string

	// E-mail padrão da plataforma (o cliente pode trocar em Configurações →
	// E-mail: Mandrill ou Maileroo, por API ou SMTP).
	MandrillChave string
	FromEmail     string
	FromName      string
	EmailDev      string
	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPass      string

	MandrillWebhookKey string
	MandrillWebhookURL string
}

// Cfg é a configuração carregada por LoadConfig.
var Cfg Config

var carregarEnv sync.Once

// Env devolve o primeiro valor não vazio entre os nomes informados. Aceita o
// nome em MAIÚSCULAS (padrão da plataforma) e o legado em minúsculas do .env.
func Env(nomes ...string) string {
	carregarEnv.Do(func() {
		arquivo := ".env"
		if runtime.GOOS == "linux" {
			if _, err := os.Stat(".env.production"); err == nil {
				arquivo = ".env.production"
			}
		}
		// Só preenche o que o ambiente não trouxe: variável do container vence.
		_ = godotenv.Load(arquivo)
	})
	for _, n := range nomes {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

func envInt(padrao int, nomes ...string) int {
	if v := Env(nomes...); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return padrao
}

// LoadConfig lê o ambiente e preenche Cfg e as variáveis de compatibilidade.
func LoadConfig() {
	c := Config{
		Ambiente:   strings.ToLower(Env("APP_ENV", "env")),
		Porta:      Env("PORT", "port_server"),
		AppURL:     strings.TrimRight(Env("APP_URL", "app_url"), "/"),
		BasePath:   Env("BASE_PATH", "base_path"),
		TenantSlug: Env("TENANT_SLUG", "tenant_slug"),
		TenantNome: Env("TENANT_NOME", "tenant_nome"),
		TenantCNPJ: Env("TENANT_CNPJ", "tenant_cnpj"),
		Versao:     Env("VERSAO", "versao"),

		DatabaseURL: Env("DATABASE_URL", "database_url"),
		RedisURL:    Env("REDIS_URL", "redis_url"),

		JWTSecret:    Env("JWT_SEGREDO", "JWT_SECRET", "jwt_secret"),
		ChaveCripto:  Env("CHAVE_CRIPTO", "chave_cripto"),
		TokenInterno: Env("TOKEN_INTERNO", "token_interno"),
		PainelURL:    strings.TrimRight(Env("PAINEL_URL", "painel_url"), "/"),

		UsuariosMax: envInt(0, "LICENCA_USUARIOS_MAX", "USUARIOS_MAX", "usuarios_max"),

		AdminNome:  Env("ADMIN_NOME", "admin_nome"),
		AdminEmail: Env("ADMIN_EMAIL", "admin_email"),
		AdminSenha: Env("ADMIN_SENHA", "ADMIN_PASSWORD", "admin_password"),

		CorPrimaria: Env("TEMA_COR_PRIMARIA", "tema_cor_primaria"),
		LogoURL:     Env("TEMA_LOGO_URL", "tema_logo_url"),

		MandrillChave: Env("MANDRILL_API_KEY", "mandrill"),
		FromEmail:     Env("MANDRILL_FROM_EMAIL", "FROM_EMAIL", "from"),
		FromName:      Env("MANDRILL_FROM_NOME", "FROM_NAME", "from_name"),
		EmailDev:      strings.ToLower(Env("EMAIL_DEV", "email_dev")),
		SMTPHost:      Env("SMTP_HOST", "smtp_host"),
		SMTPPort:      envInt(587, "SMTP_PORT", "smtp_port"),
		SMTPUser:      Env("SMTP_USER", "smtp_user"),
		SMTPPass:      Env("SMTP_PASS", "smtp_pass"),

		MandrillWebhookKey: Env("MANDRILL_WEBHOOK_KEY", "mandrill_webhook_key"),
		MandrillWebhookURL: Env("MANDRILL_WEBHOOK_URL", "mandrill_webhook_url"),
	}
	if c.Ambiente == "" {
		c.Ambiente = "development"
	}
	if c.Porta == "" {
		c.Porta = "8080"
	}
	if c.BasePath == "/" {
		c.BasePath = ""
	}
	if c.BasePath != "" && !strings.HasPrefix(c.BasePath, "/") {
		c.BasePath = "/" + c.BasePath
	}
	c.BasePath = strings.TrimRight(c.BasePath, "/")
	if c.AppURL == "" {
		c.AppURL = "http://localhost:" + c.Porta + c.BasePath
	}
	if c.TenantSlug == "" {
		c.TenantSlug = "dev"
	}
	if c.TenantNome == "" {
		c.TenantNome = "CRM IA"
	}
	if c.Versao == "" {
		c.Versao = "dev"
	}
	if c.CorPrimaria == "" {
		c.CorPrimaria = "#6d5df6"
	}
	if c.FromName == "" {
		c.FromName = c.TenantNome
	}
	if c.MandrillWebhookURL == "" {
		c.MandrillWebhookURL = c.AppURL + "/api/webhooks/mandrill/inbound"
	}

	Cfg = c
	JWTSecret = c.JWTSecret
	AppURL = c.AppURL
	MandrillWebhookKey = c.MandrillWebhookKey
	MandrillWebhookURL = c.MandrillWebhookURL
}

// Producao diz se os e-mails saem de verdade (fora dela vão para EMAIL_DEV).
func (c Config) Producao() bool { return c.Ambiente == "production" }

// URLPublica monta um endereço absoluto do ambiente a partir de um caminho.
func (c Config) URLPublica(caminho string) string {
	if !strings.HasPrefix(caminho, "/") {
		caminho = "/" + caminho
	}
	return c.AppURL + caminho
}

// DSN monta a string de conexão: DATABASE_URL quando existe; senão, as
// chaves legadas do .env (host, port_banco, user, password, dbname).
func (c Config) DSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	host := Env("host")
	if host == "" {
		host = "localhost"
	}
	porta := Env("port_banco")
	if porta == "" {
		porta = "5432"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable application_name=crmia",
		host, porta, Env("user"), Env("password"), Env("dbname"))
}

// ConectarBanco abre o PostgreSQL e espera ele responder (o banco do cliente
// sobe na mesma stack e pode levar alguns segundos).
func ConectarBanco(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(40)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)
	var ultimo error
	for tentativa := 1; tentativa <= 30; tentativa++ {
		if ultimo = db.Ping(); ultimo == nil {
			return db, nil
		}
		if tentativa == 1 || tentativa%5 == 0 {
			log.Printf("aguardando o PostgreSQL (%d/30): %v", tentativa, ultimo)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("PostgreSQL não respondeu: %w", ultimo)
}
