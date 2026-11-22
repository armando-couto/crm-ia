package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Fila de ligações: resultado da ligação em um clique (com a cadência e os
// efeitos de cada resultado), próximo lead e resumo do dia. Contrato:
// docs/PROSPECCAO.md seção 8.2 com as correções de docs/PROSPECCAO-ADENDO.md.

// ErroResultado é um ResultadoRequest inválido (422 com a mensagem).
type ErroResultado struct{ Msg string }

func (e *ErroResultado) Error() string { return e.Msg }

func erroResultado(format string, a ...any) error {
	return &ErroResultado{Msg: fmt.Sprintf(format, a...)}
}

// ContatoDecisor é quem decide, informado em "nao_e_decisor".
type ContatoDecisor struct {
	Nome     string `json:"nome"`
	Cargo    string `json:"cargo"`
	Telefone string `json:"telefone"`
	Email    string `json:"email"`
}

// ReuniaoPedido é a reunião marcada em "reuniao".
type ReuniaoPedido struct {
	Titulo     string   `json:"titulo"`
	Inicio     string   `json:"inicio"` // RFC 3339 (o "inicio" devolvido pela disponibilidade)
	DuracaoMin int      `json:"duracao_min"`
	Local      string   `json:"local"`
	Notas      string   `json:"notas"`
	UserID     *int64   `json:"user_id"`
	PipelineID *int64   `json:"pipeline_id"`
	StageID    *int64   `json:"stage_id"`
	Valor      *float64 `json:"valor"`
	Forcar     bool     `json:"forcar"`
}

// ResultadoPedido é o ResultadoRequest.
type ResultadoPedido struct {
	Resultado       string          `json:"resultado"`
	Chave           string          `json:"chave"`
	Numero          string          `json:"numero"`
	DuracaoSegundos int             `json:"duracao_segundos"`
	Notas           string          `json:"notas"`
	RetornarEm      string          `json:"retornar_em"`
	Motivo          string          `json:"motivo"`
	Contato         *ContatoDecisor `json:"contato"`
	Reuniao         *ReuniaoPedido  `json:"reuniao"`
	WhatsAppOptin   *bool           `json:"whatsapp_optin"`
	Temperatura     *string         `json:"temperatura"`
}

// ProximoPasso diz ao vendedor o que acontece com o lead depois do resultado.
type ProximoPasso struct {
	Tipo  string     `json:"tipo"` // ligar | retornar | reuniao | nenhum
	Em    *time.Time `json:"em"`
	Texto string     `json:"texto"`
}

// ResultadoResposta é a resposta de POST /leads/{id}/resultado.
type ResultadoResposta struct {
	Lead         *models.Lead    `json:"lead"`
	Call         *models.Call    `json:"call"`
	Meeting      *models.Meeting `json:"meeting,omitempty"`
	Deal         *models.Deal    `json:"deal,omitempty"`
	ProximoPasso ProximoPasso    `json:"proximo_passo"`
}

// pedidoValidado é o ResultadoPedido já conferido e com as datas lidas.
type pedidoValidado struct {
	ResultadoPedido
	retornarEm *time.Time
	inicio     time.Time
	duracao    int
}

const maxNotasResultado = 5000

func validarPedido(p ResultadoPedido, cfg models.ConfigProspeccao, agora time.Time) (*pedidoValidado, error) {
	p.Resultado = strings.TrimSpace(p.Resultado)
	if !models.ResultadoValido(p.Resultado) {
		return nil, erroResultado("resultado inválido")
	}
	if len(p.Chave) > 40 {
		return nil, erroResultado("chave de idempotência longa demais (até 40 caracteres)")
	}
	if p.DuracaoSegundos < 0 || p.DuracaoSegundos > 24*3600 {
		return nil, erroResultado("duração da ligação inválida")
	}
	p.Notas = strings.TrimSpace(p.Notas)
	if len([]rune(p.Notas)) > maxNotasResultado {
		return nil, erroResultado("as notas aceitam até %d caracteres", maxNotasResultado)
	}
	if p.Temperatura != nil && !models.LeadTemperaturaValida(*p.Temperatura) {
		return nil, erroResultado("temperatura inválida")
	}
	v := &pedidoValidado{ResultadoPedido: p}
	if s := strings.TrimSpace(p.RetornarEm); s != "" && (p.Resultado == models.ResultadoRetornar || p.Resultado == models.ResultadoAtendeu) {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, erroResultado("retornar_em inválido: use data e hora com fuso (RFC 3339)")
		}
		if t.Before(agora.Add(-time.Minute)) {
			return nil, erroResultado("a data de retorno já passou")
		}
		v.retornarEm = &t
	}
	switch p.Resultado {
	case models.ResultadoRetornar:
		if v.retornarEm == nil {
			return nil, erroResultado("informe quando retornar")
		}
	case models.ResultadoSemInteresse:
		// Obrigatório; texto fora da lista configurada é aceito (adendo, lente telas).
		v.Motivo = strings.Join(strings.Fields(p.Motivo), " ")
		if v.Motivo == "" {
			return nil, erroResultado("informe o motivo do descarte")
		}
	case models.ResultadoNumeroErrado:
		if models.NormalizarTelefone(p.Numero) == "" {
			return nil, erroResultado("informe o número errado")
		}
	case models.ResultadoNaoEDecisor:
		if p.Contato != nil {
			p.Contato.Nome = strings.Join(strings.Fields(p.Contato.Nome), " ")
			if p.Contato.Nome == "" {
				return nil, erroResultado("informe o nome do decisor")
			}
		}
	case models.ResultadoReuniao:
		r := p.Reuniao
		if r == nil || strings.TrimSpace(r.Inicio) == "" {
			return nil, erroResultado("informe o início da reunião")
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.Inicio))
		if err != nil {
			return nil, erroResultado("início da reunião inválido: use data e hora com fuso (RFC 3339)")
		}
		v.inicio = t.In(time.Local)
		if r.DuracaoMin != 0 && (r.DuracaoMin < DuracaoReuniaoMin || r.DuracaoMin > DuracaoReuniaoMax) {
			return nil, erroResultado("a duração da reunião deve ficar entre %d e %d minutos", DuracaoReuniaoMin, DuracaoReuniaoMax)
		}
		if r.Valor != nil && *r.Valor < 0 {
			return nil, erroResultado("o valor do negócio não pode ser negativo")
		}
	}
	return v, nil
}

