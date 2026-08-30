package middleware

import (
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

const claimsKey = "auth.claims"

// JWTAuth valida o Bearer token e injeta as claims no contexto.
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

	ctx.Values().Set(claimsKey, claims)
	ctx.Next()
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
