package models

import (
	"math"
	"testing"
	"time"
)

// Cenário fixo: hoje é sábado, 24/10/2026, período "30" => de 24/09/2026.
var funilAgora = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func funilPipelineTeste() FunilPipeline {
	prazoProposta := 10
	return FunilPipeline{ID: 1, Name: "Vendas", Stages: []FunilStage{
		{ID: 1, Name: "Novo Lead", Position: 0, Probability: 10},
		{ID: 2, Name: "Proposta", Position: 1, Probability: 50, PrazoDias: &prazoProposta},
		{ID: 3, Name: "Negociação", Position: 2, Probability: 75},
		{ID: 4, Name: "Ganho", Position: 3, Probability: 100, IsWon: true},
		{ID: 5, Name: "Perdido", Position: 4, Probability: 0, IsLost: true},
	}}
}

func dias(n int) time.Time { return funilAgora.AddDate(0, 0, n) }
func ptrTime(t time.Time) *time.Time {
	return &t
}

func filtroTeste(modo string) FunilFilter {
	return FunilFilter{PipelineID: 1, Periodo: "30", Modo: modo, PrazoPadraoDias: 14, SemAtividadeDias: 7, Now: funilAgora}
}

func quase(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// percorre devolve o histórico de um negócio passando pelas etapas nas datas dadas.
func percorre(dealID int64, etapas []int64, quando []time.Time) []funilHist {
	h := []funilHist{}
	for i, s := range etapas {
		h = append(h, funilHist{DealID: dealID, StageID: s, EnteredAt: quando[i]})
	}
	return h
}

// Em coorte, o funil conta quem foi criado no período e até onde chegou, de
// forma "solta": ganhar conta como ter passado por todas as etapas.
func TestCalcularFunilCoorte(t *testing.T) {
	dados := &funilDados{Pipeline: funilPipelineTeste(), Metas: map[int64]float64{}}
	dados.Deals = []funilDeal{
		{ID: 1, Name: "A", Amount: 1000, StageID: 4, Status: DealGanho, CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-2))},
		{ID: 2, Name: "B", Amount: 2000, StageID: 2, Status: DealAberto, CreatedAt: dias(-15)},
		{ID: 3, Name: "C", Amount: 500, StageID: 5, Status: DealPerdido, CreatedAt: dias(-10), ClosedAt: ptrTime(dias(-1))},
		{ID: 4, Name: "D", Amount: 100, StageID: 1, Status: DealAberto, CreatedAt: dias(-3)},
		// Criado antes do período: fora da coorte.
		{ID: 5, Name: "E", Amount: 9999, StageID: 3, Status: DealAberto, CreatedAt: dias(-60)},
	}
	dados.Historico = append(dados.Historico, percorre(1, []int64{1, 2, 3, 4}, []time.Time{dias(-20), dias(-15), dias(-8), dias(-2)})...)
	dados.Historico = append(dados.Historico, percorre(2, []int64{1, 2}, []time.Time{dias(-15), dias(-12)})...)
	dados.Historico = append(dados.Historico, percorre(3, []int64{1, 5}, []time.Time{dias(-10), dias(-1)})...)
	dados.Historico = append(dados.Historico, percorre(4, []int64{1}, []time.Time{dias(-3)})...)
	dados.Historico = append(dados.Historico, percorre(5, []int64{1, 2, 3}, []time.Time{dias(-60), dias(-50), dias(-40)})...)

	out := calcularFunil(dados, filtroTeste(FunilModoCoorte))

	if out.Periodo.De != "2026-09-24" || out.Periodo.Ate != "2026-10-24" || out.Periodo.Rotulo != "Últimos 30 dias" || out.Periodo.Modo != "coorte" {
		t.Fatalf("período errado: %+v", out.Periodo)
	}
	alc := []int{}
	for _, e := range out.Funil {
		alc = append(alc, e.Alcancaram)
	}
	if want := []int{4, 2, 1, 1, 1}; !equalInts(alc, want) {
		t.Fatalf("alcançaram = %v, esperado %v", alc, want)
	}
	if !quase(out.Funil[0].ConversaoTotal, 1) || !quase(out.Funil[1].ConversaoTotal, 0.5) || !quase(out.Funil[2].ConversaoTotal, 0.25) {
		t.Fatalf("conversão total errada: %+v", out.Funil)
	}
	if !quase(out.Funil[1].ConversaoEtapa, 0.5) || !quase(out.Funil[2].ConversaoEtapa, 0.5) {
		t.Fatalf("conversão etapa a etapa errada: %+v", out.Funil)
	}
	if !quase(out.Funil[0].Valor, 3600) || !quase(out.Funil[1].Valor, 3000) {
		t.Fatalf("valor por etapa errado: %+v", out.Funil)
	}
	// Estado atual só da coorte: E (fora) não conta como aberto.
	if out.Resumo.Abertos != 2 || !quase(out.Resumo.ValorAberto, 2100) || out.Resumo.Ganhos != 1 || out.Resumo.Perdidos != 1 {
		t.Fatalf("resumo errado: %+v", out.Resumo)
	}
	if !quase(out.Resumo.TaxaGanho, 0.5) || !quase(out.Resumo.TicketMedio, 1000) || out.Resumo.CicloMedianoDias != 18 {
		t.Fatalf("taxa/ticket/ciclo errados: %+v", out.Resumo)
	}
	// Ponderado: B 2000×50% + D 100×10%.
	if !quase(out.Resumo.Ponderado, 1010) {
		t.Fatalf("ponderado = %v", out.Resumo.Ponderado)
	}
	if len(out.Negocios) != 4 {
		t.Fatalf("negócios da coorte = %d, esperado 4", len(out.Negocios))
	}
	// Em coorte nada existia no início do período.
	if out.Movimentacao.Inicio.Qtd != 0 || out.Movimentacao.Criados.Qtd != 4 {
		t.Fatalf("movimentação da coorte errada: %+v", out.Movimentacao)
	}
}

