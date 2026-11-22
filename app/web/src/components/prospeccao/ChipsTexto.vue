<script setup lang="ts">
import { ref } from 'vue'

/**
 * Lista de textos livres em chips (bairros, CEPs, etiquetas): Enter, vírgula
 * ou ponto e vírgula acrescentam; colar vários de uma vez também funciona.
 */
const props = withDefaults(
  defineProps<{
    modelValue: string[]
    nome: string
    placeholder?: string
    disabled?: boolean
    /** Devolve o valor pronto para guardar, ou `null` para recusar. */
    normalizar?: (bruto: string) => string | null
    /** Mensagem quando `normalizar` recusa. */
    invalido?: string
    maximo?: number
  }>(),
  { placeholder: '', disabled: false, normalizar: (v: string) => v.trim() || null, invalido: 'Valor inválido', maximo: 50 }
)
const emit = defineEmits<{ 'update:modelValue': [itens: string[]] }>()

const texto = ref('')
const erro = ref('')

function acrescentar(bruto: string) {
  const pedacos = bruto.split(/[,;\n]+/).map((p) => p.trim()).filter(Boolean)
  if (!pedacos.length) return
  const lista = [...props.modelValue]
  erro.value = ''
  for (const p of pedacos) {
    const v = props.normalizar(p)
    if (v === null) {
      erro.value = `${props.invalido}: "${p}"`
      continue
    }
    if (lista.includes(v)) continue
    if (lista.length >= props.maximo) {
      erro.value = `No máximo ${props.maximo}`
      break
    }
    lista.push(v)
  }
  emit('update:modelValue', lista)
  texto.value = ''
}

function aoTeclar(event: KeyboardEvent) {
  if (event.key === 'Enter' || event.key === ',' || event.key === ';') {
    if (!texto.value.trim()) {
      if (event.key !== 'Enter') event.preventDefault()
      return
    }
    event.preventDefault()
    acrescentar(texto.value)
  } else if (event.key === 'Backspace' && !texto.value && props.modelValue.length) {
    emit('update:modelValue', props.modelValue.slice(0, -1))
  }
}

function aoColar(event: ClipboardEvent) {
  const colado = event.clipboardData?.getData('text') ?? ''
  if (!/[,;\n]/.test(colado)) return
  event.preventDefault()
  acrescentar(colado)
}

function remover(v: string) {
  emit('update:modelValue', props.modelValue.filter((i) => i !== v))
}
</script>

<template>
  <div class="chips-texto">
    <input
      v-model="texto"
      type="text"
      autocomplete="off"
      :aria-label="nome"
      :placeholder="placeholder"
      :disabled="disabled"
      @keydown="aoTeclar"
      @paste="aoColar"
      @blur="acrescentar(texto)"
    />
    <div v-if="erro" class="error-text">{{ erro }}</div>
    <div v-if="modelValue.length" class="chips">
      <span v-for="v in modelValue" :key="v" class="chip active">
        {{ v }}
        <button type="button" class="chip-x" :disabled="disabled" :aria-label="`Remover ${v}`" @click="remover(v)">×</button>
      </span>
    </div>
  </div>
</template>

<style scoped>
.chips-texto input {
  width: 100%;
  padding: 9px 12px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  font-size: 14px;
  font-family: inherit;
  outline: none;
}
.chips-texto input:focus {
  border-color: var(--ci-purple);
  box-shadow: 0 0 0 3px rgba(var(--ci-purple-rgb), 0.15);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.chips .chip {
  cursor: default;
}
</style>
