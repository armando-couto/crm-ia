<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatDate, formatDateTime, formatMoney, relativeDate } from '../format'
import ReportChart from '../components/ReportChart.vue'
import type {
  Company,
  Deal,
  Meeting,
  Panel,
  Paginated,
  Task,
  Workspace,
  WorkspaceItem
} from '../types'

const auth = useAuthStore()
const toast = useToastStore()

type TabKey = 'resumo' | 'empresas' | 'negocios' | 'tarefas' | 'programacao' | 'painel'

const tabs: { key: TabKey; label: string }[] = [
  { key: 'resumo', label: 'Resumo' },
  { key: 'empresas', label: 'Empresas' },
  { key: 'negocios', label: 'Negócios' },
  { key: 'tarefas', label: 'Tarefas' },
  { key: 'programacao', label: 'Programação' },
  { key: 'painel', label: 'Painel' }
]

const tab = ref<TabKey>('resumo')
const loading = ref(true)

// Resumo
const data = ref<Workspace | null>(null)
const collapsed = ref<Record<string, boolean>>({})

// Demais abas (carregadas sob demanda)
const companies = ref<Company[]>([])
const deals = ref<Deal[]>([])
const tasks = ref<Task[]>([])
const meetings = ref<Meeting[]>([])
const taskView = ref<'pendente' | 'atrasada' | 'concluida' | ''>('pendente')

// Painel
const panels = ref<Panel[]>([])
const panel = ref<Panel | null>(null)
const panelId = ref<number | null>(null)

/**
 * As filas do resumo, na ordem em que o vendedor deve atacar o dia. Cada uma
 * tem o próprio vazio: fila zerada vira "está em dia", não some da tela.
 */
const queues = computed(() => {
  if (!data.value) return []
  return [
    {
      key: 'atrasadas',
      title: 'Tarefas atrasadas',
      items: data.value.overdue_tasks,
      empty: 'Nenhuma tarefa passou do prazo.',
      tone: 'red'
    },
    {
      key: 'hoje',
      title: 'Tarefas de hoje',
      items: data.value.today_tasks,
      empty: 'Você está em dia com as tarefas de hoje.',
      tone: 'purple'
    },
    {
      key: 'reunioes',
      title: 'Reuniões de hoje',
      items: data.value.today_meetings,
      empty: 'Nenhuma reunião marcada para hoje.',
      tone: 'blue'
    },
    {
      key: 'fechando',
      title: 'Fechando esta semana',
      items: data.value.closing_soon,
      empty: 'Nenhum negócio com fechamento previsto nos próximos 7 dias.',
      tone: 'green'
    },
    {
      key: 'parados',
      title: 'Negócios parados',
      items: data.value.stale_deals,
      empty: 'Você está em dia com os negócios que estavam parados.',
      tone: 'red'
    },
    {
      key: 'leads',
      title: 'Leads sem contato',
      items: data.value.untouched_leads,
      empty: 'Todo lead da sua carteira já recebeu alguma interação.',
      tone: 'purple'
    }
  ]
})

const pendencias = computed(() =>
  queues.value.reduce((sum, q) => sum + q.items.length, 0)
)

const goalPercent = computed(() => {
  if (!data.value?.goal_month) return 0
  return Math.min(100, (data.value.won_month / data.value.goal_month) * 100)
})

function linkFor(item: WorkspaceItem): string {
  if (item.deal_id) return `/negocios/${item.deal_id}`
  if (item.contact_id) return `/contatos/${item.contact_id}`
  if (item.company_id) return `/empresas/${item.company_id}`
  return '/tarefas'
}

function toggle(key: string) {
  collapsed.value[key] = !collapsed.value[key]
}

