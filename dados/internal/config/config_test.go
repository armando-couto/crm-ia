package config

import (
	"reflect"
	"testing"
)

func preparar(t *testing.T) {
	t.Helper()
	for _, n := range []string{"APP_ENV", "PORT", "DADOS_CNPJ_URL", "DADOS_PGFN_URL", "DADOS_DIR_TRABALHO", "DADOS_CARGA_AUTOMATICA", "DADOS_AMOSTRA", "DADOS_UFS", "DADOS_CONEXOES", "DADOS_DISCO_MINIMO_GB"} {
		t.Setenv(n, "")
	}
	t.Setenv("DATABASE_URL", "postgres://u:s@localhost/b")
	t.Setenv("DADOS_TOKEN", "0123456789abcdef0123456789abcdef")
}

func TestPadroes(t *testing.T) {
	preparar(t)
	c, err := Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if c.Porta != "7896" || c.DirTrabalho != "/trabalho" || !c.CargaAutomatica || c.Amostra || c.Conexoes != 2 || c.DiscoBancoGB != 70 {
		t.Fatalf("padrões inesperados: %+v", c)
	}
	if c.CNPJURL != CNPJURLPadrao || c.PGFNURL != PGFNURLPadrao || len(c.UFs) != 0 {
		t.Fatalf("fontes inesperadas: %+v", c)
	}
}

func TestTokenObrigatorioEComTamanhoMinimo(t *testing.T) {
	preparar(t)
	t.Setenv("DADOS_TOKEN", "")
	if _, err := Carregar(); err == nil {
		t.Fatal("token vazio deveria falhar")
	}
	t.Setenv("DADOS_TOKEN", "curto")
	if _, err := Carregar(); err == nil {
		t.Fatal("token curto deveria falhar")
	}
}

func TestRecorteDeUFs(t *testing.T) {
	preparar(t)
	t.Setenv("DADOS_UFS", "sp, ce;SP")
	t.Setenv("DADOS_CNPJ_URL", "http://exemplo.test/dav")
	c, err := Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.UFs, []string{"CE", "SP"}) {
		t.Fatalf("UFs = %v", c.UFs)
	}
	if c.CNPJURL != "http://exemplo.test/dav/" {
		t.Fatalf("URL sem barra final: %s", c.CNPJURL)
	}
	t.Setenv("DADOS_UFS", "Ceará")
	if _, err := Carregar(); err == nil {
		t.Fatal("UF inválida deveria falhar")
	}
}
