<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FunilEtapa } from '../../types-funil'
import { formatMoney } from '../../format'
import ChartTooltip from './ChartTooltip.vue'
import { formatDias, formatMoneyShort, formatPct, prefersReducedMotion, useTooltip } from './chart-utils'

const props = withDefaults(
  defineProps<{
    etapas: FunilEtapa[]
    /** "periodo" usa alcancaram/valor; "agora" usa abertos_agora/valor_aberto. */
    modo?: 'periodo' | 'agora'
    selecionado?: number | null
  }>(),
  { modo: 'periodo', selecionado: null }
)

const emit = defineEmits<{ select: [stageId: number] }>()

const { tooltip, mostrar, mover, esconder } = useTooltip()
const animar = ref(!prefersReducedMotion())

// Geometria: coluna de nomes à esquerda, faixas no centro, métricas à direita.
const W = 760
const NOME_W = 150
const FAIXA_X = NOME_W + 10
const FAIXA_W = 380
const INFO_X = FAIXA_X + FAIXA_W + 24
const ROW_H = 52
const GAP_H = 20
const PAD_Y = 6

const H = computed(() => PAD_Y * 2 + props.etapas.length * ROW_H + Math.max(0, props.etapas.length - 1) * GAP_H)

function medida(e: FunilEtapa): number {
  return props.modo === 'agora' ? e.abertos_agora : e.alcancaram
}

function valorDe(e: FunilEtapa): number {
  return props.modo === 'agora' ? e.valor_aberto : e.valor
}

const maximo = computed(() => Math.max(1, ...props.etapas.map(medida)))

const maisLenta = computed(() => {
  const candidatas = props.etapas.filter((e) => e.mediana_dias > 0)
  if (!candidatas.length) return null
  return candidatas.reduce((a, b) => (b.mediana_dias > a.mediana_dias ? b : a)).stage_id
})

interface Faixa {
  etapa: FunilEtapa
  i: number
  y: number
  wTopo: number
  wBase: number
  pontos: string
  pctTotal: number
  rotuloDentro: boolean
  qtd: number
  valor: number
}

function largura(qtd: number): number {
  if (qtd <= 0) return FAIXA_W * 0.02
  return Math.max(FAIXA_W * 0.08, (qtd / maximo.value) * FAIXA_W)
}

const faixas = computed<Faixa[]>(() => {
  const lista = props.etapas
  const centro = FAIXA_X + FAIXA_W / 2
  const primeiro = lista.length ? medida(lista[0]) : 0
  return lista.map((e, i) => {
    const qtd = medida(e)
    const wTopo = largura(qtd)
    const prox = lista[i + 1]
    const wBase = prox ? largura(medida(prox)) : wTopo * 0.82
    const y = PAD_Y + i * (ROW_H + GAP_H)
    const pontos = [
      [centro - wTopo / 2, y],
      [centro + wTopo / 2, y],
      [centro + wBase / 2, y + ROW_H],
      [centro - wBase / 2, y + ROW_H]
    ]
      .map(([x, yy]) => `${x.toFixed(1)},${yy.toFixed(1)}`)
      .join(' ')
    const pctTotal = props.modo === 'agora' ? (primeiro > 0 ? qtd / primeiro : 0) : e.conversao_total
    return {
      etapa: e,
      i,
      y,
      wTopo,
      wBase,
      pontos,
      pctTotal,
      rotuloDentro: Math.min(wTopo, wBase) >= 90,
      qtd,
      valor: valorDe(e)
    }
  })
})

/** Rótulo "% etapa→etapa" na junção entre a etapa i e a i+1. */
const juncoes = computed(() =>
  faixas.value.slice(1).map((f) => ({
    stage_id: f.etapa.stage_id,
    y: f.y - GAP_H / 2,
    texto: formatPct(props.modo === 'agora' ? (faixas.value[f.i - 1].qtd > 0 ? f.qtd / faixas.value[f.i - 1].qtd : 0) : f.etapa.conversao_etapa)
  }))
)

const centroX = FAIXA_X + FAIXA_W / 2

