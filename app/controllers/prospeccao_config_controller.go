package controllers

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Configurações de prospecção (janela de contato, cadência, meta, destino da
// conversão, roteiro padrão, motivos de descarte). Contrato: docs/PROSPECCAO.md
// seção 8.4. Guardadas em app_settings na chave "prospeccao".

// GetConfigProspeccao: GET /settings/prospeccao → ConfigProspeccao (qualquer um com leads.view).
func GetConfigProspeccao(ctx iris.Context) {
	ctx.JSON(models.LoadConfigProspeccao(utils.DB))
}

// UpdateConfigProspeccao: PUT /settings/prospeccao (prospecting.manage). O
// corpo é aplicado sobre a configuração atual: chave ausente mantém o valor.
func UpdateConfigProspeccao(ctx iris.Context) {
	cfg := models.LoadConfigProspeccao(utils.DB)
	corpo, err := io.ReadAll(io.LimitReader(ctx.Request().Body, 1<<20))
	if err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	dec := json.NewDecoder(bytes.NewReader(corpo))
	if err := dec.Decode(&cfg); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	if msg := validarDestinoProspeccao(&cfg); msg != "" {
		unprocessable(ctx, msg)
		return
	}
	userID := middlewareClaims(ctx).UserID
	if err := models.SaveConfigProspeccao(utils.DB, &cfg, &userID); err != nil {
		// Validate devolve mensagens prontas; o resto é erro de banco.
		if cfg.Validate() != nil {
			unprocessable(ctx, err.Error())
			return
		}
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "configuracao", 0, "alterou as configurações de prospecção")
	ctx.JSON(models.LoadConfigProspeccao(utils.DB))
}

// validarDestinoProspeccao confere funil, etapa e roteiro: a etapa precisa ser
// do funil e não pode ser de ganho nem de perda; o roteiro precisa existir.
func validarDestinoProspeccao(c *models.ConfigProspeccao) string {
	if c.PipelineID != nil && *c.PipelineID <= 0 {
		c.PipelineID = nil
	}
	if c.StageID != nil && *c.StageID <= 0 {
		c.StageID = nil
	}
	if c.PlaybookID != nil && *c.PlaybookID <= 0 {
		c.PlaybookID = nil
	}
	if c.PipelineID != nil {
		var existe bool
		if err := utils.DB.QueryRow(`SELECT EXISTS (SELECT 1 FROM pipelines WHERE id = $1)`, *c.PipelineID).Scan(&existe); err != nil || !existe {
			return "funil de destino não encontrado"
		}
	}
	if c.StageID != nil {
		if c.PipelineID == nil {
			return "informe o funil da etapa de destino"
		}
		st, err := models.StageByID(utils.DB, *c.StageID)
		if err != nil || st.PipelineID != *c.PipelineID {
			return "a etapa de destino não pertence ao funil escolhido"
		}
		if st.IsWon || st.IsLost {
			return "a etapa de destino não pode ser de ganho nem de perda"
		}
	}
	if c.PlaybookID != nil {
		if _, err := models.PlaybookByID(utils.DB, *c.PlaybookID); err != nil {
			return "roteiro (manual de atividades) não encontrado"
		}
	}
	return ""
}
