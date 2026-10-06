<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import type { SavedView } from '../types'

const router = useRouter()
const toast = useToastStore()

const views = ref<SavedView[]>([])
const loading = ref(false)
const search = ref('')
const entityFilter = ref('')

const entityLabels: Record<string, string> = {
  contacts: 'Contatos',
  companies: 'Empresas',
  deals: 'Negócios'
}

const entityRoutes: Record<string, string> = {
  contacts: '/contatos',
  companies: '/empresas',
  deals: '/negocios'
}

const entityBadge: Record<string, string> = {
  contacts: 'blue',
  companies: 'amber',
  deals: 'green'
}

const filtered = computed(() => {
  let list = views.value
  if (entityFilter.value) list = list.filter((v) => v.entity === entityFilter.value)
  const term = search.value.trim().toLowerCase()
  if (term) list = list.filter((v) => v.name.toLowerCase().includes(term))
  return list
})

async function load() {
  loading.value = true
  try {
    views.value = await api.get<SavedView[]>('/views')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function open(view: SavedView) {
  router.push(`${entityRoutes[view.entity]}?view=${view.id}`)
}

async function rename(view: SavedView) {
  const name = prompt('Novo nome da visualização:', view.name)
  if (!name?.trim() || name.trim() === view.name) return
  try {
    await api.put(`/views/${view.id}`, { name: name.trim() })
    views.value = views.value.map((v) => (v.id === view.id ? { ...v, name: name.trim() } : v))
    toast.push('Visualização renomeada')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function clone(view: SavedView) {
  const name = prompt('Nome da cópia:', `${view.name} (cópia)`)
  if (!name?.trim()) return
  try {
    const created = await api.post<SavedView>('/views', {
      entity: view.entity,
      name: name.trim(),
      filters: view.filters
    })
    views.value = [...views.value, { ...created, created_by_name: 'você' }]
    toast.push('Visualização clonada')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function remove(view: SavedView) {
  if (!confirm(`Excluir a visualização "${view.name}"? Os registros não são afetados.`)) return
  try {
    await api.delete(`/views/${view.id}`)
    views.value = views.value.filter((v) => v.id !== view.id)
    toast.push('Visualização excluída')
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
        <h1>Todas as visualizações</h1>
        <p class="muted" style="margin: 4px 0 0">
          As visualizações salvas aparecem como abas nas telas de contatos, empresas e negócios — para toda a equipe.
        </p>
      </div>
    </div>

    <div class="toolbar filters">
      <input v-model="search" type="search" placeholder="Pesquisar visualizações…" />
      <select v-model="entityFilter">
        <option value="">Todos os objetos</option>
        <option value="contacts">Contatos</option>
        <option value="companies">Empresas</option>
        <option value="deals">Negócios</option>
      </select>
      <button v-if="search || entityFilter" class="btn btn-outline btn-sm" type="button" @click="search = ''; entityFilter = ''">
        Limpar tudo
      </button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Objeto</th>
            <th>Nome da visualização</th>
            <th>Proprietário</th>
            <th>Criada em</th>
            <th style="width: 240px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="v in filtered" :key="v.id" @click="open(v)">
            <td><span class="badge" :class="entityBadge[v.entity]">{{ entityLabels[v.entity] || v.entity }}</span></td>
            <td><strong>{{ v.name }}</strong></td>
            <td :class="{ muted: !v.created_by_name }">{{ v.created_by_name || '—' }}</td>
            <td class="muted">{{ formatDate(v.created_at) }}</td>
            <td @click.stop class="row-actions">
              <button class="btn btn-outline btn-sm" type="button" @click="open(v)">Abrir</button>
              <button class="btn btn-outline btn-sm" type="button" @click="rename(v)">Renomear</button>
              <button class="btn btn-outline btn-sm" type="button" @click="clone(v)">Clonar</button>
              <button class="btn btn-danger btn-sm" type="button" @click="remove(v)">Excluir</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !filtered.length" class="empty-state">
        <strong>Nenhuma visualização aqui</strong>
        Aplique filtros em contatos, empresas ou negócios e clique em "☆ Salvar visualização".
      </div>
    </div>
  </div>
</template>

<style scoped>
.filters {
  margin-bottom: 14px;
}

.row-actions {
  white-space: nowrap;
}

.row-actions .btn {
  margin-right: 4px;
}
</style>
