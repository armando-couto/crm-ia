package controllers

import (
	"errors"
	"fmt"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Prospecção: enriquecimento em lote pelo Who Lead. Contrato:
// docs/PROSPECCAO.md seção 7, com as correções de docs/PROSPECCAO-ADENDO.md.
// O handler só confere a estimativa e insere o job "enriquecer"; o trabalho é
// do worker (services/prospeccao_enriquecer.go).

type enriquecerProspRequest struct {
	Selecao                 models.ProspSelecao `json:"selecao"`
	Reprocessar             bool                `json:"reprocessar"`
	CustoConfirmadoCentavos *int                `json:"custo_confirmado_centavos"`
}

// enriqLerPedido carrega a lista (404) e lê e valida a seleção (422).
func enriqLerPedido(ctx iris.Context) (*models.ProspLista, *enriquecerProspRequest, bool) {
	lista, err := models.ProspListaByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return nil, nil, false
	}
	var req enriquecerProspRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return nil, nil, false
	}
	if err := req.Selecao.Validar(); err != nil {
		unprocessable(ctx, err.Error())
		return nil, nil, false
	}
	return lista, &req, true
}

// enriqResponderJobAtivo responde 409 com o job em andamento na lista.
func enriqResponderJobAtivo(ctx iris.Context, listaID int64) {
	corpo := iris.Map{"error": models.ErrJobAtivo.Error(), "codigo": services.CodigoJobAtivo}
	if j, err := models.ProspJobAtivoDaLista(utils.DB, listaID); err == nil && j != nil {
		corpo["job"] = j
	}
	ctx.StopWithJSON(iris.StatusConflict, corpo)
}

// EnriquecerProspEstimativa: POST /prospeccao/listas/{id}/enriquecer/estimativa { selecao, reprocessar? } → EnriquecerEstimativa.
func EnriquecerProspEstimativa(ctx iris.Context) {
	lista, req, ok := enriqLerPedido(ctx)
	if !ok {
		return
	}
	est, err := services.EstimarEnriquecimento(ctx.Request().Context(), utils.DB, lista.ID, req.Selecao, req.Reprocessar, true)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(est)
}

// EnriquecerProspLista: POST /prospeccao/listas/{id}/enriquecer { selecao, reprocessar?, custo_confirmado_centavos } → 202 { job }.
func EnriquecerProspLista(ctx iris.Context) {
	lista, req, ok := enriqLerPedido(ctx)
	if !ok {
		return
	}
	if j, err := models.ProspJobAtivoDaLista(utils.DB, lista.ID); err != nil {
		serverError(ctx, err)
		return
	} else if j != nil {
		enriqResponderJobAtivo(ctx, lista.ID)
		return
	}
	// O saldo do provedor não muda a decisão aqui: a estimativa sai sem ele.
	est, err := services.EstimarEnriquecimento(ctx.Request().Context(), utils.DB, lista.ID, req.Selecao, req.Reprocessar, false)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if !est.Provedor.Pronto {
		unprocessable(ctx, est.Provedor.Motivo)
		return
	}
	if est.AEnriquecer == 0 {
		unprocessable(ctx, "nenhuma empresa da seleção precisa ser enriquecida")
		return
	}
	confirmado := 0
	if req.CustoConfirmadoCentavos != nil {
		confirmado = *req.CustoConfirmadoCentavos
	}
	if confirmado < est.CustoEstimadoCentavos {
		ctx.StopWithJSON(iris.StatusConflict, iris.Map{
			"error":      "o custo estimado mudou; confira a nova estimativa e confirme de novo",
			"codigo":     services.CodigoCustoMudou,
			"estimativa": est,
		})
		return
	}
	if est.EstourariaLimite() {
		ctx.StopWithJSON(iris.StatusTooManyRequests, iris.Map{
			"error": fmt.Sprintf("o enriquecimento passaria do limite mensal de consultas pagas (R$ %.2f usados de R$ %.2f; estimativa R$ %.2f). Ajuste em Configurações → Integrações.",
				float64(est.UsoMes.CustoCentavos)/100, float64(est.UsoMes.LimiteCentavos)/100, float64(est.CustoEstimadoCentavos)/100),
			"codigo":  "limite_mensal",
			"uso_mes": est.UsoMes,
		})
		return
	}
	job, err := services.CriarJobEnriquecer(utils.DB, lista.ID, est, req.Selecao, req.Reprocessar, userIDPtr(ctx))
	if errors.Is(err, models.ErrJobAtivo) {
		enriqResponderJobAtivo(ctx, lista.ID)
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	resumo := fmt.Sprintf("pediu o enriquecimento de %d empresas da lista %q pelo %s (estimativa R$ %.2f)",
		est.AEnriquecer, lista.Nome, est.Provedor.Nome, float64(est.CustoEstimadoCentavos)/100)
	if est.Provedor.Demo {
		resumo += " — demonstração"
	}
	audit(ctx, models.AuditUpdate, "lista_prospeccao", lista.ID, enriquecimento.TruncarTexto(resumo, 300))
	ctx.StopWithJSON(iris.StatusAccepted, iris.Map{"job": job})
}
