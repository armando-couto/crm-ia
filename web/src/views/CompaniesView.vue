<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Paginated, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const companies = ref<Company[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 25
const search = ref('')
const loading = ref(false)
const users = ref<User[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  name: '',
  domain: '',
  phone: '',
  industry: '',
  city: '',
  state: '',
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

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
    if (search.value.trim()) params.set('q', search.value.trim())
    const resp = await api.get<Paginated<Company>>(`/companies?${params}`)
    companies.value = resp.data
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
    await api.post('/companies', form.value)
    toast.push('Empresa criada')
    modalOpen.value = false
    form.value = { name: '', domain: '', phone: '', industry: '', city: '', state: '', owner_id: null }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
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
  } catch {
    /* opcional */
  }
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Empresas <span class="muted" v-if="total">({{ total }})</span></h1>
      <div class="toolbar">
        <input v-model="search" type="search" placeholder="Buscar por nome ou domínio…" />
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Nova empresa</button>
      </div>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>Domínio</th>
            <th>Segmento</th>
            <th>Cidade/UF</th>
            <th>Contatos</th>
            <th>Dono</th>
            <th>Criada</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in companies" :key="c.id" @click="router.push(`/empresas/${c.id}`)">
            <td><strong>{{ c.name }}</strong></td>
            <td>{{ c.domain || '—' }}</td>
            <td>{{ c.industry || '—' }}</td>
            <td>{{ c.city ? `${c.city}${c.state ? '/' + c.state : ''}` : '—' }}</td>
            <td>{{ c.contacts_count ?? 0 }}</td>
            <td>{{ c.owner_name || '—' }}</td>
            <td class="muted">{{ formatDate(c.created_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !companies.length" class="empty-state">
        <strong>Nenhuma empresa encontrada</strong>
        Crie a primeira empresa para organizar seus contatos.
      </div>

      <div class="pager" v-if="total > perPage">
        <span>{{ (page - 1) * perPage + 1 }}–{{ Math.min(page * perPage, total) }} de {{ total }}</span>
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="changePage(-1)">Anterior</button>
        <button class="btn btn-outline btn-sm" :disabled="page * perPage >= total" @click="changePage(1)">Próxima</button>
      </div>
    </div>

    <ModalDialog title="Nova empresa" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.name" required />
          </div>
          <div class="field">
            <label>Domínio</label>
            <input v-model="form.domain" placeholder="empresa.com.br" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Telefone</label>
            <input v-model="form.phone" />
          </div>
          <div class="field">
            <label>Segmento</label>
            <input v-model="form.industry" placeholder="varejo, saúde, serviços…" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Cidade</label>
            <input v-model="form.city" />
          </div>
          <div class="field">
            <label>UF</label>
            <input v-model="form.state" maxlength="2" />
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
          {{ saving ? 'Salvando…' : 'Criar empresa' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>
