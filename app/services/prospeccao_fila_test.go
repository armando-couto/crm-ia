package services_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

// Terça-feira, 24/11/2026, 10h no fuso do ambiente: dia útil, janela aberta.
func filaAgora() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.Local) }

func TestCalcularCadencia(t *testing.T) {
	cfg := models.DefaultConfigProspeccao() // 9h–18h, max 6, [3 24 48 72 120], 2 por dia
	agora := filaAgora()

	c := services.CalcularCadencia(cfg, models.ResultadoNaoAtendeu, 0, 1, agora)
	if c.Descartar || c.Tentativas != 1 || !c.Proxima.Equal(agora.Add(3*time.Hour)) {
		t.Fatalf("1ª tentativa: %+v", c)
	}
	// Segunda do dia: o limite (2) foi atingido e 13h cairia hoje → amanhã 9h.
	c = services.CalcularCadencia(cfg, models.ResultadoOcupado, 1, 2, agora)
	amanha9 := time.Date(2026, 11, 25, 9, 0, 0, 0, time.Local)
	if c.Tentativas != 2 || !c.Proxima.Equal(amanha9) {
		t.Fatalf("limite diário: %+v", c)
	}
	// Ocupado com folga no dia: 1 h.
	c = services.CalcularCadencia(cfg, models.ResultadoOcupado, 0, 1, agora)
	if !c.Proxima.Equal(agora.Add(time.Hour)) {
		t.Fatalf("ocupado: %+v", c)
	}
	// Índice além do fim da lista usa o último intervalo (120 h): cai no
	// domingo e vai para segunda 9h.
	cfg.MaxTentativas = 10
	c = services.CalcularCadencia(cfg, models.ResultadoCaixaPostal, 7, 0, agora)
	if want := time.Date(2026, 11, 30, 9, 0, 0, 0, time.Local); c.Tentativas != 8 || !c.Proxima.Equal(want) {
		t.Fatalf("depois do fim da lista: %+v (esperado %v)", c, want)
	}
	// Fora da janela (17h + 3 h = 20h) vai para o início do próximo dia permitido.
	tarde := time.Date(2026, 11, 24, 17, 0, 0, 0, time.Local)
	c = services.CalcularCadencia(models.DefaultConfigProspeccao(), models.ResultadoNaoAtendeu, 0, 0, tarde)
	if !c.Proxima.Equal(amanha9) {
		t.Fatalf("fora da janela: %+v", c)
	}
	// Atingiu o máximo: descarta, inclusive com ocupado.
	c = services.CalcularCadencia(models.DefaultConfigProspeccao(), models.ResultadoOcupado, 5, 0, agora)
	if !c.Descartar || c.Proxima != nil {
		t.Fatalf("máximo de tentativas: %+v", c)
	}
	// Sexta 17h30 + 24 h cai no sábado: segunda 9h (lista vazia = 24 h).
	vazia := models.DefaultConfigProspeccao()
	vazia.IntervalosHoras = []int{}
	sexta := time.Date(2026, 11, 27, 17, 30, 0, 0, time.Local)
	c = services.CalcularCadencia(vazia, models.ResultadoNaoAtendeu, 0, 0, sexta)
	if want := time.Date(2026, 11, 30, 9, 0, 0, 0, time.Local); !c.Proxima.Equal(want) {
		t.Fatalf("fim de semana: %v, esperado %v", c.Proxima, want)
	}
}

// prepararFila limpa as tabelas da fila e grava a configuração padrão.
func prepararFila(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE leads, calls, meetings, deals, contato_bloqueios, companies, booking_pages
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	cfg := models.DefaultConfigProspeccao()
	if err := models.SaveConfigProspeccao(db, &cfg, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		services.AguardarSegundoPlano()
		db.Exec(`DELETE FROM app_settings WHERE key = $1`, models.SettingProspeccao)
	})
	return db
}