// RegistrarResultado grava o resultado da ligação numa transação: a chamada
// (com a chave de idempotência), a atividade "ligacao", o estado do lead e os
// efeitos do resultado (bloqueios, contato decisor, reunião e negócio).
//
// Erros: *ErroResultado e *ErroConversao (422), sql.ErrNoRows (404),
// models.ErrLeadEncerrado (409 "estado"), *models.ErroLeadEmAtendimento (423),
// ErrAgendaOcupada (409 "agenda_ocupada"). A mesma chave repetida devolve a
// resposta da primeira gravação sem gravar nada.
func RegistrarResultado(db *sql.DB, id int64, v models.LeadVisao, p ResultadoPedido, agora time.Time) (*ResultadoResposta, error) {
	cfg := models.LoadConfigProspeccao(db)
	p.Chave = strings.TrimSpace(p.Chave)
	if p.Chave != "" {
		if r, ok, err := respostaRepetida(db, id, v, p.Chave); err != nil || ok {
			return r, err
		}
	}
	pv, err := validarPedido(p, cfg, agora)
	if err != nil {
		return nil, err
	}
	// Dados que se leem fora da transação (o banco de teste tem uma conexão só).
	if pv.Resultado == models.ResultadoReuniao && pv.duracao == 0 {
		pv.duracao = pv.Reuniao.DuracaoMin
		if pv.duracao == 0 {
			dono := v.UserID
			if pv.Reuniao.UserID != nil && *pv.Reuniao.UserID > 0 {
				dono = *pv.Reuniao.UserID
			}
			pv.duracao = DuracaoReuniaoDe(db, dono)
		}
	}

	res, efeitos, err := gravarResultado(db, id, v, pv, cfg, agora)
	if err != nil {
		if p.Chave != "" && (errors.Is(err, models.ErrChamadaRepetida) || errors.Is(err, models.ErrLeadEncerrado)) {
			// Corrida com o mesmo pedido: a outra gravação venceu.
			if r, ok, err2 := respostaRepetida(db, id, v, p.Chave); err2 == nil && ok {
				return r, nil
			}
		}
		return nil, err
	}
	efeitos.aplicar(db, v.UserID)

	if res.Lead, err = models.LeadByID(db, id, models.VisaoTotal(v.UserID)); err != nil {
		return nil, err
	}
	return res, nil
}

