package middleware

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

const (
	claimsKey = "auth.claims"
	userKey   = "auth.user"
)

// Cache curto do usuário no store do ambiente (Redis em produção): mantém o
// papel e o status vindos do banco sem consultar a cada requisição e vale para
// todas as réplicas da API. Alterações de perfil valem em segundos, e o
// controller invalida a entrada quando edita o usuário.
const userCacheTTL = 30 * time.Second

func chaveUsuario(id int64) string { return "usuario:" + strconv.FormatInt(id, 10) }

// InvalidateUser derruba o cache de um usuário (efeito imediato ao mudar
// perfil, desativar ou alterar a senha).
func InvalidateUser(userID int64) {
	utils.Cache.Del(chaveUsuario(userID))
}

// CacheUser guarda um usuário já lido do banco. O login usa isso para evitar
// uma consulta extra na requisição seguinte.
func CacheUser(u *models.User) {
	if u == nil {
		return
	}
	b, err := json.Marshal(cachedUser{User: u, PasswordHash: u.PasswordHash, PasswordChangedAt: u.PasswordChangedAt})
	if err != nil {
		return
	}
	utils.Cache.Set(chaveUsuario(u.ID), string(b), userCacheTTL)
}

// cachedUser leva junto o que o JSON do usuário omite (hash e data da senha):
// o middleware precisa deles para cortar sessões antigas.
type cachedUser struct {
	*models.User
	PasswordHash      string    `json:"password_hash"`
	PasswordChangedAt time.Time `json:"password_changed_at"`
}

// ResetUserCache limpa o cache inteiro (usado nos testes).
func ResetUserCache() {
	utils.Cache.DelPrefixo("usuario:")
}

func loadUser(id int64) (*models.User, error) {
	if raw, ok := utils.Cache.Get(chaveUsuario(id)); ok {
		var c cachedUser
		if err := json.Unmarshal([]byte(raw), &c); err == nil && c.User != nil {
			u := *c.User
			u.PasswordHash = c.PasswordHash
			u.PasswordChangedAt = c.PasswordChangedAt
			return &u, nil
		}
	}
	user, err := models.UserByID(utils.DB, id)
	if err != nil {
		return nil, err
	}
	CacheUser(user)
	return user, nil
}

// JWTAuth valida o Bearer token e injeta as claims no contexto. O papel e o
// status vêm do banco (não do token), então mudanças de perfil, desativação e
// troca de senha têm efeito quase imediato.
func JWTAuth(ctx iris.Context) {
	header := ctx.GetHeader("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || token == "" {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}

	claims, err := services.ParseToken(token, utils.JWTSecret)
	if err != nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "sessão expirada, faça login novamente"})
		return
	}

	user, err := loadUser(claims.UserID)
	if err == sql.ErrNoRows {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "usuário não encontrado"})
		return
	}
	if err != nil {
		ctx.StopWithJSON(iris.StatusInternalServerError, iris.Map{"error": "erro ao validar a sessão"})
		return
	}
	if !user.Active {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "acesso desativado, procure um administrador"})
		return
	}

	// Trocar a senha invalida os tokens emitidos antes.
	if claims.IssuedAt != nil && user.PasswordChangedAt.After(claims.IssuedAt.Time.Add(time.Second)) {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "sua senha mudou: faça login novamente"})
		return
	}

	// O papel do banco prevalece sobre o gravado no token.
	claims.Role = user.Role

	// Com troca de senha pendente, só o perfil e a própria troca ficam liberados.
	if user.MustChangePassword && !allowedWhilePasswordPending(ctx) {
		ctx.StopWithJSON(iris.StatusForbidden, iris.Map{
			"error":                "defina uma nova senha antes de continuar",
			"must_change_password": true,
		})
		return
	}

	ctx.Values().Set(claimsKey, claims)
	ctx.Values().Set(userKey, user)
	ctx.Next()
}

// allowedWhilePasswordPending libera apenas o necessário para trocar a senha.
func allowedWhilePasswordPending(ctx iris.Context) bool {
	path := ctx.Path()
	return path == "/api/v1/me" || path == "/api/v1/me/permissions"
}

// CurrentClaims retorna as claims do usuário autenticado na requisição.
func CurrentClaims(ctx iris.Context) *services.Claims {
	if v := ctx.Values().Get(claimsKey); v != nil {
		if c, ok := v.(*services.Claims); ok {
			return c
		}
	}
	return nil
}

// CurrentUser retorna o usuário carregado do banco nesta requisição.
func CurrentUser(ctx iris.Context) *models.User {
	if v := ctx.Values().Get(userKey); v != nil {
		if u, ok := v.(*models.User); ok {
			return u
		}
	}
	return nil
}

// RequirePermission limita a rota a quem tem a permissão no seu perfil.
// O Admin sempre passa (models.RoleCan).
func RequirePermission(permission string) iris.Handler {
	return func(ctx iris.Context) {
		claims := CurrentClaims(ctx)
		if claims == nil {
			ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
			return
		}
		if !models.RoleCan(claims.Role, permission) {
			ctx.StopWithJSON(iris.StatusForbidden, iris.Map{
				"error": "seu perfil não tem permissão para esta ação",
			})
			return
		}
		ctx.Next()
	}
}

// RequireRoles limita a rota aos papéis informados (admin sempre passa).
func RequireRoles(roles ...string) iris.Handler {
	return func(ctx iris.Context) {
		claims := CurrentClaims(ctx)
		if claims == nil {
			ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
			return
		}
		if claims.Role == models.RoleAdmin {
			ctx.Next()
			return
		}
		for _, r := range roles {
			if claims.Role == r {
				ctx.Next()
				return
			}
		}
		ctx.StopWithJSON(iris.StatusForbidden, iris.Map{"error": "você não tem permissão para esta ação"})
	}
}
