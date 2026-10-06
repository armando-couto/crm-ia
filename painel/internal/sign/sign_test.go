package sign

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimuladoCriaPagadorEAssinaturaSemRede(t *testing.T) {
	c := Novo(Config{Simulado: true})
	ctx := context.Background()
	id, err := c.GarantirPagador(ctx, Pagador{Nome: "ACME", Documento: "11222333000181", CEP: "60000000", Logradouro: "Rua X", Bairro: "Centro", Municipio: "Fortaleza"})
	if err != nil || !strings.HasPrefix(id, "sim-pag-") {
		t.Fatalf("pagador simulado: %q %v", id, err)
	}
	if _, err := c.GarantirPagador(ctx, Pagador{Nome: "Sem endereço", Documento: "1"}); err == nil {
		t.Fatal("endereço incompleto deveria ser recusado")
	}
	ass, err := c.CriarAssinatura(ctx, NovaAssinatura{Documento: "11222333000181", ValorCentavos: 24900, Produto: "CRM IA · Profissional"})
	if err != nil || ass.Token == "" || !strings.HasPrefix(ass.ID, "sim-ass-") {
		t.Fatalf("assinatura simulada: %+v %v", ass, err)
	}
	if _, err := c.CriarAssinatura(ctx, NovaAssinatura{Documento: "1", ValorCentavos: 100}); err == nil {
		t.Fatal("valor abaixo do mínimo deveria falhar")
	}
}

func TestCartaoSimuladoAprovaERecusa(t *testing.T) {
	c := Novo(Config{Simulado: true})
	ctx := context.Background()
	base := Cartao{Titular: "Ana Silva", Documento: "06300000000", Validade: "12/35", CVV: "123"}
	ok := base
	ok.Numero = "4111 1111 1111 1111"
	r, err := c.CadastrarCartao(ctx, "tok", ok)
	if err != nil || !r.Aprovado || r.Final != "1111" || r.Bandeira != "VISA" {
		t.Fatalf("aprovação simulada: %+v %v", r, err)
	}
	recusa := base
	recusa.Numero = "4000000000000002"
	r, err = c.CadastrarCartao(ctx, "tok", recusa)
	if err != nil || r.Aprovado || r.Mensagem == "" {
		t.Fatalf("recusa simulada: %+v %v", r, err)
	}
	invalido := base
	invalido.Numero = "1234"
	if _, err := c.CadastrarCartao(ctx, "tok", invalido); err == nil {
		t.Fatal("número inválido deveria falhar")
	}
	vencido := ok
	vencido.Validade = "01/20"
	if _, err := c.CadastrarCartao(ctx, "tok", vencido); err == nil {
		t.Fatal("cartão vencido deveria falhar")
	}
}

func TestBandeiras(t *testing.T) {
	casos := map[string]string{"4111111111111111": "VISA", "5555555555554444": "MASTERCARD", "378282246310005": "AMEX", "6062825624254001": "HIPERCARD", "6363680000457013": "ELO", "9999": ""}
	for n, esperado := range casos {
		if got := Bandeira(n); got != esperado {
			t.Errorf("%s = %q, esperava %q", n, got, esperado)
		}
	}
}

func TestChamadaRealEnviaTokenETrataRecusa(t *testing.T) {
	var recebido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-crmia" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v2/signature/withtokenapi":
			_ = json.NewDecoder(r.Body).Decode(&recebido)
			_, _ = w.Write([]byte(`{"data": 123, "link": "https://x/signature/9/abc", "token": "abc"}`))
		case "/signature/integration/9/abc":
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"Transacao":"Cartão recusado","Token":"abc"}}`))
		case "/v2/signature/withtokenapi/billingreportsignatures":
			_, _ = w.Write([]byte(`[{"id":1,"tid":"T1","status":"PAGO","valor":249.00,"data_pagamento":"2026-10-05T10:00:00Z"},{"id":2,"status":"EM ABERTO","valor":249.00}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := Novo(Config{TokenAPI: "tok-crmia", ApisURL: srv.URL, Gateway: srv.URL, GrupoID: 9})
	ctx := context.Background()

	ass, err := c.CriarAssinatura(ctx, NovaAssinatura{Documento: "11222333000181", ValorCentavos: 24900, Produto: "CRM IA"})
	if err != nil || ass.ID != "123" || ass.Token != "abc" {
		t.Fatalf("assinatura: %+v %v", ass, err)
	}
	if recebido["tipo_recorrencia"] != "MENSAL" || recebido["valor"] != 249.0 || recebido["forma_pagamento"] != "cartao" {
		t.Fatalf("corpo inesperado: %v", recebido)
	}
	r, err := c.CadastrarCartao(ctx, "abc", Cartao{Titular: "Ana", Documento: "1", Numero: "4111111111111111", Validade: "12/35", CVV: "123"})
	if err != nil || r.Aprovado || !strings.Contains(r.Mensagem, "recusado") {
		t.Fatalf("recusa do emissor deveria virar resultado, não erro: %+v %v", r, err)
	}
	cobs, err := c.Cobrancas(ctx, "123")
	if err != nil || len(cobs) != 2 || !cobs[0].Pago || cobs[0].ValorCentavos != 24900 || cobs[1].Pago {
		t.Fatalf("cobranças: %+v %v", cobs, err)
	}
	c2 := Novo(Config{TokenAPI: "errado", ApisURL: srv.URL, Gateway: srv.URL, GrupoID: 9})
	if _, err := c2.CriarAssinatura(ctx, NovaAssinatura{Documento: "1", ValorCentavos: 24900}); err == nil || !strings.Contains(err.Error(), "recusado") {
		t.Fatalf("token recusado deveria ser explícito: %v", err)
	}
}