function selecionar(stageId: number) {
  emit('select', stageId)
}

function onEnter(event: MouseEvent, f: Faixa) {
  const e = f.etapa
  mostrar(event, e.nome, [
    { rotulo: props.modo === 'agora' ? 'Abertos agora' : 'Alcançaram', valor: String(f.qtd) },
    { rotulo: 'Valor', valor: formatMoney(f.valor) },
    { rotulo: 'Conversão total', valor: formatPct(f.pctTotal) },
    ...(f.i > 0 ? [{ rotulo: 'Da etapa anterior', valor: formatPct(e.conversao_etapa) }] : []),
    { rotulo: 'Mediana na etapa', valor: formatDias(e.mediana_dias), tom: maisLenta.value === e.stage_id ? ('warn' as const) : undefined },
    { rotulo: 'Estagnados', valor: String(e.estagnados), tom: e.estagnados > 0 ? ('danger' as const) : ('muted' as const) },
    { rotulo: 'Ponderado', valor: formatMoney(e.ponderado), tom: 'muted' as const }
  ])
}

const descricao = computed(
  () =>
    `Funil por etapa (${props.modo === 'agora' ? 'abertos agora' : 'no período'}): ${props.etapas
      .map((e) => `${e.nome} ${medida(e)} (${formatPct(e.conversao_total)})`)
      .join(', ')}`
)
</script>

<template>
  <figure class="funnel" :class="{ animar }">
    <svg v-if="etapas.length" :viewBox="`0 0 ${W} ${H}`" role="img" :aria-label="descricao" class="funnel-svg">
      <title>{{ descricao }}</title>

      <g
        v-for="f in faixas"
        :key="f.etapa.stage_id"
        class="etapa-g"
        :class="{ selecionada: selecionado === f.etapa.stage_id, apagada: selecionado != null && selecionado !== f.etapa.stage_id }"
        role="button"
        tabindex="0"
        :data-stage-id="f.etapa.stage_id"
        :aria-label="`${f.etapa.nome}: ${f.qtd} negócios, ${formatPct(f.pctTotal)} do total. Clique para filtrar.`"
        :style="{ '--i': f.i }"
        @click="selecionar(f.etapa.stage_id)"
        @keydown.enter.prevent="selecionar(f.etapa.stage_id)"
        @keydown.space.prevent="selecionar(f.etapa.stage_id)"
        @mouseenter="onEnter($event, f)"
        @mousemove="mover"
        @mouseleave="esconder"
      >
        <rect class="hit" x="0" :y="f.y - GAP_H / 2" :width="W" :height="ROW_H + GAP_H" />

        <text class="nome" :x="NOME_W" :y="f.y + ROW_H / 2 - 4" text-anchor="end">{{ f.etapa.nome }}</text>
        <text class="nome-sub" :x="NOME_W" :y="f.y + ROW_H / 2 + 12" text-anchor="end">
          {{ f.qtd }} · {{ formatMoneyShort(f.valor) }}
        </text>

        <polygon class="funnel-band" :points="f.pontos" />

        <text
          class="pct"
          :class="{ fora: !f.rotuloDentro }"
          :x="f.rotuloDentro ? centroX : centroX + Math.max(f.wTopo, f.wBase) / 2 + 8"
          :y="f.y + ROW_H / 2 + 5"
          :text-anchor="f.rotuloDentro ? 'middle' : 'start'"
        >
          {{ formatPct(f.pctTotal) }}
        </text>

        <g class="info">
          <text
            class="mediana"
            :class="{ lenta: maisLenta === f.etapa.stage_id }"
            :x="INFO_X"
            :y="f.y + ROW_H / 2 - 4"
          >
            {{ maisLenta === f.etapa.stage_id ? '⏱ ' : '' }}mediana {{ formatDias(f.etapa.mediana_dias) }}
          </text>
          <text class="estagnados" :class="{ zero: !f.etapa.estagnados }" :x="INFO_X" :y="f.y + ROW_H / 2 + 12">
            {{ f.etapa.estagnados ? `${f.etapa.estagnados} estagnado(s)` : 'sem estagnados' }}
          </text>
        </g>
      </g>

      <g v-for="j in juncoes" :key="`j${j.stage_id}`" class="juncao">
        <rect :x="centroX - 26" :y="j.y - 9" width="52" height="18" rx="9" />
        <text :x="centroX" :y="j.y + 4" text-anchor="middle">{{ j.texto }}</text>
      </g>
    </svg>
    <p v-else class="muted vazio">Nenhuma etapa para mostrar.</p>

    <table class="sr-only">
      <caption>Funil por etapa</caption>
      <thead>
        <tr>
          <th>Etapa</th>
          <th>{{ modo === 'agora' ? 'Abertos agora' : 'Alcançaram' }}</th>
          <th>Valor</th>
          <th>Conversão total</th>
          <th>Conversão da etapa anterior</th>
          <th>Mediana (dias)</th>
          <th>Estagnados</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="f in faixas" :key="f.etapa.stage_id">
          <td>{{ f.etapa.nome }}</td>
          <td>{{ f.qtd }}</td>
          <td>{{ formatMoney(f.valor) }}</td>
          <td>{{ formatPct(f.pctTotal) }}</td>
          <td>{{ formatPct(f.etapa.conversao_etapa) }}</td>
          <td>{{ f.etapa.mediana_dias }}</td>
          <td>{{ f.etapa.estagnados }}</td>
        </tr>
      </tbody>
    </table>

    <ChartTooltip :visivel="tooltip.visivel" :x="tooltip.x" :y="tooltip.y" :titulo="tooltip.titulo" :linhas="tooltip.linhas" />
  </figure>
