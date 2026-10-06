// Package config lê o ambiente do provisionador.
package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

// Config é tudo que o provisionador precisa para subir a stack de um cliente.
type Config struct {
	Ambiente string
	Porta    string
	Token    string

	Dominio      string
	RedePublica  string
	TenantsDir   string
	TemplatePath string
	BackupsDir   string

	// Repositórios das imagens (sem tag). A tag é a versão escolhida por
	// cliente — é o que permite um cliente ficar numa versão e outro em outra.
	ImagemApp      string
	ImagemDB       string
	ImagemRedis    string
	ImagemSuspenso string
	VersaoPadrao   string

	// Credencial do registro (quando as imagens são privadas).
	RegistroUsuario string
	RegistroToken   string

	CPULimite      string
	MemLimite      string
	DBMemLimite    string
	RedisMemLimite string

	// Intervalo entre consultas ao healthcheck do container durante a subida.
	EsperaHealthcheck time.Duration

	// PainelURL: como o app do cliente alcança o painel pela rede interna
	// (Meu plano). Vai para o container como PAINEL_URL.
	PainelURL string

	// E-mail padrão repassado aos ambientes (o cliente pode trocar na tela).
	MandrillChave, FromEmail, FromNome string
}

func (c Config) EmDesenvolvimento() bool { return c.Ambiente == "development" }

// Carregar lê o ambiente e valida o essencial.
func Carregar() (Config, error) {
	c := Config{
		Ambiente:     texto("APP_ENV", "production"),
		Porta:        texto("PORT", "7898"),
		Token:        os.Getenv("PROVISIONER_TOKEN"),
		Dominio:      texto("DOMAIN", "localhost"),
		RedePublica:  texto("REDE_PUBLICA", "crmia_public"),
		TenantsDir:   texto("TENANTS_DIR", "/tenants"),
		TemplatePath: texto("TEMPLATE_PATH", "/templates/docker-compose.tenant.tmpl"),
		BackupsDir:   texto("BACKUPS_DIR", "/backups"),

		ImagemApp:      texto("TENANT_IMAGE_APP", "crmia/app"),
		ImagemDB:       texto("TENANT_IMAGE_DB", "postgres:16-alpine"),
		ImagemRedis:    texto("TENANT_IMAGE_REDIS", "redis:7-alpine"),
		ImagemSuspenso: texto("TENANT_IMAGE_SUSPENSO", "crmia/suspenso:latest"),
		VersaoPadrao:   texto("TENANT_VERSAO_PADRAO", "latest"),

		RegistroUsuario: os.Getenv("REGISTRY_USER"),
		RegistroToken:   os.Getenv("REGISTRY_TOKEN"),

		CPULimite:      texto("TENANT_CPU_LIMIT", "1.0"),
		MemLimite:      texto("TENANT_MEM_LIMIT", "768m"),
		DBMemLimite:    texto("TENANT_DB_MEM_LIMIT", "512m"),
		RedisMemLimite: texto("TENANT_REDIS_MEM_LIMIT", "128m"),

		EsperaHealthcheck: duracao("TENANT_ESPERA_HEALTHCHECK", 2*time.Second),

		PainelURL: texto("PAINEL_URL", "http://painel:7894/painel"),

		MandrillChave: os.Getenv("MANDRILL_API_KEY"),
		FromEmail:     texto("MANDRILL_FROM_EMAIL", ""),
		FromNome:      texto("MANDRILL_FROM_NOME", "CRM IA"),
	}
	if c.Token == "" {
		return c, errors.New("PROVISIONER_TOKEN não definido")
	}
	if len(c.Token) < 24 {
		return c, errors.New("PROVISIONER_TOKEN curto demais; gere com: openssl rand -hex 32")
	}
	return c, nil
}

func texto(nome, padrao string) string {
	if v := os.Getenv(nome); v != "" {
		return v
	}
	return padrao
}

func duracao(nome string, padrao time.Duration) time.Duration {
	if v := os.Getenv(nome); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return padrao
}

// Inteiro lê um inteiro do ambiente (exportado para os testes de config).
func Inteiro(nome string, padrao int) int {
	if v := os.Getenv(nome); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return padrao
}
