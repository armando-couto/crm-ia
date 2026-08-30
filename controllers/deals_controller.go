package controllers

import (
	"fmt"
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

func ListDeals(ctx iris.Context) {
	f := models.DealFilter{
		Search:     ctx.URLParam("q"),
		PipelineID: ctx.URLParamInt64Default("pipeline_id", 0),
		StageID:    ctx.URLParamInt64Default("stage_id", 0),
		OwnerID:    ctx.URLParamInt64Default("owner_id", 0),
		Status:     ctx.URLParam("status"),
		ContactID:  ctx.URLParamInt64Default("contact_id", 0),
		CompanyID:  ctx.URLParamInt64Default("company_id", 0),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	list, total, err := models.ListDeals(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

// DealsBoard retorna o kanban (negócios abertos do pipeline agrupados no front).
func DealsBoard(ctx iris.Context) {
	pipelineID := ctx.URLParamInt64Default("pipeline_id", 0)
	if pipelineID == 0 {
		badRequest(ctx, "informe o pipeline_id")
		return
	}
	deals, err := models.BoardDeals(utils.DB, pipelineID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": deals})
}

func GetDeal(ctx iris.Context) {
	deal, err := models.DealByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(deal)
}

type dealRequest struct {
	Name       string  `json:"name"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	PipelineID int64   `json:"pipeline_id"`
	StageID    int64   `json:"stage_id"`
	ContactID  *int64  `json:"contact_id"`
	CompanyID  *int64  `json:"company_id"`
	OwnerID    *int64  `json:"owner_id"`
	CloseDate  string  `json:"close_date"` // YYYY-MM-DD
}

func (r *dealRequest) validate(creating bool) string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome do negócio"
	}
	if r.Amount < 0 {
		return "o valor não pode ser negativo"
	}
	if creating && (r.PipelineID == 0 || r.StageID == 0) {
		return "informe pipeline e etapa"
	}
	return ""
}

func (r *dealRequest) closeDate() (*time.Time, string) {
	if r.CloseDate == "" {
		return nil, ""
	}
	t, err := time.Parse("2006-01-02", r.CloseDate)
	if err != nil {
		return nil, "data de fechamento inválida (use AAAA-MM-DD)"
	}
	return &t, ""
}

func CreateDeal(ctx iris.Context) {
	var req dealRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(true); msg != "" {
		badRequest(ctx, msg)
		return
	}
	closeDate, msg := req.closeDate()
	if msg != "" {
		badRequest(ctx, msg)
		return
	}

	stage, err := models.StageByID(utils.DB, req.StageID)
	if err != nil {
		badRequest(ctx, "etapa não encontrada")
		return
	}
	if stage.PipelineID != req.PipelineID {
		badRequest(ctx, "a etapa não pertence ao pipeline informado")
		return
	}

	deal := &models.Deal{
		Name:       req.Name,
		Amount:     req.Amount,
		Currency:   req.Currency,
		PipelineID: req.PipelineID,
		StageID:    req.StageID,
		ContactID:  req.ContactID,
		CompanyID:  req.CompanyID,
		OwnerID:    req.OwnerID,
		CloseDate:  closeDate,
	}
	if deal.OwnerID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			deal.OwnerID = &claims.UserID
		}
	}
	if err := models.CreateDeal(utils.DB, deal); err != nil {
		serverError(ctx, err)
		return
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   fmt.Sprintf("Negócio criado na etapa %s", stage.Name),
		DealID:    &deal.ID,
		ContactID: deal.ContactID,
		CompanyID: deal.CompanyID,
	})
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(deal)
}

func UpdateDeal(ctx iris.Context) {
	deal, err := models.DealByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req dealRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(false); msg != "" {
		badRequest(ctx, msg)
		return
	}
	closeDate, msg := req.closeDate()
	if msg != "" {
		badRequest(ctx, msg)
		return
	}

	deal.Name = req.Name
	deal.Amount = req.Amount
	if req.Currency != "" {
		deal.Currency = req.Currency
	}
	deal.ContactID = req.ContactID
	deal.CompanyID = req.CompanyID
	deal.OwnerID = req.OwnerID
	deal.CloseDate = closeDate
	if err := models.UpdateDeal(utils.DB, deal); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(deal)
}

// MoveDeal troca a etapa/posição do negócio (drag & drop do kanban).
func MoveDeal(ctx iris.Context) {
	var req struct {
		StageID  int64 `json:"stage_id"`
		Position int   `json:"position"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.StageID == 0 {
		badRequest(ctx, "informe a etapa de destino")
		return
	}

	id := paramID(ctx)
	deal, err := models.DealByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	stage, err := models.StageByID(utils.DB, req.StageID)
	if err != nil {
		badRequest(ctx, "etapa não encontrada")
		return
	}
	if stage.PipelineID != deal.PipelineID {
		badRequest(ctx, "a etapa não pertence ao pipeline do negócio")
		return
	}

	if err := models.MoveDealStage(utils.DB, id, req.StageID, req.Position); err != nil {
		serverError(ctx, err)
		return
	}
	if deal.StageID != req.StageID {
		logActivity(ctx, models.Activity{
			Kind:      models.ActivitySistema,
			Content:   fmt.Sprintf("Negócio movido de %s para %s", deal.StageName, stage.Name),
			DealID:    &deal.ID,
			ContactID: deal.ContactID,
			CompanyID: deal.CompanyID,
		})
	}

	updated, err := models.DealByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(updated)
}

// CloseDealHandler fecha o negócio como ganho ou perdido.
func CloseDealHandler(ctx iris.Context) {
	var req struct {
		Won bool `json:"won"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	id := paramID(ctx)
	if err := models.CloseDeal(utils.DB, id, req.Won); err != nil {
		handleDBError(ctx, err)
		return
	}

	deal, err := models.DealByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	result := "perdido"
	if req.Won {
		result = "ganho"
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   fmt.Sprintf("Negócio marcado como %s", result),
		DealID:    &deal.ID,
		ContactID: deal.ContactID,
		CompanyID: deal.CompanyID,
	})
	ctx.JSON(deal)
}

func DeleteDeal(ctx iris.Context) {
	if err := models.DeleteDeal(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "negócio removido"})
}
