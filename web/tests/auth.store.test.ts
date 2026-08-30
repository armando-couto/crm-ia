import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import { getToken, setToken } from '../src/api'

const user = {
  id: 1,
  name: 'Ana',
  email: 'ana@fixpay.com.br',
  role: 'admin',
  active: true,
  created_at: '',
  updated_at: ''
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    setToken(null)
    vi.restoreAllMocks()
  })

  it('faz login e guarda token e usuário', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: () => Promise.resolve(JSON.stringify({ token: 'jwt-abc', user }))
      })
    )

    const store = useAuthStore()
    await store.login('ana@fixpay.com.br', 'senha')

    expect(getToken()).toBe('jwt-abc')
    expect(store.user?.email).toBe('ana@fixpay.com.br')
    expect(store.isAuthenticated).toBe(true)
    expect(store.isAdmin).toBe(true)
    expect(store.canManage).toBe(true)
  })

  it('propaga erro de credenciais inválidas', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        text: () => Promise.resolve(JSON.stringify({ error: 'e-mail ou senha inválidos' }))
      })
    )

    const store = useAuthStore()
    await expect(store.login('x@y.z', 'errada')).rejects.toThrowError()
    expect(store.user).toBeNull()
    expect(store.loading).toBe(false)
  })

  it('logout limpa token e usuário', async () => {
    setToken('algum-token')
    const store = useAuthStore()
    store.user = user as any

    store.logout()

    expect(getToken()).toBeNull()
    expect(store.user).toBeNull()
    expect(store.isAuthenticated).toBe(false)
  })

  it('vendedor não pode gerenciar', () => {
    const store = useAuthStore()
    store.user = { ...user, role: 'vendedor' } as any
    expect(store.canManage).toBe(false)
    expect(store.isAdmin).toBe(false)
  })
})
