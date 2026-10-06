<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { MessageTemplate } from '../types'

const toast = useToastStore()

const templates = ref<MessageTemplate[]>([])
const loading = ref(false)

const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<MessageTemplate | null>(null)
const form = ref({ name: '', subject: '', body: '' })

async function load() {
  loading.value = true
  try {
    templates.value = await api.get<MessageTemplate[]>('/templates')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function openNew() {
  editing.value = null
  form.value = { name: '', subject: '', body: '' }
  modalOpen.value = true
}

function openEdit(t: MessageTemplate) {
  editing.value = t
  form.value = { name: t.name, subject: t.subject, body: t.body }
  modalOpen.value = true
}

async function save() {
  saving.value = true
  try {
    if (editing.value) {
      await api.put(`/templates/${editing.value.id}`, form.value)
      toast.push('Modelo atualizado')
    } else {
      await api.post('/templates', form.value)
      toast.push('Modelo criado')
    }
    modalOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(t: MessageTemplate) {
  if (!confirm(`Remover o modelo "${t.name}"?`)) return
  try {
    await api.delete(`/templates/${t.id}`)
    toast.push('Modelo removido')
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
        <h1>Modelos de mensagens</h1>
        <p class="muted" style="margin: 4px 0 0">
          Use as variáveis <code v-pre>{{nome}}</code>, <code v-pre>{{sobrenome}}</code>, <code v-pre>{{email}}</code>,
          <code v-pre>{{empresa}}</code> e <code v-pre>{{cargo}}</code> — elas são preenchidas com os dados do contato no envio.
        </p>
      </div>
      <button class="btn btn-primary" type="button" @click="openNew">+ Novo modelo</button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>Assunto</th>
            <th>Atualizado</th>
            <th style="width: 140px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in templates" :key="t.id" style="cursor: default">
            <td><strong>{{ t.name }}</strong></td>
            <td>{{ t.subject }}</td>
            <td class="muted">{{ formatDate(t.updated_at) }}</td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="openEdit(t)">Editar</button>
              <button class="btn btn-danger btn-sm" type="button" @click="remove(t)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !templates.length" class="empty-state">
        <strong>Nenhum modelo criado</strong>
        Modelos aceleram o envio de e-mails recorrentes (apresentação, follow-up, proposta).
      </div>
    </div>

    <ModalDialog :title="editing ? 'Editar modelo' : 'Novo modelo'" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required placeholder="ex.: Primeiro contato" />
        </div>
        <div class="field">
          <label>Assunto *</label>
          <input v-model="form.subject" required placeholder="ex.: {{nome}}, uma proposta para a {{empresa}}" />
        </div>
        <div class="field">
          <label>Mensagem *</label>
          <textarea v-model="form.body" rows="8" required placeholder="Olá {{nome}}, tudo bem?&#10;&#10;…"></textarea>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar modelo' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>
