<script setup lang="ts">
// Semáforo da próxima atividade do negócio: vermelho = atrasada, verde = hoje,
// cinza = futura, amarelo = nenhuma atividade agendada.
import { computed } from 'vue'
import { formatDate, taskTypeLabels, urgenciaDe, urgenciaTons } from '../../format'

const props = defineProps<{
  /** `proxima_atividade_em` de GET /deals/board; nulo = nenhuma agendada. */
  em: string | null | undefined
  /** `proxima_atividade_tipo` (tipo da tarefa). */
  tipo?: string
  /** Só a bolinha, sem o texto (linha de tabela). */
  compacto?: boolean
}>()

const urgencia = computed(() => urgenciaDe(props.em))
const tom = computed(() => urgenciaTons[urgencia.value])

const nomeTipo = computed(() => (props.tipo ? taskTypeLabels[props.tipo] ?? props.tipo : 'Atividade'))

const texto = computed(() => {
  switch (urgencia.value) {
    case 'nenhuma':
      return 'Sem próxima atividade'
    case 'atrasado':
      return `Atrasada: ${nomeTipo.value} · ${formatDate(props.em)}`
    case 'hoje':
      return `Hoje: ${nomeTipo.value}`
    default:
      return `${nomeTipo.value} · ${formatDate(props.em)}`
  }
})
</script>

<template>
  <span class="proxima" :class="urgencia" :data-urgencia="urgencia" :title="compacto ? texto : undefined">
    <span class="semaforo" :class="tom" aria-hidden="true"></span>
    <span v-if="!compacto" class="texto">{{ texto }}</span>
    <span v-else class="sr-only">{{ texto }}</span>
  </span>
</template>

<style scoped>
.proxima {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--ci-text-2);
}

.proxima.nenhuma .texto {
  color: var(--ci-amber);
  font-weight: 500;
}

.proxima.atrasado .texto {
  color: var(--ci-red);
  font-weight: 500;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
