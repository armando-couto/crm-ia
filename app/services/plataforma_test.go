package services_test

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

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"
)

// painelSimulado sobe um painel de mentira e aponta o cliente para ele.
func painelSimulado(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Cleanup(services.UsarPlataformaDeTeste(srv.URL))
}

func responder(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

func TestPlataformaSemPainelConfigurado(t *testing.T) {
	antes := utils.Cfg
	utils.Cfg.PainelURL, utils.Cfg.TokenInterno = "", ""
	t.Cleanup(func() { utils.Cfg = antes })
	t.Cleanup(services.UsarPlataformaDeTeste(""))

	if services.PlataformaConfigurada() {
		t.Fatal("sem PAINEL_URL e TOKEN_INTERNO a plataforma não está configurada")
	}
	// O resumo não é erro: a tela mostra só o motivo.
	r, err := services.PlataformaResumo(context.Background())
	if err != nil || r.Habilitada || r.Motivo == "" {
		t.Fatalf("resumo sem painel: %+v, %v", r, err)
	}
	_, err = services.PlataformaContagem(context.Background(), models.ProspFiltros{UFs: []string{"CE"}})
	e, ok := services.ComoErroPlataforma(err)
	if !ok || e.Status != http.StatusServiceUnavailable || e.Codigo != services.CodigoPlataforma || e.Transitorio() {
		t.Fatalf("contagem sem painel: %+v", err)
	}
	if _, _, err := services.ChamarPainel(context.Background(), http.MethodGet, "/plano", nil); !errors.Is(err, services.ErrPlataformaNaoConfigurada) {
		t.Fatalf("ChamarPainel sem painel deveria dar ErrPlataformaNaoConfigurada, veio %v", err)
	}
}

func TestPlataformaChamadasTipadas(t *testing.T) {
	antes := utils.Cfg
	utils.Cfg.TokenInterno = "token-do-ambiente-de-teste"
	t.Cleanup(func() { utils.Cfg = antes })

	var visto []string
	painelSimulado(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token-do-ambiente-de-teste" {
			responder(w, http.StatusUnauthorized, map[string]string{"erro": "não autorizado"})
			return
		}
		corpo, _ := io.ReadAll(r.Body)
		visto = append(visto, r.Method+" "+r.URL.RequestURI()+" "+string(corpo))
		switch r.URL.Path {
		case "/prospeccao":
			responder(w, 200, map[string]any{"habilitada": true, "cota": map[string]any{"mes": "2026-11", "limite": 5000, "usado": 1240, "restante": 3760},
				"base": map[string]any{"pronta": true, "amostra": true, "versao": "amostra-1", "empresas": 3000, "carga": map[string]any{"estado": "ociosa"}}})
		case "/prospeccao/catalogos/cnaes":
			responder(w, 200, map[string]any{"itens": []map[string]any{{"codigo": "56", "descricao": "Alimentação", "nivel": "divisao"}}})
		case "/prospeccao/contagem":
			responder(w, 200, map[string]any{"total": 10000, "total_modo": "teto", "total_limite_superior": 48211,
				"parciais": map[string]any{"com_celular": nil, "com_email": nil, "com_divida": nil}})
		case "/prospeccao/previa":
			responder(w, 200, map[string]any{"ordem_aplicada": "base", "itens": []map[string]any{{"cnpj": "11222333000181", "razao_social": "PADARIA", "tem_celular": true}}})
		case "/prospeccao/candidatos":
			responder(w, 200, map[string]any{"cnpjs": []string{"11222333000181", "12ABC34501DE35"}, "cursor": "abc", "fim": false, "versao": "v7", "ordem_aplicada": "relevancia"})
		case "/prospeccao/extracoes":
			responder(w, 200, map[string]any{"nao_encontrados": []string{"00000000000191"}, "cota": map[string]any{"mes": "2026-11", "limite": 5000, "usado": 1241, "restante": 3759},
				"itens": []map[string]any{{"cnpj": "11222333000181", "razao_social": "PADARIA DOCE LTDA", "porte": "me", "capital": 1000.5,
					"cnae":      map[string]any{"codigo": 1091102, "descricao": "Padaria"},
					"endereco":  map[string]any{"municipio": "FORTALEZA", "uf": "CE"},
					"telefones": []map[string]any{{"numero": "85999990000", "tipo": "celular", "origem": "receita"}},
					"socios":    []map[string]any{{"nome": "MARIA", "qualificacao": "Sócio-Administrador", "tipo": "pf", "documento": "***456789**", "desde": "2019-03-01"}},
					"divida":    map[string]any{"total": 1234.56, "inscricoes": 2, "tipos": []string{"geral"}, "referencia": "2026-T3"}}}})
		case "/prospeccao/cnpj/12ABC34501DE35":
			responder(w, 200, map[string]any{"cnpj": "12ABC34501DE35", "razao_social": "ALFA LTDA", "pessoa_fisica": false})
		default:
			responder(w, 404, map[string]string{"erro": "não encontrado", "error": "não encontrado", "codigo": "nao_encontrado"})
		}
	})
	ctx := context.Background()

	resumo, err := services.PlataformaResumo(ctx)
	if err != nil || !resumo.Habilitada || resumo.Cota.Restante != 3760 || !resumo.Base.Amostra || resumo.Base.Carga.Estado != "ociosa" {
		t.Fatalf("resumo: %+v, %v", resumo, err)
	}
	cat, err := services.PlataformaCatalogo(ctx, "cnaes", url.Values{"q": {"aliment"}, "nivel": {"divisao"}})
	if err != nil || !strings.Contains(string(cat), `"nivel":"divisao"`) {
		t.Fatalf("catálogo: %s, %v", cat, err)
	}
	if _, err := services.PlataformaCatalogo(ctx, "segredos", nil); err == nil {
		t.Fatal("catálogo desconhecido deveria ser recusado sem chamar o painel")
	}
	cont, err := services.PlataformaContagem(ctx, models.ProspFiltros{UFs: []string{"CE"}, ComCelular: true})
	if err != nil || cont.Total != 10000 || cont.TotalModo != "teto" || cont.TotalLimiteSuperior == nil || *cont.TotalLimiteSuperior != 48211 || cont.Parciais.ComCelular != nil {
		t.Fatalf("contagem: %+v, %v", cont, err)
	}
	previa, err := services.PlataformaPrevia(ctx, models.ProspFiltros{UFs: []string{"SP"}}, models.OrdemCapitalDesc, 0)
	if err != nil || len(previa.Itens) != 1 || !previa.Itens[0].TemCelular || previa.OrdemAplicada != models.OrdemBase {
		t.Fatalf("prévia: %+v, %v", previa, err)
	}
	cand, err := services.PlataformaCandidatos(ctx, services.CandidatosPedido{PedidoID: "uuid-da-lista", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, Limite: 5000})
	if err != nil || len(cand.CNPJs) != 2 || cand.Cursor != "abc" || cand.Fim || cand.Versao != "v7" {
		t.Fatalf("candidatos: %+v, %v", cand, err)
	}
	ext, err := services.PlataformaExtracao(ctx, "uuid-da-lista", []string{"11222333000181", "00000000000191"}, true)
	if err != nil || len(ext.Itens) != 1 || ext.Cota.Usado != 1241 || ext.NaoEncontrados[0] != "00000000000191" {
		t.Fatalf("extração: %+v, %v", ext, err)
	}
	e := ext.Itens[0]
	if e.Capital == nil || *e.Capital != 1000.5 || e.CNAE.Codigo != 1091102 || e.Endereco.UF != "CE" || e.Telefones[0].Tipo != models.TelefoneCelular ||
		e.Socios[0].Documento != "***456789**" || e.Divida == nil || e.Divida.Inscricoes != 2 {
		t.Fatalf("ficha da empresa decodificada errado: %+v", e)
	}
	if _, err := services.PlataformaExtracao(ctx, "x", make([]string, services.MaxCNPJsExtracao+1), false); err == nil {
		t.Fatal("extração acima do teto deveria ser recusada antes de chamar o painel")
	}
	ficha, err := services.PlataformaCNPJ(ctx, "12.abc.345/01de-35")
	if err != nil || ficha.RazaoSocial != "ALFA LTDA" {
		t.Fatalf("ficha de um CNPJ alfanumérico: %+v, %v", ficha, err)
	}
	_, err = services.PlataformaCNPJ(ctx, "11.222.333/0001-81")
	if pe, ok := services.ComoErroPlataforma(err); !ok || pe.Status != 404 || pe.Codigo != services.CodigoNaoEncontrado {
		t.Fatalf("CNPJ fora da base: %+v", err)
	}
	if _, err := services.PlataformaCNPJ(ctx, "123"); err == nil {
		t.Fatal("CNPJ malformado deveria ser recusado sem chamar o painel")
	}

	quer := []string{
		`GET /prospeccao `,
		`GET /prospeccao/catalogos/cnaes?nivel=divisao&q=aliment `,
		`POST /prospeccao/contagem {"filtros":{"ufs":["CE"],"com_celular":true}}`,
		`POST /prospeccao/previa {"filtros":{"ufs":["SP"]},"ordem":"capital_desc"}`,
		`POST /prospeccao/candidatos {"pedido_id":"uuid-da-lista","filtros":{"ufs":["CE"]},"limite":5000}`,
		`POST /prospeccao/extracoes {"cnpjs":["11222333000181","00000000000191"],"incluir_pf":true,"pedido_id":"uuid-da-lista"}`,
		`GET /prospeccao/cnpj/12ABC34501DE35 `,
		`GET /prospeccao/cnpj/11222333000181 `,
	}
	if len(visto) != len(quer) {
		t.Fatalf("chamadas ao painel: %q", visto)
	}
	for i := range quer {
		if visto[i] != quer[i] {
			t.Errorf("chamada %d:\n veio %s\n quer %s", i, visto[i], quer[i])
		}
	}
}

