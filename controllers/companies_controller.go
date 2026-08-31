package controllers

import (
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

func ListCompanies(ctx iris.Context) {
	f := models.CompanyFilter{
		Search:      ctx.URLParam("q"),
		OwnerID:     ctx.URLParamInt64Default("owner_id", 0),
		Industry:    ctx.URLParam("industry"),
		Unassigned:  ctx.URLParamBoolDefault("sem_dono", false),
		CreatedDays: ctx.URLParamIntDefault("criado_dias", 0),
		SortBy:      ctx.URLParam("sort"),
		SortDir:     ctx.URLParam("dir"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	adv, err := models.ParseAdvancedFilters(ctx.URLParam("af"), models.CompanyFilterFieldsSpec)
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	f.Advanced = adv
	list, total, err := models.ListCompanies(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

func GetCompany(ctx iris.Context) {
	company, err := models.CompanyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(company)
}

type companyRequest struct {
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Phone    string `json:"phone"`
	Industry string `json:"industry"`
	City     string `json:"city"`
	State    string `json:"state"`
	OwnerID  *int64 `json:"owner_id"`

	ECNumber         string   `json:"ec_number"`
	EconomicGroup    string   `json:"economic_group"`
	CNPJ             string   `json:"cnpj"`
	AccreditedAt     string   `json:"accredited_at"` // AAAA-MM-DD
	Representative   string   `json:"representative"`
	Instagram        string   `json:"instagram"`
	Products         []string `json:"products"`
	MachinesCount    int      `json:"machines_count"`
	IsClient         bool     `json:"is_client"`
	AnticipationMode string   `json:"anticipation_mode"`
	Validator        bool     `json:"validator"`
	DoNotDisturb     bool     `json:"do_not_disturb"`
}

func (r *companyRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome da empresa"
	}
	if r.MachinesCount < 0 {
		return "quantidade de máquinas não pode ser negativa"
	}
	if r.AnticipationMode != "" && r.AnticipationMode != "pontual" && r.AnticipationMode != "automatica" && r.AnticipationMode != "nenhuma" {
		return "modalidade de antecipação inválida (pontual, automatica ou nenhuma)"
	}
	return ""
}

func (r *companyRequest) apply(c *models.Company) string {
	c.Name = r.Name
	c.Domain = strings.TrimSpace(strings.ToLower(r.Domain))
	c.Phone = r.Phone
	c.Industry = r.Industry
	c.City = r.City
	c.State = r.State
	c.OwnerID = r.OwnerID

	c.ECNumber = strings.TrimSpace(r.ECNumber)
	c.EconomicGroup = strings.TrimSpace(r.EconomicGroup)
	c.CNPJ = strings.TrimSpace(r.CNPJ)
	c.Representative = strings.TrimSpace(r.Representative)
	c.Instagram = strings.TrimSpace(strings.TrimPrefix(r.Instagram, "@"))
	c.MachinesCount = r.MachinesCount
	c.IsClient = r.IsClient
	c.AnticipationMode = r.AnticipationMode
	c.Validator = r.Validator
	c.DoNotDisturb = r.DoNotDisturb

	c.Products = []string{}
	for _, p := range r.Products {
		if p = strings.TrimSpace(p); p != "" {
			c.Products = append(c.Products, p)
		}
	}

	c.AccreditedAt = nil
	if r.AccreditedAt != "" {
		t, err := time.Parse("2006-01-02", r.AccreditedAt)
		if err != nil {
			return "data do credenciamento inválida (use AAAA-MM-DD)"
		}
		c.AccreditedAt = &t
	}
	return ""
}

func CreateCompany(ctx iris.Context) {
	var req companyRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	company := &models.Company{}
	if msg := req.apply(company); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if company.OwnerID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			company.OwnerID = &claims.UserID
		}
	}
	if err := models.CreateCompany(utils.DB, company); err != nil {
		serverError(ctx, err)
		return
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   "Empresa criada",
		CompanyID: &company.ID,
	})
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(company)
}

func UpdateCompany(ctx iris.Context) {
	company, err := models.CompanyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req companyRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	if msg := req.apply(company); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if err := models.UpdateCompany(utils.DB, company); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(company)
}

func DeleteCompany(ctx iris.Context) {
	id := paramID(ctx)
	label := ""
	if company, err := models.CompanyByID(utils.DB, id); err == nil {
		label = company.Name
	}
	if err := models.DeleteCompany(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "empresa", id, "excluiu a empresa "+label)
	ctx.JSON(iris.Map{"message": "empresa removida"})
}
