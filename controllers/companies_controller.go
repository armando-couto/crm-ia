package controllers

import (
	"strings"

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
}

func (r *companyRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome da empresa"
	}
	return ""
}

func (r *companyRequest) apply(c *models.Company) {
	c.Name = r.Name
	c.Domain = strings.TrimSpace(strings.ToLower(r.Domain))
	c.Phone = r.Phone
	c.Industry = r.Industry
	c.City = r.City
	c.State = r.State
	c.OwnerID = r.OwnerID
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
	req.apply(company)
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

	req.apply(company)
	if err := models.UpdateCompany(utils.DB, company); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(company)
}

func DeleteCompany(ctx iris.Context) {
	if err := models.DeleteCompany(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "empresa removida"})
}
