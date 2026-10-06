<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatMoney } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import type { GoalPoint, Pipeline, TrackedGoal, User } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const goals = ref<TrackedGoal[]>([])
const users = ref<User[]>([])
const pipelines = ref<Pipeline[]>([])
const loading = ref(true)
const saving = ref(false)
const scope = ref<'ativo' | 'passado'>('ativo')

// Meta aberta com a série mensal.
const detail = ref<{ goal: TrackedGoal; points: GoalPoint[] } | null>(null)

// Modal de criação em dois passos.
const pickerOpen = ref(false)
const editing = ref<TrackedGoal | null>(null)

const canManage = computed(() => auth.can('goals.manage'))

const kindHints: Record<string, string> = {
  ganho: 'Com base no número ou valor de negócios ganhos',
  adicionado: 'Com base no número ou valor de novos negócios',
  progresso: 'Com base no número ou valor de negócios entrando em determinada etapa',
  atividade: 'Com base no número de atividades realizadas'
}

const kindLabels: Record<string, string> = {
  ganho: 'Negócios ganhos',
  adicionado: 'Negócios adicionados',
  progresso: 'Negócios em progresso',
  atividade: 'Atividades realizadas'
}

const activityKinds = [
  { value: '', label: 'Qualquer atividade' },
  { value: 'ligacao', label: 'Ligações' },
  { value: 'reuniao', label: 'Reuniões' },
  { value: 'email', label: 'E-mails' },
  { value: 'nota', label: 'Observações' }
]

const visible = computed(() =>
  goals.value.filter((g) => (scope.value === 'passado' ? g.finished : !g.finished))
)

const stagesOfPipeline = computed(() => {
  const pid = editing.value?.pipeline_id
  const pipe = pipelines.value.find((p) => p.id === pid) ?? pipelines.value[0]
  return pipe?.stages.filter((s) => !s.is_won && !s.is_lost) ?? []
})

/** Nome exibido da meta, como o Insights monta: tipo + responsável. */
function goalName(g: TrackedGoal): string {
  const who = g.user_name || 'Toda a equipe'
  return `${kindLabels[g.kind] ?? g.kind} · ${who}`
}

function fmtValue(g: TrackedGoal, value: number): string {
  return g.metric === 'valor' ? formatMoney(value) : String(Math.round(value))
}

function monthLabel(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return new Date(y, m - 1, 1).toLocaleDateString('pt-BR', { month: 'short', year: '2-digit' })
}

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{ data: TrackedGoal[] }>('/tracked-goals')
    goals.value = resp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadRefs() {
  try {
    users.value = await api.get<User[]>('/users')
    const resp = await api.get<{ data: Pipeline[] }>('/pipelines')
    pipelines.value = resp.data ?? []
  } catch {
    /* o formulário funciona sem as referências */
  }
}

function pickKind(kind: string) {
  pickerOpen.value = false
  editing.value = {
    id: 0,
    kind,
    metric: kind === 'atividade' ? 'numero' : 'valor',
    user_id: null,
    pipeline_id: null,
    stage_id: null,
    activity_kind: '',
    amount: 0,
    start_period: new Date().toISOString().slice(0, 7),
    end_period: '',
    created_by: null,
    created_at: '',
    updated_at: '',
    finished: false,
    current_value: 0,
    attainment: 0
  }
}

function edit(g: TrackedGoal) {
  editing.value = JSON.parse(JSON.stringify(g))
}

