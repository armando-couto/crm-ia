package models

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// ===== Funil de vendas visual (/funil) =====
//
// Contrato em docs/FUNIL-E-INTELIGENCIA.md, Parte 1. Os nomes JSON daqui são o
// contrato com o frontend: não renomear sem atualizar o documento.

// FunilFilter são os parâmetros da análise.
type FunilFilter struct {
	PipelineID int64
	// OwnerID restringe a um dono (vendedor sem forecast.view só vê o próprio).
	OwnerID *int64
	// Periodo: 7|30|90|180|365 (dias corridos até hoje) ou mes|trimestre|ano.
	Periodo string
	// Modo: coorte (criados no período) ou fluxo (o que aconteceu no período).
	Modo string
	// PrazoPadraoDias vale para etapas sem prazo_dias (padrão 14).
	PrazoPadraoDias int
	// SemAtividadeDias: dias sem atividade para entrar em risco (padrão 7).
	SemAtividadeDias int
	// Now é "hoje" (parâmetro para os testes serem determinísticos).
	Now time.Time
}

const (
	FunilModoCoorte = "coorte"
	FunilModoFluxo  = "fluxo"
)

var (
	ErrFunilPeriodoInvalido = errors.New("período inválido (use 7, 30, 90, 180, 365, mes, trimestre ou ano)")
	ErrFunilModoInvalido    = errors.New("modo inválido (use coorte ou fluxo)")
)

// Funil é a resposta de GET /funil.
type Funil struct {
	Pipeline        FunilPipeline     `json:"pipeline"`
	Periodo         FunilPeriodo      `json:"periodo"`
	Resumo          FunilResumo       `json:"resumo"`
	Funil           []FunilEtapa      `json:"funil"`
	Movimentacao    FunilMovimentacao `json:"movimentacao"`
	GanhosPorSemana []FunilSemana     `json:"ganhos_por_semana"`
	PorDono         []FunilDono       `json:"por_dono"`
	Negocios        []FunilNegocio    `json:"negocios"`
}

type FunilPipeline struct {
	ID     int64        `json:"id"`
	Name   string       `json:"name"`
	Stages []FunilStage `json:"stages"`
}

type FunilStage struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Probability int    `json:"probability"`
	IsWon       bool   `json:"is_won"`
	IsLost      bool   `json:"is_lost"`
	PrazoDias   *int   `json:"prazo_dias"`
}

type FunilPeriodo struct {
	De     string `json:"de"`
	Ate    string `json:"ate"`
	Modo   string `json:"modo"`
	Rotulo string `json:"rotulo"`
}

type FunilResumo struct {
	Abertos          int     `json:"abertos"`
	ValorAberto      float64 `json:"valor_aberto"`
	Ponderado        float64 `json:"ponderado"`
	Ganhos           int     `json:"ganhos"`
	ValorGanho       float64 `json:"valor_ganho"`
	Perdidos         int     `json:"perdidos"`
	ValorPerdido     float64 `json:"valor_perdido"`
	TaxaGanho        float64 `json:"taxa_ganho"`
	TicketMedio      float64 `json:"ticket_medio"`
	CicloMedianoDias int     `json:"ciclo_mediano_dias"`
	Meta             float64 `json:"meta"`
	Gap              float64 `json:"gap"`
	Cobertura        float64 `json:"cobertura"`
	EmRisco          int     `json:"em_risco"`
	ValorEmRisco     float64 `json:"valor_em_risco"`
}

type FunilEtapa struct {
	StageID        int64   `json:"stage_id"`
	Nome           string  `json:"nome"`
	Alcancaram     int     `json:"alcancaram"`
	Valor          float64 `json:"valor"`
	ConversaoTotal float64 `json:"conversao_total"`
	ConversaoEtapa float64 `json:"conversao_etapa"`
	MedianaDias    int     `json:"mediana_dias"`
	AbertosAgora   int     `json:"abertos_agora"`
	ValorAberto    float64 `json:"valor_aberto"`
	Ponderado      float64 `json:"ponderado"`
	Estagnados     int     `json:"estagnados"`
}

// FunilBarra é um degrau do waterfall.
type FunilBarra struct {
	Qtd   int     `json:"qtd"`
	Valor float64 `json:"valor"`
}

