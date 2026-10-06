package controllers

import (
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

func ListContactLists(ctx iris.Context) {
	lists, err := models.ListContactLists(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(lists)
}

func GetContactList(ctx iris.Context) {
	list, err := models.ContactListByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(list)
}

type listRequest struct {
	Name  string            `json:"name"`
	Kind  string            `json:"kind"`
	Rules *models.ListRules `json:"rules"`
}

func (r *listRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome da lista"
	}
	if r.Kind == "" {
		r.Kind = "estatica"
	}
	if r.Kind != "estatica" && r.Kind != "dinamica" {
		return "tipo inválido (estatica ou dinamica)"
	}
	if r.Kind == "dinamica" {
		if r.Rules == nil || (r.Rules.LifecycleStage == "" && r.Rules.OwnerID == 0 && r.Rules.Source == "") {
			return "listas dinâmicas precisam de pelo menos um filtro"
		}
		if r.Rules.LifecycleStage != "" && !models.ValidLifecycleStage(r.Rules.LifecycleStage) {
			return "estágio do ciclo de vida inválido"
		}
	}
	return ""
}

func CreateContactList(ctx iris.Context) {
	var req listRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	list := &models.ContactList{Name: req.Name, Kind: req.Kind}
	if req.Kind == "dinamica" {
		list.Rules = req.Rules
	}
	if claims := middlewareClaims(ctx); claims != nil {
		list.CreatedBy = &claims.UserID
	}
	if err := models.CreateContactList(utils.DB, list); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(list)
}

func UpdateContactList(ctx iris.Context) {
	list, err := models.ContactListByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req listRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	req.Kind = list.Kind // o tipo não muda depois de criada
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	list.Name = req.Name
	if list.Kind == "dinamica" {
		list.Rules = req.Rules
	}
	if err := models.UpdateContactList(utils.DB, list); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

func DeleteContactList(ctx iris.Context) {
	if err := models.DeleteContactList(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "lista removida"})
}

// ListMembers retorna os contatos da lista (estática ou dinâmica).
func ListMembers(ctx iris.Context) {
	list, err := models.ContactListByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	p := models.Pagination{
		Page:    ctx.URLParamIntDefault("page", 1),
		PerPage: ctx.URLParamIntDefault("per_page", 25),
	}
	contacts, total, err := models.ListContactsOfList(utils.DB, list, p)
	if err != nil {
		serverError(ctx, err)
		return
	}
	p.Normalize()
	p.Total = total
	ctx.JSON(iris.Map{"data": contacts, "pagination": p, "list": list})
}

func AddMember(ctx iris.Context) {
	var req struct {
		ContactID int64 `json:"contact_id"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.ContactID == 0 {
		badRequest(ctx, "informe o contato")
		return
	}
	list, err := models.ContactListByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if list.Kind != "estatica" {
		badRequest(ctx, "listas dinâmicas são calculadas pelas regras, não aceitam inclusão manual")
		return
	}
	if err := models.AddListMember(utils.DB, list.ID, req.ContactID); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "contato adicionado à lista"})
}

func RemoveMember(ctx iris.Context) {
	contactID := ctx.Params().GetInt64Default("contactId", 0)
	if err := models.RemoveListMember(utils.DB, paramID(ctx), contactID); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "contato removido da lista"})
}
