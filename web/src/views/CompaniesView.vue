<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import AdvancedFilters from '../components/AdvancedFilters.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, FilterFieldDef, FilterGroup, Paginated, SavedView, User } from '../types'

const router = useRouter()
const toast = useToastStore()

// ===== Abas de visualização =====
const savedViews = ref<SavedView[]>([])
const activeTab = ref('todas')
const activeViewId = computed(() => {
  if (!activeTab.value.startsWith('view-')) return 0
  return Number(activeTab.value.replace('view-', ''))
})

const companies = ref<Company[]>([])
const total = ref(0)
const page = ref(1)
const perPage = ref(25)
const search = ref('')
const ownerFilter = ref(0)
const createdFilter = ref(0)
const unassignedOnly = ref(false)
const sortBy = ref('name')
const sortDir = ref<'asc' | 'desc'>('asc')
const loading = ref(false)
const users = ref<User[]>([])

// ===== Filtros avançados =====
const advOpen = ref(false)
const advGroups = ref<FilterGroup[]>([])
const advCount = computed(() => advGroups.value.reduce((sum, g) => sum + g.conditions.length, 0))

const advFields = computed<FilterFieldDef[]>(() => [
  { key: 'name', label: 'Nome da empresa', kind: 'text' },
  { key: 'domain', label: 'Domínio', kind: 'text' },
  { key: 'industry', label: 'Segmento', kind: 'text' },
  { key: 'city', label: 'Cidade', kind: 'text' },
  { key: 'state', label: 'UF', kind: 'text' },
  { key: 'phone', label: 'Telefone', kind: 'text' },
  {
    key: 'owner_id',
    label: 'Proprietário da empresa',
    kind: 'ref',
    options: users.value.map((u) => ({ value: String(u.id), label: u.name }))
  },
  { key: 'created_at', label: 'Data de criação', kind: 'date' }
])

function applyAdvanced(groups: FilterGroup[]) {
  advGroups.value = groups
  page.value = 1
  load()
}

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

function query(): string {
  const params = new URLSearchParams({
    page: String(page.value),
    per_page: String(perPage.value),
    sort: sortBy.value,
    dir: sortDir.value
  })
  if (search.value.trim()) params.set('q', search.value.trim())
  if (ownerFilter.value) params.set('owner_id', String(ownerFilter.value))
  if (createdFilter.value) params.set('criado_dias', String(createdFilter.value))
  if (unassignedOnly.value) params.set('sem_dono', 'true')
  if (advGroups.value.length) params.set('af', JSON.stringify({ groups: advGroups.value }))
  return params.toString()
}

