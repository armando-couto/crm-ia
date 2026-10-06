import { defineStore } from 'pinia'
import { api, getToken, setToken } from '../api'
import type { User, Workspace as WorkspaceInfo } from '../types'
import { aplicarTema, tenant } from '../tenant'

interface AuthState {
  user: User | null
  permissions: Record<string, boolean>
  loading: boolean
  /** Convite com senha temporária: o acesso fica travado até a troca. */
  mustChangePassword: boolean
  /** Identidade do ambiente (nome, cor, logo, setup) vinda da API. */
  workspace: WorkspaceInfo | null
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    user: null,
    permissions: {},
    loading: false,
    mustChangePassword: false,
    workspace: null
  }),

  getters: {
    isAuthenticated: (s) => !!s.user,
    isAdmin: (s) => s.user?.role === 'admin',
    /** can informa se o perfil do usuário tem a permissão (admin sempre tem). */
    can:
      (s) =>
      (permission: string): boolean => {
        if (s.user?.role === 'admin') return true
        return s.permissions[permission] === true
      },
    /** Nome da empresa para o shell (o do servidor até a API responder). */
    workspaceName: (s) => s.workspace?.name || tenant.name,
    workspaceLogo: (s) => s.workspace?.logo_url ?? tenant.logo_url,
    /** O assistente ainda não foi concluído neste ambiente. */
    setupPending: (s) => (s.workspace ? !s.workspace.setup_done : !tenant.setup_done),
    /** Atalho para telas de configuração em geral. */
    canManage(): boolean {
      return (
        this.can('settings.users') ||
        this.can('settings.pipelines') ||
        this.can('settings.properties') ||
        this.can('settings.permissions') ||
        this.can('settings.forms') ||
        this.can('settings.audit') ||
        this.can('records.merge')
      )
    }
  },

  actions: {
    async login(email: string, password: string): Promise<void> {
      this.loading = true
      try {
        const resp = await api.post<{ token: string; user: User; must_change_password?: boolean }>(
          '/auth/login',
          { email, password }
        )
        setToken(resp.token)
        this.user = resp.user
        this.mustChangePassword = resp.must_change_password === true
        if (!this.mustChangePassword) {
          await Promise.all([this.fetchPermissions(), this.fetchWorkspace()])
        }
      } finally {
        this.loading = false
      }
    },

    async fetchMe(): Promise<void> {
      if (!getToken()) return
      try {
        this.user = await api.get<User>('/me')
        this.mustChangePassword = this.user.must_change_password === true
        if (!this.mustChangePassword) {
          await Promise.all([this.fetchPermissions(), this.fetchWorkspace()])
        }
      } catch {
        this.user = null
        this.permissions = {}
      }
    },

    /** Lê a identidade do ambiente e aplica a marca do cliente. */
    async fetchWorkspace(): Promise<void> {
      try {
        this.workspace = await api.get<WorkspaceInfo>('/me/workspace')
        aplicarTema(this.workspace.color, this.workspace.logo_url)
      } catch {
        /* fica com a identidade injetada no boot */
      }
    },

    async fetchPermissions(): Promise<void> {
      try {
        const resp = await api.get<{ role: string; permissions: Record<string, boolean> }>('/me/permissions')
        this.permissions = resp.permissions ?? {}
      } catch {
        this.permissions = {}
      }
    },

    /**
     * changePassword troca a senha do usuário logado. O backend invalida os
     * tokens emitidos antes da troca, então guardamos o token novo que vem
     * na resposta para a sessão continuar de pé.
     */
    async changePassword(currentPassword: string, newPassword: string, name?: string): Promise<void> {
      const resp = await api.put<{ user: User; token?: string }>('/me', {
        name: name ?? '',
        current_password: currentPassword,
        new_password: newPassword
      })
      if (resp.token) setToken(resp.token)
      this.user = resp.user
      this.mustChangePassword = false
      await this.fetchPermissions()
    },

    logout(): void {
      setToken(null)
      this.user = null
      this.permissions = {}
      this.mustChangePassword = false
      this.workspace = null
    }
  }
})
