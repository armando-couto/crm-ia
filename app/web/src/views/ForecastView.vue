<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatMoney } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import StatCard from '../components/StatCard.vue'
import type { CategoryForecast, Forecast, ForecastSubmission, Pipeline, SalesGoal, User } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const forecast = ref<Forecast | null>(null)
const categories = ref<CategoryForecast | null>(null)

// Visões do Sales Forecast: por etapa (estatística) ou por categoria (manual).
const view = ref<'etapa' | 'categoria'>('etapa')

// Envio de previsão: o número que o próprio vendedor submete para o mês.
const submitOpen = ref(false)
const submission = ref<ForecastSubmission | null>(null)
const submitDraft = ref({ amount: 0, note: '' })
const submitting = ref(false)
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
    const [byStage, byCategory, mySub] = await Promise.all([
      api.get<Forecast>(`/forecast${query}`),
      api.get<{ forecast: CategoryForecast }>(`/forecast/categories${query}`),
      api.get<{ submission: ForecastSubmission | null }>(`/forecast/submission?period=${period.value}`)
    ])
    forecast.value = byStage
    categories.value = byCategory.forecast
    submission.value = mySub.submission
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function openSubmit() {
  submitDraft.value = {
    amount: submission.value?.amount ?? 0,
    note: submission.value?.note ?? ''
  }
  submitOpen.value = true
}

async function submitForecast() {
  submitting.value = true
  try {
    const resp = await api.put<{ submission: ForecastSubmission }>('/forecast/submission', {
      period: period.value,
      amount: submitDraft.value.amount,
      note: submitDraft.value.note
    })
    submission.value = resp.submission
    toast.push('Previsão enviada')
    submitOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    submitting.value = false
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
        <div class="view-tabs">
          <button class="btn" :class="{ 'btn-primary': view === 'etapa' }" type="button" @click="view = 'etapa'">
            Etapa do negócio
          </button>
          <button class="btn" :class="{ 'btn-primary': view === 'categoria' }" type="button" @click="view = 'categoria'">
            Categoria de previsão
          </button>
        </div>
        <select v-model="period">
          <option v-for="p in periods" :key="p" :value="p">{{ periodLabel(p) }}</option>
        </select>
        <select v-model.number="pipelineId">
          <option :value="0">Todos os pipelines</option>
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <button v-if="canSetGoals" class="btn" type="button" @click="openGoals">Metas</button>
        <button class="btn" type="button" @click="openSubmit">
          {{ submission ? 'Atualizar previsão' : 'Enviar previsão' }}
        </button>
      </div>
    </div>

    <p v-if="loading" class="muted">Calculando previsão…</p>

    <template v-else-if="view === 'etapa' && forecast">
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

    <!-- ===== Visão por categoria de previsão ===== -->
    <template v-else-if="view === 'categoria' && categories">
      <div class="stats">
        <StatCard label="Meta" :value="formatMoney(categories.team_goal)" />
        <StatCard label="Fechado" :value="formatMoney(categories.closed)" tone="green" />
        <StatCard
          label="Lacuna"
          :value="formatMoney(Math.max(0, categories.gap))"
          hint="meta menos o fechado"
          :tone="categories.gap > 0 ? 'red' : 'green'"
        />
        <StatCard
          label="Envio de previsão"
          :value="formatMoney(categories.submitted)"
          hint="soma do que a equipe submeteu"
          tone="purple"
        />
      </div>

      <div v-if="!categories.rows.length" class="card empty">
        <p class="muted">
          Nenhum negócio classificado para {{ periodLabel(period) }}. Defina a categoria de
          previsão nos negócios com fechamento neste mês.
        </p>
      </div>

      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th>Vendedor</th>
              <th style="width: 160px">Cumprimento da meta</th>
              <th style="width: 120px">Fechado</th>
              <th style="width: 130px">Comprometido</th>
              <th style="width: 120px">Melhor caso</th>
              <th style="width: 120px">Pipeline</th>
              <th style="width: 140px">Previsão enviada</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in categories.rows" :key="row.owner_id ?? 0">
              <td><strong>{{ row.owner_name }}</strong></td>
              <td>
                <template v-if="row.goal">
                  <div class="mini-track">
                    <div
                      class="mini-fill"
                      :class="{ ok: row.closed >= row.goal }"
                      :style="{ width: `${Math.min(100, (row.closed / row.goal) * 100)}%` }"
                    ></div>
                  </div>
                  <span class="small">
                    {{ ((row.closed / row.goal) * 100).toFixed(0) }}% de {{ formatMoney(row.goal) }}
                  </span>
                </template>
                <span v-else class="muted small">sem meta</span>
              </td>
              <td><strong>{{ formatMoney(row.closed) }}</strong></td>
              <td>{{ formatMoney(row.committed) }}</td>
              <td>{{ formatMoney(row.best_case) }}</td>
              <td class="muted">{{ formatMoney(row.pipeline) }}</td>
              <td>
                <template v-if="row.submitted">
                  {{ formatMoney(row.submitted) }}
                  <div v-if="row.submitted_note" class="muted small" :title="row.submitted_note">
                    {{ row.submitted_note }}
                  </div>
                </template>
                <span v-else class="muted small">não enviou</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <p class="muted footer-note">
        Ganhos contam como <strong>Fechado</strong> automaticamente; os abertos entram no balde
        escolhido em cada negócio, e "Excluído" fica fora da previsão.
      </p>
    </template>

    <!-- ===== Envio de previsão ===== -->
    <ModalDialog :title="`Envio de previsão · ${periodLabel(period)}`" :open="submitOpen" @close="submitOpen = false">
      <p class="muted" style="margin-top: 0">
        O número que você prevê fechar no mês, na sua avaliação — ele aparece na coluna
        "Previsão enviada" ao lado dos valores calculados.
      </p>
      <div class="field">
        <label>Valor previsto</label>
        <input v-model.number="submitDraft.amount" type="number" min="0" step="100" />
      </div>
      <div class="field">
        <label>Observação</label>
        <textarea v-model="submitDraft.note" rows="3" placeholder="O que sustenta esse número?"></textarea>
      </div>

      <template #footer>
        <button class="btn" type="button" @click="submitOpen = false">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="submitting" @click="submitForecast">
          {{ submitting ? 'Enviando…' : 'Enviar previsão' }}
        </button>
      </template>
    </ModalDialog>

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
  align-items: center;
}

.view-tabs {
  display: flex;
  gap: 6px;
  margin-right: 8px;
}

.footer-note {
  margin-top: 14px;
  font-size: 12px;
}

.filters select {
  padding: 8px 12px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  background: var(--ci-surface);
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
  background: var(--ci-bg);
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
  background: var(--ci-purple-tint);
}

.progress-won {
  background: var(--ci-purple);
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
  background: var(--ci-purple);
}

.dot.projected {
  background: var(--ci-purple-tint);
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
  background: var(--ci-bg);
  border-radius: 3px;
  overflow: hidden;
  margin-bottom: 3px;
}

.mini-fill {
  height: 100%;
  background: var(--ci-purple);
  border-radius: 3px;
}

.mini-fill.ok {
  background: var(--ci-green);
}
</style>
