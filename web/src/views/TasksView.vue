<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDateTime, taskTypeLabels } from '../format'
import { useToastStore } from '../stores/toast'
import { useAuthStore } from '../stores/auth'
import ModalDialog from '../components/ModalDialog.vue'
import type { Contact, Paginated, Task, User } from '../types'

const toast = useToastStore()
const auth = useAuthStore()

const tasks = ref<Task[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25
const statusFilter = ref('pendente')
const onlyMine = ref(true)
const loading = ref(false)

const users = ref<User[]>([])
const contacts = ref<Contact[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  title: '',
  description: '',
  type: 'tarefa',
  priority: 'media',
  due_date: '',
  owner_id: null as number | null,
  contact_id: null as number | null
})

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
    if (statusFilter.value) params.set('status', statusFilter.value)
    if (onlyMine.value && auth.user) params.set('owner_id', String(auth.user.id))
    const resp = await api.get<Paginated<Task>>(`/tasks?${params}`)
    tasks.value = resp.data
    total.value = resp.pagination.total
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function toggle(task: Task) {
  try {
    const updated = await api.patch<Task>(`/tasks/${task.id}/toggle`, { done: !task.completed_at })
    tasks.value = tasks.value.map((t) => (t.id === task.id ? updated : t))
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function remove(task: Task) {
  if (!confirm(`Remover a tarefa "${task.title}"?`)) return
  try {
    await api.delete(`/tasks/${task.id}`)
    tasks.value = tasks.value.filter((t) => t.id !== task.id)
    toast.push('Tarefa removida')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function save() {
  saving.value = true
  try {
    await api.post('/tasks', form.value)
    toast.push('Tarefa criada')
    modalOpen.value = false
    form.value = { title: '', description: '', type: 'tarefa', priority: 'media', due_date: '', owner_id: null, contact_id: null }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

function isOverdue(task: Task): boolean {
  return !task.completed_at && !!task.due_date && new Date(task.due_date).getTime() < Date.now()
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

const priorityBadge: Record<string, string> = { baixa: 'gray', media: 'blue', alta: 'red' }
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Tarefas <span class="muted" v-if="total">({{ total }})</span></h1>
      <div class="toolbar">
        <label class="check">
          <input v-model="onlyMine" type="checkbox" @change="page = 1; load()" />
          Somente minhas
        </label>
        <select v-model="statusFilter" @change="page = 1; load()">
          <option value="pendente">Pendentes</option>
          <option value="atrasada">Atrasadas</option>
          <option value="concluida">Concluídas</option>
          <option value="">Todas</option>
        </select>
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Nova tarefa</button>
      </div>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th style="width: 40px"></th>
            <th>Tarefa</th>
            <th>Tipo</th>
            <th>Prioridade</th>
            <th>Contato</th>
            <th>Vencimento</th>
            <th>Dono</th>
            <th style="width: 40px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tasks" :key="t.id" :class="{ overdue: isOverdue(t) }" style="cursor: default">
            <td>
              <input type="checkbox" :checked="!!t.completed_at" @change="toggle(t)" />
            </td>
            <td>
              <strong :class="{ done: t.completed_at }">{{ t.title }}</strong>
              <div class="muted" v-if="t.description">{{ t.description }}</div>
            </td>
            <td>{{ taskTypeLabels[t.type] || t.type }}</td>
            <td><span class="badge" :class="priorityBadge[t.priority]">{{ t.priority }}</span></td>
            <td>
              <router-link v-if="t.contact_id" :to="`/contatos/${t.contact_id}`">{{ t.contact_name }}</router-link>
              <template v-else>—</template>
            </td>
            <td :class="{ 'overdue-text': isOverdue(t) }">{{ formatDateTime(t.due_date) }}</td>
            <td>{{ t.owner_name || '—' }}</td>
            <td>
              <button class="btn btn-danger btn-sm" type="button" title="Remover" @click="remove(t)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !tasks.length" class="empty-state">
        <strong>Nenhuma tarefa aqui</strong>
        Crie tarefas para não perder nenhum follow-up.
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>

    <ModalDialog title="Nova tarefa" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Título *</label>
          <input v-model="form.title" required placeholder="ex.: Ligar para apresentar proposta" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="form.description" rows="2"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Tipo</label>
            <select v-model="form.type">
              <option v-for="(label, key) in taskTypeLabels" :key="key" :value="key">{{ label }}</option>
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
            <label>Vencimento</label>
            <input v-model="form.due_date" type="datetime-local" />
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
          {{ saving ? 'Salvando…' : 'Criar tarefa' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.check {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--fix-text-2);
  cursor: pointer;
}

.done {
  text-decoration: line-through;
  color: var(--fix-text-3);
}

tr.overdue {
  background: var(--fix-red-tint);
}

.overdue-text {
  color: var(--fix-red);
  font-weight: 500;
}
</style>
