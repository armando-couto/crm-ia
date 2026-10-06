<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, formatDateTime } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Paginated, Project, Task, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const project = ref<Project | null>(null)
const tasks = ref<Task[]>([])
const users = ref<User[]>([])

const editOpen = ref(false)
const taskModalOpen = ref(false)
const saving = ref(false)
const form = ref<any>({})
const taskForm = ref({ title: '', description: '', priority: 'media', due_date: '', owner_id: null as number | null })

async function load() {
  try {
    project.value = await api.get<Project>(`/projects/${id}`)
    const resp = await api.get<Paginated<Task>>(`/tasks?project_id=${id}&per_page=100`)
    tasks.value = resp.data
  } catch (e: any) {
    toast.error(e.message)
    router.push('/projetos')
  }
}

function openEdit() {
  if (!project.value) return
  form.value = { ...project.value, due_date: project.value.due_date ? project.value.due_date.slice(0, 10) : '' }
  editOpen.value = true
}

async function save() {
  saving.value = true
  try {
    project.value = await api.put<Project>(`/projects/${id}`, form.value)
    toast.push('Projeto atualizado')
    editOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function saveTask() {
  saving.value = true
  try {
    await api.post('/tasks', { ...taskForm.value, project_id: id })
    toast.push('Tarefa adicionada ao projeto')
    taskModalOpen.value = false
    taskForm.value = { title: '', description: '', priority: 'media', due_date: '', owner_id: null }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function toggleTask(task: Task) {
  try {
    const updated = await api.patch<Task>(`/tasks/${task.id}/toggle`, { done: !task.completed_at })
    tasks.value = tasks.value.map((t) => (t.id === task.id ? updated : t))
    if (project.value) {
      project.value.tasks_done += updated.completed_at ? 1 : -1
    }
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removeProject() {
  if (!confirm('Remover este projeto e todas as suas tarefas?')) return
  try {
    await api.delete(`/projects/${id}`)
    toast.push('Projeto removido')
    router.push('/projetos')
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(async () => {
  await load()
  try {
    users.value = await api.get<User[]>('/users')
  } catch {
    /* opcional */
  }
})

const statusLabels: Record<string, string> = { ativo: 'Ativo', concluido: 'Concluído', arquivado: 'Arquivado' }
</script>

<template>
  <div class="page" v-if="project">
    <div class="page-head">
      <div>
        <h1>{{ project.name }}</h1>
        <p class="muted head-sub">
          <span class="badge" :class="project.status === 'ativo' ? 'blue' : project.status === 'concluido' ? 'green' : 'gray'">
            {{ statusLabels[project.status] }}
          </span>
          <span>{{ project.tasks_done }}/{{ project.tasks_total }} tarefas concluídas</span>
          <span v-if="project.due_date">· prazo {{ formatDate(project.due_date) }}</span>
        </p>
      </div>
      <div class="toolbar">
        <button class="btn btn-primary" @click="taskModalOpen = true">+ Tarefa</button>
        <button class="btn btn-outline" @click="openEdit">Editar</button>
        <button class="btn btn-danger" @click="removeProject">Remover</button>
      </div>
    </div>

    <p class="muted" v-if="project.description">{{ project.description }}</p>

    <div class="table-wrap" style="margin-top: 14px">
      <table class="data">
        <thead>
          <tr>
            <th style="width: 40px"></th>
            <th>Tarefa</th>
            <th>Prioridade</th>
            <th>Vencimento</th>
            <th>Dono</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tasks" :key="t.id" style="cursor: default">
            <td><input type="checkbox" :checked="!!t.completed_at" @change="toggleTask(t)" /></td>
            <td>
              <strong :class="{ done: t.completed_at }">{{ t.title }}</strong>
              <div class="muted" v-if="t.description">{{ t.description }}</div>
            </td>
            <td><span class="badge" :class="t.priority === 'alta' ? 'red' : t.priority === 'baixa' ? 'gray' : 'blue'">{{ t.priority }}</span></td>
            <td class="muted">{{ formatDateTime(t.due_date) }}</td>
            <td>{{ t.owner_name || '—' }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!tasks.length" class="empty-state">
        <strong>Sem tarefas ainda</strong>
        Adicione as tarefas do projeto pelo botão acima.
      </div>
    </div>

    <ModalDialog title="Editar projeto" :open="editOpen" @close="editOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="form.description" rows="2"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Status</label>
            <select v-model="form.status">
              <option value="ativo">Ativo</option>
              <option value="concluido">Concluído</option>
              <option value="arquivado">Arquivado</option>
            </select>
          </div>
          <div class="field">
            <label>Prazo</label>
            <input v-model="form.due_date" type="date" />
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

    <ModalDialog title="Nova tarefa do projeto" :open="taskModalOpen" @close="taskModalOpen = false">
      <form @submit.prevent="saveTask">
        <div class="field">
          <label>Título *</label>
          <input v-model="taskForm.title" required />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="taskForm.description" rows="2"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Prioridade</label>
            <select v-model="taskForm.priority">
              <option value="baixa">Baixa</option>
              <option value="media">Média</option>
              <option value="alta">Alta</option>
            </select>
          </div>
          <div class="field">
            <label>Vencimento</label>
            <input v-model="taskForm.due_date" type="datetime-local" />
          </div>
        </div>
        <div class="field">
          <label>Responsável</label>
          <select v-model="taskForm.owner_id">
            <option :value="null">Eu</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Adicionar tarefa' }}
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
  align-items: center;
  flex-wrap: wrap;
}

.done {
  text-decoration: line-through;
  color: var(--fix-text-3);
}
</style>
