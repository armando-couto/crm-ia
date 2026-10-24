<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FunilMovimentacao, MovimentacaoChave, WaterfallPasso } from '../../types-funil'
import { formatMoney } from '../../format'
import ChartTooltip from './ChartTooltip.vue'
import { formatMoneyShort, formatSigned, prefersReducedMotion, useTooltip } from './chart-utils'

const props = defineProps<{
  movimentacao: FunilMovimentacao
  selecionado?: MovimentacaoChave | null
}>()

const emit = defineEmits<{ select: [key: MovimentacaoChave] }>()

const { tooltip, mostrar, mover, esconder } = useTooltip()
const animar = ref(!prefersReducedMotion())

const W = 720
const H = 280
const TOP = 34
const BOTTOM = 52
const LEFT = 8
const RIGHT = 8
const plotH = H - TOP - BOTTOM

/** Ordem fixa das barras; "adiados" fica fora da soma (só informativo). */
const passos = computed<WaterfallPasso[]>(() => {
  const m = props.movimentacao
  return [
    { key: 'inicio', rotulo: 'Início', valor: m.inicio.valor, qtd: m.inicio.qtd, tipo: 'inicio' },
    { key: 'criados', rotulo: 'Criados', valor: m.criados.valor, qtd: m.criados.qtd, tipo: 'entrada' },
    { key: 'aumentos', rotulo: 'Aumentos', valor: m.aumentos.valor, qtd: m.aumentos.qtd, tipo: 'entrada' },
    { key: 'reducoes', rotulo: 'Reduções', valor: m.reducoes.valor, qtd: m.reducoes.qtd, tipo: 'reducao' },
    { key: 'ganhos', rotulo: 'Ganhos', valor: m.ganhos.valor, qtd: m.ganhos.qtd, tipo: 'ganho' },
    { key: 'perdidos', rotulo: 'Perdidos', valor: m.perdidos.valor, qtd: m.perdidos.qtd, tipo: 'perdido' },
    { key: 'fim', rotulo: 'Fim', valor: m.fim.valor, qtd: m.fim.qtd, tipo: 'fim' }
  ]
})

interface Barra extends WaterfallPasso {
  de: number
  ate: number
  x: number
  y: number
  w: number
  h: number
  labelY: number
}

/** Faixas (de → até) de cada barra e a escala vertical comum. */
const escala = computed(() => {
  let acumulado = 0
  const faixas = passos.value.map((p) => {
    if (p.tipo === 'inicio' || p.tipo === 'fim') {
      acumulado = p.valor
      return { de: 0, ate: p.valor }
    }
    const de = acumulado
    acumulado += p.valor
    return { de, ate: acumulado }
  })
  const maxY = Math.max(1, ...faixas.flatMap((f) => [f.de, f.ate]))
  const minY = Math.min(0, ...faixas.flatMap((f) => [f.de, f.ate]))
  return { faixas, maxY, minY, amplitude: maxY - minY || 1 }
})

function yDe(valor: number): number {
  const { maxY, amplitude } = escala.value
  return TOP + ((maxY - valor) / amplitude) * plotH
}

const barras = computed<Barra[]>(() => {
  const lista = passos.value
  const n = lista.length
  const slot = (W - LEFT - RIGHT) / n
  const barW = Math.min(72, slot * 0.62)

  return lista.map((p, i) => {
    const f = escala.value.faixas[i]
    const topo = Math.max(f.de, f.ate)
    const base = Math.min(f.de, f.ate)
    const y = yDe(topo)
    const h = Math.max(f.de === f.ate ? 0 : 2, yDe(base) - y)
    const x = LEFT + i * slot + (slot - barW) / 2
    return { ...p, de: f.de, ate: f.ate, x, y, w: barW, h, labelY: y - 8 }
  })
})

/** Linhas pontilhadas ligando o nível final de uma barra ao início da seguinte. */
const conectores = computed(() =>
  barras.value.slice(0, -1).map((b, i) => ({ x1: b.x + b.w, x2: barras.value[i + 1].x, y: yDe(b.ate) }))
)

