package utils

import (
	"database/sql"

	"github.com/armando-couto/goutils"
)

// DB é a conexão global com o PostgreSQL, inicializada no application.go.
var DB *sql.DB

// JWTSecret é carregado do .env (chave jwt_secret) na inicialização.
var JWTSecret string

// AppURL é a URL pública do CRM, usada nos links dos e-mails.
var AppURL string

// LoadConfig carrega as configurações obrigatórias do ambiente.
func LoadConfig() {
	JWTSecret = goutils.Godotenv("jwt_secret")
	AppURL = goutils.Godotenv("app_url")
	if AppURL == "" {
		AppURL = "http://localhost:9000"
	}
}
