<script setup lang="ts">
import { computed, ref } from 'vue'
import { formatMoney } from '../../format'
import { chartId, formatMoneyShort, prefersReducedMotion } from './chart-utils'

const props = defineProps<{
  ganho: number
  ponderado: number
  meta: number
  /** Cobertura já calculada pelo backend (ponderado / gap). Opcional: calcula se faltar. */
  cobertura?: number | null
}>()

const W = 600
const H = 28
const padId = chartId('goal-hatch')
const animar = ref(!prefersReducedMotion())

const temMeta = computed(() => props.meta > 0)

const escala = computed(() => {
  const soma = props.ganho + props.ponderado
  const base = temMeta.value ? Math.max(props.meta, soma) : Math.max(soma, 1)
  return base * 1.04
})

function px(valor: number): number {
  if (escala.value <= 0) return 0
  return Math.max(0, Math.min(W, (valor / escala.value) * W))
}

const ganhoW = computed(() => px(props.ganho))
const ponderadoW = computed(() => px(props.ponderado))
const metaX = computed(() => px(props.meta))

const pctGanho = computed(() => (temMeta.value ? props.ganho / props.meta : 0))

const coberturaCalc = computed(() => {
  if (props.cobertura != null) return props.cobertura
  const gap = Math.max(props.meta - props.ganho, 0)
  if (gap <= 0) return Infinity
  return props.ponderado / gap
})

/** Semáforo de cobertura: < 2 vermelho, 2–3 âmbar, > 3 verde. */
const semaforo = computed<'red' | 'amber' | 'green'>(() => {
  const c = coberturaCalc.value
  if (c < 2) return 'red'
  if (c <= 3) return 'amber'
  return 'green'
})

const coberturaTexto = computed(() => {
  const c = coberturaCalc.value
  if (!Number.isFinite(c)) return 'meta batida'
  return `${c.toFixed(1).replace('.', ',')}× de cobertura`
})

const descricao = computed(() => {
  const partes = [`Ganho ${formatMoney(props.ganho)}`, `ponderado ${formatMoney(props.ponderado)}`]
  if (temMeta.value) partes.push(`meta ${formatMoney(props.meta)}`, coberturaTexto.value)
  return `Meta × realizado: ${partes.join(', ')}`
})
</script>

<template>
  <figure class="goal-bar" :class="{ animar }">
    <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" role="img" :aria-label="descricao" class="goal-svg">
      <title>{{ descricao }}</title>
      <defs>
        <pattern :id="padId" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
          <rect width="8" height="8" class="hatch-bg" />
          <rect width="4" height="8" class="hatch-fg" />
        </pattern>
      </defs>
      <rect class="track" x="0" y="4" :width="W" :height="H - 8" rx="6" />
      <rect class="ganho" x="0" y="4" :width="ganhoW" :height="H - 8" rx="6" />
      <rect class="ponderado" :x="ganhoW" y="4" :width="ponderadoW" :height="H - 8" :fill="`url(#${padId})`" />
      <g v-if="temMeta" class="meta-marker">
        <line :x1="metaX" :x2="metaX" y1="0" :y2="H" vector-effect="non-scaling-stroke" />
      </g>
    </svg>

    <div class="legenda">
      <span class="item"><i class="sw ganho"></i>Ganho <strong>{{ formatMoneyShort(ganho) }}</strong>
        <span v-if="temMeta" class="muted">({{ Math.round(pctGanho * 100) }}% da meta)</span>
      </span>
      <span class="item"><i class="sw pond"></i>Ponderado <strong>{{ formatMoneyShort(ponderado) }}</strong></span>
      <span v-if="temMeta" class="item"><i class="sw meta"></i>Meta <strong>{{ formatMoneyShort(meta) }}</strong></span>
      <span v-if="temMeta" class="cobertura" :class="`semaforo-${semaforo}`" :title="'Ponderado dividido pelo que falta para a meta'">
        {{ coberturaTexto }}
      </span>
      <span v-else class="muted sem-meta">Sem meta definida para o período.</span>
    </div>

    <table class="sr-only">
      <caption>Meta × realizado</caption>
      <tbody>
        <tr><th>Ganho</th><td>{{ formatMoney(ganho) }}</td></tr>
        <tr><th>Ponderado</th><td>{{ formatMoney(ponderado) }}</td></tr>
        <tr v-if="temMeta"><th>Meta</th><td>{{ formatMoney(meta) }}</td></tr>
        <tr v-if="temMeta"><th>Cobertura</th><td>{{ coberturaTexto }}</td></tr>
      </tbody>
    </table>
  </figure>
</template>

<style scoped>
.goal-bar {
  margin: 0;
  width: 100%;
}

.goal-svg {
  width: 100%;
  height: 28px;
  display: block;
  overflow: visible;
}

.track {
  fill: var(--ci-bg, #f7f5fa);
}

.ganho {
  fill: var(--ok, var(--ci-green, #1d9e6f));
}

.hatch-bg {
  fill: var(--ci-purple-tint, #efedfe);
}

.hatch-fg {
  fill: var(--ci-purple, #6d5df6);
  fill-opacity: 0.55;
}

.meta-marker line {
  stroke: var(--ci-text, #222);
  stroke-width: 2;
  stroke-dasharray: 3 2;
}

.animar .ganho,
.animar .ponderado {
  transform-box: fill-box;
  transform-origin: left center;
  animation: goal-grow 0.7s ease-out both;
}

.animar .ponderado {
  animation-delay: 0.25s;
}

@keyframes goal-grow {
  from {
    transform: scaleX(0);
  }
  to {
    transform: scaleX(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .animar .ganho,
  .animar .ponderado {
    animation: none;
  }
}

.legenda {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 18px;
  margin-top: 10px;
  font-size: 13px;
  color: var(--ci-text-2, #464646);
}

.item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.sw {
  width: 12px;
  height: 12px;
  border-radius: 3px;
  display: inline-block;
}

.sw.ganho {
  background: var(--ok, var(--ci-green, #1d9e6f));
}

.sw.pond {
  background: repeating-linear-gradient(
    45deg,
    var(--ci-purple, #6d5df6) 0 3px,
    var(--ci-purple-tint, #efedfe) 3px 6px
  );
}

.sw.meta {
  width: 2px;
  border-radius: 0;
  background: var(--ci-text, #222);
}

.cobertura {
  margin-left: auto;
  padding: 3px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.semaforo-red {
  background: var(--ci-red-tint, #fbe9ea);
  color: var(--danger, var(--ci-red, #d64550));
}

.semaforo-amber {
  background: var(--ci-amber-tint, #fdf3df);
  color: var(--warn, var(--ci-amber, #c78a1b));
}

.semaforo-green {
  background: var(--ci-green-tint, #e3f6ee);
  color: var(--ok, var(--ci-green, #1d9e6f));
}

.sem-meta {
  margin-left: auto;
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
