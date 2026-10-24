package controllers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
	"github.com/armando-couto/crm-ia/app/utils"

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

	// Dados cadastrais (enriquecimento). Ponteiros: ausente no JSON mantém
	// o valor atual, para formulários antigos não apagarem o que a Receita
	// preencheu.
	LegalName            *string  `json:"legal_name"`
	TradeName            *string  `json:"trade_name"`
	CNAECode             *string  `json:"cnae_code"`
	CNAEDescription      *string  `json:"cnae_description"`
	LegalNature          *string  `json:"legal_nature"`
	FoundedAt            *string  `json:"founded_at"` // AAAA-MM-DD ("" limpa)
	Capital              *float64 `json:"capital"`
	CompanySize          *string  `json:"company_size"`
	RegistrationStatus   *string  `json:"registration_status"`
	RegistrationStatusAt *string  `json:"registration_status_at"`
	Simples              *bool    `json:"simples"`
	MEI                  *bool    `json:"mei"`
	Street               *string  `json:"street"`
	StreetNumber         *string  `json:"street_number"`
	Complement           *string  `json:"complement"`
	Neighborhood         *string  `json:"neighborhood"`
	Zip                  *string  `json:"zip"`
	Email                *string  `json:"email"`
	Website              *string  `json:"website"`
	EmployeesRange       *string  `json:"employees_range"`
	RevenueRange         *string  `json:"revenue_range"`

	// AutoReceita (só na criação) sobrepõe a configuração "preencher
	// automaticamente pela Receita Federal".
	AutoReceita *bool `json:"auto_receita"`
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
	return r.applyCadastro(c)
}

// applyCadastro grava os campos cadastrais presentes no JSON.
func (r *companyRequest) applyCadastro(c *models.Company) string {
	texto := func(dest *string, v *string) {
		if v != nil {
			*dest = strings.TrimSpace(*v)
		}
	}
	texto(&c.LegalName, r.LegalName)
	texto(&c.TradeName, r.TradeName)
	texto(&c.CNAECode, r.CNAECode)
	texto(&c.CNAEDescription, r.CNAEDescription)
	texto(&c.LegalNature, r.LegalNature)
	texto(&c.CompanySize, r.CompanySize)
	texto(&c.RegistrationStatus, r.RegistrationStatus)
	texto(&c.Street, r.Street)
	texto(&c.StreetNumber, r.StreetNumber)
	texto(&c.Complement, r.Complement)
	texto(&c.Neighborhood, r.Neighborhood)
	texto(&c.Email, r.Email)
	texto(&c.Website, r.Website)
	texto(&c.EmployeesRange, r.EmployeesRange)
	texto(&c.RevenueRange, r.RevenueRange)
	if r.Zip != nil {
		c.Zip = enriquecimento.SomenteDigitos(*r.Zip)
	}
	if r.Email != nil {
		c.Email = strings.ToLower(c.Email)
	}
	if r.Capital != nil {
		if *r.Capital < 0 {
			return "capital social não pode ser negativo"
		}
		v := *r.Capital
		c.Capital = &v
	}
	if r.Simples != nil {
		v := *r.Simples
		c.Simples = &v
	}
	if r.MEI != nil {
		v := *r.MEI
		c.MEI = &v
	}
	if r.FoundedAt != nil {
		d, err := models.ParseDate(*r.FoundedAt)
		if err != nil {
			return "data de abertura inválida (use AAAA-MM-DD)"
		}
		c.FoundedAt = d
	}
	if r.RegistrationStatusAt != nil {
		d, err := models.ParseDate(*r.RegistrationStatusAt)
		if err != nil {
			return "data da situação cadastral inválida (use AAAA-MM-DD)"
		}
		c.RegistrationStatusAt = d
	}
	if c.CNPJ != "" && !enriquecimento.ValidarCNPJ(c.CNPJ) && len(enriquecimento.SomenteDigitos(c.CNPJ)) == 14 {
		return "CNPJ inválido (dígitos verificadores não conferem)"
	}
	return ""
}

