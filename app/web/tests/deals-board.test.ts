import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
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
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
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

describe('DealsBoardView - visão de lista', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.removeItem('crmia_deals_view')
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
  })

  function stubFetchWithList() {
    return vi.fn().mockImplementation((url: string) => {
      const u = String(url)
      let body: unknown = []
      if (u.includes('/pipelines')) {
        body = pipelines
      } else if (u.includes('/deals/board')) {
        body = { data: deals }
      } else if (u.includes('/deals?')) {
        body = { data: deals, pagination: { page: 1, per_page: 25, total: 2 } }
      } else if (u.includes('/views?entity=deals')) {
        body = []
      } else if (u.includes('/contacts?') || u.includes('/companies?')) {
        body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
      }
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
    })
  }

  it('alterna do quadro para a lista e mostra a tabela', async () => {
    vi.stubGlobal('fetch', stubFetchWithList())
    const wrapper = mount(DealsBoardView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    // Começa no quadro
    expect(wrapper.find('.board').exists()).toBe(true)
    expect(wrapper.find('.list-view').exists()).toBe(false)

    const listBtn = wrapper.findAll('.view-toggle button')[1]
    await listBtn.trigger('click')
    await flushPromises()

    expect(wrapper.find('.list-view').exists()).toBe(true)
    expect(wrapper.find('.board').exists()).toBe(false)
    const headers = wrapper.findAll('.list-view th').map((h) => h.text())
    expect(headers.join(' ')).toContain('Nome do negócio')
    expect(headers.join(' ')).toContain('Última atividade')
    expect(wrapper.text()).toContain('Adquirência Loja X')
    expect(wrapper.text()).toContain('2 negócio(s)')

    // Preferência persistida no navegador
    expect(localStorage.getItem('crmia_deals_view')).toBe('lista')
    wrapper.unmount()
  })
})

describe('DealsBoardView - drawer Criar Negócio', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.removeItem('crmia_deals_view')
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
  })

  const dealFormFields = [
    { key: 'name', visible: true, required: true },
    { key: 'pipeline_id', visible: true, required: true },
    { key: 'stage_id', visible: true, required: true },
    { key: 'amount', visible: true, required: false },
    { key: 'temperature', visible: false, required: false },
    { key: 'owner_id', visible: false, required: false },
    { key: 'contact_id', visible: false, required: false },
    { key: 'company_id', visible: false, required: false },
    { key: 'close_date', visible: false, required: false }
  ]

  function stubFetchWithForm() {
    return vi.fn().mockImplementation((url: string) => {
      const u = String(url)
      let body: unknown = []
      if (u.includes('/settings/deal-form')) {
        body = { fields: dealFormFields }
      } else if (u.includes('/pipelines')) {
        body = pipelines
      } else if (u.includes('/deals/board')) {
        body = { data: deals }
      } else if (u.includes('/views?entity=deals')) {
        body = []
      } else if (u.includes('/contacts?') || u.includes('/companies?')) {
        body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
      }
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
    })
  }

  it('abre o drawer com os campos configurados, na ordem, ocultando os desativados', async () => {
    vi.stubGlobal('fetch', stubFetchWithForm())
    const wrapper = mount(DealsBoardView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.find('.create-drawer').exists()).toBe(true)
    const labels = wrapper.findAll('.create-body .field label').map((l) => l.text())
    expect(labels[0]).toContain('Nome do negócio')
    expect(labels[1]).toContain('Pipeline')
    expect(labels[2]).toContain('Fase do negócio')
    expect(labels.join(' ')).toContain('Valor')
    expect(labels.join(' ')).not.toContain('Temperatura') // oculto na config
    expect(wrapper.text()).toContain('Criar e adicionar outro')
    expect(wrapper.text()).toContain('Cancelar')
    wrapper.unmount()
  })
})

