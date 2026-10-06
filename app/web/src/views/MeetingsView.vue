<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDateTime } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Contact, Meeting, Paginated } from '../types'

const toast = useToastStore()

const meetings = ref<Meeting[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25
const periodFilter = ref('proximas')
const loading = ref(false)
const contacts = ref<Contact[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<Meeting | null>(null)
const form = ref({
  title: '',
  starts_at: '',
  ends_at: '',
  location: '',
  notes: '',
  status: 'agendada',
  contact_id: null as number | null
})

const statusLabels: Record<string, string> = {
  agendada: 'Agendada',
  realizada: 'Realizada',
  cancelada: 'Cancelada',
  nao_compareceu: 'Não compareceu'
}
const statusBadge: Record<string, string> = {
  agendada: 'blue',
  realizada: 'green',
  cancelada: 'gray',
  nao_compareceu: 'red'
}

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
    if (periodFilter.value) params.set('period', periodFilter.value)
    const resp = await api.get<Paginated<Meeting>>(`/meetings?${params}`)
    meetings.value = resp.data
    total.value = resp.pagination.total
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function openNew() {
  editing.value = null
  form.value = { title: '', starts_at: '', ends_at: '', location: '', notes: '', status: 'agendada', contact_id: null }
  modalOpen.value = true
}

function openEdit(meeting: Meeting) {
  editing.value = meeting
  form.value = {
    title: meeting.title,
    starts_at: meeting.starts_at.slice(0, 16),
    ends_at: meeting.ends_at ? meeting.ends_at.slice(0, 16) : '',
    location: meeting.location,
    notes: meeting.notes,
    status: meeting.status,
    contact_id: meeting.contact_id
  }
  modalOpen.value = true
}

async function save() {
  saving.value = true
  try {
    if (editing.value) {
      await api.put(`/meetings/${editing.value.id}`, form.value)
      toast.push('Reunião atualizada')
    } else {
      await api.post('/meetings', form.value)
      toast.push('Reunião agendada')
    }
    modalOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(meeting: Meeting) {
  if (!confirm(`Remover a reunião "${meeting.title}"?`)) return
  try {
    await api.delete(`/meetings/${meeting.id}`)
    toast.push('Reunião removida')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

function changePage(delta: number) {
  page.value += delta
  load()
}

onMounted(async () => {
  await load()
  try {
    const resp = await api.get<Paginated<Contact>>('/contacts?per_page=100')
    contacts.value = resp.data
  } catch {
    /* opcional */
  }
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Reuniões <span class="muted" v-if="total">({{ total }})</span></h1>
      <div class="toolbar">
        <select v-model="periodFilter" @change="page = 1; load()">
          <option value="proximas">Próximas</option>
          <option value="passadas">Passadas</option>
          <option value="">Todas</option>
        </select>
        <button class="btn btn-primary" type="button" @click="openNew">+ Agendar reunião</button>
      </div>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Reunião</th>
            <th>Quando</th>
            <th>Status</th>
            <th>Contato</th>
            <th>Local/Link</th>
            <th>Organizador</th>
            <th style="width: 140px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in meetings" :key="m.id" style="cursor: default">
            <td><strong>{{ m.title }}</strong></td>
            <td class="muted">{{ formatDateTime(m.starts_at) }}</td>
            <td><span class="badge" :class="statusBadge[m.status]">{{ statusLabels[m.status] }}</span></td>
            <td>
              <router-link v-if="m.contact_id" :to="`/contatos/${m.contact_id}`">{{ m.contact_name }}</router-link>
              <template v-else>—</template>
            </td>
            <td class="muted location-cell">{{ m.location || '—' }}</td>
            <td>{{ m.user_name || '—' }}</td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="openEdit(m)">Editar</button>
              <button class="btn btn-danger btn-sm" type="button" @click="remove(m)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !meetings.length" class="empty-state">
        <strong>Nenhuma reunião aqui</strong>
        Agende reuniões com contatos e acompanhe o resultado.
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>

    <ModalDialog :title="editing ? 'Editar reunião' : 'Agendar reunião'" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Título *</label>
          <input v-model="form.title" required placeholder="ex.: Apresentação da proposta" />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Início *</label>
            <input v-model="form.starts_at" type="datetime-local" required />
          </div>
          <div class="field">
            <label>Término</label>
            <input v-model="form.ends_at" type="datetime-local" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Contato</label>
            <select v-model="form.contact_id">
              <option :value="null">Sem contato</option>
              <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Status</label>
            <select v-model="form.status">
              <option v-for="(label, key) in statusLabels" :key="key" :value="key">{{ label }}</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Local ou link da videochamada</label>
          <input v-model="form.location" placeholder="ex.: https://meet.google.com/…" />
        </div>
        <div class="field">
          <label>Notas</label>
          <textarea v-model="form.notes" rows="2"></textarea>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : editing ? 'Salvar alterações' : 'Agendar' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.location-cell {
  max-width: 200px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
