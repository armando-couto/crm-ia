package config

import (
	"strings"
	"testing"
)

func TestTokenObrigatorio(t *testing.T) {
	t.Setenv("PROVISIONER_TOKEN", "")
	if _, err := Carregar(); err == nil {
		t.Fatal("sem token deveria falhar")
	}
	t.Setenv("PROVISIONER_TOKEN", "curto")
	if _, err := Carregar(); err == nil || !strings.Contains(err.Error(), "curto") {
		t.Fatalf("token curto deveria ser recusado: %v", err)
	}
}

func TestPadroes(t *testing.T) {
	t.Setenv("PROVISIONER_TOKEN", strings.Repeat("a", 32))
	c, err := Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if c.ImagemApp != "crmia/app" || c.ImagemRedis != "redis:7-alpine" || c.RedePublica != "crmia_public" {
		t.Fatalf("padrões inesperados: %+v", c)
	}
	if c.EsperaHealthcheck.Seconds() != 2 {
		t.Fatalf("espera padrão = %v", c.EsperaHealthcheck)
	}
}
