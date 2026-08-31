<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { formatMoney } from '../format'
import StatCard from '../components/StatCard.vue'
import type { Pipeline, SalesAnalytics } from '../types'

const toast = useToastStore()

const data = ref<SalesAnalytics | null>(null)
const pipelines = ref<Pipeline[]>([])
const loading = ref(true)
const days = ref(90)
const pipelineId = ref(0)

const activityLabels: Record<string, string> = {
  nota: 'Observações',
  email: 'E-mails',
  ligacao: 'Ligações',
  reuniao: 'Reuniões',
  sistema: 'Eventos do sistema'
}

function maxActivity(): number {
  return Math.max(1, ...(data.value?.activity_by_kind ?? []).map((a) => a.value))
}

async function load() {
  loading.value = true
  try {
    const query = `?days=${days.value}${pipelineId.value ? `&pipeline_id=${pipelineId.value}` : ''}`
    data.value = await api.get<SalesAnalytics>(`/sales-analytics${query}`)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

watch([days, pipelineId], load)

onMounted(async () => {
  try {
    const resp = await api.get<{ data: Pipeline[] }>('/pipelines')
    pipelines.value = resp.data ?? []
  } catch {
    /* segue sem o filtro de pipeline */
  }
  await load()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Análise de vendas</h1>
        <p class="muted" style="margin: 4px 0 0">
          Taxa de ganho, ticket médio, tempo de fechamento e onde os negócios estão parando.
        </p>
      </div>
      <div class="filters">
        <select v-model.number="days">
          <option :value="30">Últimos 30 dias</option>
          <option :value="90">Últimos 90 dias</option>
          <option :value="180">Últimos 6 meses</option>
          <option :value="365">Último ano</option>
        </select>
        <select v-model.number="pipelineId">
          <option :value="0">Todos os pipelines</option>
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
    </div>

    <p v-if="loading" class="muted">Calculando…</p>

    <template v-else-if="data">
      <div class="stats">
        <StatCard
          label="Taxa de ganho"
          :value="`${data.win_rate.toFixed(0)}%`"
          :hint="`${data.won} ganhos · ${data.lost} perdidos`"
          :tone="data.win_rate >= 50 ? 'green' : 'purple'"
        />
        <StatCard label="Receita ganha" :value="formatMoney(data.won_amount)" tone="green" />
        <StatCard label="Ticket médio" :value="formatMoney(data.avg_ticket)" tone="blue" />
        <StatCard
          label="Ciclo de vendas"
          :value="`${data.avg_cycle_days.toFixed(0)} dias`"
          hint="da criação ao ganho"
        />
        <StatCard label="Negócios criados" :value="String(data.created)" hint="no período" />
      </div>

      <div class="cards">
        <div class="card">
          <h2>Onde os negócios estão</h2>
          <p class="muted" v-if="!data.funnel.length">Nenhuma etapa configurada.</p>
          <div v-else class="funnel">
            <div v-for="stage in data.funnel" :key="stage.stage_id" class="funnel-row">
              <span class="funnel-label" :title="stage.stage_name">{{ stage.stage_name }}</span>
              <div class="funnel-track">
                <div class="funnel-fill" :style="{ width: `${Math.max(2, stage.rate)}%` }"></div>
              </div>
              <span class="funnel-value">
                {{ stage.count }}
                <span class="muted small">{{ formatMoney(stage.amount) }}</span>
              </span>
            </div>
          </div>
          <p class="muted hint">
            A barra compara cada etapa com a primeira do funil: onde ela encolhe de repente é onde a
            equipe está perdendo negócio.
          </p>
        </div>

        <div class="card">
          <h2>Interações no período</h2>
          <p class="muted" v-if="!data.activity_by_kind.length">Nenhuma interação registrada.</p>
          <div v-else class="funnel">
            <div v-for="item in data.activity_by_kind" :key="item.label" class="funnel-row">
              <span class="funnel-label">{{ activityLabels[item.label] ?? item.label }}</span>
              <div class="funnel-track">
                <div class="funnel-fill alt" :style="{ width: `${(item.value / maxActivity()) * 100}%` }"></div>
              </div>
              <span class="funnel-value">{{ item.value }}</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.filters {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.filters select {
  padding: 8px 12px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  background: var(--fix-surface);
  font-size: 14px;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 14px;
  margin-bottom: 20px;
}

.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
  gap: 16px;
}

.cards h2 {
  font-size: 15px;
  margin-bottom: 14px;
}

.funnel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.funnel-row {
  display: grid;
  grid-template-columns: 130px minmax(0, 1fr) 120px;
  gap: 10px;
  align-items: center;
  font-size: 13px;
}

.funnel-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--fix-text-2);
}

.funnel-track {
  background: var(--fix-bg);
  border-radius: 6px;
  height: 18px;
  overflow: hidden;
}

.funnel-fill {
  height: 100%;
  background: var(--fix-purple);
  border-radius: 6px;
  transition: width 0.25s;
}

.funnel-fill.alt {
  background: var(--fix-blue);
}

.funnel-value {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.small {
  font-size: 11px;
  display: block;
}

.hint {
  font-size: 12px;
  margin: 14px 0 0;
}
</style>
