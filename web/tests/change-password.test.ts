import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import { getToken, setToken } from '../src/api'
import ChangePasswordView from '../src/views/ChangePasswordView.vue'

const push = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
  useRoute: () => ({ params: {}, query: {} })
}))

describe('ChangePasswordView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    setToken('token-temporario')
    push.mockClear()
    vi.restoreAllMocks()
  })

  async function fill(wrapper: any, current: string, next: string, confirm: string) {
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue(current)
    await inputs[1].setValue(next)
    await inputs[2].setValue(confirm)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
  }

  it('recusa senha curta sem chamar a API', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(ChangePasswordView)
    await fill(wrapper, 'temporaria', 'curta', 'curta')

    expect(fetchMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('pelo menos 8 caracteres')
  })

  it('recusa quando a confirmação não confere', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(ChangePasswordView)
    await fill(wrapper, 'temporaria', 'senha-nova-forte', 'outra-senha-forte')

    expect(fetchMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('as senhas não conferem')
  })

  it('troca a senha, guarda o token novo e vai para o início', async () => {
    const fetchMock = vi.fn().mockImplementation((_url: string, options?: any) => {
      if (options?.method === 'PUT') {
        return Promise.resolve({
          ok: true,
          status: 200,
          text: () =>
            Promise.resolve(
              JSON.stringify({
                user: { id: 5, name: 'Novo', email: 'n@fixpay.com.br', role: 'seller', active: true },
                token: 'token-definitivo'
              })
            )
        })
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: () => Promise.resolve(JSON.stringify({ role: 'seller', permissions: { 'contacts.view': true } }))
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    const auth = useAuthStore()
    auth.mustChangePassword = true

    const wrapper = mount(ChangePasswordView)
    await fill(wrapper, 'temporaria', 'senha-nova-forte', 'senha-nova-forte')

    expect(getToken()).toBe('token-definitivo')
    expect(auth.mustChangePassword).toBe(false)
    expect(push).toHaveBeenCalledWith('/')
  })

  it('mostra o erro devolvido pela API', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        text: () => Promise.resolve(JSON.stringify({ error: 'senha atual incorreta' }))
      })
    )

    const wrapper = mount(ChangePasswordView)
    await fill(wrapper, 'errada', 'senha-nova-forte', 'senha-nova-forte')

    expect(wrapper.text()).toContain('senha atual incorreta')
    expect(push).not.toHaveBeenCalled()
  })
})
