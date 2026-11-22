package operacoes

import (
	"errors"
	"testing"
	"time"
)

func TestCompetenciaViraNoFusoDeBrasilia(t *testing.T) {
	casos := []struct{ instante, mes, dia string }{
		// 1º de dezembro, 02:30 UTC ainda é 30 de novembro, 23:30, em Brasília.
		{"2026-12-01T02:30:00Z", "2026-11", "2026-11-30"},
		{"2026-12-01T03:00:00Z", "2026-12", "2026-12-01"},
		{"2026-11-30T23:59:59-03:00", "2026-11", "2026-11-30"},
		{"2027-01-01T00:00:00-03:00", "2027-01", "2027-01-01"},
		// Meio-dia em Tóquio do dia 1º = 00:00 de Brasília do mesmo dia.
		{"2026-03-01T12:00:00+09:00", "2026-03", "2026-03-01"},
		{"2026-03-01T11:59:00+09:00", "2026-02", "2026-02-28"},
	}
	for _, c := range casos {
		instante, err := time.Parse(time.RFC3339, c.instante)
		if err != nil {
			t.Fatal(err)
		}
		comp := Competencia(instante)
		if comp.Format("2006-01") != c.mes || comp.Day() != 1 {
			t.Errorf("Competencia(%s) = %s (esperava %s)", c.instante, comp.Format("2006-01-02"), c.mes)
		}
		if got := DiaDaCota(instante).Format("2006-01-02"); got != c.dia {
			t.Errorf("DiaDaCota(%s) = %s (esperava %s)", c.instante, got, c.dia)
		}
	}
	if fusoCota.String() != "America/Sao_Paulo" {
		t.Fatalf("fuso da cota: %s", fusoCota)
	}
}

func TestNormalizarCNPJ(t *testing.T) {
	validos := map[string]string{
		"11.222.333/0001-81": "11222333000181",
		"11222333000181":     "11222333000181",
		"12.abc.345/01de-35": "12ABC34501DE35",
		" 12ABC34501DE35 ":   "12ABC34501DE35",
	}
	for entrada, esperado := range validos {
		if got, ok := NormalizarCNPJ(entrada); !ok || got != esperado {
			t.Errorf("NormalizarCNPJ(%q) = %q, %v", entrada, got, ok)
		}
	}
	for _, entrada := range []string{"", "123", "112223330001811", "12ABC34501DEAB", "1122233300018X", "é1222333000181"} {
		if _, ok := NormalizarCNPJ(entrada); ok {
			t.Errorf("NormalizarCNPJ(%q) deveria recusar", entrada)
		}
	}
}

func TestHashDoLote(t *testing.T) {
	a, lista, err := HashDoLote([]string{"11.222.333/0001-81", "00000001000191", "12abc34501de35"})
	if err != nil || len(lista) != 3 || lista[0] != "00000001000191" || lista[2] != "12ABC34501DE35" {
		t.Fatalf("lote: %v %v", lista, err)
	}
	// sha256 de "00000001000191,11222333000181,12ABC34501DE35"
	if len(a) != 64 {
		t.Fatalf("hash: %s", a)
	}
	// Ordem, máscara, caixa e repetição não mudam o lote.
	b, lista, _ := HashDoLote([]string{"12ABC34501DE35", "00.000.001/0001-91", "11222333000181", "11222333000181"})
	if a != b || len(lista) != 3 {
		t.Fatalf("o mesmo conjunto deveria dar o mesmo lote: %s x %s (%v)", a, b, lista)
	}
	if c, _, _ := HashDoLote([]string{"11222333000181", "00000001000191"}); c == a {
		t.Fatal("conjuntos diferentes deveriam dar lotes diferentes")
	}
	if _, _, err := HashDoLote([]string{"11222333000181", "nada"}); !errors.Is(err, ErrPedidoInvalido) {
		t.Fatalf("CNPJ inválido no lote: %v", err)
	}
}
