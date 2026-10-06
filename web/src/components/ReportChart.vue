<script setup lang="ts">
import { computed } from 'vue'
import { formatMoney } from '../format'
import type { ReportResult } from '../types'

const props = defineProps<{
  result: ReportResult
  chart: string
}>()

const rows = computed(() => props.result.rows ?? [])
const maxValue = computed(() => Math.max(1, ...rows.value.map((r) => r.value)))

function fmt(value: number): string {
  return props.result.is_money ? formatMoney(value) : String(Math.round(value))
}

// Paleta derivada do roxo da Fix Pay, para o gráfico de pizza.
const palette = [
  '#9B52DF', '#7B3FB8', '#C084F5', '#5E2E8A', '#D8B4FE',
  '#4C1D95', '#A78BFA', '#6D28D9', '#E9D5FF', '#3B0764'
]

function color(i: number): string {
  return palette[i % palette.length]
}

/** Fatias da pizza como caminhos SVG, a partir do ângulo acumulado. */
const slices = computed(() => {
  const total = rows.value.reduce((sum, r) => sum + r.value, 0)
  if (total <= 0) return []

  let angle = -Math.PI / 2
  return rows.value.map((row, i) => {
    const portion = row.value / total
    const sweep = portion * Math.PI * 2
    const start = angle
    const end = angle + sweep
    angle = end

    const r = 70
    const cx = 80
    const cy = 80
    const x1 = cx + r * Math.cos(start)
    const y1 = cy + r * Math.sin(start)
    const x2 = cx + r * Math.cos(end)
    const y2 = cy + r * Math.sin(end)
    const largeArc = sweep > Math.PI ? 1 : 0

    // Fatia única vira círculo completo (o arco de 360° degenera).
    const d =
      portion >= 0.999
        ? `M ${cx} ${cy - r} A ${r} ${r} 0 1 1 ${cx - 0.01} ${cy - r} Z`
        : `M ${cx} ${cy} L ${x1} ${y1} A ${r} ${r} 0 ${largeArc} 1 ${x2} ${y2} Z`

    return { d, color: color(i), label: row.label, value: row.value, percent: portion * 100 }
  })
})