// Em fluxo, entram os abertos agora e os fechados no período; o funil olha
// quem se movimentou no período, e a mediana só conta quem saiu da etapa no período.
func TestCalcularFunilFluxoEMediana(t *testing.T) {
	dados := &funilDados{Pipeline: funilPipelineTeste(), Metas: map[int64]float64{}}
	dados.Deals = []funilDeal{
		// X: criado no período, 3 dias na etapa 1.
		{ID: 1, Name: "X", Amount: 100, StageID: 2, Status: DealAberto, CreatedAt: dias(-29)},
		// Y: criado antes, saiu da etapa 1 dentro do período (15 dias).
		{ID: 2, Name: "Y", Amount: 100, StageID: 2, Status: DealAberto, CreatedAt: dias(-40)},
		// Z: saiu da etapa 1 antes do período: não conta na mediana nem na coorte.
		{ID: 3, Name: "Z", Amount: 100, StageID: 2, Status: DealAberto, CreatedAt: dias(-50)},
		// W: 1 dia na etapa 1.
		{ID: 4, Name: "W", Amount: 100, StageID: 2, Status: DealAberto, CreatedAt: dias(-29)},
		// Fechado antes do período: fora de tudo.
		{ID: 5, Name: "Velho", Amount: 100, StageID: 5, Status: DealPerdido, CreatedAt: dias(-90), ClosedAt: ptrTime(dias(-45))},
	}
	dados.Historico = append(dados.Historico, percorre(1, []int64{1, 2}, []time.Time{dias(-29), dias(-26)})...)
	dados.Historico = append(dados.Historico, percorre(2, []int64{1, 2}, []time.Time{dias(-40), dias(-25)})...)
	dados.Historico = append(dados.Historico, percorre(3, []int64{1, 2}, []time.Time{dias(-50), dias(-32)})...)
	dados.Historico = append(dados.Historico, percorre(4, []int64{1, 2}, []time.Time{dias(-29), dias(-28)})...)
	dados.Historico = append(dados.Historico, percorre(5, []int64{1, 5}, []time.Time{dias(-90), dias(-45)})...)

	out := calcularFunil(dados, filtroTeste(FunilModoFluxo))

	if out.Funil[0].MedianaDias != 3 {
		t.Fatalf("mediana da etapa 1 = %d, esperado 3 (amostra 1, 3, 15)", out.Funil[0].MedianaDias)
	}
	// Coorte do fluxo: X, Y, W (Z não se moveu no período; Velho está fora).
	if out.Funil[0].Alcancaram != 3 || out.Funil[1].Alcancaram != 3 {
		t.Fatalf("alcançaram em fluxo = %d/%d, esperado 3/3", out.Funil[0].Alcancaram, out.Funil[1].Alcancaram)
	}
	// Mas o estado atual conta todos os abertos, inclusive Z.
	if out.Resumo.Abertos != 4 || out.Funil[1].AbertosAgora != 4 || len(out.Negocios) != 4 {
		t.Fatalf("abertos agora = %d (etapa 2: %d, negócios: %d), esperado 4", out.Resumo.Abertos, out.Funil[1].AbertosAgora, len(out.Negocios))
	}
	// Z e Y existiam no início; X e W foram criados no período.
	if out.Movimentacao.Inicio.Qtd != 2 || out.Movimentacao.Criados.Qtd != 2 || out.Movimentacao.Fim.Qtd != 4 {
		t.Fatalf("movimentação errada: %+v", out.Movimentacao)
	}
}

