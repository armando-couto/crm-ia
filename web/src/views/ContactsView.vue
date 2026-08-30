<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, getToken } from '../api'
import { formatDate, lifecycleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, Paginated, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const contacts = ref<Contact[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25
const search = ref('')
const stageFilter = ref('')
const ownerFilter = ref(0)
const loading = ref(false)

const users = ref<User[]>([])
const companies = ref<Company[]>([])

const modalOpen = ref(false)
const importOpen = ref(false)
const saving = ref(false)
const form = ref({
  first_name: '',
  last_name: '',
  email: '',
  phone: '',
  job_title: '',
  lifecycle_stage: 'lead',
  source: '',
  company_id: null as number | null,
  owner_id: null as number | null
})

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

function query(): string {
  const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
  if (search.value.trim()) params.set('q', search.value.trim())
  if (stageFilter.value) params.set('lifecycle_stage', stageFilter.value)
  if (ownerFilter.value) params.set('owner_id', String(ownerFilter.value))
  return params.toString()
}

async function load() {
  loading.value = true
  try {
    const resp = await api.get<Paginated<Contact>>(`/contacts?${query()}`)
    contacts.value = resp.data
    total.value = resp.pagination.total
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await api.post('/contacts', form.value)
    toast.push('Contato criado')
    modalOpen.value = false
    resetForm()
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

function resetForm() {
  form.value = {
    first_name: '',
    last_name: '',
    email: '',
    phone: '',
    job_title: '',
    lifecycle_stage: 'lead',
    source: '',
    company_id: null,
    owner_id: null
  }
}

async function importCSV(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const data = new FormData()
  data.append('file', file)
  try {
    const resp = await api.post<{ created: number; skipped: number }>('/contacts/import', data)
    toast.push(`${resp.created} contato(s) importado(s), ${resp.skipped} ignorado(s)`)
    importOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    input.value = ''
  }
}

async function exportCSV() {
  try {
    const resp = await fetch(`/api/v1/contacts/export?${query()}`, {
      headers: { Authorization: `Bearer ${getToken()}` }
    })
    if (!resp.ok) throw new Error('falha na exportação')
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `contatos-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
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
  try {
    users.value = await api.get<User[]>('/users')
    const resp = await api.get<Paginated<Company>>('/companies?per_page=100')
    companies.value = resp.data
  } catch {
    /* filtros opcionais */
  }
})

const stageBadge: Record<string, string> = {
  lead: 'gray',
  mql: 'blue',
  sql: 'blue',
  oportunidade: 'amber',
  cliente: 'green',
  perdido: 'red'
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Contatos <span class="muted" v-if="total">({{ total }})</span></h1>
      <div class="toolbar">
        <button class="btn btn-outline" type="button" @click="importOpen = true">Importar CSV</button>
        <button class="btn btn-outline" type="button" @click="exportCSV">Exportar CSV</button>
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Novo contato</button>
      </div>
    </div>

    <div class="toolbar filters">
      <input v-model="search" type="search" placeholder="Buscar por nome ou e-mail…" />
      <select v-model="stageFilter" @change="page = 1; load()">
        <option value="">Todos os estágios</option>
        <option v-for="(label, key) in lifecycleLabels" :key="key" :value="key">{{ label }}</option>
      </select>
      <select v-model.number="ownerFilter" @change="page = 1; load()">
        <option :value="0">Todos os donos</option>
        <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>E-mail</th>
            <th>Empresa</th>
            <th>Estágio</th>
            <th>Dono</th>
            <th>Atualizado</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in contacts" :key="c.id" @click="router.push(`/contatos/${c.id}`)">
            <td><strong>{{ c.first_name }} {{ c.last_name }}</strong></td>
            <td>{{ c.email || '—' }}</td>
            <td>{{ c.company_name || '—' }}</td>
            <td><span class="badge" :class="stageBadge[c.lifecycle_stage]">{{ lifecycleLabels[c.lifecycle_stage] || c.lifecycle_stage }}</span></td>
            <td>{{ c.owner_name || '—' }}</td>
            <td class="muted">{{ formatDate(c.updated_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !contacts.length" class="empty-state">
        <strong>Nenhum contato encontrado</strong>
        Ajuste os filtros ou crie o primeiro contato.
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>

    <ModalDialog title="Novo contato" :open="modalOpen" wide @close="modalOpen = false">
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
            <input v-model="form.source" placeholder="site, indicação, evento…" />
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
            <option :value="null">Eu</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar contato' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Importar contatos (CSV)" :open="importOpen" @close="importOpen = false">
      <p class="muted" style="margin-top: 0">
        Envie um CSV com as colunas: <code>nome, sobrenome, email, telefone, cargo, estagio, origem</code>.
        A primeira linha pode ser o cabeçalho.
      </p>
      <input type="file" accept=".csv,text/csv" @change="importCSV" />
    </ModalDialog>
  </div>
</template>

<style scoped>
.filters {
  margin-bottom: 14px;
}
</style>
