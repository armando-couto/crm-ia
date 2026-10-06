<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Project, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const projects = ref<Project[]>([])
const statusFilter = ref('ativo')
const loading = ref(false)
const users = ref<User[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({ name: '', description: '', due_date: '', owner_id: null as number | null })

async function load() {
  loading.value = true
  try {
    const query = statusFilter.value ? `?status=${statusFilter.value}` : ''
    projects.value = await api.get<Project[]>(`/projects${query}`)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await api.post('/projects', form.value)
    toast.push('Projeto criado')
    modalOpen.value = false
    form.value = { name: '', description: '', due_date: '', owner_id: null }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

function progress(p: Project): number {
  if (!p.tasks_total) return 0
  return Math.round((p.tasks_done / p.tasks_total) * 100)
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
const statusBadge: Record<string, string> = { ativo: 'blue', concluido: 'green', arquivado: 'gray' }
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Projetos</h1>
      <div class="toolbar">
        <select v-model="statusFilter" @change="load">
          <option value="ativo">Ativos</option>
          <option value="concluido">Concluídos</option>
          <option value="arquivado">Arquivados</option>
          <option value="">Todos</option>
        </select>
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Novo projeto</button>
      </div>
    </div>

    <div class="grid">
      <div v-for="p in projects" :key="p.id" class="card project-card" @click="router.push(`/projetos/${p.id}`)">
        <div class="project-head">
          <strong>{{ p.name }}</strong>
          <span class="badge" :class="statusBadge[p.status]">{{ statusLabels[p.status] }}</span>
        </div>
        <p class="muted" v-if="p.description">{{ p.description }}</p>
        <div class="progress-track">
          <div class="progress-bar" :style="{ width: `${progress(p)}%` }"></div>
        </div>
        <div class="project-foot muted">
          <span>{{ p.tasks_done }}/{{ p.tasks_total }} tarefas</span>
          <span v-if="p.due_date">prazo {{ formatDate(p.due_date) }}</span>
          <span v-if="p.owner_name">{{ p.owner_name }}</span>
        </div>
      </div>
    </div>

    <div v-if="!loading && !projects.length" class="empty-state">
      <strong>Nenhum projeto aqui</strong>
      Organize iniciativas internas com tarefas e prazos.
    </div>

    <ModalDialog title="Novo projeto" :open="modalOpen" @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required placeholder="ex.: Migração do HubSpot" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="form.description" rows="2"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Prazo</label>
            <input v-model="form.due_date" type="date" />
          </div>
          <div class="field">
            <label>Dono</label>
            <select v-model="form.owner_id">
              <option :value="null">Eu</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar projeto' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 14px;
}

.project-card {
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: box-shadow 0.15s;
}

.project-card:hover {
  box-shadow: var(--shadow-lg);
}

.project-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.project-card p {
  margin: 0;
  font-size: 13px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.progress-track {
  height: 6px;
  background: var(--ci-bg);
  border-radius: 3px;
  overflow: hidden;
}

.progress-bar {
  height: 100%;
  background: var(--ci-purple);
  border-radius: 3px;
  transition: width 0.3s;
}

.project-foot {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  flex-wrap: wrap;
}
</style>