// Motivos de risco: estagnado (prazo da etapa ou padrão), sem atividade,
// adiado e vencido; fechados não têm risco.
func TestCalcularFunilMotivosDeRisco(t *testing.T) {
	dados := &funilDados{Pipeline: funilPipelineTeste(), Metas: map[int64]float64{}}
	venceOntem := inicioDoDia(dias(-1))
	venceDepois := dias(10)
	dados.Deals = []funilDeal{
		// R1: 15 dias na Proposta (prazo 10), com atividade recente.
		{ID: 1, Name: "R1", Amount: 1000, StageID: 2, Status: DealAberto, CreatedAt: dias(-30), CloseDate: &venceDepois, LastActivityAt: ptrTime(dias(-2))},
		// R2: entrou há 3 dias, nunca teve atividade (criado há 20) e a previsão venceu.
		{ID: 2, Name: "R2", Amount: 2000, StageID: 2, Status: DealAberto, CreatedAt: dias(-20), CloseDate: &venceOntem},
		// R3: saudável, mas adiou a previsão no período.
		{ID: 3, Name: "R3", Amount: 3000, StageID: 1, Status: DealAberto, CreatedAt: dias(-5), CloseDate: &venceDepois, LastActivityAt: ptrTime(dias(0))},
		// R4: ganho, tudo velho: fechados não entram em risco.
		{ID: 4, Name: "R4", Amount: 4000, StageID: 4, Status: DealGanho, CreatedAt: dias(-60), ClosedAt: ptrTime(dias(-3))},
		// R5: 15 dias na Negociação (sem prazo: padrão 14), atividade ontem.
		{ID: 5, Name: "R5", Amount: 5000, StageID: 3, Status: DealAberto, CreatedAt: dias(-40), CloseDate: &venceDepois, LastActivityAt: ptrTime(dias(-1))},
		// R6: dentro do prazo, ativo, sem adiar, não vencido.
		{ID: 6, Name: "R6", Amount: 600, StageID: 2, Status: DealAberto, CreatedAt: dias(-4), CloseDate: &venceDepois, LastActivityAt: ptrTime(dias(-1))},
		// R7: histórico importado mais antigo que a criação: vale o histórico (12 dias > prazo 10).
		{ID: 7, Name: "R7", Amount: 700, StageID: 2, Status: DealAberto, CreatedAt: dias(-1), CloseDate: &venceDepois, LastActivityAt: ptrTime(dias(0))},
	}
	dados.Historico = append(dados.Historico, percorre(1, []int64{1, 2}, []time.Time{dias(-30), dias(-15)})...)
	dados.Historico = append(dados.Historico, percorre(2, []int64{1, 2}, []time.Time{dias(-20), dias(-3)})...)
	dados.Historico = append(dados.Historico, percorre(3, []int64{1}, []time.Time{dias(-5)})...)
	dados.Historico = append(dados.Historico, percorre(4, []int64{1, 4}, []time.Time{dias(-60), dias(-3)})...)
	dados.Historico = append(dados.Historico, percorre(5, []int64{1, 3}, []time.Time{dias(-40), dias(-15)})...)
	dados.Historico = append(dados.Historico, percorre(6, []int64{1, 2}, []time.Time{dias(-4), dias(-4)})...)
	dados.Historico = append(dados.Historico, percorre(7, []int64{2}, []time.Time{dias(-12)})...)
	dados.Mudancas = []funilChange{
		{DealID: 3, Campo: DealChangeCloseDate, De: "2026-10-01", Para: "2026-10-20", CreatedAt: dias(-4)},
		// Antecipar não é adiar.
		{DealID: 6, Campo: DealChangeCloseDate, De: "2026-11-20", Para: "2026-11-03", CreatedAt: dias(-2)},
	}

	out := calcularFunil(dados, filtroTeste(FunilModoFluxo))

	motivos := map[int64][]string{}
	for _, n := range out.Negocios {
		motivos[n.ID] = n.Motivos
	}
	casos := map[int64][]string{
		1: {RiscoEstagnado},
		2: {RiscoSemAtividade, RiscoVencido},
		3: {RiscoAdiado},
		4: {},
		5: {RiscoEstagnado},
		6: {},
		7: {RiscoEstagnado},
	}
	for id, want := range casos {
		if !equalStrings(motivos[id], want) {
			t.Errorf("negócio %d: motivos %v, esperado %v", id, motivos[id], want)
		}
	}
	if out.Resumo.EmRisco != 5 || !quase(out.Resumo.ValorEmRisco, 11700) {
		t.Fatalf("em risco = %d (%v), esperado 5 (11700)", out.Resumo.EmRisco, out.Resumo.ValorEmRisco)
	}
	if out.Funil[1].Estagnados != 2 || out.Funil[2].Estagnados != 1 || out.Funil[0].Estagnados != 0 {
		t.Fatalf("estagnados por etapa errados: %+v", out.Funil)
	}
	for _, n := range out.Negocios {
		switch n.ID {
		case 1:
			if n.DiasNaEtapa != 15 || n.DiasSemAtividade != 2 || n.PrazoDias == nil || *n.PrazoDias != 10 {
				t.Errorf("R1: %+v", n)
			}
		case 2:
			if n.DiasNaEtapa != 3 || n.DiasSemAtividade != 20 {
				t.Errorf("R2: %+v", n)
			}
		case 3:
			if n.Adiamentos != 1 || n.PrazoDias != nil {
				t.Errorf("R3: %+v", n)
			}
		}
	}
}

