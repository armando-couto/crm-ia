package controllers

import (
	"strings"
	"time"

	"fixpay/fix-crm/middleware"
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// InviteTTL é a validade da senha temporária enviada no convite.
const InviteTTL = 7 * 24 * time.Hour

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
		r.Role = models.RoleSeller
	}
	if !models.ValidRole(r.Role) {
		return "perfil inválido (seller, manager ou admin)"
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

	expiresAt := time.Now().Add(InviteTTL)
	user := &models.User{
		Name:               req.Name,
		Email:              req.Email,
		Role:               req.Role,
		Active:             true,
		TeamID:             req.TeamID,
		PasswordHash:       hash,
		MustChangePassword: true,
		InviteExpiresAt:    &expiresAt,
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
	audit(ctx, models.AuditCreate, "usuario", user.ID,
		"criou o usuário "+user.Email+" com perfil "+models.RoleLabel(user.Role))
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(user)
}

// ResendUserInvite gera uma senha temporária nova e reenvia o e-mail de acesso.
// Serve para quem perdeu o convite ou deixou ele expirar: o usuário volta a ser
// obrigado a trocar a senha no primeiro acesso e as sessões antigas caem.
func ResendUserInvite(ctx iris.Context) {
	user, err := models.UserByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if !user.Active {
		badRequest(ctx, "usuário desativado: reative o acesso antes de reenviar a senha")
		return
	}
	// Sem serviço de e-mail a senha seria trocada e ninguém receberia a nova:
	// melhor recusar antes de mexer no banco.
	if services.Mail == nil {
		badRequest(ctx, "serviço de e-mail não configurado")
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

	expiresAt := time.Now().Add(InviteTTL)
	if err := models.ResetUserInvite(utils.DB, user.ID, hash, expiresAt); err != nil {
		serverError(ctx, err)
		return
	}
	// O middleware guarda o usuário por 30s: sem isso a sessão antiga
	// sobreviveria até o cache expirar.
	middleware.InvalidateUser(user.ID)

	subject, html := services.WelcomeEmail(user.Name, tempPassword, utils.AppURL)
	if err := services.Mail.Send(user.Email, user.Name, subject, html); err != nil {
		ctx.Application().Logger().Errorf("falha ao reenviar a senha de %s: %v", user.Email, err)
		ctx.StopWithJSON(iris.StatusBadGateway, iris.Map{
			"error": "a senha foi trocada, mas o e-mail não pôde ser enviado — tente de novo",
		})
		return
	}

	audit(ctx, models.AuditUpdate, "usuario", user.ID,
		"reenviou a senha de acesso para "+user.Email)
	ctx.JSON(iris.Map{"message": "senha nova enviada para " + user.Email})
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

	changes := []string{}
	if user.Role != req.Role {
		changes = append(changes, "perfil "+models.RoleLabel(user.Role)+" → "+models.RoleLabel(req.Role))
	}
	if req.Active != nil && user.Active != *req.Active {
		if *req.Active {
			changes = append(changes, "acesso reativado")
		} else {
			changes = append(changes, "acesso desativado")
		}
	}
	if user.Email != req.Email {
		changes = append(changes, "e-mail "+user.Email+" → "+req.Email)
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
	// Papel e status são lidos do banco a cada requisição com cache curto:
	// limpar a entrada faz a mudança valer na requisição seguinte.
	middleware.InvalidateUser(user.ID)

	summary := "editou o usuário " + user.Email
	if len(changes) > 0 {
		summary += " (" + strings.Join(changes, "; ") + ")"
	}
	audit(ctx, models.AuditUpdate, "usuario", user.ID, summary)

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
	middleware.InvalidateUser(user.ID)
	audit(ctx, models.AuditUpdate, "usuario", user.ID, "desativou o acesso de "+user.Email)
	ctx.JSON(user)
}
