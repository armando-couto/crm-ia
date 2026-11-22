package services_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

// -----------------------------------------------------------------------------
// Envio ao CRM (job enviar_crm) contra Postgres de verdade.
// -----------------------------------------------------------------------------

func envioCNPJ(n int) string {
	base := fmt.Sprintf("%08d0001", 30000000+n)
	return base + models.CNPJDigitosVerificadores(base)
}

func sim() *bool { v := true; return &v }

func prepararEnvio(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE leads, prospeccao_jobs, prospeccao_itens, prospeccao_listas, contato_bloqueios,
		company_enrichments, companies, contacts, activities RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	cfg := models.DefaultConfigProspeccao()
	if err := models.SaveConfigProspeccao(db, &cfg, nil); err != nil {
		t.Fatal(err)
	}
	esperas := services.ProspeccaoEsperas
	services.ProspeccaoEsperas = []time.Duration{0, 0, 0}
	porFatia := services.EnvioItensPorFatia
	services.EnvioItensPorFatia = 2 // várias fatias mesmo com poucos itens
	t.Cleanup(func() {
		services.ProspeccaoEsperas = esperas
		services.EnvioItensPorFatia = porFatia
		db.Exec(`DELETE FROM app_settings WHERE key = $1`, models.SettingProspeccao)
	})
	return db
}

