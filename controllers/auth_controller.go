package controllers

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fixpay/fix-crm/middleware"
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login autentica por e-mail e senha e devolve o JWT. Tentativas erradas são
// contadas por IP+e-mail e o acesso é bloqueado temporariamente após o limite.
func Login(ctx iris.Context) {
	var req loginRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "informe e-mail e senha")
		return
	}
	req.Email = models.NormalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" {
		badRequest(ctx, "informe e-mail e senha")
		return
	}

	rateKey := clientIP(ctx) + "|" + req.Email
	if blocked, wait := services.LoginBlocked(rateKey); blocked {
		minutes := int(wait.Minutes()) + 1
		ctx.StopWithJSON(iris.StatusTooManyRequests, iris.Map{
			"error": fmt.Sprintf("muitas tentativas de acesso: tente novamente em %d minuto(s)", minutes),
		})
		return
	}

	fail := func(reason string) {
		if services.RegisterLoginFailure(rateKey) {
			auditLogin(ctx, nil, req.Email, models.AuditLoginFail,
				"bloqueado após "+strconv.Itoa(services.LoginMaxAttempts)+" tentativas inválidas")
		} else {
			auditLogin(ctx, nil, req.Email, models.AuditLoginFail, reason)
		}
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "e-mail ou senha inválidos"})
	}

	user, err := models.UserByEmail(utils.DB, req.Email)
	if err == sql.ErrNoRows {
		fail("e-mail não cadastrado")
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	if !user.Active {
		fail("usuário desativado")
		return
	}
	if !services.CheckPassword(user.PasswordHash, req.Password) {
		fail("senha incorreta")
		return
	}

	// Convite com senha temporária vencida: só volta com um novo link de acesso.
	if user.MustChangePassword && user.InviteExpiresAt != nil && time.Now().After(*user.InviteExpiresAt) {
		auditLogin(ctx, &user.ID, user.Email, models.AuditLoginFail, "convite expirado")
		ctx.StopWithJSON(iris.StatusForbidden, iris.Map{
			"error": "sua senha temporária expirou: use \"Esqueci minha senha\" para definir uma nova",
		})
		return
	}

	services.ClearLoginFailures(rateKey)

	token, err := services.GenerateToken(user, utils.JWTSecret)
	if err != nil {
		serverError(ctx, err)
		return
	}
	// O usuário acabou de vir do banco: aproveita no cache do middleware.
	middleware.CacheUser(user)
	auditLogin(ctx, &user.ID, user.Email, models.AuditLogin, "acesso realizado")
	ctx.JSON(iris.Map{
		"token":                token,
		"user":                 user,
		"must_change_password": user.MustChangePassword,
	})
}

// auditLogin registra tentativas de acesso (o usuário ainda não está autenticado,
// por isso a identificação vem do próprio e-mail informado).
func auditLogin(ctx iris.Context, userID *int64, email, action, summary string) {
	entry := models.AuditEntry{
		UserID:   userID,
		UserName: email,
		Action:   action,
		Entity:   "usuario",
		EntityID: userID,
		Summary:  summary,
		IP:       clientIP(ctx),
	}
	if err := models.RecordAudit(utils.DB, &entry); err != nil {
		ctx.Application().Logger().Errorf("falha ao registrar auditoria de login: %v", err)
	}
}

// Me retorna o usuário autenticado (já carregado pelo middleware).
func Me(ctx iris.Context) {
	user := middleware.CurrentUser(ctx)
	if user == nil {
		notFound(ctx)
		return
	}
	ctx.JSON(user)
}

type updateMeRequest struct {
	Name            string `json:"name"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UpdateMe permite ao usuário alterar o próprio nome e senha.
func UpdateMe(ctx iris.Context) {
	var req updateMeRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	user := middleware.CurrentUser(ctx)
	if user == nil {
		notFound(ctx)
		return
	}

	if req.Name != "" {
		user.Name = strings.TrimSpace(req.Name)
		if err := models.UpdateUser(utils.DB, user); err != nil {
			serverError(ctx, err)
			return
		}
		middleware.InvalidateUser(user.ID)
	}

	if req.NewPassword == "" {
		ctx.JSON(iris.Map{"user": user})
		return
	}

	if len(req.NewPassword) < 8 {
		badRequest(ctx, "a nova senha deve ter pelo menos 8 caracteres")
		return
	}
	if !services.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		badRequest(ctx, "senha atual incorreta")
		return
	}
	hash, err := services.HashPassword(req.NewPassword)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if err := models.UpdateUserPassword(utils.DB, user.ID, hash); err != nil {
		serverError(ctx, err)
		return
	}
	user.PasswordHash = hash
	user.MustChangePassword = false
	user.InviteExpiresAt = nil
	user.PasswordChangedAt = time.Now()
	middleware.CacheUser(user)

	// A troca de senha invalida os tokens emitidos antes, então devolvemos um
	// novo para o usuário continuar na sessão atual.
	token, err := services.GenerateToken(user, utils.JWTSecret)
	if err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "usuario", user.ID, "alterou a própria senha")
	ctx.JSON(iris.Map{"user": user, "token": token})
}

// ForgotPassword gera o token de redefinição e envia por e-mail (Mandrill).
// A resposta é sempre 200 para não revelar quais e-mails existem.
func ForgotPassword(ctx iris.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.Email == "" {
		badRequest(ctx, "informe o e-mail")
		return
	}

	ok := iris.Map{"message": "se o e-mail existir, você receberá o link de redefinição"}

	user, err := models.UserByEmail(utils.DB, req.Email)
	if err != nil {
		ctx.JSON(ok)
		return
	}

	token, err := services.RandomToken()
	if err != nil {
		serverError(ctx, err)
		return
	}
	if err := models.CreatePasswordReset(utils.DB, user.ID, token, time.Now().Add(2*time.Hour)); err != nil {
		serverError(ctx, err)
		return
	}

	subject, html := services.ResetPasswordEmail(user.Name, token, utils.AppURL)
	if services.Mail != nil {
		if err := services.Mail.Send(user.Email, user.Name, subject, html); err != nil {
			ctx.Application().Logger().Errorf("falha ao enviar e-mail de reset: %v", err)
		}
	}
	ctx.JSON(ok)
}

// ResetPassword valida o token recebido por e-mail e define a nova senha.
func ResetPassword(ctx iris.Context) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.Token == "" {
		badRequest(ctx, "token inválido")
		return
	}
	if len(req.Password) < 8 {
		badRequest(ctx, "a senha deve ter pelo menos 8 caracteres")
		return
	}

	reset, err := models.PasswordResetByToken(utils.DB, req.Token)
	if err != nil || reset.Used || time.Now().After(reset.ExpiresAt) {
		badRequest(ctx, "token inválido ou expirado, solicite um novo link")
		return
	}

	hash, err := services.HashPassword(req.Password)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if err := models.UpdateUserPassword(utils.DB, reset.UserID, hash); err != nil {
		serverError(ctx, err)
		return
	}
	if err := models.MarkPasswordResetUsed(utils.DB, reset.ID); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "senha alterada com sucesso"})
}
