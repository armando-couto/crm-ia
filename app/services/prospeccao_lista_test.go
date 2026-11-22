package services_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

// O job gerar_lista é testado contra Postgres de verdade e uma plataforma
// simulada (httptest) que guarda o que foi pedido e desconta a cota como o
// painel: idempotente por (pedido_id, lote).

// cnpjTeste monta um CNPJ válido a partir de um número.
func cnpjTeste(n int) string {
	base := fmt.Sprintf("%08d0001", 10000+n)
	return base + models.CNPJDigitosVerificadores(base)
}

type plataformaFalsa struct {
	mu          sync.Mutex
	candidatos  []string
	limite      int // cota do mês
	usado       int
	restanteIni *int // restante informado no resumo (nil = limite - usado)
	versao      string
	pf          map[string]bool
	semTelefone map[string]bool
	pagos       map[string]int // pedido|hash -> quantidade debitada
	lotes       [][]string     // lotes recebidos, em ordem
	pedidos     map[string]bool
	paginas     int
	// falhas
	falharExtracaoApos int  // > 0: a extração de número N debita e devolve 500
	extracoes          int  // quantas extrações chegaram
	trocarVersaoNa     int  // > 0: a página N de candidatos responde 409 uma vez
	trocou             bool // já respondeu o 409
}

func hashLote(cnpjs []string) string {
	c := append([]string(nil), cnpjs...)
	sort.Strings(c)
	s := sha256.Sum256([]byte(strings.Join(c, ",")))
	return hex.EncodeToString(s[:])
}

func (p *plataformaFalsa) cota() models.ProspCota {
	return models.ProspCota{Mes: "2026-11", Limite: p.limite, Usado: p.usado, Restante: max(0, p.limite-p.usado)}
}

func (p *plataformaFalsa) empresa(cnpj string) models.ProspEmpresa {
	ab := "2015-01-10"
	e := models.ProspEmpresa{
		CNPJ: cnpj, Matriz: true, RazaoSocial: "EMPRESA " + cnpj[:8] + " LTDA", NomeFantasia: "LOJA " + cnpj[:8],
		Porte: models.PorteEPP, Abertura: &ab, Situacao: "ATIVA",
		CNAE:     models.ProspCodigoDescricao{Codigo: 5611201, Descricao: "Restaurantes e similares"},
		Endereco: models.ProspEndereco{Bairro: "CENTRO", CEP: "60000000", Municipio: "FORTALEZA", MunicipioCodigo: 1389, UF: "CE"},
		Email:    "contato@empresa" + cnpj[:8] + ".com.br", EmailTipo: "corporativo",
		Telefones: []models.ProspTelefone{{Numero: "8599" + cnpj[1:8], Tipo: models.TelefoneCelular, Origem: "receita"}},
		Socios:    []models.ProspSocio{{Nome: "FULANO DE TAL", Qualificacao: "Sócio-Administrador", Tipo: "pf", Documento: "***123456**"}},
		Qualidade: 80, Referencia: "2026-11",
	}
	if p.semTelefone[cnpj] {
		e.Telefones = nil
	}
	if p.pf[cnpj] {
		e.PessoaFisica, e.MEI = true, true
		e.Telefones, e.Email, e.Socios = nil, "", nil // pessoa física sem incluir_pf vem sem contatos
	}
	return e
}

