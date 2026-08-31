import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import ReportsView from '../src/views/ReportsView.vue'
import ReportChart from '../src/components/ReportChart.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const reports = [
  {
    id: 1,
    name: 'Receita por vendedor',
    description: 'Valor ganho no trimestre',
    entity: 'negocios',
    metric: 'soma_valor',
    dimension: 'dono',
    filters: { days: 90, owner_id: 0, status: '' },
    chart: 'barras',
    shared: true,
    position: 0,
    created_by: 1,
    created_at: '',
    updated_at: ''
  }
]

const result = {
  rows: [
    { label: 'Ana', value: 1500 },
    { label: 'Bruno', value: 300 }
  ],
  total: 1800,
  metric_label: 'Valor total',
  is_money: true,
  dimension_label: 'Dono'
}

const catalog = [
  {
    key: 'negocios',
    label: 'Negócios',
    metrics: { contagem: 'Quantidade', soma_valor: 'Valor total' },
    dimensions: { dono: 'Dono', etapa: 'Etapa' }
  },
  {
    key: 'contatos',
    label: 'Contatos',
    metrics: { contagem: 'Quantidade' },
    dimensions: { origem: 'Origem' }
  }
]

function stubFetch(list = reports) {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const path = String(url)
    if (path.includes('/reports/preview')) {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify({ result })) })
    }
    if (options?.method === 'POST' || options?.method === 'PUT' || options?.method === 'DELETE') {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{"id":1}') })
    }
    let body: unknown = {}
    if (path.match(/\/reports\/\d+\/run/)) body = { result }
    else if (path.includes('/reports')) body = { data: list, catalog }
    else if (path.includes('/users')) body = [{ id: 1, name: 'Ana' }]
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'T', email: 't@fixpay.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

describe('ReportsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  it('mostra o relatório com o total formatado em dinheiro', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(ReportsView)
    await flushPromises()

    expect(wrapper.text()).toContain('Receita por vendedor')
    expect(wrapper.text().replace(/ /g, ' ')).toContain('R$ 1.800,00')
    expect(wrapper.text()).toContain('por dono')
  })

  it('mostra estado vazio com exemplos', async () => {
    vi.stubGlobal('fetch', stubFetch([]))
    const wrapper = mount(ReportsView)
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum relatório ainda')
  })

  it('troca de entidade e ajusta métrica e agrupamento inválidos', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(ReportsView)
    await flushPromises()

    const vm = wrapper.vm as any
    vm.novo()
    await flushPromises()
    expect(vm.editing.entity).toBe('negocios')

    vm.editing.metric = 'soma_valor'
    vm.editing.dimension = 'etapa'
    vm.editing.entity = 'contatos'
    await flushPromises()

    // Contatos não têm soma_valor nem etapa: ambos caem no primeiro válido.
    expect(vm.editing.metric).toBe('contagem')
    expect(vm.editing.dimension).toBe('origem')
  })

  it('pede a prévia ao montar um relatório novo', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(ReportsView)
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    await flushPromises()

    const previewCall = fetchMock.mock.calls.find((c: any[]) => String(c[0]).includes('/reports/preview'))
    expect(previewCall).toBeTruthy()
    expect((wrapper.vm as any).preview.total).toBe(1800)
  })

  it('esconde criação e edição de quem só pode ver', async () => {
    loginAs('seller', { 'reports.view': true })
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(ReportsView)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Novo relatório')
    expect(wrapper.find('.report-actions').exists()).toBe(false)
  })
})

describe('ReportChart', () => {
  it('desenha uma barra por linha, proporcional ao maior valor', () => {
    const wrapper = mount(ReportChart, { props: { result, chart: 'barras' } })
    const fills = wrapper.findAll('.bar-fill')

    expect(fills).toHaveLength(2)
    expect(fills[0].attributes('style')).toContain('width: 100%')
    expect(fills[1].attributes('style')).toContain('width: 20%')
  })

  it('desenha uma fatia por linha na pizza, com o percentual na legenda', () => {
    const wrapper = mount(ReportChart, { props: { result, chart: 'pizza' } })

    expect(wrapper.findAll('.pie-svg path')).toHaveLength(2)
    expect(wrapper.text()).toContain('83%')
    expect(wrapper.text()).toContain('17%')
  })

  it('monta a tabela com os rótulos das colunas', () => {
    const wrapper = mount(ReportChart, { props: { result, chart: 'tabela' } })

    expect(wrapper.find('thead').text()).toContain('Dono')
    expect(wrapper.find('thead').text()).toContain('Valor total')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
  })

  it('traça a linha com um ponto por valor', () => {
    const wrapper = mount(ReportChart, { props: { result, chart: 'linha' } })
    const points = wrapper.find('polyline').attributes('points') ?? ''

    expect(points.split(' ')).toHaveLength(2)
  })

  it('avisa quando não há dados', () => {
    const vazio = { ...result, rows: [], total: 0 }
    const wrapper = mount(ReportChart, { props: { result: vazio, chart: 'barras' } })

    expect(wrapper.text()).toContain('Sem dados no período')
  })
})
