<script setup lang="ts">
import { computed } from 'vue'

/**
 * Paginação numerada das listas da prospecção. Recebe a página atual, o
 * tamanho da página e o total (os três campos de `pagination` da API).
 */
const props = withDefaults(
  defineProps<{
    page: number
    perPage: number
    total: number
    /** Opções do seletor "por página"; vazio esconde o seletor. */
    perPageOptions?: number[]
    disabled?: boolean
    /** Substantivo do resumo: "1–25 de 130 leads". */
    rotulo?: string
  }>(),
  { perPageOptions: () => [], disabled: false, rotulo: '' }
)

const emit = defineEmits<{ 'update:page': [page: number]; 'update:perPage': [perPage: number] }>()

const pageCount = computed(() => Math.max(1, Math.ceil(props.total / Math.max(1, props.perPage))))
const de = computed(() => (props.total === 0 ? 0 : (props.page - 1) * props.perPage + 1))
const ate = computed(() => Math.min(props.total, props.page * props.perPage))

/** Primeira, última e a vizinhança da atual, com reticências nos saltos. */
const numeros = computed<(number | '…')[]>(() => {
  const count = pageCount.value
  const atual = Math.min(Math.max(1, props.page), count)
  const mostrar = new Set<number>([1, count, atual - 1, atual, atual + 1])
  if (atual <= 3) [2, 3, 4].forEach((n) => mostrar.add(n))
  if (atual >= count - 2) [count - 1, count - 2, count - 3].forEach((n) => mostrar.add(n))
  const ordenadas = [...mostrar].filter((n) => n >= 1 && n <= count).sort((a, b) => a - b)
  const saida: (number | '…')[] = []
  let anterior = 0
  for (const n of ordenadas) {
    if (n - anterior > 1) saida.push('…')
    saida.push(n)
    anterior = n
  }
  return saida
})

const inteiro = new Intl.NumberFormat('pt-BR')

function ir(n: number) {
  if (props.disabled) return
  const destino = Math.min(Math.max(1, n), pageCount.value)
  if (destino !== props.page) emit('update:page', destino)
}

function trocarTamanho(event: Event) {
  const n = Number((event.target as HTMLSelectElement).value)
  if (n > 0 && n !== props.perPage) emit('update:perPage', n)
}
</script>

<template>
  <nav class="pagination-bar" aria-label="Paginação">
    <span class="resumo">
      {{ inteiro.format(de) }}–{{ inteiro.format(ate) }} de {{ inteiro.format(total) }}{{ rotulo ? ` ${rotulo}` : '' }}
    </span>

    <label v-if="perPageOptions.length" class="por-pagina">
      por página
      <select :value="perPage" :disabled="disabled" @change="trocarTamanho">
        <option v-for="n in perPageOptions" :key="n" :value="n">{{ n }}</option>
      </select>
    </label>

    <div class="paginas">
      <button type="button" class="pg anterior" :disabled="disabled || page <= 1" aria-label="Página anterior" @click="ir(page - 1)">‹</button>
      <template v-for="(n, i) in numeros" :key="`${n}-${i}`">
        <span v-if="n === '…'" class="salto">…</span>
        <button
          v-else
          type="button"
          class="pg"
          :class="{ atual: n === page }"
          :aria-current="n === page ? 'page' : undefined"
          :disabled="disabled"
          @click="ir(n)"
        >
          {{ n }}
        </button>
      </template>
      <button type="button" class="pg proxima" :disabled="disabled || page >= pageCount" aria-label="Próxima página" @click="ir(page + 1)">›</button>
    </div>
  </nav>
</template>

<style scoped>
.pagination-bar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 14px;
  padding: 12px 16px;
  font-size: 13px;
  color: var(--ci-text-3);
}

.resumo {
  margin-right: auto;
}

.por-pagina {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.por-pagina select {
  padding: 4px 6px;
  border: 1px solid var(--ci-border);
  border-radius: 6px;
  background: var(--ci-surface);
  font-size: 13px;
}

.paginas {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.pg {
  min-width: 30px;
  height: 30px;
  padding: 0 8px;
  border-radius: 7px;
  border: 1px solid var(--ci-border);
  background: var(--ci-surface);
  color: var(--ci-text-2);
  font-size: 13px;
  cursor: pointer;
}

.pg:hover:not(:disabled) {
  border-color: var(--ci-purple);
  color: var(--ci-purple);
}

.pg.atual {
  background: var(--ci-purple);
  border-color: var(--ci-purple);
  color: #fff;
  font-weight: 600;
}

.pg:disabled {
  opacity: 0.5;
  cursor: default;
}

.salto {
  padding: 0 4px;
}
</style>
