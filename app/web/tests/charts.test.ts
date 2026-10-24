import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FunnelChart from '../src/components/charts/FunnelChart.vue'
import WaterfallChart from '../src/components/charts/WaterfallChart.vue'
import GoalBar from '../src/components/charts/GoalBar.vue'
import Sparkline from '../src/components/charts/Sparkline.vue'
import ChartTooltip from '../src/components/charts/ChartTooltip.vue'
import { formatMoneyShort, formatPct, formatSigned } from '../src/components/charts/chart-utils'
import type { FunilEtapa, FunilMovimentacao } from '../src/types-funil'

const stubs = { Teleport: true }

const etapas: FunilEtapa[] = [
  { stage_id: 1, nome: 'Novo Lead', alcancaram: 40, valor: 310000, conversao_total: 1, conversao_etapa: 1, mediana_dias: 3, abertos_agora: 12, valor_aberto: 60000, ponderado: 6000, estagnados: 2 },
  { stage_id: 2, nome: 'Qualificação', alcancaram: 28, valor: 240000, conversao_total: 0.7, conversao_etapa: 0.7, mediana_dias: 6, abertos_agora: 9, valor_aberto: 44000, ponderado: 11000, estagnados: 0 },
  { stage_id: 3, nome: 'Proposta', alcancaram: 18, valor: 170000, conversao_total: 0.45, conversao_etapa: 0.643, mediana_dias: 12, abertos_agora: 6, valor_aberto: 30000, ponderado: 15000, estagnados: 1 },
  { stage_id: 4, nome: 'Negociação', alcancaram: 11, valor: 95000, conversao_total: 0.275, conversao_etapa: 0.611, mediana_dias: 9, abertos_agora: 4, valor_aberto: 14000, ponderado: 11200, estagnados: 0 }
]

const movimentacao: FunilMovimentacao = {
  inicio: { qtd: 28, valor: 120000 },
  criados: { qtd: 14, valor: 70000 },
  aumentos: { qtd: 3, valor: 12000 },
  reducoes: { qtd: 2, valor: -8000 },
  adiados: { qtd: 4, valor: 25000 },
  ganhos: { qtd: 8, valor: -62000 },
  perdidos: { qtd: 5, valor: -21000 },
  fim: { qtd: 31, valor: 148000 }
}

describe('FunnelChart', () => {
  it('desenha uma faixa por etapa, junções entre elas e a tabela oculta', () => {
    const wrapper = mount(FunnelChart, { props: { etapas }, global: { stubs } })

    expect(wrapper.findAll('.funnel-band')).toHaveLength(4)
    expect(wrapper.findAll('.juncao')).toHaveLength(3)
    expect(wrapper.find('svg[role="img"] title').text()).toContain('Funil por etapa')
    expect(wrapper.findAll('table.sr-only tbody tr')).toHaveLength(4)

    // A primeira faixa é a mais larga; a última a mais estreita.
    const larguras = wrapper.findAll('.funnel-band').map((b) => {
      const pts = b.attributes('points')!.split(' ').map((p) => Number(p.split(',')[0]))
      return pts[1] - pts[0]
    })
    expect(larguras[0]).toBeGreaterThan(larguras[1])
    expect(larguras[1]).toBeGreaterThan(larguras[3])

    // Percentual total dentro e percentual etapa→etapa na junção.
    expect(wrapper.findAll('.pct').map((p) => p.text())).toEqual(['100%', '70%', '45%', '28%'])
    expect(wrapper.findAll('.juncao text').map((p) => p.text())).toEqual(['70%', '64%', '61%'])
  })

  it('destaca a etapa mais lenta e os estagnados em vermelho', () => {
    const wrapper = mount(FunnelChart, { props: { etapas }, global: { stubs } })

    const lenta = wrapper.find('.mediana.lenta')
    expect(lenta.exists()).toBe(true)
    expect(lenta.text()).toContain('12 d')
    expect(wrapper.findAll('.estagnados:not(.zero)')).toHaveLength(2)
    expect(wrapper.find('.etapa-g[data-stage-id="1"] .estagnados').text()).toContain('2 estagnado(s)')
  })

  it('emite select com o stage_id ao clicar ou pressionar Enter', async () => {
    const wrapper = mount(FunnelChart, { props: { etapas }, global: { stubs } })

    await wrapper.find('.etapa-g[data-stage-id="3"]').trigger('click')
    await wrapper.find('.etapa-g[data-stage-id="2"]').trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('select')).toEqual([[3], [2]])
  })

  it('marca a etapa selecionada e apaga as demais', () => {
    const wrapper = mount(FunnelChart, { props: { etapas, selecionado: 2 }, global: { stubs } })

    expect(wrapper.find('.etapa-g[data-stage-id="2"]').classes()).toContain('selecionada')
    expect(wrapper.find('.etapa-g[data-stage-id="1"]').classes()).toContain('apagada')
  })

  it('no modo "agora" usa os abertos e não mostra junções de conversão do período', () => {
    const wrapper = mount(FunnelChart, { props: { etapas, modo: 'agora' }, global: { stubs } })

    expect(wrapper.find('.etapa-g[data-stage-id="1"] .nome-sub').text()).toContain('12')
    expect(wrapper.findAll('.pct').map((p) => p.text())[0]).toBe('100%')
    expect(wrapper.findAll('.pct').map((p) => p.text())[1]).toBe('75%') // 9 / 12
  })

  it('mostra tooltip ao passar o mouse', async () => {
    const wrapper = mount(FunnelChart, { props: { etapas }, global: { stubs } })

    await wrapper.find('.etapa-g[data-stage-id="3"]').trigger('mouseenter', { clientX: 10, clientY: 10 })
    expect(wrapper.find('.chart-tooltip').exists()).toBe(true)
    expect(wrapper.find('.chart-tooltip').text()).toContain('Proposta')
    expect(wrapper.find('.chart-tooltip').text()).toContain('Estagnados')

    await wrapper.find('.etapa-g[data-stage-id="3"]').trigger('mouseleave')
    expect(wrapper.find('.chart-tooltip').exists()).toBe(false)
  })

  it('sem etapas mostra mensagem', () => {
    const wrapper = mount(FunnelChart, { props: { etapas: [] }, global: { stubs } })
    expect(wrapper.text()).toContain('Nenhuma etapa para mostrar')
  })
})

