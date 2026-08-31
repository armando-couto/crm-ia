package models

import (
	"database/sql"
	"fmt"
	"slices"
	"sync"
)

// Catálogo de permissões: cada rota protegida exige uma destas chaves.
const (
	PermContactsView   = "contacts.view"
	PermContactsEdit   = "contacts.edit"
	PermContactsDelete = "contacts.delete"
	PermContactsImport = "contacts.import"
	PermContactsExport = "contacts.export"

	PermCompaniesView   = "companies.view"
	PermCompaniesEdit   = "companies.edit"
	PermCompaniesDelete = "companies.delete"

	PermDealsView   = "deals.view"
	PermDealsEdit   = "deals.edit"
	PermDealsDelete = "deals.delete"
	PermDealsExport = "deals.export"

	PermTicketsView   = "tickets.view"
	PermTicketsEdit   = "tickets.edit"
	PermTicketsDelete = "tickets.delete"

	PermTasksView = "tasks.view"
	PermTasksEdit = "tasks.edit"

	PermListsView    = "lists.view"
	PermListsManage  = "lists.manage"
	PermProjectsView = "projects.view"
	PermProjectsEdit = "projects.edit"

	PermInboxView      = "inbox.view"
	PermInboxReply     = "inbox.reply"
	PermEmailSend      = "email.send"
	PermCallsView      = "calls.view"
	PermCallsLog       = "calls.log"
	PermMeetingsView   = "meetings.view"
	PermMeetingsManage = "meetings.manage"

	PermLibraryView   = "library.view"
	PermLibraryManage = "library.manage"

	PermFilesView   = "files.view"
	PermFilesManage = "files.manage"

	PermRecordsMerge = "records.merge"

	PermAutomationsView   = "automations.view"
	PermAutomationsManage = "automations.manage"

	PermDashboardView = "dashboard.view"
	PermViewsManage   = "views.manage"

	PermSettingsUsers       = "settings.users"
	PermSettingsPipelines   = "settings.pipelines"
	PermSettingsProperties  = "settings.properties"
	PermSettingsForms       = "settings.forms"
	PermSettingsPermissions = "settings.permissions"
	PermSettingsAudit       = "settings.audit"
)

// Perfis de acesso.
const (
	RoleSeller  = "seller"
	RoleManager = "manager"
	RoleAdmin   = "admin"
)

var Roles = []string{RoleSeller, RoleManager, RoleAdmin}

func ValidRole(role string) bool {
	return slices.Contains(Roles, role)
}

// RoleLabels são os nomes exibidos na interface.
var RoleLabels = map[string]string{
	RoleSeller:  "Seller",
	RoleManager: "Manager",
	RoleAdmin:   "Admin",
}

// RoleLabel devolve o nome exibido do perfil (ou o próprio código, se novo).
func RoleLabel(role string) string {
	if label, ok := RoleLabels[role]; ok {
		return label
	}
	return role
}

// PermissionDef descreve uma permissão para a tela de configuração.
type PermissionDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"`
}

// PermissionCatalog é a lista fechada de permissões do sistema, na ordem em
// que aparece na tela de configuração.
var PermissionCatalog = []PermissionDef{
	{PermContactsView, "Ver contatos", "Contatos"},
	{PermContactsEdit, "Criar e editar contatos", "Contatos"},
	{PermContactsDelete, "Excluir contatos", "Contatos"},
	{PermContactsImport, "Importar contatos", "Contatos"},
	{PermContactsExport, "Exportar contatos", "Contatos"},

	{PermCompaniesView, "Ver empresas", "Empresas"},
	{PermCompaniesEdit, "Criar e editar empresas", "Empresas"},
	{PermCompaniesDelete, "Excluir empresas", "Empresas"},

	{PermDealsView, "Ver negócios", "Negócios"},
	{PermDealsEdit, "Criar, editar e mover negócios", "Negócios"},
	{PermDealsDelete, "Excluir negócios", "Negócios"},
	{PermDealsExport, "Exportar negócios", "Negócios"},

	{PermTicketsView, "Ver tickets", "Tickets"},
	{PermTicketsEdit, "Criar e editar tickets", "Tickets"},
	{PermTicketsDelete, "Excluir tickets", "Tickets"},

	{PermTasksView, "Ver tarefas", "Tarefas e projetos"},
	{PermTasksEdit, "Criar e editar tarefas", "Tarefas e projetos"},
	{PermProjectsView, "Ver projetos", "Tarefas e projetos"},
	{PermProjectsEdit, "Criar e editar projetos", "Tarefas e projetos"},

	{PermListsView, "Ver listas", "Listas e visualizações"},
	{PermListsManage, "Criar e editar listas", "Listas e visualizações"},
	{PermViewsManage, "Criar e editar visualizações salvas", "Listas e visualizações"},

	{PermInboxView, "Ver caixa de entrada", "Comunicação"},
	{PermInboxReply, "Responder conversas", "Comunicação"},
	{PermEmailSend, "Enviar e-mails aos contatos", "Comunicação"},
	{PermCallsView, "Ver chamadas", "Comunicação"},
	{PermCallsLog, "Registrar chamadas", "Comunicação"},
	{PermMeetingsView, "Ver reuniões", "Comunicação"},
	{PermMeetingsManage, "Agendar e editar reuniões", "Comunicação"},

	{PermLibraryView, "Ver manuais, modelos e snippets", "Biblioteca"},
	{PermLibraryManage, "Criar e editar a biblioteca", "Biblioteca"},

	{PermFilesView, "Ver e baixar anexos", "Arquivos"},
	{PermFilesManage, "Anexar e remover arquivos", "Arquivos"},

	{PermAutomationsView, "Ver automações", "Automações"},
	{PermAutomationsManage, "Criar e editar automações", "Automações"},

	{PermRecordsMerge, "Mesclar registros duplicados", "Manutenção"},

	{PermDashboardView, "Ver dashboard e métricas", "Relatórios"},

	{PermSettingsUsers, "Gerenciar usuários e equipes", "Configurações"},
	{PermSettingsPipelines, "Gerenciar pipelines e fases", "Configurações"},
	{PermSettingsProperties, "Gerenciar propriedades", "Configurações"},
	{PermSettingsForms, "Personalizar formulários", "Configurações"},
	{PermSettingsPermissions, "Gerenciar permissões", "Configurações"},
	{PermSettingsAudit, "Ver trilha de auditoria", "Configurações"},
}