async function load() {
  loading.value = true
  try {
    const resp = await api.get<Paginated<Company>>(`/companies?${query()}`)
    companies.value = resp.data
    total.value = resp.pagination.total
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

// ===== Visualizações salvas =====
const hasActiveFilters = computed(
  () =>
    !!search.value.trim() ||
    !!ownerFilter.value ||
    !!createdFilter.value ||
    unassignedOnly.value ||
    advGroups.value.length > 0
)

function currentFilters(): Record<string, unknown> {
  return {
    q: search.value.trim(),
    owner_id: ownerFilter.value,
    criado_dias: createdFilter.value,
    sem_dono: unassignedOnly.value,
    sort: sortBy.value,
    dir: sortDir.value,
    af: advGroups.value
  }
}

function applyFilters(filters: Record<string, any>) {
  search.value = filters.q ?? ''
  ownerFilter.value = Number(filters.owner_id) || 0
  createdFilter.value = Number(filters.criado_dias) || 0
  unassignedOnly.value = !!filters.sem_dono
  sortBy.value = filters.sort || 'name'
  sortDir.value = filters.dir === 'desc' ? 'desc' : 'asc'
  advGroups.value = Array.isArray(filters.af) ? filters.af : []
}

function selectTab(key: string) {
  activeTab.value = key
  page.value = 1
  if (key.startsWith('view-')) {
    const view = savedViews.value.find((v) => v.id === Number(key.replace('view-', '')))
    if (view) applyFilters((view.filters as Record<string, any>) || {})
  } else {
    applyFilters({})
  }
  load()
}

async function saveCurrentView() {
  const name = prompt('Nome da visualização (ex.: Clientes ativos):')
  if (!name?.trim()) return
  try {
    const view = await api.post<SavedView>('/views', {
      entity: 'companies',
      name: name.trim(),
      filters: currentFilters()
    })
    savedViews.value = [...savedViews.value, view]
    activeTab.value = `view-${view.id}`
    toast.push('Visualização salva para toda a equipe')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function deleteView(viewId: number) {
  if (!confirm('Remover esta visualização? As empresas não são afetadas.')) return
  try {
    await api.delete(`/views/${viewId}`)
    savedViews.value = savedViews.value.filter((v) => v.id !== viewId)
    if (activeViewId.value === viewId) selectTab('todas')
    toast.push('Visualização removida')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Ordenação =====
function sortByColumn(column: string) {
  if (sortBy.value === column) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortBy.value = column
    sortDir.value = column === 'name' ? 'asc' : 'desc'
  }
  load()
}

function sortIcon(column: string): string {
  if (sortBy.value !== column) return ''
  return sortDir.value === 'asc' ? '↑' : '↓'
}

// ===== Paginação numerada =====
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / perPage.value)))
const pageNumbers = computed(() => {
  const count = pageCount.value
  const current = page.value
  const pages: (number | '…')[] = []
  let last = 0
  for (let i = 1; i <= count; i++) {
    if (i === 1 || i === count || Math.abs(i - current) <= 2) {
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

onMounted(async () => {
  await load()
  try {
    const [usersResp, viewsResp] = await Promise.all([
      api.get<User[]>('/users'),
      api.get<SavedView[]>('/views?entity=companies')
    ])
    users.value = usersResp
    savedViews.value = viewsResp
  } catch {
    /* opcional */
  }
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Empresas <span class="muted" v-if="total">({{ total }})</span></h1>
      <button class="btn btn-primary" type="button" @click="modalOpen = true">+ Adicionar empresa</button>
    </div>

    <!-- Abas de visualização -->
    <div class="tabs">
      <span class="tab-wrap">
        <button type="button" class="tab" :class="{ active: activeTab === 'todas' }" @click="selectTab('todas')">
          Todas as empresas
        </button>
      </span>
      <span v-for="v in savedViews" :key="v.id" class="tab-wrap">
        <button type="button" class="tab" :class="{ active: activeTab === `view-${v.id}` }" @click="selectTab(`view-${v.id}`)">
          {{ v.name }}
        </button>
        <button
          v-if="activeTab === `view-${v.id}`"
          type="button"
          class="tab-close"
          title="Remover visualização"
          @click="deleteView(v.id)"
        >
          ×
        </button>
      </span>
    </div>

    <!-- Filtros rápidos -->
    <div class="toolbar filters">
      <input v-model="search" type="search" placeholder="Pesquisar por nome ou domínio…" />
      <select v-model.number="ownerFilter" @change="page = 1; load()">
        <option :value="0">Proprietário da empresa</option>
        <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
      <select v-model.number="createdFilter" @change="page = 1; load()">
        <option :value="0">Data de criação</option>
        <option :value="7">Últimos 7 dias</option>
        <option :value="30">Últimos 30 dias</option>
        <option :value="90">Últimos 90 dias</option>
      </select>
      <label class="check">
        <input v-model="unassignedOnly" type="checkbox" @change="page = 1; load()" />
        Sem proprietário
      </label>
      <button class="btn btn-outline btn-sm adv-btn" type="button" :class="{ on: advCount }" @click="advOpen = true">
        ≡ Filtros avançados
        <span v-if="advCount" class="adv-count">{{ advCount }}</span>
      </button>
      <button v-if="hasActiveFilters" class="btn btn-outline btn-sm save-view" type="button" @click="saveCurrentView">
        ☆ Salvar visualização
      </button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th class="sortable" @click="sortByColumn('name')">Nome da empresa {{ sortIcon('name') }}</th>
            <th>Domínio</th>
            <th>Segmento</th>
            <th>Cidade/UF</th>
            <th class="sortable" @click="sortByColumn('contacts')">Contatos {{ sortIcon('contacts') }}</th>
            <th>Proprietário</th>
            <th class="sortable" @click="sortByColumn('created_at')">Data de criação {{ sortIcon('created_at') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in companies" :key="c.id" @click="router.push(`/empresas/${c.id}`)">
            <td><strong>{{ c.name }}</strong></td>
            <td>{{ c.domain || '—' }}</td>
            <td>{{ c.industry || '—' }}</td>
            <td>{{ c.city ? `${c.city}${c.state ? '/' + c.state : ''}` : '—' }}</td>
            <td>{{ c.contacts_count ?? 0 }}</td>
            <td :class="{ muted: !c.owner_name }">{{ c.owner_name || 'Nenhum proprietário' }}</td>
            <td class="muted">{{ formatDate(c.created_at) }}</td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !companies.length" class="empty-state">
        <strong>Nenhuma empresa encontrada</strong>
        Ajuste os filtros ou adicione a primeira empresa.
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

    <AdvancedFilters
      :open="advOpen"
      :fields="advFields"
      :model-value="advGroups"
      @close="advOpen = false"
      @apply="applyAdvanced"
    />

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
          <label>Proprietário</label>
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

<style scoped>
.tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 14px;
  overflow-x: auto;
}

.tab-wrap {
  display: inline-flex;
  align-items: center;
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
}

.tab:hover {
  color: var(--fix-purple);
}

.tab.active {
  color: var(--fix-purple);
  border-bottom-color: var(--fix-purple);
  font-weight: 600;
}

.tab-close {
  border: none;
  background: none;
  color: var(--fix-text-3);
  cursor: pointer;
  font-size: 15px;
  padding: 2px 6px;
  border-radius: 4px;
}

.tab-close:hover {
  color: var(--fix-red);
  background: var(--fix-red-tint);
}

.filters {
  margin-bottom: 14px;
}

.check {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--fix-text-2);
  cursor: pointer;
}

.save-view {
  color: var(--fix-purple);
}

.adv-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.adv-btn.on {
  border-color: var(--fix-purple);
  color: var(--fix-purple);
  background: var(--fix-purple-tint);
}

.adv-count {
  background: var(--fix-purple);
  color: #fff;
  border-radius: 999px;
  min-width: 18px;
  height: 18px;
  font-size: 11px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 5px;
}

.sortable {
  cursor: pointer;
  user-select: none;
}

.sortable:hover {
  color: var(--fix-purple);
}

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