func envioUsuario(t *testing.T, db *sql.DB, email string, ativo bool) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
		INSERT INTO users (name, email, password_hash, role, active) VALUES ($1, $1, 'x', 'vendedor', $2)
		ON CONFLICT (email) DO UPDATE SET active = $2 RETURNING id`, email, ativo).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func envioLista(t *testing.T, db *sql.DB, nome string, empresas ...models.ProspEmpresa) (*models.ProspLista, []int64) {
	t.Helper()
	l := &models.ProspLista{Nome: nome, Status: models.ListaPronta, QuantidadePedida: len(empresas),
		Filtros: models.ProspFiltros{UFs: []string{"CE"}}}
	if err := models.CreateProspLista(db, l); err != nil {
		t.Fatal(err)
	}
	itens := []models.ProspItem{}
	for _, e := range empresas {
		e := e
		tel := ""
		if len(e.Telefones) > 0 {
			tel = e.Telefones[0].Numero
		}
		itens = append(itens, models.ProspItem{CNPJ: e.CNPJ, RazaoSocial: e.RazaoSocial, NomeFantasia: e.NomeFantasia,
			UF: e.Endereco.UF, Municipio: e.Endereco.Municipio, CnaeCodigo: e.CNAE.Codigo, CnaeDescricao: e.CNAE.Descricao,
			Porte: e.Porte, PessoaFisica: e.PessoaFisica, Nota: 50, TelefonePrincipal: tel, Dados: &e})
	}
	if _, err := models.InsertProspItens(db, l.ID, itens); err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	rows, err := db.Query(`SELECT id FROM prospeccao_itens WHERE lista_id = $1 ORDER BY id`, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	return l, ids
}

func envioEmpresa(n int, fantasia string, telefones ...string) models.ProspEmpresa {
	abertura := "2019-03-10"
	capital := 50000.0
	e := models.ProspEmpresa{
		CNPJ: envioCNPJ(n), Matriz: true, RazaoSocial: strings.ToUpper(fantasia) + " LTDA", NomeFantasia: fantasia,
		Natureza: models.ProspCodigoDescricao{Codigo: 2062, Descricao: "Sociedade Empresária Limitada"},
		Porte:    models.PorteME, Capital: &capital, Abertura: &abertura, Situacao: "ATIVA", Simples: true,
		CNAE: models.ProspCodigoDescricao{Codigo: 1091102, Descricao: "Padaria e confeitaria com predominância de revenda"},
		Endereco: models.ProspEndereco{TipoLogradouro: "RUA", Logradouro: "DAS FLORES", Numero: "100", Bairro: "CENTRO",
			CEP: "60000000", Municipio: "FORTALEZA", MunicipioCodigo: 1389, UF: "CE"},
		Socios: []models.ProspSocio{{Nome: "JOAO DA SILVA", Qualificacao: "Sócio-Administrador", Tipo: "pf", Documento: "***123456**"}},
	}
	for _, n := range telefones {
		tipo := models.TelefoneFixo
		if len(n) == 11 {
			tipo = models.TelefoneCelular
		}
		e.Telefones = append(e.Telefones, models.ProspTelefone{Numero: n, Tipo: tipo, Origem: models.OrigemContatoReceita})
	}
	return e
}

func enviar(t *testing.T, db *sql.DB, listaID int64, req services.EnvioRequest, criador int64) *models.ProspJob {
	t.Helper()
	if err := req.Normalizar(db); err != nil {
		t.Fatal(err)
	}
	job, err := services.EnfileirarEnvio(db, listaID, req, &criador)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.ProcessarJobsAte(context.Background(), db, 100); err != nil {
		t.Fatal(err)
	}
	job, err = models.ProspJobByID(db, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != models.JobConcluido {
		t.Fatalf("job %s: %s", job.Status, job.Mensagem)
	}
	return job
}

func resultadoEnvio(t *testing.T, j *models.ProspJob) services.ResultadoEnvio {
	t.Helper()
	var r services.ResultadoEnvio
	if err := json.Unmarshal(j.Resultado, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func itemEnvio(t *testing.T, db *sql.DB, id int64) *models.ProspItem {
	t.Helper()
	it, err := models.ProspItemByID(db, id)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestEnvioCriaEmpresaContatosELeadComBloqueiosEDuplicados(t *testing.T) {
	db := prepararEnvio(t)
	ana := envioUsuario(t, db, "ana.envio@exemplo.com.br", true)
	inativo := envioUsuario(t, db, "inativo.envio@exemplo.com.br", false)
	bia := envioUsuario(t, db, "bia.envio@exemplo.com.br", true)

	e1 := envioEmpresa(1, "Padaria Pão Bom", "8533334444")
	e1.Email = "contato@paobom.com.br"
	e2 := envioEmpresa(2, "Bloqueada pelo CNPJ", "8533335555")
	e3 := envioEmpresa(3, "Já No CRM", "8533336666")
	e4 := envioEmpresa(4, "Só Telefone Bloqueado", "8533337777")
	e5 := envioEmpresa(5, "Um Telefone Bloqueado", "8533338888", "85988887777")
	lista, ids := envioLista(t, db, "Padarias de Fortaleza", e1, e2, e3, e4, e5)

	// Item 1 enriquecido: decisora com celular e WhatsApp.
	wa := true
	perfil := &models.PerfilContato{Provedor: models.ProvedorWhoLead, Site: "https://www.paobom.com.br/",
		FuncionariosFaixa: "1 a 5",
		Pessoas: []models.PessoaContato{{Nome: "MARIA SOUZA", Papel: "socio", Cargo: "Sócia",
			Telefones: []models.ProspTelefone{{Numero: "85999990001", Tipo: models.TelefoneCelular, WhatsApp: &wa}},
			Emails:    []string{"maria@paobom.com.br"}}}}
	if err := models.UpdateProspItemEnriquecimento(db, ids[0], models.ProspItemEnriquecimento{
		Estado: models.ItemEnriquecido, Perfil: perfil, Provedor: models.ProvedorWhoLead, CustoCentavos: 80, Nota: 70}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []models.Bloqueio{
		{Tipo: models.BloqueioCNPJ, Valor: e2.CNPJ, Motivo: models.BloqueioOptOut},
		{Tipo: models.BloqueioTelefone, Valor: "8533337777", Motivo: models.BloqueioNumeroErrado},
		{Tipo: models.BloqueioTelefone, Valor: "8533338888", Motivo: models.BloqueioNumeroErrado},
	} {
		b := b
		if _, err := models.CreateBloqueio(db, &b); err != nil {
			t.Fatal(err)
		}
	}
	existente := &models.Company{Name: "Já No CRM", CNPJ: models.FormatarCNPJ(e3.CNPJ)}
	if err := models.CreateCompany(db, existente); err != nil {
		t.Fatal(err)
	}

	req := services.EnvioRequest{
		Selecao:      models.ProspSelecao{Todos: true},
		Duplicados:   services.DuplicadosPular,
		Distribuicao: services.DistribuicaoEnvio{Modo: services.DistribuicaoRodizio, Usuarios: []int64{ana, inativo, bia}},
		Temperatura:  "quente", Etiquetas: []string{"padarias", "padarias", " q4 "}, ContaAlvoTier: 2,
	}
	// O inativo não passa na validação; ele sai do rodízio quando é desativado depois.
	if err := req.Normalizar(db); err == nil {
		t.Fatal("rodízio com usuário inativo deveria ser recusado")
	}
	req.Distribuicao.Usuarios = []int64{ana, bia}

	// Prévia: 5 selecionados, 1 bloqueado por CNPJ, 1 já no CRM → 3 aptos.
	if err := req.Normalizar(db); err != nil {
		t.Fatal(err)
	}
	previa, err := services.PreviaEnvio(db, lista.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if previa.Selecionados != 5 || previa.Bloqueados != 1 || previa.DuplicadosNoCRM != 1 || previa.Aptos != 3 {
		t.Fatalf("prévia: %+v", previa)
	}
	if len(previa.PorUsuario) != 2 || previa.PorUsuario[0].Quantidade != 2 || previa.PorUsuario[1].Quantidade != 1 {
		t.Fatalf("prévia por usuário: %+v", previa.PorUsuario)
	}

	job := enviar(t, db, lista.ID, req, ana)
	res := resultadoEnvio(t, job)
	if res.Leads != 2 || res.Bloqueados != 2 || res.Duplicados != 1 || res.EmpresasCriadas != 2 || res.Contatos != 3 {
		t.Fatalf("resultado: %+v", res)
	}
	estados := []string{models.ItemEnviado, models.ItemBloqueado, models.ItemDuplicado, models.ItemBloqueado, models.ItemEnviado}
	for i, id := range ids {
		if it := itemEnvio(t, db, id); it.Estado != estados[i] {
			t.Fatalf("item %d: estado %q, esperado %q", i+1, it.Estado, estados[i])
		}
	}
	if it := itemEnvio(t, db, ids[2]); it.CompanyID == nil || *it.CompanyID != existente.ID || it.LeadID != nil {
		t.Fatalf("duplicado deveria guardar a empresa existente: %+v", it)
	}

	// Item 1: empresa com cadastro completo, inteligência e fontes; conta-alvo.
	it1 := itemEnvio(t, db, ids[0])
	c, err := models.CompanyByID(db, *it1.CompanyID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Padaria Pão Bom" || c.LegalName != "PADARIA PÃO BOM LTDA" || c.City != "Fortaleza" || c.State != "CE" ||
		c.CNAECode != "1091-1/02" || c.Website == "" || c.EmployeesRange != "1 a 5" || c.Email != "contato@paobom.com.br" ||
		c.OwnerID == nil || *c.OwnerID != ana || !c.IsTarget || c.TargetTier != 2 || c.EnrichedAt == nil || len(c.Intel) < 10 {
		t.Fatalf("empresa criada: %+v", c)
	}
	var consultas, custo int
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(custo_centavos),0) FROM company_enrichments WHERE company_id = $1`, c.ID).
		Scan(&consultas, &custo); err != nil || consultas != 2 || custo != 0 {
		t.Fatalf("consultas gravadas: %d, custo %d (%v)", consultas, custo, err)
	}
	var atividade int
	_ = db.QueryRow(`SELECT COUNT(*) FROM activities WHERE company_id = $1 AND kind = 'sistema' AND content LIKE 'Empresa criada pela lista%'`, c.ID).Scan(&atividade)
	if atividade != 1 {
		t.Fatalf("atividade de sistema: %d", atividade)
	}
	// Contatos: Maria (com telefone) primeiro, depois o sócio-administrador.
	var nomes []string
	rows, err := db.Query(`SELECT first_name || ' ' || last_name, buying_role FROM contacts WHERE company_id = $1 ORDER BY id`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n, papel string
		_ = rows.Scan(&n, &papel)
		if papel != "decisor" {
			t.Fatalf("papel do contato %s: %q", n, papel)
		}
		nomes = append(nomes, n)
	}
	rows.Close()
	if strings.Join(nomes, "|") != "Maria Souza|Joao da Silva" {
		t.Fatalf("contatos: %v", nomes)
	}

	l1, err := models.LeadByID(db, *it1.LeadID, models.VisaoTotal(ana))
	if err != nil {
		t.Fatal(err)
	}
	if l1.OwnerID == nil || *l1.OwnerID != ana || l1.ContactID == nil || l1.Origem != models.LeadOrigemProspeccao ||
		l1.Temperatura != "quente" || strings.Join(l1.Etiquetas, ",") != "padarias,q4" || l1.Nota != 70 ||
		l1.ProximaAcaoEm == nil || l1.ListaID == nil || *l1.ListaID != lista.ID {
		t.Fatalf("lead 1: %+v", l1)
	}
	if len(l1.Telefones) != 2 || l1.Telefones[0].Numero != "85999990001" || !l1.Telefones[0].Principal ||
		l1.Telefones[0].Pessoa != "MARIA SOUZA" {
		t.Fatalf("telefones do lead 1: %+v", l1.Telefones)
	}
	if len(l1.Resumo.Frases) == 0 || len(l1.Resumo.Decisores) == 0 || l1.Resumo.Decisores[0].Nome != "Maria Souza" ||
		l1.Resumo.Cadastro.Site == "" {
		t.Fatalf("resumo do lead 1: %+v", l1.Resumo)
	}

	// Item 5: o telefone bloqueado não entra no lead; o dono é a Bia (rodízio).
	it5 := itemEnvio(t, db, ids[4])
	l5, err := models.LeadByID(db, *it5.LeadID, models.VisaoTotal(ana))
	if err != nil {
		t.Fatal(err)
	}
	if l5.OwnerID == nil || *l5.OwnerID != bia || len(l5.Telefones) != 1 || l5.Telefones[0].Numero != "85988887777" {
		t.Fatalf("lead 5: dono %v, telefones %+v", l5.OwnerID, l5.Telefones)
	}
	if len(res.PorUsuario) != 2 {
		t.Fatalf("por usuário: %+v", res.PorUsuario)
	}

	// Um segundo envio da mesma seleção não acha nada apto.
	job2 := enviar(t, db, lista.ID, req, ana)
	if r := resultadoEnvio(t, job2); r.Leads != 0 || r.EmpresasCriadas != 0 {
		t.Fatalf("segundo envio: %+v", r)
	}
	var leads, empresas int
	_ = db.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&leads)
	_ = db.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&empresas)
	if leads != 2 || empresas != 3 {
		t.Fatalf("leads %d, empresas %d", leads, empresas)
	}
}

