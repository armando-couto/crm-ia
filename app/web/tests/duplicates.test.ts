import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import DuplicatesView from '../src/views/DuplicatesView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} }),
  RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' }
}))

const groups = [
  {
    reason: 'mesmo e-mail',
    value: 'ana@exemplo.com.br',
    records: [
      { id: 1, label: 'Ana Silva', email: 'ana@exemplo.com.br', deals: 3, created_at: '2026-01-10T10:00:00Z' },
      { id: 2, label: 'Ana S.', email: 'ana@exemplo.com.br', deals: 0, created_at: '2026-05-10T10:00:00Z' }
    ]
  }
]

function stubFetch(payload: any = { entity: 'contacts', groups }) {
  return vi.fn().mockImplementation((_url: string, options?: any) => {
    if (options?.method === 'POST') {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{"id":1}') })
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(payload)) })
  })
}

// router-link fora do router real precisa de stub.
const routerStubs = { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } }

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'T', email: 't@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

describe('DuplicatesView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  it('mostra os grupos com o critério que os aproximou', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(DuplicatesView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('mesmo e-mail')
    expect(wrapper.text()).toContain('Ana Silva')
    expect(wrapper.text()).toContain('3 negócio(s)')
  })

  it('mostra estado limpo quando não há duplicados', async () => {
    vi.stubGlobal('fetch', stubFetch({ entity: 'contacts', groups: [] }))
    const wrapper = mount(DuplicatesView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum duplicado encontrado')
  })

  it('troca para empresas e refaz a busca', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(DuplicatesView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.findAll('.tabs button')[1].trigger('click')
    await flushPromises()

    const urls = fetchMock.mock.calls.map((c: any[]) => String(c[0]))
    expect(urls.some((u: string) => u.includes('entity=companies'))).toBe(true)
  })

  it('sugere como principal o registro com mais negócios', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(DuplicatesView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.find('.group-head .btn-primary').trigger('click')
    await flushPromises()

    expect((wrapper.vm as any).primaryId).toBe(1)
  })

  it('mescla enviando principal e duplicado', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(DuplicatesView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.find('.group-head .btn-primary').trigger('click')
    await (wrapper.vm as any).merge()
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(post).toBeTruthy()
    const body = JSON.parse(post![1].body)
    expect(body).toMatchObject({ entity: 'contacts', primary_id: 1, duplicate_id: 2 })
  })

  it('esconde o botão de mesclar de quem não tem a permissão', async () => {
    loginAs('seller', { 'contacts.view': true })
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(DuplicatesView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.find('.group-head .btn-primary').exists()).toBe(false)
  })
})
