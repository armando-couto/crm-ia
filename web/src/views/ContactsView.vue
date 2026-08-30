<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, getToken } from '../api'
import { formatCompact, formatDate, initials, lifecycleLabels, relativeDate } from '../format'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, ContactList, Paginated, User } from '../types'

const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

// ===== Abas de visualização =====
type ViewTab = { key: string; label: string; listId?: number }
const baseTabs: ViewTab[] = [
  { key: 'todos', label: 'Todos os contatos' },
  { key: 'meus', label: 'Meus contatos' },
  { key: 'sem-dono', label: 'Não atribuídos' }
]
const listTabs = ref<ViewTab[]>([])
const activeTab = ref('todos')
const tabs = computed(() => [...baseTabs, ...listTabs.value])
const isListTab = computed(() => activeTab.value.startsWith('lista-'))

// ===== Estado da listagem =====
interface Stats {
  total: number
  sem_dono: number
  sem_email: number
  leads_sem_avanco: number
  sem_atividade_30d: number
}

const contacts = ref<Contact[]>([])
const stats = ref<Stats | null>(null)
const total = ref(0)
const page = ref(1)
const perPage = ref(25)
const search = ref('')
const stageFilter = ref('')
const ownerFilter = ref(0)
const createdFilter = ref(0) // dias
const activityFilter = ref(0) // sem atividade há N dias
const cardFilter = ref('') // sem_dono | sem_email | leads | inativos
const sortBy = ref('created_at')
const sortDir = ref<'asc' | 'desc'>('desc')
const loading = ref(false)

const selected = ref<Set<number>>(new Set())
const bulkOwner = ref<number | ''>('')
const bulkStage = ref('')

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
  const params = new URLSearchParams({
    page: String(page.value),
    per_page: String(perPage.value),
    sort: sortBy.value,
    dir: sortDir.value
  })
  if (search.value.trim()) params.set('q', search.value.trim())
  if (stageFilter.value) params.set('lifecycle_stage', stageFilter.value)
  if (ownerFilter.value) params.set('owner_id', String(ownerFilter.value))
  if (createdFilter.value) params.set('criado_dias', String(createdFilter.value))
  if (activityFilter.value) params.set('sem_atividade_dias', String(activityFilter.value))

  if (activeTab.value === 'meus' && auth.user) params.set('owner_id', String(auth.user.id))
  if (activeTab.value === 'sem-dono') params.set('sem_dono', 'true')

  if (cardFilter.value === 'sem_dono') params.set('sem_dono', 'true')
  if (cardFilter.value === 'sem_email') params.set('sem_email', 'true')
  if (cardFilter.value === 'leads') params.set('lifecycle_stage', 'lead')
  if (cardFilter.value === 'inativos') params.set('sem_atividade_dias', '30')
  return params.toString()
}

async function load() {
  loading.value = true
  selected.value = new Set()
  try {
    if (isListTab.value) {
      const listId = activeTab.value.replace('lista-', '')
      const resp = await api.get<Paginated<Contact>>(
        `/lists/${listId}/contacts?page=${page.value}&per_page=${perPage.value}`
      )
      contacts.value = resp.data
      total.value = resp.pagination.total
    } else {
      const resp = await api.get<Paginated<Contact>>(`/contacts?${query()}`)
      contacts.value = resp.data
      total.value = resp.pagination.total
    }
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  try {
    stats.value = await api.get<Stats>('/contacts/stats')
  } catch {
    /* cartões opcionais */
  }
}

function selectTab(key: string) {
  activeTab.value = key
  page.value = 1
  cardFilter.value = ''
  load()
}

function toggleCard(key: string) {
  cardFilter.value = cardFilter.value === key ? '' : key
  page.value = 1
  load()
}

function sortByColumn(column: string) {
  if (sortBy.value === column) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortBy.value = column
    sortDir.value = 'desc'
  }
  load()
}

function sortIcon(column: string): string {
  if (sortBy.value !== column) return ''
  return sortDir.value === 'asc' ? '↑' : '↓'
}

// ===== Seleção e ações em massa =====
const allSelected = computed(
  () => contacts.value.length > 0 && contacts.value.every((c) => selected.value.has(c.id))
)