/** Pontos de uma série do progresso, na mesma grade das demais. */
function seriesPoints(serie: { points: { value: number }[] }): string {
  const all = props.result.series ?? []
  const top = Math.max(1, ...all.flatMap((s) => s.points.map((p) => p.value)))
  const width = 100
  const height = 100
  const step = serie.points.length > 1 ? width / (serie.points.length - 1) : 0
  return serie.points
    .map((p, i) => {
      const x = serie.points.length > 1 ? i * step : width / 2
      const y = height - (p.value / top) * (height - 10)
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
}

/** Pontos da linha, normalizados na área do SVG. */
const linePoints = computed(() => {
  if (rows.value.length === 0) return ''
  const width = 100
  const height = 100
  const step = rows.value.length > 1 ? width / (rows.value.length - 1) : 0
  return rows.value
    .map((row, i) => {
      const x = rows.value.length > 1 ? i * step : width / 2
      const y = height - (row.value / maxValue.value) * (height - 10)
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
})
</script>

<template>
  <!-- Progresso: várias séries mensais no mesmo gráfico de linhas. -->
  <div v-if="result.series?.length" class="line-chart">
    <svg viewBox="0 0 100 100" preserveAspectRatio="none" class="line-svg">
      <polyline
        v-for="(serie, i) in result.series"
        :key="serie.name"
        :points="seriesPoints(serie)"
        fill="none"
        :stroke="color(i)"
        stroke-width="1.5"
        vector-effect="non-scaling-stroke"
      />
    </svg>
    <div class="line-labels">
      <span v-for="p in result.series[0].points" :key="p.label">{{ p.label.slice(5) }}</span>
    </div>
    <ul class="legend horizontal">
      <li v-for="(serie, i) in result.series" :key="serie.name">
        <span class="dot" :style="{ background: color(i) }"></span>
        <span>{{ serie.name }}</span>
        <span class="muted">{{ serie.points.reduce((s, p) => s + p.value, 0) }}</span>
      </li>
    </ul>
  </div>

  <!-- Conversão de funil: barras com a taxa contra a etapa anterior. -->
  <div v-else-if="chart === 'conversao'" class="bars">
    <template v-for="(row, i) in rows" :key="row.label">
      <div v-if="i > 0" class="conv-arrow">
        <span class="conv-rate">{{ (row.percent ?? 0).toFixed(0) }}%</span>
      </div>
      <div class="bar-row">
        <span class="bar-label" :title="row.label">{{ row.label }}</span>
        <div class="bar-track">
          <div
            class="bar-fill"
            :style="{ width: `${(row.value / maxValue) * 100}%`, background: row.label === 'Ganho' ? 'var(--fix-green)' : color(i) }"
          ></div>
        </div>
        <span class="bar-value">{{ fmt(row.value) }}</span>
      </div>
    </template>
  </div>

  <p v-else-if="!rows.length" class="muted empty">Sem dados no período escolhido.</p>

  <!-- Barras horizontais: legível mesmo com rótulo longo (nome de pessoa, etapa). -->
  <div v-else-if="chart === 'barras'" class="bars">
    <div v-for="(row, i) in rows" :key="row.label" class="bar-row">
      <span class="bar-label" :title="row.label">{{ row.label }}</span>
      <div class="bar-track">
        <div class="bar-fill" :style="{ width: `${(row.value / maxValue) * 100}%`, background: color(i) }"></div>
      </div>
      <span class="bar-value">{{ fmt(row.value) }}</span>
    </div>
  </div>

  <div v-else-if="chart === 'linha'" class="line-chart">
    <svg viewBox="0 0 100 100" preserveAspectRatio="none" class="line-svg">
      <polyline :points="linePoints" fill="none" stroke="#9B52DF" stroke-width="1.5" vector-effect="non-scaling-stroke" />
    </svg>
    <div class="line-labels">
      <span v-for="row in rows" :key="row.label" :title="`${row.label}: ${fmt(row.value)}`">
        {{ row.label }}
      </span>
    </div>
  </div>

  <div v-else-if="chart === 'pizza'" class="pie">
    <svg viewBox="0 0 160 160" class="pie-svg">
      <path v-for="(slice, i) in slices" :key="i" :d="slice.d" :fill="slice.color" />
    </svg>
    <ul class="legend">
      <li v-for="(slice, i) in slices" :key="i">
        <span class="dot" :style="{ background: slice.color }"></span>
        <span class="legend-label" :title="slice.label">{{ slice.label }}</span>
        <span class="muted">{{ fmt(slice.value) }} · {{ slice.percent.toFixed(0) }}%</span>
      </li>
    </ul>
  </div>

  <div v-else class="table-wrap">
    <table class="data">
      <thead>
        <tr>
          <th>{{ result.dimension_label }}</th>
          <th style="width: 160px; text-align: right">{{ result.metric_label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.label">
          <td>{{ row.label }}</td>
          <td style="text-align: right">{{ fmt(row.value) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.empty {
  text-align: center;
  padding: 24px 0;
}

.bars {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.bar-row {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr) 110px;
  gap: 10px;
  align-items: center;
  font-size: 13px;
}

.bar-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--fix-text-2);
}

.bar-track {
  background: var(--fix-bg);
  border-radius: 6px;
  height: 20px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 6px;
  transition: width 0.25s;
}

.bar-value {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.line-chart {
  padding-top: 8px;
}

.line-svg {
  width: 100%;
  height: 160px;
  overflow: visible;
}

.line-labels {
  display: flex;
  justify-content: space-between;
  gap: 4px;
  margin-top: 6px;
  font-size: 11px;
  color: var(--fix-text-3);
}

.line-labels span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pie {
  display: flex;
  gap: 20px;
  align-items: center;
  flex-wrap: wrap;
}

.pie-svg {
  width: 160px;
  height: 160px;
  flex-shrink: 0;
}

.conv-arrow {
  display: flex;
  justify-content: center;
  padding: 1px 0;
}

.conv-rate {
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  font-size: 11px;
  font-weight: 600;
  border-radius: 999px;
  padding: 1px 10px;
}

.legend.horizontal {
  display: flex;
  gap: 16px;
  margin-top: 10px;
  min-width: 0;
}

.legend.horizontal li {
  padding: 0;
}

.legend {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-width: 180px;
  font-size: 13px;
}

.legend li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 3px;
  flex-shrink: 0;
}

.legend-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
