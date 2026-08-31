import { defineStore } from 'pinia'
import { api, getToken, setToken } from '../api'
import type { User } from '../types'

interface AuthState {
  user: User | null
  permissions: Record<string, boolean>
  loading: boolean
  /** Convite com senha temporária: o acesso fica travado até a troca. */
  mustChangePassword: boolean
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    user: null,
    permissions: {},
    loading: false,
    mustChangePassword: false
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
    /** Atalho para telas de configuração em geral. */
    canManage(): boolean {
      return (
        this.can('settings.users') ||
        this.can('settings.pipelines') ||
        this.can('settings.properties') ||
        this.can('settings.permissions') ||
        this.can('settings.forms') ||
        this.can('settings.audit')
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
        if (!this.mustChangePassword) await this.fetchPermissions()
      } finally {
        this.loading = false
      }
    },

    async fetchMe(): Promise<void> {
      if (!getToken()) return
      try {
        this.user = await api.get<User>('/me')
        this.mustChangePassword = this.user.must_change_password === true
        if (!this.mustChangePassword) await this.fetchPermissions()
      } catch {
        this.user = null
        this.permissions = {}
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
    }
  }
})