const fecha = computed(() => {
  const m = props.movimentacao
  const soma =
    m.inicio.valor + m.criados.valor + m.aumentos.valor + m.reducoes.valor + m.ganhos.valor + m.perdidos.valor
  return Math.abs(soma - m.fim.valor) < 1
})

function rotuloValor(b: Barra): string {
  if (b.tipo === 'inicio' || b.tipo === 'fim') return formatMoneyShort(b.valor)
  return formatSigned(b.valor)
}

function onEnter(event: MouseEvent, b: Barra) {
  mostrar(event, b.rotulo, [
    { rotulo: 'Valor', valor: formatMoney(Math.abs(b.valor)), tom: b.tipo === 'ganho' ? 'ok' : b.tipo === 'perdido' ? 'danger' : undefined },
    { rotulo: 'Negócios', valor: String(b.qtd) },
    ...(b.tipo !== 'inicio' && b.tipo !== 'fim'
      ? [{ rotulo: 'Pipeline após', valor: formatMoney(b.ate), tom: 'muted' as const }]
      : [])
  ])
}

function selecionar(key: MovimentacaoChave) {
  emit('select', key)
}

const descricao = computed(
  () =>
    `Movimentação do pipeline: ${passos.value.map((p) => `${p.rotulo} ${formatMoney(p.valor)} (${p.qtd})`).join(', ')}`
)
</script>

<template>
  <figure class="waterfall" :class="{ animar }">
    <svg :viewBox="`0 0 ${W} ${H}`" role="img" :aria-label="descricao" class="wf-svg">
      <title>{{ descricao }}</title>
      <line class="eixo" :x1="LEFT" :x2="W - RIGHT" :y1="TOP + plotH" :y2="TOP + plotH" />
      <line
        v-for="(c, i) in conectores"
        :key="`c${i}`"
        class="conector"
        :x1="c.x1"
        :x2="c.x2"
        :y1="c.y"
        :y2="c.y"
      />
      <g
        v-for="(b, i) in barras"
        :key="b.key"
        class="barra-g"
        :class="[`tipo-${b.tipo}`, { selecionada: selecionado === b.key, apagada: selecionado && selecionado !== b.key }]"
        role="button"
        tabindex="0"
        :aria-label="`${b.rotulo}: ${formatMoney(b.valor)}, ${b.qtd} negócios. Clique para filtrar.`"
        :data-key="b.key"
        :style="{ '--i': i }"
        @click="selecionar(b.key)"
        @keydown.enter.prevent="selecionar(b.key)"
        @keydown.space.prevent="selecionar(b.key)"
        @mouseenter="onEnter($event, b)"
        @mousemove="mover"
        @mouseleave="esconder"
      >
        <rect class="hit" :x="b.x - 10" :y="TOP - 20" :width="b.w + 20" :height="plotH + 20 + BOTTOM" />
        <rect class="barra" :x="b.x" :y="b.y" :width="b.w" :height="b.h" rx="3" />
        <text class="valor" :x="b.x + b.w / 2" :y="b.labelY" text-anchor="middle">{{ rotuloValor(b) }}</text>
        <text class="rotulo" :x="b.x + b.w / 2" :y="TOP + plotH + 18" text-anchor="middle">{{ b.rotulo }}</text>
        <text class="qtd" :x="b.x + b.w / 2" :y="TOP + plotH + 34" text-anchor="middle">{{ b.qtd }} neg.</text>
      </g>
    </svg>

    <div class="rodape">
      <button
        type="button"
        class="adiados"
        :class="{ on: selecionado === 'adiados' }"
        title="Negócios cuja data de fechamento foi empurrada no período (não entra na soma)"
        @click="selecionar('adiados')"
      >
        → {{ movimentacao.adiados.qtd }} adiado(s) · {{ formatMoneyShort(movimentacao.adiados.valor) }}
      </button>
      <span v-if="!fecha" class="aviso muted">A soma não fecha exatamente com o fim (arredondamentos).</span>
    </div>

    <table class="sr-only">
      <caption>Movimentação do pipeline</caption>
      <thead>
        <tr><th>Passo</th><th>Valor</th><th>Negócios</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in passos" :key="p.key">
          <td>{{ p.rotulo }}</td>
          <td>{{ formatMoney(p.valor) }}</td>
          <td>{{ p.qtd }}</td>
        </tr>
        <tr>
          <td>Adiados (informativo)</td>
          <td>{{ formatMoney(movimentacao.adiados.valor) }}</td>
          <td>{{ movimentacao.adiados.qtd }}</td>
        </tr>
      </tbody>
    </table>

    <ChartTooltip :visivel="tooltip.visivel" :x="tooltip.x" :y="tooltip.y" :titulo="tooltip.titulo" :linhas="tooltip.linhas" />
  </figure>
