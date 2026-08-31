<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatMoney } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import ReportChart from '../components/ReportChart.vue'
import type { Report, ReportCatalogEntry, ReportResult, User } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const reports = ref<Report[]>([])
const catalog = ref<ReportCatalogEntry[]>([])
const users = ref<User[]>([])
const results = ref<Record<number, ReportResult>>({})
const loading = ref(true)
const saving = ref(false)

const editing = ref<Report | null>(null)
const preview = ref<ReportResult | null>(null)

// Modal de criação em dois passos: escolher a base e o tipo de relatório.
const pickerOpen = ref(false)
const pickerEntity = ref('negocios')
const kinds = ref<Record<string, string>>({})

// Descrição de cada tipo, como no modal do Insights.
const kindHints: Record<string, string> = {
  agregado: 'Quantas/quanto, agrupado como você escolher',
  conversao: 'Qual é a taxa de conversão entre as etapas do funil?',
  duracao: 'Quantos dias o negócio passa em cada etapa?',
  progresso: 'Criados, ganhos e perdidos mês a mês'
}

/** Os tipos prontos só existem para negócios; o resto usa o agregado. */
function kindsFor(entity: string): string[] {
  if (entity === 'negocios') return ['agregado', 'conversao', 'duracao', 'progresso']
  return ['agregado']
}

function pickKind(kind: string) {
  pickerOpen.value = false
  const first = catalog.value.find((c) => c.key === pickerEntity.value)
  editing.value = {
    id: 0,
    kind,
    name: '',
    description: '',
    entity: pickerEntity.value,
    metric: Object.keys(first?.metrics ?? { contagem: '' })[0],
    dimension: Object.keys(first?.dimensions ?? { dono: '' })[0],
    filters: { days: kind === 'progresso' ? 365 : 90, owner_id: 0, status: '', pipeline_id: 0 },
    chart: kind === 'conversao' ? 'conversao' : kind === 'progresso' ? 'linha' : 'barras',
    shared: true,
    position: 0,
    created_by: null,
    created_at: '',
    updated_at: ''
  }
  preview.value = null
  runPreview()
}

const isAggregate = computed(() => (editing.value?.kind ?? 'agregado') === 'agregado')

const charts = [
  { value: 'barras', label: 'Barras' },
  { value: 'linha', label: 'Linha' },
  { value: 'pizza', label: 'Pizza' },
  { value: 'tabela', label: 'Tabela' }
]

const canManage = computed(() => auth.can('reports.manage'))

const currentEntity = computed(() =>
  catalog.value.find((c) => c.key === editing.value?.entity)
)

function totalOf(result: ReportResult): string {
  return result.is_money ? formatMoney(result.total) : String(Math.round(result.total))
}

