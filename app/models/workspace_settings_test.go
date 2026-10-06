package models

import (
	"testing"
	"time"
)

func TestJanelaDeDisparo(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	s := SendingSettings{WindowStart: 8, WindowEnd: 19, WeekdaysOnly: true, Timezone: "America/Sao_Paulo"}
	segunda10h := time.Date(2026, 10, 5, 10, 0, 0, 0, loc)
	segunda22h := time.Date(2026, 10, 5, 22, 0, 0, 0, loc)
	sabado10h := time.Date(2026, 10, 10, 10, 0, 0, 0, loc)
	if !s.DentroDaJanela(segunda10h) {
		t.Fatal("segunda 10h deveria estar na janela")
	}
	if s.DentroDaJanela(segunda22h) {
		t.Fatal("22h está fora da janela 8–19")
	}
	if s.DentroDaJanela(sabado10h) {
		t.Fatal("sábado está fora com weekdays_only")
	}
	s.WeekdaysOnly = false
	if !s.DentroDaJanela(sabado10h) {
		t.Fatal("sem weekdays_only o sábado entra")
	}
	// Janela que cruza a meia-noite.
	s.WindowStart, s.WindowEnd = 20, 6
	if !s.DentroDaJanela(segunda22h) || s.DentroDaJanela(segunda10h) {
		t.Fatal("janela 20→6 mal calculada")
	}
	// Início igual ao fim = dia inteiro.
	s.WindowStart, s.WindowEnd = 0, 0
	if !s.DentroDaJanela(segunda22h) {
		t.Fatal("janela 0→0 é o dia inteiro")
	}
}

func TestValidacaoEmailSettings(t *testing.T) {
	e := EmailSettings{Provider: "maileroo_smtp", FromEmail: "Vendas@Exemplo.com.br "}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	if e.FromEmail != "vendas@exemplo.com.br" {
		t.Fatalf("remetente não normalizado: %q", e.FromEmail)
	}
	e = EmailSettings{Provider: "smtp", FromEmail: "x@y.com"}
	if err := e.Validate(); err == nil {
		t.Fatal("SMTP genérico sem servidor deveria falhar")
	}
	e = EmailSettings{Provider: "inventado"}
	if err := e.Validate(); err == nil {
		t.Fatal("provedor desconhecido deveria falhar")
	}
	e = EmailSettings{Provider: "mandrill_api", FromEmail: "a@b.com", SecretCifrado: "xyz"}
	pub := e.Publica()
	if pub.SecretCifrado != "" || !pub.HasSecret {
		t.Fatal("a cópia pública não pode levar o segredo")
	}
}

func TestValidacaoWorkspace(t *testing.T) {
	w := WorkspaceSettings{Name: " Minha Empresa ", Color: "#ABCDEF"}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	if w.Color != "#abcdef" || w.Name != "Minha Empresa" {
		t.Fatalf("normalização: %+v", w)
	}
	w = WorkspaceSettings{Name: "X", Color: "azul"}
	if err := w.Validate(); err == nil {
		t.Fatal("cor inválida deveria falhar")
	}
}