type FunilMovimentacao struct {
	Inicio   FunilBarra `json:"inicio"`
	Criados  FunilBarra `json:"criados"`
	Aumentos FunilBarra `json:"aumentos"`
	Reducoes FunilBarra `json:"reducoes"`
	Adiados  FunilBarra `json:"adiados"`
	Ganhos   FunilBarra `json:"ganhos"`
	Perdidos FunilBarra `json:"perdidos"`
	Fim      FunilBarra `json:"fim"`
}

type FunilSemana struct {
	Semana string  `json:"semana"` // segunda-feira da semana, AAAA-MM-DD
	Qtd    int     `json:"qtd"`
	Valor  float64 `json:"valor"`
}

type FunilDono struct {
	OwnerID     *int64  `json:"owner_id"`
	Nome        string  `json:"nome"`
	Abertos     int     `json:"abertos"`
	ValorAberto float64 `json:"valor_aberto"`
	Ponderado   float64 `json:"ponderado"`
	Ganhos      int     `json:"ganhos"`
	ValorGanho  float64 `json:"valor_ganho"`
	Perdidos    int     `json:"perdidos"`
	TaxaGanho   float64 `json:"taxa_ganho"`
	EmRisco     int     `json:"em_risco"`
	Meta        float64 `json:"meta"`
}

type FunilVariacao struct {
	Valor         float64 `json:"valor"`
	CloseDateDias int     `json:"close_date_dias"`
	Etapa         string  `json:"etapa"`
}

type FunilNegocio struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	Amount           float64        `json:"amount"`
	Ponderado        float64        `json:"ponderado"`
	StageID          int64          `json:"stage_id"`
	StageName        string         `json:"stage_name"`
	Status           string         `json:"status"`
	OwnerID          *int64         `json:"owner_id"`
	OwnerName        string         `json:"owner_name"`
	CompanyID        *int64         `json:"company_id"`
	CompanyName      string         `json:"company_name"`
	CloseDate        *string        `json:"close_date"`
	CreatedAt        time.Time      `json:"created_at"`
	ClosedAt         *time.Time     `json:"closed_at"`
	DiasNaEtapa      int            `json:"dias_na_etapa"`
	PrazoDias        *int           `json:"prazo_dias"`
	DiasSemAtividade int            `json:"dias_sem_atividade"`
	Adiamentos       int            `json:"adiamentos"`
	Variacao7d       *FunilVariacao `json:"variacao_7d,omitempty"`
	Motivos          []string       `json:"motivos"`
}

// Motivos de risco de um negócio aberto.
const (
	RiscoEstagnado    = "estagnado"
	RiscoSemAtividade = "sem_atividade"
	RiscoAdiado       = "adiado"
	RiscoVencido      = "vencido"
)

// ----- Entrada da agregação (o que vem do banco) -----

// funilDeal é o negócio como a análise precisa dele.
type funilDeal struct {
	ID             int64
	Name           string
	Amount         float64
	StageID        int64
	Status         string
	OwnerID        *int64
	OwnerName      string
	CompanyID      *int64
	CompanyName    string
	CloseDate      *time.Time
	CreatedAt      time.Time
	ClosedAt       *time.Time
	LastActivityAt *time.Time
}

// funilHist é uma entrada de deal_stage_history.
type funilHist struct {
	DealID    int64
	StageID   int64
	EnteredAt time.Time
}

// funilChange é uma linha de deal_changes.
type funilChange struct {
	DealID    int64
	Campo     string
	De        string
	Para      string
	CreatedAt time.Time
}

// funilDados reúne tudo que a agregação pura consome.
type funilDados struct {
	Pipeline  FunilPipeline
	Deals     []funilDeal
	Historico []funilHist
	Mudancas  []funilChange
	// Metas do mês corrente por dono e da equipe (ver loadGoals).
	Metas      map[int64]float64
	MetaEquipe float64
}

// ----- Período -----

const dataISO = "2006-01-02"

func inicioDoDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// periodoFunil traduz o parâmetro em data de início (meia-noite) e rótulo.
func periodoFunil(periodo string, now time.Time) (time.Time, string, error) {
	hoje := inicioDoDia(now)
	switch periodo {
	case "7", "30", "90", "180", "365":
		dias, _ := strconv.Atoi(periodo)
		return hoje.AddDate(0, 0, -dias), fmt.Sprintf("Últimos %d dias", dias), nil
	case "mes":
		return time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, hoje.Location()), "Este mês", nil
	case "trimestre":
		primeiroMes := time.Month((int(hoje.Month())-1)/3*3 + 1)
		return time.Date(hoje.Year(), primeiroMes, 1, 0, 0, 0, 0, hoje.Location()), "Este trimestre", nil
	case "ano":
		return time.Date(hoje.Year(), 1, 1, 0, 0, 0, 0, hoje.Location()), "Este ano", nil
	}
	return time.Time{}, "", ErrFunilPeriodoInvalido
}

