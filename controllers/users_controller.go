package controllers

import (
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ListUsers lista todos os usuários (qualquer autenticado pode ver, para atribuir donos).
func ListUsers(ctx iris.Context) {
	users, err := models.ListUsers(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(users)
}

type userRequest struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Active *bool  `json:"active"`
	TeamID *int64 `json:"team_id"`
}

func (r *userRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	r.Email = models.NormalizeEmail(r.Email)
	if r.Name == "" {
		return "informe o nome"
	}
	if r.Email == "" || !strings.Contains(r.Email, "@") {
		return "informe um e-mail válido"
	}
	if r.Role == "" {
		r.Role = models.RoleVendedor
	}
	if !models.ValidRole(r.Role) {
		return "papel inválido (admin, gestor ou vendedor)"
	}
	return ""
}

// CreateUser (admin) cria o usuário com senha temporária e envia boas-vindas via Mandrill.
func CreateUser(ctx iris.Context) {
	var req userRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	tempPassword, err := services.RandomToken()
	if err != nil {
		serverError(ctx, err)
		return
	}
	tempPassword = tempPassword[:12]

	hash, err := services.HashPassword(tempPassword)
	if err != nil {
		serverError(ctx, err)
		return
	}

	user := &models.User{
		Name:         req.Name,
		Email:        req.Email,
		Role:         req.Role,
		Active:       true,
		TeamID:       req.TeamID,
		PasswordHash: hash,
	}
	if err := models.CreateUser(utils.DB, user); err != nil {
		if strings.Contains(err.Error(), "users_email_key") {
			badRequest(ctx, "já existe um usuário com este e-mail")
			return
		}
		serverError(ctx, err)
		return
	}

	if services.Mail != nil {
		subject, html := services.WelcomeEmail(user.Name, tempPassword, utils.AppURL)
		if err := services.Mail.Send(user.Email, user.Name, subject, html); err != nil {
			ctx.Application().Logger().Errorf("falha ao enviar boas-vindas: %v", err)
		}
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(user)
}

// UpdateUserByID (admin) atualiza nome, e-mail, papel e status.
func UpdateUserByID(ctx iris.Context) {
	id := paramID(ctx)
	user, err := models.UserByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req userRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	user.Name = req.Name
	user.Email = req.Email
	user.Role = req.Role
	user.TeamID = req.TeamID
	if req.Active != nil {
		user.Active = *req.Active
	}
	if err := models.UpdateUser(utils.DB, user); err != nil {
		serverError(ctx, err)
		return
	}
	updated, err := models.UserByID(utils.DB, user.ID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(updated)
}

// DeactivateUser (admin) desativa o acesso sem apagar o histórico.
func DeactivateUser(ctx iris.Context) {
	id := paramID(ctx)
	claims := middlewareClaims(ctx)
	if claims != nil && claims.UserID == id {
		badRequest(ctx, "você não pode desativar o próprio usuário")
		return
	}
	user, err := models.UserByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	user.Active = false
	if err := models.UpdateUser(utils.DB, user); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(user)
}