function toggleAll() {
  if (allSelected.value) {
    selected.value = new Set()
  } else {
    selected.value = new Set(contacts.value.map((c) => c.id))
  }
}

function toggleOne(id: number) {
  const next = new Set(selected.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selected.value = next
}

async function bulk(action: string, payload: Record<string, unknown> = {}) {
  try {
    const resp = await api.post<{ affected: number }>('/contacts/bulk', {
      ids: [...selected.value],
      action,
      ...payload
    })
    toast.push(`${resp.affected} contato(s) atualizado(s)`)
    bulkOwner.value = ''
    bulkStage.value = ''
    await Promise.all([load(), loadStats()])
  } catch (e: any) {
    toast.error(e.message)
  }
}

function bulkAssignOwner() {
  if (bulkOwner.value === '') return
  bulk('dono', { owner_id: bulkOwner.value === 0 ? null : bulkOwner.value })
}

function bulkSetStage() {
  if (!bulkStage.value) return
  bulk('estagio', { lifecycle_stage: bulkStage.value })
}

function bulkDelete() {
  if (!confirm(`Excluir ${selected.value.size} contato(s)? Essa ação não pode ser desfeita.`)) return
  bulk('excluir')
}

// ===== Criação / importação / exportação =====
async function save() {
  saving.value = true
  try {
    await api.post('/contacts', form.value)
    toast.push('Contato criado')
    modalOpen.value = false
    resetForm()
    await Promise.all([load(), loadStats()])
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
    await Promise.all([load(), loadStats()])
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

// ===== Paginação numerada =====
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / perPage.value)))
const pageNumbers = computed(() => {
  const count = pageCount.value
  const current = page.value
  const pages: (number | '…')[] = []
  const window = 2
  let last = 0
  for (let i = 1; i <= count; i++) {
    if (i === 1 || i === count || Math.abs(i - current) <= window) {
      if (last && i - last > 1) pages.push('…')
      pages.push(i)
      last = i
    }
  }
  return pages
})

function goToPage(p: number | '…') {
  if (p === '…' || p === page.value) return
  page.value = p
  load()
}

