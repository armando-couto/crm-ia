<script setup lang="ts">
import { computed, ref } from 'vue'
import type { SparkPonto } from '../../types-funil'
import { formatMoneyShort, prefersReducedMotion } from './chart-utils'

const props = withDefaults(
  defineProps<{
    pontos: SparkPonto[]
    titulo?: string
    altura?: number
    dinheiro?: boolean
  }>(),
  { titulo: 'Evolução', altura: 56, dinheiro: true }
)

const W = 300
const H = computed(() => props.altura)
const PAD = 6

const animar = ref(!prefersReducedMotion())

const coords = computed(() => {
  const pts = props.pontos ?? []
  if (!pts.length) return []
  const max = Math.max(1, ...pts.map((p) => p.valor))
  const step = pts.length > 1 ? (W - PAD * 2) / (pts.length - 1) : 0
  return pts.map((p, i) => {
    const x = pts.length > 1 ? PAD + i * step : W / 2
    const y = H.value - PAD - (p.valor / max) * (H.value - PAD * 2)
    return { x: Number(x.toFixed(2)), y: Number(y.toFixed(2)), ...p }
  })
})

const linha = computed(() => coords.value.map((c, i) => `${i === 0 ? 'M' : 'L'} ${c.x} ${c.y}`).join(' '))

const area = computed(() => {
  if (!coords.value.length) return ''
  const first = coords.value[0]
  const last = coords.value[coords.value.length - 1]
  return `${linha.value} L ${last.x} ${H.value - PAD} L ${first.x} ${H.value - PAD} Z`
})

const ultimo = computed(() => coords.value[coords.value.length - 1] ?? null)

function fmt(v: number): string {
  return props.dinheiro ? formatMoneyShort(v) : String(v)
}

const descricao = computed(() => {
  if (!coords.value.length) return `${props.titulo}: sem dados`
  return `${props.titulo}: ${coords.value.map((c) => `${c.rotulo} ${fmt(c.valor)}`).join(', ')}`
})
</script>

<template>
  <figure class="sparkline" :class="{ animar }">
    <svg
      v-if="coords.length"
      :viewBox="`0 0 ${W} ${H}`"
      preserveAspectRatio="none"
      role="img"
      :aria-label="descricao"
      class="spark-svg"
      :style="{ height: `${H}px` }"
    >
      <title>{{ descricao }}</title>
      <path class="spark-area" :d="area" />
      <path class="spark-line" :d="linha" pathLength="1" vector-effect="non-scaling-stroke" />
      <circle v-if="ultimo" class="spark-last" :cx="ultimo.x" :cy="ultimo.y" r="3.5" vector-effect="non-scaling-stroke" />
    </svg>
    <p v-else class="spark-empty muted">Sem ganhos no período.</p>

    <table class="sr-only">
      <caption>{{ titulo }}</caption>
      <thead>
        <tr><th>Semana</th><th>Valor</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in pontos" :key="p.rotulo">
          <td>{{ p.rotulo }}</td>
          <td>{{ fmt(p.valor) }}</td>
        </tr>
      </tbody>
    </table>
  </figure>
</template>

<style scoped>
.sparkline {
  margin: 0;
  width: 100%;
}

.spark-svg {
  width: 100%;
  display: block;
  overflow: visible;
}

.spark-area {
  fill: var(--ci-purple, #6d5df6);
  fill-opacity: 0.12;
}

.spark-line {
  fill: none;
  stroke: var(--ci-purple, #6d5df6);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}

.spark-last {
  fill: var(--ci-purple, #6d5df6);
  stroke: var(--ci-surface, #fff);
  stroke-width: 2;
}

.animar .spark-line {
  stroke-dasharray: 1;
  stroke-dashoffset: 1;
  animation: spark-draw 0.9s ease-out forwards;
}

.animar .spark-area {
  animation: spark-fade 0.9s ease-out forwards;
  opacity: 0;
}

@keyframes spark-draw {
  to {
    stroke-dashoffset: 0;
  }
}

@keyframes spark-fade {
  to {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .animar .spark-line,
  .animar .spark-area {
    animation: none;
    stroke-dashoffset: 0;
    opacity: 1;
  }
}

.spark-empty {
  margin: 0;
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
