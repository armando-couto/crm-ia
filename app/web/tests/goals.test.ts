import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import GoalsView from '../src/views/GoalsView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const goals = [
  {
    id: 1,
    kind: 'ganho',
    metric: 'valor',
    user_id: 1,
    user_name: 'Eliseu',
    pipeline_id: null,
    stage_id: null,
    activity_kind: '',
    amount: 200000,
    start_period: '2026-08',
    end_period: '2026-09',
    created_by: 1,
    created_at: '',
    updated_at: '',
    finished: false,
    current_value: 90000,
    attainment: 45
  },
  {
    id: 2,
    kind: 'atividade',
    metric: 'numero',
    user_id: null,
    pipeline_id: null,
    stage_id: null,
    activity_kind: 'ligacao',
    amount: 50,
    start_period: '2026-01',
    end_period: '2026-02',
    created_by: 1,
    created_at: '',
    updated_at: '',
    finished: true,
    current_value: 0,
    attainment: 0
  }
]

const detail = {
  goal: goals[0],
  points: [
    { month: '2026-08', actual: 90000, target: 200000, attainment: 45 },
    { month: '2026-09', actual: 0, target: 200000, attainment: 0 }
  ]
}

function stubFetch() {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const path = String(url)
    if (options?.method === 'POST' || options?.method === 'PUT') {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{"id":1}') })
    }
    let body: unknown = {}
    if (path.match(/\/tracked-goals\/\d+/)) body = detail
    else if (path.includes('/tracked-goals')) body = { data: goals }
    else if (path.includes('/users')) body = [{ id: 1, name: 'Eliseu' }]
    else if (path.includes('/pipelines')) {
      body = { data: [{ id: 1, name: 'Vendas', stages: [{ id: 10, name: 'Proposta', is_won: false, is_lost: false }] }] }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Eliseu', email: 'e@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

describe('GoalsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.stubGlobal('fetch', stubFetch())
  })

  function mountView() {
    return mount(GoalsView, { global: { stubs: { teleport: true, RouterLink: true } } })
  }

  it('mostra a meta ativa com o progresso do mês', async () => {
    const wrapper = mountView()
    await flushPromises()

    const text = wrapper.text().replace(/ /g, ' ')
    expect(text).toContain('Negócios ganhos · Eliseu')
    expect(text).toContain('R$ 90.000,00')
    expect(text).toContain('de R$ 200.000,00 neste mês')
    expect(text).toContain('45%')
  })

  it('separa ativas de passadas', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Atividades realizadas')
    await wrapper.findAll('.tabs .btn')[1].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Atividades realizadas · Toda a equipe')
    expect(wrapper.text()).not.toContain('Negócios ganhos')
  })

  it('abre o modal 1/2 com os quatro tipos', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('.head-actions > .btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Adicionar meta 1/2')
    expect(wrapper.text()).toContain('Negócios ganhos')
    expect(wrapper.text()).toContain('Negócios em progresso')
    expect(wrapper.text()).toContain('número ou valor de negócios ganhos')
  })

  it('o passo 2 configura responsável, métrica e duração', async () => {
    const wrapper = mountView()
    await flushPromises()

    const vm = wrapper.vm as any
    vm.pickKind('ganho')
    await flushPromises()

    expect(wrapper.text()).toContain('Adicionar meta 2/2')
    expect(wrapper.text()).toContain('Responsável')
    expect(wrapper.text()).toContain('Métrica de rastreamento')
    expect(vm.editing.metric).toBe('valor')

    vm.editing.amount = 200000
    await vm.save()
    await flushPromises()

    const post = (globalThis.fetch as any).mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(JSON.parse(post![1].body)).toMatchObject({ kind: 'ganho', amount: 200000 })
  })

  it('meta de atividade fixa a métrica em número e esconde o funil', async () => {
    const wrapper = mountView()
    await flushPromises()

    const vm = wrapper.vm as any
    vm.pickKind('atividade')
    await flushPromises()

    expect(vm.editing.metric).toBe('numero')
    expect(wrapper.text()).toContain('Tipo de atividade')
    expect(wrapper.text()).not.toContain('Funil de vendas')
  })

  it('abre o detalhe com o gráfico mês a mês', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('.goal').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.month')).toHaveLength(2)
    expect(wrapper.text()).toContain('ago. de 26')
    // 45% da meta no mês corrente.
    const fill = wrapper.findAll('.month-fill')[0]
    expect(fill.attributes('style')).toContain('height: 45%')
  })

  it('esconde a criação de quem não pode definir metas', async () => {
    loginAs('seller', { 'forecast.view': true })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('Criar meta')
  })
})
