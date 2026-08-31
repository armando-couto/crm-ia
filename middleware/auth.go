package middleware

import (
	"database/sql"
	"strings"
	"sync"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

const (
	claimsKey = "auth.claims"
	userKey   = "auth.user"
)

// Cache curto do usuário: mantém o papel e o status vindos do banco sem
// consultar a cada requisição. Alterações de perfil valem em segundos, e o
// controller invalida a entrada quando edita o usuário.
const userCacheTTL = 30 * time.Second

type cachedUser struct {
	user     *models.User
	loadedAt time.Time
}

var (
	userMu    sync.RWMutex
	userCache = map[int64]cachedUser{}
)

// InvalidateUser derruba o cache de um usuário (efeito imediato ao mudar
// perfil, desativar ou alterar a senha).
func InvalidateUser(userID int64) {
	userMu.Lock()
	delete(userCache, userID)
	userMu.Unlock()
}

// CacheUser guarda um usuário já lido do banco. O login usa isso para evitar
// uma consulta extra na requisição seguinte.
func CacheUser(u *models.User) {
	if u == nil {
		return
	}
	copied := *u
	userMu.Lock()
	userCache[u.ID] = cachedUser{user: &copied, loadedAt: time.Now()}
	userMu.Unlock()
}

// ResetUserCache limpa o cache inteiro (usado nos testes).
func ResetUserCache() {
	userMu.Lock()
	userCache = map[int64]cachedUser{}
	userMu.Unlock()
}

func loadUser(id int64) (*models.User, error) {
	userMu.RLock()
	entry, ok := userCache[id]
	userMu.RUnlock()
	if ok && time.Since(entry.loadedAt) < userCacheTTL {
		// Cópia por requisição: o handler não altera o registro em cache.
		copied := *entry.user
		return &copied, nil
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
