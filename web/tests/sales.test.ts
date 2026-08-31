import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import SalesWorkspaceView from '../src/views/SalesWorkspaceView.vue'
import TargetAccountsView from '../src/views/TargetAccountsView.vue'
import ForecastView from '../src/views/ForecastView.vue'
import SalesAnalyticsView from '../src/views/SalesAnalyticsView.vue'
import ActivitiesView from '../src/views/ActivitiesView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const routerStubs = { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } }

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana Silva', email: 'a@fixpay.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

// ===== Espaço de trabalho =====

const workspace = {
  overdue_tasks: [{ id: 1, title: 'Ligar para o cliente', subtitle: 'ligacao', due: '2026-08-20T10:00:00Z' }],
  today_tasks: [{ id: 2, title: 'Enviar proposta', subtitle: 'email', due: '2026-08-31T10:00:00Z' }],
  today_meetings: [],
  stale_deals: [
    {
      id: 5,
      title: 'Maquininhas',
      subtitle: 'Proposta',
      due: null,
      amount: 8000,
      deal_id: 5,
      reason: 'sem interação há 21 dias'
    }
  ],
  closing_soon: [],
  untouched_leads: [],
  open_deals: 4,
  open_amount: 32000,
  won_month: 12000,
  goal_month: 20000,
  tasks_pending: 7,
  meetings_week: 3,
  target_accounts: 2
}

describe('SalesWorkspaceView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('seller', { 'tasks.edit': true })
  })

  it('monta as filas do dia e conta os pendentes', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: () => Promise.resolve(JSON.stringify(workspace))
      })
    )
    const wrapper = mount(SalesWorkspaceView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Tarefas atrasadas')
    expect(wrapper.text()).toContain('Para hoje')
    expect(wrapper.text()).toContain('Negócios parados')
    // Filas vazias não aparecem.
    expect(wrapper.text()).not.toContain('Reuniões de hoje')
    expect(wrapper.text()).toContain('3 item(ns) na sua fila')
  })

  it('mostra por que o negócio está na fila', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(workspace)) })
    )
    const wrapper = mount(SalesWorkspaceView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('sem interação há 21 dias')
  })

  it('mostra o progresso da meta pessoal', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(workspace)) })
    )
    const wrapper = mount(SalesWorkspaceView, { global: { stubs: routerStubs } })
    await flushPromises()

    // 12000 de 20000 = 60%
    expect(wrapper.find('.goal-fill').attributes('style')).toContain('width: 60%')
  })

  it('comemora o dia limpo quando não há pendência', async () => {
    const vazio = {
      ...workspace,
      overdue_tasks: [],
      today_tasks: [],
      stale_deals: []
    }
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(vazio)) })
    )
    const wrapper = mount(SalesWorkspaceView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Dia limpo')
  })

  it('conclui a tarefa direto da fila', async () => {
    const fetchMock = vi.fn().mockImplementation((_url: string, options?: any) => {
      if (options?.method === 'PATCH') {
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{}') })
      }
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(workspace)) })
    })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SalesWorkspaceView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.find('.check').trigger('click')
    await flushPromises()

    const patch = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'PATCH')
    expect(String(patch![0])).toContain('/tasks/1/toggle')
  })
})

// ===== Contas-alvo =====

const targets = {
  data: [
    {
      id: 3,
      name: 'Mercado Central',
      target_tier: 1,
      target_notes: 'grupo econômico grande',
      owner_name: 'Ana',
      contacts_count_total: 4,
      decision_makers: 0,
      open_deals: 2,
      open_amount: 45000,
      won_amount: 0,
      last_activity_at: '2026-01-10T10:00:00Z',
      open_tasks: 1
    }
  ],
  summary: {
    total: 1,
    tier1: 1,
    with_deals: 1,
    open_amount: 45000,
    no_activity_30d: 1,
    without_decision_maker: 1
  }
}

describe('TargetAccountsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, options?: any) => {
        if (options?.method === 'PUT') {
          return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{}') })
        }
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(targets)) })
      })
    )
  })

  it('lista a conta com o resumo do que já andou', async () => {
    const wrapper = mount(TargetAccountsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Mercado Central')
    expect(wrapper.text()).toContain('grupo econômico grande')
    expect(wrapper.text()).toContain('sem decisor')
  })

  it('destaca a conta parada há mais de 30 dias', async () => {
    const wrapper = mount(TargetAccountsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.find('tr.stale').exists()).toBe(true)
  })

  it('muda o tier da conta', async () => {
    const wrapper = mount(TargetAccountsView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.find('select.tier').setValue('3')
    await flushPromises()

    const put = (globalThis.fetch as any).mock.calls.find((c: any[]) => c[1]?.method === 'PUT')
    expect(JSON.parse(put![1].body)).toMatchObject({ is_target: true, target_tier: 3 })
  })
})