// O waterfall fecha a conta: inicio + criados + aumentos + reducoes + ganhos +
// perdidos = fim, com o valor do início reconstruído a partir de deal_changes.
func TestCalcularFunilWaterfallFechaAConta(t *testing.T) {
	dados := &funilDados{Pipeline: funilPipelineTeste(), Metas: map[int64]float64{}}
	dados.Deals = []funilDeal{
		// P1: existia; 1000 -> 1500 no período; aberto.
		{ID: 1, Name: "P1", Amount: 1500, StageID: 2, Status: DealAberto, CreatedAt: dias(-60)},
		// P2: existia; ganho no período.
		{ID: 2, Name: "P2", Amount: 800, StageID: 4, Status: DealGanho, CreatedAt: dias(-60), ClosedAt: ptrTime(dias(-10))},
		// P3: criado com 300, reduzido para 200, perdido.
		{ID: 3, Name: "P3", Amount: 200, StageID: 5, Status: DealPerdido, CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-5))},
		// P4: criado no período, aberto.
		{ID: 4, Name: "P4", Amount: 400, StageID: 1, Status: DealAberto, CreatedAt: dias(-7)},
		// P5: fechado antes do período: fora.
		{ID: 5, Name: "P5", Amount: 2000, StageID: 5, Status: DealPerdido, CreatedAt: dias(-90), ClosedAt: ptrTime(dias(-40))},
		// P6: existia valendo 500 no início (mudança anterior ao período ignorada), duas mudanças depois.
		{ID: 6, Name: "P6", Amount: 600, StageID: 3, Status: DealAberto, CreatedAt: dias(-70)},
		// P7: existia; adiou a previsão.
		{ID: 7, Name: "P7", Amount: 1000, StageID: 2, Status: DealAberto, CreatedAt: dias(-45)},
	}
	dados.Mudancas = []funilChange{
		{DealID: 1, Campo: DealChangeAmount, De: "1000", Para: "1500", CreatedAt: dias(-25)},
		{DealID: 3, Campo: DealChangeAmount, De: "300", Para: "200", CreatedAt: dias(-15)},
		{DealID: 6, Campo: DealChangeAmount, De: "400", Para: "500", CreatedAt: dias(-32)},
		{DealID: 6, Campo: DealChangeAmount, De: "500", Para: "700", CreatedAt: dias(-29)},
		{DealID: 6, Campo: DealChangeAmount, De: "700", Para: "600", CreatedAt: dias(-27)},
		{DealID: 7, Campo: DealChangeCloseDate, De: "2026-10-01", Para: "2026-10-15", CreatedAt: dias(-20)},
	}

	out := calcularFunil(dados, filtroTeste(FunilModoFluxo))
	m := out.Movimentacao

	check := func(nome string, got FunilBarra, qtd int, valor float64) {
		if got.Qtd != qtd || !quase(got.Valor, valor) {
			t.Errorf("%s = %+v, esperado {%d %v}", nome, got, qtd, valor)
		}
	}
	check("inicio", m.Inicio, 4, 3300)
	check("criados", m.Criados, 2, 700)
	check("aumentos", m.Aumentos, 2, 600)
	check("reducoes", m.Reducoes, 1, -100)
	check("adiados", m.Adiados, 1, 1000)
	check("ganhos", m.Ganhos, 1, -800)
	check("perdidos", m.Perdidos, 1, -200)
	check("fim", m.Fim, 4, 3500)

	soma := m.Inicio.Valor + m.Criados.Valor + m.Aumentos.Valor + m.Reducoes.Valor + m.Ganhos.Valor + m.Perdidos.Valor
	if !quase(soma, m.Fim.Valor) {
		t.Fatalf("a conta não fecha: %v != fim %v", soma, m.Fim.Valor)
	}
	if m.Inicio.Qtd+m.Criados.Qtd-m.Ganhos.Qtd-m.Perdidos.Qtd != m.Fim.Qtd {
		t.Fatalf("a quantidade não fecha: %+v", m)
	}
}