func filaUsuario(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
		INSERT INTO users (name, email, password_hash, role, active) VALUES ($1, $1, 'x', 'vendedor', TRUE)
		ON CONFLICT (email) DO UPDATE SET active = TRUE RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func filaLead(t *testing.T, db *sql.DB, dono *int64, telefones ...string) *models.Lead {
	t.Helper()
	var empresa int64
	if err := db.QueryRow(`INSERT INTO companies (name, cnpj) VALUES ('Padaria Pão Bom', '11222333000181') RETURNING id`).
		Scan(&empresa); err != nil {
		t.Fatal(err)
	}
	l := &models.Lead{Titulo: "Padaria Pão Bom", CompanyID: &empresa, CNPJ: "11222333000181", OwnerID: dono,
		Origem: models.LeadOrigemProspeccao}
	for i, n := range telefones {
		l.Telefones = append(l.Telefones, models.ProspTelefone{Numero: n, Tipo: models.TelefoneFixo, Principal: i == 0})
	}
	if _, err := models.CreateLead(db, l); err != nil {
		t.Fatal(err)
	}
	return l
}

func leadAgora(t *testing.T, db *sql.DB, id int64) *models.Lead {
	t.Helper()
	l, err := models.LeadByID(db, id, models.VisaoTotal(1))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestResultadoCadenciaIdempotenciaEZeragem(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	lead := filaLead(t, db, nil, "1133334444")
	v := models.LeadVisao{UserID: ana}
	agora := filaAgora()

	r, err := services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoNaoAtendeu, Chave: "chave-1", DuracaoSegundos: 15}, agora)
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.Estado != models.LeadTentando || r.Lead.Tentativas != 1 || r.Call.Outcome != "sem_resposta" ||
		r.Call.Numero != "1133334444" || r.ProximoPasso.Tipo != "ligar" {
		t.Fatalf("resposta: %+v / %+v / %+v", r.Lead, r.Call, r.ProximoPasso)
	}
	if r.Lead.OwnerID == nil || *r.Lead.OwnerID != ana {
		t.Fatalf("lead sem dono deveria passar a quem registrou: %+v", r.Lead.OwnerID)
	}
	if !r.Lead.ProximaAcaoEm.Equal(agora.Add(3 * time.Hour)) {
		t.Fatalf("próxima ação = %v", r.Lead.ProximaAcaoEm)
	}

	// Mesma chave: resposta original, nada gravado.
	r2, err := services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoNaoAtendeu, Chave: "chave-1"}, agora)
	if err != nil {
		t.Fatal(err)
	}
	var chamadas int
	db.QueryRow(`SELECT COUNT(*) FROM calls WHERE lead_id = $1`, lead.ID).Scan(&chamadas)
	if r2.Call.ID != r.Call.ID || chamadas != 1 || r2.Lead.Tentativas != 1 {
		t.Fatalf("repetição gravou de novo: call %d/%d, chamadas %d, tentativas %d", r2.Call.ID, r.Call.ID, chamadas, r2.Lead.Tentativas)
	}

	// Atendeu zera o contador e agenda o retorno padrão (48 h corridas, ajustadas).
	r, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{Resultado: models.ResultadoAtendeu}, agora)
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.Tentativas != 0 || !r.Lead.ProximaAcaoEm.Equal(agora.Add(48*time.Hour)) {
		t.Fatalf("atendeu: tentativas %d, próxima %v", r.Lead.Tentativas, r.Lead.ProximaAcaoEm)
	}

	// Retornar: obrigatório, passado é 422, fora da janela é deslocado.
	_, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{Resultado: models.ResultadoRetornar}, agora)
	var e422 *services.ErroResultado
	if !errors.As(err, &e422) {
		t.Fatalf("retornar sem data: %v", err)
	}
	_, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoRetornar, RetornarEm: agora.Add(-2 * time.Hour).Format(time.RFC3339)}, agora)
	if !errors.As(err, &e422) {
		t.Fatalf("retornar no passado: %v", err)
	}
	noite := time.Date(2026, 11, 24, 21, 0, 0, 0, time.Local)
	r, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoRetornar, RetornarEm: noite.Format(time.RFC3339)}, agora)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 25, 9, 0, 0, 0, time.Local); r.Lead.Estado != models.LeadRetornar ||
		!r.Lead.ProximaAcaoEm.Equal(want) || r.ProximoPasso.Tipo != "retornar" || !r.ProximoPasso.Em.Equal(want) {
		t.Fatalf("retornar fora da janela: %+v / %+v", r.Lead, r.ProximoPasso)
	}

	// Sem interesse exige motivo e descarta.
	_, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{Resultado: models.ResultadoSemInteresse}, agora)
	if !errors.As(err, &e422) {
		t.Fatalf("sem motivo: %v", err)
	}
	r, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoSemInteresse, Motivo: "Já tem fornecedor"}, agora)
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.Estado != models.LeadDescartado || r.Lead.MotivoDescarte != "Já tem fornecedor" || r.ProximoPasso.Tipo != "nenhum" {
		t.Fatalf("sem interesse: %+v", r.Lead)
	}
	// Lead encerrado: 409.
	_, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{Resultado: models.ResultadoAtendeu}, agora)
	if !errors.Is(err, models.ErrLeadEncerrado) {
		t.Fatalf("lead encerrado: %v", err)
	}
}