// normalizar aplica os padrões do filtro.
func (f FunilFilter) normalizar() FunilFilter {
	if f.Periodo == "" {
		f.Periodo = "30"
	}
	if f.Modo == "" {
		f.Modo = FunilModoFluxo
	}
	if f.PrazoPadraoDias <= 0 {
		f.PrazoPadraoDias = 14
	}
	if f.SemAtividadeDias <= 0 {
		f.SemAtividadeDias = 7
	}
	if f.Now.IsZero() {
		f.Now = time.Now()
	}
	return f
}

// metaProporcional converte a meta mensal na meta do período: inteira quando o
// período é o mês; senão, proporcional aos dias (meta diária × dias do período).
func metaProporcional(metaMes float64, periodo string, de, now time.Time) float64 {
	if metaMes == 0 || periodo == "mes" {
		return metaMes
	}
	hoje := inicioDoDia(now)
	diasMes := time.Date(hoje.Year(), hoje.Month()+1, 0, 0, 0, 0, 0, hoje.Location()).Day()
	var diasPeriodo int
	switch periodo {
	case "trimestre", "ano":
		diasPeriodo = diasDesde(de, hoje) + 1 // calendário: hoje conta
	default:
		diasPeriodo = diasDesde(de, hoje)
	}
	if diasPeriodo < 1 {
		diasPeriodo = 1
	}
	return metaMes / float64(diasMes) * float64(diasPeriodo)
}

// ----- Carga -----

// LoadFunil monta a análise do funil do pipeline no período.
func LoadFunil(db *sql.DB, f FunilFilter) (*Funil, error) {
	f = f.normalizar()
	if f.Modo != FunilModoCoorte && f.Modo != FunilModoFluxo {
		return nil, ErrFunilModoInvalido
	}
	de, _, err := periodoFunil(f.Periodo, f.Now)
	if err != nil {
		return nil, err
	}
	dados, err := carregarFunil(db, f, de)
	if err != nil {
		return nil, err
	}
	return calcularFunil(dados, f), nil
}

