package controllers

import (
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// MyPermissions devolve as permissões efetivas do usuário logado (para o front
// esconder menus e botões conforme o perfil).
func MyPermissions(ctx iris.Context) {
	claims := middlewareClaims(ctx)
	ctx.JSON(iris.Map{
		"role":        claims.Role,
		"permissions": models.RolePermissions(claims.Role),
	})
}

// GetPermissions devolve o catálogo e a matriz completa (tela de configuração).
func GetPermissions(ctx iris.Context) {
	matrix, err := models.PermissionMatrix(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"catalog": models.PermissionCatalog,
		"roles":   models.Roles,
		"labels":  models.RoleLabels,
		"matrix":  matrix,
	})
}

// UpdatePermissions grava as permissões de um perfil (Admin é sempre total).
func UpdatePermissions(ctx iris.Context) {
	var req struct {
		Role        string          `json:"role"`
		Permissions map[string]bool `json:"permissions"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if err := models.SavePermissions(utils.DB, req.Role, req.Permissions); err != nil {
		badRequest(ctx, err.Error())
		return
	}

	matrix, err := models.PermissionMatrix(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"matrix": matrix})
}
