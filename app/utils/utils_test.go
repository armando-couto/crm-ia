package utils

import (
	"testing"
	"time"
)

func TestMemoriaIncrRespeitaTTL(t *testing.T) {
	s := NovoMemoria()
	if n := s.Incr("x", 50*time.Millisecond); n != 1 {
		t.Fatalf("primeiro incr = %d", n)
	}
	if n := s.Incr("x", 50*time.Millisecond); n != 2 {
		t.Fatalf("segundo incr = %d", n)
	}
	time.Sleep(70 * time.Millisecond)
	if n := s.Incr("x", 50*time.Millisecond); n != 1 {
		t.Fatalf("depois do TTL o contador deveria recomeçar, veio %d", n)
	}
}

func TestMemoriaDelPrefixo(t *testing.T) {
	s := NovoMemoria()
	s.Set("login:a", "1", time.Minute)
	s.Set("login:b", "1", time.Minute)
	s.Set("outro", "1", time.Minute)
	s.DelPrefixo("login:")
	if _, ok := s.Get("login:a"); ok {
		t.Fatal("prefixo não foi removido")
	}
	if _, ok := s.Get("outro"); !ok {
		t.Fatal("chave fora do prefixo foi removida")
	}
}

func TestCifrarDecifrar(t *testing.T) {
	Cfg.JWTSecret = "segredo-de-teste-com-tamanho-suficiente"
	Cfg.ChaveCripto = ""
	cifrado, err := Cifrar("senha-smtp-123")
	if err != nil {
		t.Fatal(err)
	}
	if cifrado == "senha-smtp-123" || cifrado == "" {
		t.Fatal("o texto não foi cifrado")
	}
	texto, err := Decifrar(cifrado)
	if err != nil || texto != "senha-smtp-123" {
		t.Fatalf("decifrar = %q, %v", texto, err)
	}
	if v, _ := Decifrar(""); v != "" {
		t.Fatal("vazio deveria voltar vazio")
	}
}

func TestBasePathNormalizado(t *testing.T) {
	t.Setenv("BASE_PATH", "minhaempresa/")
	t.Setenv("APP_URL", "")
	LoadConfig()
	if Cfg.BasePath != "/minhaempresa" {
		t.Fatalf("BasePath = %q", Cfg.BasePath)
	}
	if Cfg.AppURL != "http://localhost:8080/minhaempresa" {
		t.Fatalf("AppURL = %q", Cfg.AppURL)
	}
}