func TestResultadoMaxTentativasDescarta(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	cfg := models.DefaultConfigProspeccao()
	cfg.MaxTentativas = 2
	if err := models.SaveConfigProspeccao(db, &cfg, nil); err != nil {
		t.Fatal(err)
	}
	lead := filaLead(t, db, &ana, "1133334444")
	v := models.LeadVisao{UserID: ana}
	for i := 0; i < 2; i++ {
		r, err := services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{Resultado: models.ResultadoOcupado}, filaAgora())
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && (r.Lead.Estado != models.LeadDescartado || r.Lead.MotivoDescarte != models.DescarteSemContato) {
			t.Fatalf("deveria descartar por sem_contato: %+v", r.Lead)
		}
	}
}

func TestResultadoTravaDeOutroUsuario(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	bia := filaUsuario(t, db, "bia.fila@exemplo.com.br")
	lead := filaLead(t, db, &bia, "1133334444")
	if l, err := models.ProximoLeadDaFila(db, models.FilaOpcoes{UserID: bia}); err != nil || l == nil || l.ID != lead.ID {
		t.Fatalf("trava da bia: %v %v", l, err)
	}
	_, err := services.RegistrarResultado(db, lead.ID, models.VisaoTotal(ana),
		services.ResultadoPedido{Resultado: models.ResultadoAtendeu}, filaAgora())
	var em *models.ErroLeadEmAtendimento
	if !errors.As(err, &em) || em.UserID != bia {
		t.Fatalf("esperado 423, veio %v", err)
	}
	// Lead de outra pessoa fora da visão: 404.
	_, err = services.RegistrarResultado(db, lead.ID, models.LeadVisao{UserID: ana},
		services.ResultadoPedido{Resultado: models.ResultadoAtendeu}, filaAgora())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("fora da visão: %v", err)
	}
	// A dona da trava registra e a trava é solta.
	r, err := services.RegistrarResultado(db, lead.ID, models.LeadVisao{UserID: bia},
		services.ResultadoPedido{Resultado: models.ResultadoAtendeu}, filaAgora())
	if err != nil || r.Lead.EmAtendimentoPor != nil {
		t.Fatalf("resultado da dona: %v %+v", err, r)
	}
}