// Reconstrução do valor num instante: o "de" da primeira mudança a partir dali.
func TestValorEm(t *testing.T) {
	d := &funilDeal{ID: 1, Amount: 600}
	mudancas := []funilChange{
		{DealID: 1, Campo: DealChangeAmount, De: "400", Para: "500", CreatedAt: dias(-32)},
		{DealID: 1, Campo: DealChangeCloseDate, De: "2026-10-01", Para: "2026-10-10", CreatedAt: dias(-31)},
		{DealID: 1, Campo: DealChangeAmount, De: "500", Para: "700", CreatedAt: dias(-29)},
		{DealID: 1, Campo: DealChangeAmount, De: "700", Para: "600", CreatedAt: dias(-27)},
	}
	casos := []struct {
		quando time.Time
		want   float64
	}{
		{dias(-40), 400}, // antes de tudo
		{dias(-30), 500}, // entre a 1ª e a 2ª mudança de valor
		{dias(-28), 700},
		{dias(-26), 600}, // depois de todas: valor atual
	}
	for _, c := range casos {
		if got := valorEm(d, mudancas, c.quando); !quase(got, c.want) {
			t.Errorf("valorEm(%s) = %v, esperado %v", c.quando.Format(dataISO), got, c.want)
		}
	}
	if got := valorEm(d, nil, dias(-10)); !quase(got, 600) {
		t.Errorf("sem mudanças deveria devolver o valor atual, veio %v", got)
	}
}

// Meta proporcional aos dias quando o período não é o mês.
func TestMetaProporcional(t *testing.T) {
	// Outubro tem 31 dias; meta mensal 3100 => 100 por dia.
	casos := []struct {
		periodo string
		want    float64
	}{
		{"mes", 3100},
		{"30", 3000},
		{"7", 700},
		{"trimestre", 2400}, // 1/10 a 24/10 = 24 dias
		{"ano", 29700},      // 1/1 a 24/10 = 297 dias
	}
	for _, c := range casos {
		de, _, err := periodoFunil(c.periodo, funilAgora)
		if err != nil {
			t.Fatal(err)
		}
		if got := metaProporcional(3100, c.periodo, de, funilAgora); !quase(got, c.want) {
			t.Errorf("meta(%s) = %v, esperado %v", c.periodo, got, c.want)
		}
	}
	if got := metaProporcional(0, "30", dias(-30), funilAgora); got != 0 {
		t.Errorf("sem meta deveria dar 0, veio %v", got)
	}
}

