package controllers

import (
	"database/sql"
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

// Login autentica por e-mail e senha e devolve o JWT.
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

	user, err := models.UserByEmail(utils.DB, req.Email)
	if err == sql.ErrNoRows {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "e-mail ou senha inválidos"})
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	if !user.Active || !services.CheckPassword(user.PasswordHash, req.Password) {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "e-mail ou senha inválidos"})
		return
	}

	token, err := services.GenerateToken(user, utils.JWTSecret)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"token": token, "user": user})
}

// Me retorna o usuário autenticado.
func Me(ctx iris.Context) {
	claims := middleware.CurrentClaims(ctx)
	user, err := models.UserByID(utils.DB, claims.UserID)
	if err != nil {
		handleDBError(ctx, err)
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
	claims := middleware.CurrentClaims(ctx)
	var req updateMeRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	user, err := models.UserByID(utils.DB, claims.UserID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	if req.Name != "" {
		user.Name = strings.TrimSpace(req.Name)
		if err := models.UpdateUser(utils.DB, user); err != nil {
			serverError(ctx, err)
			return
		}
	}

	if req.NewPassword != "" {
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
	}
	ctx.JSON(user)
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
