package controllers

import (
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ===== Manuais de atividades (playbooks) =====

func ListPlaybooks(ctx iris.Context) {
	list, err := models.ListPlaybooks(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

type playbookRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Active      *bool  `json:"active"`
}

func (r *playbookRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome do manual"
	}
	if strings.TrimSpace(r.Body) == "" {
		return "informe o conteúdo do manual"
	}
	return ""
}

func CreatePlaybook(ctx iris.Context) {
	var req playbookRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	p := &models.Playbook{Name: req.Name, Description: req.Description, Body: req.Body, Active: true}
	if req.Active != nil {
		p.Active = *req.Active
	}
	if claims := middlewareClaims(ctx); claims != nil {
		p.CreatedBy = &claims.UserID
	}
	if err := models.CreatePlaybook(utils.DB, p); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(p)
}

func UpdatePlaybook(ctx iris.Context) {
	p, err := models.PlaybookByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	var req playbookRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	p.Name = req.Name
	p.Description = req.Description
	p.Body = req.Body
	if req.Active != nil {
		p.Active = *req.Active
	}
	if err := models.UpdatePlaybook(utils.DB, p); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(p)
}

func DeletePlaybook(ctx iris.Context) {
	if err := models.DeletePlaybook(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "manual removido"})
}

// ===== Modelos de mensagens =====

func ListMessageTemplates(ctx iris.Context) {
	list, err := models.ListMessageTemplates(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

type templateRequest struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (r *templateRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	r.Subject = strings.TrimSpace(r.Subject)
	if r.Name == "" {
		return "informe o nome do modelo"
	}
	if r.Subject == "" {
		return "informe o assunto"
	}
	if strings.TrimSpace(r.Body) == "" {
		return "informe o corpo da mensagem"
	}
	return ""
}

func CreateMessageTemplate(ctx iris.Context) {
	var req templateRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	t := &models.MessageTemplate{Name: req.Name, Subject: req.Subject, Body: req.Body}
	if claims := middlewareClaims(ctx); claims != nil {
		t.CreatedBy = &claims.UserID
	}
	if err := models.CreateMessageTemplate(utils.DB, t); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(t)
}

func UpdateMessageTemplate(ctx iris.Context) {
	var req templateRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	t := &models.MessageTemplate{ID: paramID(ctx), Name: req.Name, Subject: req.Subject, Body: req.Body}
	if err := models.UpdateMessageTemplate(utils.DB, t); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(t)
}

func DeleteMessageTemplate(ctx iris.Context) {
	if err := models.DeleteMessageTemplate(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "modelo removido"})
}

// RenderMessageTemplate devolve assunto/corpo do modelo com as variáveis
// preenchidas pelos dados do contato informado.
func RenderMessageTemplate(ctx iris.Context) {
	templateID := paramID(ctx)
	contactID := ctx.URLParamInt64Default("contact_id", 0)
	if contactID == 0 {
		badRequest(ctx, "informe o contact_id")
		return
	}

	templates, err := models.ListMessageTemplates(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	var tpl *models.MessageTemplate
	for i := range templates {
		if templates[i].ID == templateID {
			tpl = &templates[i]
			break
		}
	}
	if tpl == nil {
		notFound(ctx)
		return
	}

	contact, err := models.ContactByID(utils.DB, contactID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"subject": models.RenderTemplate(tpl.Subject, contact),
		"body":    models.RenderTemplate(tpl.Body, contact),
	})
}

// ===== Snippets =====

func ListSnippets(ctx iris.Context) {
	list, err := models.ListSnippets(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

type snippetRequest struct {
	Name     string `json:"name"`
	Shortcut string `json:"shortcut"`
	Body     string `json:"body"`
}

func (r *snippetRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	r.Shortcut = strings.TrimSpace(r.Shortcut)
	if r.Name == "" {
		return "informe o nome do snippet"
	}
	if r.Shortcut == "" {
		return "informe o atalho (ex.: #saudacao)"
	}
	if strings.TrimSpace(r.Body) == "" {
		return "informe o texto do snippet"
	}
	return ""
}

func CreateSnippet(ctx iris.Context) {
	var req snippetRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	s := &models.Snippet{Name: req.Name, Shortcut: req.Shortcut, Body: req.Body}
	if claims := middlewareClaims(ctx); claims != nil {
		s.CreatedBy = &claims.UserID
	}
	if err := models.CreateSnippet(utils.DB, s); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(s)
}

func UpdateSnippet(ctx iris.Context) {
	var req snippetRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	s := &models.Snippet{ID: paramID(ctx), Name: req.Name, Shortcut: req.Shortcut, Body: req.Body}
	if err := models.UpdateSnippet(utils.DB, s); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(s)
}

func DeleteSnippet(ctx iris.Context) {
	if err := models.DeleteSnippet(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "snippet removido"})
}
