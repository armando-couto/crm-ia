<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, foiAbortado } from '../api'
import {
  estadoLeadLabels,
  estadoLeadTons,
  formatDateTime,
  formatNumber,
  relativeDate,
  resultadoLabels,
  urgenciaLabels,
  urgenciaTons
} from '../format'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatarTelefone, telefonePrincipal } from '../telefone'
import { useAtalhos } from '../composables/useAtalhos'
import type { User } from '../types'
import {
  CONFIG_PROSPECCAO_PADRAO,
  ESTADOS_LEAD_ABERTOS,
  filaLeadsLabels,
  ordemLeadsLabels,
  temperaturaLabels,
  type ConfigProspeccao,
  type ContadoresLeads,
  type EstadoLead,
  type FilaLeads,
  type Lead,
  type LeadLista,
  type LeadsBulkRequest,
  type LeadsResposta,
  type OrdemLeads,
  type Temperatura,
  type Urgencia
} from '../types-prospeccao'
import ModalDialog from '../components/ModalDialog.vue'
import PaginationBar from '../components/ui/PaginationBar.vue'
import SideDrawer from '../components/ui/SideDrawer.vue'
import LeadPainel from '../components/leads/LeadPainel.vue'
import { baseDeDemonstracao, carregarConfig } from '../components/leads/util'

/**
 * Leads de prospecção (10.3): filas no topo, chips de estado com contadores,
 * tabela com semáforo de urgência, ações em massa e o painel contextual
 * (`/leads?lead=<id>`, com J, K e Esc).
 */
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()
/** Query da rota atual (a tela também monta fora do roteador, nos testes da fundação). */
const consulta = computed<Record<string, any>>(() => route?.query ?? {})

const podeAtribuir = computed(() => auth.can('leads.assign'))
const podeEditar = computed(() => auth.can('leads.edit'))

const filas = computed<FilaLeads[]>(() => (podeAtribuir.value ? ['meus', 'sem_dono', 'todos'] : ['meus', 'sem_dono']))
const ESTADOS: EstadoLead[] = ['novo', 'tentando', 'retornar', 'convertido', 'descartado']

// ===== Filtros =====
const fila = ref<FilaLeads>('meus')
const estado = ref<EstadoLead | ''>('')
const urgencia = ref<Urgencia | ''>('')
const listaId = ref<number | ''>('')
const temperatura = ref<Temperatura | 'todas'>('todas')
const ownerId = ref<number | ''>('')
const busca = ref('')
const ordem = ref<OrdemLeads>('urgencia')
const dir = ref<'asc' | 'desc'>('asc')
const page = ref(1)
const perPage = ref(25)

// ===== Dados =====
const leads = ref<Lead[]>([])
const total = ref(0)
const contadores = ref<ContadoresLeads | null>(null)
const carregando = ref(false)
const listas = ref<LeadLista[]>([])
const usuarios = ref<User[]>([])
const config = ref<ConfigProspeccao>({ ...CONFIG_PROSPECCAO_PADRAO })
const amostra = ref(false)
let controle: AbortController | null = null

function parametros(): string {
  const p = new URLSearchParams({
    fila: fila.value,
    page: String(page.value),
    per_page: String(perPage.value),
    sort: ordem.value,
    dir: dir.value
  })
  if (estado.value) p.set('estado', estado.value)
  if (urgencia.value) p.set('urgencia', urgencia.value)
  if (listaId.value !== '') p.set('lista_id', String(listaId.value))
  if (temperatura.value !== 'todas') p.set('temperatura', temperatura.value)
  if (podeAtribuir.value && ownerId.value !== '') p.set('owner_id', String(ownerId.value))
  if (busca.value.trim()) p.set('q', busca.value.trim())
  return p.toString()
}

async function carregar() {
  controle?.abort()
  controle = new AbortController()
  carregando.value = true
  try {
    const r = await api.get<LeadsResposta>(`/leads?${parametros()}`, { signal: controle.signal })
    leads.value = (r?.data ?? []).map((l) => ({ ...l, telefones: l.telefones ?? [], etiquetas: l.etiquetas ?? [] }))
    total.value = r?.pagination?.total ?? leads.value.length
    contadores.value = r?.contadores ?? null
    // Some da seleção o que saiu da página.
    const ids = new Set(leads.value.map((l) => l.id))
    selecionados.value = new Set([...selecionados.value].filter((id) => ids.has(id)))
  } catch (e: any) {
    if (foiAbortado(e)) return
    toast.error(e.message)
  } finally {
    carregando.value = false
  }
}

function recomecar() {
  page.value = 1
  selecionados.value = new Set()
  void carregar()
}

