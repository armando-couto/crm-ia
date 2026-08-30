import { defineStore } from 'pinia'
import { api, getToken, setToken } from '../api'
import type { User } from '../types'

interface AuthState {
  user: User | null
  loading: boolean
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    user: null,
    loading: false
  }),

  getters: {
    isAuthenticated: (s) => !!s.user,
    isAdmin: (s) => s.user?.role === 'admin',
    canManage: (s) => s.user?.role === 'admin' || s.user?.role === 'gestor'
  },

  actions: {
    async login(email: string, password: string): Promise<void> {
      this.loading = true
      try {
        const resp = await api.post<{ token: string; user: User }>('/auth/login', { email, password })
        setToken(resp.token)
        this.user = resp.user
      } finally {
        this.loading = false
      }
    },

    async fetchMe(): Promise<void> {
      if (!getToken()) return
      try {
        this.user = await api.get<User>('/me')
      } catch {
        this.user = null
      }
    },

    logout(): void {
      setToken(null)
      this.user = null
    }
  }
})