// carregarFunil traz do banco o pipeline, os negócios relevantes (abertos ou
// fechados a partir do início do período), o histórico de etapas e as
// mudanças deles, mais as metas do mês. A agregação fica em calcularFunil.
func carregarFunil(db *sql.DB, f FunilFilter, de time.Time) (*funilDados, error) {
	dados := &funilDados{Metas: map[int64]float64{}}

	if err := db.QueryRow(`SELECT id, name FROM pipelines WHERE id = $1`, f.PipelineID).
		Scan(&dados.Pipeline.ID, &dados.Pipeline.Name); err != nil {
		return nil, err
	}
	dados.Pipeline.Stages = []FunilStage{}
	stageRows, err := db.Query(`
		SELECT id, name, position, probability, is_won, is_lost, prazo_dias
		FROM pipeline_stages WHERE pipeline_id = $1 ORDER BY position, id`, f.PipelineID)
	if err != nil {
		return nil, err
	}
	defer stageRows.Close()
	for stageRows.Next() {
		var s FunilStage
		if err := stageRows.Scan(&s.ID, &s.Name, &s.Position, &s.Probability, &s.IsWon, &s.IsLost, &s.PrazoDias); err != nil {
			return nil, err
		}
		dados.Pipeline.Stages = append(dados.Pipeline.Stages, s)
	}
	if err := stageRows.Err(); err != nil {
		return nil, err
	}

	// Negócios fechados antes do período não entram em nada: ficam de fora já
	// na consulta. O mesmo recorte vale para histórico e mudanças.
	recorte := `d.pipeline_id = $1 AND (d.closed_at IS NULL OR d.closed_at >= $2)`
	args := []any{f.PipelineID, de}
	if f.OwnerID != nil {
		args = append(args, *f.OwnerID)
		recorte += fmt.Sprintf(" AND d.owner_id = $%d", len(args))
	}

	dealRows, err := db.Query(`
		SELECT d.id, d.name, d.amount, d.stage_id, d.status,
		       d.owner_id, COALESCE(u.name, ''), d.company_id, COALESCE(co.name, ''),
		       d.close_date, d.created_at, d.closed_at,
		       (SELECT MAX(a.created_at) FROM activities a WHERE a.deal_id = d.id)
		FROM deals d
		LEFT JOIN users u ON u.id = d.owner_id
		LEFT JOIN companies co ON co.id = d.company_id
		WHERE `+recorte+`
		ORDER BY d.id`, args...)
	if err != nil {
		return nil, err
	}
	defer dealRows.Close()
	for dealRows.Next() {
		var d funilDeal
		if err := dealRows.Scan(&d.ID, &d.Name, &d.Amount, &d.StageID, &d.Status,
			&d.OwnerID, &d.OwnerName, &d.CompanyID, &d.CompanyName,
			&d.CloseDate, &d.CreatedAt, &d.ClosedAt, &d.LastActivityAt); err != nil {
			return nil, err
		}
		dados.Deals = append(dados.Deals, d)
	}
	if err := dealRows.Err(); err != nil {
		return nil, err
	}

	histRows, err := db.Query(`
		SELECT h.deal_id, h.stage_id, h.entered_at
		FROM deal_stage_history h JOIN deals d ON d.id = h.deal_id
		WHERE `+recorte+`
		ORDER BY h.deal_id, h.entered_at, h.id`, args...)
	if err != nil {
		return nil, err
	}
	defer histRows.Close()
	for histRows.Next() {
		var h funilHist
		if err := histRows.Scan(&h.DealID, &h.StageID, &h.EnteredAt); err != nil {
			return nil, err
		}
		dados.Historico = append(dados.Historico, h)
	}
	if err := histRows.Err(); err != nil {
		return nil, err
	}

	// Mudanças desde o início do período (ou dos últimos 7 dias, para a
	// variação recente, quando o período for mais curto).
	desde := de
	if sete := f.Now.AddDate(0, 0, -7); sete.Before(desde) {
		desde = sete
	}
	changeArgs := append(append([]any{}, args...), desde)
	changeRows, err := db.Query(fmt.Sprintf(`
		SELECT c.deal_id, c.campo, COALESCE(c.de, ''), COALESCE(c.para, ''), c.created_at
		FROM deal_changes c JOIN deals d ON d.id = c.deal_id
		WHERE %s AND c.created_at >= $%d
		ORDER BY c.deal_id, c.created_at, c.id`, recorte, len(changeArgs)), changeArgs...)
	if err != nil {
		return nil, err
	}
	defer changeRows.Close()
	for changeRows.Next() {
		var c funilChange
		if err := changeRows.Scan(&c.DealID, &c.Campo, &c.De, &c.Para, &c.CreatedAt); err != nil {
			return nil, err
		}
		dados.Mudancas = append(dados.Mudancas, c)
	}
	if err := changeRows.Err(); err != nil {
		return nil, err
	}

	hoje := inicioDoDia(f.Now)
	metas, metaEquipe, err := loadGoals(db, time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, hoje.Location()))
	if err != nil {
		return nil, err
	}
	dados.Metas, dados.MetaEquipe = metas, metaEquipe
	return dados, nil
}

// ----- Agregação -----