</template>

<style scoped>
.waterfall {
  margin: 0;
  width: 100%;
  --teal: #0e9f8a;
  --laranja: #e07b2a;
}

.wf-svg {
  width: 100%;
  height: auto;
  display: block;
  overflow: visible;
  font-family: inherit;
}

.eixo {
  stroke: var(--ci-border, #e4e4e4);
  stroke-width: 1;
}

.conector {
  stroke: var(--ci-text-3, #8e8e8e);
  stroke-width: 1;
  stroke-dasharray: 3 3;
  opacity: 0.7;
}

.barra-g {
  cursor: pointer;
  outline: none;
}

.hit {
  fill: transparent;
}

.barra {
  transition: opacity 0.15s, filter 0.15s;
}

.tipo-inicio .barra,
.tipo-fim .barra {
  fill: var(--ci-purple, #6d5df6);
}

.tipo-entrada .barra {
  fill: var(--teal);
}

.tipo-ganho .barra {
  fill: var(--ok, var(--ci-green, #1d9e6f));
}

.tipo-perdido .barra {
  fill: var(--danger, var(--ci-red, #d64550));
}

.tipo-reducao .barra {
  fill: var(--warn, var(--laranja));
}

.barra-g:hover .barra,
.barra-g:focus-visible .barra {
  filter: brightness(1.1);
}

.barra-g.selecionada .barra {
  stroke: var(--ci-text, #222);
  stroke-width: 2;
}

.barra-g.apagada .barra {
  opacity: 0.35;
}

.valor {
  font-size: 12px;
  font-weight: 600;
  fill: var(--ci-text, #222);
  font-variant-numeric: tabular-nums;
}

.rotulo {
  font-size: 12px;
  fill: var(--ci-text-2, #464646);
}

.qtd {
  font-size: 11px;
  fill: var(--ci-text-3, #8e8e8e);
}

.animar .barra {
  transform-box: fill-box;
  transform-origin: center bottom;
  animation: wf-grow 0.5s ease-out both;
  animation-delay: calc(var(--i, 0) * 70ms);
}

.animar .valor {
  animation: wf-fade 0.4s ease-out both;
  animation-delay: calc(var(--i, 0) * 70ms + 0.3s);
}

@keyframes wf-grow {
  from {
    transform: scaleY(0);
  }
  to {
    transform: scaleY(1);
  }
}

@keyframes wf-fade {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .animar .barra,
  .animar .valor {
    animation: none;
  }
}

.rodape {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.adiados {
  border: 1px solid var(--ci-border, #e4e4e4);
  background: var(--ci-surface, #fff);
  color: var(--warn, var(--ci-amber, #c78a1b));
  border-radius: 999px;
  padding: 3px 10px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
}

.adiados:hover,
.adiados.on {
  border-color: var(--warn, var(--ci-amber, #c78a1b));
  background: var(--ci-amber-tint, #fdf3df);
}

.aviso {
  font-size: 12px;
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
