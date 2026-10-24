import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import PipelineInsightsView from '../src/views/PipelineInsightsView.vue'
import type { FunilResposta } from '../src/types-funil'

const { routerMock, routeMock } = vi.hoisted(() => ({
  routerMock: { replace: vi.fn(), push: vi.fn() },
  routeMock: { params: {}, query: {} as Record<string, string> }
}))

vi.mock('vue-router', () => ({
  useRouter: () => routerMock,
  useRoute: () => routeMock
}))

// ===== Fixture realista: pipeline de 4 etapas abertas + ganho + perdido, 30 dias em modo fluxo =====
const pipelines = [
  {
    id: 1,
    name: 'Vendas',
    position: 0,
    stages: [
      { id: 1, pipeline_id: 1, name: 'Novo Lead', position: 0, probability: 10, is_won: false, is_lost: false, prazo_dias: 7 },
      { id: 2, pipeline_id: 1, name: 'Qualificação', position: 1, probability: 25, is_won: false, is_lost: false, prazo_dias: 10 },
      { id: 3, pipeline_id: 1, name: 'Proposta', position: 2, probability: 50, is_won: false, is_lost: false, prazo_dias: 10 },
      { id: 4, pipeline_id: 1, name: 'Negociação', position: 3, probability: 80, is_won: false, is_lost: false, prazo_dias: 14 },
      { id: 5, pipeline_id: 1, name: 'Ganho', position: 4, probability: 100, is_won: true, is_lost: false, prazo_dias: null },
      { id: 6, pipeline_id: 1, name: 'Perdido', position: 5, probability: 0, is_won: false, is_lost: true, prazo_dias: null }
    ]
  },
  { id: 2, name: 'Pós-venda', position: 1, stages: [] }
]

const users = [
  { id: 3, name: 'Ana Souza', email: 'ana@x.com', role: 'seller', active: true, created_at: '', updated_at: '' },
  { id: 4, name: 'Bruno Lima', email: 'bruno@x.com', role: 'seller', active: true, created_at: '', updated_at: '' }
]

