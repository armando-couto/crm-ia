<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api, ApiError } from '../../api'
import { formatDate, formatNumber } from '../../format'
import { useToastStore } from '../../stores/toast'
import {
  origemListaLabels,
  statusListaLabels,
  type Lista,
  type ListasResposta,
  type StatusLista
} from '../../types-prospeccao'
import PaginationBar from '../ui/PaginationBar.vue'

/** Aba "Listas de prospecção": as listas já geradas, com os contadores. */
const props = defineProps<{ podeGerenciar: boolean }>()
const emit = defineEmits<{ abrir: [id: number]; nova: [] }>()

const toast = useToastStore()
const listas = ref<Lista[]>([])
const total = ref(0)
const page = ref(1)
const perPage = ref(25)
const q = ref('')
const status = ref<'' | StatusLista>('')
const carregando = ref(false)
const erro = ref('')
let timerBusca: ReturnType<typeof setTimeout> | null = null

const tonsStatus: Record<StatusLista, string> = { gerando: 'blue', pronta: 'green', falhou: 'red' }

async function carregar() {
  carregando.value = true
  erro.value = ''
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage.value) })
    if (q.value.trim()) params.set('q', q.value.trim())
    if (status.value) params.set('status', status.value)
    const r = await api.get<ListasResposta>(`/prospeccao/listas?${params}`)
    listas.value = r?.data ?? []
    total.value = r?.pagination?.total ?? listas.value.length
  } catch (e: any) {
    erro.value = e?.message || 'não foi possível carregar as listas'
  } finally {
    carregando.value = false
  }
}

watch([page, perPage, status], () => void carregar())
watch(q, () => {
  if (timerBusca) clearTimeout(timerBusca)
  timerBusca = setTimeout(() => {
    if (page.value !== 1) page.value = 1
    else void carregar()
  }, 300)
})

async function excluir(lista: Lista) {
  if (!props.podeGerenciar) return
  if (!confirm(`Excluir a lista "${lista.nome}" e os ${formatNumber(lista.contadores?.total ?? 0)} itens dela? Leads já enviados ao CRM continuam lá.`)) return
  try {
    await api.delete(`/prospeccao/listas/${lista.id}`)
    toast.push('Lista excluída')
    await carregar()
  } catch (e: any) {
    if (e instanceof ApiError && e.codigo === 'job_ativo') toast.error('A lista tem um trabalho em andamento; cancele ou espere terminar para excluir.')
    else toast.error(e?.message || 'não foi possível excluir a lista')
  }
}

onMounted(carregar)
defineExpose({ carregar })
</script>

<template>
  <div>
    <div class="toolbar">
      <input v-model="q" type="search" placeholder="Buscar lista pelo nome" aria-label="Buscar lista" />
      <select v-model="status" aria-label="Situação">
        <option value="">Todas as situações</option>
        <option v-for="(r, v) in statusListaLabels" :key="v" :value="v">{{ r }}</option>
      </select>
    </div>

    <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>
    <div v-else-if="!carregando && !listas.length" class="card empty-state">
      <strong>{{ q || status ? 'Nenhuma lista encontrada' : 'Nenhuma lista ainda' }}</strong>
      <template v-if="!q && !status">
        Escolha os filtros na aba "Nova lista", confira a contagem e gere a primeira.
        <div v-if="podeGerenciar" class="acao-vazia">
          <button type="button" class="btn btn-primary btn-sm" @click="emit('nova')">Montar uma lista</button>
        </div>
      </template>
    </div>
    <div v-else class="card table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Lista</th>
            <th>Situação</th>
            <th class="num">Empresas</th>
            <th class="num">Com celular</th>
            <th class="num">Enriquecidas</th>
            <th class="num">Enviadas</th>
            <th>Criada</th>
            <th v-if="podeGerenciar"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in listas" :key="l.id" class="clicavel" @click="emit('abrir', l.id)">
            <td>
              <a href="#" class="nome" @click.prevent.stop="emit('abrir', l.id)">{{ l.nome }}</a>
              <div class="muted pequeno">
                {{ origemListaLabels[l.origem] ?? l.origem }}<span v-if="l.incluir_pf"> · inclui pessoa física</span>
              </div>
            </td>
            <td>
              <span class="badge" :class="tonsStatus[l.status]" :title="l.mensagem || undefined">{{ statusListaLabels[l.status] ?? l.status }}</span>
            </td>
            <td class="num">{{ formatNumber(l.contadores?.total) }}</td>
            <td class="num">{{ formatNumber(l.contadores?.com_celular) }}</td>
            <td class="num">{{ formatNumber(l.contadores?.enriquecido) }}</td>
            <td class="num">{{ formatNumber(l.contadores?.enviado) }}</td>
            <td>
              {{ formatDate(l.criado_em) }}
              <div v-if="l.criado_por_nome" class="muted pequeno">{{ l.criado_por_nome }}</div>
            </td>
            <td v-if="podeGerenciar" class="acoes">
              <button type="button" class="btn btn-outline btn-sm" title="Excluir lista" @click.stop="excluir(l)">Excluir</button>
            </td>
          </tr>
        </tbody>
      </table>
      <PaginationBar v-model:page="page" v-model:per-page="perPage" :total="total" :per-page-options="[25, 50, 100]" rotulo="listas" />
    </div>
  </div>
</template>

<style scoped>
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.clicavel {
  cursor: pointer;
}
.nome {
  font-weight: 500;
}
.pequeno {
  font-size: 12px;
}
.acoes {
  text-align: right;
  white-space: nowrap;
}
.acao-vazia {
  margin-top: 12px;
}
</style>
