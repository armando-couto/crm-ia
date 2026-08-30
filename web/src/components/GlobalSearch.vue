<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import type { SearchResult } from '../types'

const router = useRouter()
const term = ref('')
const results = ref<SearchResult[]>([])
const open = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

watch(term, (value) => {
  if (timer) clearTimeout(timer)
  if (value.trim().length < 2) {
    results.value = []
    open.value = false
    return
  }
  timer = setTimeout(search, 250)
})

async function search() {
  try {
    const resp = await api.get<{ data: SearchResult[] }>(`/search?q=${encodeURIComponent(term.value.trim())}`)
    results.value = resp.data
    open.value = true
  } catch {
    results.value = []
  }
}

const routeByType: Record<string, string> = {
  contato: '/contatos',
  empresa: '/empresas',
  negocio: '/negocios'
}

function go(r: SearchResult) {
  open.value = false
  term.value = ''
  router.push(`${routeByType[r.type]}/${r.id}`)
}

const typeLabels: Record<string, string> = {
  contato: 'Contato',
  empresa: 'Empresa',
  negocio: 'Negócio'
}

function closeSoon() {
  setTimeout(() => (open.value = false), 150)
}
</script>

<template>
  <div class="global-search">
    <input
      v-model="term"
      type="search"
      placeholder="Buscar contatos, empresas e negócios…"
      @focus="open = results.length > 0"
      @blur="closeSoon"
    />
    <div v-if="open && results.length" class="results">
      <button v-for="r in results" :key="`${r.type}-${r.id}`" type="button" @mousedown.prevent="go(r)">
        <span class="badge gray">{{ typeLabels[r.type] }}</span>
        <span class="title">{{ r.title }}</span>
        <span class="muted">{{ r.sub }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.global-search {
  position: relative;
  width: min(440px, 100%);
}

input {
  width: 100%;
  padding: 8px 14px;
  border: 1px solid var(--fix-border);
  border-radius: 999px;
  font-size: 14px;
  outline: none;
  background: var(--fix-bg);
  transition: border-color 0.15s, background 0.15s;
}

input:focus {
  border-color: var(--fix-purple);
  background: var(--fix-surface);
}

.results {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  background: var(--fix-surface);
  border-radius: 10px;
  box-shadow: var(--shadow-lg);
  padding: 6px;
  z-index: 100;
  max-height: 320px;
  overflow-y: auto;
}

.results button {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 9px 10px;
  border: none;
  background: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  text-align: left;
}

.results button:hover {
  background: var(--fix-purple-tint);
}

.title {
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.muted {
  margin-left: auto;
  font-size: 12px;
  white-space: nowrap;
}
</style>