// Resumo: meta da equipe (ou do dono filtrado), gap e cobertura; por dono com
// a meta individual; semanas ISO do período com os ganhos.
func TestCalcularFunilMetaCoberturaEDonos(t *testing.T) {
	ana, bia := int64(3), int64(4)
	dados := &funilDados{
		Pipeline:   funilPipelineTeste(),
		Metas:      map[int64]float64{ana: 31000, bia: 15500},
		MetaEquipe: 62000,
	}
	dados.Deals = []funilDeal{
		{ID: 1, Name: "Ana aberto", Amount: 20000, StageID: 2, Status: DealAberto, OwnerID: &ana, OwnerName: "Ana", CreatedAt: dias(-10)},
		{ID: 2, Name: "Ana ganho", Amount: 10000, StageID: 4, Status: DealGanho, OwnerID: &ana, OwnerName: "Ana", CreatedAt: dias(-20), ClosedAt: ptrTime(time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC))},
		{ID: 3, Name: "Bia perdido", Amount: 5000, StageID: 5, Status: DealPerdido, OwnerID: &bia, OwnerName: "Bia", CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-2))},
		{ID: 4, Name: "Sem dono", Amount: 1000, StageID: 1, Status: DealAberto, CreatedAt: dias(-1)},
	}
	for _, d := range dados.Deals {
		dados.Historico = append(dados.Historico, funilHist{DealID: d.ID, StageID: 1, EnteredAt: d.CreatedAt})
	}

	out := calcularFunil(dados, filtroTeste(FunilModoFluxo))

	// Meta da equipe: 62000/31 × 30 = 60000; gap = 60000 − 10000; cobertura = ponderado / gap.
	if !quase(out.Resumo.Meta, 60000) || !quase(out.Resumo.Gap, 50000) {
		t.Fatalf("meta/gap = %v/%v", out.Resumo.Meta, out.Resumo.Gap)
	}
	ponderado := 20000*0.5 + 1000*0.1
	if !quase(out.Resumo.Ponderado, ponderado) || !quase(out.Resumo.Cobertura, ponderado/50000) {
		t.Fatalf("ponderado/cobertura = %v/%v", out.Resumo.Ponderado, out.Resumo.Cobertura)
	}

	if len(out.PorDono) != 3 || out.PorDono[0].Nome != "Ana" {
		t.Fatalf("por dono: %+v", out.PorDono)
	}
	anaRow := out.PorDono[0]
	if anaRow.Ganhos != 1 || !quase(anaRow.ValorGanho, 10000) || anaRow.Abertos != 1 || !quase(anaRow.Meta, 30000) || !quase(anaRow.TaxaGanho, 1) {
		t.Fatalf("linha da Ana: %+v", anaRow)
	}
	var semDono *FunilDono
	for i := range out.PorDono {
		if out.PorDono[i].OwnerID == nil {
			semDono = &out.PorDono[i]
		}
	}
	if semDono == nil || semDono.Nome != "Sem dono" || semDono.Meta != 0 {
		t.Fatalf("linha sem dono: %+v", semDono)
	}

	// Semanas: segundas de 21/09 a 19/10; o ganho de 20/10 cai na última.
	if len(out.GanhosPorSemana) != 5 || out.GanhosPorSemana[0].Semana != "2026-09-21" || out.GanhosPorSemana[4].Semana != "2026-10-19" {
		t.Fatalf("semanas: %+v", out.GanhosPorSemana)
	}
	if out.GanhosPorSemana[4].Qtd != 1 || !quase(out.GanhosPorSemana[4].Valor, 10000) || out.GanhosPorSemana[3].Qtd != 0 {
		t.Fatalf("ganhos por semana: %+v", out.GanhosPorSemana)
	}

	// Filtrando por dono, a meta é a dele.
	f := filtroTeste(FunilModoFluxo)
	f.OwnerID = &ana
	out = calcularFunil(dados, f)
	if !quase(out.Resumo.Meta, 30000) {
		t.Fatalf("meta da Ana = %v, esperado 30000", out.Resumo.Meta)
	}

	// Sem meta cadastrada: meta, gap e cobertura zerados.
	dados.MetaEquipe = 0
	out = calcularFunil(dados, filtroTeste(FunilModoFluxo))
	if out.Resumo.Meta != 0 || out.Resumo.Gap != 0 || out.Resumo.Cobertura != 0 {
		t.Fatalf("sem meta deveria zerar: %+v", out.Resumo)
	}
}

