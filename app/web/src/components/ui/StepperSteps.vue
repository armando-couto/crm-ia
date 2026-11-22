<script setup lang="ts">
/**
 * Passos numerados de um assistente (enviar ao CRM: o que criar →
 * distribuição → confirmar). Só mostra onde o usuário está; o conteúdo de
 * cada passo fica na tela que usa o componente.
 */
const props = withDefaults(
  defineProps<{
    /** Rótulos dos passos, na ordem. */
    passos: string[]
    /** Índice do passo atual, começando em 0. */
    atual: number
    /** Deixa clicar nos passos já concluídos para voltar. */
    clicavel?: boolean
  }>(),
  { clicavel: true }
)

const emit = defineEmits<{ 'update:atual': [indice: number] }>()

function ir(i: number) {
  if (props.clicavel && i < props.atual) emit('update:atual', i)
}
</script>

<template>
  <ol class="stepper">
    <li
      v-for="(passo, i) in passos"
      :key="i"
      class="passo"
      :class="{ feito: i < atual, atual: i === atual }"
      :aria-current="i === atual ? 'step' : undefined"
    >
      <button type="button" class="marca" :disabled="!clicavel || i >= atual" :aria-label="`Passo ${i + 1}: ${passo}`" @click="ir(i)">
        <template v-if="i < atual">✓</template>
        <template v-else>{{ i + 1 }}</template>
      </button>
      <span class="nome">{{ passo }}</span>
    </li>
  </ol>
</template>

<style scoped>
.stepper {
  display: flex;
  list-style: none;
  margin: 0 0 18px;
  padding: 0;
  gap: 8px;
}

.passo {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--ci-text-3);
  font-size: 13px;
}

.passo:not(:last-child)::after {
  content: '';
  flex: 1;
  height: 2px;
  min-width: 12px;
  background: var(--ci-border);
}

.passo.feito:not(:last-child)::after {
  background: var(--ci-purple);
}

.marca {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 2px solid var(--ci-border);
  background: var(--ci-surface);
  color: var(--ci-text-3);
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
  padding: 0;
  cursor: default;
}

.passo.feito .marca {
  border-color: var(--ci-purple);
  background: var(--ci-purple);
  color: #fff;
}

.passo.feito .marca:not(:disabled) {
  cursor: pointer;
}

.passo.atual .marca {
  border-color: var(--ci-purple);
  color: var(--ci-purple);
}

.passo.atual .nome {
  color: var(--ci-text);
  font-weight: 600;
}

.nome {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