function escolherFila(f: FilaLeads) {
  if (fila.value === f) return
  fila.value = f
  recomecar()
}

function escolherEstado(e: EstadoLead | '') {
  estado.value = estado.value === e ? '' : e
  recomecar()
}

function escolherUrgencia(u: Urgencia | '') {
  urgencia.value = urgencia.value === u ? '' : u
  recomecar()
}

function ordenarPor(col: OrdemLeads) {
  if (ordem.value === col) dir.value = dir.value === 'asc' ? 'desc' : 'asc'
  else {
    ordem.value = col
    dir.value = col === 'nota' ? 'desc' : 'asc'
  }
  recomecar()
}

function setaOrdem(col: OrdemLeads): string {
  if (ordem.value !== col) return ''
  return dir.value === 'asc' ? '↑' : '↓'
}

let temporizador: ReturnType<typeof setTimeout> | null = null
watch(busca, () => {
  if (temporizador) clearTimeout(temporizador)
  temporizador = setTimeout(recomecar, 300)
})

watch([page, perPage], () => carregar())

// ===== Seleção e ações em massa =====
const selecionados = ref<Set<number>>(new Set())
const todosMarcados = computed(() => leads.value.length > 0 && leads.value.every((l) => selecionados.value.has(l.id)))

function marcarTodos() {
  selecionados.value = todosMarcados.value ? new Set() : new Set(leads.value.map((l) => l.id))
}

