package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/armando-couto/goutils"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// TokenTTL define a validade do token de acesso.
const TokenTTL = 12 * time.Hour

var ErrInvalidToken = errors.New("token inválido ou expirado")

type Claims struct {
	UserID int64  `json:"uid"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateToken emite um JWT HS256 para o usuário autenticado.
func GenerateToken(u *models.User, secret string) (string, error) {
	if secret == "" {
		return "", errors.New("jwt_secret não configurado")
	}
	claims := Claims{
		UserID: u.ID,
		Name:   u.Name,
		Email:  u.Email,
		Role:   u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "crm-ia",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken valida o JWT e retorna as claims do usuário.
func ParseToken(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inesperado: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// RandomToken gera um token hexadecimal seguro (usado no reset de senha).
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SeedAdmin cria o usuário administrador inicial quando o banco está vazio.
// E-mail e senha vêm das chaves admin_email / admin_password do .env.
func SeedAdmin(db *sql.DB) error {
	total, err := models.CountUsers(db)
	if err != nil {
		return err
	}
	if total > 0 {
		return nil
	}

	email := goutils.Godotenv("admin_email")
	password := goutils.Godotenv("admin_password")
	if email == "" || password == "" {
		return errors.New("banco sem usuários: configure admin_email e admin_password no .env para criar o administrador inicial")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	admin := &models.User{
		Name:         "Administrador",
		Email:        email,
		Role:         models.RoleAdmin,
		Active:       true,
		PasswordHash: hash,
	}
	if err := models.CreateUser(db, admin); err != nil {
		return err
	}
	log.Printf("Usuário administrador inicial criado: %s", admin.Email)
	return nil
}

// JWTSecret retorna o segredo carregado na inicialização (utils.LoadConfig).
func JWTSecret() string {
	return utils.JWTSecret
}

// ResetAdmin (comando de manutenção ./crm-ia -reset-admin) redefine a senha
// do administrador com os valores admin_email/admin_password do .env:
// atualiza o usuário se ele existir (reativando e garantindo papel admin)
// ou o cria caso não exista. Útil quando a equipe fica sem acesso.
func ResetAdmin(db *sql.DB) error {
	email := goutils.Godotenv("admin_email")
	password := goutils.Godotenv("admin_password")
	if email == "" || password == "" {
		return errors.New("configure admin_email e admin_password no .env antes de rodar -reset-admin")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	user, err := models.UserByEmail(db, email)
	if err == sql.ErrNoRows {
		admin := &models.User{
			Name:         "Administrador",
			Email:        email,
			Role:         models.RoleAdmin,
			Active:       true,
			PasswordHash: hash,
		}
		if err := models.CreateUser(db, admin); err != nil {
			return err
		}
		log.Printf("Administrador criado: %s", admin.Email)
		return nil
	}
	if err != nil {
		return err
	}

	user.Role = models.RoleAdmin
	user.Active = true
	if err := models.UpdateUser(db, user); err != nil {
		return err
	}
	if err := models.UpdateUserPassword(db, user.ID, hash); err != nil {
		return err
	}
	log.Printf("Senha do administrador %s redefinida com o admin_password do .env", user.Email)
	return nil
}
