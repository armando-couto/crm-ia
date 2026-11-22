package controllers

import (
	"database/sql"
	"errors"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Prospecção: envio dos itens ao CRM (empresa, contatos e lead, com
// distribuição entre vendedores). Contrato: docs/PROSPECCAO.md seção 8.1, com
// as correções de docs/PROSPECCAO-ADENDO.md. O handler só confere o pedido e
// cria o job "enviar_crm"; o trabalho é do worker (services/prospeccao_envio_job.go).

// lerPedidoEnvio lê a lista e o corpo e normaliza o pedido. Responde o erro e
// devolve nil quando algo não confere.
func lerPedidoEnvio(ctx iris.Context) (*models.ProspLista, *services.EnvioRequest) {
	lista, err := models.ProspListaByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return nil, nil
	}
	var req services.EnvioRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return nil, nil
	}
	if err := req.Normalizar(utils.DB); err != nil {
		if e, ok := services.ComoErroEnvio(err); ok {
			unprocessable(ctx, e.Mensagem)
			return nil, nil
		}
		serverError(ctx, err)
		return nil, nil
	}
	// Entregar leads a outra pessoa é distribuir: exige leads.assign.
	claims := middlewareClaims(ctx)
	if !models.RoleCan(claims.Role, models.PermLeadsAssign) {
		for _, u := range req.Distribuicao.Usuarios {
			if u != claims.UserID {
				ctx.StopWithJSON(iris.StatusForbidden, iris.Map{"error": "você só pode enviar leads para você mesmo"})
				return nil, nil
			}
		}
	}
	return lista, &req
}

// responderEnvioJobAtivo devolve 409 {error, codigo: "job_ativo", job}.
func responderEnvioJobAtivo(ctx iris.Context, listaID int64) {
	job, _ := models.ProspJobAtivoDaLista(utils.DB, listaID)
	ctx.StopWithJSON(iris.StatusConflict, iris.Map{
		"error":  "já existe um processamento em andamento nesta lista",
		"codigo": services.CodigoJobAtivo,
		"job":    job,
	})
}

// EnviarProspPrevia: POST /prospeccao/listas/{id}/enviar/previa (EnviarRequest) → EnviarPrevia.
func EnviarProspPrevia(ctx iris.Context) {
	lista, req := lerPedidoEnvio(ctx)
	if lista == nil {
		return
	}
	previa, err := services.PreviaEnvio(utils.DB, lista.ID, *req)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(previa)
}

// EnviarProspLista: POST /prospeccao/listas/{id}/enviar (EnviarRequest) → 202 { job }.
func EnviarProspLista(ctx iris.Context) {
	lista, req := lerPedidoEnvio(ctx)
	if lista == nil {
		return
	}
	if lista.Status == models.ListaGerando {
		responderEnvioJobAtivo(ctx, lista.ID)
		return
	}
	n, err := models.CountProspSelecao(utils.DB, lista.ID, req.Selecao, models.ItemEstadosAptosEnvio)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if n == 0 {
		unprocessable(ctx, "nenhum item da seleção pode ser enviado (os aptos são os novos, enriquecidos e sem resultado)")
		return
	}
	userID := middlewareClaims(ctx).UserID
	job, err := services.EnfileirarEnvio(utils.DB, lista.ID, *req, &userID)
	if errors.Is(err, models.ErrJobAtivo) {
		responderEnvioJobAtivo(ctx, lista.ID)
		return
	}
	if err != nil {
		if err == sql.ErrNoRows {
			notFound(ctx)
			return
		}
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusAccepted)
	ctx.JSON(iris.Map{"job": job})
}
