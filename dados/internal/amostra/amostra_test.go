package amostra

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

func TestGerarEDeterministicoEFicticio(t *testing.T) {
	a, b := Gerar(), Gerar()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("a amostra tem de ser a mesma a cada geração")
	}
	if len(a.Estabelecimentos) < 2000 || len(a.Estabelecimentos) > 6000 {
		t.Fatalf("esperados alguns milhares de estabelecimentos, há %d", len(a.Estabelecimentos))
	}
	if len(a.Empresas) != Raizes || len(a.Socios) == 0 || len(a.Dividas) == 0 {
		t.Fatalf("empresas=%d sócios=%d dívidas=%d", len(a.Empresas), len(a.Socios), len(a.Dividas))
	}
	celular := regexp.MustCompile(`^[1-9][0-9]900000[0-9]{3}$`)
	fixo := regexp.MustCompile(`^[1-9][0-9]30000[0-9]{3}$`)
	ufs, cnaes, vistos := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, e := range a.Estabelecimentos {
		cnpj := e[0].(string)
		// raiz na faixa 00.000.0NN e dígitos verificadores válidos
		if !strings.HasPrefix(cnpj, "000000") || !texto.CNPJValido(cnpj) || vistos[cnpj] {
			t.Fatalf("CNPJ fora da regra da amostra: %s", cnpj)
		}
		vistos[cnpj] = true
		ufs[e[16].(string)] = true
		cnaes[e[7].(string)] = true
		for _, i := range []int{22, 24} {
			if f, ok := e[i].(string); ok {
				if tipo := e[i+1].(string); (tipo == texto.Celular && !celular.MatchString(f)) || (tipo == texto.Fixo && !fixo.MatchString(f)) {
					t.Fatalf("telefone fora da faixa fictícia: %s (%s)", f, tipo)
				}
				if !texto.DDDValido(f[:2]) {
					t.Fatalf("DDD inexistente: %s", f)
				}
			}
		}
		if email, ok := e[26].(string); ok && !strings.HasSuffix(email, "@exemplo.invalid") {
			t.Fatalf("e-mail fora de @exemplo.invalid: %s", email)
		}
	}
	if len(ufs) < 10 || len(cnaes) < 10 {
		t.Fatalf("pouca variedade: %d UFs, %d CNAEs", len(ufs), len(cnaes))
	}
	for _, s := range a.Socios {
		if !strings.HasPrefix(s[2].(string), "Sócio Exemplo ") || !strings.HasPrefix(s[3].(string), "***") {
			t.Fatalf("sócio fora da regra: %v", s)
		}
	}
}