</template>

<style scoped>
.funnel {
  margin: 0;
  width: 100%;
}

.funnel-svg {
  width: 100%;
  height: auto;
  display: block;
  overflow: visible;
  font-family: inherit;
}

.etapa-g {
  cursor: pointer;
  outline: none;
}

.hit {
  fill: transparent;
}

.funnel-band {
  fill: var(--ci-purple, #6d5df6);
  fill-opacity: 0.88;
  transition: fill-opacity 0.15s, filter 0.15s;
}

.etapa-g:hover .funnel-band,
.etapa-g:focus-visible .funnel-band {
  fill-opacity: 1;
  filter: brightness(1.08);
}

.etapa-g.selecionada .funnel-band {
  fill-opacity: 1;
  stroke: var(--ci-text, #222);
  stroke-width: 2;
}

.etapa-g.apagada .funnel-band {
  fill-opacity: 0.35;
}

.nome {
  font-size: 13px;
  font-weight: 600;
  fill: var(--ci-text, #222);
}

.nome-sub {
  font-size: 11px;
  fill: var(--ci-text-3, #8e8e8e);
  font-variant-numeric: tabular-nums;
}

.pct {
  font-size: 14px;
  font-weight: 700;
  fill: #fff;
  pointer-events: none;
}

.pct.fora {
  fill: var(--ci-text, #222);
}

.mediana {
  font-size: 12px;
  fill: var(--ci-text-2, #464646);
}

.mediana.lenta {
  fill: var(--warn, var(--ci-amber, #c78a1b));
  font-weight: 700;
}

.estagnados {
  font-size: 12px;
  font-weight: 600;
  fill: var(--danger, var(--ci-red, #d64550));
}

.estagnados.zero {
  fill: var(--ci-text-3, #8e8e8e);
  font-weight: 400;
}

.juncao rect {
  fill: var(--ci-surface, #fff);
  stroke: var(--ci-border, #e4e4e4);
}

.juncao text {
  font-size: 11px;
  font-weight: 600;
  fill: var(--ci-purple-dark, #4b3fd1);
}

.animar .funnel-band {
  transform-box: fill-box;
  transform-origin: center;
  animation: funnel-grow 0.5s ease-out both;
  animation-delay: calc(var(--i, 0) * 60ms);
}

@keyframes funnel-grow {
  from {
    transform: scaleX(0);
  }
  to {
    transform: scaleX(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .animar .funnel-band {
    animation: none;
  }
}

.vazio {
  margin: 0;
  padding: 16px 0;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