onMounted(async () => {
  await Promise.all([load(), loadStats()])
  try {
    users.value = await api.get<User[]>('/users')
    const [companiesResp, listsResp] = await Promise.all([
      api.get<Paginated<Company>>('/companies?per_page=100'),
      api.get<ContactList[]>('/lists')
    ])
    companies.value = companiesResp.data
    listTabs.value = listsResp.map((l) => ({ key: `lista-${l.id}`, label: l.name, listId: l.id }))
  } catch {
    /* abas e filtros opcionais */
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
  <div class="page contacts-page">
    <div class="page-head">
      <h1>Contatos <span class="muted" v-if="stats">({{ formatCompact(stats.total) }})</span></h1>
      <div class="toolbar">
        <button class="btn btn-outline" type="button" @click="importOpen = true">Importar</button>
        <button class="btn btn-outline" type="button" @click="exportCSV">Exportar</button>
        <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Adicionar contato</button>
      </div>
    </div>

    <!-- Abas de visualização -->
    <div class="tabs">
      <button
        v-for="t in tabs"
        :key="t.key"
        type="button"
        class="tab"
        :class="{ active: activeTab === t.key }"
        @click="selectTab(t.key)"
      >
        {{ t.label }}
      </button>
      <button type="button" class="tab add" title="Criar visualização (lista)" @click="router.push('/listas')">+</button>
    </div>

    <!-- Filtros rápidos -->
    <div class="toolbar filters" v-if="!isListTab">
      <input v-model="search" type="search" placeholder="Pesquisar por nome ou e-mail…" />
      <select v-model.number="ownerFilter" @change="page = 1; load()" :disabled="activeTab !== 'todos'">
        <option :value="0">Proprietário do contato</option>
        <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
      <select v-model="stageFilter" @change="page = 1; load()">
        <option value="">Estágio do lead</option>
        <option v-for="(label, key) in lifecycleLabels" :key="key" :value="key">{{ label }}</option>
      </select>
      <select v-model.number="createdFilter" @change="page = 1; load()">
        <option :value="0">Data de criação</option>
        <option :value="7">Últimos 7 dias</option>
        <option :value="30">Últimos 30 dias</option>
        <option :value="90">Últimos 90 dias</option>
      </select>
      <select v-model.number="activityFilter" @change="page = 1; load()">
        <option :value="0">Última atividade</option>
        <option :value="7">Sem atividade há 7+ dias</option>
        <option :value="30">Sem atividade há 30+ dias</option>
        <option :value="90">Sem atividade há 90+ dias</option>
      </select>
    </div>

    <!-- Cartões de métricas -->
    <div class="stat-strip" v-if="stats && !isListTab">
      <button type="button" class="stat-card" :class="{ active: cardFilter === 'sem_dono' }" @click="toggleCard('sem_dono')">
        <span class="stat-label">Contatos sem proprietário</span>
        <span class="stat-value">{{ formatCompact(stats.sem_dono) }}</span>
      </button>
      <button type="button" class="stat-card" :class="{ active: cardFilter === 'sem_email' }" @click="toggleCard('sem_email')">
        <span class="stat-label">Contatos sem e-mail</span>
        <span class="stat-value">{{ formatCompact(stats.sem_email) }}</span>
      </button>
      <button type="button" class="stat-card" :class="{ active: cardFilter === 'leads' }" @click="toggleCard('leads')">
        <span class="stat-label">Leads sem avanço</span>
        <span class="stat-value">{{ formatCompact(stats.leads_sem_avanco) }}</span>
      </button>
      <button type="button" class="stat-card" :class="{ active: cardFilter === 'inativos' }" @click="toggleCard('inativos')">
        <span class="stat-label">Sem atividade recente (30d)</span>
        <span class="stat-value">{{ formatCompact(stats.sem_atividade_30d) }}</span>
      </button>
    </div>

    <!-- Barra de ações em massa -->
    <div class="bulk-bar" v-if="selected.size">
      <strong>{{ selected.size }} selecionado(s)</strong>
      <select v-model="bulkOwner" @change="bulkAssignOwner">
        <option value="" disabled>Atribuir proprietário…</option>
        <option :value="0">Remover proprietário</option>
        <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
      <select v-model="bulkStage" @change="bulkSetStage">
        <option value="" disabled>Alterar estágio…</option>
        <option v-for="(label, key) in lifecycleLabels" :key="key" :value="key">{{ label }}</option>
      </select>
      <button class="btn btn-danger btn-sm" type="button" @click="bulkDelete">Excluir</button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th style="width: 36px">
              <input type="checkbox" :checked="allSelected" @change="toggleAll" />
            </th>
            <th class="sortable" @click="sortByColumn('name')">Nome {{ sortIcon('name') }}</th>
            <th>E-mail</th>
            <th>Telefone</th>
            <th>Proprietário</th>
            <th>Empresa principal</th>
            <th class="sortable" @click="sortByColumn('last_activity')">Última atividade {{ sortIcon('last_activity') }}</th>
            <th>Estágio</th>
            <th class="sortable" @click="sortByColumn('created_at')">Criado em {{ sortIcon('created_at') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in contacts" :key="c.id" @click="router.push(`/contatos/${c.id}`)">
            <td @click.stop>
              <input type="checkbox" :checked="selected.has(c.id)" @change="toggleOne(c.id)" />
            </td>
            <td>
              <span class="name-cell">
                <span class="avatar-sm">{{ initials(`${c.first_name} ${c.last_name}`) }}</span>
                <strong>{{ c.first_name }} {{ c.last_name }}</strong>
              </span>
            </td>
            <td @click.stop>
              <a v-if="c.email" :href="`mailto:${c.email}`" class="cell-link">{{ c.email }}</a>
              <span v-else class="muted">—</span>
            </td>
            <td @click.stop>
              <a v-if="c.phone" :href="`tel:${c.phone}`" class="cell-link">{{ c.phone }}</a>
              <span v-else class="muted">—</span>
            </td>
            <td :class="{ muted: !c.owner_name }">{{ c.owner_name || 'Nenhum proprietário' }}</td>
            <td @click.stop>
              <router-link v-if="c.company_id" :to="`/empresas/${c.company_id}`" class="cell-link">
                {{ c.company_name }}
              </router-link>
              <span v-else class="muted">—</span>
            </td>
            <td class="muted">{{ c.last_activity_at ? relativeDate(c.last_activity_at) : '—' }}</td>
            <td>
              <span class="badge" :class="stageBadge[c.lifecycle_stage]">
                {{ lifecycleLabels[c.lifecycle_stage] || c.lifecycle_stage }}
              </span>
            </td>
            <td class="muted">{{ formatDate(c.created_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !contacts.length" class="empty-state">
        <strong>Nenhum contato encontrado</strong>
        Ajuste os filtros ou adicione o primeiro contato.
      </div>

      <div class="pager" v-if="pageCount > 1 || total > 25">
        <button class="btn btn-outline btn-sm" :disabled="page === 1" @click="goToPage(page - 1)">‹ Voltar</button>
        <button
          v-for="(p, i) in pageNumbers"
          :key="`${p}-${i}`"
          type="button"
          class="page-btn"
          :class="{ current: p === page, dots: p === '…' }"
          :disabled="p === '…'"
          @click="goToPage(p)"
        >
          {{ p }}
        </button>
        <button class="btn btn-outline btn-sm" :disabled="page >= pageCount" @click="goToPage(page + 1)">Próximo ›</button>
        <select v-model.number="perPage" @change="page = 1; load()">
          <option :value="25">25 por página</option>
          <option :value="50">50 por página</option>
          <option :value="100">100 por página</option>
        </select>
      </div>
    </div>

    <ModalDialog title="Adicionar contato" :open="modalOpen" wide @close="modalOpen = false">
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
          <label>Proprietário</label>
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
.contacts-page {
  max-width: 1500px;
}

/* Abas */
.tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 14px;
  overflow-x: auto;
}

.tab {
  padding: 9px 16px;
  border: none;
  background: none;
  font-size: 14px;
  color: var(--fix-text-2);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  white-space: nowrap;
  transition: color 0.15s, border-color 0.15s;
}

.tab:hover {
  color: var(--fix-purple);
}

.tab.active {
  color: var(--fix-purple);
  border-bottom-color: var(--fix-purple);
  font-weight: 600;
}

.tab.add {
  font-size: 16px;
  padding: 9px 12px;
}

.filters {
  margin-bottom: 14px;
}

/* Cartões de métricas */
.stat-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.stat-card {
  background: var(--fix-surface);
  border: 1px solid transparent;
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  padding: 14px 16px;
  text-align: center;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
  transition: border-color 0.15s, box-shadow 0.15s;
}

.stat-card:hover {
  box-shadow: var(--shadow-lg);
}

.stat-card.active {
  border-color: var(--fix-purple);
  background: var(--fix-purple-tint);
}

.stat-label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  font-weight: 600;
  color: var(--fix-text-3);
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--fix-purple-dark);
}

/* Barra de ações em massa */
.bulk-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  background: var(--fix-purple-tint);
  border: 1px solid var(--fix-purple);
  border-radius: 10px;
  padding: 10px 16px;
  margin-bottom: 12px;
  font-size: 14px;
  flex-wrap: wrap;
}

.bulk-bar select {
  padding: 6px 10px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  font-size: 13px;
  background: var(--fix-surface);
}

/* Tabela */
.sortable {
  cursor: pointer;
  user-select: none;
}

.sortable:hover {
  color: var(--fix-purple);
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.avatar-sm {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  font-size: 11px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.cell-link {
  color: var(--fix-purple);
}

.cell-link:hover {
  text-decoration: underline;
}

/* Paginação numerada */
.pager {
  justify-content: center;
  flex-wrap: wrap;
}

.page-btn {
  min-width: 30px;
  height: 30px;
  border: none;
  background: none;
  border-radius: 6px;
  font-size: 13px;
  color: var(--fix-text-2);
  cursor: pointer;
}

.page-btn:hover:not(.current):not(.dots) {
  background: var(--fix-purple-tint);
}

.page-btn.current {
  background: var(--fix-purple);
  color: #fff;
  font-weight: 600;
}

.page-btn.dots {
  cursor: default;
}

.pager select {
  padding: 5px 8px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  font-size: 13px;
  margin-left: 8px;
}
</style>