func TestPlataformaErrosNormalizados(t *testing.T) {
	casos := []struct {
		nome        string
		status      int
		corpo       map[string]any
		retryAfter  string
		querStatus  int
		querCodigo  string
		querTrecho  string
		transitorio bool
	}{
		{"cota esgotada", 429, map[string]any{"erro": "x", "error": "x", "codigo": "cota", "cota": map[string]any{"mes": "2026-11", "limite": 1000, "usado": 1000, "restante": 0}},
			"", 429, services.CodigoCotaEsgotada, "cota mensal de prospecção esgotada", false},
		{"limite de requisições", 429, map[string]any{"error": "devagar", "codigo": "limite_requisicoes", "tentar_em_segundos": 3},
			"7", 429, services.CodigoMuitasRequisicoes, "muitas consultas em pouco tempo", true},
		{"base indisponível", 503, map[string]any{"error": "a base ainda não foi carregada", "codigo": "base_em_carga"},
			"", 503, services.CodigoBaseEmCarga, "ainda não está disponível", true},
		{"cursor de versão antiga", 409, map[string]any{"error": "a base foi atualizada; refaça a busca", "codigo": "base_atualizada"},
			"", 409, services.CodigoBaseAtualizada, "refaça a busca", false},
		{"filtro inválido", 422, map[string]any{"erro": "informe ao menos UF, município ou CNAE", "error": "informe ao menos UF, município ou CNAE", "codigo": "filtro_invalido"},
			"", 422, services.CodigoFiltroInvalido, "informe ao menos UF", false},
		{"ambiente não reconhecido", 401, map[string]any{"erro": "não autorizado"},
			"", 502, services.CodigoPlataforma, "não reconheceu este ambiente", false},
		{"erro interno do painel", 500, map[string]any{"erro": "pane"},
			"", 502, services.CodigoPlataforma, "respondeu com erro", true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			painelSimulado(t, func(w http.ResponseWriter, r *http.Request) {
				if c.retryAfter != "" {
					w.Header().Set("Retry-After", c.retryAfter)
				}
				responder(w, c.status, c.corpo)
			})
			_, err := services.PlataformaContagem(context.Background(), models.ProspFiltros{UFs: []string{"CE"}})
			e, ok := services.ComoErroPlataforma(err)
			if !ok {
				t.Fatalf("esperava *ErroPlataforma, veio %v", err)
			}
			if e.Status != c.querStatus || e.Codigo != c.querCodigo || !strings.Contains(e.Mensagem, c.querTrecho) || e.Transitorio() != c.transitorio || e.StatusPainel != c.status {
				t.Fatalf("erro normalizado: %+v (transitório=%v)", e, e.Transitorio())
			}
			corpo := e.Corpo()
			if corpo["error"] != e.Mensagem || corpo["codigo"] != c.querCodigo {
				t.Fatalf("corpo para a tela: %v", corpo)
			}
			switch c.querCodigo {
			case services.CodigoCotaEsgotada:
				if e.Cota == nil || e.Cota.Limite != 1000 || corpo["cota"] == nil {
					t.Fatalf("o 429 de cota leva a cota: %+v", e)
				}
			case services.CodigoMuitasRequisicoes:
				if e.TentarEmSegundos != 7 || corpo["tentar_em_segundos"] != 7 {
					t.Fatalf("Retry-After deveria prevalecer: %+v", e)
				}
			}
		})
	}
}

func TestPlataformaForaDoArEhTransitorio(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	endereco := srv.URL
	srv.Close() // ninguém escuta mais neste endereço
	t.Cleanup(services.UsarPlataformaDeTeste(endereco))

	_, err := services.PlataformaContagem(context.Background(), models.ProspFiltros{UFs: []string{"CE"}})
	e, ok := services.ComoErroPlataforma(err)
	if !ok || e.Status != http.StatusBadGateway || e.Codigo != services.CodigoPlataforma || !e.Transitorio() || e.StatusPainel != 0 {
		t.Fatalf("painel fora do ar: %+v", err)
	}
	if _, _, err := services.ChamarPainel(context.Background(), http.MethodGet, "/plano", nil); !errors.Is(err, services.ErrPlataformaInacessivel) {
		t.Fatalf("ChamarPainel com painel fora do ar: %v", err)
	}
}
