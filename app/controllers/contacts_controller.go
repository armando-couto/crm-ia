package controllers

import (
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

func contactFilterFromQuery(ctx iris.Context) models.ContactFilter {
	return models.ContactFilter{
		Search:         ctx.URLParam("q"),
		OwnerID:        ctx.URLParamInt64Default("owner_id", 0),
		CompanyID:      ctx.URLParamInt64Default("company_id", 0),
		LifecycleStage: ctx.URLParam("lifecycle_stage"),
		Source:         ctx.URLParam("source"),
		Unassigned:     ctx.URLParamBoolDefault("sem_dono", false),
		NoEmail:        ctx.URLParamBoolDefault("sem_email", false),
		CreatedDays:    ctx.URLParamIntDefault("criado_dias", 0),
		InactiveDays:   ctx.URLParamIntDefault("sem_atividade_dias", 0),
		SortBy:         ctx.URLParam("sort"),
		SortDir:        ctx.URLParam("dir"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
}

// ContactStatsHandler alimenta os cartões de métricas da tela de contatos.
func ContactStatsHandler(ctx iris.Context) {
	stats, err := models.LoadContactStats(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(stats)
}

type bulkContactsRequest struct {
	IDs            []int64 `json:"ids"`
	Action         string  `json:"action"` // dono | estagio | excluir
	OwnerID        *int64  `json:"owner_id"`
	LifecycleStage string  `json:"lifecycle_stage"`
}

// BulkContacts aplica uma ação em massa aos contatos selecionados.
func BulkContacts(ctx iris.Context) {
	var req bulkContactsRequest
	if err := ctx.ReadJSON(&req); err != nil || len(req.IDs) == 0 {
		badRequest(ctx, "selecione ao menos um contato")
		return
	}
	if len(req.IDs) > 500 {
		badRequest(ctx, "no máximo 500 contatos por vez")
		return
	}

	var affected int64
	var err error
	switch req.Action {
	case "dono":
		affected, err = models.BulkAssignOwner(utils.DB, req.IDs, req.OwnerID)
	case "estagio":
		if !models.ValidLifecycleStage(req.LifecycleStage) {
			badRequest(ctx, "estágio do ciclo de vida inválido")
			return
		}
		affected, err = models.BulkSetLifecycleStage(utils.DB, req.IDs, req.LifecycleStage)
	case "excluir":
		affected, err = models.BulkDeleteContacts(utils.DB, req.IDs)
	default:
		badRequest(ctx, "ação inválida (dono, estagio ou excluir)")
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditBulk, "contato", 0,
		fmt.Sprintf("ação em massa \"%s\" em %d contatos", req.Action, affected))
	ctx.JSON(iris.Map{"affected": affected})
}

// parseContactAdvanced lê o parâmetro af (filtros avançados) e responde 400 se inválido.
func parseContactAdvanced(ctx iris.Context, f *models.ContactFilter) bool {
	adv, err := models.ParseAdvancedFilters(ctx.URLParam("af"), models.ContactFilterFieldsSpec)
	if err != nil {
		badRequest(ctx, err.Error())
		return false
	}
	f.Advanced = adv
	return true
}

func ListContacts(ctx iris.Context) {
	f := contactFilterFromQuery(ctx)
	if !parseContactAdvanced(ctx, &f) {
		return
	}
	list, total, err := models.ListContacts(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

func GetContact(ctx iris.Context) {
	contact, err := models.ContactByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(contact)
}

type contactRequest struct {
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	JobTitle       string `json:"job_title"`
	LifecycleStage string `json:"lifecycle_stage"`
	Source         string `json:"source"`
	CompanyID      *int64 `json:"company_id"`
	OwnerID        *int64 `json:"owner_id"`
	BuyingRole     string `json:"buying_role"`
}

func (r *contactRequest) validate() string {
	r.FirstName = strings.TrimSpace(r.FirstName)
	if r.FirstName == "" {
		return "informe o nome do contato"
	}
	if r.LifecycleStage != "" && !models.ValidLifecycleStage(r.LifecycleStage) {
		return "estágio do ciclo de vida inválido"
	}
	if !models.ValidBuyingRole(r.BuyingRole) {
		return "papel de compra inválido"
	}
	return ""
}

func (r *contactRequest) apply(c *models.Contact) {
	c.FirstName = r.FirstName
	c.LastName = strings.TrimSpace(r.LastName)
	c.Email = r.Email
	c.Phone = r.Phone
	c.JobTitle = r.JobTitle
	if r.LifecycleStage != "" {
		c.LifecycleStage = r.LifecycleStage
	}
	c.Source = r.Source
	c.CompanyID = r.CompanyID
	c.OwnerID = r.OwnerID
	c.BuyingRole = r.BuyingRole
}

func CreateContact(ctx iris.Context) {
	var req contactRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	contact := &models.Contact{}
	req.apply(contact)
	if contact.OwnerID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			contact.OwnerID = &claims.UserID
		}
	}
	if err := models.CreateContact(utils.DB, contact); err != nil {
		serverError(ctx, err)
		return
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   "Contato criado",
		ContactID: &contact.ID,
	})
	// As automações rodam em segundo plano: nunca seguram a resposta.
	go services.FireContactCreated(utils.DB, contact)

	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(contact)
}

func UpdateContact(ctx iris.Context) {
	contact, err := models.ContactByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req contactRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	stageBefore := contact.LifecycleStage
	req.apply(contact)
	if err := models.UpdateContact(utils.DB, contact); err != nil {
		serverError(ctx, err)
		return
	}
	if stageBefore != contact.LifecycleStage {
		go services.FireContactStage(utils.DB, contact)
	}
	ctx.JSON(contact)
}

func DeleteContact(ctx iris.Context) {
	id := paramID(ctx)
	label := ""
	if contact, err := models.ContactByID(utils.DB, id); err == nil {
		label = strings.TrimSpace(contact.FirstName + " " + contact.LastName)
	}
	if err := models.DeleteContact(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "contato", id, "excluiu o contato "+label)
	ctx.JSON(iris.Map{"message": "contato removido"})
}

// ExportContacts exporta a listagem filtrada em CSV.
func ExportContacts(ctx iris.Context) {
	f := contactFilterFromQuery(ctx)
	if !parseContactAdvanced(ctx, &f) {
		return
	}
	f.PerPage = 100
	f.Page = 1

	ctx.Header("Content-Type", "text/csv; charset=utf-8")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="contatos-%s.csv"`, time.Now().Format("2006-01-02")))

	w := csv.NewWriter(ctx.ResponseWriter())
	defer w.Flush()
	w.Write([]string{"nome", "sobrenome", "email", "telefone", "cargo", "estagio", "origem", "empresa", "dono"})

	exported := 0
	// Exportar leva a base inteira embora: fica registrado na auditoria.
	defer func() {
		audit(ctx, models.AuditExport, "contato", 0,
			fmt.Sprintf("exportou %d contatos em CSV", exported))
	}()

	for {
		list, total, err := models.ListContacts(utils.DB, f)
		if err != nil {
			return
		}
		for _, c := range list {
			w.Write([]string{c.FirstName, c.LastName, c.Email, c.Phone, c.JobTitle,
				c.LifecycleStage, c.Source, c.CompanyName, c.OwnerName})
		}
		exported += len(list)
		if f.Page*f.PerPage >= total || len(list) == 0 {
			return
		}
		f.Page++
	}
}

// ImportContacts recebe um CSV (nome, sobrenome, email, telefone, cargo, estagio, origem)
// e cria os contatos em lote, ignorando linhas inválidas.
func ImportContacts(ctx iris.Context) {
	file, _, err := ctx.FormFile("file")
	if err != nil {
		badRequest(ctx, "envie o arquivo CSV no campo 'file'")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	var claims = middlewareClaims(ctx)
	created, skipped := 0, 0
	first := true
	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if first {
			first = false
			// Pula o cabeçalho quando presente.
			if len(record) > 0 && strings.EqualFold(strings.TrimSpace(record[0]), "nome") {
				continue
			}
		}
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			skipped++
			continue
		}
		get := func(i int) string {
			if i < len(record) {
				return strings.TrimSpace(record[i])
			}
			return ""
		}
		contact := &models.Contact{
			FirstName:      get(0),
			LastName:       get(1),
			Email:          get(2),
			Phone:          get(3),
			JobTitle:       get(4),
			LifecycleStage: get(5),
			Source:         get(6),
		}
		if contact.LifecycleStage != "" && !models.ValidLifecycleStage(contact.LifecycleStage) {
			contact.LifecycleStage = "lead"
		}
		if claims != nil {
			contact.OwnerID = &claims.UserID
		}
		if err := models.CreateContact(utils.DB, contact); err != nil {
			skipped++
			continue
		}
		created++
	}
	audit(ctx, models.AuditImport, "contato", 0,
		fmt.Sprintf("importou %d contatos por CSV (%d linhas ignoradas)", created, skipped))
	ctx.JSON(iris.Map{"created": created, "skipped": skipped})
}