func TestEnvioRetomadaNaoDuplicaEmpresaNemLead(t *testing.T) {
	db := prepararEnvio(t)
	ana := envioUsuario(t, db, "ana.envio@exemplo.com.br", true)
	lista, ids := envioLista(t, db, "Retomada", envioEmpresa(11, "Empresa Criada Antes", "8533331111"),
		envioEmpresa(12, "Lead Criado Antes", "8533332222"))

	// Item 1: a empresa já tinha sido criada por este envio antes de um reinício.
	c := &models.Company{Name: "Empresa Criada Antes", CNPJ: envioCNPJ(11)}
	if err := models.CreateCompany(db, c); err != nil {
		t.Fatal(err)
	}
	if err := models.SetProspItemEmpresa(db, ids[0], c.ID); err != nil {
		t.Fatal(err)
	}
	// Item 2: o lead já tinha nascido, faltou fechar o item.
	c2 := &models.Company{Name: "Lead Criado Antes", CNPJ: envioCNPJ(12)}
	if err := models.CreateCompany(db, c2); err != nil {
		t.Fatal(err)
	}
	if err := models.SetProspItemEmpresa(db, ids[1], c2.ID); err != nil {
		t.Fatal(err)
	}
	item2 := ids[1]
	anterior := &models.Lead{Titulo: "Lead Criado Antes", CompanyID: &c2.ID, ItemID: &item2, ListaID: &lista.ID,
		Origem: models.LeadOrigemProspeccao}
	if _, err := models.CreateLead(db, anterior); err != nil {
		t.Fatal(err)
	}

	job := enviar(t, db, lista.ID, services.EnvioRequest{Selecao: models.ProspSelecao{Todos: true},
		Duplicados: services.DuplicadosPular, CriarContatos: sim(),
		Distribuicao: services.DistribuicaoEnvio{Modo: services.DistribuicaoUnico, Usuarios: []int64{ana}}}, ana)
	if r := resultadoEnvio(t, job); r.Leads != 2 || r.Duplicados != 0 {
		t.Fatalf("resultado: %+v", r)
	}
	var empresas, leads int
	_ = db.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&empresas)
	_ = db.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&leads)
	if empresas != 2 || leads != 2 {
		t.Fatalf("empresas %d, leads %d", empresas, leads)
	}
	it1, it2 := itemEnvio(t, db, ids[0]), itemEnvio(t, db, ids[1])
	if it1.Estado != models.ItemEnviado || *it1.CompanyID != c.ID || it2.Estado != models.ItemEnviado || *it2.LeadID != anterior.ID {
		t.Fatalf("itens: %+v / %+v", it1, it2)
	}
	// O sócio-administrador virou contato só uma vez, mesmo repetindo o passo.
	var contatos int
	_ = db.QueryRow(`SELECT COUNT(*) FROM contacts WHERE company_id = $1`, c.ID).Scan(&contatos)
	if contatos != 1 {
		t.Fatalf("contatos: %d", contatos)
	}
}