export const funilFixture: FunilResposta = {
  pipeline: {
    id: 1,
    name: 'Vendas',
    stages: pipelines[0].stages.map((s) => ({
      id: s.id,
      name: s.name,
      position: s.position,
      probability: s.probability,
      is_won: s.is_won,
      is_lost: s.is_lost,
      prazo_dias: s.prazo_dias
    }))
  },
  periodo: { de: '2026-09-24', ate: '2026-10-24', modo: 'fluxo', rotulo: 'Últimos 30 dias' },
  resumo: {
    abertos: 31,
    valor_aberto: 148000,
    ponderado: 61200,
    ganhos: 8,
    valor_ganho: 62000,
    perdidos: 5,
    valor_perdido: 21000,
    taxa_ganho: 0.615,
    ticket_medio: 7750,
    ciclo_mediano_dias: 23,
    meta: 150000,
    gap: 88000,
    cobertura: 0.7,
    em_risco: 3,
    valor_em_risco: 34000
  },
  funil: [
    { stage_id: 1, nome: 'Novo Lead', alcancaram: 40, valor: 310000, conversao_total: 1, conversao_etapa: 1, mediana_dias: 3, abertos_agora: 12, valor_aberto: 60000, ponderado: 6000, estagnados: 2 },
    { stage_id: 2, nome: 'Qualificação', alcancaram: 28, valor: 240000, conversao_total: 0.7, conversao_etapa: 0.7, mediana_dias: 6, abertos_agora: 9, valor_aberto: 44000, ponderado: 11000, estagnados: 0 },
    { stage_id: 3, nome: 'Proposta', alcancaram: 18, valor: 170000, conversao_total: 0.45, conversao_etapa: 0.643, mediana_dias: 12, abertos_agora: 6, valor_aberto: 30000, ponderado: 15000, estagnados: 1 },
    { stage_id: 4, nome: 'Negociação', alcancaram: 11, valor: 95000, conversao_total: 0.275, conversao_etapa: 0.611, mediana_dias: 9, abertos_agora: 4, valor_aberto: 14000, ponderado: 11200, estagnados: 0 },
    { stage_id: 5, nome: 'Ganho', alcancaram: 8, valor: 62000, conversao_total: 0.2, conversao_etapa: 0.727, mediana_dias: 0, abertos_agora: 0, valor_aberto: 0, ponderado: 0, estagnados: 0 },
    { stage_id: 6, nome: 'Perdido', alcancaram: 5, valor: 21000, conversao_total: 0.125, conversao_etapa: 0, mediana_dias: 0, abertos_agora: 0, valor_aberto: 0, ponderado: 0, estagnados: 0 }
  ],
  movimentacao: {
    inicio: { qtd: 28, valor: 120000 },
    criados: { qtd: 14, valor: 70000 },
    aumentos: { qtd: 3, valor: 12000 },
    reducoes: { qtd: 2, valor: -8000 },
    adiados: { qtd: 4, valor: 25000 },
    ganhos: { qtd: 8, valor: -62000 },
    perdidos: { qtd: 5, valor: -21000 },
    fim: { qtd: 31, valor: 148000 }
  },
  ganhos_por_semana: [
    { semana: '2026-09-28', qtd: 2, valor: 15000 },
    { semana: '2026-10-05', qtd: 1, valor: 9000 },
    { semana: '2026-10-12', qtd: 3, valor: 22000 },
    { semana: '2026-10-19', qtd: 2, valor: 16000 }
  ],
  por_dono: [
    { owner_id: 3, nome: 'Ana Souza', abertos: 16, valor_aberto: 80000, ponderado: 35000, ganhos: 5, valor_ganho: 40000, perdidos: 2, taxa_ganho: 0.714, em_risco: 2, meta: 80000 },
    { owner_id: 4, nome: 'Bruno Lima', abertos: 15, valor_aberto: 68000, ponderado: 26200, ganhos: 3, valor_ganho: 22000, perdidos: 3, taxa_ganho: 0.5, em_risco: 1, meta: 70000 }
  ],
  negocios: [
    {
      id: 10, name: 'ACME — expansão', amount: 12000, ponderado: 6000, stage_id: 3, stage_name: 'Proposta', status: 'aberto',
      owner_id: 3, owner_name: 'Ana Souza', company_id: 5, company_name: 'ACME', close_date: '2026-11-10',
      created_at: '2026-09-01T12:00:00Z', closed_at: null, dias_na_etapa: 19, prazo_dias: 10, dias_sem_atividade: 9, adiamentos: 2,
      variacao_7d: { valor: -2000, close_date_dias: 14, etapa: 'Negociação→Proposta' }, motivos: ['estagnado', 'sem_atividade', 'adiado']
    },
    {
      id: 11, name: 'Beta Ltda — licenças', amount: 20000, ponderado: 16000, stage_id: 4, stage_name: 'Negociação', status: 'aberto',
      owner_id: 4, owner_name: 'Bruno Lima', company_id: 6, company_name: 'Beta', close_date: '2026-10-01',
      created_at: '2026-10-02T12:00:00Z', closed_at: null, dias_na_etapa: 4, prazo_dias: 14, dias_sem_atividade: 1, adiamentos: 0,
      variacao_7d: null, motivos: ['vencido']
    },
    {
      id: 12, name: 'Gama — onboarding', amount: 5000, ponderado: 500, stage_id: 1, stage_name: 'Novo Lead', status: 'aberto',
      owner_id: 3, owner_name: 'Ana Souza', company_id: 7, company_name: 'Gama', close_date: null,
      created_at: '2026-10-20T12:00:00Z', closed_at: null, dias_na_etapa: 2, prazo_dias: 7, dias_sem_atividade: 0, adiamentos: 0,
      motivos: []
    },
    {
      id: 13, name: 'Delta — upgrade', amount: 15000, ponderado: 15000, stage_id: 5, stage_name: 'Ganho', status: 'ganho',
      owner_id: 3, owner_name: 'Ana Souza', company_id: 8, company_name: 'Delta', close_date: '2026-10-10',
      created_at: '2026-09-10T12:00:00Z', closed_at: '2026-10-10T12:00:00Z', dias_na_etapa: 0, prazo_dias: 0, dias_sem_atividade: 14, adiamentos: 0,
      motivos: []
    },
    {
      id: 14, name: 'Épsilon — renovação', amount: 7000, ponderado: 0, stage_id: 6, stage_name: 'Perdido', status: 'perdido',
      owner_id: 4, owner_name: 'Bruno Lima', company_id: 9, company_name: 'Épsilon', close_date: '2026-10-05',
      created_at: '2026-08-10T12:00:00Z', closed_at: '2026-10-05T12:00:00Z', dias_na_etapa: 0, prazo_dias: 0, dias_sem_atividade: 19, adiamentos: 0,
      motivos: []
    },
    {
      id: 15, name: 'Zeta — piloto', amount: 9000, ponderado: 4500, stage_id: 3, stage_name: 'Proposta', status: 'aberto',
      owner_id: 4, owner_name: 'Bruno Lima', company_id: 10, company_name: 'Zeta', close_date: '2026-11-20',
      created_at: '2026-09-15T12:00:00Z', closed_at: null, dias_na_etapa: 6, prazo_dias: 10, dias_sem_atividade: 2, adiamentos: 0,
      variacao_7d: { valor: 1500 }, motivos: []
    }
  ]
}