// preencherPelaReceita consulta a Receita Federal de forma síncrona (6 s) na
// criação e preenche o que estiver vazio. Falha é silenciosa: a empresa é
// criada do mesmo jeito. Devolve o perfil e os campos preenchidos.
func preencherPelaReceita(ctx iris.Context, company *models.Company, cfg models.EnrichmentSettings) (*enriquecimento.Perfil, []enriquecimento.CampoDiff) {
	reqCtx, cancelar := context.WithTimeout(ctx.Request().Context(), 6*time.Second)
	defer cancelar()
	prov := &enriquecimento.Receita{PrazoCache: time.Duration(cfg.PrazoCacheDias) * 24 * time.Hour}
	perfil, _, err := prov.Consultar(reqCtx, company.CNPJ, enriquecimento.Opcoes{Timeout: 6 * time.Second})
	if err != nil || perfil == nil {
		return nil, nil
	}
	aplicados := []enriquecimento.CampoDiff{}
	for _, d := range enriquecimento.Diff(company, perfil, "vazios") {
		if d.Sugerido && models.EnrichableColumns[d.Campo] && enriquecimento.AtribuirCampo(company, d.Campo, d.Proposto) {
			aplicados = append(aplicados, d)
		}
	}
	return perfil, aplicados
}

// registrarAutoReceita grava o histórico, as fontes e o intel do preenchimento
// automático feito na criação.
func registrarAutoReceita(ctx iris.Context, company *models.Company, perfil *enriquecimento.Perfil, aplicados []enriquecimento.CampoDiff) {
	if perfil == nil {
		return
	}
	campos := make([]string, 0, len(aplicados))
	for _, d := range aplicados {
		campos = append(campos, d.Campo)
	}
	raw, _ := json.Marshal(perfil)
	row := models.Enrichment{CompanyID: company.ID, Provedor: models.ProvedorReceita, Status: models.EnrichmentOK,
		Perfil: raw, UserID: userIDPtr(ctx)}
	if err := models.InsertEnrichment(utils.DB, &row); err != nil {
		return
	}
	fontes := make([]models.FieldSource, 0, len(campos))
	for _, c := range campos {
		id := row.ID
		fontes = append(fontes, models.FieldSource{Campo: c, Provedor: models.ProvedorReceita, EnrichmentID: &id})
	}
	_ = models.UpsertFieldSources(utils.DB, company.ID, fontes)
	_ = models.MarkEnrichmentsApplied(utils.DB, []int64{row.ID}, campos)
	if mesclado := enriquecimento.Mesclar(perfil); mesclado != nil {
		if b, err := json.Marshal(mesclado); err == nil {
			_ = models.SetCompanyIntel(utils.DB, company.ID, b)
		}
	}
	if len(campos) > 0 {
		logActivity(ctx, models.Activity{
			Kind:      models.ActivitySistema,
			Content:   fraseEnriquecimento([]string{models.ProvedorReceita}, len(campos)),
			CompanyID: &company.ID,
		})
	}
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

	// Preenchimento automático pela Receita Federal (configurável; só com
	// CNPJ válido e sem razão social informada).
	var perfilAuto *enriquecimento.Perfil
	var camposAuto []enriquecimento.CampoDiff
	if enriquecimento.ValidarCNPJ(company.CNPJ) && company.LegalName == "" {
		cfg := models.LoadEnrichmentSettings(utils.DB)
		auto := cfg.AutoReceita
		if req.AutoReceita != nil {
			auto = *req.AutoReceita
		}
		if auto {
			perfilAuto, camposAuto = preencherPelaReceita(ctx, company, cfg)
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
	registrarAutoReceita(ctx, company, perfilAuto, camposAuto)
	if perfilAuto != nil {
		if atualizada, err := models.CompanyByID(utils.DB, company.ID); err == nil {
			company = atualizada
		}
	}
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
