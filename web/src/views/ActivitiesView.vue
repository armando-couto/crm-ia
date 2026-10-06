<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { formatDateTime } from '../format'
import type { Activity, User } from '../types'

const toast = useToastStore()

const activities = ref<Activity[]>([])
const users = ref<User[]>([])
const loading = ref(true)

const filters = ref({ kind: '', user_id: 0, days: 30, q: '' })

const kindLabels: Record<string, string> = {
  nota: 'Observação',
  email: 'E-mail',
  ligacao: 'Ligação',
  reuniao: 'Reunião',
  sistema: 'Sistema'
}

const kindIcons: Record<string, string> = {
  nota: '✎',
  email: '✉',
  ligacao: '☎',
  reuniao: '⚑',
  sistema: '⚙'
}

/** Agrupa por dia, como o HubSpot faz na linha do tempo. */
const grouped = computed(() => {
  const groups: { day: string; items: Activity[] }[] = []
  for (const item of activities.value) {
    const day = new Date(item.created_at).toLocaleDateString('pt-BR', {
      weekday: 'long',
      day: '2-digit',
      month: 'long'
    })
    const last = groups[groups.length - 1]
    if (last && last.day === day) last.items.push(item)
    else groups.push({ day, items: [item] })
  }
  return groups
})

/** A que registro a atividade pertence, com link. */
function target(item: Activity): { label: string; to: string } | null {
  if (item.deal_id) return { label: item.deal_name || `Negócio #${item.deal_id}`, to: `/negocios/${item.deal_id}` }
  if (item.contact_id) {
    return { label: item.contact_name || `Contato #${item.contact_id}`, to: `/contatos/${item.contact_id}` }
  }
  if (item.company_id) {
    return { label: item.company_name || `Empresa #${item.company_id}`, to: `/empresas/${item.company_id}` }
  }
  return null
}

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ feed: 'true', limit: '150' })
    if (filters.value.kind) params.set('kind', filters.value.kind)
    if (filters.value.user_id) params.set('user_id', String(filters.value.user_id))
    if (filters.value.days) params.set('days', String(filters.value.days))
    if (filters.value.q.trim()) params.set('q', filters.value.q.trim())

    const resp = await api.get<{ data: Activity[] }>(`/activities?${params.toString()}`)
    activities.value = resp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

watch(() => ({ ...filters.value }), load, { deep: true })

onMounted(async () => {
  try {
    users.value = await api.get<User[]>('/users')
  } catch {
    users.value = []
  }
  await load()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Atividades</h1>
        <p class="muted" style="margin: 4px 0 0">
          Tudo o que a equipe registrou, em ordem de acontecimento.
        </p>
      </div>
    </div>

    <div class="filters card">
      <div class="field">
        <label>Tipo</label>
        <select v-model="filters.kind">
          <option value="">Todos</option>
          <option v-for="(label, key) in kindLabels" :key="key" :value="key">{{ label }}</option>
        </select>
      </div>
      <div class="field">
        <label>Quem</label>
        <select v-model.number="filters.user_id">
          <option :value="0">Todos</option>
          <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
        </select>
      </div>
      <div class="field">
        <label>Período</label>
        <select v-model.number="filters.days">
          <option :value="1">Hoje</option>
          <option :value="7">Últimos 7 dias</option>
          <option :value="30">Últimos 30 dias</option>
          <option :value="90">Últimos 90 dias</option>
          <option :value="0">Tudo</option>
        </select>
      </div>
      <div class="field grow">
        <label>Buscar</label>
        <input v-model="filters.q" placeholder="ex.: proposta, retorno, contrato" />
      </div>
    </div>

    <p v-if="loading" class="muted">Carregando atividades…</p>

    <div v-else-if="!activities.length" class="card empty">
      <p class="muted">Nenhuma atividade para os filtros escolhidos.</p>
    </div>

    <div v-else class="feed">
      <div v-for="group in grouped" :key="group.day" class="day-group">
        <h2 class="day">{{ group.day }}</h2>
        <ul class="items">
          <li v-for="item in group.items" :key="item.id">
            <span class="icon" :class="item.kind">{{ kindIcons[item.kind] || '•' }}</span>
            <div class="body">
              <div class="head">
                <span class="kind">{{ kindLabels[item.kind] || item.kind }}</span>
                <span v-if="item.user_name" class="muted">por {{ item.user_name }}</span>
                <router-link v-if="target(item)" :to="target(item)!.to" class="target">
                  {{ target(item)!.label }}
                </router-link>
                <span class="muted when">{{ formatDateTime(item.created_at) }}</span>
              </div>
              <p>{{ item.content }}</p>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.filters {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  align-items: flex-end;
  margin-bottom: 16px;
}

.filters .field {
  margin: 0;
  min-width: 150px;
}

.filters .grow {
  flex: 1;
  min-width: 200px;
}

.empty {
  text-align: center;
  padding: 32px;
}

.day-group {
  margin-bottom: 22px;
}

.day {
  font-size: 13px;
  text-transform: capitalize;
  color: var(--fix-text-3);
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--fix-border);
}

.items {
  list-style: none;
  margin: 0;
  padding: 0;
}

.items li {
  display: flex;
  gap: 12px;
  padding: 10px 0;
}

.icon {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  flex-shrink: 0;
}

.icon.sistema {
  background: var(--fix-bg);
  color: var(--fix-text-3);
}

.body {
  flex: 1;
  min-width: 0;
}

.head {
  display: flex;
  gap: 10px;
  align-items: baseline;
  flex-wrap: wrap;
  font-size: 12px;
  margin-bottom: 2px;
}

.kind {
  font-weight: 600;
  font-size: 13px;
}

.target {
  color: var(--fix-purple-dark);
}

.when {
  margin-left: auto;
}

.body p {
  margin: 0;
  font-size: 14px;
  white-space: pre-wrap;
}
</style>
