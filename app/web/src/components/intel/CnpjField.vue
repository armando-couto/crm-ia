<script setup lang="ts">
// Campo de CNPJ (ou CPF) com máscara, validação dos dígitos verificadores e
// consulta à Receita quando o CNPJ fica completo — o pai faz o GET /cnpj e
// informa `preenchido` / `existente` para os avisos abaixo do campo.
import { computed, ref, watch } from 'vue'
import { cnpjValido, cpfValido, formatarCNPJ, formatarDocumento, somenteDigitos } from '../../cnpj'

const props = withDefaults(
  defineProps<{
    modelValue: string | null | undefined
    /** Aceita CPF (11 dígitos) além do CNPJ — rótulo "CNPJ/CPF". */
    aceitaCpf?: boolean
    /** O pai preencheu o formulário com dados da Receita. */
    preenchido?: boolean
    /** Já existe empresa com este CNPJ. */
    existente?: { id: number; name: string } | null
    consultando?: boolean
    disabled?: boolean
    placeholder?: string
  }>(),
  { aceitaCpf: true, preenchido: false, existente: null, consultando: false, disabled: false, placeholder: '00.000.000/0000-00' }
)

const emit = defineEmits<{ 'update:modelValue': [valor: string]; lookup: [cnpj: string] }>()

const tocado = ref(false)
const ultimoConsultado = ref('')

const digitos = computed(() => somenteDigitos(props.modelValue))
const exibido = computed(() => (props.aceitaCpf ? formatarDocumento(props.modelValue) : formatarCNPJ(props.modelValue)))
const cnpjOk = computed(() => digitos.value.length === 14 && cnpjValido(digitos.value))

const erro = computed(() => {
  const d = digitos.value
  if (!d) return ''
  if (d.length === 14) return cnpjValido(d) ? '' : 'CNPJ inválido — confira os dígitos'
  if (d.length === 11 && props.aceitaCpf) return cpfValido(d) ? '' : 'CPF inválido — confira os dígitos'
  if (tocado.value) return props.aceitaCpf ? 'Documento incompleto' : 'CNPJ incompleto'
  return ''
})

/** Recebe os dígitos locais: no input o v-model do pai ainda não voltou para a prop. */
function consultarSePossivel(d: string) {
  if (d.length !== 14 || !cnpjValido(d) || d === ultimoConsultado.value) return
  ultimoConsultado.value = d
  emit('lookup', d)
}

function onInput(event: Event) {
  const el = event.target as HTMLInputElement
  const d = somenteDigitos(el.value).slice(0, 14)
  const formatado = props.aceitaCpf ? formatarDocumento(d) : formatarCNPJ(d)
  // O v-bind não repinta quando o valor formatado não muda (ex.: letra digitada), então ajusta na mão.
  el.value = formatado
  emit('update:modelValue', formatado)
  consultarSePossivel(d)
}

function onBlur() {
  tocado.value = true
  consultarSePossivel(digitos.value)
}

// Se o pai limpar o campo (novo formulário), libera uma nova consulta.
watch(digitos, (d) => {
  if (!d) ultimoConsultado.value = ''
})
</script>

<template>
  <div class="cnpj-field">
    <div class="input-wrap">
      <input
        :value="exibido"
        type="text"
        inputmode="numeric"
        autocomplete="off"
        maxlength="18"
        :placeholder="placeholder"
        :disabled="disabled"
        :aria-invalid="!!erro"
        @input="onInput"
        @blur="onBlur"
      />
      <span v-if="consultando" class="status" aria-live="polite">Consultando Receita…</span>
      <span v-else-if="cnpjOk" class="status ok" aria-hidden="true">✓</span>
    </div>
    <p v-if="erro" class="error-text">{{ erro }}</p>
    <p v-if="preenchido" class="fill-chip">
      <span class="star">✦</span>
      Preenchido pela Receita Federal — revise antes de salvar
    </p>
    <p v-if="existente" class="exists">
      Já existe a empresa
      <router-link :to="`/empresas/${existente.id}`">{{ existente.name }}</router-link>
      com este CNPJ.
    </p>
  </div>
</template>

<style scoped>
.cnpj-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.input-wrap {
  position: relative;
  display: flex;
}

.input-wrap input {
  width: 100%;
  padding: 9px 12px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  font-size: 14px;
  font-family: inherit;
  color: var(--ci-text);
  background: var(--ci-surface);
  outline: none;
  transition: border-color 0.15s, box-shadow 0.15s;
  letter-spacing: 0.02em;
}

.input-wrap input:focus {
  border-color: var(--ci-purple);
  box-shadow: 0 0 0 3px rgba(var(--ci-purple-rgb), 0.15);
}

.input-wrap input[aria-invalid='true'] {
  border-color: var(--ci-red);
}

.status {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 11px;
  color: var(--ci-text-3);
  pointer-events: none;
}

.status.ok {
  color: var(--ci-green);
  font-size: 13px;
  font-weight: 700;
}

.error-text {
  margin: 0;
}

.fill-chip {
  margin: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  align-self: flex-start;
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  font-size: 12px;
  font-weight: 500;
}

.star {
  font-size: 11px;
}

.exists {
  margin: 0;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
  font-size: 12px;
}

.exists a {
  font-weight: 600;
  color: var(--ci-amber);
  text-decoration: underline;
}
</style>
