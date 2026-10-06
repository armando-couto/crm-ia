package services

import (
	"strings"
	"testing"
)

func TestWelcomeEmail(t *testing.T) {
	subject, html := WelcomeEmail("João", "abc123def456", "https://crm.fixpay.com.br")
	if subject == "" {
		t.Fatal("assunto vazio")
	}
	for _, want := range []string{"João", "abc123def456", "https://crm.fixpay.com.br", "#9B52DF"} {
		if !strings.Contains(html, want) {
			t.Fatalf("e-mail de boas-vindas deveria conter %q", want)
		}
	}
}

func TestResetPasswordEmail(t *testing.T) {
	subject, html := ResetPasswordEmail("Maria", "token-xyz", "https://crm.fixpay.com.br/")
	if subject == "" {
		t.Fatal("assunto vazio")
	}
	if !strings.Contains(html, "https://crm.fixpay.com.br/redefinir-senha?token=token-xyz") {
		t.Fatal("link de redefinição incorreto (barra final da URL deve ser tratada)")
	}
	if !strings.Contains(html, "Maria") {
		t.Fatal("e-mail deveria conter o nome do usuário")
	}
}