func (p *plataformaFalsa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	escrever := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch r.URL.Path {
	case "/prospeccao":
		c := p.cota()
		if p.restanteIni != nil {
			c.Restante = *p.restanteIni
		}
		escrever(200, models.ProspResumoPlataforma{Habilitada: true, Cota: c,
			Base: models.ProspBaseResumo{Pronta: true, Versao: p.versao}})
	case "/prospeccao/candidatos":
		var req services.CandidatosPedido
		_ = json.NewDecoder(r.Body).Decode(&req)
		p.paginas++
		if p.pedidos == nil {
			p.pedidos = map[string]bool{}
		}
		p.pedidos[req.PedidoID] = true
		if p.trocarVersaoNa > 0 && p.paginas == p.trocarVersaoNa && !p.trocou {
			p.trocou = true
			p.versao = "v2"
			escrever(409, map[string]string{"erro": "a base foi atualizada; refaça a busca", "error": "a base foi atualizada; refaça a busca"})
			return
		}
		ini := 0
		if req.Cursor != "" {
			ini, _ = strconv.Atoi(strings.TrimPrefix(req.Cursor, p.versao+":"))
		}
		limite := req.Limite
		if limite <= 0 {
			limite = 5000
		}
		fim := min(len(p.candidatos), ini+limite)
		escrever(200, services.CandidatosPlataforma{CNPJs: p.candidatos[ini:fim], Cursor: p.versao + ":" + strconv.Itoa(fim),
			Fim: fim >= len(p.candidatos), Versao: p.versao, OrdemAplicada: req.Ordem})
	case "/prospeccao/extracoes":
		var req struct {
			PedidoID  string   `json:"pedido_id"`
			CNPJs     []string `json:"cnpjs"`
			IncluirPF bool     `json:"incluir_pf"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		p.extracoes++
		p.lotes = append(p.lotes, req.CNPJs)
		if p.pagos == nil {
			p.pagos = map[string]int{}
		}
		chave := req.PedidoID + "|" + hashLote(req.CNPJs)
		itens := make([]models.ProspEmpresa, 0, len(req.CNPJs))
		for _, c := range req.CNPJs {
			itens = append(itens, p.empresa(c))
		}
		if _, pago := p.pagos[chave]; !pago {
			if p.usado+len(itens) > p.limite {
				c := p.cota()
				escrever(429, map[string]any{"erro": "cota", "error": "cota", "codigo": "cota", "cota": c})
				return
			}
			p.usado += len(itens)
			p.pagos[chave] = len(itens)
		}
		if p.falharExtracaoApos > 0 && p.extracoes == p.falharExtracaoApos {
			escrever(502, map[string]string{"erro": "falha simulada"})
			return
		}
		escrever(200, services.ExtracaoPlataforma{Itens: itens, NaoEncontrados: []string{}, Cota: p.cota()})
	default:
		escrever(404, map[string]string{"erro": "rota desconhecida"})
	}
}

func prepararGerarLista(t *testing.T, p *plataformaFalsa) *sql.DB {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE prospeccao_jobs, prospeccao_itens, prospeccao_listas, contato_bloqueios RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM audit_log WHERE entity = $1`, services.EntidadeListaProspeccao); err != nil {
		t.Fatal(err)
	}
	esperas := services.ProspeccaoEsperas
	services.ProspeccaoEsperas = []time.Duration{0, 0, 0}
	t.Cleanup(func() { services.ProspeccaoEsperas = esperas })
	if p.versao == "" {
		p.versao = "v1"
	}
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	t.Cleanup(services.UsarPlataformaDeTeste(srv.URL))
	return db
}

func novaLista(t *testing.T, db *sql.DB, l *models.ProspLista) (*models.ProspLista, *models.ProspJob) {
	t.Helper()
	if err := l.Validar(); err != nil {
		t.Fatal(err)
	}
	job := &models.ProspJob{Tipo: models.JobGerarLista}
	if err := models.CreateProspListaComJob(db, l, job); err != nil {
		t.Fatal(err)
	}
	return l, job
}

func rodarJobs(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := services.ProcessarJobsAte(context.Background(), db, 200); err != nil {
		t.Fatal(err)
	}
}

func resultadoGerar(t *testing.T, j *models.ProspJob) services.ResultadoGerarLista {
	t.Helper()
	var r services.ResultadoGerarLista
	if len(j.Resultado) > 0 {
		if err := json.Unmarshal(j.Resultado, &r); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestGerarListaPulaExclusoesEEntrega(t *testing.T) {
	p := &plataformaFalsa{limite: 1000}
	for i := 1; i <= 40; i++ {
		p.candidatos = append(p.candidatos, cnpjTeste(i))
	}
	db := prepararGerarLista(t, p)

	// Bloqueado (CNPJ), já no CRM (com máscara) e já em outra lista.
	if _, err := models.CreateBloqueio(db, &models.Bloqueio{Tipo: models.BloqueioCNPJ, Valor: cnpjTeste(1)}); err != nil {
		t.Fatal(err)
	}
	var companyID int64
	if err := db.QueryRow(`INSERT INTO companies (name, cnpj) VALUES ('Já cliente', $1) RETURNING id`,
		models.FormatarCNPJ(cnpjTeste(2))).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM companies WHERE id = $1`, companyID) })
	outra := &models.ProspLista{Nome: "Outra", Status: models.ListaPronta, Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 1}
	if err := models.CreateProspLista(db, outra); err != nil {
		t.Fatal(err)
	}
	if _, err := models.InsertProspItens(db, outra.ID, []models.ProspItem{{CNPJ: cnpjTeste(3), RazaoSocial: "X"}}); err != nil {
		t.Fatal(err)
	}
	// Repetido na base: o mesmo CNPJ duas vezes conta uma só.
	p.candidatos = append([]string{cnpjTeste(4)}, p.candidatos...)

	var uid int64
	if err := db.QueryRow(`INSERT INTO users (name, email, password_hash, role) VALUES ('Gestora da lista', 'gestora.lista@exemplo.com.br', 'x', 'gestor')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	lista, job := novaLista(t, db, &models.ProspLista{
		Nome: "Restaurantes CE", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 10,
		ExcluirNoCRM: true, ExcluirEmListas: true, CriadoPor: &uid,
	})
	rodarJobs(t, db)

	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobConcluido {
		t.Fatalf("job: %+v", j)
	}
	r := resultadoGerar(t, j)
	if r.Pedidos != 10 || r.Entregues != 10 || r.PuladosBloqueio != 1 || r.PuladosNoCRM != 1 || r.PuladosEmListas != 1 {
		t.Fatalf("resultado: %+v", r)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.Status != models.ListaPronta || l.Contadores.Total != 10 || l.Contadores.Novo != 10 || l.VersaoBase != "v1" {
		t.Fatalf("lista: %+v", l)
	}
	for _, c := range []string{cnpjTeste(1), cnpjTeste(2), cnpjTeste(3)} {
		if m, _ := models.ProspItensPorCNPJ(db, lista.ID, []string{c}); len(m) > 0 {
			t.Fatalf("%s deveria ter sido pulado", c)
		}
	}
	if !p.pedidos[lista.PedidoID] || len(p.lotes) != 1 || len(p.lotes[0]) != 10 || p.usado != 10 {
		t.Fatalf("candidatos com o pedido da lista e uma extração de 10: pedidos=%v lotes=%d usado=%d", p.pedidos, len(p.lotes), p.usado)
	}

	// Item com retrato, nota e motivos.
	itens, _, err := models.ListProspItens(db, lista.ID, models.ProspItemLista{})
	if err != nil || len(itens) != 10 {
		t.Fatalf("itens: %d %v", len(itens), err)
	}
	it := itens[0]
	if it.Nota <= 0 || len(it.NotaMotivos) == 0 || !it.TemCelular || !it.TemEmail || it.TelefonePrincipal == "" || it.UF != "CE" {
		t.Fatalf("item: %+v", it)
	}
	completo, _ := models.ProspItemByID(db, it.ID)
	if completo.Dados == nil || completo.Dados.CNPJ != it.CNPJ {
		t.Fatalf("retrato da empresa não gravado: %+v", completo.Dados)
	}

	// Auditoria "lista gerada" com os filtros e o total.
	var resumo string
	var detalhes []byte
	if err := db.QueryRow(`SELECT summary, details FROM audit_log WHERE entity = $1 AND entity_id = $2 ORDER BY id DESC LIMIT 1`,
		services.EntidadeListaProspeccao, lista.ID).Scan(&resumo, &detalhes); err != nil {
		t.Fatal(err)
	}
	var usuario string
	_ = db.QueryRow(`SELECT COALESCE(user_name,'') FROM audit_log WHERE entity = $1 AND entity_id = $2 ORDER BY id DESC LIMIT 1`,
		services.EntidadeListaProspeccao, lista.ID).Scan(&usuario)
	if usuario != "Gestora da lista" {
		t.Fatalf("auditoria em nome de quem criou o job: %q", usuario)
	}
	if !strings.Contains(resumo, "lista gerada") || !strings.Contains(string(detalhes), `"total": 10`) {
		t.Fatalf("auditoria: %s %s", resumo, detalhes)
	}
}

func TestGerarListaLimitadaPelaCotaEEsgotadaNoMeio(t *testing.T) {
	p := &plataformaFalsa{limite: 1000}
	for i := 1; i <= 600; i++ {
		p.candidatos = append(p.candidatos, cnpjTeste(i))
	}
	// O resumo diz que restam 450, mas o painel só tem saldo para 250 (outra
	// lista gastou no meio): o 2º lote de 200 encolhe para 50 e a lista sai com 250.
	restante := 450
	p.restanteIni = &restante
	p.usado = 750
	db := prepararGerarLista(t, p)
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Cota", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 500})
	rodarJobs(t, db)

	j, _ := models.ProspJobByID(db, job.ID)
	r := resultadoGerar(t, j)
	if j.Status != models.JobConcluido || r.Pedidos != 500 || r.Entregues != 250 || !r.LimitadoPelaCota {
		t.Fatalf("job: %+v resultado %+v", j, r)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.Status != models.ListaPronta || l.Contadores.Total != 250 || !strings.Contains(l.Mensagem, "250 de 500") {
		t.Fatalf("lista: %s %d %q", l.Status, l.Contadores.Total, l.Mensagem)
	}
	if p.usado != 1000 {
		t.Fatalf("cota usada: %d", p.usado)
	}

}

// Cota zerada no meio: encerra como concluído com "cota esgotada: N de M".
func TestGerarListaCotaZeradaNoMeioConclui(t *testing.T) {
	p2 := &plataformaFalsa{limite: 200}
	for i := 1; i <= 600; i++ {
		p2.candidatos = append(p2.candidatos, cnpjTeste(i))
	}
	restante2 := 400
	p2.restanteIni = &restante2
	db := prepararGerarLista(t, p2)
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Cota 2", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 400})
	rodarJobs(t, db)
	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobConcluido || j.Mensagem != "cota esgotada: 200 de 400 entregues" {
		t.Fatalf("job: %s %q", j.Status, j.Mensagem)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.Status != models.ListaPronta || l.Contadores.Total != 200 {
		t.Fatalf("lista: %s %d", l.Status, l.Contadores.Total)
	}
}

func TestGerarListaRetomadaReenviaOsMesmosLotesSemCobrarDeNovo(t *testing.T) {
	p := &plataformaFalsa{limite: 1000, falharExtracaoApos: 2}
	for i := 1; i <= 500; i++ {
		p.candidatos = append(p.candidatos, cnpjTeste(i))
	}
	db := prepararGerarLista(t, p)
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Retomada", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 450})
	rodarJobs(t, db)

	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobConcluido || j.Feitos != 450 {
		t.Fatalf("job: %+v", j)
	}
	// 3 lotes (200, 200, 50) + 1 repetição do 2º, que já tinha sido pago.
	if len(p.lotes) != 4 || hashLote(p.lotes[1]) != hashLote(p.lotes[2]) || p.usado != 450 || p.paginas != 1 {
		t.Fatalf("lotes=%d usado=%d paginas=%d", len(p.lotes), p.usado, p.paginas)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.Contadores.Total != 450 {
		t.Fatalf("itens: %d", l.Contadores.Total)
	}
}

func TestGerarListaRefazBuscaQuandoABaseTroca(t *testing.T) {
	p := &plataformaFalsa{limite: 5000, trocarVersaoNa: 2}
	for i := 1; i <= 3000; i++ {
		p.candidatos = append(p.candidatos, cnpjTeste(i))
	}
	db := prepararGerarLista(t, p)
	outra := &models.ProspLista{Nome: "Outra", Status: models.ListaPronta, Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 1}
	if err := models.CreateProspLista(db, outra); err != nil {
		t.Fatal(err)
	}
	ja := make([]models.ProspItem, 0, 1500)
	for i := 1; i <= 1500; i++ {
		ja = append(ja, models.ProspItem{CNPJ: cnpjTeste(i), RazaoSocial: "X"})
	}
	if _, err := models.InsertProspItens(db, outra.ID, ja); err != nil {
		t.Fatal(err)
	}
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Troca", Filtros: models.ProspFiltros{UFs: []string{"CE"}},
		QuantidadePedida: 600, ExcluirEmListas: true})
	rodarJobs(t, db)

	j, _ := models.ProspJobByID(db, job.ID)
	r := resultadoGerar(t, j)
	if j.Status != models.JobConcluido || r.Entregues != 600 || !p.trocou {
		t.Fatalf("job: %+v %+v", j, r)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.VersaoBase != "v2" || l.Contadores.Total != 600 {
		t.Fatalf("lista: versão %q total %d", l.VersaoBase, l.Contadores.Total)
	}
}

func TestGerarListaSemCotaFalhaEListaFalha(t *testing.T) {
	p := &plataformaFalsa{limite: 100, usado: 100, candidatos: []string{cnpjTeste(1)}}
	db := prepararGerarLista(t, p)
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Sem cota", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 10})
	rodarJobs(t, db)
	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobFalhou || !strings.Contains(j.Mensagem, "cota") {
		t.Fatalf("job: %s %q", j.Status, j.Mensagem)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.Status != models.ListaFalhou || l.Mensagem == "" {
		t.Fatalf("lista: %s %q", l.Status, l.Mensagem)
	}
}

func TestGerarListaCanceladaFicaParcial(t *testing.T) {
	p := &plataformaFalsa{limite: 1000}
	for i := 1; i <= 500; i++ {
		p.candidatos = append(p.candidatos, cnpjTeste(i))
	}
	db := prepararGerarLista(t, p)
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Cancelada", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 450})
	ctx := context.Background()
	// início, candidatos e o primeiro lote.
	for range 3 {
		if _, err := services.ProcessarProximoJob(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := models.CancelarProspJob(db, job.ID); err != nil {
		t.Fatal(err)
	}
	rodarJobs(t, db)
	l, _ := models.ProspListaByID(db, lista.ID)
	if l.Status != models.ListaPronta || l.Contadores.Total != 200 || !strings.Contains(l.Mensagem, "parcial") {
		t.Fatalf("lista: %s %d %q", l.Status, l.Contadores.Total, l.Mensagem)
	}
	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobCancelado || !strings.Contains(j.Mensagem, "200 de 450") {
		t.Fatalf("job: %s %q", j.Status, j.Mensagem)
	}

	// Cancelada antes de entregar: a lista falha.
	lista, job = novaLista(t, db, &models.ProspLista{Nome: "Cancelada cedo", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 10})
	if _, err := models.CancelarProspJob(db, job.ID); err != nil {
		t.Fatal(err)
	}
	rodarJobs(t, db)
	l, _ = models.ProspListaByID(db, lista.ID)
	if l.Status != models.ListaFalhou {
		t.Fatalf("lista cancelada sem itens: %s %q", l.Status, l.Mensagem)
	}
}

func TestGerarListaDeCNPJsPulaPessoaFisica(t *testing.T) {
	p := &plataformaFalsa{limite: 1000, pf: map[string]bool{cnpjTeste(2): true}, semTelefone: map[string]bool{cnpjTeste(3): true}}
	db := prepararGerarLista(t, p)
	lista, job := novaLista(t, db, &models.ProspLista{Nome: "Colada", Origem: models.ListaOrigemCNPJs,
		Filtros: models.ProspFiltros{CNPJs: []string{models.FormatarCNPJ(cnpjTeste(1)), cnpjTeste(2), cnpjTeste(3), cnpjTeste(1)}}})
	rodarJobs(t, db)
	j, _ := models.ProspJobByID(db, job.ID)
	r := resultadoGerar(t, j)
	if j.Status != models.JobConcluido || r.Entregues != 2 || r.PuladosPF != 1 || p.paginas != 0 {
		t.Fatalf("job: %+v %+v paginas=%d", j, r, p.paginas)
	}
	l, _ := models.ProspListaByID(db, lista.ID)
	if !strings.Contains(l.Mensagem, "pessoa física") {
		t.Fatalf("mensagem: %q", l.Mensagem)
	}
	m, _ := models.ProspItensPorCNPJ(db, lista.ID, []string{cnpjTeste(3)})
	it, _ := models.ProspItemByID(db, m[cnpjTeste(3)])
	if it.Nota != 0 || it.TemCelular {
		t.Fatalf("sem telefone: %+v", it)
	}
	tem := false
	for _, mo := range it.NotaMotivos {
		tem = tem || mo.Pontos == -40
	}
	if !tem {
		t.Fatalf("motivo sem telefone ausente: %+v", it.NotaMotivos)
	}
}

func TestCachesDaPlataforma(t *testing.T) {
	var contagens, catalogos int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/prospeccao/contagem":
			contagens++
			_, _ = w.Write([]byte(`{"total": 42, "total_modo": "exato", "parciais": {"com_celular": 10, "com_email": null, "com_divida": 1}}`))
		case "/prospeccao/catalogos/cnaes":
			catalogos++
			_, _ = w.Write([]byte(`{"itens": [{"codigo": "56", "descricao": "Alimentação", "nivel": "divisao", "q": "` + r.URL.Query().Get("q") + `"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(services.UsarPlataformaDeTeste(srv.URL))
	services.LimparCachesProspeccao()
	t.Cleanup(services.LimparCachesProspeccao)
	ctx := context.Background()

	f := models.ProspFiltros{UFs: []string{"CE"}}
	for range 3 {
		c, err := services.PlataformaContagemComCache(ctx, f)
		if err != nil || c.Total != 42 || c.Parciais.ComEmail != nil {
			t.Fatalf("contagem: %+v %v", c, err)
		}
	}
	if _, err := services.PlataformaContagemComCache(ctx, models.ProspFiltros{UFs: []string{"SP"}}); err != nil {
		t.Fatal(err)
	}
	if contagens != 2 {
		t.Fatalf("contagem deveria ir à plataforma 2 vezes, foi %d", contagens)
	}

	for _, q := range []string{"rest", "rest", "padaria"} {
		out, err := services.PlataformaCatalogoComCache(ctx, "cnaes", map[string][]string{"q": {q}})
		if err != nil || !strings.Contains(string(out), q) {
			t.Fatalf("catálogo: %s %v", out, err)
		}
	}
	if catalogos != 2 {
		t.Fatalf("catálogo deveria ir à plataforma 2 vezes, foi %d", catalogos)
	}
}
