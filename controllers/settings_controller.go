package controllers

import (
	"encoding/json"
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

const contactFormKey = "contact_form"

// GetContactForm devolve a configuração do formulário de criação de contato.
func GetContactForm(ctx iris.Context) {
	var fields []models.FormField
	found, err := models.GetSetting(utils.DB, contactFormKey, &fields)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if !found {
		fields = models.DefaultContactForm()
	}
	ctx.JSON(iris.Map{"fields": fields})
}

// UpdateContactForm (admin/gestor) personaliza os campos do formulário.
func UpdateContactForm(ctx iris.Context) {
	var req struct {
		Fields []models.FormField `json:"fields"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if err := models.ValidateContactForm(req.Fields); err != nil {
		badRequest(ctx, err.Error())
		return
	}

	var userID *int64
	if claims := middlewareClaims(ctx); claims != nil {
		userID = &claims.UserID
	}
	if err := models.SetSetting(utils.DB, contactFormKey, req.Fields, userID); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"fields": req.Fields})
}

// ListViews lista as visualizações salvas de uma entidade; sem entity, todas.
func ListViews(ctx iris.Context) {
	entity := ctx.URLParam("entity")
	if entity != "" && !models.ValidViewEntity(entity) {
		badRequest(ctx, "entity inválida (contacts, companies ou deals)")
		return
	}
	views, err := models.ListSavedViews(utils.DB, entity)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(views)
}

// CreateView salva os filtros atuais como uma visualização com nome.
func CreateView(ctx iris.Context) {
	var req struct {
		Entity  string          `json:"entity"`
		Name    string          `json:"name"`
		Filters json.RawMessage `json:"filters"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(ctx, "informe o nome da visualização")
		return
	}
	if !models.ValidViewEntity(req.Entity) {
		badRequest(ctx, "entity inválida (contacts ou companies)")
		return
	}
	if len(req.Filters) == 0 || len(req.Filters) > 4096 {
		badRequest(ctx, "filtros inválidos")
		return
	}
	if !json.Valid(req.Filters) {
		badRequest(ctx, "filtros inválidos")
		return
	}

	view := &models.SavedView{Entity: req.Entity, Name: req.Name, Filters: req.Filters}
	if claims := middlewareClaims(ctx); claims != nil {
		view.CreatedBy = &claims.UserID
	}
	if err := models.CreateSavedView(utils.DB, view); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(view)
}

// RenameView renomeia uma visualização salva.
func RenameView(ctx iris.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		badRequest(ctx, "informe o nome")
		return
	}
	if err := models.RenameSavedView(utils.DB, paramID(ctx), strings.TrimSpace(req.Name)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "visualização renomeada"})
}

// DeleteView remove uma visualização salva.
func DeleteView(ctx iris.Context) {
	if err := models.DeleteSavedView(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "visualização removida"})
}