func TestEnvioVincularDuplicadosEDistribuicaoPorDia(t *testing.T) {
	db := prepararEnvio(t)
	ana := envioUsuario(t, db, "ana.envio@exemplo.com.br", true)
	comLead := envioEmpresa(21, "Já Tem Lead Aberto", "8533330001")
	semLead := envioEmpresa(22, "Empresa Sem Lead", "8533330002")
	nova1 := envioEmpresa(23, "Nova Um", "8533330003")
	nova2 := envioEmpresa(24, "Nova Dois", "8533330004")
	lista, ids := envioLista(t, db, "Vincular", comLead, semLead, nova1, nova2)

	c1 := &models.Company{Name: "Já Tem Lead Aberto", CNPJ: models.FormatarCNPJ(comLead.CNPJ)}
	c2 := &models.Company{Name: "Empresa Sem Lead", CNPJ: semLead.CNPJ}
	for _, c := range []*models.Company{c1, c2} {
		if err := models.CreateCompany(db, c); err != nil {
			t.Fatal(err)
		}
	}
	aberto := &models.Lead{Titulo: "Já Tem Lead Aberto", CompanyID: &c1.ID}
	if _, err := models.CreateLead(db, aberto); err != nil {
		t.Fatal(err)
	}

	no := false
	job := enviar(t, db, lista.ID, services.EnvioRequest{Selecao: models.ProspSelecao{Todos: true},
		Duplicados: services.DuplicadosVincular, CriarContatos: &no,
		Distribuicao: services.DistribuicaoEnvio{Modo: services.DistribuicaoUnico, Usuarios: []int64{ana}, PorDia: 2}}, ana)
	r := resultadoEnvio(t, job)
	if r.Leads != 3 || r.Duplicados != 1 || r.EmpresasExistentes != 1 || r.EmpresasCriadas != 2 || r.Contatos != 0 {
		t.Fatalf("resultado: %+v", r)
	}
	if it := itemEnvio(t, db, ids[0]); it.Estado != models.ItemDuplicado || it.LeadID == nil || *it.LeadID != aberto.ID {
		t.Fatalf("duplicado com lead aberto: %+v", it)
	}
	it2 := itemEnvio(t, db, ids[1])
	if it2.Estado != models.ItemEnviado || *it2.CompanyID != c2.ID {
		t.Fatalf("vinculado: %+v", it2)
	}
	// Dois por dia: o terceiro lead da Ana vai para o próximo dia útil, 9h.
	it4 := itemEnvio(t, db, ids[3])
	l4, err := models.LeadByID(db, *it4.LeadID, models.VisaoTotal(ana))
	if err != nil {
		t.Fatal(err)
	}
	p := l4.ProximaAcaoEm.In(time.Local)
	if !p.After(time.Now()) || p.Hour() != 9 || p.Minute() != 0 || !services.DiaUtil(p) {
		t.Fatalf("próxima ação do terceiro lead: %v", p)
	}
	l2, _ := models.LeadByID(db, *it2.LeadID, models.VisaoTotal(ana))
	if l2.ProximaAcaoEm.After(time.Now()) && services.DiaUtil(time.Now()) {
		t.Fatalf("o primeiro lead deveria estar para agora: %v", l2.ProximaAcaoEm)
	}
}