describe('WaterfallChart', () => {
  it('desenha sete barras flutuantes com cores por estado e rótulos de valor e quantidade', () => {
    const wrapper = mount(WaterfallChart, { props: { movimentacao }, global: { stubs } })

    const barras = wrapper.findAll('.barra-g')
    expect(barras).toHaveLength(7)
    expect(barras.map((b) => b.attributes('data-key'))).toEqual([
      'inicio', 'criados', 'aumentos', 'reducoes', 'ganhos', 'perdidos', 'fim'
    ])
    expect(wrapper.findAll('.tipo-entrada')).toHaveLength(2)
    expect(wrapper.findAll('.tipo-ganho')).toHaveLength(1)
    expect(wrapper.findAll('.tipo-perdido')).toHaveLength(1)
    expect(wrapper.findAll('.tipo-reducao')).toHaveLength(1)
    expect(wrapper.findAll('.tipo-inicio, .tipo-fim')).toHaveLength(2)

    const texto = wrapper.text().replace(/ /g, ' ')
    expect(texto).toContain('14 neg.')
    expect(texto).toContain('+R$ 70 mil')
    expect(texto).toContain('−R$ 62 mil')
    expect(texto).toContain('4 adiado(s)')
    expect(wrapper.find('svg[role="img"] title').exists()).toBe(true)
    expect(wrapper.findAll('table.sr-only tbody tr')).toHaveLength(8)
  })

  it('barras flutuam: "criados" começa onde "início" termina', () => {
    const wrapper = mount(WaterfallChart, { props: { movimentacao }, global: { stubs } })

    const rect = (key: string) => wrapper.find(`.barra-g[data-key="${key}"] rect.barra`)
    const inicio = rect('inicio')
    const criados = rect('criados')
    const topoInicio = Number(inicio.attributes('y'))
    const baseCriados = Number(criados.attributes('y')) + Number(criados.attributes('height'))
    expect(Math.abs(baseCriados - topoInicio)).toBeLessThan(0.5)

    // "ganhos" é negativo: sua base fica abaixo do topo, no nível acumulado anterior.
    const ganhos = rect('ganhos')
    const fim = rect('fim')
    expect(Number(ganhos.attributes('height'))).toBeGreaterThan(0)
    expect(Number(fim.attributes('y')) + Number(fim.attributes('height'))).toBeCloseTo(Number(inicio.attributes('y')) + Number(inicio.attributes('height')), 1)
  })

  it('emite select com a chave ao clicar numa barra e no botão de adiados', async () => {
    const wrapper = mount(WaterfallChart, { props: { movimentacao }, global: { stubs } })

    await wrapper.find('.barra-g[data-key="perdidos"]').trigger('click')
    await wrapper.find('.adiados').trigger('click')

    expect(wrapper.emitted('select')).toEqual([['perdidos'], ['adiados']])
  })

  it('destaca a barra selecionada', () => {
    const wrapper = mount(WaterfallChart, { props: { movimentacao, selecionado: 'ganhos' }, global: { stubs } })
    expect(wrapper.find('.barra-g[data-key="ganhos"]').classes()).toContain('selecionada')
    expect(wrapper.find('.barra-g[data-key="inicio"]').classes()).toContain('apagada')
  })
})