// variacao_7d só aparece com mudança nos últimos 7 dias, e resume valor,
// deslocamento da previsão e a troca de etapa.
func TestCalcularFunilVariacao7d(t *testing.T) {
	dados := &funilDados{Pipeline: funilPipelineTeste(), Metas: map[int64]float64{}}
	dados.Deals = []funilDeal{
		{ID: 1, Name: "Recente", Amount: 3000, StageID: 3, Status: DealAberto, CreatedAt: dias(-20)},
		{ID: 2, Name: "Antigo", Amount: 3000, StageID: 2, Status: DealAberto, CreatedAt: dias(-20)},
	}
	dados.Mudancas = []funilChange{
		{DealID: 1, Campo: DealChangeAmount, De: "5000", Para: "3000", CreatedAt: dias(-3)},
		{DealID: 1, Campo: DealChangeCloseDate, De: "2026-11-01", Para: "2026-11-15", CreatedAt: dias(-2)},
		{DealID: 1, Campo: DealChangeStage, De: "2", Para: "3", CreatedAt: dias(-1)},
		{DealID: 1, Campo: DealChangeStatus, De: "aberto", Para: "aberto", CreatedAt: dias(-1)},
		{DealID: 2, Campo: DealChangeAmount, De: "5000", Para: "3000", CreatedAt: dias(-10)},
	}

	out := calcularFunil(dados, filtroTeste(FunilModoFluxo))
	for _, n := range out.Negocios {
		switch n.ID {
		case 1:
			if n.Variacao7d == nil {
				t.Fatal("negócio recente deveria ter variacao_7d")
			}
			if !quase(n.Variacao7d.Valor, -2000) || n.Variacao7d.CloseDateDias != 14 || n.Variacao7d.Etapa != "Proposta→Negociação" {
				t.Fatalf("variação: %+v", *n.Variacao7d)
			}
			if n.Adiamentos != 1 {
				t.Fatalf("adiamentos = %d", n.Adiamentos)
			}
		case 2:
			if n.Variacao7d != nil {
				t.Fatalf("mudança de 10 dias atrás não é variação recente: %+v", *n.Variacao7d)
			}
		}
	}
}

func TestPeriodoFunil(t *testing.T) {
	casos := []struct {
		periodo, de, rotulo string
	}{
		{"7", "2026-10-17", "Últimos 7 dias"},
		{"90", "2026-07-26", "Últimos 90 dias"},
		{"mes", "2026-10-01", "Este mês"},
		{"trimestre", "2026-10-01", "Este trimestre"},
		{"ano", "2026-01-01", "Este ano"},
	}
	for _, c := range casos {
		de, rotulo, err := periodoFunil(c.periodo, funilAgora)
		if err != nil {
			t.Fatal(err)
		}
		if de.Format(dataISO) != c.de || rotulo != c.rotulo {
			t.Errorf("periodo %s: %s %q, esperado %s %q", c.periodo, de.Format(dataISO), rotulo, c.de, c.rotulo)
		}
	}
	if _, _, err := periodoFunil("15", funilAgora); err != ErrFunilPeriodoInvalido {
		t.Fatalf("período inválido deveria ser recusado, veio %v", err)
	}
	// Agosto está no 3º trimestre.
	de, _, _ := periodoFunil("trimestre", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	if de.Format(dataISO) != "2026-07-01" {
		t.Fatalf("trimestre de agosto começa em 01/07, veio %s", de.Format(dataISO))
	}
}

func TestMediana(t *testing.T) {
	if mediana(nil) != 0 || mediana([]float64{4}) != 4 || mediana([]float64{1, 3, 15}) != 3 || mediana([]float64{3, 15}) != 9 {
		t.Fatal("mediana errada")
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