// calcularFunil é a agregação pura: recebe os dados já carregados e devolve a
// resposta completa. Sem SQL, para os testes exercitarem as regras direto.
func calcularFunil(dados *funilDados, f FunilFilter) *Funil {
	f = f.normalizar()
	now := f.Now
	hoje := inicioDoDia(now)
	de, rotulo, err := periodoFunil(f.Periodo, now)
	if err != nil {
		de, rotulo, _ = periodoFunil("30", now)
		f.Periodo = "30"
	}
	noPeriodo := func(t time.Time) bool { return !t.Before(de) && !t.After(now) }

	stages := append([]FunilStage{}, dados.Pipeline.Stages...)
	sort.SliceStable(stages, func(i, j int) bool {
		if stages[i].Position != stages[j].Position {
			return stages[i].Position < stages[j].Position
		}
		return stages[i].ID < stages[j].ID
	})
	stageByID := map[int64]*FunilStage{}
	maiorPosNormal := -1
	for i := range stages {
		s := &stages[i]
		stageByID[s.ID] = s
		if !s.IsWon && !s.IsLost && s.Position > maiorPosNormal {
			maiorPosNormal = s.Position
		}
	}
	prazoDe := func(stageID int64) int {
		if s := stageByID[stageID]; s != nil && s.PrazoDias != nil && *s.PrazoDias > 0 {
			return *s.PrazoDias
		}
		return f.PrazoPadraoDias
	}
	probabilidade := func(stageID int64) float64 {
		if s := stageByID[stageID]; s != nil {
			return float64(s.Probability) / 100
		}
		return 0
	}
	nomeEtapa := func(stageID int64) string {
		if s := stageByID[stageID]; s != nil {
			return s.Name
		}
		return ""
	}

	histPorDeal := map[int64][]funilHist{}
	for _, h := range dados.Historico {
		histPorDeal[h.DealID] = append(histPorDeal[h.DealID], h)
	}
	for id := range histPorDeal {
		h := histPorDeal[id]
		sort.SliceStable(h, func(i, j int) bool { return h[i].EnteredAt.Before(h[j].EnteredAt) })
	}
	mudancasPorDeal := map[int64][]funilChange{}
	for _, c := range dados.Mudancas {
		mudancasPorDeal[c.DealID] = append(mudancasPorDeal[c.DealID], c)
	}
	for id := range mudancasPorDeal {
		c := mudancasPorDeal[id]
		sort.SliceStable(c, func(i, j int) bool { return c[i].CreatedAt.Before(c[j].CreatedAt) })
	}

	fechadoNoPeriodo := func(d *funilDeal) bool {
		return d.Status != DealAberto && d.ClosedAt != nil && noPeriodo(*d.ClosedAt)
	}

	// Conjunto relevante ao modo: é o que alimenta resumo, risco, donos e lista.
	relevantes := []*funilDeal{}
	for i := range dados.Deals {
		d := &dados.Deals[i]
		switch f.Modo {
		case FunilModoCoorte:
			if noPeriodo(d.CreatedAt) {
				relevantes = append(relevantes, d)
			}
		default:
			if d.Status == DealAberto || fechadoNoPeriodo(d) {
				relevantes = append(relevantes, d)
			}
		}
	}

	// Coorte do funil: em coorte, os criados no período; em fluxo, quem teve
	// alguma passagem de etapa (ou fechamento) dentro do período.
	coorte := []*funilDeal{}
	for _, d := range relevantes {
		if f.Modo == FunilModoCoorte {
			coorte = append(coorte, d)
			continue
		}
		entrou := noPeriodo(d.CreatedAt) || fechadoNoPeriodo(d)
		if !entrou {
			for _, h := range histPorDeal[d.ID] {
				if noPeriodo(h.EnteredAt) {
					entrou = true
					break
				}
			}
		}
		if entrou {
			coorte = append(coorte, d)
		}
	}

	out := &Funil{
		Pipeline: FunilPipeline{ID: dados.Pipeline.ID, Name: dados.Pipeline.Name, Stages: stages},
		Periodo: FunilPeriodo{
			De: de.Format(dataISO), Ate: hoje.Format(dataISO), Modo: f.Modo, Rotulo: rotulo,
		},
		Funil:           []FunilEtapa{},
		GanhosPorSemana: []FunilSemana{},
		PorDono:         []FunilDono{},
		Negocios:        []FunilNegocio{},
	}

	// ----- Funil por etapa (coorte) -----
	// Funil "solto": chegar a uma posição conta como ter passado pelas
	// anteriores; ganhar conta como ter passado por todas as etapas normais.
	// Etapas de ganho/perda contam só quem de fato passou por elas.
	etapas := make([]FunilEtapa, len(stages))
	duracoes := map[int64][]float64{}
	for i, s := range stages {
		etapas[i] = FunilEtapa{StageID: s.ID, Nome: s.Name}
	}
	for _, d := range coorte {
		hist := histPorDeal[d.ID]
		maiorPos := -1
		passou := map[int64]bool{d.StageID: true}
		ganhou := d.Status == DealGanho
		for _, h := range hist {
			if f.Modo == FunilModoFluxo && !noPeriodo(h.EnteredAt) {
				continue
			}
			passou[h.StageID] = true
			s := stageByID[h.StageID]
			if s == nil {
				continue
			}
			if s.IsWon {
				ganhou = true
			} else if !s.IsLost && s.Position > maiorPos {
				maiorPos = s.Position
			}
		}
		if ganhou {
			maiorPos = maiorPosNormal
		}
		for i, s := range stages {
			alcancou := false
			if s.IsWon || s.IsLost {
				alcancou = passou[s.ID]
			} else {
				alcancou = maiorPos >= s.Position
			}
			if alcancou {
				etapas[i].Alcancaram++
				etapas[i].Valor += d.Amount
			}
		}
		// Tempo em cada etapa: só quem saiu dela dentro do período.
		for i := 0; i+1 < len(hist); i++ {
			if noPeriodo(hist[i+1].EnteredAt) {
				duracoes[hist[i].StageID] = append(duracoes[hist[i].StageID],
					hist[i+1].EnteredAt.Sub(hist[i].EnteredAt).Hours()/24)
			}
		}
	}
	for i := range etapas {
		if i == 0 {
			if etapas[i].Alcancaram > 0 {
				etapas[i].ConversaoTotal, etapas[i].ConversaoEtapa = 1, 1
			}
		} else {
			if topo := etapas[0].Alcancaram; topo > 0 {
				etapas[i].ConversaoTotal = float64(etapas[i].Alcancaram) / float64(topo)
			}
			if anterior := etapas[i-1].Alcancaram; anterior > 0 {
				etapas[i].ConversaoEtapa = float64(etapas[i].Alcancaram) / float64(anterior)
			}
		}
		etapas[i].MedianaDias = int(mediana(duracoes[etapas[i].StageID]) + 0.5)
	}
	indiceEtapa := map[int64]int{}
	for i, e := range etapas {
		indiceEtapa[e.StageID] = i
	}

	// ----- Negócios: idade na etapa, risco, variação, waterfall -----
	seteDias := now.AddDate(0, 0, -7)
	semana := func(t time.Time) time.Time {
		dia := inicioDoDia(t)
		recuo := (int(dia.Weekday()) + 6) % 7 // segunda = 0
		return dia.AddDate(0, 0, -recuo)
	}
	semanas := map[string]*FunilSemana{}
	for w := semana(de); !w.After(hoje); w = w.AddDate(0, 0, 7) {
		chave := w.Format(dataISO)
		semanas[chave] = &FunilSemana{Semana: chave}
		out.GanhosPorSemana = append(out.GanhosPorSemana, FunilSemana{Semana: chave})
	}

	type acumuladoDono struct {
		dono FunilDono
		meta float64
	}
	donos := map[int64]*acumuladoDono{}
	var ciclos []float64
	mov := &out.Movimentacao

	for _, d := range relevantes {
		hist := histPorDeal[d.ID]
		mudancas := mudancasPorDeal[d.ID]

		// Última entrada na etapa atual; sem histórico, a criação.
		var entrouNaEtapa time.Time
		for _, h := range hist {
			if h.StageID == d.StageID && h.EnteredAt.After(entrouNaEtapa) {
				entrouNaEtapa = h.EnteredAt
			}
		}
		if entrouNaEtapa.IsZero() {
			entrouNaEtapa = d.CreatedAt
		}
		ultimaAtividade := d.CreatedAt
		if d.LastActivityAt != nil && d.LastActivityAt.After(ultimaAtividade) {
			ultimaAtividade = *d.LastActivityAt
		}

		n := FunilNegocio{
			ID: d.ID, Name: d.Name, Amount: d.Amount, StageID: d.StageID, StageName: nomeEtapa(d.StageID),
			Status: d.Status, OwnerID: d.OwnerID, OwnerName: d.OwnerName,
			CompanyID: d.CompanyID, CompanyName: d.CompanyName,
			CreatedAt: d.CreatedAt, ClosedAt: d.ClosedAt,
			DiasNaEtapa:      diasDesde(entrouNaEtapa, now),
			DiasSemAtividade: diasDesde(ultimaAtividade, now),
			Motivos:          []string{},
		}
		if s := stageByID[d.StageID]; s != nil {
			n.PrazoDias = s.PrazoDias
		}
		if d.CloseDate != nil {
			cd := d.CloseDate.Format(dataISO)
			n.CloseDate = &cd
		}
		aberto := d.Status == DealAberto
		if aberto {
			n.Ponderado = d.Amount * probabilidade(d.StageID)
		}

		// Adiamentos no período e variação dos últimos 7 dias.
		var variacao FunilVariacao
		temVariacao := false
		var primeiraDataDe, ultimaDataPara *time.Time
		var primeiraEtapaDe, ultimaEtapaPara string
		for _, c := range mudancas {
			switch c.Campo {
			case DealChangeCloseDate:
				deData, okDe := time.Parse(dataISO, c.De)
				paraData, okPara := time.Parse(dataISO, c.Para)
				if noPeriodo(c.CreatedAt) && okDe == nil && okPara == nil && paraData.After(deData) {
					n.Adiamentos++
				}
				if !c.CreatedAt.Before(seteDias) {
					temVariacao = true
					if okDe == nil && primeiraDataDe == nil {
						primeiraDataDe = &deData
					}
					if okPara == nil {
						ultimaDataPara = &paraData
					}
				}
			case DealChangeAmount:
				if !c.CreatedAt.Before(seteDias) {
					temVariacao = true
					deV, _ := strconv.ParseFloat(c.De, 64)
					paraV, _ := strconv.ParseFloat(c.Para, 64)
					variacao.Valor += paraV - deV
				}
			case DealChangeStage:
				if !c.CreatedAt.Before(seteDias) {
					temVariacao = true
					deID, _ := strconv.ParseInt(c.De, 10, 64)
					paraID, _ := strconv.ParseInt(c.Para, 10, 64)
					if primeiraEtapaDe == "" {
						primeiraEtapaDe = nomeEtapa(deID)
					}
					ultimaEtapaPara = nomeEtapa(paraID)
				}
			}
		}
		if temVariacao {
			if primeiraDataDe != nil && ultimaDataPara != nil {
				variacao.CloseDateDias = int(ultimaDataPara.Sub(*primeiraDataDe).Hours() / 24)
			}
			if primeiraEtapaDe != "" || ultimaEtapaPara != "" {
				variacao.Etapa = primeiraEtapaDe + "→" + ultimaEtapaPara
			}
			n.Variacao7d = &variacao
		}

		// Risco (só abertos).
		if aberto {
			if n.DiasNaEtapa > prazoDe(d.StageID) {
				n.Motivos = append(n.Motivos, RiscoEstagnado)
			}
			if n.DiasSemAtividade > f.SemAtividadeDias {
				n.Motivos = append(n.Motivos, RiscoSemAtividade)
			}
			if n.Adiamentos > 0 {
				n.Motivos = append(n.Motivos, RiscoAdiado)
			}
			if d.CloseDate != nil && inicioDoDia(*d.CloseDate).Before(hoje) {
				n.Motivos = append(n.Motivos, RiscoVencido)
			}
		}
		emRisco := len(n.Motivos) > 0

		// Resumo e etapa atual.
		r := &out.Resumo
		if aberto {
			r.Abertos++
			r.ValorAberto += d.Amount
			r.Ponderado += n.Ponderado
			if emRisco {
				r.EmRisco++
				r.ValorEmRisco += d.Amount
			}
			if i, ok := indiceEtapa[d.StageID]; ok {
				etapas[i].AbertosAgora++
				etapas[i].ValorAberto += d.Amount
				etapas[i].Ponderado += n.Ponderado
				if n.DiasNaEtapa > prazoDe(d.StageID) {
					etapas[i].Estagnados++
				}
			}
		}
		ganhoNoPeriodo := fechadoNoPeriodo(d) && d.Status == DealGanho
		perdidoNoPeriodo := fechadoNoPeriodo(d) && d.Status == DealPerdido
		if ganhoNoPeriodo {
			r.Ganhos++
			r.ValorGanho += d.Amount
			ciclos = append(ciclos, d.ClosedAt.Sub(d.CreatedAt).Hours()/24)
			if w := semanas[semana(*d.ClosedAt).Format(dataISO)]; w != nil {
				w.Qtd++
				w.Valor += d.Amount
			}
		}
		if perdidoNoPeriodo {
			r.Perdidos++
			r.ValorPerdido += d.Amount
		}

		// Waterfall: cada negócio começa no valor do início do período (ou no
		// valor de criação), soma a variação líquida e termina aberto ou sai
		// como ganho/perdido. Assim inicio+criados+aumentos+reducoes+ganhos+perdidos=fim.
		var inicial float64
		if d.CreatedAt.Before(de) {
			inicial = valorEm(d, mudancas, de)
			mov.Inicio.Qtd++
			mov.Inicio.Valor += inicial
		} else {
			inicial = valorEm(d, mudancas, d.CreatedAt)
			mov.Criados.Qtd++
			mov.Criados.Valor += inicial
		}
		switch liquido := d.Amount - inicial; {
		case liquido > 0:
			mov.Aumentos.Qtd++
			mov.Aumentos.Valor += liquido
		case liquido < 0:
			mov.Reducoes.Qtd++
			mov.Reducoes.Valor += liquido
		}
		if n.Adiamentos > 0 {
			mov.Adiados.Qtd++
			mov.Adiados.Valor += d.Amount
		}
		switch {
		case ganhoNoPeriodo:
			mov.Ganhos.Qtd++
			mov.Ganhos.Valor -= d.Amount
		case perdidoNoPeriodo:
			mov.Perdidos.Qtd++
			mov.Perdidos.Valor -= d.Amount
		case aberto:
			mov.Fim.Qtd++
			mov.Fim.Valor += d.Amount
		}

		// Por dono.
		chave := int64(0)
		if d.OwnerID != nil {
			chave = *d.OwnerID
		}
		ac := donos[chave]
		if ac == nil {
			ac = &acumuladoDono{dono: FunilDono{OwnerID: d.OwnerID, Nome: d.OwnerName}}
			if d.OwnerID == nil {
				ac.dono.Nome = "Sem dono"
			} else {
				ac.meta = dados.Metas[*d.OwnerID]
			}
			donos[chave] = ac
		}
		if aberto {
			ac.dono.Abertos++
			ac.dono.ValorAberto += d.Amount
			ac.dono.Ponderado += n.Ponderado
			if emRisco {
				ac.dono.EmRisco++
			}
		}
		if ganhoNoPeriodo {
			ac.dono.Ganhos++
			ac.dono.ValorGanho += d.Amount
		}
		if perdidoNoPeriodo {
			ac.dono.Perdidos++
		}

		out.Negocios = append(out.Negocios, n)
	}

	// ----- Resumo: taxas, ciclo, meta -----
	r := &out.Resumo
	if fechados := r.Ganhos + r.Perdidos; fechados > 0 {
		r.TaxaGanho = float64(r.Ganhos) / float64(fechados)
	}
	if r.Ganhos > 0 {
		r.TicketMedio = r.ValorGanho / float64(r.Ganhos)
	}
	r.CicloMedianoDias = int(mediana(ciclos) + 0.5)

	metaMes := dados.MetaEquipe
	if f.OwnerID != nil {
		metaMes = dados.Metas[*f.OwnerID]
	}
	r.Meta = metaProporcional(metaMes, f.Periodo, de, now)
	if r.Meta > 0 {
		r.Gap = r.Meta - r.ValorGanho
		if r.Gap > 0 {
			r.Cobertura = r.Ponderado / r.Gap
		}
	}

	out.Funil = etapas
	for i := range out.GanhosPorSemana {
		if w := semanas[out.GanhosPorSemana[i].Semana]; w != nil {
			out.GanhosPorSemana[i] = *w
		}
	}

	for _, ac := range donos {
		dono := ac.dono
		if fechados := dono.Ganhos + dono.Perdidos; fechados > 0 {
			dono.TaxaGanho = float64(dono.Ganhos) / float64(fechados)
		}
		dono.Meta = metaProporcional(ac.meta, f.Periodo, de, now)
		out.PorDono = append(out.PorDono, dono)
	}
	sort.SliceStable(out.PorDono, func(i, j int) bool {
		a, b := out.PorDono[i], out.PorDono[j]
		if a.ValorGanho != b.ValorGanho {
			return a.ValorGanho > b.ValorGanho
		}
		if a.Ponderado != b.Ponderado {
			return a.Ponderado > b.Ponderado
		}
		return a.Nome < b.Nome
	})

	sort.SliceStable(out.Negocios, func(i, j int) bool {
		if out.Negocios[i].Amount != out.Negocios[j].Amount {
			return out.Negocios[i].Amount > out.Negocios[j].Amount
		}
		return out.Negocios[i].ID < out.Negocios[j].ID
	})
	if len(out.Negocios) > 1000 {
		out.Negocios = out.Negocios[:1000]
	}
	return out
}

// valorEm reconstrói o valor do negócio no instante t: o valor atual revertendo
// as mudanças de amount feitas a partir de t, ou seja, o "de" da primeira
// mudança a partir de t (sem mudanças, o valor atual).
func valorEm(d *funilDeal, mudancas []funilChange, t time.Time) float64 {
	for _, c := range mudancas {
		if c.Campo != DealChangeAmount || c.CreatedAt.Before(t) {
			continue
		}
		if v, err := strconv.ParseFloat(c.De, 64); err == nil {
			return v
		}
	}
	return d.Amount
}

// mediana de uma amostra (0 quando vazia).
func mediana(valores []float64) float64 {
	if len(valores) == 0 {
		return 0
	}
	v := append([]float64{}, valores...)
	sort.Float64s(v)
	meio := len(v) / 2
	if len(v)%2 == 1 {
		return v[meio]
	}
	return (v[meio-1] + v[meio]) / 2
}
