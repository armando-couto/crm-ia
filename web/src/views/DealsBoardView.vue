<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, getToken } from '../api'
import { formatMoney, relativeDate } from '../format'
import { useToastStore } from '../stores/toast'
import AdvancedFilters from '../components/AdvancedFilters.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, Deal, FilterFieldDef, FilterGroup, Paginated, Pipeline, SavedView, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const pipelines = ref<Pipeline[]>([])
const pipelineId = ref(0)
const deals = ref<Deal[]>([])
const loading = ref(true)
const dragging = ref<number | null>(null)
const dragOverStage = ref<number | null>(null)

const search = ref('')
const ownerFilter = ref(0)
const temperatureFilter = ref('')
const createdFilter = ref(0)
const activityFilter = ref(0)
const closeWindowFilter = ref('')

// ===== Filtros avançados =====
const advOpen = ref(false)
const advGroups = ref<FilterGroup[]>([])
const advCount = computed(() => advGroups.value.reduce((sum, g) => sum + g.conditions.length, 0))

const advFields = computed<FilterFieldDef[]>(() => [
  { key: 'name', label: 'Nome do negócio', kind: 'text' },
  { key: 'amount', label: 'Valor (R$)', kind: 'number' },
  {
    key: 'temperature',
    label: 'Temperatura do deal',
    kind: 'enum',
    options: [
      { value: 'quente', label: 'Quente' },
      { value: 'media', label: 'Média' },
      { value: 'fria', label: 'Fria' }
    ]
  },
  {
    key: 'status',
    label: 'Status',
    kind: 'enum',
    options: [
      { value: 'aberto', label: 'Aberto' },
      { value: 'ganho', label: 'Ganho' },
      { value: 'perdido', label: 'Perdido' }
    ]
  },
  {
    key: 'stage_id',
    label: 'Etapa',
    kind: 'ref',
    options: (pipeline.value?.stages ?? []).map((s) => ({ value: String(s.id), label: s.name }))
  },
  {
    key: 'owner_id',
    label: 'Proprietário do negócio',
    kind: 'ref',
    options: users.value.map((u) => ({ value: String(u.id), label: u.name }))
  },
  {
    key: 'company_id',
    label: 'Empresa',
    kind: 'ref',
    options: companies.value.map((c) => ({ value: String(c.id), label: c.name }))
  },
  { key: 'created_at', label: 'Data de criação', kind: 'date' },
  { key: 'close_date', label: 'Data de fechamento (previsão)', kind: 'date' },
  { key: 'last_activity', label: 'Data da última atividade', kind: 'date' }
])

function applyAdvanced(groups: FilterGroup[]) {
  advGroups.value = groups
  loadBoard()
}

const users = ref<User[]>([])
const contacts = ref<Contact[]>([])
const companies = ref<Company[]>([])

// ===== Visualizações salvas =====
const savedViews = ref<SavedView[]>([])
const activeTab = ref('funil')

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  name: '',
  amount: 0,
  stage_id: 0,
  contact_id: null as number | null,
  company_id: null as number | null,
  owner_id: null as number | null,
  temperature: '',
  close_date: ''
})

const pipeline = computed(() => pipelines.value.find((p) => p.id === pipelineId.value))
const openStages = computed(() => pipeline.value?.stages.filter((s) => !s.is_won && !s.is_lost) ?? [])
const closedStages = computed(() => pipeline.value?.stages.filter((s) => s.is_won || s.is_lost) ?? [])

const temperatureLabels: Record<string, string> = { quente: 'Quente', media: 'Média', fria: 'Fria' }
const temperatureDot: Record<string, string> = { quente: '#d64550', media: '#c78a1b', fria: '#2b7ecb' }

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(loadBoard, 300)
})

function dealsInStage(stageId: number): Deal[] {
  return deals.value.filter((d) => d.stage_id === stageId && d.status === 'aberto')
}

function closedInStage(stageId: number): Deal[] {
  return deals.value.filter((d) => d.stage_id === stageId && d.status !== 'aberto')
}

function stageTotal(stageId: number): number {
  return dealsInStage(stageId).reduce((sum, d) => sum + d.amount, 0)
}

function stageWeighted(stage: { id: number; probability: number }): number {
  return (stageTotal(stage.id) * stage.probability) / 100
}

const totalDeals = computed(() => deals.value.length)

