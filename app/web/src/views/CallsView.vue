<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDateTime } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Call, Contact, Paginated } from '../types'

const toast = useToastStore()

const calls = ref<Call[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25
const outcomeFilter = ref('')
const loading = ref(false)
const contacts = ref<Contact[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  contact_id: null as number | null,
  direction: 'saida',
  outcome: 'conectada',
  duration_seconds: 0,
  called_at: '',
  notes: ''
})

const outcomeLabels: Record<string, string> = {
  conectada: 'Conectada',
  sem_resposta: 'Sem resposta',
  caixa_postal: 'Caixa postal',
  ocupado: 'Ocupado',
  numero_errado: 'Número errado'
}
const outcomeBadge: Record<string, string> = {
  conectada: 'green',
  sem_resposta: 'amber',
  caixa_postal: 'gray',
  ocupado: 'amber',
  numero_errado: 'red'
}

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
    if (outcomeFilter.value) params.set('outcome', outcomeFilter.value)
    const resp = await api.get<Paginated<Call>>(`/calls?${params}`)
    calls.value = resp.data
    total.value = resp.pagination.total
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await api.post('/calls', { ...form.value, duration_seconds: Number(form.value.duration_seconds) || 0 })
    toast.push('Chamada registrada')
    modalOpen.value = false
    form.value = { contact_id: null, direction: 'saida', outcome: 'conectada', duration_seconds: 0, called_at: '', notes: '' }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

function formatDuration(seconds: number): string {
  if (!seconds) return '—'
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return m ? `${m}min ${s}s` : `${s}s`
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
      <h1>Chamadas <span class="muted" v-if="total">({{ total }})</span></h1>
      <div class="toolbar">
        <select v-model="outcomeFilter" @change="page = 1; load()">
          <option value="">Todos os resultados</option>
          <option v-for="(label, key) in outcomeLabels" :key="key" :value="key">{{ label }}</option>
        </select>
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Registrar chamada</button>
      </div>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Contato</th>
            <th>Direção</th>
            <th>Resultado</th>
            <th>Duração</th>
            <th>Quando</th>
            <th>Por</th>
            <th>Notas</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in calls" :key="c.id" style="cursor: default">
            <td>
              <router-link v-if="c.contact_id" :to="`/contatos/${c.contact_id}`"><strong>{{ c.contact_name }}</strong></router-link>
              <template v-else>—</template>
            </td>
            <td>{{ c.direction === 'entrada' ? '↓ Entrada' : '↑ Saída' }}</td>
            <td><span class="badge" :class="outcomeBadge[c.outcome]">{{ outcomeLabels[c.outcome] || c.outcome }}</span></td>
            <td>{{ formatDuration(c.duration_seconds) }}</td>
            <td class="muted">{{ formatDateTime(c.called_at) }}</td>
            <td>{{ c.user_name || '—' }}</td>
            <td class="muted notes-cell">{{ c.notes || '—' }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !calls.length" class="empty-state">
        <strong>Nenhuma chamada registrada</strong>
        Registre as ligações para manter o histórico do relacionamento.
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>

    <ModalDialog title="Registrar chamada" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Contato *</label>
          <select v-model="form.contact_id" required>
            <option :value="null" disabled>Escolha o contato…</option>
            <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
          </select>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Direção</label>
            <select v-model="form.direction">
              <option value="saida">Saída (liguei)</option>
              <option value="entrada">Entrada (recebi)</option>
            </select>
          </div>
          <div class="field">
            <label>Resultado</label>
            <select v-model="form.outcome">
              <option v-for="(label, key) in outcomeLabels" :key="key" :value="key">{{ label }}</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Duração (segundos)</label>
            <input v-model.number="form.duration_seconds" type="number" min="0" />
          </div>
          <div class="field">
            <label>Quando</label>
            <input v-model="form.called_at" type="datetime-local" />
          </div>
        </div>
        <div class="field">
          <label>Notas</label>
          <textarea v-model="form.notes" rows="3" placeholder="O que foi conversado…"></textarea>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving || !form.contact_id" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Registrar chamada' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.notes-cell {
  max-width: 260px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
