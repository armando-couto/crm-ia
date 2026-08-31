package controllers

import (
	"database/sql"
	"fmt"
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ===== Administração dos formulários =====

func ListPublicForms(ctx iris.Context) {
	forms, err := models.ListPublicForms(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"data":           forms,
		"default_fields": models.DefaultPublicFormFields(),
		"embed_base":     strings.TrimSuffix(utils.AppURL, "/"),
	})
}

type publicFormRequest struct {
	Name           string                   `json:"name"`
	Slug           string                   `json:"slug"`
	Headline       string                   `json:"headline"`
	Description    string                   `json:"description"`
	Fields         []models.PublicFormField `json:"fields"`
	SubmitLabel    string                   `json:"submit_label"`
	SuccessMessage string                   `json:"success_message"`
	RedirectURL    string                   `json:"redirect_url"`
	OwnerID        *int64                   `json:"owner_id"`
	ListID         *int64                   `json:"list_id"`
	LifecycleStage string                   `json:"lifecycle_stage"`
	Source         string                   `json:"source"`
	Active         *bool                    `json:"active"`
}

func (r *publicFormRequest) apply(form *models.PublicForm) string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome do formulário"
	}
	if err := models.ValidatePublicFormFields(r.Fields); err != nil {
		return err.Error()
	}
	if r.LifecycleStage == "" {
		r.LifecycleStage = "lead"
	}
	if !models.ValidLifecycleStage(r.LifecycleStage) {
		return "estágio do ciclo de vida inválido"
	}

	slug := models.Slugify(r.Slug)
	if slug == "" {
		slug = models.Slugify(r.Name)
	}
	if slug == "" {
		return "não foi possível gerar o endereço do formulário"
	}

	form.Name = r.Name
	form.Slug = slug
	form.Headline = strings.TrimSpace(r.Headline)
	form.Description = strings.TrimSpace(r.Description)
	form.Fields = r.Fields
	form.SubmitLabel = strings.TrimSpace(r.SubmitLabel)
	if form.SubmitLabel == "" {
		form.SubmitLabel = "Enviar"
	}
	form.SuccessMessage = strings.TrimSpace(r.SuccessMessage)
	if form.SuccessMessage == "" {
		form.SuccessMessage = "Recebemos seus dados. Em breve entraremos em contato."
	}
	form.RedirectURL = strings.TrimSpace(r.RedirectURL)
	form.OwnerID = r.OwnerID
	form.ListID = r.ListID
	form.LifecycleStage = r.LifecycleStage
	form.Source = strings.TrimSpace(r.Source)
	if form.Source == "" {
		form.Source = "formulario"
	}
	form.Active = true
	if r.Active != nil {
		form.Active = *r.Active
	}
	return ""
}

func CreatePublicForm(ctx iris.Context) {
	var req publicFormRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	form := &models.PublicForm{}
	if msg := req.apply(form); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if claims := middlewareClaims(ctx); claims != nil {
		form.CreatedBy = &claims.UserID
	}

	if err := models.CreatePublicForm(utils.DB, form); err != nil {
		if strings.Contains(err.Error(), "public_forms_slug_key") {
			badRequest(ctx, "já existe um formulário com este endereço (slug)")
			return
		}
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, "formulario", form.ID, "criou o formulário "+form.Name)
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(form)
}

func UpdatePublicFormByID(ctx iris.Context) {
	form, err := models.PublicFormByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req publicFormRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.apply(form); msg != "" {
		badRequest(ctx, msg)
		return
	}

	if err := models.UpdatePublicForm(utils.DB, form); err != nil {
		if strings.Contains(err.Error(), "public_forms_slug_key") {
			badRequest(ctx, "já existe um formulário com este endereço (slug)")
			return
		}
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "formulario", form.ID, "editou o formulário "+form.Name)
	ctx.JSON(form)
}

