import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SavedViewsView from '../src/views/SavedViewsView.vue'

const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
  useRoute: () => ({ params: {}, query: {} })
}))

const views = [
  {
    id: 1,
    entity: 'deals',
    name: 'Clientes acima de R$ 20.000,00',
    filters: { af: [] },
    position: 0,
    created_by: 1,
    created_by_name: 'Eliseu Becco',
    created_at: new Date().toISOString()
  },
  {
    id: 2,
    entity: 'companies',
    name: 'Clientes ativos',
    filters: {},
    position: 0,
    created_by: 1,
    created_by_name: 'Administrador',
    created_at: new Date().toISOString()
  },
  {
    id: 3,
    entity: 'contacts',
    name: 'Leads sem dono',
    filters: {},
    position: 0,
    created_by: null,
    created_by_name: '',
    created_at: new Date().toISOString()
  }
]

describe('SavedViewsView (Todas as visualizações)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    push.mockClear()
  })

  function build(fetchImpl?: any) {
    vi.stubGlobal(
      'fetch',
      fetchImpl ??
        vi.fn().mockResolvedValue({
          ok: true,
          status: 200,
          text: () => Promise.resolve(JSON.stringify(views))
        })
    )
    return mount(SavedViewsView, {
      global: { stubs: { Teleport: true } }
    })
  }

  it('lista todas as visualizações com objeto e proprietário', async () => {
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Clientes acima de R$ 20.000,00')
    expect(wrapper.text()).toContain('Negócios')
    expect(wrapper.text()).toContain('Empresas')
    expect(wrapper.text()).toContain('Contatos')
    expect(wrapper.text()).toContain('Eliseu Becco')
    wrapper.unmount()
  })

  it('filtra por objeto e por busca', async () => {
    const wrapper = build()
    await flushPromises()

    await wrapper.find('select').setValue('companies')
    expect(wrapper.text()).toContain('Clientes ativos')
    expect(wrapper.text()).not.toContain('Leads sem dono')

    await wrapper.find('select').setValue('')
    await wrapper.find('input[type="search"]').setValue('leads')
    expect(wrapper.text()).toContain('Leads sem dono')
    expect(wrapper.text()).not.toContain('Clientes ativos')
    wrapper.unmount()
  })

  it('abrir navega para a tela do objeto com o parâmetro da visualização', async () => {
    const wrapper = build()
    await flushPromises()

    await wrapper.findAll('.row-actions .btn')[0].trigger('click')
    expect(push).toHaveBeenCalledWith('/negocios?view=1')
    wrapper.unmount()
  })

  it('clonar cria uma cópia via POST /views', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string, options?: any) => {
      if (options?.method === 'POST') {
        return Promise.resolve({
          ok: true,
          status: 201,
          text: () => Promise.resolve(JSON.stringify({ ...views[0], id: 9, name: 'Cópia' }))
        })
      }
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(views)) })
    })
    vi.stubGlobal('prompt', vi.fn().mockReturnValue('Cópia'))

    const wrapper = build(fetchMock)
    await flushPromises()

    const cloneBtn = wrapper.findAll('.row-actions .btn').find((b) => b.text() === 'Clonar')!
    await cloneBtn.trigger('click')
    await flushPromises()

    const postCall = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(postCall).toBeTruthy()
    expect(JSON.parse(postCall![1].body).entity).toBe('deals')
    expect(wrapper.text()).toContain('Cópia')
    wrapper.unmount()
  })
})