func TestProximaAcaoDistribuida(t *testing.T) {
	sexta := time.Date(2026, 11, 13, 10, 0, 0, 0, time.Local) // 20/11 é feriado (Consciência Negra)
	casos := []struct {
		inicio, agora time.Time
		indice        int
		esperado      time.Time
	}{
		{sexta, sexta, 0, sexta},
		{sexta, sexta, 1, sexta},
		{sexta, sexta, 2, time.Date(2026, 11, 16, 9, 0, 0, 0, time.Local)}, // segunda
		{sexta, sexta, 4, time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)},
		// Começou na quinta 19/11: a sexta 20/11 é feriado, o dia seguinte útil é segunda 23.
		{time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local), time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local), 2,
			time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)},
		// Véspera de Natal (quinta): o dia seguinte é feriado e depois vem o fim de semana.
		{time.Date(2026, 12, 24, 9, 0, 0, 0, time.Local), time.Date(2026, 12, 24, 9, 0, 0, 0, time.Local), 2,
			time.Date(2026, 12, 28, 9, 0, 0, 0, time.Local)},
		// Começou no sábado: o primeiro dia útil é segunda, no início da janela.
		{time.Date(2026, 11, 21, 15, 0, 0, 0, time.Local), time.Date(2026, 11, 21, 15, 0, 0, 0, time.Local), 0,
			time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)},
	}
	for i, c := range casos {
		if got := services.ProximaAcaoDistribuida(c.inicio, c.agora, c.indice, 2, 9); !got.Equal(c.esperado) {
			t.Errorf("caso %d: %v, esperado %v", i, got, c.esperado)
		}
	}
	if got := services.ProximaAcaoDistribuida(sexta, sexta, 50, 0, 9); !got.Equal(sexta) {
		t.Errorf("sem por_dia deveria ser agora: %v", got)
	}
}