// efeitosDepois é o que roda depois do commit (fora da transação).
type efeitosDepois struct {
	contatoReuniao *int64
	deal           *models.Deal
}

func (e efeitosDepois) aplicar(db *sql.DB, autor int64) {
	if e.contatoReuniao != nil {
		SequenceExitOnMeeting(db, *e.contatoReuniao)
	}
	if e.deal != nil {
		depoisDeCriarNegocio(db, e.deal, autor)
	}
}

// respostaRepetida remonta a resposta de um resultado já gravado com a chave.
func respostaRepetida(db *sql.DB, id int64, v models.LeadVisao, chave string) (*ResultadoResposta, bool, error) {
	call, err := models.CallByChave(db, chave)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if call.LeadID == nil || *call.LeadID != id {
		return nil, false, erroResultado("esta chave já foi usada em outra ligação")
	}
	lead, err := models.LeadByID(db, id, models.VisaoTotal(v.UserID))
	if err != nil {
		return nil, false, err
	}
	r := &ResultadoResposta{Lead: lead, Call: call}
	if call.Desfecho == models.ResultadoReuniao {
		if lead.MeetingID != nil {
			if m, err := models.MeetingByID(db, *lead.MeetingID); err == nil {
				r.Meeting = m
			}
		}
		if lead.DealID != nil {
			if d, err := models.DealByID(db, *lead.DealID); err == nil {
				r.Deal = d
			}
		}
	}
	r.ProximoPasso = passoDoLead(lead, r.Meeting, time.Now())
	return r, true, nil
}

// passoDoLead descreve o próximo passo a partir do estado do lead.
func passoDoLead(l *models.Lead, m *models.Meeting, agora time.Time) ProximoPasso {
	switch l.Estado {
	case models.LeadConvertido:
		if m != nil {
			em := m.StartsAt.In(time.Local)
			return ProximoPasso{Tipo: "reuniao", Em: &em, Texto: "Reunião marcada para " + TextoQuando(em, agora)}
		}
		return ProximoPasso{Tipo: "nenhum", Texto: "Lead convertido em negócio"}
	case models.LeadDescartado:
		return ProximoPasso{Tipo: "nenhum", Texto: "Lead descartado: " + rotuloDescarte(l.MotivoDescarte)}
	}
	if l.ProximaAcaoEm == nil {
		return ProximoPasso{Tipo: "nenhum", Texto: "Sem próxima ação agendada"}
	}
	em := l.ProximaAcaoEm.In(time.Local)
	if l.Estado == models.LeadRetornar {
		return ProximoPasso{Tipo: "retornar", Em: &em, Texto: "Retornar " + TextoQuando(em, agora)}
	}
	if !em.After(agora) {
		return ProximoPasso{Tipo: "ligar", Em: &em, Texto: "Volta para a fila agora"}
	}
	return ProximoPasso{Tipo: "ligar", Em: &em, Texto: "Nova tentativa " + TextoQuando(em, agora)}
}

func rotuloDescarte(m string) string {
	switch m {
	case models.DescarteSemTelefone:
		return "sem telefone válido"
	case models.DescarteSemContato:
		return "sem contato após as tentativas"
	case models.DescarteOptOut:
		return "pediu para não ligar"
	case models.DescarteManual, "":
		return "descartado manualmente"
	}
	return m
}

