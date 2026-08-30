<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import TimelinePanel from '../components/TimelinePanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Contact, Paginated, Ticket, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const ticket = ref<Ticket | null>(null)
const users = ref<User[]>([])
const contacts = ref<Contact[]>([])

const editOpen = ref(false)
const saving = ref(false)
const form = ref<any>({})

const statusLabels: Record<string, string> = {
  aberto: 'Aberto',
  pendente: 'Pendente',
  resolvido: 'Resolvido',
  fechado: 'Fechado'
}
const statusBadge: Record<string, string> = { aberto: 'blue', pendente: 'amber', resolvido: 'green', fechado: 'gray' }

async function load() {
  try {
    ticket.value = await api.get<Ticket>(`/tickets/${id}`)
  } catch (e: any) {
    toast.error(e.message)
    router.push('/tickets')
  }
}

async function setStatus(status: string) {
  if (!ticket.value) return
  try {
    ticket.value = await api.put<Ticket>(`/tickets/${id}`, { ...ticket.value, status })
    toast.push(`Ticket ${statusLabels[status].toLowerCase()}`)
  } catch (e: any) {
    toast.error(e.message)
  }
}

function openEdit() {
  if (!ticket.value) return
  form.value = { ...ticket.value }
  editOpen.value = true
}

async function save() {
  saving.value = true
  try {
    ticket.value = await api.put<Ticket>(`/tickets/${id}`, form.value)
    toast.push('Ticket atualizado')
    editOpen.value = false
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!confirm('Remover este ticket?')) return
  try {
    await api.delete(`/tickets/${id}`)
    toast.push('Ticket removido')
    router.push('/tickets')
  } catch (e: any) {
    toast.error(e.message)
  }
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
</script>

<template>
  <div class="page" v-if="ticket">
    <div class="page-head">
      <div>
        <h1>#{{ ticket.id }} · {{ ticket.subject }}</h1>
        <p class="muted head-sub">
          <span class="badge" :class="statusBadge[ticket.status]">{{ statusLabels[ticket.status] }}</span>
          <span class="badge" :class="ticket.priority === 'alta' ? 'red' : ticket.priority === 'baixa' ? 'gray' : 'blue'">
            prioridade {{ ticket.priority }}
          </span>
        </p>
      </div>
      <div class="toolbar">
        <template v-if="ticket.status === 'aberto' || ticket.status === 'pendente'">
          <button class="btn btn-outline" style="color: var(--fix-green)" @click="setStatus('resolvido')">✓ Resolver</button>
        </template>
        <button v-else class="btn btn-outline" @click="setStatus('aberto')">Reabrir</button>
        <button class="btn btn-outline" @click="openEdit">Editar</button>
        <button class="btn btn-danger" @click="remove">Remover</button>
      </div>
    </div>

    <div class="layout">
      <div class="side">
        <div class="card">
          <h2>Detalhes</h2>
          <dl>
            <dt>Contato</dt>
            <dd>
              <router-link v-if="ticket.contact_id" :to="`/contatos/${ticket.contact_id}`">{{ ticket.contact_name }}</router-link>
              <template v-else>—</template>
            </dd>
            <dt>Empresa</dt>
            <dd>
              <router-link v-if="ticket.company_id" :to="`/empresas/${ticket.company_id}`">{{ ticket.company_name }}</router-link>
              <template v-else>—</template>
            </dd>
            <dt>Dono</dt>
            <dd>{{ ticket.owner_name || '—' }}</dd>
            <dt>Aberto em</dt>
            <dd>{{ formatDate(ticket.created_at) }}</dd>
            <dt>Fechado em</dt>
            <dd>{{ formatDate(ticket.closed_at) }}</dd>
          </dl>
          <p class="description" v-if="ticket.description">{{ ticket.description }}</p>
        </div>
      </div>

      <div class="main-col">
        <TimelinePanel :ticket-id="id" :contact-id="ticket.contact_id || undefined" :email-contact-id="ticket.contact_id || undefined" />
      </div>
    </div>

    <ModalDialog title="Editar ticket" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Assunto *</label>
          <input v-model="form.subject" required />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="form.description" rows="3"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Status</label>
            <select v-model="form.status">
              <option v-for="(label, key) in statusLabels" :key="key" :value="key">{{ label }}</option>
            </select>
          </div>
          <div class="field">
            <label>Prioridade</label>
            <select v-model="form.priority">
              <option value="baixa">Baixa</option>
              <option value="media">Média</option>
              <option value="alta">Alta</option>
            </select>
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
            <label>Dono</label>
            <select v-model="form.owner_id">
              <option :value="null">Sem dono</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar alterações' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.head-sub {
  display: flex;
  gap: 8px;
  margin: 6px 0 0;
}

.layout {
  display: grid;
  grid-template-columns: 340px 1fr;
  gap: 16px;
  align-items: start;
}

@media (max-width: 980px) {
  .layout {
    grid-template-columns: 1fr;
  }
}

.card h2 {
  font-size: 14px;
  margin-bottom: 12px;
}

dl {
  margin: 0;
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px 14px;
  font-size: 13px;
}

dt {
  color: var(--fix-text-3);
}

dd {
  margin: 0;
}

.description {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--fix-bg);
  font-size: 13px;
  color: var(--fix-text-2);
  white-space: pre-wrap;
}
</style>
