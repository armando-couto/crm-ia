<script setup lang="ts">
import { computed } from 'vue'
import type { TooltipLinha } from './chart-utils'

const props = defineProps<{
  visivel: boolean
  x: number
  y: number
  titulo?: string
  linhas?: TooltipLinha[]
}>()

// Posição fixa junto ao cursor, sem sair da janela.
const estilo = computed(() => {
  const largura = 240
  const altura = 40 + (props.linhas?.length ?? 0) * 20
  const vw = typeof window !== 'undefined' ? window.innerWidth : 1200
  const vh = typeof window !== 'undefined' ? window.innerHeight : 800
  let left = props.x + 14
  let top = props.y + 14
  if (left + largura > vw) left = Math.max(8, props.x - largura - 14)
  if (top + altura > vh) top = Math.max(8, props.y - altura - 14)
  return { left: `${left}px`, top: `${top}px` }
})
</script>

<template>
  <Teleport to="body">
    <div v-if="visivel" class="chart-tooltip" role="tooltip" :style="estilo">
      <strong v-if="titulo" class="ct-title">{{ titulo }}</strong>
      <div v-for="(l, i) in linhas ?? []" :key="i" class="ct-line" :class="l.tom">
        <span class="ct-label">{{ l.rotulo }}</span>
        <span class="ct-value">{{ l.valor }}</span>
      </div>
      <slot />
    </div>
  </Teleport>
</template>

<style scoped>
.chart-tooltip {
  position: fixed;
  z-index: 300;
  pointer-events: none;
  min-width: 160px;
  max-width: 260px;
  background: var(--ci-purple-deep, #1e1b4b);
  color: #fff;
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.4;
  box-shadow: var(--shadow-lg, 0 8px 32px rgba(34, 20, 60, 0.16));
}

.ct-title {
  display: block;
  font-size: 13px;
  margin-bottom: 4px;
}

.ct-line {
  display: flex;
  justify-content: space-between;
  gap: 14px;
}

.ct-label {
  color: rgba(255, 255, 255, 0.72);
}

.ct-value {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

.ct-line.ok .ct-value {
  color: #7fe0b5;
}

.ct-line.danger .ct-value {
  color: #ff9aa2;
}

.ct-line.warn .ct-value {
  color: #ffd27a;
}

.ct-line.muted .ct-value {
  color: rgba(255, 255, 255, 0.6);
  font-weight: 400;
}
</style>
