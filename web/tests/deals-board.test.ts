import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import DealsBoardView from '../src/views/DealsBoardView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {} })
}))

const pipelines = [
  {
    id: 1,
    name: 'Pipeline de Vendas',
    position: 0,
    stages: [
      { id: 1, pipeline_id: 1, name: 'Novo Lead', position: 0, probability: 10, is_won: false, is_lost: false },
      { id: 2, pipeline_id: 1, name: 'Negociação', position: 1, probability: 80, is_won: false, is_lost: false },
      { id: 3, pipeline_id: 1, name: 'Ganho', position: 2, probability: 100, is_won: true, is_lost: false }
    ]
  }
]

const deals = [
  {
    id: 1,
    name: 'Adquirência Loja X',
    amount: 100000,
    currency: 'BRL',
    pipeline_id: 1,
    stage_id: 2,
    stage_name: 'Negociação',
    contact_id: null,
    company_id: 1,
    company_name: 'Supra Bikes',
    owner_id: 1,
    owner_name: 'Luiz Rocha',
    status: 'aberto',
    temperature: 'quente',
    close_date: null,
    position: 0,
    closed_at: null,
    last_activity_at: new Date().toISOString(),
    created_at: '',
    updated_at: ''
  },
  {
    id: 2,
    name: 'Pix Mercado Y',
    amount: 50000,
    currency: 'BRL',
    pipeline_id: 1,
    stage_id: 3,
    stage_name: 'Ganho',
    contact_id: null,
    company_id: null,
    owner_id: 1,
    status: 'ganho',
    temperature: '',
    close_date: null,
    position: 0,
    closed_at: new Date().toISOString(),
    created_at: '',
    updated_at: ''
  }
]

const savedViews = [
  { id: 5, entity: 'deals', name: 'Funil Limpo', filters: { temperature: 'quente' }, position: 0, created_by: 1, created_at: '' }
]

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/pipelines')) {
      body = pipelines
    } else if (u.includes('/deals/board')) {
      body = { data: deals }
    } else if (u.includes('/views?entity=deals')) {
      body = savedViews
    } else if (u.includes('/contacts?') || u.includes('/companies?')) {
      body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('DealsBoardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build() {
    return mount(DealsBoardView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
  }

  it('mostra contagem no cabeçalho e valor total/ponderado no rodapé da coluna', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const negociacao = wrapper.findAll('.column').find((c) => c.text().includes('Negociação'))!
    expect(negociacao.find('.stage-count').text()).toBe('1')
    const foot = negociacao.find('.column-foot').text().replace(/ /g, ' ')
    expect(foot).toContain('R$ 100.000,00 · Valor total')
    expect(foot).toContain('R$ 80.000,00 (80%) · Valor ponderado')
    wrapper.unmount()
  })

  it('card mostra empresa, temperatura, dono e última atividade', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const card = wrapper.find('.deal-card')
    expect(card.text()).toContain('Adquirência Loja X')
    expect(card.text()).toContain('Supra Bikes')
    expect(card.text()).toContain('Quente')
    expect(card.text()).toContain('Luiz Rocha')
    expect(card.text()).toContain('Atividade hoje')
    wrapper.unmount()
  })

  it('coluna de ganho mostra os fechados recentes com contagem', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const ganho = wrapper.findAll('.column').find((c) => c.text().includes('Ganho'))!
    expect(ganho.find('.stage-count').text()).toBe('1')
    expect(ganho.text()).toContain('Pix Mercado Y')
    wrapper.unmount()
  })

  it('renderiza visualizações salvas de negócios como abas', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Funil Limpo')
    expect(wrapper.text()).toContain('2 negócio(s) no quadro')
    wrapper.unmount()
  })
})
