<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatMoney } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import StatCard from '../components/StatCard.vue'
import type { Forecast, Pipeline, SalesGoal, User } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const forecast = ref<Forecast | null>(null)
const pipelines = ref<Pipeline[]>([])
const users = ref<User[]>([])
const loading = ref(true)

const period = ref(new Date().toISOString().slice(0, 7))
const pipelineId = ref(0)

const goalsOpen = ref(false)
const goalDrafts = ref<Record<string, number>>({})
const savingGoals = ref(false)

const canSetGoals = computed(() => auth.can('goals.manage'))

/** Últimos 6 meses e os 3 próximos, para navegar a previsão. */
const periods = computed(() => {
  const list: string[] = []
  const now = new Date()
  for (let i = -6; i <= 3; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() + i, 1)
    list.push(d.toISOString().slice(0, 7))
  }
  return list
})

function periodLabel(value: string): string {
  const [y, m] = value.split('-').map(Number)
  return new Date(y, m - 1, 1).toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' })
}

/** Quanto da meta a projeção já cobre (limitado a 100% na barra). */
const projectedPercent = computed(() => {
  if (!forecast.value?.team_goal) return 0
  return Math.min(100, (forecast.value.projected / forecast.value.team_goal) * 100)
})

const wonPercent = computed(() => {
  if (!forecast.value?.team_goal) return 0
  return Math.min(100, (forecast.value.won / forecast.value.team_goal) * 100)
})

