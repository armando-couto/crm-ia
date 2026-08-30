<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatMoney } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, Deal, Paginated, Pipeline, User } from '../types'

const router = useRouter()
const toast = useToastStore()

const pipelines = ref<Pipeline[]>([])
const pipelineId = ref(0)
const deals = ref<Deal[]>([])
const loading = ref(true)
const dragging = ref<number | null>(null)
const dragOverStage = ref<number | null>(null)

const users = ref<User[]>([])
const contacts = ref<Contact[]>([])
const companies = ref<Company[]>([])

const modalOpen = ref(false)
const saving = ref(false)
const form = ref({
  name: '',
  amount: 0,
  stage_id: 0,
  contact_id: null as number | null,
  company_id: null as number | null,
  owner_id: null as number | null,
  close_date: ''
})

const pipeline = computed(() => pipelines.value.find((p) => p.id === pipelineId.value))
const openStages = computed(() => pipeline.value?.stages.filter((s) => !s.is_won && !s.is_lost) ?? [])
const closedStages = computed(() => pipeline.value?.stages.filter((s) => s.is_won || s.is_lost) ?? [])

function dealsInStage(stageId: number): Deal[] {
  return deals.value.filter((d) => d.stage_id === stageId)
}

function stageTotal(stageId: number): number {
  return dealsInStage(stageId).reduce((sum, d) => sum + d.amount, 0)
}

async function loadPipelines() {
  pipelines.value = await api.get<Pipeline[]>('/pipelines')
  if (!pipelineId.value && pipelines.value.length) {
    pipelineId.value = pipelines.value[0].id
  }
}

async function loadBoard() {
  if (!pipelineId.value) return
  loading.value = true
  try {
    const resp = await api.get<{ data: Deal[] }>(`/deals/board?pipeline_id=${pipelineId.value}`)
    deals.value = resp.data
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

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
  deal.stage_id = stageId // atualização otimista
  try {
    const updated = await api.patch<Deal>(`/deals/${dealId}/stage`, {
      stage_id: stageId,
      position: dealsInStage(stageId).length
    })
    if (updated.status !== 'aberto') {
      deals.value = deals.value.filter((d) => d.id !== dealId)
      toast.push(`Negócio ${updated.status === 'ganho' ? 'ganho! 🎉' : 'marcado como perdido'}`)
    } else {
      deals.value = deals.value.map((d) => (d.id === dealId ? updated : d))
    }
  } catch (e: any) {
    deal.stage_id = previousStage
    toast.error(e.message)
  }
}

async function closeDeal(deal: Deal, won: boolean) {
  try {
    await api.patch(`/deals/${deal.id}/close`, { won })
    deals.value = deals.value.filter((d) => d.id !== deal.id)
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
    const [usersResp, contactsResp, companiesResp] = await Promise.all([
      api.get<User[]>('/users'),
      api.get<Paginated<Contact>>('/contacts?per_page=100'),
      api.get<Paginated<Company>>('/companies?per_page=100')
    ])
    users.value = usersResp
    contacts.value = contactsResp.data
    companies.value = companiesResp.data
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
        <select v-model.number="pipelineId" @change="loadBoard">
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <button class="btn btn-primary" type="button" @click="openNew()">+ Novo negócio</button>
      </div>
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
          <span class="stage-name">{{ stage.name }}</span>
          <span class="stage-meta">{{ dealsInStage(stage.id).length }} · {{ formatMoney(stageTotal(stage.id)) }}</span>
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
            <span class="amount">{{ formatMoney(deal.amount) }}</span>
            <span class="muted small" v-if="deal.contact_name || deal.company_name">
              {{ deal.contact_name || deal.company_name }}
            </span>
            <div class="deal-actions" @click.stop>
              <button type="button" class="win" title="Marcar como ganho" @click="closeDeal(deal, true)">✓</button>
              <button type="button" class="lose" title="Marcar como perdido" @click="closeDeal(deal, false)">✕</button>
            </div>
          </article>

          <button class="add-in-column" type="button" @click="openNew(stage.id)">+ Adicionar</button>
        </div>
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
          <span class="stage-name">{{ stage.is_won ? '✓' : '✕' }} {{ stage.name }}</span>
        </header>
        <p class="drop-hint">Arraste aqui para fechar</p>
      </div>
    </div>

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
            <label>Previsão de fechamento</label>
            <input v-model="form.close_date" type="date" />
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
        <div class="field">
          <label>Dono</label>
          <select v-model="form.owner_id">
            <option :value="null">Eu</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
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
  margin-bottom: 16px;
}

.board-loading {
  padding: 24px 0;
}

.board {
  display: flex;
  gap: 14px;
  overflow-x: auto;
  flex: 1;
  padding-bottom: 16px;
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
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stage-name {
  font-weight: 600;
  font-size: 14px;
}

.stage-meta {
  font-size: 12px;
  color: var(--fix-text-3);
}

.cards {
  padding: 4px 10px 10px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
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

.column.closed {
  width: 150px;
  align-items: center;
  justify-content: flex-start;
  text-align: center;
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

.drop-hint {
  font-size: 12px;
  color: var(--fix-text-3);
  padding: 0 12px;
}
</style>
