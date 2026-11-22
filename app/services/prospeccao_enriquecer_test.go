package services_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"
)

// O job "enriquecer" roda contra Postgres de verdade e um Who Lead simulado
// (httptest): lote assíncrono com consultation_id, andamento, resultados em
// páginas, 402 e cancelamento com lote já pago.

// whoLeadFalso guarda o que foi pedido e responde como a API do integrador.
type whoLeadFalso struct {
	mu        sync.Mutex
	t         *testing.T
	lotes     map[int64][]string // consultation_id -> CNPJs
	posts     int
	andamento map[int64]int // consultas ao andamento por lote
	// prontoApos: o lote fica "processing" nas N primeiras consultas.
	prontoApos int
	saldo      bool // true: POST /enrichment responde 402
	// resultado por CNPJ (padrão: found)
	resultado map[string]string
	estornado map[string]bool
}

func (f *whoLeadFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	escrever := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	if r.URL.Path == "/oauth/token" {
		escrever(200, map[string]any{"access_token": "tok", "token_type": "Bearer", "expires_in": 3600})
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		escrever(401, map[string]any{"detail": "unauthorized"})
		return
	}
	switch {
	case r.URL.Path == "/account":
		escrever(200, map[string]any{"plan_name": "integrator_basic", "quota_total": 1000, "quota_used": 10, "quota_remaining": 990})
	case r.URL.Path == "/enrichment" && r.Method == http.MethodPost:
		if f.saldo {
			escrever(402, map[string]any{"detail": map[string]any{"code": "insufficient_wallet_balance"}})
			return
		}
		var corpo struct {
			CNPJs []string `json:"cnpjs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&corpo)
		f.posts++
		id := int64(100 + f.posts)
		if f.lotes == nil {
			f.lotes = map[int64][]string{}
		}
		f.lotes[id] = corpo.CNPJs
		// R$ 0,80 por CNPJ, em texto decimal.
		total := fmt.Sprintf("%d.%02d", 80*len(corpo.CNPJs)/100, 80*len(corpo.CNPJs)%100)
		escrever(202, map[string]any{"consultation_id": id, "status": "pending", "total_cost": total,
			"units_debited": len(corpo.CNPJs), "item_count": len(corpo.CNPJs)})
	case strings.HasSuffix(r.URL.Path, "/results"):
		var id int64
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/enrichment/"), "%d", &id)
		cnpjs := f.lotes[id]
		var offset, limite int
		fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
		fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limite)
		itens := []map[string]any{}
		for i := offset; i < len(cnpjs) && i < offset+limite; i++ {
			c := cnpjs[i]
			res := f.resultado[c]
			if res == "" {
				res = "found"
			}
			it := map[string]any{"cnpj": c, "found": res == "found", "result": res, "refunded": f.estornado[c] || res != "found",
				"cnpj_data": nil, "persons": []any{}}
			if res == "found" {
				it["cnpj_data"] = map[string]any{
					"company_name": "EMPRESA " + c,
					"mobile_phones": []any{map[string]any{"ddd": 85, "number": "999990000", "priority": 1, "whatsapp": true,
						"active_nao_me_ligue": false}},
					"emails":   []any{map[string]any{"email": "contato@empresa.com.br", "priority": 1}},
					"partners": []any{map[string]any{"name": "FULANO DE TAL", "qualification": "Sócio-Administrador", "cpf": "12345678900"}},
				}
				it["persons"] = []any{map[string]any{"cpf": "12345678900", "nome": "FULANO DE TAL", "qualificacao": "Sócio-Administrador",
					"telefones": []string{"85988887777"}}}
			}
			itens = append(itens, it)
		}
		escrever(200, map[string]any{"consultation_id": id, "ready": true, "total": len(cnpjs), "offset": offset, "limit": limite, "items": itens})
	case strings.HasPrefix(r.URL.Path, "/enrichment/"):
		var id int64
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/enrichment/"), "%d", &id)
		if _, ok := f.lotes[id]; !ok {
			escrever(404, map[string]any{"detail": "not found"})
			return
		}
		if f.andamento == nil {
			f.andamento = map[int64]int{}
		}
		f.andamento[id]++
		status := "ready"
		if f.andamento[id] <= f.prontoApos {
			status = "processing"
		}
		escrever(200, map[string]any{"id": id, "status": status, "ready": status == "ready",
			"items_total": len(f.lotes[id]), "items_processed": 0})
	default:
		escrever(404, map[string]any{"detail": "rota desconhecida"})
	}
}

// enriqPreparar zera as tabelas, configura o Who Lead (real, apontando para o
// servidor falso, ou demonstração) e cria uma lista com n itens.
func enriqPreparar(t *testing.T, f *whoLeadFalso, demo bool, lote int, n int) (*sql.DB, *models.ProspLista, []int64) {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE prospeccao_jobs, prospeccao_itens, prospeccao_listas RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM audit_log WHERE entity = 'lista_prospeccao'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM company_enrichments`); err != nil {
		t.Fatal(err)
	}
	segredo := utils.Cfg.JWTSecret
	utils.Cfg.JWTSecret = "segredo-de-teste"
	esperasJob, esperasLote := services.ProspeccaoEsperas, services.EnriquecerEsperas
	services.ProspeccaoEsperas = []time.Duration{0, 0, 0}
	services.EnriquecerEsperas = []time.Duration{0}
	utils.Cache = utils.NovoMemoria()
	t.Cleanup(func() {
		utils.Cfg.JWTSecret = segredo
		services.ProspeccaoEsperas, services.EnriquecerEsperas = esperasJob, esperasLote
		utils.Cache = utils.NovoMemoria()
		db.Exec(`DELETE FROM app_settings WHERE key = $1`, models.SettingEnrichment)
	})

	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	cfg := models.DefaultEnrichmentSettings()
	cfg.WhoLead.Habilitado, cfg.WhoLead.Demo = true, demo
	cfg.WhoLead.ClientID, cfg.WhoLead.BaseURL, cfg.WhoLead.LoteTamanho = "cid", srv.URL, lote
	cifrado, err := utils.Cifrar("segredo")
	if err != nil {
		t.Fatal(err)
	}
	cfg.WhoLead.ClientSecretCifrado = cifrado
	if err := models.SetSetting(db, models.SettingEnrichment, cfg.ParaGravar(), nil); err != nil {
		t.Fatal(err)
	}

	l := &models.ProspLista{Nome: "Padarias de Fortaleza", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: n,
		Status: models.ListaPronta}
	if err := models.CreateProspLista(db, l); err != nil {
		t.Fatal(err)
	}
	itens := make([]models.ProspItem, n)
	for i := range itens {
		c := cnpjTeste(500 + i)
		itens[i] = models.ProspItem{CNPJ: c, RazaoSocial: "EMPRESA " + c, UF: "CE", Municipio: "FORTALEZA", Porte: models.PorteME,
			Dados: &models.ProspEmpresa{CNPJ: c, RazaoSocial: "EMPRESA " + c}}
	}
	if _, err := models.InsertProspItens(db, l.ID, itens); err != nil {
		t.Fatal(err)
	}
	ids, err := models.ResolveProspSelecao(db, l.ID, models.ProspSelecao{Todos: true}, nil, 0, n+1)
	if err != nil || len(ids) != n {
		t.Fatalf("ids = %v err = %v", ids, err)
	}
	return db, l, ids
}

func enriqCriarJob(t *testing.T, db *sql.DB, l *models.ProspLista, sel models.ProspSelecao, reprocessar bool) (*services.EnriquecerEstimativa, *models.ProspJob) {
	t.Helper()
	est, err := services.EstimarEnriquecimento(context.Background(), db, l.ID, sel, reprocessar, true)
	if err != nil {
		t.Fatal(err)
	}
	job, err := services.CriarJobEnriquecer(db, l.ID, est, sel, reprocessar, nil)
	if err != nil {
		t.Fatal(err)
	}
	return est, job
}

func enriqItem(t *testing.T, db *sql.DB, id int64) *models.ProspItem {
	t.Helper()
	it, err := models.ProspItemByID(db, id)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func enriqJob(t *testing.T, db *sql.DB, id int64) *models.ProspJob {
	t.Helper()
	j, err := models.ProspJobByID(db, id)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestEnriquecerLoteComRetomadaPaginasECusto(t *testing.T) {
	f := &whoLeadFalso{t: t, prontoApos: 2}
	db, l, ids := enriqPreparar(t, f, false, 2, 3)
	f.resultado = map[string]string{cnpjTeste(501): "not_found", cnpjTeste(502): "rate_limited"}

	est, job := enriqCriarJob(t, db, l, models.ProspSelecao{Todos: true}, false)
	if est.AEnriquecer != 3 || est.CustoUnitarioCentavos != 80 || est.CustoEstimadoCentavos != 240 || est.Saldo == nil ||
		est.Saldo.FranquiaRestante == nil || *est.Saldo.FranquiaRestante != 990 || !est.Provedor.Pronto {
		t.Fatalf("estimativa = %+v", est)
	}
	// Primeira fatia: cria o lote e grava o consultation_id no estado.
	if _, err := services.ProcessarProximoJob(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	j := enriqJob(t, db, job.ID)
	var estado struct {
		Lote *struct {
			ConsultationID int64 `json:"consultation_id"`
		} `json:"lote"`
	}
	_ = json.Unmarshal(j.Estado, &estado)
	if f.posts != 1 || estado.Lote == nil || estado.Lote.ConsultationID != 101 {
		t.Fatalf("depois da 1ª fatia: posts=%d estado=%s", f.posts, j.Estado)
	}
	// "Reinício": as fatias seguintes leem o estado gravado e não pagam de novo.
	if _, err := models.RecuperarProspJobsOrfaos(db); err != nil {
		t.Fatal(err)
	}
	rodarJobs(t, db)

	j = enriqJob(t, db, job.ID)
	if j.Status != models.JobConcluido {
		t.Fatalf("job = %s %q", j.Status, j.Mensagem)
	}
	if f.posts != 2 || f.andamento[101] != 3 {
		t.Errorf("posts = %d, consultas ao andamento do 1º lote = %d", f.posts, f.andamento[101])
	}
	a, b, c := enriqItem(t, db, ids[0]), enriqItem(t, db, ids[1]), enriqItem(t, db, ids[2])
	if a.Estado != models.ItemEnriquecido || !a.TemCelular || !a.TemWhatsApp || !a.TemDecisor || a.CustoCentavos != 80 ||
		a.EnriquecimentoProvedor != models.ProvedorWhoLead || a.Nota == 0 {
		t.Errorf("item enriquecido = %+v", a)
	}
	if a.Enriquecimento == nil || len(a.Enriquecimento.Pessoas) == 0 || a.Enriquecimento.Pessoas[0].Documento != "***456789**" {
		t.Errorf("perfil = %+v", a.Enriquecimento)
	}
	if b.Estado != models.ItemSemResultado || b.CustoCentavos != 0 { // estornado
		t.Errorf("sem resultado = %+v", b)
	}
	if c.Estado != models.ItemErro || c.Erro == "" {
		t.Errorf("rate_limited = %+v", c)
	}
	if j.Feitos != 3 || j.Erros != 1 || j.CustoCentavos != 80 {
		t.Errorf("job feitos=%d erros=%d custo=%d", j.Feitos, j.Erros, j.CustoCentavos)
	}
	var res services.ResultadoEnriquecer
	_ = json.Unmarshal(j.Resultado, &res)
	if res.Enriquecidos != 1 || res.SemResultado != 1 || res.Erros != 1 || res.Lotes != 2 || res.CustoCentavos != 80 {
		t.Errorf("resultado = %+v", res)
	}
	// CPF completo não é persistido em lugar nenhum.
	var vazou bool
	if err := db.QueryRow(`SELECT bool_or(enriquecimento_bruto::text LIKE '%12345678900%' OR enriquecimento::text LIKE '%12345678900%')
		FROM prospeccao_itens`).Scan(&vazou); err != nil || vazou {
		t.Errorf("CPF completo gravado (err=%v)", err)
	}
	// Uso do mês soma o lote (uma consulta só).
	uso, err := models.EnrichmentUsage(db, time.Now())
	if err != nil || uso.PorProvedor[models.ProvedorWhoLead].CustoCentavos != 80 || uso.PorProvedor[models.ProvedorWhoLead].Consultas != 3 {
		t.Errorf("uso = %+v err = %v", uso, err)
	}
	var auditorias int
	_ = db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE entity = 'lista_prospeccao' AND summary LIKE 'lista enriquecida%'`).Scan(&auditorias)
	if auditorias != 1 {
		t.Errorf("auditorias = %d", auditorias)
	}

	// A estimativa seguinte só conta o que falta (erro e sem_resultado) e o
	// limite mensal entra na conta.
	est, err = services.EstimarEnriquecimento(context.Background(), db, l.ID, models.ProspSelecao{Todos: true}, false, false)
	if err != nil || est.AEnriquecer != 2 || est.JaEnriquecidos != 1 || est.UsoMes.CustoCentavos != 80 || est.Saldo != nil {
		t.Errorf("estimativa depois = %+v err = %v", est, err)
	}
	est.UsoMes.LimiteCentavos = 200
	if !est.EstourariaLimite() {
		t.Error("80 + 160 > 200 deveria estourar o limite")
	}
	// Reprocessar soma o novo custo ao do item.
	_, job2 := enriqCriarJob(t, db, l, models.ProspSelecao{IDs: []int64{ids[0]}}, true)
	rodarJobs(t, db)
	if j2 := enriqJob(t, db, job2.ID); j2.Status != models.JobConcluido || j2.CustoCentavos != 80 {
		t.Errorf("reprocessar: %s custo=%d", j2.Status, j2.CustoCentavos)
	}
	if a = enriqItem(t, db, ids[0]); a.CustoCentavos != 160 {
		t.Errorf("custo acumulado do item = %d", a.CustoCentavos)
	}
	// O custo sobrevive à exclusão da lista.
	if err := models.DeleteProspLista(db, l.ID); err != nil {
		t.Fatal(err)
	}
	if uso, _ := models.EnrichmentUsage(db, time.Now()); uso.PorProvedor[models.ProvedorWhoLead].CustoCentavos != 160 {
		t.Errorf("uso depois de apagar a lista = %+v", uso)
	}
}

func TestEnriquecerSaldoEsgotadoParaSemMexerNosItens(t *testing.T) {
	f := &whoLeadFalso{t: t, saldo: true}
	db, l, ids := enriqPreparar(t, f, false, 500, 2)
	_, job := enriqCriarJob(t, db, l, models.ProspSelecao{Todos: true}, false)
	rodarJobs(t, db)
	j := enriqJob(t, db, job.ID)
	if j.Status != models.JobConcluido || !strings.Contains(j.Mensagem, "saldo") || j.CustoCentavos != 0 {
		t.Fatalf("job = %s %q custo=%d", j.Status, j.Mensagem, j.CustoCentavos)
	}
	var res services.ResultadoEnriquecer
	_ = json.Unmarshal(j.Resultado, &res)
	if res.Parado != "saldo" {
		t.Errorf("resultado = %+v", res)
	}
	for _, id := range ids {
		if it := enriqItem(t, db, id); it.Estado != models.ItemNovo {
			t.Errorf("item %d = %s", id, it.Estado)
		}
	}
}

func TestEnriquecerDemonstracaoSemCusto(t *testing.T) {
	f := &whoLeadFalso{t: t}
	db, l, ids := enriqPreparar(t, f, true, 500, 3)
	est, job := enriqCriarJob(t, db, l, models.ProspSelecao{Todos: true, Exceto: []int64{ids[2]}}, false)
	if est.CustoEstimadoCentavos != 0 || !est.Provedor.Demo || est.AEnriquecer != 2 || est.Saldo != nil {
		t.Fatalf("estimativa demo = %+v", est)
	}
	rodarJobs(t, db)
	j := enriqJob(t, db, job.ID)
	if j.Status != models.JobConcluido || j.CustoCentavos != 0 || j.Feitos != 2 || f.posts != 0 {
		t.Fatalf("job = %+v posts=%d", j, f.posts)
	}
	if it := enriqItem(t, db, ids[0]); it.Estado != models.ItemEnriquecido || it.Enriquecimento == nil || !it.Enriquecimento.Demo || !it.TemCelular {
		t.Errorf("item demo = %+v", it)
	}
	if it := enriqItem(t, db, ids[2]); it.Estado != models.ItemNovo {
		t.Errorf("item exceto = %s", it.Estado)
	}
	if uso, _ := models.EnrichmentUsage(db, time.Now()); uso.Consultas != 0 {
		t.Errorf("demonstração não entra no uso: %+v", uso)
	}
}

func TestEnriquecerCanceladoAindaLeOLotePago(t *testing.T) {
	f := &whoLeadFalso{t: t, prontoApos: 1}
	db, l, ids := enriqPreparar(t, f, false, 500, 2)
	_, job := enriqCriarJob(t, db, l, models.ProspSelecao{Todos: true}, false)
	if _, err := services.ProcessarProximoJob(context.Background(), db); err != nil { // cria o lote
		t.Fatal(err)
	}
	if _, err := models.CancelarProspJob(db, job.ID); err != nil {
		t.Fatal(err)
	}
	rodarJobs(t, db)
	j := enriqJob(t, db, job.ID)
	if j.Status != models.JobCancelado || f.posts != 1 || j.CustoCentavos != 160 || !strings.Contains(j.Mensagem, "cancelado") {
		t.Fatalf("job = %s %q custo=%d posts=%d", j.Status, j.Mensagem, j.CustoCentavos, f.posts)
	}
	for _, id := range ids {
		if it := enriqItem(t, db, id); it.Estado != models.ItemEnriquecido {
			t.Errorf("item %d = %s", id, it.Estado)
		}
	}
}

// Lote maior que uma página de resultados (500): duas páginas, custo
// distribuído sem perder centavo.
func TestEnriquecerResultadosEmPaginas(t *testing.T) {
	f := &whoLeadFalso{t: t}
	db, l, _ := enriqPreparar(t, f, false, 1000, 601)
	_, job := enriqCriarJob(t, db, l, models.ProspSelecao{Todos: true}, false)
	rodarJobs(t, db)
	j := enriqJob(t, db, job.ID)
	if j.Status != models.JobConcluido || f.posts != 1 || j.Feitos != 601 || j.CustoCentavos != 601*80 {
		t.Fatalf("job = %s %q feitos=%d custo=%d posts=%d", j.Status, j.Mensagem, j.Feitos, j.CustoCentavos, f.posts)
	}
	var n, soma int
	if err := db.QueryRow(`SELECT COUNT(*) FILTER (WHERE estado = 'enriquecido'), COALESCE(SUM(custo_centavos), 0)
		FROM prospeccao_itens WHERE lista_id = $1`, l.ID).Scan(&n, &soma); err != nil || n != 601 || soma != 601*80 {
		t.Errorf("itens enriquecidos = %d, soma = %d (err %v)", n, soma, err)
	}
}
