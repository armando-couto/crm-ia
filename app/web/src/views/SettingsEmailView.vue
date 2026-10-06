<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { formatDateTime } from '../format'
import type { EmailPreset, EmailProvider, EmailSettings } from '../types'

const toast = useToastStore()
const presets = ref<Record<string, EmailPreset>>({})
const current = ref<EmailSettings | null>(null)
const active = ref(false)
const platformDefault = ref(false)
const inboundWebhook = ref('')
const form = ref({ provider: 'mandrill_api' as EmailProvider, from_email: '', from_name: '', reply_to: '', smtp_host: '', smtp_port: 587, smtp_user: '', secret: '' })
const saving = ref(false)
const testing = ref(false)
const testTo = ref('')

const smtpFixo = computed(() => !!presets.value[form.value.provider]?.host)
const dica = computed(() => presets.value[form.value.provider]?.dica ?? '')
const precisaSegredo = computed(() => form.value.provider !== 'plataforma' && !(current.value?.has_secret && current.value.provider === form.value.provider))

async function load() {
  try {
    const r = await api.get<{ settings: EmailSettings | null; presets: Record<string, EmailPreset>; active: boolean; platform_default: boolean; inbound_webhook: string }>('/settings/email')
    presets.value = r.presets
    current.value = r.settings
    active.value = r.active
    platformDefault.value = r.platform_default
    inboundWebhook.value = r.inbound_webhook
    if (r.settings) form.value = { ...form.value, ...r.settings, smtp_port: r.settings.smtp_port || 587, secret: '' }
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function save() {
  saving.value = true
  try {
    const r = await api.put<{ settings: EmailSettings; description: string }>('/settings/email', form.value)
    current.value = r.settings
    form.value.secret = ''
    active.value = true
    toast.push(`Provedor ativo: ${r.description}`)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function test() {
  testing.value = true
  try {
    const r = await api.post<{ to: string }>('/settings/email/test', { to: testTo.value })
    toast.push(`E-mail de teste enviado para ${r.to}`)
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Envio de e-mails</h1>
        <p class="muted" style="margin: 4px 0 0">Escolha por onde o CRM envia: Mandrill (API ou SMTP), Maileroo (SMTP) ou outro servidor SMTP.</p>
      </div>
      <span class="badge" :class="active ? 'ok' : 'off'">{{ active ? 'envio ativo' : 'envio desativado' }}</span>
    </div>

    <form class="card" style="max-width: 720px" @submit.prevent="save">
      <div class="providers">
        <label v-for="(p, codigo) in presets" :key="codigo" class="provider" :class="{ selected: form.provider === codigo }">
          <input v-model="form.provider" type="radio" :value="codigo" />
          <strong>{{ p.nome }}<small v-if="codigo === 'mandrill_api'"> · API</small><small v-else-if="p.host"> · SMTP</small></strong>
          <small>{{ p.host || (codigo === 'mandrill_api' ? 'envio pela API HTTPS' : 'servidor próprio') }}</small>
        </label>
        <label v-if="platformDefault" class="provider" :class="{ selected: form.provider === 'plataforma' }">
          <input v-model="form.provider" type="radio" value="plataforma" />
          <strong>Padrão</strong><small>remetente da plataforma</small>
        </label>
      </div>
      <p v-if="dica" class="hint">{{ dica }}</p>

      <template v-if="form.provider !== 'plataforma'">
        <div class="field"><label>E-mail do remetente *</label><input v-model="form.from_email" type="email" required /></div>
        <div class="field"><label>Nome do remetente</label><input v-model="form.from_name" /></div>
        <div class="field"><label>Responder para</label><input v-model="form.reply_to" type="email" /></div>
        <div class="field">
          <label>{{ form.provider === 'mandrill_api' ? 'Chave de API' : 'Senha SMTP' }}</label>
          <input v-model="form.secret" type="password" autocomplete="off" :required="precisaSegredo" :placeholder="current?.has_secret ? '•••••• (mantida se vazio)' : ''" />
        </div>
        <template v-if="form.provider !== 'mandrill_api'">
          <div class="field"><label>Servidor SMTP</label><input v-model="form.smtp_host" :disabled="smtpFixo" :placeholder="presets[form.provider]?.host" /></div>
          <div class="field"><label>Porta</label><input v-model.number="form.smtp_port" type="number" :disabled="smtpFixo" /></div>
          <div class="field"><label>Usuário SMTP</label><input v-model="form.smtp_user" /></div>
        </template>
      </template>

      <div style="display: flex; gap: 10px; flex-wrap: wrap">
        <button class="btn btn-primary" type="submit" :disabled="saving">{{ saving ? 'Salvando…' : 'Salvar e ativar' }}</button>
      </div>
      <p v-if="current?.tested_at" class="muted" style="margin-top: 10px">Último teste em {{ formatDateTime(current.tested_at) }}: {{ current.test_result }}</p>
    </form>

    <div class="card" style="max-width: 720px; margin-top: 16px">
      <h2 style="margin-bottom: 8px">Testar o envio</h2>
      <div style="display: flex; gap: 10px; flex-wrap: wrap; align-items: flex-end">
        <div class="field" style="flex: 1; margin: 0"><label>Enviar para (vazio = você)</label><input v-model="testTo" type="email" /></div>
        <button class="btn btn-outline" type="button" :disabled="testing || !active" @click="test">{{ testing ? 'Enviando…' : 'Enviar teste' }}</button>
      </div>
    </div>

    <div class="card" style="max-width: 720px; margin-top: 16px">
      <h2 style="margin-bottom: 8px">Caixa de entrada (respostas)</h2>
      <p class="muted">Para as respostas dos contatos caírem na Caixa de entrada do CRM, cadastre no Mandrill (Inbound) a rota que aponta para:</p>
      <code style="display: block; margin-top: 8px; word-break: break-all">{{ inboundWebhook }}</code>
    </div>
  </div>
</template>

<style scoped>
.providers { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 10px; margin-bottom: 12px; }
.provider { border: 2px solid var(--ci-border); border-radius: 10px; padding: 12px; cursor: pointer; display: flex; flex-direction: column; gap: 2px; }
.provider.selected { border-color: var(--ci-purple); background: var(--ci-purple-tint); }
.provider input { display: none; }
.provider small { color: var(--ci-text-3); font-size: 12px; }
.hint { font-size: 13px; color: var(--ci-text-2); background: var(--ci-blue-tint); padding: 10px 12px; border-radius: 8px; margin: 0 0 12px; }
.badge { font-size: 12px; padding: 4px 10px; border-radius: 999px; font-weight: 600; }
.badge.ok { background: var(--ci-green-tint); color: var(--ci-green); }
.badge.off { background: var(--ci-red-tint); color: var(--ci-red); }
</style>
