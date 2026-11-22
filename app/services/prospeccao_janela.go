package services

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Janela de contato, cadência da fila e agenda do vendedor. Contrato:
// docs/PROSPECCAO.md seção 8.2 com as correções de docs/PROSPECCAO-ADENDO.md
// ("Regras da cadência", resultado "reuniao" e GET /agenda/disponibilidade).
// Todos os cálculos usam time.Local (o fuso do ambiente).

// Esperas fixas da fila (horas corridas, depois ajustadas à janela).
const (
	EsperaOcupado     = time.Hour
	EsperaNaoEDecisor = 24 * time.Hour
	DuracaoReuniaoPad = 30 // minutos, sem página de agendamento
	DuracaoReuniaoMin = 5
	DuracaoReuniaoMax = 480
	MaxDiasAgenda     = 14
	DiasAgendaPadrao  = 7
	travaAgendaChave  = 7301 // pg_advisory_xact_lock(chave, dono): uma marcação por vez na agenda de cada um
)

// Cadencia é o efeito de um resultado sem contato (não atendeu, caixa postal,
// ocupado) sobre o lead.
type Cadencia struct {
	Tentativas int        // contador depois de incrementar
	Descartar  bool       // atingiu max_tentativas: descartado (sem_contato)
	Proxima    *time.Time // próxima ação (nil quando descarta)
}

// CalcularCadencia aplica a regra do adendo: tentativas = anteriores + 1; se
// chegar a max_tentativas, descarta (inclusive "ocupado"). Senão a espera é
// intervalos_horas[min(tentativas, len) - 1] (lista vazia = 24 h), ou 1 h para
// "ocupado", ajustada à janela; e se as tentativas sem contato de hoje (já
// contando esta) somam tentativas_dia, vai para o início da janela do próximo
// dia permitido.
func CalcularCadencia(cfg models.ConfigProspeccao, resultado string, anteriores, feitasHoje int, agora time.Time) Cadencia {
	c := Cadencia{Tentativas: anteriores + 1}
	if c.Tentativas >= cfg.MaxTentativas {
		c.Descartar = true
		return c
	}
	espera := cfg.IntervaloCadencia(c.Tentativas)
	if resultado == models.ResultadoOcupado {
		espera = EsperaOcupado
	}
	quando := ProximaAcaoNaJanela(cfg, agora.Add(espera), feitasHoje, agora)
	c.Proxima = &quando
	return c
}

// ProximaAcaoNaJanela é o "ajustarJanela" do adendo: fora da janela vai para
// janela_inicio do próximo dia permitido; com o limite de tentativas do dia já
// atingido (feitasHoje >= tentativas_dia), um instante que cairia hoje vai para
// o próximo dia permitido. feitasHoje = -1 ignora o limite diário.
func ProximaAcaoNaJanela(cfg models.ConfigProspeccao, t time.Time, feitasHoje int, agora time.Time) time.Time {
	t = cfg.AjustarJanela(t)
	if feitasHoje >= 0 && cfg.TentativasDia > 0 && feitasHoje >= cfg.TentativasDia && mesmoDia(t, agora) {
		t = cfg.ProximoDiaPermitido(agora)
	}
	return t.In(time.Local)
}

