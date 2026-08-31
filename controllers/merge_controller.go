package controllers

import (
	"fmt"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ListDuplicates devolve os grupos de possíveis duplicados (?entity=contacts|companies).
func ListDuplicates(ctx iris.Context) {
	entity := ctx.URLParam("entity")

	var (
		groups []models.DuplicateGroup
		err    error
	)
	switch entity {
	case "companies":
		groups, err = models.FindDuplicateCompanies(utils.DB)
	case "contacts", "":
		entity = "contacts"
		groups, err = models.FindDuplicateContacts(utils.DB)
	default:
		badRequest(ctx, "entidade inválida (contacts ou companies)")
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"entity": entity, "groups": groups})
}

type mergeRequest struct {
	Entity      string `json:"entity"`
	PrimaryID   int64  `json:"primary_id"`
	DuplicateID int64  `json:"duplicate_id"`
}

// MergeRecords funde o duplicado no principal: histórico, negócios, tarefas e
// anexos migram, os campos vazios do principal são completados e o duplicado
// deixa de existir. A operação é uma transação só e não tem desfazer.
func MergeRecords(ctx iris.Context) {
	var req mergeRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if req.PrimaryID == 0 || req.DuplicateID == 0 {
		badRequest(ctx, "informe o registro principal e o duplicado")
		return
	}
	if req.PrimaryID == req.DuplicateID {
		badRequest(ctx, "escolha dois registros diferentes")
		return
	}

	switch req.Entity {
	case "companies":
		company, err := models.MergeCompanies(utils.DB, req.PrimaryID, req.DuplicateID)
		if err != nil {
			handleDBError(ctx, err)
			return
		}
		audit(ctx, models.AuditUpdate, "empresa", company.ID,
			fmt.Sprintf("mesclou a empresa #%d em %s (#%d)", req.DuplicateID, company.Name, company.ID))
		logActivity(ctx, models.Activity{
			Kind:      models.ActivitySistema,
			Content:   fmt.Sprintf("Empresa duplicada #%d mesclada neste registro", req.DuplicateID),
			CompanyID: &company.ID,
		})
		ctx.JSON(company)

	case "contacts", "":
		contact, err := models.MergeContacts(utils.DB, req.PrimaryID, req.DuplicateID)
		if err != nil {
			handleDBError(ctx, err)
			return
		}
		audit(ctx, models.AuditUpdate, "contato", contact.ID,
			fmt.Sprintf("mesclou o contato #%d em %s %s (#%d)",
				req.DuplicateID, contact.FirstName, contact.LastName, contact.ID))
		logActivity(ctx, models.Activity{
			Kind:      models.ActivitySistema,
			Content:   fmt.Sprintf("Contato duplicado #%d mesclado neste registro", req.DuplicateID),
			ContactID: &contact.ID,
		})
		ctx.JSON(contact)

	default:
		badRequest(ctx, "entidade inválida (contacts ou companies)")
	}
}