function marcar(id: number) {
  const s = new Set(selecionados.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  selecionados.value = s
}

const massaDono = ref<number | '' | 'nenhum'>('')
const massaTemperatura = ref<Temperatura | 'x'>('x')
const massaEtiqueta = ref('')
const descarteAberto = ref(false)
const massaMotivo = ref('')
const massaOutro = ref('')
const executando = ref(false)

async function emMassa(corpo: Omit<LeadsBulkRequest, 'ids'>, mensagem: string) {
  const ids = [...selecionados.value].slice(0, 500)
  if (!ids.length) return
  executando.value = true
  try {
    const r = await api.post<{ afetados: number }>('/leads/bulk', { ids, ...corpo })
    toast.push(`${mensagem}: ${r?.afetados ?? 0} lead(s)`)
    selecionados.value = new Set()
    await carregar()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    executando.value = false
  }
}

function atribuir() {
  if (massaDono.value === '') return
  const owner = massaDono.value === 'nenhum' ? null : massaDono.value
  void emMassa({ acao: 'dono', owner_id: owner }, 'Dono alterado')
  massaDono.value = ''
}

function mudarTemperatura() {
  if (massaTemperatura.value === 'x') return
  void emMassa({ acao: 'temperatura', temperatura: massaTemperatura.value }, 'Temperatura alterada')
  massaTemperatura.value = 'x'
}

function etiquetar() {
  const etiqueta = massaEtiqueta.value.trim()
  if (!etiqueta) return
  void emMassa({ acao: 'etiqueta', etiqueta }, 'Etiqueta aplicada')
  massaEtiqueta.value = ''
}

function abrirDescarte() {
  massaMotivo.value = ''
  massaOutro.value = ''
  descarteAberto.value = true
}

async function descartarEmMassa() {
  const motivo = (massaMotivo.value === '__outro' ? massaOutro.value : massaMotivo.value).trim()
  if (!motivo) {
    toast.error('Escolha o motivo do descarte')
    return
  }
  descarteAberto.value = false
  await emMassa({ acao: 'descartar', motivo }, 'Descartados')
}

function reabrirEmMassa() {
  void emMassa({ acao: 'reabrir' }, 'Reabertos')
}

// ===== Painel contextual =====
const leadAberto = ref<number | null>(null)
const tituloPainel = ref('')

function lerQuery() {
  const v = Number(consulta.value.lead)
  leadAberto.value = Number.isFinite(v) && v > 0 ? v : null
  if (!leadAberto.value) tituloPainel.value = ''
}

watch(() => consulta.value.lead, lerQuery)

function abrir(id: number) {
  const atual = leads.value.find((l) => l.id === id)
  tituloPainel.value = atual?.titulo ?? ''
  void router?.replace({ query: { ...consulta.value, lead: String(id) } })
}

function fechar() {
  const q = { ...consulta.value }
  delete q.lead
  void router?.replace({ query: q })
}

const indiceAberto = computed(() => leads.value.findIndex((l) => l.id === leadAberto.value))

function vizinho(passo: 1 | -1) {
  if (!leads.value.length) return
  const i = indiceAberto.value
  const alvo = i < 0 ? (passo > 0 ? 0 : leads.value.length - 1) : i + passo
  if (alvo < 0 || alvo >= leads.value.length) return
  abrir(leads.value[alvo].id)
}

useAtalhos(
  {
    j: () => vizinho(1),
    k: () => vizinho(-1)
  },
  { ativo: () => leadAberto.value !== null }
)

function aoAtualizar(novo: Lead) {
  const i = leads.value.findIndex((l) => l.id === novo?.id)
  if (i >= 0) leads.value[i] = { ...leads.value[i], ...novo }
  void carregar()
}

// ===== Ligar =====
function comecarALigar() {
  const query: Record<string, string> = {}
  if (listaId.value !== '') query.lista = String(listaId.value)
  void router?.push({ path: '/fila-de-ligacoes', query })
}

function principal(l: Lead): string {
  const t = telefonePrincipal(l.telefones)
  return t ? formatarTelefone(t.numero) : ''
}

function contadorDaFila(f: FilaLeads): number | null {
  const c = contadores.value
  if (!c) return null
  return c[f] ?? null
}

async function carregarApoio() {
  const tarefas: Promise<unknown>[] = [
    carregarConfig().then((c) => (config.value = c)),
    baseDeDemonstracao(auth.can('prospecting.view')).then((a) => (amostra.value = a)),
    api
      .get<LeadLista[] | null>('/leads/listas')
      .then((r) => (listas.value = Array.isArray(r) ? r : []))
      .catch(() => (listas.value = []))
  ]
  if (podeAtribuir.value) {
    tarefas.push(
      api
        .get<User[] | null>('/users')
        .then((r) => (usuarios.value = (Array.isArray(r) ? r : []).filter((u) => u.active)))
        .catch(() => (usuarios.value = []))
    )
  }
  await Promise.all(tarefas)
}

onMounted(() => {
  lerQuery()
  void carregarApoio()
  void carregar()
})
</script>

<template>
  <div class="page leads-page">
    <div class="page-head">
      <h1>Leads de prospecção</h1>
      <div class="toolbar">
        <button v-if="podeEditar" type="button" class="btn btn-primary comecar" @click="comecarALigar">Começar a ligar</button>
      </div>
    </div>

    <nav class="filas" aria-label="Filas">
      <button
        v-for="f in filas"
        :key="f"
        type="button"
        class="fila"
        :class="{ active: fila === f }"
        :data-fila="f"
        @click="escolherFila(f)"
      >
        {{ filaLeadsLabels[f] }}
        <span v-if="contadorDaFila(f) != null" class="qtd">{{ formatNumber(contadorDaFila(f)) }}</span>
      </button>
    </nav>

    <div class="chips" role="group" aria-label="Estado">
      <button type="button" class="chip" :class="{ active: estado === '' }" @click="escolherEstado('')">Todos os estados</button>
      <button
        v-for="e in ESTADOS"
        :key="e"
        type="button"
        class="chip"
        :class="{ active: estado === e }"
        :data-estado="e"
        @click="escolherEstado(e)"
      >
        {{ estadoLeadLabels[e] }}
        <span v-if="contadores" class="qtd">{{ formatNumber(contadores[e]) }}</span>
      </button>
      <span class="separador" aria-hidden="true"></span>
      <button type="button" class="chip" :class="{ active: urgencia === 'atrasado' }" data-urgencia="atrasado" @click="escolherUrgencia('atrasado')">
        <span class="semaforo red"></span> Atrasados
        <span v-if="contadores" class="qtd">{{ formatNumber(contadores.atrasados) }}</span>
      </button>
      <button type="button" class="chip" :class="{ active: urgencia === 'hoje' }" data-urgencia="hoje" @click="escolherUrgencia('hoje')">
        <span class="semaforo green"></span> Hoje
        <span v-if="contadores" class="qtd">{{ formatNumber(contadores.hoje) }}</span>
      </button>
      <button type="button" class="chip" :class="{ active: urgencia === 'nenhuma' }" data-urgencia="nenhuma" @click="escolherUrgencia('nenhuma')">
        <span class="semaforo amber"></span> Sem próxima ação
      </button>
    </div>

    <div class="toolbar filtros">
      <input v-model="busca" type="search" placeholder="Buscar por empresa, CNPJ ou telefone" aria-label="Buscar" />
      <select v-model="listaId" aria-label="Lista" @change="recomecar">
        <option value="">Todas as listas</option>
        <option v-for="l in listas" :key="l.id" :value="l.id">{{ l.nome }} ({{ l.abertos }})</option>
      </select>
      <select v-model="temperatura" aria-label="Temperatura" @change="recomecar">
        <option value="todas">Qualquer temperatura</option>
        <option v-for="(rotulo, valor) in temperaturaLabels" :key="valor" :value="valor">{{ rotulo }}</option>
      </select>
      <select v-if="podeAtribuir && fila === 'todos'" v-model="ownerId" aria-label="Dono" @change="recomecar">
        <option value="">Qualquer dono</option>
        <option v-for="u in usuarios" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
      <select v-model="ordem" aria-label="Ordenar" @change="recomecar">
        <option v-for="(rotulo, valor) in ordemLeadsLabels" :key="valor" :value="valor">Ordenar por {{ rotulo.toLowerCase() }}</option>
      </select>
    </div>

    <div v-if="selecionados.size && podeEditar" class="bulk-bar">
      <strong>{{ selecionados.size }} selecionado(s)</strong>
      <select v-if="podeAtribuir" v-model="massaDono" aria-label="Atribuir" :disabled="executando" @change="atribuir">
        <option value="" disabled>Atribuir a…</option>
        <option value="nenhum">Sem dono</option>
        <option v-for="u in usuarios" :key="u.id" :value="u.id">{{ u.name }}</option>
      </select>
      <select v-model="massaTemperatura" aria-label="Temperatura em massa" :disabled="executando" @change="mudarTemperatura">
        <option value="x" disabled>Temperatura…</option>
        <option v-for="(rotulo, valor) in temperaturaLabels" :key="valor" :value="valor">{{ rotulo }}</option>
      </select>
      <form class="etiqueta" @submit.prevent="etiquetar">
        <input v-model="massaEtiqueta" type="text" placeholder="Etiqueta" aria-label="Etiqueta" maxlength="40" />
        <button type="submit" class="btn btn-outline btn-sm" :disabled="executando || !massaEtiqueta.trim()">Aplicar</button>
      </form>
      <button type="button" class="btn btn-outline btn-sm massa-descartar" :disabled="executando" @click="abrirDescarte">Descartar</button>
      <button type="button" class="btn btn-outline btn-sm massa-reabrir" :disabled="executando" @click="reabrirEmMassa">Reabrir</button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th v-if="podeEditar" class="col-check">
              <input type="checkbox" :checked="todosMarcados" aria-label="Selecionar a página" @change="marcarTodos" />
            </th>
            <th class="sortable" title="Urgência" @click="ordenarPor('urgencia')">● {{ setaOrdem('urgencia') }}</th>
            <th class="sortable" @click="ordenarPor('nota')">Nota {{ setaOrdem('nota') }}</th>
            <th class="sortable" @click="ordenarPor('titulo')">Empresa {{ setaOrdem('titulo') }}</th>
            <th>Cidade</th>
            <th>Telefone</th>
            <th>Tentativas</th>
            <th>Último resultado</th>
            <th>Próxima ação</th>
            <th>Dono</th>
            <th>Estado</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="l in leads"
            :key="l.id"
            :class="{ aberto: l.id === leadAberto, novo: !l.visto }"
            :data-lead="l.id"
            @click="abrir(l.id)"
          >
            <td v-if="podeEditar" class="col-check" @click.stop>
              <input type="checkbox" :checked="selecionados.has(l.id)" :aria-label="`Selecionar ${l.titulo}`" @change="marcar(l.id)" />
            </td>
            <td>
              <span
                v-if="ESTADOS_LEAD_ABERTOS.includes(l.estado)"
                class="semaforo"
                :class="urgenciaTons[l.urgencia]"
                :title="urgenciaLabels[l.urgencia]"
                :data-urgencia="l.urgencia"
              ></span>
            </td>
            <td class="nota">{{ l.nota }}</td>
            <td>
              <strong>{{ l.titulo }}</strong>
              <span v-if="l.pessoa_fisica" class="badge amber pf">PF</span>
              <div v-if="l.company_name && l.company_name !== l.titulo" class="muted small">{{ l.company_name }}</div>
            </td>
            <td class="muted">{{ l.cidade ? `${l.cidade}/${l.uf}` : '—' }}</td>
            <td class="tel">{{ principal(l) || '—' }}</td>
            <td>{{ l.tentativas }}</td>
            <td class="muted">{{ l.ultimo_resultado ? resultadoLabels[l.ultimo_resultado] : '—' }}</td>
            <td class="muted" :title="l.proxima_acao_em ? formatDateTime(l.proxima_acao_em) : ''">
              {{ l.proxima_acao_em ? relativeDate(l.proxima_acao_em) : '—' }}
            </td>
            <td :class="{ muted: !l.owner_name }">{{ l.owner_name || 'Sem dono' }}</td>
            <td>
              <span class="badge" :class="estadoLeadTons[l.estado]">{{ estadoLeadLabels[l.estado] }}</span>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!carregando && !leads.length" class="empty-state">
        <strong>Nenhum lead aqui</strong>
        <template v-if="estado || urgencia || busca || listaId !== ''">Ajuste os filtros para ver outros leads.</template>
        <template v-else>Os leads chegam pelas listas de prospecção enviadas ao CRM.</template>
      </div>

      <PaginationBar
        v-if="total > perPage || page > 1"
        v-model:page="page"
        v-model:per-page="perPage"
        :total="total"
        :per-page-options="[25, 50, 100]"
        rotulo="leads"
      />
    </div>

    <SideDrawer :open="leadAberto !== null" :title="tituloPainel || 'Lead'" :largura="560" :fundo="false" @close="fechar">
      <template #actions>
        <span class="dica-teclas muted">
          <span class="tecla">K</span> <span class="tecla">J</span>
        </span>
        <button type="button" class="btn btn-outline btn-sm" :disabled="indiceAberto <= 0" title="Anterior (K)" @click="vizinho(-1)">‹</button>
        <button
          type="button"
          class="btn btn-outline btn-sm"
          :disabled="indiceAberto < 0 || indiceAberto >= leads.length - 1"
          title="Próximo (J)"
          @click="vizinho(1)"
        >
          ›
        </button>
      </template>
      <LeadPainel
        v-if="leadAberto !== null"
        :lead-id="leadAberto"
        :config="config"
        :pode-atribuir="podeAtribuir"
        :usuarios="usuarios"
        :amostra="amostra"
        @atualizado="aoAtualizar"
        @titulo="tituloPainel = $event"
      />
    </SideDrawer>

    <ModalDialog title="Descartar leads" :open="descarteAberto" @close="descarteAberto = false">
      <form @submit.prevent="descartarEmMassa">
        <p class="muted">{{ selecionados.size }} lead(s) selecionado(s).</p>
        <div class="field">
          <label for="massa-motivo">Motivo</label>
          <select id="massa-motivo" v-model="massaMotivo">
            <option value="" disabled>Escolha…</option>
            <option v-for="m in config.motivos_descarte" :key="m" :value="m">{{ m }}</option>
            <option value="__outro">Outro</option>
          </select>
        </div>
        <div v-if="massaMotivo === '__outro'" class="field">
          <label for="massa-outro">Qual?</label>
          <input id="massa-outro" v-model="massaOutro" type="text" maxlength="120" />
        </div>
        <div class="rodape">
          <button type="button" class="btn btn-outline" @click="descarteAberto = false">Cancelar</button>
          <button type="submit" class="btn btn-danger">Descartar</button>
        </div>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.filas {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--ci-border);
  margin-bottom: 12px;
  overflow-x: auto;
}