func DeletePublicFormByID(ctx iris.Context) {
	id := paramID(ctx)
	form, err := models.PublicFormByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeletePublicForm(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "formulario", id, "excluiu o formulário "+form.Name)
	ctx.JSON(iris.Map{"message": "formulário removido"})
}

func ListFormSubmissions(ctx iris.Context) {
	list, err := models.ListFormSubmissions(utils.DB, paramID(ctx),
		ctx.URLParamIntDefault("limit", 50))
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": list})
}

// ===== Rotas públicas (site do cliente) =====

// GetPublicForm devolve a definição do formulário para renderizar a página
// pública. Só expõe o necessário: nada de dono, lista ou contadores internos.
func GetPublicForm(ctx iris.Context) {
	form, err := models.PublicFormBySlug(utils.DB, ctx.Params().Get("slug"))
	if err == sql.ErrNoRows || (err == nil && !form.Active) {
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "formulário não encontrado"})
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}

	ctx.JSON(iris.Map{
		"slug":         form.Slug,
		"name":         form.Name,
		"headline":     form.Headline,
		"description":  form.Description,
		"fields":       form.Fields,
		"submit_label": form.SubmitLabel,
	})
}

// SubmitPublicFormHandler recebe o envio do site do cliente: cria ou atualiza o
// contato, registra na timeline e devolve a mensagem de sucesso.
func SubmitPublicFormHandler(ctx iris.Context) {
	slug := ctx.Params().Get("slug")

	// Limite por IP: segura robô de spam sem atrapalhar quem preenche de verdade.
	if services.PublicRateLimited(clientIP(ctx)) {
		ctx.StopWithJSON(iris.StatusTooManyRequests,
			iris.Map{"error": "muitos envios deste endereço: tente novamente em alguns minutos"})
		return
	}

	form, err := models.PublicFormBySlug(utils.DB, slug)
	if err == sql.ErrNoRows || (err == nil && !form.Active) {
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "formulário não encontrado"})
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}

	var payload map[string]string
	if err := ctx.ReadJSON(&payload); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	// Campo isca invisível: preenchido significa robô. Responde sucesso para o
	// robô não descobrir que foi barrado, mas nada é gravado.
	if strings.TrimSpace(payload["_gotcha"]) != "" {
		ctx.JSON(iris.Map{"message": form.SuccessMessage})
		return
	}

	values := map[string]string{}
	for _, field := range form.Fields {
		value := strings.TrimSpace(payload[field.Key])
		if len(value) > 2000 {
			value = value[:2000]
		}
		if field.Required && value == "" {
			badRequest(ctx, "preencha o campo "+field.Label)
			return
		}
		values[field.Key] = value
	}
	if !strings.Contains(values["email"], "@") {
		badRequest(ctx, "informe um e-mail válido")
		return
	}

	contact, existed, err := models.SubmitPublicForm(utils.DB, form, values, clientIP(ctx))
	if err != nil {
		serverError(ctx, err)
		return
	}

	// Timeline: de onde veio o lead e o que ele escreveu.
	content := fmt.Sprintf("Lead recebido pelo formulário \"%s\"", form.Name)
	if extra := models.ExtraFormValues(form, values); extra != "" {
		content += "\n" + extra
	}
	if err := models.CreateActivity(utils.DB, &models.Activity{
		Kind:      models.ActivitySistema,
		Content:   content,
		ContactID: &contact.ID,
	}); err != nil {
		ctx.Application().Logger().Errorf("falha ao registrar atividade do formulário: %v", err)
	}

	// Avisa o dono do formulário que chegou lead novo.
	if form.OwnerID != nil && !existed {
		go services.NotifyNewLead(utils.DB, *form.OwnerID, form.Name,
			strings.TrimSpace(contact.FirstName+" "+contact.LastName), contact.Email, contact.ID)
	}

	ctx.JSON(iris.Map{
		"message":      form.SuccessMessage,
		"redirect_url": form.RedirectURL,
	})
}
