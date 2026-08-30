<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, lifecycleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { ContactList, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const lists = ref<ContactList[]>([])
const users = ref<User[]>([])
const loading = ref(false)

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  name: '',
  kind: 'estatica' as 'estatica' | 'dinamica',
  rules: { lifecycle_stage: '', owner_id: 0, source: '' }
})

async function load() {
  loading.value = true
  try {
    lists.value = await api.get<ContactList[]>('/lists')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const payload: any = { name: form.value.name, kind: form.value.kind }
    if (form.value.kind === 'dinamica') {
      payload.rules = {
        lifecycle_stage: form.value.rules.lifecycle_stage || undefined,
        owner_id: form.value.rules.owner_id || undefined,
        source: form.value.rules.source || undefined
      }
    }
    await api.post('/lists', payload)
    toast.push('Lista criada')
    modalOpen.value = false
    form.value = { name: '', kind: 'estatica', rules: { lifecycle_stage: '', owner_id: 0, source: '' } }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(list: ContactList) {
  if (!confirm(`Remover a lista "${list.name}"? Os contatos não são apagados.`)) return
  try {
    await api.delete(`/lists/${list.id}`)
    toast.push('Lista removida')
    await load()
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
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Listas</h1>
      <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Nova lista</button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>Tipo</th>
            <th>Contatos</th>
            <th>Atualizada</th>
            <th style="width: 100px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in lists" :key="l.id" @click="router.push(`/listas/${l.id}`)">
            <td><strong>{{ l.name }}</strong></td>
            <td>
              <span class="badge" :class="l.kind === 'dinamica' ? 'blue' : 'gray'">
                {{ l.kind === 'dinamica' ? 'Dinâmica' : 'Estática' }}
              </span>
            </td>
            <td>{{ l.kind === 'dinamica' ? 'pelas regras' : l.members_count }}</td>
            <td class="muted">{{ formatDate(l.updated_at) }}</td>
            <td @click.stop>
              <button class="btn btn-danger btn-sm" type="button" @click="remove(l)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !lists.length" class="empty-state">
        <strong>Nenhuma lista criada</strong>
        Use listas para segmentar contatos (ex.: clientes ativos, leads de eventos).
      </div>
    </div>

    <ModalDialog title="Nova lista" :open="modalOpen" @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required placeholder="ex.: Leads do evento de setembro" />
        </div>
        <div class="field">
          <label>Tipo</label>
          <select v-model="form.kind">
            <option value="estatica">Estática — adiciono os contatos manualmente</option>
            <option value="dinamica">Dinâmica — calculada por regras</option>
          </select>
        </div>

        <template v-if="form.kind === 'dinamica'">
          <div class="field">
            <label>Estágio do ciclo de vida</label>
            <select v-model="form.rules.lifecycle_stage">
              <option value="">Qualquer</option>
              <option v-for="(label, key) in lifecycleLabels" :key="key" :value="key">{{ label }}</option>
            </select>
          </div>
          <div class="form-row">
            <div class="field">
              <label>Dono</label>
              <select v-model.number="form.rules.owner_id">
                <option :value="0">Qualquer</option>
                <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
              </select>
            </div>
            <div class="field">
              <label>Origem</label>
              <input v-model="form.rules.source" placeholder="ex.: site" />
            </div>
          </div>
        </template>

        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar lista' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>
