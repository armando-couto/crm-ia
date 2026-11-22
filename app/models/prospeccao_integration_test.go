package models_test

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Listas, itens, jobs, leads e bloqueios são SQL de ponta a ponta (FILTER,
// SKIP LOCKED, índices parciais): só Postgres de verdade prova. Como os demais
// testes de integração, pulam sem CRMIA_TEST_DSN.

func limparProspeccao(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE leads, prospeccao_itens, prospeccao_jobs, prospeccao_listas,
		prospeccao_segmentos, contato_bloqueios, calls, activities, deals, contacts, companies
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM app_settings WHERE key IN ('prospeccao', 'motivos_perda')`); err != nil {
		t.Fatal(err)
	}
}

// usuarioDeTeste cria (ou reaproveita) um usuário e o remove ao fim do teste.
func usuarioDeTeste(t *testing.T, db *sql.DB, nome string, ativo bool) int64 {
	t.Helper()
	email := nome + "@prospeccao.teste"
	var id int64
	if err := db.QueryRow(`
		INSERT INTO users (name, email, password_hash, role, active) VALUES ($1, $2, 'x', 'seller', $3)
		ON CONFLICT (email) DO UPDATE SET active = EXCLUDED.active RETURNING id`, nome, email, ativo).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// calls.user_id não tem ON DELETE: sem isto o usuário de teste ficaria no banco.
		db.Exec(`DELETE FROM calls WHERE user_id = $1`, id)
		db.Exec(`DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func cnpjDeTeste(n int) string {
	base := fmt.Sprintf("%08d0001", n)
	return base + models.CNPJDigitosVerificadores(base)
}

func novaListaDeTeste(t *testing.T, db *sql.DB, nome string, criadoPor *int64) (*models.ProspLista, *models.ProspJob) {
	t.Helper()
	l := &models.ProspLista{Nome: nome, Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: 10,
		ExcluirNoCRM: true, ExcluirEmListas: true, CriadoPor: criadoPor}
	if err := l.Validar(); err != nil {
		t.Fatal(err)
	}
	job := &models.ProspJob{Tipo: models.JobGerarLista}
	if err := models.CreateProspListaComJob(db, l, job); err != nil {
		t.Fatal(err)
	}
	return l, job
}

func itensDeTeste(n int) []models.ProspItem {
	itens := make([]models.ProspItem, n)
	for i := range itens {
		itens[i] = models.ProspItem{
			CNPJ: cnpjDeTeste(i + 1), RazaoSocial: fmt.Sprintf("EMPRESA %02d LTDA", i+1), NomeFantasia: fmt.Sprintf("Loja %02d", i+1),
			UF: "ce", Municipio: "FORTALEZA", CnaeCodigo: 4721102, CnaeDescricao: "Padaria", Porte: models.PorteME,
			Nota: 10 * (i + 1), NotaMotivos: []models.NotaMotivo{{Pontos: 35, Texto: "celular próprio"}},
			TemCelular: i%2 == 0, TemEmail: i%3 == 0, PessoaFisica: i == 0,
			TelefonePrincipal: map[bool]string{true: "85999990000", false: ""}[i%2 == 0],
			Dados: &models.ProspEmpresa{CNPJ: cnpjDeTeste(i + 1), RazaoSocial: fmt.Sprintf("EMPRESA %02d LTDA", i+1),
				Telefones: []models.ProspTelefone{{Numero: "85999990000", Tipo: models.TelefoneCelular, Origem: models.OrigemContatoReceita}}},
		}
	}
	return itens
}

func TestProspSegmentosCRUD(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)
	ana := usuarioDeTeste(t, db, "ana", true)
	bia := usuarioDeTeste(t, db, "bia", true)

	s := &models.ProspSegmento{Nome: " Padarias do Ceará ", Filtros: models.ProspFiltros{UFs: []string{"ce"}, CNAEPrefixos: []string{"1091"}},
		Compartilhado: false, CriadoPor: &ana}
	if err := s.Validar(); err != nil {
		t.Fatal(err)
	}
	if err := models.CreateProspSegmento(db, s); err != nil {
		t.Fatal(err)
	}
	if err := models.RegistrarContagemProspSegmento(db, s.ID, 1234); err != nil {
		t.Fatal(err)
	}
	lido, err := models.ProspSegmentoByID(db, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lido.Nome != "Padarias do Ceará" || lido.Ordem != models.OrdemRelevancia || lido.Filtros.UFs[0] != "CE" ||
		lido.UltimaContagem == nil || *lido.UltimaContagem != 1234 || lido.CriadoPorNome != "ana" {
		t.Fatalf("segmento lido errado: %+v", lido)
	}

	// Segmento privado só aparece para quem criou.
	if l, _ := models.ListProspSegmentos(db, bia); len(l) != 0 {
		t.Fatalf("segmento privado vazou para outra pessoa: %+v", l)
	}
	if l, _ := models.ListProspSegmentos(db, ana); len(l) != 1 {
		t.Fatalf("a dona deveria ver o segmento, veio %d", len(l))
	}

	// Mudar o filtro invalida a contagem; mudar só o nome, não.
	lido.Nome = "Padarias CE"
	if err := models.UpdateProspSegmento(db, lido); err != nil {
		t.Fatal(err)
	}
	if lido.UltimaContagem == nil {
		t.Fatal("renomear não deveria apagar a última contagem")
	}
	lido.Filtros.UFs = []string{"CE", "RN"}
	lido.Compartilhado = true
	if err := models.UpdateProspSegmento(db, lido); err != nil {
		t.Fatal(err)
	}
	if lido.UltimaContagem != nil {
		t.Fatal("mudar o filtro deveria invalidar a última contagem")
	}
	if l, _ := models.ListProspSegmentos(db, bia); len(l) != 1 {
		t.Fatal("segmento compartilhado deveria aparecer para todos")
	}
	if err := models.DeleteProspSegmento(db, s.ID); err != nil {
		t.Fatal(err)
	}
	if err := models.DeleteProspSegmento(db, s.ID); err != sql.ErrNoRows {
		t.Fatalf("apagar de novo deveria dar ErrNoRows, veio %v", err)
	}
}

func TestProspListaItensSelecaoEContadores(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)
	ana := usuarioDeTeste(t, db, "ana", true)

	lista, job := novaListaDeTeste(t, db, "Padarias de Fortaleza", &ana)
	if lista.ID == 0 || job.ID == 0 || job.ListaID == nil || *job.ListaID != lista.ID || len(lista.PedidoID) != 36 {
		t.Fatalf("lista e job deveriam nascer juntos: %+v %+v", lista, job)
	}

	// No máximo um job ativo por lista.
	outro := &models.ProspJob{Tipo: models.JobEnriquecer, ListaID: &lista.ID}
	if err := models.CreateProspJob(db, outro); !errors.Is(err, models.ErrJobAtivo) {
		t.Fatalf("segundo job ativo deveria dar ErrJobAtivo, veio %v", err)
	}
	if ativo, err := models.ProspJobAtivoDaLista(db, lista.ID); err != nil || ativo == nil || ativo.ID != job.ID {
		t.Fatalf("job ativo da lista: %+v, %v", ativo, err)
	}
	if err := models.DeleteProspLista(db, lista.ID); !errors.Is(err, models.ErrJobAtivo) {
		t.Fatalf("apagar lista com job ativo deveria dar ErrJobAtivo, veio %v", err)
	}

	// Itens em lote; repetir o lote não duplica.
	itens := itensDeTeste(6)
	n, err := models.InsertProspItens(db, lista.ID, itens)
	if err != nil || n != 6 {
		t.Fatalf("inseridos = %d, %v", n, err)
	}
	if n, _ := models.InsertProspItens(db, lista.ID, itens[:3]); n != 0 {
		t.Fatalf("repetir CNPJ na mesma lista não deveria inserir, inseriu %d", n)
	}

	// Listagem: ordem padrão por nota desc, filtros e paginação.
	pagina, total, err := models.ListProspItens(db, lista.ID, models.ProspItemLista{})
	if err != nil || total != 6 || len(pagina) != 6 || pagina[0].Nota != 60 || pagina[5].Nota != 10 {
		t.Fatalf("listagem por nota: total=%d, %v, primeira nota=%d", total, err, pagina[0].Nota)
	}
	if pagina[0].UF != "CE" || pagina[0].Dados != nil || len(pagina[0].NotaMotivos) != 1 {
		t.Fatalf("item da listagem: %+v", pagina[0])
	}
	sim := true
	f := models.ProspItemLista{ProspItemFiltro: models.ProspItemFiltro{ComCelular: &sim, NotaMin: 30}, Sort: "razao_social"}
	f.PerPage = 1
	pagina, total, err = models.ListProspItens(db, lista.ID, f)
	if err != nil || total != 2 || len(pagina) != 1 || pagina[0].RazaoSocial != "EMPRESA 03 LTDA" {
		t.Fatalf("filtro com_celular + nota_min: total=%d, %v, %+v", total, err, pagina)
	}
	if _, total, _ = models.ListProspItens(db, lista.ID, models.ProspItemLista{ProspItemFiltro: models.ProspItemFiltro{Q: "loja 04"}}); total != 1 {
		t.Fatalf("busca por nome fantasia deveria achar 1, achou %d", total)
	}
	if _, total, _ = models.ListProspItens(db, lista.ID, models.ProspItemLista{ProspItemFiltro: models.ProspItemFiltro{Q: models.FormatarCNPJ(cnpjDeTeste(2))}}); total != 1 {
		t.Fatalf("busca por CNPJ com pontuação deveria achar 1, achou %d", total)
	}

	// Seleção: todos, filtro, exceto e ids; resolução em páginas por id.
	todos := models.ProspSelecao{Todos: true}
	if n, _ := models.CountProspSelecao(db, lista.ID, todos, nil); n != 6 {
		t.Fatalf("todos = %d", n)
	}
	var ids []int64
	apos := int64(0)
	for {
		pag, err := models.ResolveProspSelecao(db, lista.ID, todos, nil, apos, 4)
		if err != nil {
			t.Fatal(err)
		}
		if len(pag) == 0 {
			break
		}
		ids = append(ids, pag...)
		apos = pag[len(pag)-1]
	}
	if len(ids) != 6 {
		t.Fatalf("a paginação por id deveria percorrer os 6 itens, veio %v", ids)
	}
	comFiltro := models.ProspSelecao{Todos: true, Filtro: &models.ProspItemFiltro{ComCelular: &sim}, Exceto: []int64{ids[0]}}
	if n, _ := models.CountProspSelecao(db, lista.ID, comFiltro, nil); n != 2 {
		t.Fatalf("todos com celular (3) menos uma exceção deveria dar 2, deu %d", n)
	}
	porIDs := models.ProspSelecao{IDs: []int64{ids[0], ids[1], 999999}}
	if n, _ := models.CountProspSelecao(db, lista.ID, porIDs, []string{models.ItemNovo}); n != 2 {
		t.Fatalf("ids de outra lista ou inexistentes não contam: %d", n)
	}

	// Descartar guarda o estado anterior; restaurar devolve.
	if n, err := models.DescartarProspItens(db, lista.ID, porIDs); err != nil || n != 2 {
		t.Fatalf("descartar = %d, %v", n, err)
	}
	if n, _ := models.DescartarProspItens(db, lista.ID, porIDs); n != 0 {
		t.Fatal("descartar de novo não deveria afetar ninguém")
	}

	// Enriquecimento: soma o custo (reprocessar não apaga o anterior) e nunca devolve o bruto.
	perfil := &models.PerfilContato{Provedor: "wholead", Telefones: []models.ProspTelefone{{Numero: "85988887777", Tipo: models.TelefoneCelular}},
		Pessoas: []models.PessoaContato{{Nome: "MARIA", Documento: models.MascararCPF("12345678909"), Papel: "socio"}}}
	enr := models.ProspItemEnriquecimento{Estado: models.ItemEnriquecido, Perfil: perfil, Bruto: []byte(`{"cpf":"***456789**"}`),
		Provedor: "wholead", CustoCentavos: 80, Nota: 77, TemCelular: true, TemDecisor: true, TemWhatsApp: true, TelefonePrincipal: "85988887777"}
	for i := 0; i < 2; i++ {
		if err := models.UpdateProspItemEnriquecimento(db, ids[2], enr); err != nil {
			t.Fatal(err)
		}
	}
	item, err := models.ProspItemByID(db, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if item.Estado != models.ItemEnriquecido || item.CustoCentavos != 160 || item.Nota != 77 || !item.TemDecisor ||
		item.Enriquecimento == nil || item.Enriquecimento.Pessoas[0].Documento != "***456789**" ||
		item.Dados == nil || item.Dados.Telefones[0].Numero != "85999990000" || item.EnriquecidoEm == nil {
		t.Fatalf("item enriquecido errado: %+v", item)
	}
	if err := models.UpdateProspItemEnriquecimento(db, ids[3], models.ProspItemEnriquecimento{Estado: models.ItemErro, Erro: "rate_limited"}); err != nil {
		t.Fatal(err)
	}

	// Envio ao CRM: a empresa é gravada antes, mantendo o estado; depois o item vira enviado.
	empresa := &models.Company{Name: "Loja 05", CNPJ: models.FormatarCNPJ(cnpjDeTeste(5))}
	if err := models.CreateCompany(db, empresa); err != nil {
		t.Fatal(err)
	}
	if err := models.SetProspItemEmpresa(db, ids[4], empresa.ID); err != nil {
		t.Fatal(err)
	}
	if it, _ := models.ProspItemByID(db, ids[4]); it.Estado != models.ItemNovo || it.CompanyID == nil || *it.CompanyID != empresa.ID {
		t.Fatalf("gravar a empresa não pode mudar o estado: %+v", it)
	}
	if err := models.SetProspItemEstado(db, ids[4], models.ProspItemEstado{Estado: models.ItemEnviado}); err != nil {
		t.Fatal(err)
	}
	if it, _ := models.ProspItemByID(db, ids[4]); it.Estado != models.ItemEnviado || it.EnviadoEm == nil || it.CompanyID == nil {
		t.Fatalf("item enviado: %+v", it)
	}
	if err := models.SetProspItemEstado(db, ids[4], models.ProspItemEstado{Estado: "inventado"}); err == nil {
		t.Fatal("estado inexistente deveria ser recusado")
	}

	// Contadores da lista.
	lida, err := models.ProspListaByID(db, lista.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := lida.Contadores
	if c.Total != 6 || c.Descartado != 2 || c.Enriquecido != 1 || c.Erro != 1 || c.Enviado != 1 || c.Novo != 1 ||
		c.ComDecisor != 1 || c.ComWhatsApp != 1 || c.PessoaFisica != 1 || c.CustoCentavos != 160 || lida.CriadoPorNome != "ana" {
		t.Fatalf("contadores errados: %+v", c)
	}
	resumo, err := models.ResumoProspSelecao(db, lista.ID, todos)
	if err != nil || resumo.Total != 6 || resumo.PorEstado[models.ItemDescartado] != 2 || resumo.PorEstado[models.ItemEnviado] != 1 {
		t.Fatalf("resumo da seleção: %+v, %v", resumo, err)
	}

	if n, err := models.RestaurarProspItens(db, lista.ID, todos); err != nil || n != 2 {
		t.Fatalf("restaurar = %d, %v", n, err)
	}
	if n, _ := models.CountProspSelecao(db, lista.ID, todos, []string{models.ItemNovo}); n != 3 {
		t.Fatalf("restaurados deveriam voltar a novo: %d", n)
	}

	// CNPJs que já estão em listas e no CRM (com pontuação e com letras).
	alfa := "12.ABC.345/01DE-35"
	if err := models.CreateCompany(db, &models.Company{Name: "Alfanumérica", CNPJ: alfa}); err != nil {
		t.Fatal(err)
	}
	noCRM, err := models.CNPJsNoCRM(db, []string{cnpjDeTeste(5), "12abc34501de35", cnpjDeTeste(1)})
	if err != nil || len(noCRM) != 2 || noCRM[cnpjDeTeste(5)] != empresa.ID || noCRM["12ABC34501DE35"] == 0 {
		t.Fatalf("CNPJsNoCRM: %v, %v", noCRM, err)
	}
	if comp, err := models.CompanyByCNPJ(db, "12abc.345/01de-35"); err != nil || comp.Name != "Alfanumérica" {
		t.Fatalf("CompanyByCNPJ com letras e pontuação: %+v, %v", comp, err)
	}
	if _, err := models.CompanyByCNPJ(db, ""); err != sql.ErrNoRows {
		t.Fatalf("CNPJ vazio deveria dar ErrNoRows, veio %v", err)
	}
	emListas, _ := models.CNPJsEmListas(db, []string{cnpjDeTeste(1), cnpjDeTeste(99)}, 0)
	if len(emListas) != 1 || !emListas[cnpjDeTeste(1)] {
		t.Fatalf("CNPJsEmListas: %v", emListas)
	}
	if propria, _ := models.CNPJsEmListas(db, []string{cnpjDeTeste(1)}, lista.ID); len(propria) != 0 {
		t.Fatal("a própria lista não conta em excluir_em_listas")
	}
	porCNPJ, _ := models.ProspItensPorCNPJ(db, lista.ID, []string{models.FormatarCNPJ(cnpjDeTeste(3))})
	if porCNPJ[cnpjDeTeste(3)] != ids[2] {
		t.Fatalf("ProspItensPorCNPJ: %v", porCNPJ)
	}

	// Atualização parcial e listagem de listas.
	nome := "Padarias (revisada)"
	if err := models.UpdateProspLista(db, lista.ID, models.ProspListaPatch{Nome: &nome, PlaybookID: models.DefinirID(0)}); err != nil {
		t.Fatal(err)
	}
	if err := models.SetProspListaStatus(db, lista.ID, models.ListaPronta, "", "2026-11-v1"); err != nil {
		t.Fatal(err)
	}
	listas, total, err := models.ListProspListas(db, models.ProspListaFilter{Q: "revisada", Status: models.ListaPronta})
	if err != nil || total != 1 || listas[0].VersaoBase != "2026-11-v1" || listas[0].Contadores.Total != 6 {
		t.Fatalf("listagem de listas: %+v, %d, %v", listas, total, err)
	}

	// O custo do enriquecimento mora no job e sobrevive à exclusão da lista.
	if ok, err := models.FinalizarProspJob(db, job.ID, models.JobConcluido, "pronta", map[string]int{"entregues": 6}); err != nil || !ok {
		t.Fatalf("finalizar: %v %v", ok, err)
	}
	jobEnr := &models.ProspJob{Tipo: models.JobEnriquecer, ListaID: &lista.ID, CriadoPor: &ana}
	if err := models.CreateProspJob(db, jobEnr); err != nil {
		t.Fatal(err)
	}
	if err := models.SomarProspJobProgresso(db, jobEnr.ID, 2, 0, 160); err != nil {
		t.Fatal(err)
	}
	if _, err := models.FinalizarProspJob(db, jobEnr.ID, models.JobConcluido, "", nil); err != nil {
		t.Fatal(err)
	}
	doTipo, _ := models.ProspJobsDaLista(db, lista.ID)
	if len(doTipo) != 2 {
		t.Fatalf("o último job de cada tipo deveria aparecer: %d", len(doTipo))
	}
	if err := models.DeleteProspLista(db, lista.ID); err != nil {
		t.Fatalf("sem job ativo a lista pode ser apagada: %v", err)
	}
	if _, err := models.ProspListaByID(db, lista.ID); err != sql.ErrNoRows {
		t.Fatal("a lista deveria ter sumido")
	}
	if _, err := models.ProspItemByID(db, ids[0]); err != sql.ErrNoRows {
		t.Fatal("os itens saem em cascata")
	}
	orfao, err := models.ProspJobByID(db, jobEnr.ID)
	if err != nil || orfao.ListaID != nil || orfao.CustoCentavos != 160 {
		t.Fatalf("o job deveria ficar, sem lista e com o custo: %+v, %v", orfao, err)
	}
	inicio := time.Now().AddDate(0, 0, -1)
	if q, custo, err := models.CustoEnriquecimentoJobs(db, inicio, inicio.AddDate(0, 0, 2)); err != nil || q != 2 || custo != 160 {
		t.Fatalf("uso mensal pelas listas: %d consultas, %d centavos, %v", q, custo, err)
	}
}

func TestProspJobsCicloDeVida(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)

	lista, job := novaListaDeTeste(t, db, "Lista A", nil)
	job2 := &models.ProspJob{Tipo: models.JobEnviarCRM, Parametros: []byte(`{"modo":"unico"}`)} // sem lista: não disputa o índice
	if err := models.CreateProspJob(db, job2); err != nil {
		t.Fatal(err)
	}
	if err := models.CreateProspJob(db, &models.ProspJob{Tipo: "inventado"}); err == nil {
		t.Fatal("tipo de job inexistente deveria ser recusado")
	}

	// Reivindicar: o mais antigo primeiro, vira rodando e sai da fila pelo prazo.
	j, err := models.ReivindicarProspJob(db, nil, time.Minute)
	if err != nil || j == nil || j.ID != job.ID || j.Status != models.JobRodando || j.IniciadoEm == nil {
		t.Fatalf("primeiro job reivindicado: %+v, %v", j, err)
	}
	if outro, _ := models.ReivindicarProspJob(db, []string{models.JobGerarLista}, time.Minute); outro != nil {
		t.Fatalf("job arrendado não pode ser pego de novo: %+v", outro)
	}
	j2, err := models.ReivindicarProspJob(db, []string{models.JobEnviarCRM}, time.Minute)
	if err != nil || j2 == nil || j2.ID != job2.ID || string(j2.Parametros) != `{"modo": "unico"}` {
		t.Fatalf("filtro por tipo: %+v, %v", j2, err)
	}
	if nada, err := models.ReivindicarProspJob(db, nil, time.Minute); err != nil || nada != nil {
		t.Fatalf("fila vazia deveria devolver nil, nil: %+v, %v", nada, err)
	}

	// Estado, total e progresso.
	if err := models.SetProspJobEstado(db, job.ID, map[string]any{"lotes": [][]string{{"a"}}, "proximo": 0}); err != nil {
		t.Fatal(err)
	}
	_ = models.SetProspJobTotal(db, job.ID, 10)
	_ = models.SomarProspJobProgresso(db, job.ID, 3, 1, 0)
	_ = models.SomarProspJobProgresso(db, job.ID, 2, 0, 0)
	if err := models.ReagendarProspJob(db, job.ID, 0, 0, "extraindo"); err != nil {
		t.Fatal(err)
	}
	j, err = models.ReivindicarProspJob(db, nil, time.Minute)
	if err != nil || j == nil || j.ID != job.ID || j.Total != 10 || j.Feitos != 5 || j.Erros != 1 || j.Mensagem != "extraindo" {
		t.Fatalf("job reagendado: %+v, %v", j, err)
	}

	// Órfãos: o processo morreu com o job rodando; na subida ele volta à fila.
	if n, err := models.RecuperarProspJobsOrfaos(db); err != nil || n != 2 {
		t.Fatalf("órfãos recuperados = %d, %v", n, err)
	}
	if j, _ = models.ReivindicarProspJob(db, nil, time.Minute); j == nil || j.ID != job.ID {
		t.Fatalf("órfão deveria voltar à fila: %+v", j)
	}

	// Cancelar: sai de rodando, libera a lista para outro job e pede limpeza.
	cancelado, err := models.CancelarProspJob(db, job.ID)
	if err != nil || cancelado.Status != models.JobCancelado || cancelado.FinalizadoEm == nil || !cancelado.LimpezaPendente {
		t.Fatalf("cancelado: %+v, %v", cancelado, err)
	}
	if c, _ := models.ProspJobCancelado(db, job.ID); !c {
		t.Fatal("ProspJobCancelado deveria responder true")
	}
	if ok, _ := models.FinalizarProspJob(db, job.ID, models.JobConcluido, "", nil); ok {
		t.Fatal("job cancelado não pode ser concluído pela fatia que ainda rodava")
	}
	if err := models.ReagendarProspJob(db, job.ID, 0, 0, "x"); err != nil {
		t.Fatal(err)
	}
	limpeza, err := models.ReivindicarProspJob(db, []string{models.JobGerarLista}, time.Minute)
	if err != nil || limpeza == nil || limpeza.ID != job.ID || limpeza.Status != models.JobCancelado {
		t.Fatalf("cancelado com limpeza pendente deveria ser entregue ao worker: %+v, %v", limpeza, err)
	}
	if err := models.ConcluirLimpezaProspJob(db, job.ID, "cancelado: 5 de 10 entregues", map[string]int{"entregues": 5}); err != nil {
		t.Fatal(err)
	}
	final, _ := models.ProspJobByID(db, job.ID)
	if final.LimpezaPendente || final.Mensagem != "cancelado: 5 de 10 entregues" || string(final.Resultado) != `{"entregues": 5}` {
		t.Fatalf("job depois da limpeza: %+v", final)
	}
	if again, _ := models.CancelarProspJob(db, job.ID); again.Mensagem != final.Mensagem {
		t.Fatal("cancelar job já encerrado não muda nada")
	}
	if _, err := models.CancelarProspJob(db, 999999); err != sql.ErrNoRows {
		t.Fatalf("job inexistente deveria dar ErrNoRows, veio %v", err)
	}
	// Com o primeiro cancelado, a lista aceita outro job.
	if err := models.CreateProspJob(db, &models.ProspJob{Tipo: models.JobEnriquecer, ListaID: &lista.ID}); err != nil {
		t.Fatalf("lista sem job ativo deveria aceitar um novo: %v", err)
	}
}

func novoLeadDeTeste(t *testing.T, db *sql.DB, titulo string, mexer func(*models.Lead)) *models.Lead {
	t.Helper()
	l := &models.Lead{Titulo: titulo, Origem: models.LeadOrigemProspeccao}
	if mexer != nil {
		mexer(l)
	}
	if _, err := models.CreateLead(db, l); err != nil {
		t.Fatal(err)
	}
	if l.Estado != models.LeadNovo {
		estado := l.Estado
		if err := models.UpdateLead(db, l.ID, models.LeadPatch{Estado: &estado}); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func TestLeadsFilaTravaEVisibilidade(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)
	ana := usuarioDeTeste(t, db, "ana", true)
	bia := usuarioDeTeste(t, db, "bia", true)
	caio := usuarioDeTeste(t, db, "caio", false) // desativado: os leads dele contam como sem dono

	agora := time.Now()
	ha := func(d time.Duration) *time.Time { t := agora.Add(-d); return &t }
	em := func(d time.Duration) *time.Time { t := agora.Add(d); return &t }

	retornoAntigo := novoLeadDeTeste(t, db, "Retorno antigo", func(l *models.Lead) {
		l.OwnerID = &ana
		l.Estado = models.LeadRetornar
		l.ProximaAcaoEm = ha(48 * time.Hour)
	})
	retornoNovo := novoLeadDeTeste(t, db, "Retorno recente", func(l *models.Lead) {
		l.OwnerID = &ana
		l.Estado = models.LeadRetornar
		l.ProximaAcaoEm = ha(time.Hour)
	})
	tentativa := novoLeadDeTeste(t, db, "Tentativa vencida", func(l *models.Lead) {
		l.OwnerID = &ana
		l.Estado = models.LeadTentando
		l.ProximaAcaoEm = ha(72 * time.Hour)
	})
	novoBom := novoLeadDeTeste(t, db, "Novo nota 90", func(l *models.Lead) { l.OwnerID = &ana; l.Nota = 90; l.ProximaAcaoEm = ha(time.Minute) })
	novoFraco := novoLeadDeTeste(t, db, "Novo nota 50", func(l *models.Lead) { l.OwnerID = &ana; l.Nota = 50 })
	futuro := novoLeadDeTeste(t, db, "Só amanhã", func(l *models.Lead) {
		l.OwnerID = &ana
		l.Estado = models.LeadTentando
		l.ProximaAcaoEm = em(30 * time.Hour)
	})
	bloqueado := novoLeadDeTeste(t, db, "CNPJ bloqueado", func(l *models.Lead) {
		l.OwnerID = &ana
		l.Nota = 100
		l.CNPJ = cnpjDeTeste(77)
		l.Estado = models.LeadRetornar
		l.ProximaAcaoEm = ha(90 * time.Hour)
	})
	daBia := novoLeadDeTeste(t, db, "Lead da Bia", func(l *models.Lead) { l.OwnerID = &bia; l.Nota = 99; l.ProximaAcaoEm = ha(time.Hour) })
	semDono := novoLeadDeTeste(t, db, "Sem dono", func(l *models.Lead) { l.Nota = 95; l.ProximaAcaoEm = ha(time.Hour) })
	doInativo := novoLeadDeTeste(t, db, "Dono desativado", func(l *models.Lead) { l.OwnerID = &caio; l.Nota = 96; l.ProximaAcaoEm = ha(time.Hour) })
	if _, err := models.CreateBloqueio(db, &models.Bloqueio{Tipo: models.BloqueioCNPJ, Valor: models.FormatarCNPJ(cnpjDeTeste(77)), Motivo: models.BloqueioOptOut}); err != nil {
		t.Fatal(err)
	}

	// --- Visibilidade por dono --------------------------------------------
	visAna := models.LeadVisao{UserID: ana}
	if _, err := models.LeadByID(db, daBia.ID, visAna); err != sql.ErrNoRows {
		t.Fatalf("lead alheio deveria ser invisível (ErrNoRows), veio %v", err)
	}
	for _, l := range []*models.Lead{semDono, doInativo, novoBom} {
		if _, err := models.LeadByID(db, l.ID, visAna); err != nil {
			t.Fatalf("%q deveria ser visível para a Ana: %v", l.Titulo, err)
		}
	}
	if l, err := models.LeadByID(db, daBia.ID, models.VisaoTotal(ana)); err != nil || l.OwnerName != "bia" || l.SemDono() {
		t.Fatalf("quem tem leads.assign vê tudo: %+v, %v", l, err)
	}
	if l, _ := models.LeadByID(db, doInativo.ID, visAna); !l.SemDono() || l.OwnerAtivo {
		t.Fatalf("lead de dono desativado conta como sem dono: %+v", l)
	}

	leads, total, cont, err := models.ListLeads(db, models.LeadFilter{Visao: visAna})
	if err != nil {
		t.Fatal(err)
	}
	if total != 7 || cont.Meus != 7 || cont.SemDono != 2 || cont.Todos != 9 {
		t.Fatalf("fila 'meus' da Ana: total=%d contadores=%+v", total, cont)
	}
	if cont.Novo != 2 || cont.Tentando != 2 || cont.Retornar != 3 || cont.Atrasados < 3 {
		t.Fatalf("contadores de estado da fila atual: %+v", cont)
	}
	if leads[0].ID != bloqueado.ID || leads[0].Urgencia != models.UrgenciaAtrasado {
		t.Fatalf("ordem por urgência: o mais vencido primeiro, veio %q (%s)", leads[0].Titulo, leads[0].Urgencia)
	}
	_, total, _, _ = models.ListLeads(db, models.LeadFilter{Visao: visAna, Fila: "todos"})
	if total != 9 {
		t.Fatalf("sem leads.assign, 'todos' = meus + sem dono (9), veio %d", total)
	}
	_, total, _, _ = models.ListLeads(db, models.LeadFilter{Visao: visAna, Fila: "todos", OwnerID: bia})
	if total != 0 {
		t.Fatalf("dono de outra pessoa não vaza para quem não tem leads.assign: %d", total)
	}
	_, total, cont, _ = models.ListLeads(db, models.LeadFilter{Visao: models.VisaoTotal(ana), Fila: "todos"})
	if total != 10 || cont.Todos != 10 || cont.SemDono != 2 {
		t.Fatalf("visão total: total=%d %+v", total, cont)
	}
	leads, total, cont, _ = models.ListLeads(db, models.LeadFilter{Visao: visAna, Fila: "sem_dono", Estado: models.LeadNovo, Sort: "nota"})
	if total != 2 || leads[0].ID != doInativo.ID || cont.Novo != 2 {
		t.Fatalf("fila sem dono por nota: %d %+v", total, cont)
	}
	_, total, cont, _ = models.ListLeads(db, models.LeadFilter{Visao: visAna, Urgencia: models.UrgenciaFuturo, Q: "amanhã"})
	if total != 1 || cont.Tentando != 1 || cont.Novo != 0 {
		t.Fatalf("urgência + busca: total=%d %+v", total, cont)
	}

	// --- Fila: ordem, trava e pular ---------------------------------------
	opcoes := models.FilaOpcoes{UserID: ana}
	l, err := models.ProximoLeadDaFila(db, opcoes)
	if err != nil || l == nil || l.ID != retornoAntigo.ID {
		t.Fatalf("primeiro da fila deveria ser o retorno mais antigo (o de CNPJ bloqueado fica fora): %+v, %v", l, err)
	}
	if l.EmAtendimentoPor == nil || *l.EmAtendimentoPor != ana || l.EmAtendimentoAte == nil || time.Until(*l.EmAtendimentoAte) < 19*time.Minute {
		t.Fatalf("o lead deveria sair travado por 20 minutos: %+v", l)
	}
	// Recarregar a página não troca de lead.
	if de, _ := models.ProximoLeadDaFila(db, opcoes); de == nil || de.ID != retornoAntigo.ID {
		t.Fatalf("o lead já travado pelo usuário deveria voltar: %+v", de)
	}
	// Pular: solta a trava do pulado e segue a ordem (retornos, tentativas, novos por nota).
	pulados := []int64{retornoAntigo.ID}
	esperados := []*models.Lead{retornoNovo, tentativa, novoBom, novoFraco}
	for _, quer := range esperados {
		opcoes.PularIDs = pulados
		de, err := models.ProximoLeadDaFila(db, opcoes)
		if err != nil || de == nil || de.ID != quer.ID {
			t.Fatalf("esperava %q na fila, veio %+v (%v)", quer.Titulo, de, err)
		}
		pulados = append(pulados, de.ID)
	}
	opcoes.PularIDs = pulados
	if vazio, err := models.ProximoLeadDaFila(db, opcoes); err != nil || vazio != nil {
		t.Fatalf("fila esgotada deveria devolver nil (lead futuro, bloqueado e alheios ficam fora): %+v, %v", vazio, err)
	}
	var travados int
	db.QueryRow(`SELECT COUNT(*) FROM leads WHERE em_atendimento_por = $1`, ana).Scan(&travados)
	if travados != 0 {
		t.Fatalf("com a fila vazia as travas do usuário são soltas, sobraram %d", travados)
	}
	_ = futuro

	// Sem dono só entra com a configuração; lead travado por outra pessoa não sai.
	comSemDono := models.FilaOpcoes{UserID: bia, IncluirSemDono: true}
	primeiro, _ := models.ProximoLeadDaFila(db, comSemDono)
	if primeiro == nil || primeiro.ID != daBia.ID {
		t.Fatalf("novos por nota: o lead da Bia (99) vem antes dos sem dono: %+v", primeiro)
	}
	opcoesAna := models.FilaOpcoes{UserID: ana, IncluirSemDono: true, PularIDs: pulados}
	deAna, _ := models.ProximoLeadDaFila(db, opcoesAna)
	if deAna == nil || deAna.ID != doInativo.ID {
		t.Fatalf("com incluir_sem_dono a Ana pega o de dono desativado (nota 96): %+v", deAna)
	}
	comSemDono.PularIDs = []int64{daBia.ID}
	deBia, _ := models.ProximoLeadDaFila(db, comSemDono)
	if deBia == nil || deBia.ID != semDono.ID {
		t.Fatalf("o lead travado pela Ana é pulado; a Bia recebe o outro sem dono: %+v", deBia)
	}

	// Resumo da fila da Ana (sem os sem dono): 5 elegíveis, 2 novos, 2 retornos vencidos.
	nums, err := models.ResumoDaFila(db, models.FilaOpcoes{UserID: ana}, agora)
	if err != nil || nums.NaFila != 5 || nums.Novos != 2 || nums.RetornosVencidos != 2 {
		t.Fatalf("resumo da fila: %+v, %v", nums, err)
	}

	// --- Resultado: guarda de estado e de trava ---------------------------
	visBia := models.LeadVisao{UserID: bia}
	tx, _ := db.Begin()
	_, err = models.PrepararResultadoLead(tx, doInativo.ID, visBia) // travado pela Ana
	tx.Rollback()
	var emAtendimento *models.ErroLeadEmAtendimento
	if !errors.As(err, &emAtendimento) || emAtendimento.UserID != ana || emAtendimento.Nome != "ana" {
		t.Fatalf("lead travado por outra pessoa deveria dar ErroLeadEmAtendimento, veio %v", err)
	}
	tx, _ = db.Begin()
	_, err = models.PrepararResultadoLead(tx, novoBom.ID, visBia) // lead da Ana: fora da visão da Bia
	tx.Rollback()
	if err != sql.ErrNoRows {
		t.Fatalf("lead fora da visão deveria dar ErrNoRows, veio %v", err)
	}

	tx, _ = db.Begin()
	lead, err := models.PrepararResultadoLead(tx, doInativo.ID, visAna)
	if err != nil || lead.EmAtendimentoPor != nil {
		t.Fatalf("quem detém a trava registra o resultado e a trava é solta: %+v, %v", lead, err)
	}
	// Uma chamada grava tudo: ligação com lead, desfecho, número e chave; lead atualizado.
	chave := models.NovoPedidoID()
	call := &models.Call{LeadID: &lead.ID, UserID: &ana, Outcome: models.OutcomeDoResultado(models.ResultadoNaoAtendeu),
		Desfecho: models.ResultadoNaoAtendeu, Numero: "(85) 99999-0000", Chave: chave, Duration: 12}
	if err := models.CreateCallTx(tx, call); err != nil {
		t.Fatal(err)
	}
	tentativas, resultado, estado := 1, models.ResultadoNaoAtendeu, models.LeadTentando
	if err := models.UpdateLead(tx, lead.ID, models.LeadPatch{Estado: &estado, Tentativas: &tentativas, UltimoResultado: &resultado,
		UltimoContatoEm: &agora, ProximaAcaoEm: models.DefinirTempo(agora.Add(3 * time.Hour)), Owner: models.DefinirID(ana)}); err != nil {
		t.Fatal(err)
	}
	// Repetir a mesma chave (duplo clique) não grava de novo nem aborta a transação.
	repetida := &models.Call{LeadID: &lead.ID, UserID: &ana, Desfecho: models.ResultadoNaoAtendeu, Chave: chave}
	if err := models.CreateCallTx(tx, repetida); !errors.Is(err, models.ErrChamadaRepetida) {
		t.Fatalf("chave repetida deveria dar ErrChamadaRepetida, veio %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	original, err := models.CallByChave(db, chave)
	if err != nil || original.ID != call.ID || original.Numero != "85999990000" || original.Desfecho != models.ResultadoNaoAtendeu ||
		original.LeadID == nil || *original.LeadID != lead.ID {
		t.Fatalf("chamada original pela chave: %+v, %v", original, err)
	}
	hist, _ := models.TentativasDoLead(db, lead.ID)
	if len(hist) != 1 || hist[0].Resultado != models.ResultadoNaoAtendeu || hist[0].UserName != "ana" || hist[0].DuracaoSegundos != 12 {
		t.Fatalf("histórico de tentativas: %+v", hist)
	}
	hoje := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.Local)
	if n, _ := models.TentativasSemContatoDesde(db, lead.ID, hoje); n != 1 {
		t.Fatalf("tentativas sem contato hoje = %d", n)
	}
	depois, _ := models.LeadByID(db, lead.ID, visAna)
	if depois.Estado != models.LeadTentando || depois.Tentativas != 1 || depois.OwnerID == nil || *depois.OwnerID != ana ||
		depois.UltimoResultado != models.ResultadoNaoAtendeu || depois.Urgencia == models.UrgenciaNenhuma {
		t.Fatalf("lead depois do resultado: %+v", depois)
	}
	if nums, _ := models.ResumoDaFila(db, models.FilaOpcoes{UserID: ana}, agora); nums.FeitasHoje != 1 || nums.ConectadasHoje != 0 {
		t.Fatalf("ligações de hoje: %+v", nums)
	}

	// Liberar: só quem detém a trava solta.
	if soltou, _ := models.LiberarLead(db, semDono.ID, ana); soltou {
		t.Fatal("liberar de quem não detém a trava não tem efeito")
	}
	if soltou, _ := models.LiberarLead(db, semDono.ID, bia); !soltou {
		t.Fatal("quem detém a trava deveria conseguir soltar")
	}

	// --- Ações em massa, descarte, reabertura e conversão ------------------
	n, err := models.BulkLeads(db, visAna, models.LeadBulk{Acao: models.LeadBulkDescartar, Motivo: "Fora do perfil",
		IDs: []int64{novoFraco.ID, daBia.ID, tentativa.ID}})
	if err != nil || n != 2 {
		t.Fatalf("descartar em massa ignora o lead fora da visão: afetados=%d, %v", n, err)
	}
	if l, _ := models.LeadByID(db, daBia.ID, models.VisaoTotal(ana)); l.Estado != models.LeadNovo {
		t.Fatal("o lead alheio não podia ter sido descartado")
	}
	if l, _ := models.LeadByID(db, novoFraco.ID, visAna); l.Estado != models.LeadDescartado || l.MotivoDescarte != "Fora do perfil" || l.ProximaAcaoEm != nil {
		t.Fatalf("lead descartado: %+v", l)
	}
	if err := models.DescartarLead(db, novoFraco.ID, models.DescarteManual); !errors.Is(err, models.ErrLeadEncerrado) {
		t.Fatalf("descartar lead já encerrado deveria dar ErrLeadEncerrado, veio %v", err)
	}
	tx, _ = db.Begin()
	_, err = models.PrepararResultadoLead(tx, novoFraco.ID, visAna)
	tx.Rollback()
	if !errors.Is(err, models.ErrLeadEncerrado) {
		t.Fatalf("resultado em lead encerrado deveria dar ErrLeadEncerrado, veio %v", err)
	}
	if err := models.ReabrirLead(db, novoBom.ID, agora); !errors.Is(err, models.ErrLeadEstado) {
		t.Fatalf("reabrir só vale para descartado, veio %v", err)
	}
	if err := models.ReabrirLead(db, novoFraco.ID, agora); err != nil {
		t.Fatal(err)
	}
	if l, _ := models.LeadByID(db, novoFraco.ID, visAna); l.Estado != models.LeadNovo || l.MotivoDescarte != "" || l.ProximaAcaoEm == nil {
		t.Fatalf("reaberto sem ligação volta a novo: %+v", l)
	}
	if n, _ := models.BulkLeads(db, visAna, models.LeadBulk{Acao: models.LeadBulkReabrir, IDs: []int64{tentativa.ID, novoBom.ID}, ProximaAcao: agora}); n != 1 {
		t.Fatalf("reabrir em massa só alcança descartados: %d", n)
	}
	if n, _ := models.BulkLeads(db, visAna, models.LeadBulk{Acao: models.LeadBulkEtiqueta, Etiqueta: "feira", IDs: []int64{novoBom.ID, novoFraco.ID}}); n != 2 {
		t.Fatalf("etiqueta em massa: %d", n)
	}
	if n, _ := models.BulkLeads(db, visAna, models.LeadBulk{Acao: models.LeadBulkEtiqueta, Etiqueta: "feira", IDs: []int64{novoBom.ID}}); n != 0 {
		t.Fatal("etiqueta repetida não conta como afetado")
	}
	if n, _ := models.BulkLeads(db, models.VisaoTotal(ana), models.LeadBulk{Acao: models.LeadBulkDono, OwnerID: &bia, IDs: []int64{novoBom.ID, semDono.ID}}); n != 2 {
		t.Fatalf("atribuir em massa: %d", n)
	}
	_, total, _, _ = models.ListLeads(db, models.LeadFilter{Visao: models.VisaoTotal(ana), Fila: "todos", Etiqueta: "feira", OwnerID: bia})
	if total != 1 {
		t.Fatalf("filtro por etiqueta e dono: %d", total)
	}
	if _, err := models.BulkLeads(db, visAna, models.LeadBulk{Acao: "explodir", IDs: []int64{1}}); err == nil {
		t.Fatal("ação em massa desconhecida deveria ser recusada")
	}

	// Patch parcial: só o que veio muda; opt-in de WhatsApp carimba a data.
	sim, temperatura := true, "quente"
	valor := 1500.50
	if err := models.UpdateLead(db, retornoNovo.ID, models.LeadPatch{Temperatura: &temperatura, WhatsAppOptin: &sim,
		ValorEstimado: models.OpcionalFloat{Definir: true, Valor: &valor},
		Telefones:     &[]models.ProspTelefone{{Numero: "85999990000", Tipo: models.TelefoneCelular, Principal: true}}}); err != nil {
		t.Fatal(err)
	}
	editado, _ := models.LeadByID(db, retornoNovo.ID, visAna)
	if editado.Temperatura != "quente" || !editado.WhatsAppOptin || editado.ValorEstimado == nil || *editado.ValorEstimado != 1500.50 ||
		len(editado.Telefones) != 1 || editado.Titulo != "Retorno recente" || editado.Estado != models.LeadRetornar {
		t.Fatalf("patch parcial: %+v", editado)
	}
	if err := models.UpdateLead(db, 999999, models.LeadPatch{Temperatura: &temperatura}); err != sql.ErrNoRows {
		t.Fatalf("patch em lead inexistente deveria dar ErrNoRows, veio %v", err)
	}
	if err := models.MarcarLeadVisto(db, retornoNovo.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := models.LeadByID(db, retornoNovo.ID, visAna); !l.Visto {
		t.Fatal("lead aberto deveria ficar marcado como visto")
	}

	// Conversão: só de lead aberto, e uma vez só.
	var stageID, pipelineID int64
	if err := db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).Scan(&stageID, &pipelineID); err != nil {
		t.Fatal(err)
	}
	deal := &models.Deal{Name: "Negócio do lead", PipelineID: pipelineID, StageID: stageID}
	if err := models.CreateDeal(db, deal); err != nil {
		t.Fatal(err)
	}
	if err := models.ConverterLead(db, retornoNovo.ID, deal.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := models.ConverterLead(db, retornoNovo.ID, deal.ID, nil); !errors.Is(err, models.ErrLeadEncerrado) {
		t.Fatalf("converter duas vezes deveria dar ErrLeadEncerrado, veio %v", err)
	}
	conv, _ := models.LeadByID(db, retornoNovo.ID, visAna)
	if conv.Estado != models.LeadConvertido || conv.DealID == nil || *conv.DealID != deal.ID || conv.ConvertidoEm == nil || conv.Urgencia != models.UrgenciaNenhuma {
		t.Fatalf("lead convertido: %+v", conv)
	}
	if err := models.DeleteLead(db, retornoAntigo.ID); err != nil {
		t.Fatal(err)
	}
	if err := models.DeleteLead(db, retornoAntigo.ID); err != sql.ErrNoRows {
		t.Fatalf("apagar de novo deveria dar ErrNoRows, veio %v", err)
	}
}

func TestLeadDeItemEIdempotenteEListasDosLeads(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)
	ana := usuarioDeTeste(t, db, "ana", true)
	bia := usuarioDeTeste(t, db, "bia", true)

	lista, _ := novaListaDeTeste(t, db, "Mercados", &ana)
	if _, err := models.InsertProspItens(db, lista.ID, itensDeTeste(2)); err != nil {
		t.Fatal(err)
	}
	ids, _ := models.ResolveProspSelecao(db, lista.ID, models.ProspSelecao{Todos: true}, nil, 0, 10)
	empresa := &models.Company{Name: "Mercado Bom", CNPJ: cnpjDeTeste(1)}
	if err := models.CreateCompany(db, empresa); err != nil {
		t.Fatal(err)
	}

	novo := func() *models.Lead {
		return &models.Lead{Titulo: "Mercado Bom", CompanyID: &empresa.ID, CNPJ: models.FormatarCNPJ(cnpjDeTeste(1)), Cidade: "Fortaleza", UF: "ce",
			Origem: models.LeadOrigemProspeccao, ListaID: &lista.ID, ItemID: &ids[0], OwnerID: &ana, Nota: 80,
			Telefones: []models.ProspTelefone{{Numero: "85999990000", Tipo: models.TelefoneCelular}}, Emails: []string{"contato@mercado.teste"},
			Resumo: models.ResumoLead{Frases: []string{"Mercado em Fortaleza/CE"}, Sinais: []models.Sinal{{Tipo: "divida", Nivel: "atencao", Texto: "tem dívida ativa"}}}}
	}
	l1 := novo()
	criado, err := models.CreateLead(db, l1)
	if err != nil || !criado {
		t.Fatalf("primeiro lead do item: %v %v", criado, err)
	}
	// Retomada do job: o mesmo item não gera segundo lead.
	l2 := novo()
	criado, err = models.CreateLead(db, l2)
	if err != nil || criado || l2.ID != l1.ID {
		t.Fatalf("o segundo lead do mesmo item deveria reaproveitar o primeiro: criado=%v id=%d/%d err=%v", criado, l2.ID, l1.ID, err)
	}
	lido, err := models.LeadByID(db, l1.ID, models.LeadVisao{UserID: ana})
	if err != nil {
		t.Fatal(err)
	}
	if lido.CNPJ != cnpjDeTeste(1) || lido.UF != "CE" || lido.ListaNome != "Mercados" || lido.CompanyName != "Mercado Bom" ||
		lido.Resumo.Frases[0] != "Mercado em Fortaleza/CE" || lido.Emails[0] != "contato@mercado.teste" || lido.Visto {
		t.Fatalf("lead lido: %+v", lido)
	}
	if id, err := models.LeadAbertoDaEmpresa(db, empresa.ID); err != nil || id == nil || *id != l1.ID {
		t.Fatalf("lead aberto da empresa: %v, %v", id, err)
	}
	if err := models.DescartarLead(db, l1.ID, models.DescarteSemContato); err != nil {
		t.Fatal(err)
	}
	if id, _ := models.LeadAbertoDaEmpresa(db, empresa.ID); id != nil {
		t.Fatal("lead descartado não conta como aberto")
	}

	// GET /leads/listas: só listas com leads visíveis ao usuário.
	daAna, _ := models.ListasDosLeads(db, models.LeadVisao{UserID: ana})
	if len(daAna) != 1 || daAna[0].ID != lista.ID || daAna[0].Nome != "Mercados" || daAna[0].Abertos != 0 {
		t.Fatalf("listas dos leads da Ana: %+v", daAna)
	}
	if daBia, _ := models.ListasDosLeads(db, models.LeadVisao{UserID: bia}); len(daBia) != 0 {
		t.Fatalf("a Bia não tem lead nessa lista: %+v", daBia)
	}
	_, total, _, _ := models.ListLeads(db, models.LeadFilter{Visao: models.LeadVisao{UserID: ana}, ListaID: lista.ID, Q: models.FormatarCNPJ(cnpjDeTeste(1))})
	if total != 1 {
		t.Fatalf("busca de lead por CNPJ com pontuação: %d", total)
	}
}

func TestBloqueios(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)
	ana := usuarioDeTeste(t, db, "ana", true)

	b := &models.Bloqueio{Tipo: models.BloqueioTelefone, Valor: "+55 (85) 99999-1234", Motivo: models.BloqueioNumeroErrado, Canal: "fila", CriadoPor: &ana}
	criado, err := models.CreateBloqueio(db, b)
	if err != nil || !criado || b.Valor != "85999991234" || b.ID == 0 {
		t.Fatalf("bloqueio de telefone: %+v, %v, %v", b, criado, err)
	}
	// O mesmo número em outra grafia já está bloqueado: não é erro e devolve o existente.
	de := &models.Bloqueio{Tipo: models.BloqueioTelefone, Valor: "85 99999 1234", Motivo: models.BloqueioOptOut}
	criado, err = models.CreateBloqueio(db, de)
	if err != nil || criado || de.ID != b.ID || de.Motivo != models.BloqueioNumeroErrado {
		t.Fatalf("bloqueio repetido: %+v, %v, %v", de, criado, err)
	}
	if _, err := models.CreateBloqueio(db, &models.Bloqueio{Tipo: models.BloqueioCNPJ, Valor: "11222333000182"}); err == nil {
		t.Fatal("CNPJ inválido deveria ser recusado")
	}
	if _, err := models.CreateBloqueio(db, &models.Bloqueio{Tipo: models.BloqueioEmail, Valor: "x@y.com", Motivo: "porque sim"}); err == nil {
		t.Fatal("motivo fora da lista deveria ser recusado")
	}

	criados, repetidos, invalidos, err := models.ImportarBloqueios(db, models.BloqueioEmail, models.BloqueioListaExterna,
		[]string{"Ana@Exemplo.com.br", "ana@exemplo.com.br", "sem-arroba", "", "beto@exemplo.com.br"}, &ana)
	if err != nil || criados != 2 || repetidos != 1 || invalidos != 1 {
		t.Fatalf("importação: criados=%d repetidos=%d invalidos=%d err=%v", criados, repetidos, invalidos, err)
	}
	criados, repetidos, _, _ = models.ImportarBloqueios(db, models.BloqueioEmail, "", []string{"beto@exemplo.com.br", "caio@exemplo.com.br"}, nil)
	if criados != 1 || repetidos != 1 {
		t.Fatalf("segunda importação: criados=%d repetidos=%d", criados, repetidos)
	}
	if _, _, _, err := models.ImportarBloqueios(db, models.BloqueioEmail, "", make([]string, models.MaxBloqueiosImportacao+1), nil); err == nil {
		t.Fatal("importação acima do teto deveria ser recusada")
	}
	cnpj := "12.ABC.345/01DE-35"
	if _, err := models.CreateBloqueio(db, &models.Bloqueio{Tipo: models.BloqueioCNPJ, Valor: cnpj, Motivo: models.BloqueioPedidoTitular, Observacao: "pedido por e-mail"}); err != nil {
		t.Fatal(err)
	}

	lista, total, err := models.ListBloqueios(db, models.BloqueioFilter{})
	if err != nil || total != 5 || len(lista) != 5 {
		t.Fatalf("listagem: %d, %v", total, err)
	}
	if _, total, _ = models.ListBloqueios(db, models.BloqueioFilter{Tipo: models.BloqueioEmail, Q: "EXEMPLO"}); total != 3 {
		t.Fatalf("filtro por tipo e busca: %d", total)
	}
	if lista, total, _ = models.ListBloqueios(db, models.BloqueioFilter{Q: "12.abc.345"}); total != 1 || lista[0].Valor != "12ABC34501DE35" {
		t.Fatalf("busca por CNPJ com pontuação: %d %+v", total, lista)
	}
	if lista, _, _ = models.ListBloqueios(db, models.BloqueioFilter{Tipo: models.BloqueioTelefone}); len(lista) != 1 || lista[0].CriadoPorNome != "ana" || lista[0].Canal != "fila" {
		t.Fatalf("bloqueio de telefone listado: %+v", lista)
	}

	// Efeito por tipo: CNPJ barra a empresa; telefone e e-mail só aquele contato.
	conf, err := models.ConferirBloqueios(db, "12abc34501de35", []string{"(85) 99999-1234", "8532221234"}, []string{"ANA@exemplo.com.br", "livre@exemplo.com.br"})
	if err != nil || !conf.CNPJ || !conf.Telefones["85999991234"] || conf.Telefones["8532221234"] ||
		!conf.Emails["ana@exemplo.com.br"] || conf.Emails["livre@exemplo.com.br"] {
		t.Fatalf("conferência de bloqueios: %+v, %v", conf, err)
	}
	if conf.TodosTelefonesBloqueados([]string{"85999991234", "8532221234"}) || !conf.TodosTelefonesBloqueados([]string{"+5585999991234"}) ||
		conf.TodosTelefonesBloqueados(nil) {
		t.Fatal("TodosTelefonesBloqueados só é verdadeiro quando há telefones e todos estão bloqueados")
	}
	bloq, _ := models.CNPJsBloqueados(db, []string{cnpj, cnpjDeTeste(1)})
	if len(bloq) != 1 || !bloq["12ABC34501DE35"] {
		t.Fatalf("CNPJsBloqueados: %v", bloq)
	}

	removido, err := models.DeleteBloqueio(db, b.ID)
	if err != nil || removido.Valor != "85999991234" {
		t.Fatalf("remoção devolve o bloqueio para a auditoria: %+v, %v", removido, err)
	}
	if ainda, _ := models.Bloqueado(db, models.BloqueioTelefone, "85999991234"); ainda {
		t.Fatal("o telefone deveria ter saído da lista")
	}
	if _, err := models.DeleteBloqueio(db, b.ID); err != sql.ErrNoRows {
		t.Fatalf("remover de novo deveria dar ErrNoRows, veio %v", err)
	}
}

func TestConfigProspeccaoNoBanco(t *testing.T) {
	db := testDB(t)
	limparProspeccao(t, db)

	c := models.LoadConfigProspeccao(db)
	if c.JanelaInicio != 9 || c.MaxTentativas != 6 {
		t.Fatalf("sem nada gravado valem os padrões: %+v", c)
	}
	c.JanelaFim, c.Sabado, c.IncluirSemDonoNaFila = 20, true, true
	c.MotivosDescarte = []string{"Sem interesse", "Já é cliente"}
	if err := models.SaveConfigProspeccao(db, &c, nil); err != nil {
		t.Fatal(err)
	}
	lida := models.LoadConfigProspeccao(db)
	if lida.JanelaFim != 20 || !lida.Sabado || !lida.IncluirSemDonoNaFila || len(lida.MotivosDescarte) != 2 {
		t.Fatalf("configuração gravada: %+v", lida)
	}
	c.JanelaFim = 23
	if err := models.SaveConfigProspeccao(db, &c, nil); err == nil {
		t.Fatal("janela além do teto do sistema deveria ser recusada")
	}

	m := models.LoadMotivosPerda(db)
	m.Exigir, m.Motivos = false, []string{"Preço", "Prazo"}
	if err := models.SaveMotivosPerda(db, &m, nil); err != nil {
		t.Fatal(err)
	}
	if lidos := models.LoadMotivosPerda(db); lidos.Exigir || len(lidos.Motivos) != 2 || !lidos.Contem("Prazo") {
		t.Fatalf("motivos de perda gravados: %+v", lidos)
	}
}
