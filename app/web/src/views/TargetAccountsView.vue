<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatMoney, relativeDate } from '../format'
import StatCard from '../components/StatCard.vue'
import type { TargetAccount, TargetSummary } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

const accounts = ref<TargetAccount[]>([])
const summary = ref<TargetSummary | null>(null)
const loading = ref(true)
const scope = ref<'todas' | 'minhas'>('todas')

const canEdit = computed(() => auth.can('companies.edit'))

const tierLabels: Record<number, string> = { 1: 'Tier 1', 2: 'Tier 2', 3: 'Tier 3' }

/** Contas paradas há mais de 30 dias merecem destaque. */
function isStale(account: TargetAccount): boolean {
  if (!account.last_activity_at) return true
  const days = (Date.now() - new Date(account.last_activity_at).getTime()) / 86400000
  return days > 30
}

async function load() {
  loading.value = true
  try {
    const query = scope.value === 'minhas' ? '?owner_id=me' : ''
    const resp = await api.get<{ data: TargetAccount[]; summary: TargetSummary }>(
      `/target-accounts${query}`
    )
    accounts.value = resp.data ?? []
    summary.value = resp.summary
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function changeTier(account: TargetAccount, tier: number) {
  try {
    await api.put(`/companies/${account.id}/target`, {
      is_target: true,
      target_tier: tier,
      target_notes: account.target_notes
    })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removeTarget(account: TargetAccount) {
  if (!confirm(`Tirar ${account.name} das contas-alvo?`)) return
  try {
    await api.put(`/companies/${account.id}/target`, { is_target: false })
    toast.push('Conta removida do alvo')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

watch(scope, load)
onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Contas-alvo</h1>
        <p class="muted" style="margin: 4px 0 0">
          As empresas que a equipe escolheu perseguir, com prioridade e o que já andou em cada uma.
        </p>
      </div>
      <div class="tabs">
        <button class="btn" :class="{ 'btn-primary': scope === 'todas' }" type="button" @click="scope = 'todas'">
          Todas
        </button>
        <button class="btn" :class="{ 'btn-primary': scope === 'minhas' }" type="button" @click="scope = 'minhas'">
          Minhas
        </button>
      </div>
    </div>

    <div v-if="summary" class="stats">
      <StatCard label="Contas-alvo" :value="String(summary.total)" :hint="`${summary.tier1} no tier 1`" />
      <StatCard
        label="Em aberto"
        :value="formatMoney(summary.open_amount)"
        :hint="`${summary.with_deals} com negócio aberto`"
        tone="blue"
      />
      <StatCard
        label="Sem interação"
        :value="String(summary.no_activity_30d)"
        hint="há mais de 30 dias"
        :tone="summary.no_activity_30d ? 'red' : 'green'"
      />
      <StatCard
        label="Sem decisor"
        :value="String(summary.without_decision_maker)"
        hint="nenhum contato marcado como decisor"
        :tone="summary.without_decision_maker ? 'red' : 'green'"
      />
    </div>

    <p v-if="loading" class="muted">Carregando contas-alvo…</p>

    <div v-else-if="!accounts.length" class="card empty">
      <h2>Nenhuma conta-alvo ainda</h2>
      <p class="muted">
        Abra uma empresa e marque como conta-alvo para acompanhá-la aqui com prioridade,
        decisores mapeados e alerta quando ficar parada.
      </p>
    </div>

    <div v-else class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th style="width: 90px">Prioridade</th>
            <th>Empresa</th>
            <th style="width: 130px">Dono</th>
            <th style="width: 110px">Contatos</th>
            <th style="width: 150px">Em aberto</th>
            <th style="width: 140px">Última interação</th>
            <th style="width: 80px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in accounts" :key="a.id" :class="{ stale: isStale(a) }">
            <td>
              <select
                v-if="canEdit"
                class="tier"
                :value="a.target_tier"
                @change="changeTier(a, Number(($event.target as HTMLSelectElement).value))"
              >
                <option :value="1">Tier 1</option>
                <option :value="2">Tier 2</option>
                <option :value="3">Tier 3</option>
              </select>
              <span v-else class="badge">{{ tierLabels[a.target_tier] ?? '—' }}</span>
            </td>
            <td>
              <router-link :to="`/empresas/${a.id}`"><strong>{{ a.name }}</strong></router-link>
              <div v-if="a.target_notes" class="muted small">{{ a.target_notes }}</div>
            </td>
            <td class="muted">{{ a.owner_name || '—' }}</td>
            <td>
              {{ a.contacts_count_total }}
              <span v-if="a.decision_makers" class="badge green">{{ a.decision_makers }} decisor(es)</span>
              <span v-else class="badge amber">sem decisor</span>
            </td>
            <td>
              <strong v-if="a.open_amount">{{ formatMoney(a.open_amount) }}</strong>
              <span v-else class="muted">—</span>
              <div class="muted small">{{ a.open_deals }} negócio(s)</div>
            </td>
            <td>
              <span v-if="a.last_activity_at" :class="{ 'stale-text': isStale(a) }">
                {{ relativeDate(a.last_activity_at) }}
              </span>
              <span v-else class="stale-text">nunca</span>
            </td>
            <td>
              <button
                v-if="canEdit"
                class="btn btn-sm"
                type="button"
                title="Tirar das contas-alvo"
                @click="removeTarget(a)"
              >
                ✕
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  gap: 8px;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 14px;
  margin-bottom: 20px;
}

.empty {
  text-align: center;
  padding: 36px;
}

.empty h2 {
  font-size: 16px;
  margin-bottom: 6px;
}

.tier {
  padding: 4px 8px;
  border: 1px solid var(--ci-border);
  border-radius: 6px;
  background: var(--ci-surface);
  font-size: 12px;
}

.small {
  font-size: 12px;
}

.badge {
  margin-left: 6px;
}

tr.stale td:first-child {
  box-shadow: inset 3px 0 0 var(--ci-red);
}

.stale-text {
  color: var(--ci-red);
}
</style>