.fila {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border: none;
  border-bottom: 2px solid transparent;
  background: none;
  font-family: inherit;
  font-size: 14px;
  color: var(--ci-text-2);
  cursor: pointer;
  white-space: nowrap;
}

.fila.active {
  border-bottom-color: var(--ci-purple);
  color: var(--ci-purple-dark);
  font-weight: 600;
}

.qtd {
  font-size: 12px;
  font-weight: 600;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--ci-bg);
  color: var(--ci-text-2);
}

.chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
}

.chips .chip {
  font-family: inherit;
}

.separador {
  width: 1px;
  height: 20px;
  background: var(--ci-border);
  margin: 0 4px;
}

.filtros {
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.filtros input[type='search'] {
  flex: 1 1 240px;
}

.bulk-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  margin-bottom: 12px;
  border-radius: 10px;
  background: var(--ci-purple-tint);
}

.bulk-bar select,
.bulk-bar input {
  padding: 6px 8px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  font-family: inherit;
  font-size: 13px;
  background: var(--ci-surface);
}

.etiqueta {
  display: inline-flex;
  gap: 4px;
}

.col-check {
  width: 36px;
}

.sortable {
  cursor: pointer;
  user-select: none;
}

tbody tr {
  cursor: pointer;
}

tbody tr.aberto {
  background: var(--ci-purple-tint);
}

tbody tr.novo strong {
  font-weight: 700;
}

.nota,
.tel {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.pf {
  margin-left: 6px;
  font-size: 10px;
}

.small {
  font-size: 12px;
}

.dica-teclas {
  display: inline-flex;
  gap: 2px;
  margin-right: 4px;
}

.rodape {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

@media (max-width: 720px) {
  .comecar {
    width: 100%;
    justify-content: center;
    min-height: 44px;
  }

  .dica-teclas {
    display: none;
  }
}
</style>