async function save() {
  if (!editing.value) return
  saving.value = true
  try {
    if (editing.value.id) {
      await api.put(`/tracked-goals/${editing.value.id}`, editing.value)
    } else {
      await api.post('/tracked-goals', editing.value)
    }
    toast.push('Meta salva')
    editing.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(g: TrackedGoal) {
  if (!confirm(`Excluir a meta "${goalName(g)}"?`)) return
  try {
    await api.delete(`/tracked-goals/${g.id}`)
    toast.push('Meta removida')
    detail.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function open(g: TrackedGoal) {
  try {
    detail.value = await api.get<{ goal: TrackedGoal; points: GoalPoint[] }>(`/tracked-goals/${g.id}`)
  } catch (e: any) {
    toast.error(e.message)
  }
}

/** Altura da barra do mês em % da meta (limitada para a barra não estourar). */
function barHeight(point: GoalPoint): number {
  if (!point.target) return 0
  return Math.min(100, (point.actual / point.target) * 100)
}

onMounted(async () => {
  await loadRefs()
  await load()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Metas</h1>
        <p class="muted" style="margin: 4px 0 0">
          Defina o alvo por mês e acompanhe o realizado, por pessoa ou para a equipe inteira.
        </p>
      </div>
      <div class="head-actions">
        <div class="tabs">
          <button class="btn" :class="{ 'btn-primary': scope === 'ativo' }" type="button" @click="scope = 'ativo'">
            Ativas
          </button>
          <button class="btn" :class="{ 'btn-primary': scope === 'passado' }" type="button" @click="scope = 'passado'">
            Passadas
          </button>
        </div>
        <button v-if="canManage" class="btn btn-primary" type="button" @click="pickerOpen = true">
          Criar meta
        </button>
      </div>
    </div>

    <p v-if="loading" class="muted">Carregando metas…</p>

    <div v-else-if="!visible.length" class="card empty">
      <h2>{{ scope === 'ativo' ? 'Nenhuma meta ativa' : 'Nenhuma meta passada' }}</h2>
      <p class="muted">
        Uma meta acompanha negócios ganhos, adicionados, em progresso ou atividades — em valor ou
        quantidade, por pessoa ou para a equipe.
      </p>
    </div>

    <div v-else class="grid">
      <button v-for="g in visible" :key="g.id" type="button" class="card goal" @click="open(g)">
        <div class="goal-head">
          <strong>{{ goalName(g) }}</strong>
          <span v-if="g.pipeline_name" class="muted small">{{ g.pipeline_name }}</span>
        </div>
        <div class="goal-numbers">
          <span class="big">{{ fmtValue(g, g.current_value) }}</span>
          <span class="muted">de {{ fmtValue(g, g.amount) }} neste mês</span>
        </div>
        <div class="track">
          <div
            class="fill"
            :class="{ ok: g.attainment >= 100 }"
            :style="{ width: `${Math.min(100, g.attainment)}%` }"
          ></div>
        </div>
        <span class="muted small">
          {{ g.attainment.toFixed(0) }}% ·
          {{ g.start_period }}{{ g.end_period ? ` – ${g.end_period}` : ' em diante' }}
        </span>
      </button>
    </div>

    <!-- ===== Passo 1: escolher o tipo ===== -->
    <ModalDialog title="Adicionar meta 1/2" :open="pickerOpen" @close="pickerOpen = false">
      <p class="muted" style="margin-top: 0">Escolha o tipo de meta</p>
      <div class="kinds">
        <button
          v-for="(label, kind) in kindLabels"
          :key="kind"
          type="button"
          class="kind-item"
          @click="pickKind(kind as string)"
        >
          <strong>{{ label }}</strong>
          <span class="muted">{{ kindHints[kind] }}</span>
        </button>
      </div>
    </ModalDialog>

    <!-- ===== Passo 2: configurar ===== -->
    <ModalDialog
      :title="editing?.id ? 'Editar meta' : `Adicionar meta 2/2 · ${kindLabels[editing?.kind ?? '']}`"
      :open="!!editing"
      @close="editing = null"
    >
      <template v-if="editing">
        <div class="field">
          <label>Responsável</label>
          <select v-model="editing.user_id">
            <option :value="null">Toda a equipe</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>

        <div v-if="editing.kind !== 'atividade'" class="field">
          <label>Funil de vendas</label>
          <select v-model="editing.pipeline_id">
            <option :value="null">Todos os funis</option>
            <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>

        <div v-if="editing.kind === 'progresso'" class="field">
          <label>Etapa observada *</label>
          <select v-model="editing.stage_id">
            <option :value="null">Selecione…</option>
            <option v-for="s in stagesOfPipeline" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </div>

        <div v-if="editing.kind === 'atividade'" class="field">
          <label>Tipo de atividade</label>
          <select v-model="editing.activity_kind">
            <option v-for="a in activityKinds" :key="a.value" :value="a.value">{{ a.label }}</option>
          </select>
        </div>

        <div v-if="editing.kind !== 'atividade'" class="field">
          <label>Métrica de rastreamento</label>
          <div class="radio-row">
            <label><input type="radio" value="valor" v-model="editing.metric" /> Valor</label>
            <label><input type="radio" value="numero" v-model="editing.metric" /> Número</label>
          </div>
        </div>

        <div class="field">
          <label>{{ editing.metric === 'valor' ? 'Valor por mês (R$)' : 'Quantidade por mês' }} *</label>
          <input v-model.number="editing.amount" type="number" min="1" step="100" />
        </div>

        <div class="form-row">
          <div class="field">
            <label>Início *</label>
            <input v-model="editing.start_period" type="month" />
          </div>
          <div class="field">
            <label>Término</label>
            <input v-model="editing.end_period" type="month" />
          </div>
        </div>
        <p class="muted hint">Sem término, a meta segue valendo mês após mês.</p>
      </template>

      <template #footer>
        <button class="btn" type="button" @click="editing = null">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="saving" @click="save">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
      </template>
    </ModalDialog>

    <!-- ===== Detalhe: realizado × meta, mês a mês ===== -->
    <ModalDialog :title="detail ? goalName(detail.goal) : ''" :open="!!detail" wide @close="detail = null">
      <template v-if="detail">
        <dl class="detail-list">
          <dt>Responsável</dt>
          <dd>{{ detail.goal.user_name || 'Toda a equipe' }}</dd>
          <dt>Frequência</dt>
          <dd>Mensal</dd>
          <dt>Duração</dt>
          <dd>{{ detail.goal.start_period }}{{ detail.goal.end_period ? ` – ${detail.goal.end_period}` : ' em diante' }}</dd>
          <dt>Alvo por mês</dt>
          <dd><strong>{{ fmtValue(detail.goal, detail.goal.amount) }}</strong></dd>
        </dl>

        <div class="chart">
          <div v-for="p in detail.points" :key="p.month" class="month">
            <span class="month-value" :class="{ ok: p.attainment >= 100 }">
              {{ fmtValue(detail.goal, p.actual) }}
            </span>
            <div class="month-track">
              <div class="target-line"></div>
              <div
                class="month-fill"
                :class="{ ok: p.attainment >= 100 }"
                :style="{ height: `${barHeight(p)}%` }"
              ></div>
            </div>
            <span class="month-label">{{ monthLabel(p.month) }}</span>
            <span class="muted small">{{ p.attainment.toFixed(0) }}%</span>
          </div>
        </div>
        <p class="muted hint">A linha pontilhada é o alvo de {{ fmtValue(detail.goal, detail.goal.amount) }} por mês.</p>

        <div v-if="canManage" class="detail-actions">
          <button class="btn" type="button" @click="edit(detail.goal); detail = null">Editar</button>
          <button class="btn danger" type="button" @click="remove(detail.goal)">Excluir</button>
        </div>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.head-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.tabs {
  display: flex;
  gap: 6px;
}

.empty {
  text-align: center;
  padding: 36px;
}

.empty h2 {
  font-size: 16px;
  margin-bottom: 6px;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.goal {
  text-align: left;
  cursor: pointer;
  border: 1px solid var(--fix-border);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.goal:hover {
  border-color: var(--fix-purple);
}

.goal-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.goal-numbers {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.big {
  font-size: 20px;
  font-weight: 600;
}

.track {
  height: 8px;
  background: var(--fix-bg);
  border-radius: 4px;
  overflow: hidden;
}

.fill {
  height: 100%;
  background: var(--fix-purple);
  border-radius: 4px;
}

.fill.ok {
  background: var(--fix-green);
}

.small {
  font-size: 12px;
}

.kinds {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.kind-item {
  text-align: left;
  background: var(--fix-surface);
  border: 1px solid var(--fix-border);
  border-radius: 10px;
  padding: 12px 14px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 14px;
  color: var(--fix-text);
}

.kind-item:hover {
  border-color: var(--fix-purple);
  background: var(--fix-purple-tint);
}

.kind-item .muted {
  font-size: 12px;
}

.radio-row {
  display: flex;
  gap: 18px;
  padding: 6px 0;
}

.radio-row label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  cursor: pointer;
}

.hint {
  font-size: 12px;
  margin: 4px 0 0;
}

.detail-list {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 6px 12px;
  margin: 0 0 18px;
  font-size: 14px;
}

.detail-list dt {
  color: var(--fix-text-3);
}

.detail-list dd {
  margin: 0;
}

.chart {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  overflow-x: auto;
  padding: 10px 0 4px;
}

.month {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  min-width: 64px;
}

.month-value {
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.month-value.ok {
  color: var(--fix-green);
  font-weight: 600;
}

.month-track {
  position: relative;
  width: 100%;
  height: 120px;
  background: var(--fix-bg);
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  align-items: flex-end;
}

.target-line {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  border-top: 2px dashed var(--fix-purple);
  opacity: 0.5;
}

.month-fill {
  width: 100%;
  background: var(--fix-purple);
  transition: height 0.25s;
}

.month-fill.ok {
  background: var(--fix-green);
}

.month-label {
  font-size: 11px;
  color: var(--fix-text-3);
  text-transform: capitalize;
}

.detail-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 16px;
}

.danger {
  color: var(--fix-red);
}
</style>
