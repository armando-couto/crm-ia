<script setup lang="ts">
import { computed, ref } from 'vue'
import type { NotaMotivo } from '../../types-prospeccao'

/**
 * Nota do item (0 a 100) com os motivos ao passar o mouse ou focar:
 * "+35 celular próprio", "−40 sem telefone"…
 */
const props = defineProps<{ nota: number; motivos?: NotaMotivo[] | null }>()

const aberto = ref(false)
const tom = computed(() => (props.nota >= 70 ? 'alta' : props.nota >= 40 ? 'media' : 'baixa'))
const lista = computed(() => props.motivos ?? [])
const titulo = computed(() => lista.value.map((m) => `${m.pontos > 0 ? '+' : ''}${m.pontos} ${m.texto}`).join('\n'))
</script>

<template>
  <span
    class="nota"
    :class="tom"
    tabindex="0"
    :aria-label="`Nota ${nota} de 100`"
    :title="titulo || undefined"
    @mouseenter="aberto = true"
    @mouseleave="aberto = false"
    @focus="aberto = true"
    @blur="aberto = false"
  >
    {{ nota }}
    <span v-if="aberto && lista.length" class="motivos" role="tooltip">
      <span class="motivos-titulo">Por que {{ nota }}?</span>
      <span v-for="(m, i) in lista" :key="i" class="motivo">
        <span class="pontos" :class="{ neg: m.pontos < 0 }">{{ m.pontos > 0 ? '+' : '' }}{{ m.pontos }}</span>
        {{ m.texto }}
      </span>
    </span>
  </span>
</template>

<style scoped>
.nota {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  padding: 2px 8px;
  border-radius: 999px;
  font-weight: 700;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  cursor: help;
  outline: none;
}
.nota:focus-visible {
  box-shadow: 0 0 0 3px rgba(var(--ci-purple-rgb), 0.25);
}
.nota.alta {
  background: var(--ci-green-tint);
  color: var(--ci-green);
}
.nota.media {
  background: var(--ci-amber-tint, #fff4dc);
  color: var(--ci-amber, #a86b00);
}
.nota.baixa {
  background: var(--ci-red-tint);
  color: var(--ci-red);
}
.motivos {
  position: absolute;
  z-index: 40;
  left: 0;
  top: calc(100% + 6px);
  min-width: 230px;
  padding: 10px 12px;
  background: var(--ci-surface);
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-weight: 400;
  font-size: 12.5px;
  color: var(--ci-text);
  text-align: left;
  white-space: normal;
}
.motivos-titulo {
  font-weight: 600;
  margin-bottom: 2px;
}
.motivo {
  display: flex;
  gap: 8px;
}
.pontos {
  min-width: 32px;
  font-weight: 700;
  color: var(--ci-green);
  font-variant-numeric: tabular-nums;
}
.pontos.neg {
  color: var(--ci-red);
}
</style>
