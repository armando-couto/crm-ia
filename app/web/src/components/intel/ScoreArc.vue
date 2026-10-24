<script setup lang="ts">
// Score de crédito (0–1000) em arco, no estilo dos painéis de bureau.
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    score: number | null | undefined
    faixa?: string
    nivel?: string
    max?: number
  }>(),
  { max: 1000 }
)

const RAIO = 60
const COMPRIMENTO = Math.PI * RAIO

const valor = computed(() => Math.max(0, Math.min(props.max, Number(props.score ?? 0))))
const fracao = computed(() => (props.max ? valor.value / props.max : 0))
const traco = computed(() => `${(fracao.value * COMPRIMENTO).toFixed(1)} ${COMPRIMENTO.toFixed(1)}`)

/** Cor pelo nível informado; sem nível, derivada do próprio score. */
const classe = computed(() => {
  const n = (props.nivel ?? '').toLowerCase()
  if (n === 'baixo' || n === 'alto' || n === 'medio') return n
  if (fracao.value >= 0.7) return 'baixo'
  if (fracao.value >= 0.4) return 'medio'
  return 'alto'
})

const rotuloNivel: Record<string, string> = { baixo: 'risco baixo', medio: 'risco médio', alto: 'risco alto' }
</script>

<template>
  <figure class="arc" :class="classe" :aria-label="`Score ${valor} de ${max}`">
    <svg viewBox="0 0 140 84" role="img">
      <path class="track" d="M 10 70 A 60 60 0 0 1 130 70" />
      <path class="fill" d="M 10 70 A 60 60 0 0 1 130 70" :stroke-dasharray="traco" />
      <text x="70" y="62" text-anchor="middle" class="score">{{ score === null || score === undefined ? '—' : valor }}</text>
      <text x="70" y="78" text-anchor="middle" class="max">de {{ max }}</text>
    </svg>
    <figcaption>
      <strong v-if="faixa" class="faixa">Faixa {{ faixa }}</strong>
      <span class="nivel">{{ rotuloNivel[classe] }}</span>
    </figcaption>
  </figure>
</template>

<style scoped>
.arc {
  margin: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  width: 170px;
}

svg {
  width: 100%;
  height: auto;
  overflow: visible;
}

.track,
.fill {
  fill: none;
  stroke-width: 12;
  stroke-linecap: round;
}

.track {
  stroke: var(--ci-bg);
}

.fill {
  stroke: var(--ci-green);
  transition: stroke-dasharray 0.5s ease-out;
}

.arc.medio .fill {
  stroke: var(--ci-amber);
}

.arc.alto .fill {
  stroke: var(--ci-red);
}

.score {
  font-size: 26px;
  font-weight: 700;
  fill: var(--ci-text);
}

.max {
  font-size: 10px;
  fill: var(--ci-text-3);
}

figcaption {
  display: flex;
  flex-direction: column;
  align-items: center;
  font-size: 12px;
  color: var(--ci-text-2);
}

.faixa {
  font-size: 14px;
  color: var(--ci-text);
}

.arc.baixo .nivel {
  color: var(--ci-green);
}

.arc.medio .nivel {
  color: var(--ci-amber);
}

.arc.alto .nivel {
  color: var(--ci-red);
}
</style>
