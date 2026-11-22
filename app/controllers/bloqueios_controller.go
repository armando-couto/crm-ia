package controllers

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Bloqueios ("não contatar") por telefone, e-mail e CNPJ. Contrato:
// docs/PROSPECCAO.md seção 8.3, com as correções de docs/PROSPECCAO-ADENDO.md.
// Leitura: prospecting.view ou leads.view; escrita: prospecting.manage.
//
// Efeito por tipo (adendo): CNPJ barra a empresa inteira (pulada ao gerar a
// lista, item "bloqueado" no envio, lead fora da fila com alerta); telefone e
// e-mail só tiram aquele contato (no envio não entram no lead nem viram
// contato; na ficha e na fila o telefone vem com "invalido": true). O
// bloqueio sobrevive à exclusão do lead.

const entidadeBloqueio = "bloqueio"

// ListBloqueios: GET /bloqueios → { data, pagination }.
func ListBloqueios(ctx iris.Context) {
	f := models.BloqueioFilter{
		Q:    strings.TrimSpace(ctx.URLParam("q")),
		Tipo: strings.TrimSpace(ctx.URLParam("tipo")),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	if f.Tipo != "" && !models.BloqueioTipoValido(f.Tipo) {
		unprocessable(ctx, "tipo de bloqueio inválido (telefone, email ou cnpj)")
		return
	}
	lista, total, err := models.ListBloqueios(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": lista, "pagination": f.Pagination})
}

type bloqueioRequest struct {
	Tipo       string `json:"tipo"`
	Valor      string `json:"valor"`
	Motivo     string `json:"motivo"`
	Observacao string `json:"observacao"`
}

// CreateBloqueio: POST /bloqueios { tipo, valor, motivo, observacao? } → 201
// Bloqueio. Valor que já estava bloqueado responde 200 com o registro existente.
func CreateBloqueio(ctx iris.Context) {
	var req bloqueioRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	req.Tipo, req.Motivo = strings.TrimSpace(req.Tipo), strings.TrimSpace(req.Motivo)
	if !models.BloqueioTipoValido(req.Tipo) {
		unprocessable(ctx, "tipo de bloqueio inválido (telefone, email ou cnpj)")
		return
	}
	if req.Motivo != "" && !models.BloqueioMotivoValido(req.Motivo) {
		unprocessable(ctx, "motivo de bloqueio inválido")
		return
	}
	if len(req.Observacao) > 2000 {
		unprocessable(ctx, "a observação aceita até 2.000 caracteres")
		return
	}
	userID := middlewareClaims(ctx).UserID
	b := &models.Bloqueio{Tipo: req.Tipo, Valor: req.Valor, Motivo: req.Motivo, Canal: "manual",
		Observacao: req.Observacao, CriadoPor: &userID}
	if _, err := models.NormalizarValorBloqueio(req.Tipo, req.Valor); err != nil {
		unprocessable(ctx, err.Error())
		return
	}
	criado, err := models.CreateBloqueio(utils.DB, b)
	if err != nil {
		serverError(ctx, err)
		return
	}
	completo, err := models.BloqueioByID(utils.DB, b.ID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if !criado {
		ctx.JSON(completo)
		return
	}
	audit(ctx, models.AuditCreate, entidadeBloqueio, b.ID,
		fmt.Sprintf("bloqueou %s %s (%s)", completo.Tipo, completo.Valor, completo.Motivo))
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(completo)
}

type importarBloqueiosRequest struct {
	Tipo    string   `json:"tipo"`
	Motivo  string   `json:"motivo"`
	Valores []string `json:"valores"`
}

// ImportarBloqueios: POST /bloqueios/importar { tipo, motivo, valores } → { criados, repetidos, invalidos }.
func ImportarBloqueios(ctx iris.Context) {
	var req importarBloqueiosRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	req.Tipo, req.Motivo = strings.TrimSpace(req.Tipo), strings.TrimSpace(req.Motivo)
	if req.Motivo == "" {
		req.Motivo = models.BloqueioListaExterna
	}
	switch {
	case !models.BloqueioTipoValido(req.Tipo):
		unprocessable(ctx, "tipo de bloqueio inválido (telefone, email ou cnpj)")
		return
	case !models.BloqueioMotivoValido(req.Motivo):
		unprocessable(ctx, "motivo de bloqueio inválido")
		return
	case len(req.Valores) == 0:
		unprocessable(ctx, "informe os valores a bloquear")
		return
	case len(req.Valores) > models.MaxBloqueiosImportacao:
		unprocessable(ctx, fmt.Sprintf("a importação aceita até %d valores por vez", models.MaxBloqueiosImportacao))
		return
	}
	userID := middlewareClaims(ctx).UserID
	criados, repetidos, invalidos, err := models.ImportarBloqueios(utils.DB, req.Tipo, req.Motivo, req.Valores, &userID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditImport, entidadeBloqueio, 0,
		fmt.Sprintf("importou bloqueios de %s (%s): %d novos, %d repetidos, %d inválidos", req.Tipo, req.Motivo, criados, repetidos, invalidos))
	ctx.JSON(iris.Map{"criados": criados, "repetidos": repetidos, "invalidos": invalidos})
}

// DeleteBloqueio: DELETE /bloqueios/{id} → 204.
func DeleteBloqueio(ctx iris.Context) {
	b, err := models.DeleteBloqueio(utils.DB, paramID(ctx))
	if err != nil {
		if err == sql.ErrNoRows {
			notFound(ctx)
			return
		}
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, entidadeBloqueio, b.ID, fmt.Sprintf("removeu o bloqueio de %s %s", b.Tipo, b.Valor))
	ctx.StatusCode(iris.StatusNoContent)
}
