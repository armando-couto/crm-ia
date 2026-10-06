<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Playbook } from '../types'

const toast = useToastStore()

const playbooks = ref<Playbook[]>([])
const loading = ref(false)
const reading = ref<Playbook | null>(null)

const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<Playbook | null>(null)
const form = ref({ name: '', description: '', body: '', active: true })

async function load() {
  loading.value = true
  try {
    playbooks.value = await api.get<Playbook[]>('/playbooks')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function openNew() {
  editing.value = null
  form.value = { name: '', description: '', body: '', active: true }
  modalOpen.value = true
}

function openEdit(p: Playbook) {
  editing.value = p
  form.value = { name: p.name, description: p.description, body: p.body, active: p.active }
  modalOpen.value = true
}

async function save() {
  saving.value = true
  try {
    if (editing.value) {
      await api.put(`/playbooks/${editing.value.id}`, form.value)
      toast.push('Manual atualizado')
    } else {
      await api.post('/playbooks', form.value)
      toast.push('Manual criado')
    }
    modalOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(p: Playbook) {
  if (!confirm(`Remover o manual "${p.name}"?`)) return
  try {
    await api.delete(`/playbooks/${p.id}`)
    toast.push('Manual removido')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Manuais de atividades</h1>
      <button class="btn btn-primary" type="button" @click="openNew">+ Novo manual</button>
    </div>

    <div class="grid">
      <div v-for="p in playbooks" :key="p.id" class="card playbook-card" @click="reading = p">
        <div class="playbook-head">
          <strong>{{ p.name }}</strong>
          <span class="badge" :class="p.active ? 'green' : 'gray'">{{ p.active ? 'Ativo' : 'Inativo' }}</span>
        </div>
        <p class="muted">{{ p.description || 'Sem descrição' }}</p>
        <div class="playbook-foot muted">
          <span>atualizado {{ formatDate(p.updated_at) }}</span>
          <span @click.stop>
            <button class="btn btn-outline btn-sm" type="button" @click="openEdit(p)">Editar</button>
            <button class="btn btn-danger btn-sm" type="button" @click="remove(p)">✕</button>
          </span>
        </div>
      </div>
    </div>

    <div v-if="!loading && !playbooks.length" class="empty-state">
      <strong>Nenhum manual criado</strong>
      Documente roteiros de ligação, qualificação e atendimento para a equipe seguir.
    </div>

    <ModalDialog v-if="reading" :title="reading.name" :open="true" wide @close="reading = null">
      <p class="muted" v-if="reading.description" style="margin-top: 0">{{ reading.description }}</p>
      <div class="playbook-body">{{ reading.body }}</div>
    </ModalDialog>

    <ModalDialog :title="editing ? 'Editar manual' : 'Novo manual'" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required placeholder="ex.: Roteiro de qualificação de leads" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <input v-model="form.description" placeholder="Quando usar este manual" />
        </div>
        <div class="field">
          <label>Conteúdo *</label>
          <textarea v-model="form.body" rows="10" required placeholder="Passo a passo, perguntas a fazer, objeções comuns…"></textarea>
        </div>
        <div class="field">
          <label class="check-inline">
            <input v-model="form.active" type="checkbox" />
            Manual ativo
          </label>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar manual' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.playbook-card {
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.playbook-card:hover {
  box-shadow: var(--shadow-lg);
}

.playbook-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
}

.playbook-card p {
  margin: 0;
  font-size: 13px;
  flex: 1;
}

.playbook-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}

.playbook-body {
  white-space: pre-wrap;
  font-size: 14px;
  color: var(--ci-text-2);
  max-height: 60vh;
  overflow-y: auto;
}

.check-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
</style>