describe('GoalBar', () => {
  it('ganho sólido, ponderado hachurado e marcador da meta, com semáforo vermelho abaixo de 2×', () => {
    const wrapper = mount(GoalBar, { props: { ganho: 62000, ponderado: 61200, meta: 150000, cobertura: 0.7 } })

    expect(wrapper.find('rect.ganho').exists()).toBe(true)
    expect(wrapper.find('rect.ponderado').attributes('fill')).toMatch(/^url\(#goal-hatch-\d+\)$/)
    expect(wrapper.find('.meta-marker').exists()).toBe(true)
    expect(Number(wrapper.find('rect.ponderado').attributes('x'))).toBeCloseTo(Number(wrapper.find('rect.ganho').attributes('width')), 5)

    const badge = wrapper.find('.cobertura')
    expect(badge.classes()).toContain('semaforo-red')
    expect(badge.text()).toContain('0,7×')
    expect(wrapper.text()).toContain('41% da meta')
  })

  it('semáforo âmbar entre 2 e 3 e verde acima de 3', () => {
    const amber = mount(GoalBar, { props: { ganho: 10000, ponderado: 25000, meta: 20000, cobertura: 2.5 } })
    expect(amber.find('.cobertura').classes()).toContain('semaforo-amber')

    const green = mount(GoalBar, { props: { ganho: 10000, ponderado: 45000, meta: 20000, cobertura: 4.5 } })
    expect(green.find('.cobertura').classes()).toContain('semaforo-green')
  })

  it('calcula a cobertura quando não vem do backend', () => {
    // gap = 20000 − 5000 = 15000; ponderado 45000 → 3,0× (âmbar)
    const wrapper = mount(GoalBar, { props: { ganho: 5000, ponderado: 45000, meta: 20000 } })
    expect(wrapper.find('.cobertura').text()).toContain('3,0×')
    expect(wrapper.find('.cobertura').classes()).toContain('semaforo-amber')
  })

  it('sem meta mostra só ganho e ponderado, sem marcador nem cobertura', () => {
    const wrapper = mount(GoalBar, { props: { ganho: 62000, ponderado: 61200, meta: 0 } })

    expect(wrapper.find('.meta-marker').exists()).toBe(false)
    expect(wrapper.find('.cobertura').exists()).toBe(false)
    expect(wrapper.text()).toContain('Sem meta definida')
    expect(wrapper.find('svg[role="img"] title').text()).toContain('Ganho')
  })
})

describe('Sparkline', () => {
  const pontos = [
    { rotulo: '28/09', valor: 15000 },
    { rotulo: '05/10', valor: 9000 },
    { rotulo: '12/10', valor: 22000 },
    { rotulo: '19/10', valor: 16000 }
  ]

  it('desenha linha, área e o último ponto, com tabela equivalente', () => {
    const wrapper = mount(Sparkline, { props: { pontos, titulo: 'Ganhos por semana' } })

    const linha = wrapper.find('path.spark-line')
    expect(linha.exists()).toBe(true)
    expect(linha.attributes('d')!.split('L')).toHaveLength(4)
    expect(wrapper.find('path.spark-area').attributes('d')).toContain('Z')
    expect(wrapper.find('circle.spark-last').exists()).toBe(true)
    expect(wrapper.find('svg[role="img"] title').text()).toContain('Ganhos por semana')
    expect(wrapper.findAll('table.sr-only tbody tr')).toHaveLength(4)

    // O ponto mais alto (22000) fica mais perto do topo que o mais baixo (9000).
    const ys = linha.attributes('d')!.match(/[\d.]+ [\d.]+/g)!.map((p) => Number(p.split(' ')[1]))
    expect(ys[2]).toBeLessThan(ys[1])
  })

  it('sem pontos mostra mensagem em vez do SVG', () => {
    const wrapper = mount(Sparkline, { props: { pontos: [] } })
    expect(wrapper.find('svg').exists()).toBe(false)
    expect(wrapper.text()).toContain('Sem ganhos no período')
  })
})

describe('ChartTooltip', () => {
  it('mostra título e linhas quando visível e nada quando oculto', () => {
    const visivel = mount(ChartTooltip, {
      props: { visivel: true, x: 10, y: 20, titulo: 'Proposta', linhas: [{ rotulo: 'Valor', valor: 'R$ 1' }] },
      global: { stubs }
    })
    expect(visivel.find('.chart-tooltip').text()).toContain('Proposta')
    expect(visivel.find('.chart-tooltip').text()).toContain('Valor')

    const oculto = mount(ChartTooltip, { props: { visivel: false, x: 0, y: 0 }, global: { stubs } })
    expect(oculto.find('.chart-tooltip').exists()).toBe(false)
  })
})

describe('chart-utils', () => {
  it('formata percentuais, dinheiro curto e sinais em pt-BR', () => {
    expect(formatPct(0.615)).toBe('62%')
    expect(formatPct(0.615, 1)).toBe('61,5%')
    expect(formatMoneyShort(148000).replace(/ /g, ' ')).toBe('R$ 148 mil')
    expect(formatMoneyShort(950).replace(/ /g, ' ')).toBe('R$ 950,00')
    expect(formatSigned(-8000).replace(/ /g, ' ')).toBe('−R$ 8 mil')
    expect(formatSigned(12000).replace(/ /g, ' ')).toBe('+R$ 12 mil')
  })
})
