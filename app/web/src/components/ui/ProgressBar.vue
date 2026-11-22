<script setup lang="ts">
import { computed } from 'vue'

/**
 * Barra de progresso: job em andamento (feitos de total), medidor de cota
 * (usado de limite) e meta do dia na fila (feitas de meta).
 */
const props = withDefaults(
  defineProps<{
    /** Quanto já foi feito. */
    valor: number
    /** Total esperado; 0 ou ausente mostra a barra indeterminada (se `indeterminado`) ou vazia. */
    total?: number
    /** Texto à esquerda, acima da barra. */
    rotulo?: string
    /** Mostra "N de M (P%)" à direita. */
    mostrarNumeros?: boolean
    /** Cor: padrão (cor do cliente), green, amber, red ou blue. */
    tom?: '' | 'green' | 'amber' | 'red' | 'blue'
    /** Anima sem percentual enquanto o total não é conhecido (job `pendente`). */
    indeterminado?: boolean
    fina?: boolean
  }>(),
  { total: 0, rotulo: '', mostrarNumeros: true, tom: '', indeterminado: false, fina: false }
)

const percentual = computed(() => {
  if (!props.total || props.total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((props.valor / props.total) * 100)))
})

const semTotal = computed(() => props.indeterminado && (!props.total || props.total <= 0))
const inteiro = new Intl.NumberFormat('pt-BR')
</script>

<template>
  <div class="progress" :class="[tom, { fina }]">
    <div v-if="rotulo || mostrarNumeros || $slots.default" class="linha">
      <span class="rotulo"><slot>{{ rotulo }}</slot></span>
      <span v-if="mostrarNumeros && !semTotal" class="numeros">
        {{ inteiro.format(valor) }} de {{ inteiro.format(total) }} ({{ percentual }}%)
      </span>
    </div>
    <div
      class="trilho"
      role="progressbar"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="semTotal ? undefined : percentual"
      :aria-label="rotulo || 'Progresso'"
    >
      <div class="barra" :class="{ indeterminada: semTotal }" :style="semTotal ? undefined : { width: `${percentual}%` }"></div>
    </div>
  </div>
</template>

<style scoped>
.progress {
  --cor: var(--ci-purple);
  width: 100%;
}

.progress.green {
  --cor: var(--ci-green);
}

.progress.amber {
  --cor: var(--ci-amber);
}

.progress.red {
  --cor: var(--ci-red);
}

.progress.blue {
  --cor: var(--ci-blue);
}

.linha {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  color: var(--ci-text-2);
  margin-bottom: 6px;
}

.numeros {
  color: var(--ci-text-3);
  white-space: nowrap;
}

.trilho {
  height: 8px;
  border-radius: 999px;
  background: var(--ci-bg);
  border: 1px solid var(--ci-border);
  overflow: hidden;
}

.fina .trilho {
  height: 5px;
}

.barra {
  height: 100%;
  background: var(--cor);
  border-radius: 999px;
  transition: width 0.3s ease;
}

.barra.indeterminada {
  width: 35%;
  animation: progress-vai 1.2s ease-in-out infinite;
}

@keyframes progress-vai {
  from {
    transform: translateX(-100%);
  }
  to {
    transform: translateX(300%);
  }
}
</style>
