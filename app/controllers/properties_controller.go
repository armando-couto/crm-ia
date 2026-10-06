package controllers

import (
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListProperties lista as propriedades personalizadas de um objeto.
func ListProperties(ctx iris.Context) {
	entity := ctx.URLParam("entity")
	if entity != "" && !models.ValidPropertyEntity(entity) {
		badRequest(ctx, "objeto inválido (contacts, companies, deals ou tickets)")
		return
	}
	list, err := models.ListProperties(utils.DB, entity)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

type propertyRequest struct {
	Entity      string                  `json:"entity"`
	Key         string                  `json:"key"`
	Label       string                  `json:"label"`
	Description string                  `json:"description"`
	FieldType   string                  `json:"field_type"`
	Options     []models.PropertyOption `json:"options"`
	GroupName   string                  `json:"group_name"`
}

func CreateProperty(ctx iris.Context) {
	var req propertyRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	prop := &models.CustomProperty{
		Entity:      req.Entity,
		Key:         strings.TrimSpace(req.Key),
		Label:       req.Label,
		Description: strings.TrimSpace(req.Description),
		FieldType:   req.FieldType,
		Options:     req.Options,
		GroupName:   strings.TrimSpace(req.GroupName),
	}
	if err := models.ValidateProperty(prop); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	if claims := middlewareClaims(ctx); claims != nil {
		prop.CreatedBy = &claims.UserID
	}
	if err := models.CreateProperty(utils.DB, prop); err != nil {
		if strings.Contains(err.Error(), "custom_properties_entity_key_key") {
			badRequest(ctx, "já existe uma propriedade com este nome interno neste objeto")
			return
		}
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(prop)
}

// UpdateProperty altera rótulo, descrição, grupo e opções (tipo e nome interno são imutáveis).
func UpdateProperty(ctx iris.Context) {
	prop, err := models.PropertyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req propertyRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	prop.Label = req.Label
	prop.Description = strings.TrimSpace(req.Description)
	prop.GroupName = strings.TrimSpace(req.GroupName)
	prop.Options = req.Options
	if err := models.ValidateProperty(prop); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	if err := models.UpdateProperty(utils.DB, prop); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(prop)
}

// DeleteProperty remove a propriedade e todos os valores gravados nos registros.
func DeleteProperty(ctx iris.Context) {
	if err := models.DeleteProperty(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "propriedade removida"})
}

// GetPropertyValues devolve os valores personalizados de um registro.
func GetPropertyValues(ctx iris.Context) {
	entity := ctx.URLParam("entity")
	recordID := ctx.URLParamInt64Default("record_id", 0)
	if !models.ValidPropertyEntity(entity) || recordID == 0 {
		badRequest(ctx, "informe entity e record_id")
		return
	}
	values, err := models.PropertyValues(utils.DB, entity, recordID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"values": values})
}

// SavePropertyValues grava os valores personalizados de um registro.
func SavePropertyValues(ctx iris.Context) {
	var req struct {
		Entity   string         `json:"entity"`
		RecordID int64          `json:"record_id"`
		Values   map[string]any `json:"values"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if !models.ValidPropertyEntity(req.Entity) || req.RecordID == 0 {
		badRequest(ctx, "informe entity e record_id")
		return
	}
	if len(req.Values) > 200 {
		badRequest(ctx, "muitos campos de uma vez")
		return
	}
	if err := models.SavePropertyValues(utils.DB, req.Entity, req.RecordID, req.Values); err != nil {
		badRequest(ctx, err.Error())
		return
	}

	values, err := models.PropertyValues(utils.DB, req.Entity, req.RecordID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"values": values})
}
