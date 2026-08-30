<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, initials, lifecycleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import TimelinePanel from '../components/TimelinePanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, Deal, Paginated, Task, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const contact = ref<Contact | null>(null)
const deals = ref<Deal[]>([])
const tasks = ref<Task[]>([])
const users = ref<User[]>([])
const companies = ref<Company[]>([])

const editOpen = ref(false)
const saving = ref(false)
const form = ref<any>({})

async function load() {
  try {
    contact.value = await api.get<Contact>(`/contacts/${id}`)
    const [dealsResp, tasksResp] = await Promise.all([
      api.get<Paginated<Deal>>(`/deals?contact_id=${id}&per_page=50`),
      api.get<Paginated<Task>>(`/tasks?contact_id=${id}&per_page=50`)
    ])
    deals.value = dealsResp.data
    tasks.value = tasksResp.data
  } catch (e: any) {
    toast.error(e.message)
    router.push('/contatos')
  }
}

function openEdit() {
  if (!contact.value) return
  form.value = { ...contact.value }
  editOpen.value = true
}

async function save() {
  saving.value = true
  try {
    contact.value = await api.put<Contact>(`/contacts/${id}`, form.value)
    toast.push('Contato atualizado')
    editOpen.value = false
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!confirm('Remover este contato? As atividades e tarefas vinculadas também serão removidas.')) return
  try {
    await api.delete(`/contacts/${id}`)
    toast.push('Contato removido')
    router.push('/contatos')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function toggleTask(task: Task) {
  try {
    const updated = await api.patch<Task>(`/tasks/${task.id}/toggle`, { done: !task.completed_at })
    tasks.value = tasks.value.map((t) => (t.id === task.id ? updated : t))
  } catch (e: any) {
    toast.error(e.message)
  }
}

const fullName = computed(() => (contact.value ? `${contact.value.first_name} ${contact.value.last_name}`.trim() : ''))

onMounted(async () => {
  await load()
  try {
    users.value = await api.get<User[]>('/users')
    const resp = await api.get<Paginated<Company>>('/companies?per_page=100')
    companies.value = resp.data
  } catch {
    /* opcional */
  }
})
</script>

<template>
  <div class="page" v-if="contact">
    <div class="page-head">
      <div class="who">
        <span class="avatar-lg">{{ initials(fullName) }}</span>
        <div>
          <h1>{{ fullName }}</h1>
          <p class="muted">
            {{ contact.job_title || 'Sem cargo' }}
            <template v-if="contact.company_name">
              · <router-link :to="`/empresas/${contact.company_id}`">{{ contact.company_name }}</router-link>
            </template>
          </p>
        </div>
      </div>
      <div class="toolbar">
        <button class="btn btn-outline" @click="openEdit">Editar</button>
        <button class="btn btn-danger" @click="remove">Remover</button>
      </div>
    </div>

    <div class="layout">
      <div class="side">
        <div class="card info">
          <h2>Informações</h2>
          <dl>
            <dt>E-mail</dt>
            <dd>{{ contact.email || '—' }}</dd>
            <dt>Telefone</dt>
            <dd>{{ contact.phone || '—' }}</dd>
            <dt>Estágio</dt>
            <dd><span class="badge">{{ lifecycleLabels[contact.lifecycle_stage] || contact.lifecycle_stage }}</span></dd>
            <dt>Origem</dt>
            <dd>{{ contact.source || '—' }}</dd>
            <dt>Dono</dt>
            <dd>{{ contact.owner_name || '—' }}</dd>
            <dt>Criado em</dt>
            <dd>{{ formatDate(contact.created_at) }}</dd>
          </dl>
        </div>

        <div class="card">
          <h2>Negócios ({{ deals.length }})</h2>
          <p v-if="!deals.length" class="muted">Nenhum negócio vinculado.</p>
          <ul class="mini-list">
            <li v-for="d in deals" :key="d.id">
              <router-link :to="`/negocios/${d.id}`">{{ d.name }}</router-link>
              <span class="badge" :class="d.status === 'ganho' ? 'green' : d.status === 'perdido' ? 'red' : 'gray'">
                {{ d.stage_name }}
              </span>
            </li>
          </ul>
        </div>

        <div class="card">
          <h2>Tarefas ({{ tasks.filter((t) => !t.completed_at).length }} pendentes)</h2>
          <p v-if="!tasks.length" class="muted">Nenhuma tarefa.</p>
          <ul class="mini-list">
            <li v-for="t in tasks" :key="t.id">
              <label class="task-check">
                <input type="checkbox" :checked="!!t.completed_at" @change="toggleTask(t)" />
                <span :class="{ done: t.completed_at }">{{ t.title }}</span>
              </label>
              <span class="muted" v-if="t.due_date">{{ formatDate(t.due_date) }}</span>
            </li>
          </ul>
        </div>
      </div>

      <div class="main-col">
        <TimelinePanel :contact-id="id" :email-contact-id="contact.email ? id : undefined" />
      </div>
    </div>

    <ModalDialog title="Editar contato" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="save">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.first_name" required />
          </div>
          <div class="field">
            <label>Sobrenome</label>
            <input v-model="form.last_name" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>E-mail</label>
            <input v-model="form.email" type="email" />
          </div>
          <div class="field">
            <label>Telefone</label>
            <input v-model="form.phone" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Cargo</label>
            <input v-model="form.job_title" />
          </div>
          <div class="field">
            <label>Origem</label>
            <input v-model="form.source" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Estágio</label>
            <select v-model="form.lifecycle_stage">
              <option v-for="(label, key) in lifecycleLabels" :key="key" :value="key">{{ label }}</option>
            </select>
          </div>
          <div class="field">
            <label>Empresa</label>
            <select v-model="form.company_id">
              <option :value="null">Sem empresa</option>
              <option v-for="co in companies" :key="co.id" :value="co.id">{{ co.name }}</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Dono</label>
          <select v-model="form.owner_id">
            <option :value="null">Sem dono</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar alterações' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.who {
  display: flex;
  align-items: center;
  gap: 14px;
}

.avatar-lg {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background: var(--fix-purple);
  color: #fff;
  font-size: 18px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
}

.who p {
  margin: 2px 0 0;
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

.side {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
  word-break: break-word;
}

.mini-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
}

.mini-list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.task-check {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.task-check .done {
  text-decoration: line-through;
  color: var(--fix-text-3);
}
</style>