func TestResultadoNumeroErradoENaoLigarMais(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	v := models.LeadVisao{UserID: ana}
	lead := filaLead(t, db, &ana, "1133334444", "(11) 98888-7777")

	_, err := services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{Resultado: models.ResultadoNumeroErrado}, filaAgora())
	var e422 *services.ErroResultado
	if !errors.As(err, &e422) {
		t.Fatalf("numero_errado sem número: %v", err)
	}
	r, err := services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoNumeroErrado, Numero: "1133334444"}, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.Estado != models.LeadTentando || !r.Lead.Telefones[0].Invalido || r.Lead.Telefones[1].Invalido ||
		!r.Lead.ProximaAcaoEm.Equal(filaAgora()) {
		t.Fatalf("primeiro número errado: %+v", r.Lead)
	}
	if b, _ := models.Bloqueado(db, models.BloqueioTelefone, "1133334444"); !b {
		t.Fatal("o número errado deveria ficar bloqueado")
	}
	r, err = services.RegistrarResultado(db, lead.ID, v, services.ResultadoPedido{
		Resultado: models.ResultadoNumeroErrado, Numero: "11988887777"}, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.Estado != models.LeadDescartado || r.Lead.MotivoDescarte != models.DescarteSemTelefone {
		t.Fatalf("sem telefone válido deveria descartar: %+v", r.Lead)
	}

	outro := filaLead(t, db, &ana, "1144445555")
	r, err = services.RegistrarResultado(db, outro.ID, v, services.ResultadoPedido{Resultado: models.ResultadoNaoLigarMais}, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.Estado != models.LeadDescartado || r.Lead.MotivoDescarte != models.DescarteOptOut {
		t.Fatalf("opt-out: %+v", r.Lead)
	}
	var dnd bool
	db.QueryRow(`SELECT do_not_disturb FROM companies WHERE id = $1`, *outro.CompanyID).Scan(&dnd)
	cnpj, _ := models.Bloqueado(db, models.BloqueioCNPJ, "11222333000181")
	tel, _ := models.Bloqueado(db, models.BloqueioTelefone, "1144445555")
	if !dnd || !cnpj || !tel {
		t.Fatalf("opt-out: não perturbe %v, cnpj %v, telefone %v", dnd, cnpj, tel)
	}
}

func TestResultadoNaoEDecisorCriaContato(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	lead := filaLead(t, db, &ana, "1133334444")
	r, err := services.RegistrarResultado(db, lead.ID, models.LeadVisao{UserID: ana}, services.ResultadoPedido{
		Resultado: models.ResultadoNaoEDecisor,
		Contato:   &services.ContatoDecisor{Nome: "Carla Souza", Cargo: "Sócia", Telefone: "(11) 97777-6666"},
	}, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	if r.Lead.ContactID == nil || r.Lead.ContactName != "Carla Souza" || r.Lead.Telefones[0].Numero != "11977776666" ||
		!r.Lead.Telefones[0].Principal || r.Lead.Telefones[1].Principal || len(r.Lead.Telefones) != 2 {
		t.Fatalf("decisor: %+v", r.Lead)
	}
	if !r.Lead.ProximaAcaoEm.Equal(filaAgora().Add(24 * time.Hour)) {
		t.Fatalf("próxima ação = %v", r.Lead.ProximaAcaoEm)
	}
}

func TestResultadoReuniaoCriaNegocioEConfereAgenda(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	v := models.LeadVisao{UserID: ana}

	// Funil próprio do teste com uma etapa "Reunião marcada".
	var funil, etapaReuniao, etapaGanho int64
	db.QueryRow(`INSERT INTO pipelines (name, position) VALUES ('Prospecção teste', 99) RETURNING id`).Scan(&funil)
	db.Exec(`INSERT INTO pipeline_stages (pipeline_id, name, position) VALUES ($1, 'Qualificar', 0)`, funil)
	db.QueryRow(`INSERT INTO pipeline_stages (pipeline_id, name, position) VALUES ($1, 'Reunião marcada', 1) RETURNING id`, funil).Scan(&etapaReuniao)
	db.QueryRow(`INSERT INTO pipeline_stages (pipeline_id, name, position, is_won) VALUES ($1, 'Ganho', 2, TRUE) RETURNING id`, funil).Scan(&etapaGanho)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM deals WHERE pipeline_id = $1`, funil)
		db.Exec(`DELETE FROM pipelines WHERE id = $1`, funil)
	})

	inicio := time.Date(2026, 11, 26, 14, 0, 0, 0, time.Local)
	lead := filaLead(t, db, &ana, "1133334444")
	valor := 1500.0
	pedido := services.ResultadoPedido{Resultado: models.ResultadoReuniao, Chave: "reuniao-1", Reuniao: &services.ReuniaoPedido{
		Inicio: inicio.UTC().Format(time.RFC3339), PipelineID: &funil, StageID: &etapaReuniao, Valor: &valor}}

	// Etapa que não pertence ao funil informado: 422, sem gravar nada.
	outra := int64(999999)
	ruim := pedido
	ruim.Chave = ""
	ruim.Reuniao = &services.ReuniaoPedido{Inicio: pedido.Reuniao.Inicio, PipelineID: &outra, StageID: &etapaReuniao}
	var e422 *services.ErroConversao
	if _, err := services.RegistrarResultado(db, lead.ID, v, ruim, filaAgora()); !errors.As(err, &e422) {
		t.Fatalf("etapa de outro funil: %v", err)
	}

	r, err := services.RegistrarResultado(db, lead.ID, v, pedido, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	if r.Deal == nil || r.Meeting == nil || r.Lead.Estado != models.LeadConvertido {
		t.Fatalf("reunião: %+v", r)
	}
	if r.Deal.StageID != etapaReuniao || r.Deal.Origem != models.LeadOrigemProspeccao || r.Deal.LeadID == nil ||
		*r.Deal.LeadID != lead.ID || r.Deal.Amount != 1500 || r.Deal.OwnerID == nil || *r.Deal.OwnerID != ana {
		t.Fatalf("negócio: %+v", r.Deal)
	}
	if r.Meeting.DealID == nil || *r.Meeting.DealID != r.Deal.ID || !r.Meeting.StartsAt.Equal(inicio) ||
		!r.Meeting.EndsAt.Equal(inicio.Add(30*time.Minute)) {
		t.Fatalf("reunião: %+v", r.Meeting)
	}
	if r.Lead.DealID == nil || *r.Lead.DealID != r.Deal.ID || r.Lead.MeetingID == nil || r.ProximoPasso.Tipo != "reuniao" {
		t.Fatalf("lead convertido: %+v / %+v", r.Lead, r.ProximoPasso)
	}
	// Duplo clique: mesma chave devolve o mesmo negócio, sem criar outro.
	r2, err := services.RegistrarResultado(db, lead.ID, v, pedido, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	var negocios int
	db.QueryRow(`SELECT COUNT(*) FROM deals WHERE lead_id = $1`, lead.ID).Scan(&negocios)
	if r2.Deal == nil || r2.Deal.ID != r.Deal.ID || negocios != 1 {
		t.Fatalf("repetição criou outro negócio: %d", negocios)
	}

	// Outro lead no mesmo horário: agenda ocupada (409), e com forcar passa.
	outro := filaLead(t, db, &ana, "1155556666")
	p2 := services.ResultadoPedido{Resultado: models.ResultadoReuniao, Reuniao: &services.ReuniaoPedido{
		Inicio: inicio.Add(15 * time.Minute).Format(time.RFC3339), PipelineID: &funil}}
	if _, err := services.RegistrarResultado(db, outro.ID, v, p2, filaAgora()); !errors.Is(err, services.ErrAgendaOcupada) {
		t.Fatalf("agenda ocupada: %v", err)
	}
	if l := leadAgora(t, db, outro.ID); l.Estado != models.LeadNovo {
		t.Fatalf("o 409 não pode gravar nada: %+v", l)
	}
	p2.Reuniao.Forcar = true
	r, err = services.RegistrarResultado(db, outro.ID, v, p2, filaAgora())
	if err != nil {
		t.Fatal(err)
	}
	// Só o funil: primeira etapa aberta dele.
	if r.Deal.PipelineID != funil || r.Deal.StageID == etapaGanho || r.Deal.StageID == etapaReuniao {
		t.Fatalf("destino só com funil: %+v", r.Deal)
	}
}

func TestConverterLeadEmNegocio(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	lead := filaLead(t, db, nil, "1133334444")
	res, err := services.ConverterLeadEmNegocio(db, lead.ID, models.LeadVisao{UserID: ana}, services.ConverterPedido{Nome: "Padaria: maquininhas"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Lead.Estado != models.LeadConvertido || res.Deal.Name != "Padaria: maquininhas" || res.Deal.LeadID == nil ||
		res.Deal.Origem != models.LeadOrigemProspeccao || res.Lead.OwnerID == nil || *res.Lead.OwnerID != ana {
		t.Fatalf("conversão: %+v / %+v", res.Lead, res.Deal)
	}
	st, err := models.StageByID(db, res.Deal.StageID)
	if err != nil || st.IsWon || st.IsLost || st.PipelineID != res.Deal.PipelineID {
		t.Fatalf("destino padrão inválido: %+v %v", st, err)
	}
	if _, err := services.ConverterLeadEmNegocio(db, lead.ID, models.LeadVisao{UserID: ana}, services.ConverterPedido{}); !errors.Is(err, models.ErrLeadEncerrado) {
		t.Fatalf("converter de novo: %v", err)
	}
}

func TestDisponibilidadeAgenda(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	cfg := models.DefaultConfigProspeccao()
	agora := filaAgora() // terça 10h
	ocupada := time.Date(2026, 11, 24, 14, 0, 0, 0, time.Local)
	fim := ocupada.Add(time.Hour)
	if _, err := db.Exec(`INSERT INTO meetings (title, starts_at, ends_at, user_id) VALUES ('Visita', $1, $2, $3)`,
		ocupada, fim, ana); err != nil {
		t.Fatal(err)
	}
	d, err := services.DisponibilidadeAgenda(db, ana, 7, cfg, agora)
	if err != nil {
		t.Fatal(err)
	}
	if d.DuracaoMin != 30 || len(d.Dias) == 0 || d.Dias[0].Data != "2026-11-24" {
		t.Fatalf("disponibilidade: %+v", d)
	}
	hoje := d.Dias[0]
	rotulos := map[string]bool{}
	for _, h := range hoje.Horarios {
		rotulos[h.Rotulo] = true
	}
	if rotulos["09:30"] || !rotulos["10:00"] || rotulos["14:00"] || rotulos["14:30"] || !rotulos["15:00"] || !rotulos["17:30"] || rotulos["18:00"] {
		t.Fatalf("horários de hoje: %+v", hoje.Horarios)
	}
	if len(hoje.Ocupados) != 1 || hoje.Ocupados[0].Titulo != "Visita" {
		t.Fatalf("ocupados: %+v", hoje.Ocupados)
	}
	for _, dia := range d.Dias {
		if dia.Data == "2026-11-28" || dia.Data == "2026-11-29" {
			t.Fatalf("fim de semana não é expediente: %+v", dia)
		}
	}
	// Com página de agendamento: a duração dela vale.
	page := &models.BookingPage{UserID: ana, Slug: "ana-fila", Title: "Conversa", DurationMin: 45,
		DaysAhead: 14, WeeklyHours: models.DefaultWeeklyHours(), Active: true}
	if err := models.SaveBookingPage(db, page); err != nil {
		t.Fatal(err)
	}
	d, err = services.DisponibilidadeAgenda(db, ana, 2, cfg, agora)
	if err != nil {
		t.Fatal(err)
	}
	if d.DuracaoMin != 45 || len(d.Dias) != 2 {
		t.Fatalf("com página: %+v", d)
	}
	if services.DuracaoReuniaoDe(db, ana) != 45 {
		t.Fatal("duração padrão da reunião deveria vir da página")
	}
}

func TestProximoLeadForaDaJanela(t *testing.T) {
	db := prepararFila(t)
	ana := filaUsuario(t, db, "ana.fila@exemplo.com.br")
	lead := filaLead(t, db, &ana, "1133334444")
	domingo := time.Date(2026, 11, 29, 10, 0, 0, 0, time.Local)
	r, err := services.ProximoLead(db, ana, "Ana", 0, nil, domingo)
	if err != nil || r.Lead != nil || r.MotivoVazio != "fora_da_janela" || r.Janela.ReabreEm == nil {
		t.Fatalf("fora da janela: %+v %v", r, err)
	}
	r, err = services.ProximoLead(db, ana, "Ana", 0, nil, filaAgora())
	if err != nil || r.Lead == nil || r.Lead.Lead.ID != lead.ID || r.MotivoVazio != "" {
		t.Fatalf("próximo: %+v %v", r, err)
	}
	r, err = services.ProximoLead(db, ana, "Ana", 0, []int64{lead.ID}, filaAgora())
	if err != nil || r.Lead != nil || r.MotivoVazio != "fila_vazia" {
		t.Fatalf("pulado: %+v %v", r, err)
	}
	resumo, err := services.ResumoDaFila(db, ana, 0, filaAgora())
	if err != nil || resumo.NaFila != 1 || resumo.MetaDia != 40 {
		t.Fatalf("resumo: %+v %v", resumo, err)
	}
}
