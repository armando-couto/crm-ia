<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import type { BookingPage, TimeWindow } from '../types'

const toast = useToastStore()

const page = ref<BookingPage | null>(null)
const baseUrl = ref('')
const exists = ref(false)
const loading = ref(true)
const saving = ref(false)

const weekdays = [
  { key: '1', label: 'Segunda' },
  { key: '2', label: 'Terça' },
  { key: '3', label: 'Quarta' },
  { key: '4', label: 'Quinta' },
  { key: '5', label: 'Sexta' },
  { key: '6', label: 'Sábado' },
  { key: '0', label: 'Domingo' }
]

const publicUrl = computed(() => (page.value?.slug ? `${baseUrl.value}/agendar/${page.value.slug}` : ''))

function windowsOf(day: string): TimeWindow[] {
  if (!page.value) return []
  return page.value.weekly_hours[day] ?? []
}

function toggleDay(day: string) {
  if (!page.value) return
  if (windowsOf(day).length) {
    delete page.value.weekly_hours[day]
  } else {
    page.value.weekly_hours[day] = [{ start: '09:00', end: '18:00' }]
  }
}

function addWindow(day: string) {
  if (!page.value) return
  const list = page.value.weekly_hours[day] ?? []
  list.push({ start: '09:00', end: '12:00' })
  page.value.weekly_hours[day] = list
}

function removeWindow(day: string, index: number) {
  const list = page.value?.weekly_hours[day]
  if (!list) return
  list.splice(index, 1)
  if (!list.length) delete page.value!.weekly_hours[day]
}

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{ page: BookingPage; exists: boolean; base_url: string }>('/booking/me')
    page.value = resp.page
    exists.value = resp.exists
    baseUrl.value = resp.base_url
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!page.value) return
  saving.value = true
  try {
    page.value = await api.put<BookingPage>('/booking/me', page.value)
    exists.value = true
    toast.push('Página de agendamento salva')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

function copy() {
  navigator.clipboard?.writeText(publicUrl.value)
  toast.push('Link copiado')
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Agendamento</h1>
        <p class="muted" style="margin: 4px 0 0">
          Publique sua agenda e deixe o cliente escolher o horário. A reunião entra direto no CRM.
        </p>
      </div>
      <button class="btn btn-primary" type="button" :disabled="saving || loading" @click="save">
        {{ saving ? 'Salvando…' : 'Salvar' }}
      </button>
    </div>

    <p v-if="loading" class="muted">Carregando…</p>

    <template v-else-if="page">
      <div class="card link-card">
        <div class="field" style="margin: 0">
          <label>Seu link público</label>
          <div class="copy-row">
            <input :value="publicUrl" readonly class="mono" />
            <button class="btn btn-sm" type="button" :disabled="!exists" @click="copy">Copiar</button>
          </div>
          <p class="muted hint">
            {{ exists ? 'Compartilhe na assinatura de e-mail, no WhatsApp ou no site.' : 'Salve para publicar o link.' }}
          </p>
        </div>
      </div>

      <div class="card">
        <h2>Como aparece para o cliente</h2>
        <div class="form-row">
          <div class="field">
            <label>Título</label>
            <input v-model="page.title" placeholder="Agende uma conversa" />
          </div>
          <div class="field">
            <label>Endereço (slug)</label>
            <input v-model="page.slug" class="mono" />
          </div>
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="page.description" rows="2" placeholder="Conte o que a pessoa pode esperar da conversa"></textarea>
        </div>
        <div class="field">
          <label>Local</label>
          <input v-model="page.location" placeholder="Google Meet, telefone, escritório…" />
        </div>
      </div>

      <div class="card">
        <h2>Regras da agenda</h2>
        <div class="form-row">
          <div class="field">
            <label>Duração (min)</label>
            <input v-model.number="page.duration_min" type="number" min="5" max="480" />
          </div>
          <div class="field">
            <label>Intervalo entre reuniões (min)</label>
            <input v-model.number="page.buffer_min" type="number" min="0" max="240" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Abrir agenda por (dias)</label>
            <input v-model.number="page.days_ahead" type="number" min="1" max="90" />
          </div>
          <div class="field">
            <label>Aviso mínimo (horas)</label>
            <input v-model.number="page.notice_hours" type="number" min="0" max="168" />
          </div>
        </div>
        <div class="field">
          <label>Situação</label>
          <select v-model="page.active">
            <option :value="true">Publicada</option>
            <option :value="false">Pausada</option>
          </select>
        </div>
      </div>

      <div class="card">
        <h2>Seus horários</h2>
        <p class="muted hint">
          Só aparecem para o cliente os horários livres: o que já está na sua agenda do CRM é descontado.
        </p>

        <div v-for="day in weekdays" :key="day.key" class="day-row">
          <label class="day-toggle">
            <input type="checkbox" :checked="windowsOf(day.key).length > 0" @change="toggleDay(day.key)" />
            <strong>{{ day.label }}</strong>
          </label>

          <div v-if="windowsOf(day.key).length" class="windows">
            <div v-for="(w, i) in windowsOf(day.key)" :key="i" class="window">
              <input v-model="w.start" type="time" />
              <span class="muted">até</span>
              <input v-model="w.end" type="time" />
              <button class="icon-btn danger" type="button" title="Remover" @click="removeWindow(day.key, i)">✕</button>
            </div>
            <button class="btn btn-sm" type="button" @click="addWindow(day.key)">+ Faixa</button>
          </div>
          <span v-else class="muted closed">fechado</span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.card {
  margin-bottom: 16px;
}

.card h2 {
  font-size: 15px;
  margin-bottom: 14px;
}

.link-card .copy-row {
  display: flex;
  gap: 8px;
}

.copy-row input {
  flex: 1;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
}

.hint {
  font-size: 12px;
  margin: 6px 0 0;
}

.day-row {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  padding: 10px 0;
  border-top: 1px solid var(--ci-border);
}

.day-row:first-of-type {
  border-top: none;
}

.day-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 130px;
  flex-shrink: 0;
  font-size: 14px;
  cursor: pointer;
}

.windows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.window {
  display: flex;
  align-items: center;
  gap: 8px;
}

.window input {
  width: 120px;
}

.closed {
  font-size: 13px;
  padding-top: 3px;
}

.icon-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--ci-text-3);
  padding: 4px 6px;
  border-radius: 6px;
}

.icon-btn.danger:hover {
  color: var(--ci-red);
}
</style>
