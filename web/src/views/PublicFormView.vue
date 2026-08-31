<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

interface PublicFormField {
  key: string
  label: string
  type: string
  required: boolean
  options?: string[]
}

interface PublicFormDef {
  slug: string
  name: string
  headline: string
  description: string
  fields: PublicFormField[]
  submit_label: string
}

const route = useRoute()

const form = ref<PublicFormDef | null>(null)
const values = ref<Record<string, string>>({})
const honeypot = ref('')
const loading = ref(true)
const sending = ref(false)
const error = ref('')
const success = ref('')

/**
 * Esta tela é pública e roda dentro de um iframe no site do cliente, então não
 * usa o cliente de API autenticado: fala direto com /api/public.
 */
async function load() {
  try {
    const resp = await fetch(`/api/public/forms/${route.params.slug}`)
    if (!resp.ok) throw new Error('formulário não encontrado')
    form.value = await resp.json()
    for (const field of form.value!.fields) values.value[field.key] = ''
  } catch {
    error.value = 'Formulário não encontrado ou desativado.'
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (!form.value) return
  error.value = ''
  sending.value = true
  try {
    const resp = await fetch(`/api/public/forms/${form.value.slug}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...values.value, _gotcha: honeypot.value })
    })
    const data = await resp.json()
    if (!resp.ok) throw new Error(data?.error || 'não foi possível enviar')

    if (data.redirect_url) {
      window.top!.location.href = data.redirect_url
      return
    }
    success.value = data.message
  } catch (e: any) {
    error.value = e.message
  } finally {
    sending.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="public-form">
    <p v-if="loading" class="muted">Carregando…</p>

    <div v-else-if="!form" class="panel">
      <p class="error-text">{{ error }}</p>
    </div>

    <div v-else class="panel">
      <template v-if="success">
        <div class="done">
          <span class="check">✓</span>
          <p>{{ success }}</p>
        </div>
      </template>

      <template v-else>
        <h1 v-if="form.headline">{{ form.headline }}</h1>
        <p v-if="form.description" class="description">{{ form.description }}</p>

        <form @submit.prevent="submit">
          <div v-for="field in form.fields" :key="field.key" class="field">
            <label :for="field.key">
              {{ field.label }}<span v-if="field.required" class="req">*</span>
            </label>

            <textarea
              v-if="field.type === 'textarea'"
              :id="field.key"
              v-model="values[field.key]"
              rows="4"
              :required="field.required"
            ></textarea>

            <select
              v-else-if="field.type === 'selecao'"
              :id="field.key"
              v-model="values[field.key]"
              :required="field.required"
            >
              <option value="">Selecione…</option>
              <option v-for="opt in field.options" :key="opt" :value="opt">{{ opt }}</option>
            </select>

            <input
              v-else
              :id="field.key"
              v-model="values[field.key]"
              :type="field.type === 'email' ? 'email' : field.type === 'telefone' ? 'tel' : 'text'"
              :required="field.required"
            />
          </div>

          <!-- Campo isca: pessoa nenhuma vê, robô preenche e o envio é descartado. -->
          <input
            v-model="honeypot"
            class="gotcha"
            type="text"
            tabindex="-1"
            autocomplete="off"
            aria-hidden="true"
          />

          <p v-if="error" class="error-text">{{ error }}</p>

          <button class="submit" type="submit" :disabled="sending">
            {{ sending ? 'Enviando…' : form.submit_label }}
          </button>
        </form>
      </template>
    </div>
  </div>
</template>

<style scoped>
.public-form {
  min-height: 100vh;
  padding: 24px 16px;
  background: transparent;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

.panel {
  max-width: 460px;
  margin: 0 auto;
  background: #fff;
  border-radius: 14px;
  padding: 26px;
  box-shadow: 0 1px 3px rgba(34, 20, 60, 0.12);
}

h1 {
  font-size: 20px;
  margin: 0 0 8px;
  color: #2e1a47;
}

.description {
  margin: 0 0 18px;
  font-size: 14px;
  color: #6b6b6b;
  line-height: 1.5;
}

.field {
  margin-bottom: 14px;
  display: flex;
  flex-direction: column;
  gap: 5px;
}

label {
  font-size: 13px;
  font-weight: 500;
  color: #464646;
}

.req {
  color: #9b52df;
  margin-left: 2px;
}

input,
select,
textarea {
  border: 1px solid #e3e0e8;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 14px;
  font-family: inherit;
  outline: none;
  width: 100%;
  box-sizing: border-box;
}

input:focus,
select:focus,
textarea:focus {
  border-color: #9b52df;
  box-shadow: 0 0 0 3px rgba(155, 82, 223, 0.15);
}

.gotcha {
  position: absolute;
  left: -9999px;
  width: 1px;
  height: 1px;
  opacity: 0;
}

.submit {
  width: 100%;
  background: #9b52df;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: 12px;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  margin-top: 6px;
}

.submit:disabled {
  opacity: 0.6;
  cursor: default;
}

.error-text {
  color: #d64545;
  font-size: 13px;
  margin: 8px 0;
}

.muted {
  text-align: center;
  color: #8e8e8e;
  font-size: 14px;
}

.done {
  text-align: center;
  padding: 20px 0;
}

.check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: #e6f5ec;
  color: #2e9e5b;
  font-size: 24px;
  margin-bottom: 12px;
}

.done p {
  margin: 0;
  font-size: 15px;
  color: #464646;
  line-height: 1.5;
}
</style>
