package utils

import (
	"database/sql"
	"strings"

	"github.com/armando-couto/goutils"
)

// DB é a conexão global com o PostgreSQL, inicializada no application.go.
var DB *sql.DB

// JWTSecret é carregado do .env (chave jwt_secret) na inicialização.
var JWTSecret string

// AppURL é a URL pública do CRM, usada nos links dos e-mails.
var AppURL string

// MandrillWebhookKey e MandrillWebhookURL validam a assinatura do webhook de
// entrada. Sem a chave o endpoint recusa todas as requisições.
var (
	MandrillWebhookKey string
	MandrillWebhookURL string
)

// LoadConfig carrega as configurações obrigatórias do ambiente.
func LoadConfig() {
	JWTSecret = goutils.Godotenv("jwt_secret")
	AppURL = goutils.Godotenv("app_url")
	if AppURL == "" {
		AppURL = "http://localhost:9000"
	}
	MandrillWebhookKey = goutils.Godotenv("mandrill_webhook_key")
	MandrillWebhookURL = goutils.Godotenv("mandrill_webhook_url")
	if MandrillWebhookURL == "" {
		MandrillWebhookURL = strings.TrimSuffix(AppURL, "/") + "/api/webhooks/mandrill/inbound"
	}
}
