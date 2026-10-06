<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import type { SendingSettings, SendingSummary } from '../types'

const toast = useToastStore()
const form = ref<SendingSettings>({ daily_limit: 500, window_start: 8, window_end: 19, weekdays_only: true, paused: false, signature: '', track_opens: true, track_clicks: true, timezone: 'America/Sao_Paulo' })
const summary = ref<SendingSummary | null>(null)
const saving = ref(false)

async function load() {
  try {
    const r = await api.get<{ settings: SendingSettings; summary: SendingSummary }>('/settings/sending')
    form.value = r.settings
    summary.value = r.summary
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function save() {
  saving.value = true
  try {
    form.value = await api.put<SendingSettings>('/settings/sending', form.value)
    toast.push('Regras de disparo salvas')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Disparo de e-mails</h1>
        <p class="muted" style="margin: 4px 0 0">Janela de horário, dias e teto diário das sequências e automações.</p>
      </div>
    </div>

    <div v-if="summary" class="stats" style="margin-bottom: 16px">
      <div class="stat"><span class="stat-label">Enviados hoje</span><strong class="stat-value">{{ summary.enviados_hoje }}<small v-if="summary.teto"> / {{ summary.teto }}</small></strong></div>
      <div class="stat" :class="summary.dentro_da_janela ? 'green' : 'red'"><span class="stat-label">Janela agora</span><strong class="stat-value">{{ summary.dentro_da_janela ? 'aberta' : 'fechada' }}</strong></div>
      <div class="stat"><span class="stat-label">Provedor</span><strong class="stat-value" style="font-size: 16px">{{ summary.provedor }}</strong></div>
    </div>

    <form class="card" style="max-width: 640px" @submit.prevent="save">
      <label class="check" style="margin-bottom: 12px"><input v-model="form.paused" type="checkbox" /> <strong>Pausar todos os disparos automáticos</strong></label>
      <div class="field"><label>Teto diário de e-mails (0 = sem limite)</label><input v-model.number="form.daily_limit" type="number" min="0" /></div>
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px">
        <div class="field"><label>Início da janela (hora)</label><input v-model.number="form.window_start" type="number" min="0" max="23" /></div>
        <div class="field"><label>Fim da janela (hora)</label><input v-model.number="form.window_end" type="number" min="0" max="23" /></div>
      </div>
      <div class="field"><label>Fuso horário</label><input v-model="form.timezone" /></div>
      <label class="check"><input v-model="form.weekdays_only" type="checkbox" /> Só em dias úteis</label>
      <label class="check"><input v-model="form.track_opens" type="checkbox" /> Rastrear aberturas</label>
      <label class="check"><input v-model="form.track_clicks" type="checkbox" /> Rastrear cliques</label>
      <div class="field" style="margin-top: 10px"><label>Assinatura padrão</label><textarea v-model="form.signature" rows="3"></textarea></div>
      <button class="btn btn-primary" type="submit" :disabled="saving">{{ saving ? 'Salvando…' : 'Salvar' }}</button>
    </form>
  </div>
</template>

<style scoped>
.check { display: flex; gap: 8px; align-items: center; margin: 6px 0; font-size: 14px; }
.stat { background: var(--ci-surface); border: 1px solid var(--ci-border); border-radius: var(--radius); padding: 14px 16px; display: flex; flex-direction: column; gap: 4px; }
.stat.green .stat-value { color: var(--ci-green); }
.stat.red .stat-value { color: var(--ci-red); }
.stat-label { font-size: 12px; color: var(--ci-text-3); text-transform: uppercase; letter-spacing: 0.05em; }
.stat-value { font-size: 22px; }
</style>
