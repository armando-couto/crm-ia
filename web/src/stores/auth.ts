import { defineStore } from 'pinia'
import { api, getToken, setToken } from '../api'
import type { User } from '../types'

interface AuthState {
  user: User | null
  permissions: Record<string, boolean>
  loading: boolean
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    user: null,
    permissions: {},
    loading: false
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
        this.can('settings.forms')
      )
    }
  },

  actions: {
    async login(email: string, password: string): Promise<void> {
      this.loading = true
      try {
        const resp = await api.post<{ token: string; user: User }>('/auth/login', { email, password })
        setToken(resp.token)
        this.user = resp.user
        await this.fetchPermissions()
      } finally {
        this.loading = false
      }
    },

    async fetchMe(): Promise<void> {
      if (!getToken()) return
      try {
        this.user = await api.get<User>('/me')
        await this.fetchPermissions()
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

    logout(): void {
      setToken(null)
      this.user = null
      this.permissions = {}
    }
  }
})
