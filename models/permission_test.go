package models

import "testing"

func TestDefaultPermissions(t *testing.T) {
	matrix := DefaultPermissions()

	// Admin tem tudo.
	for _, p := range PermissionCatalog {
		if !matrix[RoleAdmin][p.Key] {
			t.Fatalf("admin deveria ter %s por padrão", p.Key)
		}
	}

	// Seller opera a carteira, mas não exclui nem configura.
	if !matrix[RoleSeller][PermContactsEdit] || !matrix[RoleSeller][PermDealsEdit] {
		t.Error("seller deveria criar/editar contatos e negócios")
	}
	if matrix[RoleSeller][PermContactsDelete] || matrix[RoleSeller][PermDealsDelete] {
		t.Error("seller não deveria excluir registros por padrão")
	}
	if matrix[RoleSeller][PermSettingsUsers] || matrix[RoleSeller][PermSettingsPipelines] {
		t.Error("seller não deveria acessar configurações")
	}

	// Manager herda o seller e ganha exclusões e configurações operacionais.
	for _, p := range sellerDefaults {
		if !matrix[RoleManager][p] {
			t.Errorf("manager deveria herdar a permissão %s do seller", p)
		}
	}
	if !matrix[RoleManager][PermContactsDelete] || !matrix[RoleManager][PermSettingsPipelines] {
		t.Error("manager deveria excluir contatos e gerenciar pipelines")
	}
	if matrix[RoleManager][PermSettingsUsers] || matrix[RoleManager][PermSettingsPermissions] {
		t.Error("gerenciar usuários e permissões deveria ser exclusivo do admin por padrão")
	}
}

func TestRoleCanFallsBackToDefaults(t *testing.T) {
	permMu.Lock()
	permCache = nil // simula cache ainda não carregado
	permMu.Unlock()

	if !RoleCan(RoleAdmin, PermSettingsPermissions) {
		t.Error("admin sempre pode tudo")
	}
	if !RoleCan(RoleSeller, PermContactsView) {
		t.Error("seller deveria ver contatos pelo padrão")
	}
	if RoleCan(RoleSeller, PermSettingsUsers) {
		t.Error("seller não deveria gerenciar usuários")
	}
	if RoleCan("desconhecido", PermContactsView) {
		t.Error("perfil inexistente não deveria ter permissão")
	}
}

func TestValidRoleAndPermission(t *testing.T) {
	for _, r := range []string{RoleSeller, RoleManager, RoleAdmin} {
		if !ValidRole(r) {
			t.Errorf("%q deveria ser um perfil válido", r)
		}
	}
	if ValidRole("vendedor") || ValidRole("") {
		t.Error("perfis antigos/vazios não são mais válidos")
	}
	if !ValidPermission(PermDealsEdit) || ValidPermission("deals.hack") {
		t.Error("validação de permissão incorreta")
	}
}

// A matriz efetiva nunca pode remover acesso do Admin (evita trancar a equipe).
func TestAdminAlwaysFullAccess(t *testing.T) {
	permMu.Lock()
	permCache = map[string]map[string]bool{
		RoleAdmin:  {PermSettingsPermissions: false},
		RoleSeller: {},
	}
	permMu.Unlock()
	defer func() {
		permMu.Lock()
		permCache = nil
		permMu.Unlock()
	}()

	if !RoleCan(RoleAdmin, PermSettingsPermissions) {
		t.Fatal("admin deve manter acesso total mesmo se a matriz disser o contrário")
	}
}