// ===== Previsão =====

const forecast = {
  period: '2026-08',
  rows: [
    {
      owner_id: 1,
      owner_name: 'Ana',
      won: 4000,
      committed: 10000,
      weighted: 5000,
      open_deals: 2,
      goal: 20000,
      attainment: 20
    }
  ],
  team_goal: 20000,
  won: 4000,
  committed: 10000,
  weighted: 5000,
  projected: 9000,
  gap: 11000
}

describe('ForecastView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        const path = String(url)
        let body: unknown = forecast
        if (path.includes('/pipelines')) body = { data: [] }
        else if (path.includes('/users')) body = [{ id: 1, name: 'Ana' }]
        else if (path.includes('/goals')) body = { data: [] }
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
      })
    )
  })

  it('separa ganho, comprometido e projeção', async () => {
    const wrapper = mount(ForecastView, { global: { stubs: routerStubs } })
    await flushPromises()

    const text = wrapper.text().replace(/\u00a0/g, ' ')
    expect(text).toContain('R$ 4.000,00')
    expect(text).toContain('R$ 10.000,00')
    expect(text).toContain('R$ 9.000,00')
    expect(text).toContain('faltam R$ 11.000,00')
  })

  it('mostra o atingimento por vendedor', async () => {
    const wrapper = mount(ForecastView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('20%')
    expect(wrapper.find('.mini-fill').attributes('style')).toContain('width: 20%')
  })

  it('esconde o botão de metas de quem não pode definir', async () => {
    loginAs('seller', { 'forecast.view': true })
    const wrapper = mount(ForecastView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('Metas')
  })
})

// ===== Análise de vendas =====

const analytics = {
  days: 90,
  created: 10,
  won: 2,
  lost: 1,
  win_rate: 66.7,
  avg_ticket: 1500,
  avg_cycle_days: 12.4,
  won_amount: 3000,
  funnel: [
    { stage_id: 1, stage_name: 'Qualificação', count: 10, amount: 50000, rate: 100 },
    { stage_id: 2, stage_name: 'Proposta', count: 4, amount: 20000, rate: 40 }
  ],
  activity_by_kind: [
    { label: 'ligacao', value: 30 },
    { label: 'email', value: 12 }
  ]
}

describe('SalesAnalyticsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        const body = String(url).includes('/pipelines') ? { data: [] } : analytics
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
      })
    )
  })

  it('mostra taxa de ganho, ticket médio e ciclo', async () => {
    const wrapper = mount(SalesAnalyticsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('67%')
    expect(wrapper.text()).toContain('12 dias')
    expect(wrapper.text().replace(/\u00a0/g, ' ')).toContain('R$ 1.500,00')
  })

  it('desenha o funil proporcional à primeira etapa', async () => {
    const wrapper = mount(SalesAnalyticsView, { global: { stubs: routerStubs } })
    await flushPromises()

    const fills = wrapper.findAll('.funnel-fill')
    expect(fills[0].attributes('style')).toContain('width: 100%')
    expect(fills[1].attributes('style')).toContain('width: 40%')
  })

  it('traduz os tipos de interação', async () => {
    const wrapper = mount(SalesAnalyticsView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Ligações')
    expect(wrapper.text()).toContain('E-mails')
  })
})

// ===== Atividades =====

describe('ActivitiesView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        const body = String(url).includes('/users')
          ? [{ id: 1, name: 'Ana' }]
          : {
              data: [
                {
                  id: 1,
                  kind: 'ligacao',
                  content: 'Falei com o cliente',
                  user_id: 1,
                  user_name: 'Ana',
                  contact_id: 3,
                  company_id: null,
                  deal_id: null,
                  contact_name: 'João Cliente',
                  created_at: '2026-08-30T14:00:00Z'
                }
              ]
            }
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
      })
    )
  })

  it('mostra a atividade com o registro a que pertence', async () => {
    const wrapper = mount(ActivitiesView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Ligação')
    expect(wrapper.text()).toContain('Falei com o cliente')
    expect(wrapper.text()).toContain('João Cliente')
    expect(wrapper.text()).toContain('por Ana')
  })

  it('pede o feed geral e não a timeline de um registro', async () => {
    mount(ActivitiesView, { global: { stubs: routerStubs } })
    await flushPromises()

    const urls = (globalThis.fetch as any).mock.calls.map((c: any[]) => String(c[0]))
    expect(urls.some((u: string) => u.includes('feed=true'))).toBe(true)
  })

  it('refaz a busca ao filtrar por tipo', async () => {
    const wrapper = mount(ActivitiesView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.findAll('select')[0].setValue('email')
    await flushPromises()

    const urls = (globalThis.fetch as any).mock.calls.map((c: any[]) => String(c[0]))
    expect(urls.some((u: string) => u.includes('kind=email'))).toBe(true)
  })
})