async function load() {
  loading.value = true
  try {
    const query = `?period=${period.value}${pipelineId.value ? `&pipeline_id=${pipelineId.value}` : ''}`
    forecast.value = await api.get<Forecast>(`/forecast${query}`)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadRefs() {
  try {
    const [pipeResp, userList] = await Promise.all([
      api.get<{ data: Pipeline[] }>('/pipelines'),
      api.get<User[]>('/users')
    ])
    pipelines.value = pipeResp.data ?? []
    users.value = userList
  } catch {
    /* a previsão funciona mesmo sem os filtros */
  }
}

async function openGoals() {
  try {
    const resp = await api.get<{ data: SalesGoal[] }>(`/goals?period=${period.value}`)
    const drafts: Record<string, number> = { equipe: 0 }
    for (const u of users.value) drafts[String(u.id)] = 0
    for (const g of resp.data ?? []) {
      drafts[g.user_id === null ? 'equipe' : String(g.user_id)] = g.amount
    }
    goalDrafts.value = drafts
    goalsOpen.value = true
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function saveGoals() {
  savingGoals.value = true
  try {
    for (const [key, amount] of Object.entries(goalDrafts.value)) {
      await api.put('/goals', {
        user_id: key === 'equipe' ? null : Number(key),
        period: period.value,
        amount
      })
    }
    toast.push('Metas salvas')
    goalsOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    savingGoals.value = false
  }
}

watch([period, pipelineId], load)

onMounted(async () => {
  await loadRefs()
  await load()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Previsão</h1>
        <p class="muted" style="margin: 4px 0 0">
          Quanto já fechou, quanto está comprometido para o mês e onde isso deixa a equipe frente à meta.
        </p>
      </div>
      <div class="filters">
        <select v-model="period">
          <option v-for="p in periods" :key="p" :value="p">{{ periodLabel(p) }}</option>
        </select>
        <select v-model.number="pipelineId">
          <option :value="0">Todos os pipelines</option>
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <button v-if="canSetGoals" class="btn" type="button" @click="openGoals">Metas</button>
      </div>
    </div>

    <p v-if="loading" class="muted">Calculando previsão…</p>

    <template v-else-if="forecast">
      <div class="stats">
        <StatCard label="Ganho no mês" :value="formatMoney(forecast.won)" tone="green" />
        <StatCard
          label="Comprometido"
          :value="formatMoney(forecast.committed)"
          hint="aberto com fechamento previsto no mês"
          tone="blue"
        />
        <StatCard
          label="Projeção"
          :value="formatMoney(forecast.projected)"
          hint="ganho + ponderado pela etapa"
          tone="purple"
        />
        <StatCard
          label="Meta da equipe"
          :value="formatMoney(forecast.team_goal)"
          :hint="forecast.gap > 0 ? `faltam ${formatMoney(forecast.gap)}` : 'meta coberta pela projeção'"
          :tone="forecast.gap > 0 ? 'red' : 'green'"
        />
      </div>

      <div v-if="forecast.team_goal > 0" class="card progress-card">
        <h2>Caminho até a meta</h2>
        <div class="progress">
          <div class="progress-projected" :style="{ width: `${projectedPercent}%` }"></div>
          <div class="progress-won" :style="{ width: `${wonPercent}%` }"></div>
        </div>
        <div class="progress-legend">
          <span><i class="dot won"></i> Ganho {{ formatMoney(forecast.won) }}</span>
          <span><i class="dot projected"></i> Projeção {{ formatMoney(forecast.projected) }}</span>
          <span class="muted">Meta {{ formatMoney(forecast.team_goal) }}</span>
        </div>
      </div>

      <div v-if="!forecast.rows.length" class="card empty">
        <p class="muted">
          Nenhum negócio ganho ou com fechamento previsto para {{ periodLabel(period) }}.
        </p>
      </div>

      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th>Vendedor</th>
              <th style="width: 140px">Ganho</th>
              <th style="width: 150px">Comprometido</th>
              <th style="width: 140px">Ponderado</th>
              <th style="width: 140px">Meta</th>
              <th style="width: 160px">Atingimento</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in forecast.rows" :key="row.owner_id ?? 0">
              <td><strong>{{ row.owner_name }}</strong></td>
              <td>{{ formatMoney(row.won) }}</td>
              <td>
                {{ formatMoney(row.committed) }}
                <div class="muted small">{{ row.open_deals }} negócio(s)</div>
              </td>
              <td class="muted">{{ formatMoney(row.weighted) }}</td>
              <td>{{ row.goal ? formatMoney(row.goal) : '—' }}</td>
              <td>
                <template v-if="row.goal">
                  <div class="mini-track">
                    <div
                      class="mini-fill"
                      :class="{ ok: row.attainment >= 100 }"
                      :style="{ width: `${Math.min(100, row.attainment)}%` }"
                    ></div>
                  </div>
                  <span class="small">{{ row.attainment.toFixed(0) }}%</span>
                </template>
                <span v-else class="muted small">sem meta</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- ===== Metas do mês ===== -->
    <ModalDialog :title="`Metas de ${periodLabel(period)}`" :open="goalsOpen" @close="goalsOpen = false">
      <p class="muted" style="margin-top: 0">
        Deixe em zero para não ter meta. Sem meta da equipe, ela vira a soma das individuais.
      </p>
      <div class="field">
        <label>Meta da equipe</label>
        <input v-model.number="goalDrafts['equipe']" type="number" min="0" step="100" />
      </div>
      <div v-for="u in users" :key="u.id" class="field">
        <label>{{ u.name }}</label>
        <input v-model.number="goalDrafts[String(u.id)]" type="number" min="0" step="100" />
      </div>

      <template #footer>
        <button class="btn" type="button" @click="goalsOpen = false">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="savingGoals" @click="saveGoals">
          {{ savingGoals ? 'Salvando…' : 'Salvar metas' }}
        </button>
      </template>
    </ModalDialog>
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
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 14px;
  margin-bottom: 20px;
}

.progress-card {
  margin-bottom: 16px;
}

.progress-card h2 {
  font-size: 15px;
  margin-bottom: 12px;
}

.progress {
  position: relative;
  height: 22px;
  background: var(--fix-bg);
  border-radius: 8px;
  overflow: hidden;
}

.progress-projected,
.progress-won {
  position: absolute;
  top: 0;
  left: 0;
  height: 100%;
  border-radius: 8px;
  transition: width 0.3s;
}

.progress-projected {
  background: var(--fix-purple-tint);
}

.progress-won {
  background: var(--fix-purple);
}

.progress-legend {
  display: flex;
  gap: 16px;
  margin-top: 10px;
  font-size: 13px;
  flex-wrap: wrap;
}

.dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 3px;
  margin-right: 6px;
}

.dot.won {
  background: var(--fix-purple);
}

.dot.projected {
  background: var(--fix-purple-tint);
}

.empty {
  text-align: center;
  padding: 28px;
}

.small {
  font-size: 12px;
}

.mini-track {
  height: 6px;
  background: var(--fix-bg);
  border-radius: 3px;
  overflow: hidden;
  margin-bottom: 3px;
}

.mini-fill {
  height: 100%;
  background: var(--fix-purple);
  border-radius: 3px;
}

.mini-fill.ok {
  background: var(--fix-green);
}
</style>
