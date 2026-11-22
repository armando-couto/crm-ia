<script setup lang="ts" generic="T">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

/**
 * Campo de busca com lista suspensa e seleção múltipla em chips. É a base dos
 * seletores de CNAE, de cidade e de natureza jurídica: quem usa decide de onde
 * vêm as opções (evento `buscar`) e como cada linha aparece (slot `opcao`).
 *
 * Teclado: setas andam na lista, Enter marca ou desmarca, Esc fecha e
 * Backspace com o campo vazio tira o último chip.
 */
const props = withDefaults(
  defineProps<{
    modelValue: T[]
    opcoes: T[]
    chave: (item: T) => string
    rotulo: (item: T) => string
    /** Texto de ajuda do chip (atributo `title`). */
    detalhe?: (item: T) => string
    /** Opção que não pode ser marcada (ex.: já coberta por outra escolha). */
    bloqueada?: (item: T) => string
    carregando?: boolean
    placeholder?: string
    /** Mensagem da lista vazia (sem resultado para o texto digitado). */
    vazio?: string
    /** Mensagem antes de digitar, quando ainda não há opções. */
    dica?: string
    disabled?: boolean
    /** Nome acessível do campo. */
    nome: string
  }>(),
  { carregando: false, placeholder: 'Buscar…', vazio: 'Nada encontrado', dica: '', disabled: false }
)

const emit = defineEmits<{
  'update:modelValue': [itens: T[]]
  /** Texto digitado (já sem espaços nas pontas), a cada alteração e ao abrir. */
  buscar: [texto: string]
}>()

const texto = ref('')
const aberto = ref(false)
const ativo = ref(0)
const raiz = ref<HTMLElement | null>(null)
const campo = ref<HTMLInputElement | null>(null)
const idLista = `busca-chips-${Math.random().toString(36).slice(2, 9)}`

const marcadas = computed(() => new Set(props.modelValue.map(props.chave)))

function abrir() {
  if (props.disabled || aberto.value) return
  aberto.value = true
  ativo.value = 0
  emit('buscar', texto.value.trim())
}

function fechar() {
  aberto.value = false
}

function aoDigitar() {
  aberto.value = true
  ativo.value = 0
  emit('buscar', texto.value.trim())
}

function alternar(item: T) {
  if (props.disabled) return
  const k = props.chave(item)
  if (marcadas.value.has(k)) {
    emit('update:modelValue', props.modelValue.filter((i) => props.chave(i) !== k))
  } else {
    if (props.bloqueada?.(item)) return
    emit('update:modelValue', [...props.modelValue, item])
  }
  campo.value?.focus()
}

function remover(item: T) {
  if (props.disabled) return
  const k = props.chave(item)
  emit('update:modelValue', props.modelValue.filter((i) => props.chave(i) !== k))
}

function aoTeclar(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    if (!aberto.value) return abrir()
    ativo.value = Math.min(props.opcoes.length - 1, ativo.value + 1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    ativo.value = Math.max(0, ativo.value - 1)
  } else if (event.key === 'Enter') {
    if (!aberto.value) return
    event.preventDefault()
    const item = props.opcoes[ativo.value]
    if (item !== undefined) alternar(item)
  } else if (event.key === 'Escape') {
    if (!aberto.value) return
    // Fecha só a lista: o Esc não deve fechar o modal ou a gaveta em volta.
    event.stopPropagation()
    fechar()
  } else if (event.key === 'Backspace' && !texto.value && props.modelValue.length) {
    remover(props.modelValue[props.modelValue.length - 1])
  }
}

function cliqueFora(event: MouseEvent) {
  if (raiz.value && !raiz.value.contains(event.target as Node)) fechar()
}

watch(aberto, (v) => {
  if (v) document.addEventListener('mousedown', cliqueFora)
  else document.removeEventListener('mousedown', cliqueFora)
})
onBeforeUnmount(() => document.removeEventListener('mousedown', cliqueFora))

watch(
  () => props.opcoes,
  () => {
    if (ativo.value >= props.opcoes.length) ativo.value = Math.max(0, props.opcoes.length - 1)
  }
)

/** Limpa o texto digitado (quem usa chama depois de trocar o contexto da busca). */
function limparTexto() {
  texto.value = ''
}
defineExpose({ limparTexto, abrir, fechar })
</script>

