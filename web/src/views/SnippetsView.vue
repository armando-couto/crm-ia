<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Snippet } from '../types'

const toast = useToastStore()

const snippets = ref<Snippet[]>([])
const loading = ref(false)

const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<Snippet | null>(null)
const form = ref({ name: '', shortcut: '', body: '' })

async function load() {
  loading.value = true
  try {
    snippets.value = await api.get<Snippet[]>('/snippets')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function openNew() {
  editing.value = null
  form.value = { name: '', shortcut: '', body: '' }
  modalOpen.value = true
}

function openEdit(s: Snippet) {
  editing.value = s
  form.value = { name: s.name, shortcut: s.shortcut, body: s.body }
  modalOpen.value = true
}

async function save() {
  saving.value = true
  try {
    if (editing.value) {
      await api.put(`/snippets/${editing.value.id}`, form.value)
      toast.push('Snippet atualizado')
    } else {
      await api.post('/snippets', form.value)
      toast.push('Snippet criado')
    }
    modalOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(s: Snippet) {
  if (!confirm(`Remover o snippet "${s.name}"?`)) return
  try {
    await api.delete(`/snippets/${s.id}`)
    toast.push('Snippet removido')
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
      <div>
        <h1>Snippets</h1>
        <p class="muted" style="margin: 4px 0 0">
          Trechos rápidos reutilizáveis nas notas e e-mails (menu "Snippet…" do composer).
        </p>
      </div>
      <button class="btn btn-primary" type="button" @click="openNew">+ Novo snippet</button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>Atalho</th>
            <th>Texto</th>
            <th>Atualizado</th>
            <th style="width: 140px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in snippets" :key="s.id" style="cursor: default">
            <td><strong>{{ s.name }}</strong></td>
            <td><span class="badge">{{ s.shortcut }}</span></td>
            <td class="muted body-cell">{{ s.body }}</td>
            <td class="muted">{{ formatDate(s.updated_at) }}</td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="openEdit(s)">Editar</button>
              <button class="btn btn-danger btn-sm" type="button" @click="remove(s)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !snippets.length" class="empty-state">
        <strong>Nenhum snippet criado</strong>
        Crie respostas rápidas para saudações, assinaturas e perguntas frequentes.
      </div>
    </div>

    <ModalDialog :title="editing ? 'Editar snippet' : 'Novo snippet'" :open="modalOpen" @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required placeholder="ex.: Saudação padrão" />
        </div>
        <div class="field">
          <label>Atalho *</label>
          <input v-model="form.shortcut" required placeholder="#saudacao" />
        </div>
        <div class="field">
          <label>Texto *</label>
          <textarea v-model="form.body" rows="5" required></textarea>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar snippet' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.body-cell {
  max-width: 340px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