function stubFetch(overrides: { funil?: unknown; falhaFunil?: boolean } = {}) {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    let ok = true
    let status = 200
    if (u.includes('/pipelines')) {
      body = pipelines
    } else if (u.includes('/users')) {
      body = users
    } else if (u.includes('/funil')) {
      if (overrides.falhaFunil) {
        ok = false
        status = 500
        body = { error: 'falha ao calcular o funil' }
      } else {
        body = overrides.funil ?? funilFixture
      }
    }
    return Promise.resolve({ ok, status, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function build() {
  return mount(PipelineInsightsView, {
    global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
  })
}

describe('PipelineInsightsView (/funil)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
    routeMock.query = {}
    routerMock.replace.mockClear()
    document.body.classList.remove('modo-reuniao')
  })

  afterEach(() => {
    document.body.classList.remove('modo-reuniao')
  })

  it('renderiza os seis blocos a partir do payload mockado', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = build()
    await flushPromises()

    const blocos = wrapper.findAll('section.bloco').map((s) => s.attributes('data-bloco'))
    expect(blocos).toEqual(['meta', 'movimentacao', 'funil', 'risco', 'donos', 'lista'])

    const texto = wrapper.text().replace(/ /g, ' ')
    expect(texto).toContain('Meta × realizado')
    expect(texto).toContain('R$ 62.000,00') // ganho
    expect(texto).toContain('R$ 61.200,00') // ponderado
    expect(texto).toContain('R$ 88.000,00') // gap
    expect(texto).toContain('0,7×') // cobertura
    expect(texto).toContain('62%') // taxa de ganho
    expect(texto).toContain('23 dias') // ciclo mediano
    expect(texto).toContain('Movimentação do pipeline')
    expect(texto).toContain('Funil por etapa')
    expect(texto).toContain('Negócios em risco')
    expect(texto).toContain('Por vendedor')
    expect(texto).toContain('Ana Souza')
    expect(texto).toContain('Últimos 30 dias')

    // Pediu o funil com os filtros padrão.
    const urls = fetchMock.mock.calls.map((c: any[]) => String(c[0]))
    expect(urls.some((u) => u.includes('/funil?') && u.includes('periodo=30') && u.includes('modo=fluxo') && u.includes('pipeline_id=1'))).toBe(true)

    // Gráficos acessíveis: um role="img" com <title> por gráfico.
    const imgs = wrapper.findAll('svg[role="img"]')
    expect(imgs.length).toBeGreaterThanOrEqual(4)
    imgs.forEach((svg) => expect(svg.find('title').exists()).toBe(true))

    // Lista completa (6 negócios) sem filtro.
    expect(wrapper.findAll('table.lista tbody tr')).toHaveLength(6)
    expect(wrapper.find('.chip-filtro').exists()).toBe(false)
    wrapper.unmount()
  })

  it('funil exclui a etapa de perdido e destaca a mais lenta e os estagnados', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const bands = wrapper.findAll('.funnel-band')
    expect(bands).toHaveLength(5) // 4 abertas + ganho; perdido fica fora
    expect(wrapper.find('.mediana.lenta').text()).toContain('12 d')
    expect(wrapper.findAll('.estagnados:not(.zero)').length).toBe(2)
    wrapper.unmount()
  })

  it('clicar numa etapa do funil filtra a lista e mostra o chip; o ✕ limpa', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    await wrapper.find('.etapa-g[data-stage-id="3"]').trigger('click')
    await flushPromises()

    const chip = wrapper.find('.chip-filtro')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('Filtrando por:')
    expect(chip.text()).toContain('Proposta')

    const linhas = wrapper.findAll('table.lista tbody tr')
    expect(linhas).toHaveLength(2)
    expect(linhas.map((l) => l.text()).join(' ')).toContain('ACME — expansão')
    expect(linhas.map((l) => l.text()).join(' ')).toContain('Zeta — piloto')

    await chip.find('.chip-x').trigger('click')
    expect(wrapper.find('.chip-filtro').exists()).toBe(false)
    expect(wrapper.findAll('table.lista tbody tr')).toHaveLength(6)
    wrapper.unmount()
  })

  it('clicar em "perdidos" no waterfall mostra só os perdidos; clicar de novo desfaz', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    await wrapper.find('.barra-g[data-key="perdidos"]').trigger('click')
    let linhas = wrapper.findAll('table.lista tbody tr')
    expect(linhas).toHaveLength(1)
    expect(linhas[0].text()).toContain('Épsilon — renovação')
    expect(wrapper.find('.chip-filtro').text()).toContain('Perdidos')

    await wrapper.find('.barra-g[data-key="perdidos"]').trigger('click')
    expect(wrapper.find('.chip-filtro').exists()).toBe(false)

    await wrapper.find('.barra-g[data-key="criados"]').trigger('click')
    linhas = wrapper.findAll('table.lista tbody tr')
    expect(linhas.map((l) => l.text()).join(' ')).toContain('Beta Ltda')
    expect(linhas.map((l) => l.text()).join(' ')).toContain('Gama')
    expect(linhas).toHaveLength(2)
    wrapper.unmount()
  })

  it('negócios em risco listam motivos, variação de 7 dias e filtram por motivo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const itens = wrapper.findAll('.risco-item')
    expect(itens).toHaveLength(2)
    // Ordenado por valor: Beta (20k) antes de ACME (12k).
    expect(itens[0].text()).toContain('Beta Ltda')
    expect(itens[1].text()).toContain('ACME')
    expect(itens[1].text()).toContain('Estagnado')
    expect(itens[1].text()).toContain('Sem atividade')
    expect(itens[1].text()).toContain('Adiado')
    expect(itens[1].find('.var.baixa').text()).toContain('↓')
    expect(itens[1].find('.var.adiada').text()).toContain('+14 d')
    expect(itens[1].find('.dias.vencido').exists()).toBe(true)

    await itens[1].find('.chip-motivo.estagnado').trigger('click')
    expect(wrapper.find('.chip-filtro').text()).toContain('Estagnado')
    expect(wrapper.findAll('table.lista tbody tr')).toHaveLength(1)
    wrapper.unmount()
  })

  it('clicar num vendedor filtra a lista pelos negócios dele', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    await wrapper.find('table.donos tbody tr[data-owner-id="4"]').trigger('click')
    expect(wrapper.find('.chip-filtro').text()).toContain('Bruno Lima')
    const linhas = wrapper.findAll('table.lista tbody tr')
    expect(linhas).toHaveLength(3)
    linhas.forEach((l) => expect(l.text()).toContain('Bruno Lima'))
    wrapper.unmount()
  })

  it('trocar período e modo reflete na URL e refaz a consulta', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = build()
    await flushPromises()

    const botoes = wrapper.findAll('.filtros .segmentado button')
    await botoes.find((b) => b.text() === '90 dias')!.trigger('click')
    await flushPromises()
    await botoes.find((b) => b.text() === 'Coorte')!.trigger('click')
    await flushPromises()

    const ultimaUrl = fetchMock.mock.calls.map((c: any[]) => String(c[0])).filter((u) => u.includes('/funil?')).at(-1)!
    expect(ultimaUrl).toContain('periodo=90')
    expect(ultimaUrl).toContain('modo=coorte')

    const ultimaQuery = routerMock.replace.mock.calls.at(-1)![0].query
    expect(ultimaQuery).toMatchObject({ pipeline_id: '1', periodo: '90', modo: 'coorte' })
    wrapper.unmount()
  })

  it('lê os filtros iniciais da query string', async () => {
    routeMock.query = { pipeline_id: '2', periodo: 'trimestre', modo: 'coorte', owner_id: '4' }
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = build()
    await flushPromises()

    const url = fetchMock.mock.calls.map((c: any[]) => String(c[0])).find((u) => u.includes('/funil?'))!
    expect(url).toContain('pipeline_id=2')
    expect(url).toContain('periodo=trimestre')
    expect(url).toContain('modo=coorte')
    expect(url).toContain('owner_id=4')
    wrapper.unmount()
  })

  it('vendedor sem forecast.view não vê o filtro de dono nem pede /users', async () => {
    const auth = useAuthStore()
    auth.user = { id: 3, name: 'Ana', email: 'ana@x.com', role: 'seller', active: true, created_at: '', updated_at: '' } as any
    auth.permissions = { 'deals.view': true }
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = build()
    await flushPromises()

    expect(wrapper.find('.sel-dono').exists()).toBe(false)
    expect(fetchMock.mock.calls.some((c: any[]) => String(c[0]).includes('/users'))).toBe(false)
    wrapper.unmount()
  })

  it('mostra toast de erro quando o funil falha', async () => {
    vi.stubGlobal('fetch', stubFetch({ falhaFunil: true }))
    const wrapper = build()
    await flushPromises()

    const { useToastStore } = await import('../src/stores/toast')
    const toasts = useToastStore().toasts
    expect(toasts.some((t) => t.kind === 'error' && t.message.includes('falha ao calcular'))).toBe(true)
    expect(wrapper.text()).toContain('Não foi possível carregar o funil')
    wrapper.unmount()
  })

  it('estado vazio quando não há negócios no período', async () => {
    const vazio: FunilResposta = {
      ...funilFixture,
      resumo: { ...funilFixture.resumo, abertos: 0, ganhos: 0, perdidos: 0 },
      negocios: []
    }
    vi.stubGlobal('fetch', stubFetch({ funil: vazio }))
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum negócio neste período')
    expect(wrapper.findAll('section.bloco')).toHaveLength(0)
    wrapper.unmount()
  })

  it('modo reunião alterna a classe no body, navega por slides e sai com Esc', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const requestFullscreen = vi.fn().mockResolvedValue(undefined)
    ;(document.documentElement as any).requestFullscreen = requestFullscreen
    const wrapper = build()
    await flushPromises()

    await wrapper.find('.btn-reuniao').trigger('click')
    await flushPromises()

    expect(document.body.classList.contains('modo-reuniao')).toBe(true)
    expect(requestFullscreen).toHaveBeenCalled()
    expect(wrapper.find('.reuniao-head').exists()).toBe(true)
    expect(wrapper.find('.reuniao-head').text()).toContain('Vendas')
    expect(wrapper.find('.reuniao-head').text()).toContain('atualizado às')
    expect(wrapper.find('.reuniao-head').text()).toContain('1 / 6')

    // Um bloco por slide: só o primeiro visível.
    const meta = wrapper.find('section[data-bloco="meta"]')
    const mov = wrapper.find('section[data-bloco="movimentacao"]')
    expect(meta.attributes('style') ?? '').not.toContain('display: none')
    expect(mov.attributes('style') ?? '').toContain('display: none')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    await flushPromises()
    expect(wrapper.find('section[data-bloco="movimentacao"]').attributes('style') ?? '').not.toContain('display: none')
    expect(wrapper.find('section[data-bloco="meta"]').attributes('style') ?? '').toContain('display: none')
    expect(wrapper.find('.reuniao-head').text()).toContain('2 / 6')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.body.classList.contains('modo-reuniao')).toBe(false)
    expect(wrapper.find('.reuniao-head').exists()).toBe(false)
    expect(wrapper.find('.btn-reuniao').exists()).toBe(true)
    wrapper.unmount()
  })

  it('desmontar em modo reunião limpa a classe do body', async () => {
    vi.stubGlobal('fetch', stubFetch())
    ;(document.documentElement as any).requestFullscreen = undefined
    const wrapper = build()
    await flushPromises()

    await wrapper.find('.btn-reuniao').trigger('click')
    await flushPromises()
    expect(document.body.classList.contains('modo-reuniao')).toBe(true)

    wrapper.unmount()
    expect(document.body.classList.contains('modo-reuniao')).toBe(false)
  })
})