// gravarResultado é a transação do resultado.
func gravarResultado(db *sql.DB, id int64, v models.LeadVisao, p *pedidoValidado, cfg models.ConfigProspeccao,
	agora time.Time) (*ResultadoResposta, efeitosDepois, error) {
	var depois efeitosDepois
	tx, err := db.Begin()
	if err != nil {
		return nil, depois, err
	}
	defer tx.Rollback()

	lead, err := models.PrepararResultadoLead(tx, id, v)
	if err != nil {
		return nil, depois, err
	}
	eu := v.UserID
	patch := models.LeadPatch{UltimoResultado: &p.Resultado, UltimoContatoEm: &agora}
	if lead.SemDono() {
		patch.Owner = models.DefinirID(eu)
	}
	if p.WhatsAppOptin != nil {
		patch.WhatsAppOptin = p.WhatsAppOptin
	}
	if p.Temperatura != nil {
		patch.Temperatura = p.Temperatura
	}

	numero := models.NormalizarTelefone(p.Numero)
	if numero == "" {
		numero = numeroPrincipal(lead.Telefones)
	}
	if p.Resultado == models.ResultadoNumeroErrado && !temTelefone(lead.Telefones, numero) {
		return nil, depois, erroResultado("o número informado não está entre os telefones do lead")
	}

	call := &models.Call{
		Direction: "saida",
		Outcome:   models.OutcomeDoResultado(p.Resultado),
		Duration:  p.DuracaoSegundos,
		Notes:     p.Notas,
		CalledAt:  agora,
		ContactID: lead.ContactID,
		CompanyID: lead.CompanyID,
		UserID:    &eu,
		LeadID:    &lead.ID,
		Desfecho:  p.Resultado,
		Numero:    numero,
		Chave:     p.Chave,
	}
	if err := models.CreateCallTx(tx, call); err != nil {
		return nil, depois, err
	}
	res := &ResultadoResposta{Call: call}

	switch {
	case models.ResultadoContaTentativa(p.Resultado):
		hoje, err := models.TentativasSemContatoDesde(tx, id, InicioDoDia(agora))
		if err != nil {
			return nil, depois, err
		}
		c := CalcularCadencia(cfg, p.Resultado, lead.Tentativas, hoje, agora)
		patch.Tentativas = &c.Tentativas
		if c.Descartar {
			descartar(&patch, models.DescarteSemContato)
		} else {
			estado := models.LeadTentando
			patch.Estado = &estado
			patch.ProximaAcaoEm = models.DefinirTempo(*c.Proxima)
		}

	case p.Resultado == models.ResultadoAtendeu:
		zerarTentativas(&patch)
		quando := agora.Add(time.Duration(cfg.RetornoPadraoHoras) * time.Hour)
		if p.retornarEm != nil {
			quando = *p.retornarEm
		}
		estado := models.LeadTentando
		patch.Estado = &estado
		patch.ProximaAcaoEm = models.DefinirTempo(ProximaAcaoNaJanela(cfg, quando, -1, agora))

	case p.Resultado == models.ResultadoRetornar:
		zerarTentativas(&patch)
		estado := models.LeadRetornar
		patch.Estado = &estado
		patch.ProximaAcaoEm = models.DefinirTempo(ProximaAcaoNaJanela(cfg, *p.retornarEm, -1, agora))

	case p.Resultado == models.ResultadoSemInteresse:
		zerarTentativas(&patch)
		descartar(&patch, p.Motivo)

	case p.Resultado == models.ResultadoNaoEDecisor:
		zerarTentativas(&patch)
		estado := models.LeadTentando
		patch.Estado = &estado
		patch.ProximaAcaoEm = models.DefinirTempo(ProximaAcaoNaJanela(cfg, agora.Add(EsperaNaoEDecisor), -1, agora))
		if p.Contato != nil {
			if err := gravarDecisor(tx, lead, p.Contato, eu, &patch); err != nil {
				return nil, depois, err
			}
		}

	case p.Resultado == models.ResultadoNumeroErrado:
		telefones := append([]models.ProspTelefone(nil), lead.Telefones...)
		for i := range telefones {
			if models.NormalizarTelefone(telefones[i].Numero) == numero {
				telefones[i].Invalido = true
			}
		}
		patch.Telefones = &telefones
		if _, err := models.CreateBloqueio(tx, &models.Bloqueio{
			Tipo: models.BloqueioTelefone, Valor: numero, Motivo: models.BloqueioNumeroErrado,
			Canal: "fila", CriadoPor: &eu,
		}); err != nil {
			return nil, depois, err
		}
		validos, err := telefonesValidos(tx, telefones)
		if err != nil {
			return nil, depois, err
		}
		if validos == 0 {
			descartar(&patch, models.DescarteSemTelefone)
		} else {
			estado := models.LeadTentando
			patch.Estado = &estado
			patch.ProximaAcaoEm = models.DefinirTempo(ProximaAcaoNaJanela(cfg, agora, -1, agora))
		}

	case p.Resultado == models.ResultadoNaoLigarMais:
		zerarTentativas(&patch)
		descartar(&patch, models.DescarteOptOut)
		if err := bloquearOptOut(tx, lead, eu); err != nil {
			return nil, depois, err
		}

	case p.Resultado == models.ResultadoReuniao:
		zerarTentativas(&patch)
	}

	if err := models.UpdateLead(tx, id, patch); err != nil {
		return nil, depois, err
	}
	if err := registrarAtividade(tx, &models.Activity{
		Kind:      models.ActivityLigacao,
		Content:   textoLigacao(p, numero),
		UserID:    &eu,
		ContactID: lead.ContactID,
		CompanyID: lead.CompanyID,
	}); err != nil {
		return nil, depois, err
	}

	if p.Resultado == models.ResultadoReuniao {
		if err := marcarReuniao(tx, lead, p, cfg, eu, res, &depois); err != nil {
			return nil, depois, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, depois, err
	}

	// Próximo passo: lê o estado final (fora da transação já encerrada).
	final := *lead
	if patch.Estado != nil {
		final.Estado = *patch.Estado
	}
	if p.Resultado == models.ResultadoReuniao {
		final.Estado = models.LeadConvertido
	}
	if patch.MotivoDescarte != nil {
		final.MotivoDescarte = *patch.MotivoDescarte
	}
	final.ProximaAcaoEm = nil
	if patch.ProximaAcaoEm.Definir {
		final.ProximaAcaoEm = patch.ProximaAcaoEm.Em
	}
	res.ProximoPasso = passoDoLead(&final, res.Meeting, agora)
	return res, depois, nil
}

func zerarTentativas(p *models.LeadPatch) {
	zero := 0
	p.Tentativas = &zero
}

func descartar(p *models.LeadPatch, motivo string) {
	estado := models.LeadDescartado
	p.Estado = &estado
	p.MotivoDescarte = &motivo
	p.ProximaAcaoEm = models.LimparTempo()
}

func numeroPrincipal(tel []models.ProspTelefone) string {
	for _, t := range tel {
		if t.Principal && !t.Invalido {
			return models.NormalizarTelefone(t.Numero)
		}
	}
	for _, t := range tel {
		if !t.Invalido {
			return models.NormalizarTelefone(t.Numero)
		}
	}
	return ""
}

func temTelefone(tel []models.ProspTelefone, numero string) bool {
	for _, t := range tel {
		if numero != "" && models.NormalizarTelefone(t.Numero) == numero {
			return true
		}
	}
	return false
}

// telefonesValidos conta os telefones que não estão marcados inválidos nem
// bloqueados.
func telefonesValidos(db models.Executor, tel []models.ProspTelefone) (int, error) {
	var nums []string
	for _, t := range tel {
		if n := models.NormalizarTelefone(t.Numero); n != "" && !t.Invalido {
			nums = append(nums, n)
		}
	}
	if len(nums) == 0 {
		return 0, nil
	}
	bloq, err := models.ValoresBloqueados(db, models.BloqueioTelefone, nums)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, x := range nums {
		if !bloq[x] {
			n++
		}
	}
	return n, nil
}

// bloquearOptOut: "não ligar mais" bloqueia o CNPJ e os telefones do lead e
// marca a empresa como "não perturbe".
func bloquearOptOut(tx *sql.Tx, lead *models.Lead, autor int64) error {
	novo := func(tipo, valor string) error {
		_, err := models.CreateBloqueio(tx, &models.Bloqueio{
			Tipo: tipo, Valor: valor, Motivo: models.BloqueioOptOut, Canal: "fila", CriadoPor: &autor,
		})
		return err
	}
	if lead.CNPJ != "" {
		if err := novo(models.BloqueioCNPJ, lead.CNPJ); err != nil {
			return err
		}
	}
	for _, t := range lead.Telefones {
		if models.NormalizarTelefone(t.Numero) == "" {
			continue
		}
		if err := novo(models.BloqueioTelefone, t.Numero); err != nil {
			return err
		}
	}
	if lead.CompanyID != nil {
		if _, err := tx.Exec(`UPDATE companies SET do_not_disturb = TRUE, updated_at = NOW() WHERE id = $1`, *lead.CompanyID); err != nil {
			return err
		}
	}
	return nil
}

// gravarDecisor cria o contato decisor (na empresa do lead), faz dele o
// contato principal do lead e põe o telefone dele no topo da lista.
func gravarDecisor(tx *sql.Tx, lead *models.Lead, c *ContatoDecisor, autor int64, patch *models.LeadPatch) error {
	nome, sobrenome := c.Nome, ""
	if i := strings.IndexByte(nome, ' '); i > 0 {
		nome, sobrenome = nome[:i], strings.TrimSpace(nome[i+1:])
	}
	telefone := models.NormalizarTelefone(c.Telefone)
	var contatoID int64
	err := tx.QueryRow(`
		INSERT INTO contacts (first_name, last_name, email, phone, job_title, lifecycle_stage, source, company_id, owner_id, buying_role)
		VALUES ($1, $2, $3, $4, $5, 'lead', 'prospeccao', $6, $7, 'decisor')
		RETURNING id`,
		filaCortar(nome, 100), filaCortar(sobrenome, 100), models.NormalizeEmail(c.Email), telefone,
		filaCortar(strings.TrimSpace(c.Cargo), 100), lead.CompanyID, autor).Scan(&contatoID)
	if err != nil {
		return err
	}
	patch.Contact = models.DefinirID(contatoID)
	if telefone != "" {
		tel := []models.ProspTelefone{{
			Numero: telefone, Tipo: tipoTelefone(telefone), Origem: models.OrigemContatoManual,
			Pessoa: c.Nome, Principal: true,
		}}
		for _, t := range lead.Telefones {
			if models.NormalizarTelefone(t.Numero) == telefone {
				continue
			}
			t.Principal = false
			tel = append(tel, t)
		}
		patch.Telefones = &tel
	}
	return nil
}

func tipoTelefone(n string) string {
	switch len(n) {
	case 11:
		return models.TelefoneCelular
	case 10:
		return models.TelefoneFixo
	}
	return models.TelefoneIndefinido
}

func filaCortar(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

var rotulosResultado = map[string]string{
	models.ResultadoAtendeu:      "atendeu",
	models.ResultadoReuniao:      "reunião marcada",
	models.ResultadoRetornar:     "pediu retorno",
	models.ResultadoSemInteresse: "sem interesse",
	models.ResultadoNaoEDecisor:  "não é o decisor",
	models.ResultadoNaoAtendeu:   "não atendeu",
	models.ResultadoCaixaPostal:  "caixa postal",
	models.ResultadoOcupado:      "ocupado",
	models.ResultadoNumeroErrado: "número errado",
	models.ResultadoNaoLigarMais: "pediu para não ligar mais",
}

func textoLigacao(p *pedidoValidado, numero string) string {
	t := "Ligação: " + rotulosResultado[p.Resultado]
	if numero != "" {
		t += " (" + numero + ")"
	}
	if p.Resultado == models.ResultadoSemInteresse && p.Motivo != "" {
		t += " · motivo: " + p.Motivo
	}
	if p.Notas != "" {
		t += "\n" + p.Notas
	}
	return t
}

// marcarReuniao segue a ordem do adendo: destino, agenda, negócio, reunião
// com deal_id, atividade "reuniao" e lead convertido. A saída das sequências
// fica para depois do commit (efeitos).
func marcarReuniao(tx *sql.Tx, lead *models.Lead, p *pedidoValidado, cfg models.ConfigProspeccao, eu int64,
	res *ResultadoResposta, depois *efeitosDepois) error {
	r := p.Reuniao
	dono := eu
	if r.UserID != nil && *r.UserID > 0 {
		dono = *r.UserID
	}
	if _, ativo, err := usuarioAtivo(tx, dono); err != nil {
		return err
	} else if !ativo {
		return &ErroConversao{"o responsável pela reunião não existe ou está inativo"}
	}
	destino, err := ResolverDestino(tx, r.PipelineID, r.StageID, lead, cfg)
	if err != nil {
		return err
	}
	fim := fimDaReuniao(p.inicio, p.duracao)
	ocupado, err := AgendaOcupada(tx, dono, p.inicio, fim)
	if err != nil {
		return err
	}
	if ocupado && !r.Forcar {
		return ErrAgendaOcupada
	}
	deal, err := criarNegocioDoLead(tx, lead, NovoNegocio{Valor: r.Valor, OwnerID: dono, Destino: destino})
	if err != nil {
		return err
	}
	titulo := strings.TrimSpace(r.Titulo)
	if titulo == "" {
		titulo = "Reunião com " + deal.Name
	}
	notas := strings.TrimSpace(r.Notas)
	if notas == "" {
		notas = p.Notas
	}
	m := &models.Meeting{
		Title:     filaCortar(titulo, 200),
		StartsAt:  p.inicio,
		EndsAt:    &fim,
		Location:  strings.TrimSpace(r.Local),
		Notes:     notas,
		ContactID: lead.ContactID,
		CompanyID: lead.CompanyID,
		DealID:    &deal.ID,
		UserID:    &dono,
	}
	if err := criarReuniao(tx, m); err != nil {
		return err
	}
	if err := registrarAtividade(tx, &models.Activity{
		Kind:      models.ActivityReuniao,
		Content:   fmt.Sprintf("Reunião agendada: %s (%s)", m.Title, m.StartsAt.In(time.Local).Format("02/01/2006 15:04")),
		UserID:    &eu,
		ContactID: lead.ContactID,
		CompanyID: lead.CompanyID,
		DealID:    &deal.ID,
	}); err != nil {
		return err
	}
	if err := models.ConverterLead(tx, lead.ID, deal.ID, &m.ID); err != nil {
		return err
	}
	if lead.ContactID != nil {
		c := *lead.ContactID
		depois.contatoReuniao = &c
	}
	depois.deal = deal
	// A chamada também fica ligada ao negócio.
	if _, err := tx.Exec(`UPDATE calls SET deal_id = $1 WHERE id = $2`, deal.ID, res.Call.ID); err != nil {
		return err
	}
	res.Call.DealID = &deal.ID
	res.Meeting = m
	if d, err := models.DealByID(tx, deal.ID); err == nil {
		res.Deal = d
	} else {
		res.Deal = deal
	}
	return nil
}

// -----------------------------------------------------------------------------
// Próximo lead e resumo
// -----------------------------------------------------------------------------

// ProximoDaFila é a resposta de POST /leads/fila/proximo.
type ProximoDaFila struct {
	Lead        *LeadCompleto `json:"lead"`
	MotivoVazio string        `json:"motivo_vazio"` // "" | fila_vazia | fora_da_janela
	Janela      models.Janela `json:"janela"`
}

// ProximoLead devolve o próximo lead da fila do usuário, travado por 20 min.
// Fora da janela de contato não trava nada: lead nulo e "fora_da_janela".
func ProximoLead(db *sql.DB, userID int64, vendedor string, listaID int64, pular []int64, agora time.Time) (*ProximoDaFila, error) {
	cfg := models.LoadConfigProspeccao(db)
	out := &ProximoDaFila{Janela: cfg.JanelaEm(agora)}
	if !out.Janela.Aberta {
		out.MotivoVazio = "fora_da_janela"
		return out, nil
	}
	lead, err := models.ProximoLeadDaFila(db, models.FilaOpcoes{
		UserID: userID, ListaID: listaID, PularIDs: pular, IncluirSemDono: cfg.IncluirSemDonoNaFila,
	})
	if err != nil {
		return nil, err
	}
	if lead == nil {
		out.MotivoVazio = "fila_vazia"
		return out, nil
	}
	if out.Lead, err = MontarLeadCompleto(db, lead, vendedor, agora); err != nil {
		return nil, err
	}
	return out, nil
}

// ResumoFila é a resposta de GET /leads/fila/resumo.
type ResumoFila struct {
	models.NumerosDaFila
	MetaDia int           `json:"meta_dia"`
	Janela  models.Janela `json:"janela"`
}

// ResumoDaFila junta os números do banco, a meta do dia e a janela.
func ResumoDaFila(db *sql.DB, userID int64, listaID int64, agora time.Time) (*ResumoFila, error) {
	cfg := models.LoadConfigProspeccao(db)
	n, err := models.ResumoDaFila(db, models.FilaOpcoes{
		UserID: userID, ListaID: listaID, IncluirSemDono: cfg.IncluirSemDonoNaFila,
	}, agora)
	if err != nil {
		return nil, err
	}
	return &ResumoFila{NumerosDaFila: n, MetaDia: cfg.MetaLigacoesDia, Janela: cfg.JanelaEm(agora)}, nil
}
