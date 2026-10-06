import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import { getToken, setToken } from '../src/api'

const user = {
  id: 1,
  name: 'Ana',
  email: 'ana@exemplo.com.br',
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
    await store.login('ana@exemplo.com.br', 'senha')

    expect(getToken()).toBe('jwt-abc')
    expect(store.user?.email).toBe('ana@exemplo.com.br')
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

  it('sinaliza troca obrigatória quando o login vem de um convite', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: () =>
          Promise.resolve(
            JSON.stringify({
              token: 'jwt-temp',
              user: { ...user, must_change_password: true },
              must_change_password: true
            })
          )
      })
    )

    const store = useAuthStore()
    await store.login('novo@exemplo.com.br', 'temporaria')

    expect(store.mustChangePassword).toBe(true)
  })

  it('troca de senha guarda o token novo e libera o acesso', async () => {
    setToken('token-antigo')
    const fetchMock = vi.fn().mockImplementation((_url: string, options?: any) => {
      if (options?.method === 'PUT') {
        return Promise.resolve({
          ok: true,
          status: 200,
          text: () =>
            Promise.resolve(
              JSON.stringify({ user: { ...user, must_change_password: false }, token: 'token-novo' })
            )
        })
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: () => Promise.resolve(JSON.stringify({ role: 'admin', permissions: {} }))
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    const store = useAuthStore()
    store.mustChangePassword = true
    await store.changePassword('temporaria', 'senha-nova-forte')

    expect(getToken()).toBe('token-novo')
    expect(store.mustChangePassword).toBe(false)
  })
})