async function loadPipelines() {
  pipelines.value = await api.get<Pipeline[]>('/pipelines')
  if (!pipelineId.value && pipelines.value.length) {
    pipelineId.value = pipelines.value[0].id
  }
}

function boardQuery(): string {
  const params = new URLSearchParams({ pipeline_id: String(pipelineId.value) })
  if (search.value.trim()) params.set('q', search.value.trim())
  if (ownerFilter.value) params.set('owner_id', String(ownerFilter.value))
  if (temperatureFilter.value) params.set('temperature', temperatureFilter.value)
  if (createdFilter.value) params.set('criado_dias', String(createdFilter.value))
  if (activityFilter.value) params.set('sem_atividade_dias', String(activityFilter.value))
  if (closeWindowFilter.value) params.set('fechamento', closeWindowFilter.value)
  if (advGroups.value.length) params.set('af', JSON.stringify({ groups: advGroups.value }))
  return params.toString()
}

async function loadBoard() {
  if (!pipelineId.value) return
  loading.value = true
  try {
    const resp = await api.get<{ data: Deal[] }>(`/deals/board?${boardQuery()}`)
    deals.value = resp.data
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
    !!temperatureFilter.value ||
    !!createdFilter.value ||
    !!activityFilter.value ||
    !!closeWindowFilter.value ||
    advGroups.value.length > 0
)

function applyFilters(filters: Record<string, any>) {
  search.value = filters.q ?? ''
  ownerFilter.value = Number(filters.owner_id) || 0
  temperatureFilter.value = filters.temperature ?? ''
  createdFilter.value = Number(filters.criado_dias) || 0
  activityFilter.value = Number(filters.sem_atividade_dias) || 0
  closeWindowFilter.value = filters.fechamento ?? ''
  advGroups.value = Array.isArray(filters.af) ? filters.af : []
  if (filters.pipeline_id && pipelines.value.some((p) => p.id === Number(filters.pipeline_id))) {
    pipelineId.value = Number(filters.pipeline_id)
  }
}

function selectTab(key: string) {
  activeTab.value = key
  if (key.startsWith('view-')) {
    const view = savedViews.value.find((v) => v.id === Number(key.replace('view-', '')))
    if (view) applyFilters((view.filters as Record<string, any>) || {})
  } else {
    applyFilters({})
  }
  loadBoard()
}

async function saveCurrentView() {
  const name = prompt('Nome da visualização (ex.: Funil Limpo, Congelados):')
  if (!name?.trim()) return
  try {
    const view = await api.post<SavedView>('/views', {
      entity: 'deals',
      name: name.trim(),
      filters: {
        q: search.value.trim(),
        owner_id: ownerFilter.value,
        temperature: temperatureFilter.value,
        criado_dias: createdFilter.value,
        sem_atividade_dias: activityFilter.value,
        fechamento: closeWindowFilter.value,
        af: advGroups.value,
        pipeline_id: pipelineId.value
      }
    })
    savedViews.value = [...savedViews.value, view]
    activeTab.value = `view-${view.id}`
    toast.push('Visualização salva para toda a equipe')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function deleteView(viewId: number) {
  if (!confirm('Remover esta visualização?')) return
  try {
    await api.delete(`/views/${viewId}`)
    savedViews.value = savedViews.value.filter((v) => v.id !== viewId)
    if (activeTab.value === `view-${viewId}`) selectTab('funil')
    toast.push('Visualização removida')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function exportCSV() {
  try {
    const params = new URLSearchParams({ pipeline_id: String(pipelineId.value) })
    if (search.value.trim()) params.set('q', search.value.trim())
    if (ownerFilter.value) params.set('owner_id', String(ownerFilter.value))
    const resp = await fetch(`/api/v1/deals/export?${params}`, {
      headers: { Authorization: `Bearer ${getToken()}` }
    })
    if (!resp.ok) throw new Error('falha na exportação')
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `negocios-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Drag & drop =====
function onDragStart(deal: Deal, event: DragEvent) {
  dragging.value = deal.id
  event.dataTransfer?.setData('text/plain', String(deal.id))
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}

function onDragEnd() {
  dragging.value = null
  dragOverStage.value = null
}

async function onDrop(stageId: number) {
  const dealId = dragging.value
  dragOverStage.value = null
  dragging.value = null
  if (!dealId) return

  const deal = deals.value.find((d) => d.id === dealId)
  if (!deal || deal.stage_id === stageId) return

  const previousStage = deal.stage_id
  deal.stage_id = stageId
  try {
    const updated = await api.patch<Deal>(`/deals/${dealId}/stage`, {
      stage_id: stageId,
      position: dealsInStage(stageId).length
    })
    deals.value = deals.value.map((d) => (d.id === dealId ? updated : d))
    if (updated.status !== 'aberto') {
      toast.push(`Negócio ${updated.status === 'ganho' ? 'ganho! 🎉' : 'marcado como perdido'}`)
    }
  } catch (e: any) {
    deal.stage_id = previousStage
    toast.error(e.message)
  }
}

async function closeDeal(deal: Deal, won: boolean) {
  try {
    const updated = await api.patch<Deal>(`/deals/${deal.id}/close`, { won })
    deals.value = deals.value.map((d) => (d.id === deal.id ? updated : d))
    toast.push(won ? 'Negócio ganho! 🎉' : 'Negócio marcado como perdido')
  } catch (e: any) {
    toast.error(e.message)
  }
}

function openNew(stageId?: number) {
  form.value = {
    name: '',
    amount: 0,
    stage_id: stageId || openStages.value[0]?.id || 0,
    contact_id: null,
    company_id: null,
    owner_id: null,
    temperature: '',
    close_date: ''
  }
  modalOpen.value = true
}

async function save() {
  saving.value = true
  try {
    await api.post('/deals', {
      ...form.value,
      amount: Number(form.value.amount) || 0,
      pipeline_id: pipelineId.value
    })
    toast.push('Negócio criado')
    modalOpen.value = false
    await loadBoard()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    await loadPipelines()
    await loadBoard()
    const [usersResp, contactsResp, companiesResp, viewsResp] = await Promise.all([
      api.get<User[]>('/users'),
      api.get<Paginated<Contact>>('/contacts?per_page=100'),
      api.get<Paginated<Company>>('/companies?per_page=100'),
      api.get<SavedView[]>('/views?entity=deals')
    ])
    users.value = usersResp
    contacts.value = contactsResp.data ?? []
    companies.value = companiesResp.data ?? []
    savedViews.value = viewsResp ?? []
  } catch (e: any) {
    toast.error(e.message)
    loading.value = false
  }
})
</script>

<template>
  <div class="board-page">
    <div class="page-head board-head">
      <h1>Negócios</h1>
      <div class="toolbar">
        <button class="btn btn-outline" type="button" @click="exportCSV">Exportar</button>
        <button class="btn btn-primary" type="button" @click="openNew()">+ Novo negócio</button>
      </div>
    </div>

    <!-- Abas de visualização -->
    <div class="tabs">
      <span class="tab-wrap">
        <button type="button" class="tab" :class="{ active: activeTab === 'funil' }" @click="selectTab('funil')">
          Funil
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
      <select v-model.number="pipelineId" @change="loadBoard">
        <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <input v-model="search" type="search" placeholder="Pesquisar negócios…" />
      <select v-model.number="ownerFilter" @change="loadBoard">
        <option :value="0">Proprietário do negócio</option>
        <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
      <select v-model="temperatureFilter" @change="loadBoard">
        <option value="">Temperatura</option>
        <option value="quente">🔴 Quente</option>
        <option value="media">🟡 Média</option>
        <option value="fria">🔵 Fria</option>
      </select>
      <select v-model.number="createdFilter" @change="loadBoard">
        <option :value="0">Data de criação</option>
        <option :value="7">Últimos 7 dias</option>
        <option :value="30">Últimos 30 dias</option>
        <option :value="90">Últimos 90 dias</option>
      </select>
      <select v-model.number="activityFilter" @change="loadBoard">
        <option :value="0">Última atividade</option>
        <option :value="7">Sem atividade há 7+ dias</option>
        <option :value="30">Sem atividade há 30+ dias</option>
        <option :value="45">Sem atividade há 45+ dias</option>
      </select>
      <select v-model="closeWindowFilter" @change="loadBoard">
        <option value="">Data de fechamento</option>
        <option value="fecham_mes">Fecham este mês</option>
        <option value="previsao_vencida">Previsão vencida</option>
      </select>
      <button class="btn btn-outline btn-sm adv-btn" type="button" :class="{ on: advCount }" @click="advOpen = true">
        ≡ Filtros avançados
        <span v-if="advCount" class="adv-count">{{ advCount }}</span>
      </button>
      <button v-if="hasActiveFilters" class="btn btn-outline btn-sm save-view" type="button" @click="saveCurrentView">
        ☆ Salvar visualização
      </button>
    </div>

    <div v-if="loading" class="muted board-loading">Carregando negócios…</div>

    <div v-else class="board">
      <div
        v-for="stage in openStages"
        :key="stage.id"
        class="column"
        :class="{ 'drag-over': dragOverStage === stage.id }"
        @dragover.prevent="dragOverStage = stage.id"
        @dragleave="dragOverStage === stage.id && (dragOverStage = null)"
        @drop.prevent="onDrop(stage.id)"
      >
        <header>
          <span class="stage-name">{{ stage.name }} <span class="stage-count">{{ dealsInStage(stage.id).length }}</span></span>
        </header>

        <div class="cards">
          <article
            v-for="deal in dealsInStage(stage.id)"
            :key="deal.id"
            class="deal-card"
            :class="{ dragging: dragging === deal.id }"
            draggable="true"
            @dragstart="onDragStart(deal, $event)"
            @dragend="onDragEnd"
            @click="router.push(`/negocios/${deal.id}`)"
          >
            <strong>{{ deal.name }}</strong>
            <span class="muted small" v-if="deal.company_name || deal.contact_name">
              {{ deal.company_name || deal.contact_name }}
            </span>
            <span class="amount">{{ formatMoney(deal.amount) }}</span>
            <div class="card-meta">
              <span v-if="deal.temperature" class="temp">
                <span class="temp-dot" :style="{ background: temperatureDot[deal.temperature] }"></span>
                {{ temperatureLabels[deal.temperature] }}
              </span>
              <span class="muted small" v-if="deal.owner_name">{{ deal.owner_name }}</span>
            </div>
            <span class="muted small activity" v-if="deal.last_activity_at">
              Atividade {{ relativeDate(deal.last_activity_at) }}
            </span>
            <div class="deal-actions" @click.stop>
              <button type="button" class="win" title="Marcar como ganho" @click="closeDeal(deal, true)">✓</button>
              <button type="button" class="lose" title="Marcar como perdido" @click="closeDeal(deal, false)">✕</button>
            </div>
          </article>

          <button class="add-in-column" type="button" @click="openNew(stage.id)">+ Adicionar</button>
        </div>

        <footer class="column-foot">
          <span>{{ formatMoney(stageTotal(stage.id)) }} · Valor total</span>
          <span>{{ formatMoney(stageWeighted(stage)) }} ({{ stage.probability }}%) · Valor ponderado</span>
        </footer>
      </div>

      <div
        v-for="stage in closedStages"
        :key="stage.id"
        class="column closed"
        :class="[{ 'drag-over': dragOverStage === stage.id }, stage.is_won ? 'won' : 'lost']"
        @dragover.prevent="dragOverStage = stage.id"
        @dragleave="dragOverStage === stage.id && (dragOverStage = null)"
        @drop.prevent="onDrop(stage.id)"
      >
        <header>
          <span class="stage-name">
            {{ stage.is_won ? '✓' : '✕' }} {{ stage.name }}
            <span class="stage-count">{{ closedInStage(stage.id).length }}</span>
          </span>
        </header>
        <div class="cards">
          <article
            v-for="deal in closedInStage(stage.id)"
            :key="deal.id"
            class="deal-card closed-card"
            @click="router.push(`/negocios/${deal.id}`)"
          >
            <strong>{{ deal.name }}</strong>
            <span class="amount">{{ formatMoney(deal.amount) }}</span>
          </article>
        </div>
        <footer class="column-foot">
          <span>{{ formatMoney(closedInStage(stage.id).reduce((s, d) => s + d.amount, 0)) }} · últimos 30 dias</span>
        </footer>
      </div>
    </div>

    <div class="board-bottom" v-if="!loading">
      <span class="badge gray">{{ totalDeals }} negócio(s) no quadro</span>
    </div>

    <AdvancedFilters
      :open="advOpen"
      :fields="advFields"
      :model-value="advGroups"
      @close="advOpen = false"
      @apply="applyAdvanced"
    />

    <ModalDialog title="Novo negócio" :open="modalOpen" wide @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.name" required placeholder="ex.: Adquirência - Loja X" />
          </div>
          <div class="field">
            <label>Valor (R$)</label>
            <input v-model.number="form.amount" type="number" min="0" step="0.01" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Etapa</label>
            <select v-model.number="form.stage_id">
              <option v-for="s in openStages" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Temperatura do deal</label>
            <select v-model="form.temperature">
              <option value="">—</option>
              <option value="quente">🔴 Quente</option>
              <option value="media">🟡 Média</option>
              <option value="fria">🔵 Fria</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Contato</label>
            <select v-model="form.contact_id">
              <option :value="null">Sem contato</option>
              <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
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
        <div class="form-row">
          <div class="field">
            <label>Dono</label>
            <select v-model="form.owner_id">
              <option :value="null">Eu</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Previsão de fechamento</label>
            <input v-model="form.close_date" type="date" />
          </div>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar negócio' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.board-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 24px 28px 12px;
}

.board-head {
  margin-bottom: 12px;
}

.tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 12px;
  overflow-x: auto;
  flex-shrink: 0;
}

.tab-wrap {
  display: inline-flex;
  align-items: center;
}

.tab {
  padding: 8px 14px;
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
  flex-shrink: 0;
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

.board-loading {
  padding: 24px 0;
}

.board {
  display: flex;
  gap: 14px;
  overflow-x: auto;
  flex: 1;
  padding-bottom: 8px;
  align-items: stretch;
}

.column {
  width: 270px;
  flex-shrink: 0;
  background: rgba(155, 82, 223, 0.06);
  border: 1px solid transparent;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  max-height: 100%;
  transition: border-color 0.15s, background 0.15s;
}

.column.drag-over {
  border-color: var(--fix-purple);
  background: var(--fix-purple-tint);
}

.column header {
  padding: 12px 14px 8px;
}

.stage-name {
  font-weight: 600;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.stage-count {
  background: var(--fix-surface);
  border-radius: 999px;
  padding: 1px 8px;
  font-size: 11px;
  font-weight: 700;
  color: var(--fix-text-2);
}

.cards {
  padding: 4px 10px 10px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
}

.deal-card {
  background: var(--fix-surface);
  border-radius: 10px;
  box-shadow: var(--shadow);
  padding: 12px;
  cursor: grab;
  display: flex;
  flex-direction: column;
  gap: 3px;
  position: relative;
  border-left: 3px solid var(--fix-purple);
}

.deal-card.dragging {
  opacity: 0.4;
}

.deal-card strong {
  font-size: 14px;
  padding-right: 44px;
}

.amount {
  font-size: 13px;
  font-weight: 600;
  color: var(--fix-purple-dark);
}

.small {
  font-size: 12px;
}

.card-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 2px;
}

.temp {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--fix-text-2);
}

.temp-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.activity {
  margin-top: 2px;
}

.deal-actions {
  position: absolute;
  top: 8px;
  right: 8px;
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.15s;
}

.deal-card:hover .deal-actions {
  opacity: 1;
}

.deal-actions button {
  width: 22px;
  height: 22px;
  border-radius: 6px;
  border: none;
  cursor: pointer;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.deal-actions .win {
  background: var(--fix-green-tint);
  color: var(--fix-green);
}

.deal-actions .lose {
  background: var(--fix-red-tint);
  color: var(--fix-red);
}

.add-in-column {
  border: 1px dashed var(--fix-border);
  background: none;
  border-radius: 8px;
  padding: 8px;
  color: var(--fix-text-3);
  cursor: pointer;
  font-size: 13px;
}

.add-in-column:hover {
  border-color: var(--fix-purple);
  color: var(--fix-purple);
}

.column-foot {
  border-top: 1px solid var(--fix-border);
  padding: 8px 14px;
  font-size: 11px;
  color: var(--fix-text-3);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.column.closed {
  width: 220px;
}

.column.won {
  background: var(--fix-green-tint);
}

.column.lost {
  background: var(--fix-red-tint);
}

.column.won .stage-name {
  color: var(--fix-green);
}

.column.lost .stage-name {
  color: var(--fix-red);
}

.closed-card {
  border-left-color: var(--fix-border);
  cursor: pointer;
  opacity: 0.85;
}

.board-bottom {
  padding: 10px 0 4px;
  flex-shrink: 0;
}
</style>