async function load() {
  loading.value = true
  try {
    const [resp, userList] = await Promise.all([
      api.get<{ data: Report[]; catalog: ReportCatalogEntry[]; kinds: Record<string, string> }>('/reports'),
      api.get<User[]>('/users')
    ])
    reports.value = resp.data ?? []
    catalog.value = resp.catalog ?? []
    kinds.value = resp.kinds ?? {}
    users.value = userList
    await Promise.all(reports.value.map(run))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function run(report: Report) {
  try {
    const resp = await api.get<{ result: ReportResult }>(`/reports/${report.id}/run`)
    results.value[report.id] = resp.result
  } catch {
    // Um relatório quebrado não pode derrubar a página inteira.
    delete results.value[report.id]
  }
}

function novo() {
  pickerEntity.value = 'negocios'
  pickerOpen.value = true
}

function edit(report: Report) {
  editing.value = JSON.parse(JSON.stringify(report))
  preview.value = null
  runPreview()
}

// Trocar de entidade invalida métrica e agrupamento anteriores.
watch(
  () => editing.value?.entity,
  (entity) => {
    if (!entity || !editing.value) return
    const def = catalog.value.find((c) => c.key === entity)
    if (!def) return
    if (!def.metrics[editing.value.metric]) {
      editing.value.metric = Object.keys(def.metrics)[0]
    }
    if (!def.dimensions[editing.value.dimension]) {
      editing.value.dimension = Object.keys(def.dimensions)[0]
    }
  }
)

async function runPreview() {
  if (!editing.value) return
  try {
    const resp = await api.post<{ result: ReportResult }>('/reports/preview', editing.value)
    preview.value = resp.result
  } catch (e: any) {
    preview.value = null
    toast.error(e.message)
  }
}

async function save() {
  if (!editing.value) return
  saving.value = true
  try {
    if (editing.value.id) {
      await api.put(`/reports/${editing.value.id}`, editing.value)
    } else {
      await api.post('/reports', editing.value)
    }
    toast.push('Relatório salvo')
    editing.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(report: Report) {
  if (!confirm(`Excluir o relatório "${report.name}"?`)) return
  try {
    await api.delete(`/reports/${report.id}`)
    toast.push('Relatório removido')
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
        <h1>Relatórios</h1>
        <p class="muted" style="margin: 4px 0 0">
          Escolha o que medir, como agrupar e o formato. Os relatórios compartilhados aparecem para a equipe.
        </p>
      </div>
      <button v-if="canManage" class="btn btn-primary" type="button" @click="novo">Novo relatório</button>
    </div>

    <p v-if="loading" class="muted">Carregando relatórios…</p>

    <div v-else-if="!reports.length" class="card empty">
      <h2>Nenhum relatório ainda</h2>
      <p class="muted">
        Exemplos: valor ganho por vendedor, negócios por etapa, contatos por origem,
        tickets por prioridade, tarefas atrasadas por dono.
      </p>
    </div>

    <div v-else class="grid">
      <div v-for="r in reports" :key="r.id" class="card report">
        <div class="report-head">
          <div>
            <strong>{{ r.name }}</strong>
            <span v-if="!r.shared" class="badge gray">só meu</span>
            <p v-if="r.description" class="muted desc">{{ r.description }}</p>
          </div>
          <div v-if="canManage" class="report-actions">
            <button class="btn btn-sm" type="button" @click="edit(r)">Editar</button>
            <button class="btn btn-sm danger" type="button" @click="remove(r)">Excluir</button>
          </div>
        </div>

        <template v-if="results[r.id]">
          <p class="total">
            {{ totalOf(results[r.id]) }}
            <span class="muted">{{ results[r.id].metric_label }} · por {{ results[r.id].dimension_label.toLowerCase() }}</span>
          </p>
          <ReportChart :result="results[r.id]" :chart="r.chart" />
        </template>
        <p v-else class="muted">Não foi possível carregar este relatório.</p>
      </div>
    </div>

    <!-- ===== Escolher base e tipo (dois passos, como no Insights) ===== -->
    <ModalDialog title="Adicionar novo relatório" :open="pickerOpen" wide @close="pickerOpen = false">
      <div class="picker">
        <div class="picker-col">
          <h3>Escolha a base</h3>
          <button
            v-for="c in catalog"
            :key="c.key"
            type="button"
            class="picker-item"
            :class="{ active: pickerEntity === c.key }"
            @click="pickerEntity = c.key"
          >
            {{ c.label }}
          </button>
        </div>
        <div class="picker-col">
          <h3>Escolha o tipo de relatório</h3>
          <button
            v-for="k in kindsFor(pickerEntity)"
            :key="k"
            type="button"
            class="picker-item kind"
            @click="pickKind(k)"
          >
            <strong>{{ kinds[k] ?? k }}</strong>
            <span class="muted">{{ kindHints[k] }}</span>
          </button>
        </div>
      </div>
    </ModalDialog>

    <!-- ===== Construtor ===== -->
    <ModalDialog
      :title="editing?.id ? 'Editar relatório' : `Novo relatório · ${kinds[editing?.kind ?? 'agregado'] ?? ''}`"
      :open="!!editing"
      wide
      @close="editing = null"
    >
      <template v-if="editing">
        <div class="field">
          <label>Nome *</label>
          <input v-model="editing.name" placeholder="Receita ganha por vendedor" @blur="runPreview" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <input v-model="editing.description" placeholder="Para que serve este relatório" />
        </div>

        <template v-if="isAggregate">
          <div class="form-row">
            <div class="field">
              <label>Sobre o quê</label>
              <select v-model="editing.entity" @change="runPreview">
                <option v-for="c in catalog" :key="c.key" :value="c.key">{{ c.label }}</option>
              </select>
            </div>
            <div class="field">
              <label>Medir</label>
              <select v-model="editing.metric" @change="runPreview">
                <option v-for="(label, key) in currentEntity?.metrics ?? {}" :key="key" :value="key">
                  {{ label }}
                </option>
              </select>
            </div>
          </div>

          <div class="form-row">
            <div class="field">
              <label>Agrupar por</label>
              <select v-model="editing.dimension" @change="runPreview">
                <option v-for="(label, key) in currentEntity?.dimensions ?? {}" :key="key" :value="key">
                  {{ label }}
                </option>
              </select>
            </div>
            <div class="field">
              <label>Formato</label>
              <select v-model="editing.chart">
                <option v-for="c in charts" :key="c.value" :value="c.value">{{ c.label }}</option>
              </select>
            </div>
          </div>
        </template>
        <p v-else class="muted kind-hint">{{ kindHints[editing.kind ?? 'agregado'] }}</p>

        <h3 class="section">Filtros</h3>
        <div class="form-row">
          <div class="field">
            <label>Período</label>
            <select v-model.number="editing.filters.days" @change="runPreview">
              <option :value="0">Desde sempre</option>
              <option :value="7">Últimos 7 dias</option>
              <option :value="30">Últimos 30 dias</option>
              <option :value="90">Últimos 90 dias</option>
              <option :value="365">Último ano</option>
            </select>
          </div>
          <div v-if="isAggregate" class="field">
            <label>Dono</label>
            <select v-model.number="editing.filters.owner_id" @change="runPreview">
              <option :value="0">Todos</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Visibilidade</label>
          <select v-model="editing.shared">
            <option :value="true">Compartilhado com a equipe</option>
            <option :value="false">Só para mim</option>
          </select>
        </div>

        <h3 class="section">Prévia</h3>
        <div class="preview">
          <ReportChart v-if="preview" :result="preview" :chart="editing.chart" />
          <p v-else class="muted">Ajuste as opções acima para ver a prévia.</p>
        </div>
      </template>

      <template #footer>
        <button class="btn" type="button" @click="editing = null">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="saving" @click="save">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.picker {
  display: grid;
  grid-template-columns: 200px 1fr;
  gap: 20px;
}

.picker h3 {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--fix-text-3);
  margin-bottom: 10px;
}

.picker-col {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.picker-item {
  text-align: left;
  background: var(--fix-surface);
  border: 1px solid var(--fix-border);
  border-radius: 10px;
  padding: 10px 14px;
  cursor: pointer;
  font-size: 14px;
  color: var(--fix-text);
}

.picker-item:hover {
  border-color: var(--fix-purple);
}

.picker-item.active {
  border-color: var(--fix-purple);
  background: var(--fix-purple-tint);
  font-weight: 500;
}

.picker-item.kind {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.picker-item.kind .muted {
  font-size: 12px;
  font-weight: 400;
}

.kind-hint {
  margin: 4px 0 12px;
  font-size: 13px;
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
  grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
  gap: 16px;
}

.report-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 10px;
}

.report-head .badge {
  margin-left: 8px;
}

.report-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

.danger {
  color: var(--fix-red);
}

.desc {
  margin: 4px 0 0;
  font-size: 13px;
}

.total {
  font-size: 22px;
  font-weight: 600;
  margin: 0 0 14px;
}

.total .muted {
  font-size: 12px;
  font-weight: 400;
  display: block;
  margin-top: 2px;
}

.section {
  font-size: 14px;
  margin: 20px 0 12px;
  padding-top: 14px;
  border-top: 1px solid var(--fix-border);
}

.preview {
  background: var(--fix-bg);
  border-radius: 10px;
  padding: 14px;
}
</style>
