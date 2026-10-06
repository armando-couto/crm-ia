<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Contact, Paginated, Ticket, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const tickets = ref<Ticket[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25
const search = ref('')
const statusFilter = ref('aberto')
const loading = ref(false)

const users = ref<User[]>([])
const contacts = ref<Contact[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  subject: '',
  description: '',
  priority: 'media',
  contact_id: null as number | null,
  owner_id: null as number | null
})

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
    if (search.value.trim()) params.set('q', search.value.trim())
    if (statusFilter.value) params.set('status', statusFilter.value)
    const resp = await api.get<Paginated<Ticket>>(`/tickets?${params}`)
    tickets.value = resp.data
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
    await api.post('/tickets', form.value)
    toast.push('Ticket criado')
    modalOpen.value = false
    form.value = { subject: '', description: '', priority: 'media', contact_id: null, owner_id: null }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

function changePage(delta: number) {
  page.value += delta
  load()
}

onMounted(async () => {
  await load()
  try {
    users.value = await api.get<User[]>('/users')
    const resp = await api.get<Paginated<Contact>>('/contacts?per_page=100')
    contacts.value = resp.data
  } catch {
    /* opcional */
  }
})

const statusBadge: Record<string, string> = { aberto: 'blue', pendente: 'amber', resolvido: 'green', fechado: 'gray' }
const priorityBadge: Record<string, string> = { baixa: 'gray', media: 'blue', alta: 'red' }
const ticketStatusLabels: Record<string, string> = {
  aberto: 'Aberto',
  pendente: 'Pendente',
  resolvido: 'Resolvido',
  fechado: 'Fechado'
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Tickets <span class="muted" v-if="total">({{ total }})</span></h1>
      <div class="toolbar">
        <input v-model="search" type="search" placeholder="Buscar por assunto…" />
        <select v-model="statusFilter" @change="page = 1; load()">
          <option value="">Todos</option>
          <option value="aberto">Abertos</option>
          <option value="pendente">Pendentes</option>
          <option value="resolvido">Resolvidos</option>
          <option value="fechado">Fechados</option>
        </select>
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Novo ticket</button>
      </div>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Assunto</th>
            <th>Status</th>
            <th>Prioridade</th>
            <th>Contato</th>
            <th>Dono</th>
            <th>Atualizado</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tickets" :key="t.id" @click="router.push(`/tickets/${t.id}`)">
            <td><strong>#{{ t.id }}</strong> {{ t.subject }}</td>
            <td><span class="badge" :class="statusBadge[t.status]">{{ ticketStatusLabels[t.status] }}</span></td>
            <td><span class="badge" :class="priorityBadge[t.priority]">{{ t.priority }}</span></td>
            <td>{{ t.contact_name || '—' }}</td>
            <td>{{ t.owner_name || '—' }}</td>
            <td class="muted">{{ formatDate(t.updated_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !tickets.length" class="empty-state">
        <strong>Nenhum ticket aqui</strong>
        Registre solicitações e problemas dos clientes como tickets.
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>

    <ModalDialog title="Novo ticket" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Assunto *</label>
          <input v-model="form.subject" required placeholder="ex.: Maquininha não imprime comprovante" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="form.description" rows="3"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Prioridade</label>
            <select v-model="form.priority">
              <option value="baixa">Baixa</option>
              <option value="media">Média</option>
              <option value="alta">Alta</option>
            </select>
          </div>
          <div class="field">
            <label>Contato</label>
            <select v-model="form.contact_id">
              <option :value="null">Sem contato</option>
              <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Dono</label>
          <select v-model="form.owner_id">
            <option :value="null">Eu</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar ticket' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>
