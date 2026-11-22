package models

import "testing"

// perdas_por_motivo: só os perdidos no período, agrupados pelo motivo (o de
// mais negócios primeiro); motivo vazio vira MotivoPerdaVazio.
func TestCalcularFunilPerdasPorMotivo(t *testing.T) {
	dados := &funilDados{Pipeline: funilPipelineTeste(), Metas: map[int64]float64{}}
	dados.Deals = []funilDeal{
		{ID: 1, Amount: 100, StageID: 5, Status: DealPerdido, CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-2)), LostReason: "Preço"},
		{ID: 2, Amount: 300, StageID: 5, Status: DealPerdido, CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-3)), LostReason: "Preço"},
		{ID: 3, Amount: 50, StageID: 5, Status: DealPerdido, CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-4)), LostReason: ""},
		{ID: 4, Amount: 70, StageID: 5, Status: DealPerdido, CreatedAt: dias(-20), ClosedAt: ptrTime(dias(-5)), LostReason: "Sem orçamento"},
		// Perdido antes do período: fora.
		{ID: 5, Amount: 999, StageID: 5, Status: DealPerdido, CreatedAt: dias(-90), ClosedAt: ptrTime(dias(-60)), LostReason: "Preço"},
		// Aberto: fora.
		{ID: 6, Amount: 10, StageID: 2, Status: DealAberto, CreatedAt: dias(-5)},
	}
	for _, d := range dados.Deals {
		dados.Historico = append(dados.Historico, funilHist{DealID: d.ID, StageID: 1, EnteredAt: d.CreatedAt})
	}
	out := calcularFunil(dados, filtroTeste(FunilModoFluxo))
	got := out.PerdasPorMotivo
	if len(got) != 3 {
		t.Fatalf("perdas_por_motivo = %+v", got)
	}
	if got[0].Motivo != "Preço" || got[0].Qtd != 2 || !quase(got[0].Valor, 400) {
		t.Fatalf("primeira linha errada: %+v", got[0])
	}
	if got[1].Motivo != "Sem orçamento" || got[1].Qtd != 1 || got[2].Motivo != MotivoPerdaVazio || !quase(got[2].Valor, 50) {
		t.Fatalf("ordem ou rótulo do motivo vazio errados: %+v", got)
	}
}
