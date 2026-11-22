package controllers_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	stdhttptest "net/http/httptest"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"
)

// plataformaDeTeste responde as rotas de prospecção do painel com o que o
// teste mandar e guarda o último corpo recebido por caminho.
type plataformaDeTeste struct {
	mu        sync.Mutex
	respostas map[string]func(w http.ResponseWriter, r *http.Request)
	chamadas  map[string]int
	corpos    map[string]map[string]any
	consultas map[string]string
}

func novaPlataformaDeTeste(t *testing.T) *plataformaDeTeste {
	t.Helper()
	p := &plataformaDeTeste{respostas: map[string]func(http.ResponseWriter, *http.Request){},
		chamadas: map[string]int{}, corpos: map[string]map[string]any{}, consultas: map[string]string{}}
	srv := stdhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.chamadas[r.URL.Path]++
		p.consultas[r.URL.Path] = r.URL.RawQuery
		var corpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&corpo)
		p.corpos[r.URL.Path] = corpo
		h := p.respostas[r.URL.Path]
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"erro":"não encontrado"}`))
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(services.UsarPlataformaDeTeste(srv.URL))
	services.LimparCachesProspeccao()
	t.Cleanup(services.LimparCachesProspeccao)
	return p
}

func (p *plataformaDeTeste) responder(caminho string, status int, corpo string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.respostas[caminho] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}
}

const resumoHabilitado = `{"habilitada":true,"motivo":"","cota":{"mes":"2026-11","limite":1000,"usado":10,"restante":990},
	"base":{"pronta":true,"amostra":true,"referencia_cnpj":"2026-11","referencia_pgfn":"2026-T3","versao":"amostra-1","empresas":5000,
	"carga":{"estado":"ociosa","percentual":0,"etapa":"","erro":""}}}`

func TestProspeccaoResumo(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)
	antes := utils.Cfg
	t.Cleanup(func() { utils.Cfg = antes })

	// Sem painel: 200 com habilitada = false e o motivo.
	utils.Cfg.PainelURL, utils.Cfg.TokenInterno = "", ""
	t.Cleanup(services.UsarPlataformaDeTeste(""))
	obj := e.GET("/api/v1/prospeccao").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusOK).JSON().Object()
	obj.Value("habilitada").IsEqual(false)
	obj.Value("motivo").String().NotEmpty()

	p := novaPlataformaDeTeste(t)
	p.responder("/prospeccao", http.StatusOK, resumoHabilitado)
	obj = e.GET("/api/v1/prospeccao").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusOK).JSON().Object()
	obj.Value("habilitada").IsEqual(true)
	obj.Value("cota").Object().Value("restante").IsEqual(990)
	obj.Value("base").Object().Value("amostra").IsEqual(true)

	// Plataforma fora do ar: continua 200, desabilitada, com o motivo.
	p.responder("/prospeccao", http.StatusBadGateway, `{"erro":"x"}`)
	obj = e.GET("/api/v1/prospeccao").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusOK).JSON().Object()
	obj.Value("habilitada").IsEqual(false)
	obj.Value("motivo").String().Contains("plataforma")
}

func TestProspeccaoContagem(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	p := novaPlataformaDeTeste(t)
	p.responder("/prospeccao/contagem", http.StatusOK,
		`{"total":1234,"total_modo":"exato","parciais":{"com_celular":600,"com_email":null,"com_divida":12}}`)

	// Sem filtro obrigatório: 422 sem chamar a plataforma.
	e.POST("/api/v1/prospeccao/contagem").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"portes": []string{"me"}}}).
		Expect().Status(http.StatusUnprocessableEntity).JSON().Object().Value("codigo").IsEqual("filtro_invalido")
	if p.chamadas["/prospeccao/contagem"] != 0 {
		t.Fatal("filtro inválido não vai à plataforma")
	}

	// Com segmento: a contagem fica gravada nele.
	mock.ExpectExec("UPDATE prospeccao_segmentos SET ultima_contagem").
		WithArgs(int64(7), 1234).WillReturnResult(sqlmock.NewResult(0, 1))
	obj := e.POST("/api/v1/prospeccao/contagem").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"ce"}}, "segmento_id": 7}).
		Expect().Status(http.StatusOK).JSON().Object()
	obj.Value("total").IsEqual(1234)
	obj.Value("parciais").Object().Value("com_email").IsNull()
	if ufs := p.corpos["/prospeccao/contagem"]["filtros"].(map[string]any)["ufs"].([]any); ufs[0] != "CE" {
		t.Fatalf("UF normalizada antes de ir à plataforma: %v", ufs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// A mesma contagem de novo sai do cache.
	e.POST("/api/v1/prospeccao/contagem").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}}).
		Expect().Status(http.StatusOK)
	if p.chamadas["/prospeccao/contagem"] != 1 {
		t.Fatalf("contagem repetida deveria vir do cache: %d chamadas", p.chamadas["/prospeccao/contagem"])
	}

	// Limite de requisições do painel: 429 muitas_requisicoes com Retry-After (não é "cota esgotada").
	p.responder("/prospeccao/contagem", http.StatusTooManyRequests, `{"erro":"limite","codigo":"limite_requisicoes","tentar_em_segundos":7}`)
	resp := e.POST("/api/v1/prospeccao/contagem").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"SP"}}}).
		Expect().Status(http.StatusTooManyRequests)
	resp.Header("Retry-After").IsEqual("7")
	resp.JSON().Object().Value("codigo").IsEqual("muitas_requisicoes")

	// Base ainda não carregada: 503 base_em_carga.
	p.responder("/prospeccao/contagem", http.StatusServiceUnavailable, `{"erro":"a base ainda não foi carregada"}`)
	e.POST("/api/v1/prospeccao/contagem").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"RJ"}}}).
		Expect().Status(http.StatusServiceUnavailable).JSON().Object().Value("codigo").IsEqual("base_em_carga")
}

func TestProspeccaoPreviaECatalogo(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)
	p := novaPlataformaDeTeste(t)
	p.responder("/prospeccao/previa", http.StatusOK,
		`{"itens":[{"cnpj":"12345678000195","razao_social":"A","nome_fantasia":"","cnae":{"codigo":5611201,"descricao":"Restaurantes"},"porte":"me","abertura":null,"capital":null,"municipio":"FORTALEZA","uf":"CE","pessoa_fisica":false,"mei":false,"tem_celular":true,"tem_telefone":true,"tem_email":false,"tem_divida":false,"socios_qtd":1,"qualidade":60}],"ordem_aplicada":"base","versao":"v1"}`)
	p.responder("/prospeccao/catalogos/cnaes", http.StatusOK, `{"itens":[{"codigo":"56","descricao":"Alimentação","nivel":"divisao"}]}`)

	e.POST("/api/v1/prospeccao/previa").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}, "ordem": "qualquer"}).
		Expect().Status(http.StatusUnprocessableEntity)
	obj := e.POST("/api/v1/prospeccao/previa").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}, "ordem": "recentes"}).
		Expect().Status(http.StatusOK).JSON().Object()
	obj.Value("ordem_aplicada").IsEqual("base")
	obj.Value("itens").Array().Length().IsEqual(1)
	if p.corpos["/prospeccao/previa"]["ordem"] != "recentes" {
		t.Fatalf("ordem repassada: %v", p.corpos["/prospeccao/previa"])
	}

	e.GET("/api/v1/prospeccao/catalogos/bancos").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusNotFound)
	for range 2 {
		e.GET("/api/v1/prospeccao/catalogos/cnaes").WithQuery("q", "aliment").WithQuery("nivel", "divisao").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(http.StatusOK).JSON().Object().Value("itens").Array().Value(0).Object().Value("nivel").IsEqual("divisao")
	}
	if p.chamadas["/prospeccao/catalogos/cnaes"] != 1 || p.consultas["/prospeccao/catalogos/cnaes"] != "nivel=divisao&q=aliment" {
		t.Fatalf("catálogo: %d chamadas, consulta %q", p.chamadas["/prospeccao/catalogos/cnaes"], p.consultas["/prospeccao/catalogos/cnaes"])
	}
}

func TestCriarListaValidaEConfereCota(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)
	p := novaPlataformaDeTeste(t)
	p.responder("/prospeccao", http.StatusOK, `{"habilitada":true,"cota":{"mes":"2026-11","limite":300,"usado":300,"restante":0},"base":{"pronta":true,"versao":"v1"}}`)

	e.POST("/api/v1/prospeccao/listas").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}, "quantidade": 10}).
		Expect().Status(http.StatusUnprocessableEntity).JSON().Object().Value("error").String().Contains("nome")
	e.POST("/api/v1/prospeccao/listas").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"nome": "X", "filtros": map[string]any{"ufs": []string{"CE"}}, "quantidade": 20001}).
		Expect().Status(http.StatusUnprocessableEntity)
	e.POST("/api/v1/prospeccao/listas").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"nome": "X", "origem": "cnpjs", "filtros": map[string]any{"cnpjs": []string{"11111111111111"}}}).
		Expect().Status(http.StatusUnprocessableEntity).JSON().Object().Value("error").String().Contains("CNPJ")

	obj := e.POST("/api/v1/prospeccao/listas").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"nome": "Restaurantes", "filtros": map[string]any{"ufs": []string{"CE"}}, "quantidade": 10}).
		Expect().Status(http.StatusTooManyRequests).JSON().Object()
	obj.Value("codigo").IsEqual("cota_esgotada")
	obj.Value("cota").Object().Value("limite").IsEqual(300)
}

func TestJobInexistente(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	mock.ExpectQuery("FROM prospeccao_jobs").WillReturnError(sql.ErrNoRows)
	e.GET("/api/v1/prospeccao/jobs/99").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusNotFound)
	mock.ExpectQuery("FROM prospeccao_jobs").WillReturnError(sql.ErrNoRows)
	e.POST("/api/v1/prospeccao/jobs/99/cancelar").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusNotFound)
	// Seleção inválida: 422 antes de tocar no banco.
	e.POST("/api/v1/prospeccao/listas/1/itens/descartar").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{}}).
		Expect().Status(http.StatusUnprocessableEntity)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
