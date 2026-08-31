import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import EmailsView from '../src/views/EmailsView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const routerStubs = { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } }

const messages = [
  {
    id: 1,
    subject: 'Proposta comercial',
    to_email: 'ana@cliente.com',
    contact_id: 3,
    contact_name: 'Ana Silva',
    deal_id: null,
    user_id: 1,
    user_name: 'Bruno',
    source: 'manual',
    opens: 4,
    clicks: 2,
    first_open_at: '2026-08-30T12:00:00Z',
    last_open_at: '2026-08-30T14:00:00Z',
    first_click_at: '2026-08-30T13:00:00Z',
    sent_at: '2026-08-30T10:00:00Z'
  },
  {
    id: 2,
    subject: 'Follow-up',
    to_email: 'joao@cliente.com',
    contact_id: null,
    deal_id: null,
    user_id: 1,
    user_name: 'Bruno',
    source: 'automacao',
    opens: 0,
    clicks: 0,
    first_open_at: null,
    last_open_at: null,
    first_click_at: null,
    sent_at: '2026-08-29T10:00:00Z'
  }
]

const stats = { sent: 2, opened: 1, clicked: 1, open_rate: 50, click_rate: 50 }

function stubFetch(list = messages) {
  return vi.fn().mockImplementation((url: string) => {
    let body: unknown = {}
    const path = String(url)
    if (path.includes('/emails/stats')) body = stats
    else if (path.match(/\/emails\/sent\/\d+/)) {
      body = {
        message: list[0],
        events: [
          { id: 9, message_id: 1, kind: 'clique', url: 'https://fixpay.com.br', created_at: '2026-08-30T13:00:00Z' }
        ]
      }
    } else if (path.includes('/emails/sent')) body = { data: list }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'T', email: 't@fixpay.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

describe('EmailsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
  })

  it('mostra as taxas de abertura e clique', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(EmailsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('50% de abertura')
    expect(wrapper.text()).toContain('50% de cliques')
  })

  it('marca cada envio pelo melhor resultado obtido', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(EmailsView, { global: { stubs: routerStubs } })
    await flushPromises()

    const html = wrapper.html()
    expect(html).toContain('2 clique(s)')
    expect(html).toContain('Não aberto')
  })

  it('identifica os envios feitos por automação', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(EmailsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('automação')
  })

  it('abre o detalhe com o histórico de eventos', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(EmailsView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.findAll('tbody tr')[0].trigger('click')
    await flushPromises()

    expect((wrapper.vm as any).detail).toBeTruthy()
    expect((wrapper.vm as any).detail.events).toHaveLength(1)
  })

  it('refaz a busca ao trocar o período', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(EmailsView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.find('select').setValue('7')
    await flushPromises()

    const urls = fetchMock.mock.calls.map((c: any[]) => String(c[0]))
    expect(urls.some((u: string) => u.includes('days=7'))).toBe(true)
  })

  it('mostra estado vazio sem envios no período', async () => {
    vi.stubGlobal('fetch', stubFetch([]))
    const wrapper = mount(EmailsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum e-mail enviado no período')
  })
})
