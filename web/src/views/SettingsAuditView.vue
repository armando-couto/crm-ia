<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import type { AuditEntry, User } from '../types'

const toast = useToastStore()

const entries = ref<AuditEntry[]>([])
const actions = ref<string[]>([])
const users = ref<User[]>([])
const total = ref(0)
const page = ref(1)
const perPage = 50
const loading = ref(true)

const filters = ref({ user_id: '', action: '', days: '30', q: '' })

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / perPage)))

// Rótulos amigáveis para as ações gravadas pelo backend.
const actionLabels: Record<string, string> = {
  login: 'Acesso',
  login_falha: 'Falha de acesso',
  criar: 'Criação',
  editar: 'Edição',
  excluir: 'Exclusão',
  exportar: 'Exportação',
  importar: 'Importação',
  acao_em_massa: 'Ação em massa',
  permissoes: 'Permissões'
}

function actionLabel(action: string): string {
  return actionLabels[action] ?? action
}

function actionClass(action: string): string {
  if (action === 'excluir' || action === 'login_falha') return 'red'
  if (action === 'exportar' || action === 'importar' || action === 'acao_em_massa') return 'amber'
  if (action === 'permissoes') return 'blue'
  if (action === 'login') return 'green'
  return 'gray'
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage) })
    if (filters.value.user_id) params.set('user_id', filters.value.user_id)
    if (filters.value.action) params.set('action', filters.value.action)
    if (filters.value.days) params.set('days', filters.value.days)
    if (filters.value.q.trim()) params.set('q', filters.value.q.trim())

    const resp = await api.get<{ data: AuditEntry[]; total: number; actions: string[] }>(
      `/audit?${params.toString()}`
    )
    entries.value = resp.data ?? []
    total.value = resp.total ?? 0
    actions.value = resp.actions ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadUsers() {
  try {
    users.value = await api.get<User[]>('/users')
  } catch {
    users.value = []
  }
}

function changePage(delta: number) {
  const next = page.value + delta
  if (next < 1 || next > totalPages.value) return
  page.value = next
}

watch(
  () => ({ ...filters.value }),
  () => {
    page.value = 1
    load()
  },
  { deep: true }
)

watch(page, load)

onMounted(() => {
  loadUsers()
  load()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Auditoria</h1>
        <p class="muted" style="margin: 4px 0 0">
          Quem acessou, quem exportou e quem alterou cada registro sensível do CRM.
        </p>
      </div>
    </div>

    <div class="filters card">
      <div class="field">
        <label>Usuário</label>
        <select v-model="filters.user_id">
          <option value="">Todos</option>
          <option v-for="u in users" :key="u.id" :value="String(u.id)">{{ u.name }}</option>
        </select>
      </div>
      <div class="field">
        <label>Ação</label>
        <select v-model="filters.action">
          <option value="">Todas</option>
          <option v-for="a in actions" :key="a" :value="a">{{ actionLabel(a) }}</option>
        </select>
      </div>
      <div class="field">
        <label>Período</label>
        <select v-model="filters.days">
          <option value="1">Últimas 24 horas</option>
          <option value="7">Últimos 7 dias</option>
          <option value="30">Últimos 30 dias</option>
          <option value="90">Últimos 90 dias</option>
          <option value="0">Tudo</option>
        </select>
      </div>
      <div class="field grow">
        <label>Buscar no resumo</label>
        <input v-model="filters.q" placeholder="ex.: exportou, excluiu, perfil" />
      </div>
    </div>

    <p v-if="loading" class="muted">Carregando registros…</p>

    <template v-else>
      <div v-if="entries.length === 0" class="card empty">
        <p class="muted">Nenhum registro para os filtros escolhidos.</p>
      </div>

      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th style="width: 150px">Quando</th>
              <th style="width: 180px">Usuário</th>
              <th style="width: 140px">Ação</th>
              <th>O que aconteceu</th>
              <th style="width: 130px">Origem</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="e in entries" :key="e.id">
              <td class="muted">{{ formatDate(e.created_at) }}</td>
              <td>{{ e.user_name || '—' }}</td>
              <td><span class="badge" :class="actionClass(e.action)">{{ actionLabel(e.action) }}</span></td>
              <td>{{ e.summary }}</td>
              <td class="muted ip">{{ e.ip }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pager">
        <span class="muted">{{ total }} registro(s) · página {{ page }} de {{ totalPages }}</span>
        <div>
          <button class="btn" type="button" :disabled="page <= 1" @click="changePage(-1)">Anterior</button>
          <button class="btn" type="button" :disabled="page >= totalPages" @click="changePage(1)">Próxima</button>
        </div>
      </div>
    </template>
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

.ip {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 14px;
  gap: 12px;
}

.pager div {
  display: flex;
  gap: 8px;
}
</style>
