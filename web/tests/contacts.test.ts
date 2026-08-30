import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ContactsView from '../src/views/ContactsView.vue'
import { formatCompact } from '../src/format'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {} })
}))

const contacts = [
  {
    id: 1,
    first_name: 'Carlos',
    last_name: 'Lima',
    email: 'carlos@bompreco.com.br',
    phone: '+55-85-3491-1272',
    job_title: 'Diretor',
    lifecycle_stage: 'oportunidade',
    source: 'indicacao',
    company_id: 1,
    company_name: 'Mercado Bom Preço',
    owner_id: null,
    owner_name: '',
    last_activity_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString()
  }
]

const stats = { total: 3180, sem_dono: 68, sem_email: 12, leads_sem_avanco: 4580, sem_atividade_30d: 1740 }

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/contacts/stats')) {
      body = stats
    } else if (u.includes('/contacts?')) {
      body = { data: contacts, pagination: { page: 1, per_page: 25, total: 130 } }
    } else if (u.includes('/lists')) {
      body = [{ id: 9, name: 'Oportunidades', kind: 'dinamica', members_count: 0, created_by: 1, created_at: '', updated_at: '' }]
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('formatCompact', () => {
  it('formata números grandes no estilo HubSpot', () => {
    expect(formatCompact(3180).replace(/ /g, ' ')).toBe('3,18 mil')
    expect(formatCompact(68)).toBe('68')
    expect(formatCompact(null)).toBe('0')
  })
})

describe('ContactsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build() {
    return mount(ContactsView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
  }

  it('renderiza abas, cartões de métricas e a tabela', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    // Abas fixas + lista dinâmica como visualização extra
    expect(wrapper.text()).toContain('Todos os contatos')
    expect(wrapper.text()).toContain('Meus contatos')
    expect(wrapper.text()).toContain('Não atribuídos')
    expect(wrapper.text()).toContain('Oportunidades')

    // Cartões com número compacto
    expect(wrapper.text()).toContain('Contatos sem proprietário')
    expect(wrapper.text().replace(/ /g, ' ')).toContain('3,18 mil')

    // Linha da tabela com e-mail/telefone clicáveis e dono ausente
    expect(wrapper.find('a[href="mailto:carlos@bompreco.com.br"]').exists()).toBe(true)
    expect(wrapper.find('a[href="tel:+55-85-3491-1272"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Nenhum proprietário')
    wrapper.unmount()
  })

  it('mostra paginação numerada quando há várias páginas', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    // 130 contatos / 25 por página = 6 páginas
    const buttons = wrapper.findAll('.page-btn').map((b) => b.text())
    expect(buttons).toContain('1')
    expect(buttons).toContain('6')
    wrapper.unmount()
  })

  it('seleciona contatos e mostra a barra de ações em massa', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.find('.bulk-bar').exists()).toBe(false)
    await wrapper.find('tbody input[type="checkbox"]').setValue(true)
    expect(wrapper.find('.bulk-bar').exists()).toBe(true)
    expect(wrapper.text()).toContain('1 selecionado(s)')
    wrapper.unmount()
  })

  it('aplica filtro ao clicar em um cartão de métrica', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = build()
    await flushPromises()

    await wrapper.find('.stat-card').trigger('click')
    await flushPromises()

    const calls = fetchMock.mock.calls.map((c: any[]) => String(c[0]))
    expect(calls.some((u: string) => u.includes('sem_dono=true'))).toBe(true)
    expect(wrapper.find('.stat-card.active').exists()).toBe(true)
    wrapper.unmount()
  })
})
