package services

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("senha-secreta")
	if err != nil {
		t.Fatalf("erro ao gerar hash: %v", err)
	}
	if hash == "senha-secreta" {
		t.Fatal("hash não pode ser igual à senha")
	}
	if !CheckPassword(hash, "senha-secreta") {
		t.Fatal("senha correta deveria validar")
	}
	if CheckPassword(hash, "senha-errada") {
		t.Fatal("senha incorreta não deveria validar")
	}
}

func TestGenerateAndParseToken(t *testing.T) {
	user := &models.User{ID: 42, Name: "Ana", Email: "ana@fixpay.com.br", Role: models.RoleGestor}

	token, err := GenerateToken(user, "segredo-de-teste")
	if err != nil {
		t.Fatalf("erro ao gerar token: %v", err)
	}

	claims, err := ParseToken(token, "segredo-de-teste")
	if err != nil {
		t.Fatalf("erro ao validar token: %v", err)
	}
	if claims.UserID != 42 || claims.Email != "ana@fixpay.com.br" || claims.Role != models.RoleGestor {
		t.Fatalf("claims inesperadas: %+v", claims)
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	user := &models.User{ID: 1, Email: "a@b.c", Role: models.RoleVendedor}
	token, _ := GenerateToken(user, "segredo-a")

	if _, err := ParseToken(token, "segredo-b"); err == nil {
		t.Fatal("token assinado com outro segredo deveria falhar")
	}
}

func TestParseTokenExpired(t *testing.T) {
	claims := Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("s"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(token, "s"); err == nil {
		t.Fatal("token expirado deveria falhar")
	}
}

func TestGenerateTokenWithoutSecret(t *testing.T) {
	user := &models.User{ID: 1}
	if _, err := GenerateToken(user, ""); err == nil {
		t.Fatal("gerar token sem segredo deveria falhar")
	}
}

func TestRandomToken(t *testing.T) {
	a, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RandomToken()
	if len(a) != 64 {
		t.Fatalf("token deveria ter 64 caracteres hex, tem %d", len(a))
	}
	if a == b {
		t.Fatal("tokens consecutivos não podem se repetir")
	}
}

// ResetAdmin sem admin_email/admin_password no ambiente deve falhar com
// mensagem clara (não cria usuário fantasma).
func TestResetAdminWithoutEnv(t *testing.T) {
	t.Setenv("admin_email", "")
	t.Setenv("admin_password", "")
	if err := ResetAdmin(nil); err == nil {
		t.Fatal("sem credenciais no ambiente deveria falhar")
	}
}