func ValidPermission(key string) bool {
	for _, p := range PermissionCatalog {
		if p.Key == key {
			return true
		}
	}
	return false
}

// sellerDefaults são as permissões padrão do Seller: opera a própria carteira,
// sem excluir registros nem acessar configurações.
var sellerDefaults = []string{
	PermContactsView, PermContactsEdit, PermContactsExport,
	PermCompaniesView, PermCompaniesEdit,
	PermDealsView, PermDealsEdit,
	PermTicketsView, PermTicketsEdit,
	PermTasksView, PermTasksEdit,
	PermProjectsView,
	PermListsView, PermViewsManage,
	PermInboxView, PermInboxReply, PermEmailSend,
	PermCallsView, PermCallsLog,
	PermMeetingsView, PermMeetingsManage,
	PermLibraryView,
	PermFilesView, PermFilesManage,
	PermDashboardView,
}

// managerDefaults: tudo do Seller mais exclusões, importação e configurações
// operacionais (pipelines, propriedades, formulários, listas).
var managerDefaults = append(append([]string{}, sellerDefaults...),
	PermContactsDelete, PermContactsImport,
	PermCompaniesDelete,
	PermDealsDelete, PermDealsExport,
	PermTicketsDelete,
	PermProjectsEdit,
	PermListsManage,
	PermLibraryManage,
	PermSettingsPipelines, PermSettingsProperties, PermSettingsForms,
	PermRecordsMerge,
	PermAutomationsView, PermAutomationsManage,
)

// DefaultPermissions devolve a matriz padrão (Admin tem tudo).
func DefaultPermissions() map[string]map[string]bool {
	matrix := map[string]map[string]bool{
		RoleSeller:  {},
		RoleManager: {},
		RoleAdmin:   {},
	}
	for _, p := range PermissionCatalog {
		matrix[RoleSeller][p.Key] = slices.Contains(sellerDefaults, p.Key)
		matrix[RoleManager][p.Key] = slices.Contains(managerDefaults, p.Key)
		matrix[RoleAdmin][p.Key] = true
	}
	return matrix
}

// Cache em memória da matriz efetiva, recarregado a cada gravação.
var (
	permMu    sync.RWMutex
	permCache map[string]map[string]bool
)

// LoadPermissions lê a matriz do banco sobre os padrões e atualiza o cache.
func LoadPermissions(db *sql.DB) (map[string]map[string]bool, error) {
	matrix := DefaultPermissions()

	rows, err := db.Query(`SELECT role, permission, allowed FROM role_permissions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role, permission string
		var allowed bool
		if err := rows.Scan(&role, &permission, &allowed); err != nil {
			return nil, err
		}
		if _, ok := matrix[role]; ok && ValidPermission(permission) {
			matrix[role][permission] = allowed
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// O Admin nunca perde acesso: evita a equipe se trancar para fora.
	for _, p := range PermissionCatalog {
		matrix[RoleAdmin][p.Key] = true
	}

	permMu.Lock()
	permCache = matrix
	permMu.Unlock()
	return matrix, nil
}

// SavePermissions grava as permissões de um perfil (Admin não é editável).
func SavePermissions(db *sql.DB, role string, perms map[string]bool) error {
	if !ValidRole(role) {
		return fmt.Errorf("perfil inválido")
	}
	if role == RoleAdmin {
		return fmt.Errorf("o perfil Admin sempre tem acesso total")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for key, allowed := range perms {
		if !ValidPermission(key) {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO role_permissions (role, permission, allowed, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (role, permission) DO UPDATE SET allowed = EXCLUDED.allowed, updated_at = NOW()`,
			role, key, allowed); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = LoadPermissions(db)
	return err
}

// RoleCan responde se o perfil tem a permissão, usando o cache carregado na
// inicialização (cai nos padrões enquanto o cache estiver vazio).
func RoleCan(role, permission string) bool {
	if role == RoleAdmin {
		return true
	}
	permMu.RLock()
	matrix := permCache
	permMu.RUnlock()
	if matrix == nil {
		matrix = DefaultPermissions()
	}
	perms, ok := matrix[role]
	if !ok {
		return false
	}
	return perms[permission]
}

// RolePermissions devolve as permissões efetivas de um perfil (para o front).
func RolePermissions(role string) map[string]bool {
	permMu.RLock()
	matrix := permCache
	permMu.RUnlock()
	if matrix == nil {
		matrix = DefaultPermissions()
	}
	if perms, ok := matrix[role]; ok {
		out := make(map[string]bool, len(perms))
		for k, v := range perms {
			out[k] = v
		}
		return out
	}
	return map[string]bool{}
}

// PermissionMatrix devolve a matriz completa (para a tela de configuração).
func PermissionMatrix(db *sql.DB) (map[string]map[string]bool, error) {
	permMu.RLock()
	matrix := permCache
	permMu.RUnlock()
	if matrix != nil {
		return matrix, nil
	}
	return LoadPermissions(db)
}