func TestNormalizarPedidoDeEnvio(t *testing.T) {
	casos := []struct {
		nome string
		req  services.EnvioRequest
	}{
		{"sem seleção", services.EnvioRequest{}},
		{"duplicados inválido", services.EnvioRequest{Selecao: models.ProspSelecao{IDs: []int64{1}}, Duplicados: "apagar"}},
		{"modo inválido", services.EnvioRequest{Selecao: models.ProspSelecao{IDs: []int64{1}},
			Distribuicao: services.DistribuicaoEnvio{Modo: "sorteio"}}},
		{"rodízio vazio", services.EnvioRequest{Selecao: models.ProspSelecao{IDs: []int64{1}},
			Distribuicao: services.DistribuicaoEnvio{Modo: services.DistribuicaoRodizio}}},
		{"temperatura", services.EnvioRequest{Selecao: models.ProspSelecao{IDs: []int64{1}}, Temperatura: "morna"}},
		{"conta-alvo", services.EnvioRequest{Selecao: models.ProspSelecao{IDs: []int64{1}}, ContaAlvoTier: 4}},
		{"por dia", services.EnvioRequest{Selecao: models.ProspSelecao{IDs: []int64{1}},
			Distribuicao: services.DistribuicaoEnvio{PorDia: -1}}},
	}
	for _, c := range casos {
		err := c.req.Normalizar(nil)
		if _, ok := services.ComoErroEnvio(err); !ok {
			t.Errorf("%s: esperava erro de validação, veio %v", c.nome, err)
		}
	}
	ok := services.EnvioRequest{Selecao: models.ProspSelecao{Todos: true}}
	if err := ok.Normalizar(nil); err != nil {
		t.Fatal(err)
	}
	if ok.Duplicados != services.DuplicadosPular || ok.CriarContatos == nil || !*ok.CriarContatos ||
		ok.Distribuicao.Modo != services.DistribuicaoUnico {
		t.Fatalf("padrões: %+v", ok)
	}
}