func mesmoDia(a, b time.Time) bool {
	a, b = a.In(time.Local), b.In(time.Local)
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// InicioDoDia é a meia-noite de hoje no fuso do ambiente.
func InicioDoDia(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// TextoQuando escreve o instante para o vendedor: "hoje às 14:00", "amanhã
// às 09:00" ou "seg., 24/11 às 09:00".
func TextoQuando(t, agora time.Time) string {
	t, agora = t.In(time.Local), agora.In(time.Local)
	hora := t.Format("15:04")
	switch {
	case mesmoDia(t, agora):
		return "hoje às " + hora
	case mesmoDia(t, agora.AddDate(0, 0, 1)):
		return "amanhã às " + hora
	}
	return diasSemanaCurtos[t.Weekday()] + ", " + t.Format("02/01") + " às " + hora
}

var diasSemanaCurtos = [...]string{"dom.", "seg.", "ter.", "qua.", "qui.", "sex.", "sáb."}

// -----------------------------------------------------------------------------
// Agenda
// -----------------------------------------------------------------------------

// HorarioLivre é um horário oferecido no seletor da reunião.
type HorarioLivre struct {
	Inicio string `json:"inicio"` // RFC 3339 com offset
	Rotulo string `json:"rotulo"` // "HH:MM" no fuso do ambiente
}

// HorarioOcupado é uma reunião já marcada na agenda do usuário.
type HorarioOcupado struct {
	Inicio string `json:"inicio"`
	Fim    string `json:"fim"`
	Titulo string `json:"titulo"`
}

// DiaAgenda é um dia de expediente com os horários livres e os ocupados.
type DiaAgenda struct {
	Data     string           `json:"data"` // AAAA-MM-DD
	Horarios []HorarioLivre   `json:"horarios"`
	Ocupados []HorarioOcupado `json:"ocupados"`
}

// Disponibilidade é a resposta de GET /agenda/disponibilidade.
type Disponibilidade struct {
	UserID     int64       `json:"user_id"`
	DuracaoMin int         `json:"duracao_min"`
	Dias       []DiaAgenda `json:"dias"`
}

// expediente é de onde sai a grade: a página de agendamento do usuário, se
// existir, ou os dias permitidos dentro da janela de contato, de 30 em 30 min.
type expediente struct {
	semanal   map[string][]models.TimeWindow
	duracao   time.Duration
	passo     time.Duration
	aviso     time.Duration
	usaJanela bool
	cfg       models.ConfigProspeccao
}

func expedienteDe(db *sql.DB, userID int64, cfg models.ConfigProspeccao) (expediente, error) {
	e := expediente{cfg: cfg}
	page, err := models.BookingPageByUser(db, userID)
	if err != nil && err != sql.ErrNoRows {
		return e, err
	}
	if page != nil && len(page.WeeklyHours) > 0 {
		dur := page.DurationMin
		if dur <= 0 {
			dur = DuracaoReuniaoPad
		}
		e.semanal = page.WeeklyHours
		e.duracao = time.Duration(dur) * time.Minute
		e.passo = time.Duration(dur+max(page.BufferMin, 0)) * time.Minute
		e.aviso = time.Duration(max(page.NoticeHours, 0)) * time.Hour
		return e, nil
	}
	e.usaJanela = true
	e.duracao = DuracaoReuniaoPad * time.Minute
	e.passo = e.duracao
	return e, nil
}

// janelasDoDia devolve as faixas [início, fim) de expediente do dia.
func (e expediente) janelasDoDia(dia time.Time) [][2]time.Time {
	if e.usaJanela {
		if !e.cfg.DiaPermitido(dia) {
			return nil
		}
		return [][2]time.Time{{
			dia.Add(time.Duration(e.cfg.JanelaInicio) * time.Hour),
			dia.Add(time.Duration(e.cfg.JanelaFim) * time.Hour),
		}}
	}
	var out [][2]time.Time
	for _, w := range e.semanal[strconv.Itoa(int(dia.Weekday()))] {
		ini, err1 := relogio(w.Start)
		fim, err2 := relogio(w.End)
		if err1 != nil || err2 != nil || fim <= ini {
			continue
		}
		out = append(out, [2]time.Time{dia.Add(ini), dia.Add(fim)})
	}
	return out
}

func relogio(v string) (time.Duration, error) {
	partes := strings.Split(strings.TrimSpace(v), ":")
	if len(partes) != 2 {
		return 0, fmt.Errorf("horário inválido")
	}
	h, err1 := strconv.Atoi(partes[0])
	m, err2 := strconv.Atoi(partes[1])
	if err1 != nil || err2 != nil || h < 0 || h > 24 || m < 0 || m > 59 {
		return 0, fmt.Errorf("horário inválido")
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

// reuniaoMarcada é uma reunião não cancelada na agenda.
type reuniaoMarcada struct {
	inicio, fim time.Time
	titulo      string
}

func reunioesNoPeriodo(db models.Executor, userID int64, de, ate time.Time) ([]reuniaoMarcada, error) {
	rows, err := db.Query(`
		SELECT starts_at, COALESCE(ends_at, starts_at + INTERVAL '30 minutes'), title
		FROM meetings
		WHERE user_id = $1 AND status <> 'cancelada' AND starts_at < $3
		  AND COALESCE(ends_at, starts_at + INTERVAL '30 minutes') > $2
		ORDER BY starts_at`, userID, de, ate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []reuniaoMarcada{}
	for rows.Next() {
		var r reuniaoMarcada
		if err := rows.Scan(&r.inicio, &r.fim, &r.titulo); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AgendaOcupada diz se o usuário já tem reunião não cancelada sobrepondo
// [inicio, fim). Não olha expediente nem aviso mínimo (só a disponibilidade
// olha). Dentro de uma transação, trava a agenda do usuário até o fim dela:
// duas marcações simultâneas no mesmo horário não passam as duas.
func AgendaOcupada(db models.Executor, userID int64, inicio, fim time.Time) (bool, error) {
	if _, ok := db.(*sql.Tx); ok {
		if _, err := db.Exec(`SELECT pg_advisory_xact_lock($1, $2::int)`, travaAgendaChave, userID); err != nil {
			return false, err
		}
	}
	var ocupado bool
	err := db.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM meetings
		WHERE user_id = $1 AND status <> 'cancelada' AND starts_at < $3
		  AND COALESCE(ends_at, starts_at + INTERVAL '30 minutes') > $2)`, userID, inicio, fim).Scan(&ocupado)
	return ocupado, err
}

// DuracaoReuniaoDe é a duração padrão da reunião marcada pela fila: a da
// página de agendamento do dono, ou 30 min.
func DuracaoReuniaoDe(db *sql.DB, userID int64) int {
	page, err := models.BookingPageByUser(db, userID)
	if err == nil && page.DurationMin > 0 {
		return page.DurationMin
	}
	return DuracaoReuniaoPad
}

// DisponibilidadeAgenda monta a grade de GET /agenda/disponibilidade para os
// próximos "dias" dias (hoje incluído): horários livres a partir de agora (e do
// aviso mínimo da página), sem sobrepor reunião marcada, e as ocupações do dia.
func DisponibilidadeAgenda(db *sql.DB, userID int64, dias int, cfg models.ConfigProspeccao, agora time.Time) (*Disponibilidade, error) {
	if dias <= 0 {
		dias = DiasAgendaPadrao
	}
	dias = min(dias, MaxDiasAgenda)
	exp, err := expedienteDe(db, userID, cfg)
	if err != nil {
		return nil, err
	}
	agora = agora.In(time.Local)
	hoje := InicioDoDia(agora)
	reunioes, err := reunioesNoPeriodo(db, userID, hoje, hoje.AddDate(0, 0, dias))
	if err != nil {
		return nil, err
	}
	cedo := agora.Add(exp.aviso)
	out := &Disponibilidade{UserID: userID, DuracaoMin: int(exp.duracao / time.Minute), Dias: []DiaAgenda{}}
	for i := 0; i < dias; i++ {
		dia := hoje.AddDate(0, 0, i)
		janelas := exp.janelasDoDia(dia)
		if len(janelas) == 0 {
			continue
		}
		d := DiaAgenda{Data: dia.Format("2006-01-02"), Horarios: []HorarioLivre{}, Ocupados: []HorarioOcupado{}}
		fimDoDia := dia.AddDate(0, 0, 1)
		for _, r := range reunioes {
			if r.inicio.Before(fimDoDia) && r.fim.After(dia) {
				d.Ocupados = append(d.Ocupados, HorarioOcupado{
					Inicio: r.inicio.In(time.Local).Format(time.RFC3339),
					Fim:    r.fim.In(time.Local).Format(time.RFC3339),
					Titulo: r.titulo,
				})
			}
		}
		vistos := map[string]bool{}
		for _, j := range janelas {
			for at := j[0]; !at.Add(exp.duracao).After(j[1]); at = at.Add(exp.passo) {
				if at.Before(cedo) || sobrepoe(at, at.Add(exp.duracao), reunioes) {
					continue
				}
				rotulo := at.Format("15:04")
				if vistos[rotulo] {
					continue
				}
				vistos[rotulo] = true
				d.Horarios = append(d.Horarios, HorarioLivre{Inicio: at.Format(time.RFC3339), Rotulo: rotulo})
			}
		}
		sort.Slice(d.Horarios, func(a, b int) bool { return d.Horarios[a].Inicio < d.Horarios[b].Inicio })
		out.Dias = append(out.Dias, d)
	}
	return out, nil
}

func sobrepoe(ini, fim time.Time, reunioes []reuniaoMarcada) bool {
	for _, r := range reunioes {
		if ini.Before(r.fim) && fim.After(r.inicio) {
			return true
		}
	}
	return false
}