describe('DealsBoardView - envelhecimento na etapa e filtro "Em risco"', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.removeItem('crmia_deals_view')
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
  })

  const base = {
    currency: 'BRL',
    pipeline_id: 1,
    stage_id: 2,
    stage_name: 'Negociação',
    contact_id: null,
    company_id: null,
    owner_id: 1,
    owner_name: 'Luiz Rocha',
    status: 'aberto',
    temperature: '',
    close_date: null,
    position: 0,
    closed_at: null,
    last_activity_at: new Date().toISOString(),
    created_at: '',
    updated_at: ''
  }

  const agingDeals = [
    // 19 dias com prazo 10: estagnado (vermelho + borda + ⚠)
    { ...base, id: 1, name: 'Estagnado SA', amount: 30000, dias_na_etapa: 19, prazo_dias: 10, dias_sem_atividade: 2 },
    // 8 dias de 10 (80% do prazo): âmbar; e sem atividade há 9 dias → em risco
    { ...base, id: 2, name: 'Quase Lá Ltda', amount: 20000, dias_na_etapa: 8, prazo_dias: 10, dias_sem_atividade: 9 },
    // 2 dias de 10: cinza, saudável
    { ...base, id: 3, name: 'Fresquinho ME', amount: 10000, dias_na_etapa: 2, prazo_dias: 10, dias_sem_atividade: 0 },
    // sem prazo configurado: usa o padrão de 14 dias → 3 d é normal
    { ...base, id: 4, name: 'Sem Prazo EPP', amount: 5000, dias_na_etapa: 3, prazo_dias: null, dias_sem_atividade: 1 }
  ]

  function stubFetchAging() {
    return vi.fn().mockImplementation((url: string) => {
      const u = String(url)
      let body: unknown = []
      if (u.includes('/pipelines')) {
        body = pipelines
      } else if (u.includes('/deals/board')) {
        body = { data: agingDeals }
      } else if (u.includes('/contacts?') || u.includes('/companies?')) {
        body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
      }
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
    })
  }

  it('card mostra o selo "X d" cinza, âmbar a 70% do prazo e vermelho com borda ao ultrapassar', async () => {
    vi.stubGlobal('fetch', stubFetchAging())
    const wrapper = mount(DealsBoardView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    const cards = wrapper.findAll('.deal-card')
    const porNome = (nome: string) => cards.find((c) => c.text().includes(nome))!

    const estagnado = porNome('Estagnado SA')
    expect(estagnado.classes()).toContain('atrasado')
    expect(estagnado.find('.aging').classes()).toContain('vencido')
    expect(estagnado.find('.aging').text()).toContain('⚠')
    expect(estagnado.find('.aging').text()).toContain('19 d')

    const quase = porNome('Quase Lá Ltda')
    expect(quase.classes()).not.toContain('atrasado')
    expect(quase.find('.aging').classes()).toContain('atencao')
    expect(quase.find('.aging').text()).toBe('8 d')

    const fresco = porNome('Fresquinho ME')
    expect(fresco.find('.aging').classes()).toContain('normal')
    expect(fresco.find('.aging').classes()).not.toContain('atencao')

    const semPrazo = porNome('Sem Prazo EPP')
    expect(semPrazo.find('.aging').classes()).toContain('normal')
    wrapper.unmount()
  })

  it('cabeçalho da coluna mostra qtd · total · ponderado e o contador de estagnados', async () => {
    vi.stubGlobal('fetch', stubFetchAging())
    const wrapper = mount(DealsBoardView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    const negociacao = wrapper.findAll('.column').find((c) => c.text().includes('Negociação'))!
    expect(negociacao.find('.stage-count').text()).toBe('4')
    expect(negociacao.find('.stage-stagnant').text()).toContain('1 estagnado(s)')
    const totais = negociacao.find('.stage-totals').text().replace(/\u00a0/g, ' ').replace(/\s+/g, ' ')
    expect(totais).toContain('4 · R$ 65 mil · R$ 52 mil pond.')

    // Coluna sem estagnados não mostra o contador.
    const lead = wrapper.findAll('.column').find((c) => c.text().includes('Novo Lead'))!
    expect(lead.find('.stage-stagnant').exists()).toBe(false)
    wrapper.unmount()
  })

  it('filtro rápido "Em risco" deixa só estagnados, sem atividade há mais de 7 dias ou vencidos', async () => {
    vi.stubGlobal('fetch', stubFetchAging())
    const wrapper = mount(DealsBoardView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    expect(wrapper.findAll('.deal-card')).toHaveLength(4)

    await wrapper.find('.risk-btn').trigger('click')
    expect(wrapper.find('.risk-btn').classes()).toContain('on')

    const nomes = wrapper.findAll('.deal-card').map((c) => c.find('strong').text())
    expect(nomes).toEqual(['Estagnado SA', 'Quase Lá Ltda'])
    expect(wrapper.findAll('.column').find((c) => c.text().includes('Negociação'))!.find('.stage-count').text()).toBe('2')

    await wrapper.find('.risk-btn').trigger('click')
    expect(wrapper.findAll('.deal-card')).toHaveLength(4)
    wrapper.unmount()
  })
})
