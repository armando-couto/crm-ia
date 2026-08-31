<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatDate, formatMoney, formatDateTime } from '../format'
import StatCard from '../components/StatCard.vue'
import type { Workspace, WorkspaceItem } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const data = ref<Workspace | null>(null)
const loading = ref(true)

/** Filas na ordem em que o vendedor deve atacar o dia. */
const queues = computed(() => {
  if (!data.value) return []
  return [
    {
      key: 'atrasadas',
      title: 'Tarefas atrasadas',
      hint: 'passaram do prazo',
      tone: 'red',
      items: data.value.overdue_tasks
    },
    {
      key: 'hoje',
      title: 'Para hoje',
      hint: 'tarefas com prazo hoje',
      tone: 'purple',
      items: data.value.today_tasks
    },
    {
      key: 'reunioes',
      title: 'Reuniões de hoje',
      hint: 'na sua agenda',
      tone: 'blue',
      items: data.value.today_meetings
    },
    {
      key: 'fechando',
      title: 'Fechando esta semana',
      hint: 'previsão nos próximos 7 dias',
      tone: 'green',
      items: data.value.closing_soon
    },
    {
      key: 'parados',
      title: 'Negócios parados',
      hint: 'sem interação há mais de 14 dias',
      tone: 'red',
      items: data.value.stale_deals
    },
    {
      key: 'leads',
      title: 'Leads sem contato',
      hint: 'ainda não receberam nenhuma interação',
      tone: 'purple',
      items: data.value.untouched_leads
    }
  ].filter((q) => q.items.length > 0)
})

const totalPendencias = computed(() =>
  queues.value.reduce((sum, q) => sum + q.items.length, 0)
)

const goalPercent = computed(() => {
  if (!data.value?.goal_month) return 0
  return Math.min(100, (data.value.won_month / data.value.goal_month) * 100)
})

/** Para onde o item leva quando clicado. */
function linkFor(item: WorkspaceItem): string {
  if (item.deal_id) return `/negocios/${item.deal_id}`
  if (item.contact_id) return `/contatos/${item.contact_id}`
  if (item.company_id) return `/empresas/${item.company_id}`
  return '/tarefas'
}

async function completeTask(item: WorkspaceItem) {
  try {
    await api.patch(`/tasks/${item.id}/toggle`)
    toast.push('Tarefa concluída')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function load() {
  loading.value = true
  try {
    data.value = await api.get<Workspace>('/workspace')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Espaço de trabalho</h1>
        <p class="muted" style="margin: 4px 0 0">
          Olá, {{ auth.user?.name?.split(' ')[0] }}. O que precisa da sua atenção hoje.
        </p>
      </div>
    </div>

    <p v-if="loading" class="muted">Montando seu dia…</p>

    <template v-else-if="data">
      <div class="stats">
        <StatCard
          label="Ganho no mês"
          :value="formatMoney(data.won_month)"
          :hint="data.goal_month ? `meta ${formatMoney(data.goal_month)}` : 'sem meta definida'"
          tone="green"
        />
        <StatCard
          label="Em aberto"
          :value="formatMoney(data.open_amount)"
          :hint="`${data.open_deals} negócio(s)`"
          tone="blue"
        />
        <StatCard
          label="Tarefas pendentes"
          :value="String(data.tasks_pending)"
          :hint="data.overdue_tasks.length ? `${data.overdue_tasks.length} atrasada(s)` : 'nada atrasado'"
          :tone="data.overdue_tasks.length ? 'red' : 'purple'"
        />
        <StatCard label="Reuniões na semana" :value="String(data.meetings_week)" />
        <StatCard label="Contas-alvo" :value="String(data.target_accounts)" hint="sob sua responsabilidade" />
      </div>

      <div v-if="data.goal_month > 0" class="card goal">
        <div class="goal-head">
          <h2>Sua meta do mês</h2>
          <span :class="goalPercent >= 100 ? 'ok' : 'muted'">{{ goalPercent.toFixed(0) }}%</span>
        </div>
        <div class="goal-track">
          <div class="goal-fill" :class="{ ok: goalPercent >= 100 }" :style="{ width: `${goalPercent}%` }"></div>
        </div>
        <p class="muted small">
          {{ formatMoney(data.won_month) }} de {{ formatMoney(data.goal_month) }}
        </p>
      </div>

      <div v-if="!queues.length" class="card clean">
        <h2>Dia limpo</h2>
        <p class="muted">
          Nenhuma tarefa atrasada, nenhum negócio parado e nenhum lead esquecido. Bom trabalho.
        </p>
      </div>

      <template v-else>
        <p class="muted count">{{ totalPendencias }} item(ns) na sua fila</p>

        <div class="queues">
          <div v-for="queue in queues" :key="queue.key" class="card queue">
            <div class="queue-head">
              <h2>
                {{ queue.title }}
                <span class="badge" :class="queue.tone === 'red' ? 'red' : queue.tone === 'green' ? 'green' : 'blue'">
                  {{ queue.items.length }}
                </span>
              </h2>
              <span class="muted small">{{ queue.hint }}</span>
            </div>

            <ul class="items">
              <li v-for="item in queue.items" :key="`${queue.key}-${item.id}`">
                <button
                  v-if="queue.key === 'atrasadas' || queue.key === 'hoje'"
                  class="check"
                  type="button"
                  title="Concluir"
                  @click="completeTask(item)"
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
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 14px;
  margin-bottom: 18px;
}

.goal {
  margin-bottom: 18px;
}

.goal-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 10px;
}

.goal-head h2 {
  font-size: 15px;
}

.goal-head .ok {
  color: var(--fix-green);
  font-weight: 600;
}

.goal-track {
  height: 18px;
  background: var(--fix-bg);
  border-radius: 8px;
  overflow: hidden;
}

.goal-fill {
  height: 100%;
  background: var(--fix-purple);
  border-radius: 8px;
  transition: width 0.3s;
}

.goal-fill.ok {
  background: var(--fix-green);
}

.clean {
  text-align: center;
  padding: 36px;
}

.clean h2 {
  font-size: 16px;
  margin-bottom: 6px;
}

.count {
  margin: 0 0 12px;
  font-size: 13px;
}

.queues {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 16px;
  align-items: start;
}

.queue-head {
  margin-bottom: 12px;
}

.queue-head h2 {
  font-size: 15px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.items {
  list-style: none;
  margin: 0;
  padding: 0;
}

.items li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 0;
  border-top: 1px solid var(--fix-border);
}

.items li:first-child {
  border-top: none;
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

.item-main:hover strong {
  color: var(--fix-purple-dark);
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
</style>
