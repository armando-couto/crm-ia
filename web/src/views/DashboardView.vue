<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { formatMoney } from '../format'
import { useToastStore } from '../stores/toast'
import StatCard from '../components/StatCard.vue'
import type { Dashboard, Pipeline } from '../types'

const toast = useToastStore()
const data = ref<Dashboard | null>(null)
const pipelines = ref<Pipeline[]>([])
const pipelineId = ref(0)
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    data.value = await api.get<Dashboard>(`/dashboard?pipeline_id=${pipelineId.value}`)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    pipelines.value = await api.get<Pipeline[]>('/pipelines')
  } catch {
    /* dashboard segue com todos os pipelines */
  }
  await load()
})

const maxStage = computed(() => Math.max(1, ...(data.value?.deals_by_stage.map((s) => s.count) ?? [1])))
const maxMonth = computed(() => Math.max(1, ...(data.value?.won_by_month.map((m) => m.amount) ?? [1])))

const monthLabel = (m: string) => {
  const [y, mm] = m.split('-')
  const names = ['jan', 'fev', 'mar', 'abr', 'mai', 'jun', 'jul', 'ago', 'set', 'out', 'nov', 'dez']
  return `${names[Number(mm) - 1] ?? mm}/${y.slice(2)}`
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Dashboard</h1>
      <select v-model.number="pipelineId" class="pipeline-select" @change="load">
        <option :value="0">Todos os pipelines</option>
        <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
    </div>

    <div v-if="loading && !data" class="muted">Carregando métricas…</div>

    <template v-if="data">
      <div class="stats">
        <StatCard label="Negócios abertos" :value="String(data.open_deals)" :hint="formatMoney(data.open_amount)" />
        <StatCard label="Previsão ponderada" :value="formatMoney(data.forecast_amount)" hint="pela probabilidade das etapas" tone="blue" />
        <StatCard label="Ganhos no mês" :value="String(data.won_this_month)" :hint="formatMoney(data.won_amount_month)" tone="green" />
        <StatCard label="Perdidos no mês" :value="String(data.lost_this_month)" tone="red" />
        <StatCard label="Contatos" :value="String(data.total_contacts)" :hint="`+${data.new_contacts_week} nos últimos 7 dias`" />
        <StatCard
          label="Tarefas pendentes"
          :value="String(data.tasks_pending)"
          :hint="data.tasks_overdue ? `${data.tasks_overdue} atrasadas` : 'nenhuma atrasada'"
          :tone="data.tasks_overdue ? 'red' : 'purple'"
        />
      </div>

      <div class="charts">
        <div class="card">
          <h2>Funil de vendas</h2>
          <p class="muted" v-if="!data.deals_by_stage.length">Sem negócios abertos.</p>
          <div class="funnel">
            <div v-for="s in data.deals_by_stage" :key="s.stage_id" class="funnel-row">
              <span class="funnel-label" :title="s.stage_name">{{ s.stage_name }}</span>
              <div class="funnel-bar-track">
                <div class="funnel-bar" :style="{ width: `${(s.count / maxStage) * 100}%` }"></div>
              </div>
              <span class="funnel-count">{{ s.count }} · {{ formatMoney(s.amount) }}</span>
            </div>
          </div>
        </div>

        <div class="card">
          <h2>Receita ganha por mês</h2>
          <p class="muted" v-if="!data.won_by_month.length">Nenhum negócio ganho nos últimos 6 meses.</p>
          <div class="bars" v-else>
            <div v-for="m in data.won_by_month" :key="m.month" class="bar-col">
              <span class="bar-value">{{ formatMoney(m.amount) }}</span>
              <div class="bar" :style="{ height: `${Math.max(6, (m.amount / maxMonth) * 140)}px` }"></div>
              <span class="bar-label">{{ monthLabel(m.month) }}</span>
            </div>
          </div>
        </div>

        <div class="card">
          <h2>Ranking do mês</h2>
          <p class="muted" v-if="!data.ranking_owners.length">Nenhum negócio ganho neste mês ainda.</p>
          <ol class="ranking" v-else>
            <li v-for="(o, i) in data.ranking_owners" :key="o.owner_id">
              <span class="pos">{{ i + 1 }}º</span>
              <span class="name">{{ o.owner_name }}</span>
              <span class="muted">{{ o.won }} negócio(s)</span>
              <strong>{{ formatMoney(o.amount) }}</strong>
            </li>
          </ol>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.pipeline-select {
  padding: 8px 12px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  background: var(--fix-surface);
  font-size: 14px;
  outline: none;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 14px;
  margin-bottom: 20px;
}

.charts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 14px;
}

.card h2 {
  font-size: 15px;
  margin-bottom: 16px;
}

.funnel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.funnel-row {
  display: grid;
  grid-template-columns: 120px 1fr auto;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}

.funnel-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--fix-text-2);
}

.funnel-bar-track {
  background: var(--fix-bg);
  border-radius: 6px;
  height: 18px;
  overflow: hidden;
}

.funnel-bar {
  height: 100%;
  background: linear-gradient(90deg, var(--fix-purple) 0%, var(--fix-purple-dark) 100%);
  border-radius: 6px;
  min-width: 4px;
  transition: width 0.4s ease;
}

.funnel-count {
  color: var(--fix-text-3);
  font-size: 12px;
  white-space: nowrap;
}

.bars {
  display: flex;
  align-items: flex-end;
  gap: 18px;
  min-height: 190px;
  padding-top: 10px;
}

.bar-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  flex: 1;
}

.bar {
  width: 100%;
  max-width: 48px;
  background: linear-gradient(180deg, var(--fix-purple) 0%, var(--fix-purple-dark) 100%);
  border-radius: 8px 8px 3px 3px;
  transition: height 0.4s ease;
}

.bar-value {
  font-size: 11px;
  color: var(--fix-text-3);
  white-space: nowrap;
}

.bar-label {
  font-size: 12px;
  color: var(--fix-text-2);
}

.ranking {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}

.ranking li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 0;
  border-bottom: 1px solid var(--fix-bg);
  font-size: 14px;
}

.ranking li:last-child {
  border-bottom: none;
}

.pos {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.name {
  flex: 1;
  font-weight: 500;
}
</style>