<template>
  <div ref="raiz" class="busca-chips" :class="{ desabilitado: disabled }">
    <div class="campo-linha">
      <input
        ref="campo"
        v-model="texto"
        type="text"
        role="combobox"
        autocomplete="off"
        spellcheck="false"
        :aria-label="nome"
        :aria-expanded="aberto"
        :aria-controls="idLista"
        :placeholder="placeholder"
        :disabled="disabled"
        @focus="abrir"
        @click="abrir"
        @input="aoDigitar"
        @keydown="aoTeclar"
      />
      <span v-if="carregando" class="girando" aria-hidden="true"></span>
    </div>

    <div v-if="aberto" :id="idLista" class="lista" role="listbox" :aria-label="nome" aria-multiselectable="true">
      <template v-if="opcoes.length">
        <button
          v-for="(item, i) in opcoes"
          :key="chave(item)"
          type="button"
          role="option"
          class="opcao"
          :class="{ ativa: i === ativo, marcada: marcadas.has(chave(item)), bloqueada: !marcadas.has(chave(item)) && !!bloqueada?.(item) }"
          :aria-selected="marcadas.has(chave(item))"
          :title="(!marcadas.has(chave(item)) && bloqueada?.(item)) || undefined"
          @mouseenter="ativo = i"
          @mousedown.prevent
          @click="alternar(item)"
        >
          <span class="marca" aria-hidden="true">{{ marcadas.has(chave(item)) ? '✓' : '' }}</span>
          <span class="conteudo"><slot name="opcao" :item="item">{{ rotulo(item) }}</slot></span>
        </button>
      </template>
      <div v-else-if="carregando" class="aviso">Buscando…</div>
      <div v-else class="aviso">{{ texto.trim() ? vazio : dica || vazio }}</div>
      <div v-if="$slots.rodape" class="rodape"><slot name="rodape" /></div>
    </div>

    <div v-if="modelValue.length" class="chips">
      <span v-for="item in modelValue" :key="chave(item)" class="chip active" :title="detalhe?.(item)">
        <slot name="chip" :item="item">{{ rotulo(item) }}</slot>
        <button type="button" class="chip-x" :disabled="disabled" :aria-label="`Remover ${rotulo(item)}`" @click="remover(item)">×</button>
      </span>
    </div>
  </div>
</template>

<style scoped>
.busca-chips {
  position: relative;
}
.campo-linha {
  position: relative;
}
.campo-linha input {
  width: 100%;
  padding: 9px 34px 9px 12px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  font-size: 14px;
  font-family: inherit;
  color: var(--ci-text);
  background: var(--ci-surface);
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.campo-linha input:focus {
  border-color: var(--ci-purple);
  box-shadow: 0 0 0 3px rgba(var(--ci-purple-rgb), 0.15);
}
.campo-linha input:disabled {
  background: var(--ci-bg);
  cursor: not-allowed;
}
.girando {
  position: absolute;
  right: 11px;
  top: 50%;
  width: 14px;
  height: 14px;
  margin-top: -7px;
  border: 2px solid var(--ci-border);
  border-top-color: var(--ci-purple);
  border-radius: 50%;
  animation: busca-gira 0.7s linear infinite;
}
.lista {
  position: absolute;
  z-index: 30;
  top: 42px;
  left: 0;
  right: 0;
  max-height: 300px;
  overflow-y: auto;
  background: var(--ci-surface);
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  box-shadow: var(--shadow-lg);
  padding: 4px;
}
.opcao {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: 7px;
  background: none;
  font: inherit;
  font-size: 13.5px;
  color: var(--ci-text);
  text-align: left;
  cursor: pointer;
}
.opcao.ativa {
  background: var(--ci-purple-tint);
}
.opcao.marcada {
  color: var(--ci-purple-dark);
  font-weight: 500;
}
.opcao.bloqueada {
  color: var(--ci-text-3);
  cursor: not-allowed;
}
.marca {
  width: 14px;
  flex-shrink: 0;
  color: var(--ci-purple);
  font-weight: 700;
}
.conteudo {
  flex: 1;
  min-width: 0;
}
.aviso {
  padding: 12px;
  font-size: 13px;
  color: var(--ci-text-3);
}
.rodape {
  padding: 8px 10px 6px;
  border-top: 1px solid var(--ci-bg);
  font-size: 12px;
  color: var(--ci-text-3);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.chips .chip {
  cursor: default;
  max-width: 100%;
  white-space: normal;
}
.chips .chip-x:disabled {
  cursor: not-allowed;
}
@keyframes busca-gira {
  to {
    transform: rotate(360deg);
  }
}
</style>
