<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, lifecycleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import type { Contact, ContactList, Paginated } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const list = ref<ContactList | null>(null)
const contacts = ref<Contact[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25

const allContacts = ref<Contact[]>([])
const addContactId = ref(0)

async function load() {
  try {
    const resp = await api.get<Paginated<Contact> & { list: ContactList }>(
      `/lists/${id}/contacts?page=${page.value}&per_page=${perPage}`
    )
    contacts.value = resp.data
    total.value = resp.pagination.total
    list.value = resp.list
  } catch (e: any) {
    toast.error(e.message)
    router.push('/listas')
  }
}

async function addMember() {
  if (!addContactId.value) return
  try {
    await api.post(`/lists/${id}/contacts`, { contact_id: addContactId.value })
    addContactId.value = 0
    toast.push('Contato adicionado')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removeMember(contact: Contact) {
  try {
    await api.delete(`/lists/${id}/contacts/${contact.id}`)
    toast.push('Contato removido da lista')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

function changePage(delta: number) {
  page.value += delta
  load()
}

onMounted(async () => {
  await load()
  if (list.value?.kind === 'estatica') {
    try {
      const resp = await api.get<Paginated<Contact>>('/contacts?per_page=100')
      allContacts.value = resp.data
    } catch {
      /* opcional */
    }
  }
})
</script>

<template>
  <div class="page" v-if="list">
    <div class="page-head">
      <div>
        <h1>{{ list.name }}</h1>
        <p class="muted head-sub">
          <span class="badge" :class="list.kind === 'dinamica' ? 'blue' : 'gray'">
            {{ list.kind === 'dinamica' ? 'Dinâmica' : 'Estática' }}
          </span>
          <template v-if="list.kind === 'dinamica' && list.rules">
            <span class="badge gray" v-if="list.rules.lifecycle_stage">estágio: {{ lifecycleLabels[list.rules.lifecycle_stage] }}</span>
            <span class="badge gray" v-if="list.rules.source">origem: {{ list.rules.source }}</span>
          </template>
        </p>
      </div>
      <div class="toolbar" v-if="list.kind === 'estatica'">
        <select v-model.number="addContactId">
          <option :value="0">Adicionar contato…</option>
          <option v-for="c in allContacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
        </select>
        <button class="btn btn-primary btn-sm" type="button" :disabled="!addContactId" @click="addMember">Adicionar</button>
      </div>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>E-mail</th>
            <th>Estágio</th>
            <th>Empresa</th>
            <th>Criado</th>
            <th v-if="list.kind === 'estatica'" style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in contacts" :key="c.id" @click="router.push(`/contatos/${c.id}`)">
            <td><strong>{{ c.first_name }} {{ c.last_name }}</strong></td>
            <td>{{ c.email || '—' }}</td>
            <td><span class="badge gray">{{ lifecycleLabels[c.lifecycle_stage] || c.lifecycle_stage }}</span></td>
            <td>{{ c.company_name || '—' }}</td>
            <td class="muted">{{ formatDate(c.created_at) }}</td>
            <td v-if="list.kind === 'estatica'" @click.stop>
              <button class="btn btn-danger btn-sm" type="button" @click="removeMember(c)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!contacts.length" class="empty-state">
        <strong>Lista vazia</strong>
        <template v-if="list.kind === 'estatica'">Adicione contatos usando o seletor acima.</template>
        <template v-else>Nenhum contato corresponde às regras desta lista.</template>
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.head-sub {
  display: flex;
  gap: 8px;
  margin: 6px 0 0;
  flex-wrap: wrap;
}
</style>
