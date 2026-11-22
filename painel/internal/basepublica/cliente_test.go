package basepublica

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClienteNaoConfigurado(t *testing.T) {
	var nulo *Cliente
	if nulo.Configurado() {
		t.Fatal("cliente nulo não está configurado")
	}
	for _, c := range []*Cliente{Novo("", ""), Novo("http://dados:7896", ""), Novo("", "token"), Novo("  ", "  ")} {
		if c.Configurado() {
			t.Fatalf("sem endereço ou sem token não está configurado: %+v", c)
		}
		if _, err := c.Situacao(context.Background()); !errors.Is(err, ErrNaoConfigurado) {
			t.Fatalf("Situacao sem configuração: %v", err)
		}
		if _, _, err := c.Repassar(context.Background(), http.MethodGet, "/v1/situacao", nil, nil); !errors.Is(err, ErrNaoConfigurado) {
			t.Fatalf("Repassar sem configuração: %v", err)
		}
		if err := c.IniciarCarga(context.Background(), "tudo"); !errors.Is(err, ErrNaoConfigurado) {
			t.Fatalf("IniciarCarga sem configuração: %v", err)
		}
	}
}

func TestClienteFalaComOServico(t *testing.T) {
	var pedidos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		pedidos = append(pedidos, r.Method+" "+r.URL.RequestURI()+" "+string(corpo))
		if r.Header.Get("Authorization") != "Bearer segredo-do-servico-de-dados" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"erro":"não autorizado"}`))
			return
		}
		switch r.URL.Path {
		case "/v1/situacao":
			_, _ = w.Write([]byte(`{"pronta":true,"amostra":false,"referencia_cnpj":"2026-11","referencia_pgfn":"2026-T3","versao":"v3","empresas":28700000,"socios":1,"devedores":2,
				"atualizada_em":"2026-11-20T03:00:00-03:00","espaco_bytes":123,"ufs":["CE"],
				"carga":{"estado":"carregando","fonte":"cnpj","referencia":"2026-11","etapa":"Socios4","percentual":62.5,"iniciada_em":null,"erro":"",
				"arquivos":[{"nome":"Socios4.zip","estado":"baixado","bytes":10,"total_bytes":40,"linhas":0}]}}`))
		case "/v1/empresas/detalhes":
			_, _ = w.Write([]byte(`{"itens":[{"cnpj":"00000001000191","razao_social":"A"}],"nao_encontrados":["00000002000172"]}`))
		case "/v1/cargas":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"erro":"já há uma carga em curso"}`))
		case "/v1/catalogos/ufs":
			_, _ = w.Write([]byte(`{"itens":[]}`))
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"erro":"filtro inválido"}`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	c := Novo(srv.URL+"/", " segredo-do-servico-de-dados ")
	if !c.Configurado() {
		t.Fatal("deveria estar configurado")
	}

	sit, err := c.Situacao(ctx)
	if err != nil || !sit.Pronta || sit.Empresas != 28700000 || sit.ReferenciaPGFN != "2026-T3" || sit.AtualizadaEm == nil || sit.Carga.IniciadaEm != nil {
		t.Fatalf("situação: %+v %v", sit, err)
	}
	if !sit.Carga.EmCurso() || sit.Carga.PercentualInteiro() != 62 || len(sit.Carga.Arquivos) != 1 || sit.Carga.Arquivos[0].Percentual() != 25 {
		t.Fatalf("carga: %+v", sit.Carga)
	}

	det, err := c.Detalhes(ctx, []string{"00000001000191", "00000002000172"}, true)
	if err != nil || len(det.Itens) != 1 || len(det.NaoEncontrados) != 1 {
		t.Fatalf("detalhes: %+v %v", det, err)
	}
	var enviado struct {
		CNPJs     []string `json:"cnpjs"`
		IncluirPF bool     `json:"incluir_pf"`
	}
	ultimo := pedidos[len(pedidos)-1]
	if err := json.Unmarshal([]byte(ultimo[strings.Index(ultimo, "{"):]), &enviado); err != nil || len(enviado.CNPJs) != 2 || !enviado.IncluirPF {
		t.Fatalf("corpo dos detalhes: %s", ultimo)
	}

	// Status de erro vira *Erro com a mensagem do serviço.
	var e *Erro
	if err := c.IniciarCarga(ctx, "tudo"); !errors.As(err, &e) || e.Status != http.StatusConflict || e.Mensagem != "já há uma carga em curso" {
		t.Fatalf("carga em conflito: %v", err)
	}

	// Repassar devolve status e corpo como vieram, com a consulta codificada.
	status, corpo, err := c.Repassar(ctx, http.MethodGet, "/v1/catalogos/ufs", url.Values{"q": {"são paulo"}}, nil)
	if err != nil || status != http.StatusOK || string(corpo) != `{"itens":[]}` || !strings.Contains(pedidos[len(pedidos)-1], "q=s%C3%A3o+paulo") {
		t.Fatalf("repasse: %d %s %v (%s)", status, corpo, err, pedidos[len(pedidos)-1])
	}
	status, corpo, err = c.Repassar(ctx, http.MethodPost, "/v1/empresas/contagem", nil, []byte(`{"filtros":{}}`))
	if err != nil || status != http.StatusUnprocessableEntity || MensagemDeErro(status, corpo) != "filtro inválido" {
		t.Fatalf("repasse de erro: %d %s %v", status, corpo, err)
	}

	// Token errado: 401 do serviço, como *Erro.
	if _, err := Novo(srv.URL, "outro").Situacao(ctx); !errors.As(err, &e) || e.Status != http.StatusUnauthorized {
		t.Fatalf("token errado: %v", err)
	}
	// Serviço fora do ar.
	srv.Close()
	if _, err := c.Situacao(ctx); !errors.Is(err, ErrIndisponivel) {
		t.Fatalf("serviço fora do ar: %v", err)
	}
	if det, err := c.Detalhes(ctx, []string{"00000001000191"}, false); !errors.Is(err, ErrIndisponivel) || det.Itens == nil || det.NaoEncontrados == nil {
		t.Fatalf("detalhes com o serviço fora do ar: %+v %v", det, err)
	}
}

func TestMensagemDeErroEAuxiliares(t *testing.T) {
	if MensagemDeErro(503, []byte(`<html>fora do ar</html>`)) != "Service Unavailable" {
		t.Fatal("corpo que não é JSON cai no texto do status")
	}
	if MensagemDeErro(422, []byte(`{"error":"em inglês"}`)) != "em inglês" {
		t.Fatal("aceita também a chave error")
	}
	if (Arquivo{Estado: "carregado"}).Percentual() != 100 || (Arquivo{Estado: "pendente"}).Percentual() != 0 || (Arquivo{Bytes: 50, TotalBytes: 40}).Percentual() != 100 {
		t.Fatal("percentual do arquivo")
	}
	if (Carga{Percentual: -3}).PercentualInteiro() != 0 || (Carga{Percentual: 250}).PercentualInteiro() != 100 || (Carga{Estado: "ociosa"}).EmCurso() || (Carga{Estado: "erro"}).EmCurso() {
		t.Fatal("percentual e estado da carga")
	}
}