async function completeTask(id: number) {
  try {
    await api.patch(`/tasks/${id}/toggle`)
    toast.push('Tarefa concluída')
    await loadResumo()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function loadResumo() {
  data.value = await api.get<Workspace>('/workspace')
}

/** Cada aba busca só o que precisa, filtrado pela carteira de quem está logado. */
async function loadTab() {
  loading.value = true
  const me = auth.user?.id ?? 0
  try {
    if (tab.value === 'resumo') {
      await loadResumo()
    } else if (tab.value === 'empresas') {
      const resp = await api.get<Paginated<Company>>(`/companies?owner_id=${me}&per_page=100`)
      companies.value = resp.data ?? []
    } else if (tab.value === 'negocios') {
      const resp = await api.get<Paginated<Deal>>(`/deals?owner_id=${me}&status=aberto&per_page=100`)
      deals.value = resp.data ?? []
    } else if (tab.value === 'tarefas') {
      const status = taskView.value ? `&status=${taskView.value}` : ''
      const resp = await api.get<Paginated<Task>>(`/tasks?owner_id=${me}${status}&per_page=100`)
      tasks.value = resp.data ?? []
    } else if (tab.value === 'programacao') {
      const resp = await api.get<{ data: Meeting[] }>(`/meetings?user_id=${me}&period=proximas`)
      meetings.value = resp.data ?? []
    } else if (tab.value === 'painel') {
      const resp = await api.get<{ data: Panel[]; workspace_dashboard_id: number | null }>('/panels')
      panels.value = resp.data ?? []
      panelId.value = resp.workspace_dashboard_id
      panel.value = panelId.value ? await api.get<Panel>(`/panels/${panelId.value}`) : null
    }
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

/** Guarda o painel escolhido no perfil, para voltar nele na próxima visita. */
async function choosePanel(id: number | null) {
  try {
    await api.put('/workspace/dashboard', { dashboard_id: id })
    panelId.value = id
    panel.value = id ? await api.get<Panel>(`/panels/${id}`) : null
  } catch (e: any) {
    toast.error(e.message)
  }
}

/** Reuniões agrupadas por dia, como uma agenda. */
const schedule = computed(() => {
  const groups: { day: string; items: Meeting[] }[] = []
  for (const m of meetings.value) {
    const day = new Date(m.starts_at).toLocaleDateString('pt-BR', {
      weekday: 'long',
      day: '2-digit',
      month: 'long'
    })
    const last = groups[groups.length - 1]
    if (last && last.day === day) last.items.push(m)
    else groups.push({ day, items: [m] })
  }
  return groups
})

watch(tab, loadTab)
watch(taskView, () => tab.value === 'tarefas' && loadTab())
onMounted(loadTab)
</script>

<template>
  <div class="page workspace">
    <div class="ws-head">
      <h1>Vendas</h1>
      <span class="ws-owner">{{ auth.user?.name }}</span>
    </div>

    <nav class="ws-tabs">
      <button
        v-for="t in tabs"
        :key="t.key"
        type="button"
        :class="{ active: tab === t.key }"
        @click="tab = t.key"
      >
        {{ t.label }}
      </button>
    </nav>

    <p v-if="loading" class="muted">Carregando…</p>

    <!-- ===== Resumo ===== -->
    <template v-else-if="tab === 'resumo' && data">
      <div class="stats">
        <div class="stat">
          <span class="stat-label">Ganho no mês</span>
          <strong>{{ formatMoney(data.won_month) }}</strong>
          <span class="muted small" v-if="data.goal_month">
            meta {{ formatMoney(data.goal_month) }} · {{ goalPercent.toFixed(0) }}%
          </span>
        </div>
        <div class="stat">
          <span class="stat-label">Em aberto</span>
          <strong>{{ formatMoney(data.open_amount) }}</strong>
          <span class="muted small">{{ data.open_deals }} negócio(s)</span>
        </div>
        <div class="stat">
          <span class="stat-label">Tarefas pendentes</span>
          <strong>{{ data.tasks_pending }}</strong>
          <span class="muted small">
            {{ data.overdue_tasks.length ? `${data.overdue_tasks.length} atrasada(s)` : 'nada atrasado' }}
          </span>
        </div>
        <div class="stat">
          <span class="stat-label">Reuniões na semana</span>
          <strong>{{ data.meetings_week }}</strong>
        </div>
        <div class="stat">
          <span class="stat-label">Contas-alvo</span>
          <strong>{{ data.target_accounts }}</strong>
        </div>
      </div>

      <p class="muted count">
        {{ pendencias ? `${pendencias} item(ns) na sua fila` : 'Nada pendente: seu dia está em dia.' }}
      </p>

      <section v-for="queue in queues" :key="queue.key" class="card queue">
        <button type="button" class="queue-head" @click="toggle(queue.key)">
          <span class="caret">{{ collapsed[queue.key] ? '▸' : '▾' }}</span>
          <h2>{{ queue.title }}</h2>
          <span class="badge" :class="queue.items.length ? queue.tone : 'gray'">
            {{ queue.items.length }}
          </span>
        </button>

        <template v-if="!collapsed[queue.key]">
          <div v-if="!queue.items.length" class="queue-empty">
            <span class="check-big">✓</span>
            <p class="muted">{{ queue.empty }}</p>
          </div>

          <ul v-else class="items">
            <li v-for="item in queue.items" :key="`${queue.key}-${item.id}`">
              <button
                v-if="queue.key === 'atrasadas' || queue.key === 'hoje'"
                class="check"
                type="button"
                title="Concluir"
                @click="completeTask(item.id)"
              >
                ○
              </button>
              <router-link :to="linkFor(item)" class="item-main">
                <strong>{{ item.title }}</strong>
                <span class="muted small">
                  <template v-if="item.subtitle">{{ item.subtitle }}</template>
                  <template v-if="item.reason"> · {{ item.reason }}</template>
                </span>
              </router-link>
              <span class="item-meta">
                <strong v-if="item.amount">{{ formatMoney(item.amount) }}</strong>
                <span v-if="item.due" class="muted small">
                  {{ queue.key === 'reunioes' ? formatDateTime(item.due) : formatDate(item.due) }}
                </span>
              </span>
            </li>
          </ul>
        </template>
      </section>
    </template>

    <!-- ===== Empresas ===== -->
    <template v-else-if="tab === 'empresas'">
      <div v-if="!companies.length" class="card empty">
        <p class="muted">Nenhuma empresa na sua carteira.</p>
      </div>
      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th>Empresa</th>
              <th style="width: 120px">Nº do EC</th>
              <th style="width: 140px">Cidade</th>
              <th style="width: 110px">Situação</th>
              <th style="width: 120px">Criada em</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in companies" :key="c.id">
              <td>
                <router-link :to="`/empresas/${c.id}`"><strong>{{ c.name }}</strong></router-link>
                <span v-if="c.is_target" class="badge">alvo T{{ c.target_tier }}</span>
              </td>
              <td class="muted">{{ c.ec_number || '—' }}</td>
              <td class="muted">{{ c.city || '—' }}</td>
              <td>
                <span class="badge" :class="c.is_client ? 'green' : 'gray'">
                  {{ c.is_client ? 'cliente' : 'prospect' }}
                </span>
              </td>
              <td class="muted">{{ formatDate(c.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- ===== Negócios ===== -->
    <template v-else-if="tab === 'negocios'">
      <div v-if="!deals.length" class="card empty">
        <p class="muted">Nenhum negócio aberto na sua carteira.</p>
      </div>
      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th>Negócio</th>
              <th style="width: 150px">Etapa</th>
              <th style="width: 140px">Valor</th>
              <th style="width: 120px">Previsão</th>
              <th style="width: 150px">Última atividade</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="d in deals" :key="d.id">
              <td>
                <router-link :to="`/negocios/${d.id}`"><strong>{{ d.name }}</strong></router-link>
                <div v-if="d.company_name" class="muted small">{{ d.company_name }}</div>
              </td>
              <td><span class="badge blue">{{ d.stage_name }}</span></td>
              <td><strong>{{ formatMoney(d.amount) }}</strong></td>
              <td class="muted">{{ d.close_date ? formatDate(d.close_date) : '—' }}</td>
              <td class="muted">
                {{ d.last_activity_at ? relativeDate(d.last_activity_at) : 'nunca' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- ===== Tarefas ===== -->
    <template v-else-if="tab === 'tarefas'">
      <div class="task-views">
        <button
          v-for="view in [
            { key: 'pendente', label: 'Pendentes' },
            { key: 'atrasada', label: 'Atrasadas' },
            { key: 'concluida', label: 'Concluídas' },
            { key: '', label: 'Todas' }
          ]"
          :key="view.key"
          type="button"
          class="btn"
          :class="{ 'btn-primary': taskView === view.key }"
          @click="taskView = view.key as any"
        >
          {{ view.label }}
        </button>
      </div>

      <div v-if="!tasks.length" class="card empty">
        <p class="muted">Nenhuma tarefa nesta visão.</p>
      </div>
      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th style="width: 40px"></th>
              <th>Tarefa</th>
              <th style="width: 120px">Tipo</th>
              <th style="width: 110px">Prioridade</th>
              <th style="width: 150px">Vencimento</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="t in tasks" :key="t.id">
              <td>
                <button class="check" type="button" title="Concluir" @click="completeTask(t.id)">
                  {{ t.completed_at ? '●' : '○' }}
                </button>
              </td>
              <td>
                <strong :class="{ done: t.completed_at }">{{ t.title }}</strong>
                <div v-if="t.contact_name" class="muted small">{{ t.contact_name }}</div>
              </td>
              <td class="muted">{{ t.type }}</td>
              <td><span class="badge gray">{{ t.priority }}</span></td>
              <td :class="{ late: !t.completed_at && t.due_date && new Date(t.due_date) < new Date() }">
                {{ t.due_date ? formatDateTime(t.due_date) : '—' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- ===== Programação ===== -->
    <template v-else-if="tab === 'programacao'">
      <div v-if="!meetings.length" class="card empty">
        <p class="muted">Nenhuma reunião marcada.</p>
        <router-link to="/configuracoes/agendamento" class="btn btn-primary" style="margin-top: 12px">
          Publicar minha agenda
        </router-link>
      </div>
      <div v-else class="agenda">
        <div v-for="group in schedule" :key="group.day" class="day-group">
          <h2 class="day">{{ group.day }}</h2>
          <ul class="items">
            <li v-for="m in group.items" :key="m.id">
              <span class="hour">{{ new Date(m.starts_at).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) }}</span>
              <div class="item-main">
                <strong>{{ m.title }}</strong>
                <span class="muted small">
                  <template v-if="m.contact_name">{{ m.contact_name }}</template>
                  <template v-if="m.location"> · {{ m.location }}</template>
                </span>
              </div>
              <router-link v-if="m.contact_id" :to="`/contatos/${m.contact_id}`" class="btn btn-sm">
                Abrir
              </router-link>
            </li>
          </ul>
        </div>
      </div>
    </template>

    <!-- ===== Painel ===== -->
    <template v-else-if="tab === 'painel'">
      <div class="panel-head">
        <select :value="panelId ?? ''" @change="choosePanel(Number(($event.target as HTMLSelectElement).value) || null)">
          <option value="">Escolha um painel…</option>
          <option v-for="p in panels" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <router-link v-if="auth.can('reports.manage')" to="/relatorios" class="btn">
          Montar painéis
        </router-link>
      </div>

      <div v-if="!panel" class="card empty">
        <p class="muted">
          Escolha um painel para acompanhar seus números direto aqui, sem sair do espaço de trabalho.
        </p>
      </div>

      <div v-else-if="!panel.items.length" class="card empty">
        <p class="muted">Este painel ainda não tem relatórios.</p>
      </div>

      <div v-else class="panel-grid">
        <div
          v-for="item in panel.items"
          :key="item.id"
          class="card panel-item"
          :class="{ full: item.width === 'inteiro' }"
        >
          <h2>{{ item.name }}</h2>
          <ReportChart v-if="item.result" :result="item.result" chart="barras" />
          <p v-else class="muted">Não foi possível carregar este relatório.</p>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ws-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 14px;
}

.ws-head h1 {
  font-size: 20px;
}

.ws-owner {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--fix-text-3);
}

.ws-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 20px;
  overflow-x: auto;
}

.ws-tabs button {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 10px 14px;
  font-size: 14px;
  color: var(--fix-text-2);
  cursor: pointer;
  white-space: nowrap;
}

.ws-tabs button:hover {
  color: var(--fix-purple-dark);
}

.ws-tabs button.active {
  color: var(--fix-purple-dark);
  border-bottom-color: var(--fix-purple);
  font-weight: 500;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.stat {
  background: var(--fix-surface);
  border: 1px solid var(--fix-border);
  border-radius: 10px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stat-label {
  font-size: 12px;
  color: var(--fix-text-3);
}

.stat strong {
  font-size: 18px;
}

.count {
  margin: 0 0 12px;
  font-size: 13px;
}

.queue {
  margin-bottom: 12px;
  padding: 0;
}

.queue-head {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  background: none;
  border: none;
  padding: 14px 16px;
  cursor: pointer;
  text-align: left;
}

.queue-head h2 {
  font-size: 15px;
  flex: 1;
}

.caret {
  color: var(--fix-text-3);
  font-size: 11px;
  width: 12px;
}

.queue-empty {
  padding: 18px 16px 24px;
  text-align: center;
}

.check-big {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--fix-green-tint);
  color: var(--fix-green);
  font-size: 20px;
  margin-bottom: 8px;
}

.queue-empty p {
  margin: 0;
  font-size: 13px;
}

.items {
  list-style: none;
  margin: 0;
  padding: 0 16px 8px;
}

.items li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 0;
  border-top: 1px solid var(--fix-border);
}

.check {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--fix-text-3);
  font-size: 16px;
  padding: 0 2px;
  flex-shrink: 0;
}

.check:hover {
  color: var(--fix-green);
}

.item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  color: inherit;
}

.item-main strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
  flex-shrink: 0;
  text-align: right;
}

.small {
  font-size: 12px;
}

.empty {
  text-align: center;
  padding: 32px;
}

.badge {
  margin-left: 6px;
}

.done {
  text-decoration: line-through;
  color: var(--fix-text-3);
}

.late {
  color: var(--fix-red);
}

.task-views {
  display: flex;
  gap: 8px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}

.agenda .day-group {
  margin-bottom: 20px;
}

.day {
  font-size: 13px;
  text-transform: capitalize;
  color: var(--fix-text-3);
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--fix-border);
}

.agenda .items {
  padding: 0;
}

.hour {
  font-variant-numeric: tabular-nums;
  font-size: 13px;
  color: var(--fix-purple-dark);
  font-weight: 500;
  width: 46px;
  flex-shrink: 0;
}

.panel-head {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
  align-items: center;
}

.panel-head select {
  padding: 8px 12px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  background: var(--fix-surface);
  font-size: 14px;
  min-width: 220px;
}

.panel-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
  align-items: start;
}

.panel-item.full {
  grid-column: 1 / -1;
}

.panel-item h2 {
  font-size: 15px;
  margin-bottom: 12px;
}
</style>
