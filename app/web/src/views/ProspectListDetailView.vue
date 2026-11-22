<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '../api'
import { formatCentavos, formatDate, formatNumber, mascaraCNPJ } from '../format'
import { formatarTelefone } from '../telefone'
import { useJobPolling } from '../composables/useJobPolling'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import {
  estadoItemLabels,
  estadoItemTons,
  jobAtivo,
  origemListaLabels,
  statusListaLabels,
  type Afetados,
  type EstadoItem,
  type FiltroItens,
  type Item,
  type ItensResposta,
  type Job,
  type Lista,
  type ListaDetalhe,
  type OrdemItens,
  type ResumoPlataforma
} from '../types-prospeccao'
import PaginationBar from '../components/ui/PaginationBar.vue'
import NotaBadge from '../components/prospeccao/NotaBadge.vue'
import ItemDrawer from '../components/prospeccao/ItemDrawer.vue'
import JobAndamento from '../components/prospeccao/JobAndamento.vue'
import EnriquecerDialog from '../components/prospeccao/EnriquecerDialog.vue'
import EnviarCrmWizard from '../components/prospeccao/EnviarCrmWizard.vue'
import ConfigurarListaDialog from '../components/prospeccao/ConfigurarListaDialog.vue'
import {
  alternarItem,
  alternarPagina,
  estaMarcado,
  limparSelecao,
  MAX_SELECAO_IDS,
  paginaInteiraMarcada,
  paginaParcial,
  paraSelecao,
  quantidadeSelecionada,
  selecaoVazia,
  selecionarTodos
} from '../components/prospeccao/selecao'

// A tela também monta fora do roteador (testes da fundação): tudo com `?.`.
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

const podeGerenciar = computed(() => auth.can('prospecting.manage'))
const listaId = computed(() => {
  const n = Number(route?.params?.id)
  return Number.isInteger(n) && n > 0 ? n : 0
})

const ESTADOS: EstadoItem[] = ['novo', 'enriquecido', 'sem_resultado', 'erro', 'enviado', 'descartado', 'duplicado', 'bloqueado']
const NOTAS_MINIMAS = [0, 30, 50, 70]

// ===== Lista e trabalho em andamento =====

const detalhe = ref<ListaDetalhe | null>(null)
const lista = computed<Lista | null>(() => detalhe.value?.lista ?? null)
const carregandoLista = ref(false)
const erroLista = ref('')
const naoEncontrada = ref(false)
const amostra = ref(false)
const mostrarJob = ref(false)
const cancelando = ref(false)

const polling = useJobPolling({
  aoMudar: () => {
    void recarregarTudo()
  },
  aoTerminar: (job) => {
    mostrarJob.value = true
    if (job.status === 'concluido') toast.push('Trabalho concluído')
    else if (job.status === 'falhou') toast.error(job.mensagem || 'O trabalho falhou')
  }
})
const jobRodando = computed(() => polling.ativo.value)

let pedidoLista = 0
async function carregarLista(silencioso = false) {
  const id = listaId.value
  if (!id) return
  const meu = ++pedidoLista
  if (!silencioso) carregandoLista.value = true
  try {
    const d = await api.get<ListaDetalhe>(`/prospeccao/listas/${id}`)
    if (meu !== pedidoLista) return
    detalhe.value = d
    erroLista.value = ''
    naoEncontrada.value = false
    const ativo = (d?.jobs ?? []).find(jobAtivo)
    // Recarregar a página retoma o acompanhamento do trabalho em curso.
    const atual = polling.job.value
    // Mesmo job já acompanhado (ou já visto terminar): não recomeça a consulta.
    const recomecar = !atual || atual.id !== ativo?.id || (jobAtivo(atual) && !polling.consultando.value)
    if (ativo && recomecar) {
      polling.acompanhar(ativo)
      mostrarJob.value = true
    }
  } catch (e: any) {
    if (meu !== pedidoLista) return
    if (e instanceof ApiError && e.status === 404) naoEncontrada.value = true
    else erroLista.value = e?.message || 'não foi possível abrir a lista'
  } finally {
    if (meu === pedidoLista) carregandoLista.value = false
  }
}

function acompanharJob(job: Job) {
  polling.acompanhar(job)
  mostrarJob.value = true
}

async function cancelarJob() {
  if (!confirm('Cancelar o trabalho em andamento? O que já foi feito continua feito.')) return
  cancelando.value = true
  try {
    await polling.cancelar()
  } catch (e: any) {
    toast.error(e?.message || 'não foi possível cancelar')
  } finally {
    cancelando.value = false
  }
}

// ===== Itens: filtros, ordem, página =====

const itens = ref<Item[]>([])
const totalItens = ref(0)
const carregandoItens = ref(false)
const erroItens = ref('')
const page = ref(1)
const perPage = ref(50)
const filtro = reactive<FiltroItens>({ estado: undefined, q: '', com_celular: false, com_decisor: false, nota_min: 0 })
const busca = ref('')
const sort = ref<OrdemItens>('nota')
const dir = ref<'asc' | 'desc'>('desc')

let pedidoItens = 0
async function carregarItens() {
  const id = listaId.value
  if (!id) return
  const meu = ++pedidoItens
  carregandoItens.value = true
  try {
    const p = new URLSearchParams({ page: String(page.value), per_page: String(perPage.value), sort: sort.value, dir: dir.value })
    if (filtro.estado) p.set('estado', filtro.estado)
    if (filtro.q?.trim()) p.set('q', filtro.q.trim())
    if (filtro.com_celular) p.set('com_celular', 'true')
    if (filtro.com_decisor) p.set('com_decisor', 'true')
    if (filtro.nota_min) p.set('nota_min', String(filtro.nota_min))
    const r = await api.get<ItensResposta>(`/prospeccao/listas/${id}/itens?${p}`)
    if (meu !== pedidoItens) return
    itens.value = r?.data ?? []
    totalItens.value = r?.pagination?.total ?? itens.value.length
    erroItens.value = ''
  } catch (e: any) {
    if (meu !== pedidoItens) return
    erroItens.value = e?.message || 'não foi possível carregar as empresas'
  } finally {
    if (meu === pedidoItens) carregandoItens.value = false
  }
}

function recarregarTudo() {
  return Promise.all([carregarLista(true), carregarItens()])
}

let timerBusca: ReturnType<typeof setTimeout> | null = null
watch(busca, (q) => {
  if (timerBusca) clearTimeout(timerBusca)
  timerBusca = setTimeout(() => {
    filtro.q = q
  }, 300)
})
onBeforeUnmount(() => {
  if (timerBusca) clearTimeout(timerBusca)
})

// Mudar o filtro muda o que "todos" quer dizer: a seleção recomeça.
watch(
  () => [filtro.estado, filtro.q, filtro.com_celular, filtro.com_decisor, filtro.nota_min],
  () => {
    limparSelecao(selecao)
    if (page.value !== 1) page.value = 1
    else void carregarItens()
  }
)
watch([page, perPage, sort, dir], () => void carregarItens())

function ordenarPor(campo: OrdemItens) {
  if (sort.value === campo) dir.value = dir.value === 'asc' ? 'desc' : 'asc'
  else {
    sort.value = campo
    dir.value = campo === 'nota' ? 'desc' : 'asc'
  }
}
function setaOrdem(campo: OrdemItens): string {
  if (sort.value !== campo) return ''
  return dir.value === 'asc' ? '↑' : '↓'
}

function limparFiltros() {
  busca.value = ''
  filtro.estado = undefined
  filtro.q = ''
  filtro.com_celular = false
  filtro.com_decisor = false
  filtro.nota_min = 0
}
const temFiltro = computed(() => !!(filtro.estado || filtro.q || filtro.com_celular || filtro.com_decisor || filtro.nota_min))

// ===== Seleção além da página =====

const selecao = reactive(selecaoVazia())
const idsPagina = computed(() => itens.value.map((i) => i.id))
const qtdSelecionada = computed(() => quantidadeSelecionada(selecao, totalItens.value))
const paginaMarcada = computed(() => paginaInteiraMarcada(selecao, idsPagina.value))
const paginaIndeterminada = computed(() => paginaParcial(selecao, idsPagina.value))
const selecaoApi = computed(() => paraSelecao(selecao, filtro))

function avisarTeto() {
  toast.error(`Dá para marcar até ${formatNumber(MAX_SELECAO_IDS)} linhas soltas; use "selecionar todos" para mais.`)
}
function alternarLinha(id: number) {
  if (!alternarItem(selecao, id)) avisarTeto()
}
function alternarCabecalho() {
  if (!alternarPagina(selecao, idsPagina.value, !paginaMarcada.value)) avisarTeto()
}
function marcarTodos() {
  selecionarTodos(selecao)
}
function desmarcarTudo() {
  limparSelecao(selecao)
}

// ===== Gaveta =====

const itemAberto = ref<number | null>(null)

// ===== Ações =====

const enriquecerAberto = ref(false)
const enviarAberto = ref(false)
const configurarAberto = ref(false)
const processando = ref(false)
const acoesBloqueadas = computed(() => jobRodando.value || processando.value || lista.value?.status === 'gerando')

function trabalhoIniciado(job: Job, mensagem: string) {
  enriquecerAberto.value = false
  enviarAberto.value = false
  limparSelecao(selecao)
  acompanharJob(job)
  toast.push(mensagem)
  void carregarLista(true)
}

function jobJaAtivo(job: Job) {
  enriquecerAberto.value = false
  enviarAberto.value = false
  acompanharJob(job)
  toast.error('Já há um trabalho em andamento nesta lista; espere terminar.')
}

async function mudarEstado(acao: 'descartar' | 'restaurar') {
  const n = qtdSelecionada.value
  if (!n || !lista.value) return
  const texto =
    acao === 'descartar'
      ? `Descartar ${formatNumber(n)} empresa${n === 1 ? '' : 's'}? Elas ficam fora do enriquecimento e do envio ao CRM (dá para restaurar).`
      : `Restaurar ${formatNumber(n)} empresa${n === 1 ? '' : 's'}?`
  if (!confirm(texto)) return
  processando.value = true
  try {
    const r = await api.post<Afetados>(`/prospeccao/listas/${lista.value.id}/itens/${acao}`, { selecao: selecaoApi.value })
    toast.push(`${formatNumber(r?.afetados ?? 0)} ${acao === 'descartar' ? 'descartada(s)' : 'restaurada(s)'}`)
    limparSelecao(selecao)
    await recarregarTudo()
  } catch (e: any) {
    if (e instanceof ApiError && e.codigo === 'job_ativo' && e.data?.job) jobJaAtivo(e.data.job)
    else toast.error(e?.message || 'não foi possível concluir')
  } finally {
    processando.value = false
  }
}

function nomeArquivo(nome: string): string {
  const base = nome
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return `${base || 'lista'}.csv`
}

const exportando = ref(false)
async function exportar() {
  if (!lista.value || exportando.value) return
  exportando.value = true
  try {
    await api.download(`/prospeccao/listas/${lista.value.id}/exportar`, nomeArquivo(lista.value.nome))
  } catch (e: any) {
    toast.error(e?.message || 'não foi possível exportar')
  } finally {
    exportando.value = false
  }
}

async function excluirLista() {
  const l = lista.value
  if (!l) return
  if (!confirm(`Excluir a lista "${l.nome}" e as ${formatNumber(l.contadores?.total ?? 0)} empresas dela? Leads já enviados ao CRM continuam lá.`)) return
  try {
    await api.delete(`/prospeccao/listas/${l.id}`)
    toast.push('Lista excluída')
    if (router) void router.push({ path: '/prospeccao', query: { aba: 'listas' } })
  } catch (e: any) {
    if (e instanceof ApiError && e.codigo === 'job_ativo' && e.data?.job) jobJaAtivo(e.data.job)
    else toast.error(e?.message || 'não foi possível excluir')
  }
}

function listaSalva(l: Lista) {
  configurarAberto.value = false
  if (detalhe.value) detalhe.value = { ...detalhe.value, lista: { ...detalhe.value.lista, ...l } }
  toast.push('Lista atualizada')
}

// ===== Contadores =====

const contadores = computed(() => lista.value?.contadores ?? null)
const estadosVisiveis = computed(() => {
  const c = contadores.value
  if (!c) return [] as EstadoItem[]
  return ESTADOS.filter((e) => (c[e] ?? 0) > 0 || filtro.estado === e)
})

// ===== Carga inicial e troca de lista =====

async function carregarPlataforma() {
  try {
    const r = await api.get<ResumoPlataforma>('/prospeccao')
    amostra.value = !!r?.base?.amostra
  } catch {
    /* a faixa de demonstração é só informativa */
  }
}

watch(
  listaId,
  (id) => {
    polling.parar()
    mostrarJob.value = false
    detalhe.value = null
    itens.value = []
    totalItens.value = 0
    itemAberto.value = null
    limparSelecao(selecao)
    naoEncontrada.value = false
    erroLista.value = ''
    if (!id) return
    if (page.value !== 1) page.value = 1
    void carregarLista()
    void carregarItens()
  },
  { immediate: true }
)
void carregarPlataforma()
</script>

<template>
  <div class="page lista-prospeccao">
    <div class="voltar">
      <RouterLink :to="{ path: '/prospeccao', query: { aba: 'listas' } }">‹ Listas de prospecção</RouterLink>
    </div>
    <div class="page-head">
      <div class="titulo">
        <h1>{{ lista?.nome || 'Lista de prospecção' }}</h1>
        <p v-if="lista" class="muted sub">
          <span class="badge" :class="lista.status === 'pronta' ? 'green' : lista.status === 'falhou' ? 'red' : 'blue'">
            {{ statusListaLabels[lista.status] ?? lista.status }}
          </span>
          {{ origemListaLabels[lista.origem] ?? lista.origem }} · criada em {{ formatDate(lista.criado_em) }}
          <span v-if="lista.criado_por_nome"> por {{ lista.criado_por_nome }}</span>
          <span v-if="lista.versao_base"> · base {{ lista.versao_base }}</span>
          <span v-if="lista.incluir_pf" class="badge amber">inclui pessoa física</span>
        </p>
        <p v-if="lista?.descricao" class="descricao">{{ lista.descricao }}</p>
      </div>
      <div v-if="lista" class="cabecalho-acoes">
        <button type="button" class="btn btn-outline" :disabled="exportando || !contadores?.total" @click="exportar">
          {{ exportando ? 'Exportando…' : 'Exportar CSV' }}
        </button>
        <template v-if="podeGerenciar">
          <button type="button" class="btn btn-outline" @click="configurarAberto = true">Roteiro e funil</button>
          <button type="button" class="btn btn-outline" :disabled="jobRodando" @click="excluirLista">Excluir</button>
        </template>
      </div>
    </div>

    <div v-if="naoEncontrada" class="card empty-state">
      <strong>Lista não encontrada</strong>
      Ela pode ter sido excluída.
    </div>
    <div v-else-if="erroLista && !lista" class="card empty-state">
      <strong>Não foi possível abrir a lista</strong>
      {{ erroLista }}
      <div class="tentar"><button type="button" class="btn btn-outline btn-sm" @click="carregarLista()">Tentar de novo</button></div>
    </div>
    <div v-else-if="carregandoLista && !lista" class="card empty-state">Carregando a lista…</div>

    <template v-else-if="lista">
      <div v-if="amostra" class="faixa amber bloco">
        <strong>Dados de demonstração.</strong> Empresas e contatos fictícios: não ligue para estes números.
      </div>
      <div v-if="lista.status === 'falhou'" class="faixa red bloco">
        A geração da lista falhou<span v-if="lista.mensagem">: {{ lista.mensagem }}</span>
      </div>
      <div v-else-if="lista.mensagem && lista.status === 'pronta'" class="faixa amber bloco">{{ lista.mensagem }}</div>

      <JobAndamento
        v-if="mostrarJob && polling.job.value"
        :job="polling.job.value"
        :pode-cancelar="podeGerenciar"
        :cancelando="cancelando"
        :erro-consulta="polling.erro.value"
        @cancelar="cancelarJob"
        @fechar="mostrarJob = false"
      />

      <div v-if="contadores" class="contadores">
        <div class="card contador">
          <span class="valor">{{ formatNumber(contadores.total) }}</span>
          <span class="rotulo">empresas</span>
        </div>
        <div class="card contador">
          <span class="valor">{{ formatNumber(contadores.com_celular) }}</span>
          <span class="rotulo">com celular</span>
        </div>
        <div class="card contador">
          <span class="valor">{{ formatNumber(contadores.com_decisor) }}</span>
          <span class="rotulo">com decisor</span>
        </div>
        <div class="card contador">
          <span class="valor">{{ formatNumber(contadores.enriquecido) }}</span>
          <span class="rotulo">enriquecidas</span>
          <span v-if="contadores.custo_centavos" class="extra">{{ formatCentavos(contadores.custo_centavos) }}</span>
        </div>
        <div class="card contador">
          <span class="valor">{{ formatNumber(contadores.enviado) }}</span>
          <span class="rotulo">enviadas ao CRM</span>
        </div>
        <div v-if="contadores.pessoa_fisica" class="card contador pf" title="MEI e empresário individual: pessoas físicas">
          <span class="valor">{{ formatNumber(contadores.pessoa_fisica) }}</span>
          <span class="rotulo">pessoa física</span>
        </div>
      </div>

      <!-- Filtros -->
      <div class="card filtros-itens">
        <div class="estados" role="group" aria-label="Filtrar por estado">
          <button type="button" class="chip" :class="{ active: !filtro.estado }" @click="filtro.estado = undefined">
            Todos <span class="qtd">{{ formatNumber(contadores?.total) }}</span>
          </button>
          <button
            v-for="e in estadosVisiveis"
            :key="e"
            type="button"
            class="chip"
            :class="{ active: filtro.estado === e }"
            @click="filtro.estado = filtro.estado === e ? undefined : e"
          >
            {{ estadoItemLabels[e] }} <span class="qtd">{{ formatNumber(contadores?.[e]) }}</span>
          </button>
        </div>
        <div class="toolbar">
          <input v-model="busca" type="search" placeholder="Buscar por nome, CNPJ ou cidade" aria-label="Buscar empresa" />
          <label class="check"><input v-model="filtro.com_celular" type="checkbox" /> Com celular</label>
          <label class="check"><input v-model="filtro.com_decisor" type="checkbox" /> Com decisor</label>
          <select v-model.number="filtro.nota_min" aria-label="Nota mínima">
            <option v-for="n in NOTAS_MINIMAS" :key="n" :value="n">{{ n ? `Nota a partir de ${n}` : 'Qualquer nota' }}</option>
          </select>
          <button v-if="temFiltro" type="button" class="btn btn-outline btn-sm" @click="limparFiltros">Limpar filtros</button>
        </div>
      </div>

      <!-- Barra de seleção e ações -->
      <div class="barra-acoes" :class="{ ativa: qtdSelecionada > 0 }">
        <div class="selecao-info">
          <template v-if="selecao.todos">
            <strong>Todas as {{ formatNumber(qtdSelecionada) }}</strong> empresas
            {{ temFiltro ? 'que casam com o filtro' : 'da lista' }} estão selecionadas<span v-if="selecao.ids.size">
              (menos {{ formatNumber(selecao.ids.size) }})</span
            >.
            <button type="button" class="link" @click="desmarcarTudo">Limpar seleção</button>
          </template>
          <template v-else-if="qtdSelecionada">
            <strong>{{ formatNumber(qtdSelecionada) }}</strong> selecionada{{ qtdSelecionada === 1 ? '' : 's' }}.
            <button
              v-if="paginaMarcada && totalItens > itens.length"
              type="button"
              class="link"
              @click="marcarTodos"
            >
              Selecionar todas as {{ formatNumber(totalItens) }}
            </button>
            <button type="button" class="link" @click="desmarcarTudo">Limpar seleção</button>
          </template>
          <template v-else>
            <span class="muted">Marque empresas para enriquecer, enviar ao CRM ou descartar.</span>
            <button v-if="totalItens > 0 && podeGerenciar" type="button" class="link" @click="marcarTodos">
              Selecionar todas as {{ formatNumber(totalItens) }}
            </button>
          </template>
        </div>
        <div v-if="podeGerenciar" class="acoes">
          <button type="button" class="btn btn-outline" :disabled="acoesBloqueadas || !qtdSelecionada" @click="enriquecerAberto = true">
            Enriquecer
          </button>
          <button type="button" class="btn btn-primary" :disabled="acoesBloqueadas || !qtdSelecionada" @click="enviarAberto = true">
            Enviar ao CRM
          </button>
          <button
            v-if="filtro.estado === 'descartado'"
            type="button"
            class="btn btn-outline"
            :disabled="acoesBloqueadas || !qtdSelecionada"
            @click="mudarEstado('restaurar')"
          >
            Restaurar
          </button>
          <button v-else type="button" class="btn btn-outline" :disabled="acoesBloqueadas || !qtdSelecionada" @click="mudarEstado('descartar')">
            Descartar
          </button>
        </div>
      </div>
      <p v-if="jobRodando && podeGerenciar" class="muted aviso-job">As ações voltam quando o trabalho em andamento terminar.</p>

      <!-- Tabela -->
      <p v-if="erroItens" class="error-text" role="alert">{{ erroItens }}</p>
      <div v-if="!itens.length && !carregandoItens" class="card empty-state">
        <template v-if="lista.status === 'gerando'">
          <strong>Gerando a lista…</strong>
          As empresas aparecem aqui conforme a geração avança.
        </template>
        <template v-else-if="temFiltro">
          <strong>Nenhuma empresa com estes filtros</strong>
          <div class="tentar"><button type="button" class="btn btn-outline btn-sm" @click="limparFiltros">Limpar filtros</button></div>
        </template>
        <template v-else>
          <strong>Lista vazia</strong>
          Nenhuma empresa entrou nesta lista.
        </template>
      </div>
      <div v-else class="card table-wrap" :class="{ carregando: carregandoItens }">
        <table class="data itens">
          <thead>
            <tr>
              <th class="sel">
                <input
                  type="checkbox"
                  aria-label="Selecionar a página"
                  :checked="paginaMarcada"
                  :indeterminate.prop="paginaIndeterminada"
                  @change="alternarCabecalho"
                />
              </th>
              <th class="ordenavel" @click="ordenarPor('nota')">Nota {{ setaOrdem('nota') }}</th>
              <th class="ordenavel" @click="ordenarPor('razao_social')">Empresa {{ setaOrdem('razao_social') }}</th>
              <th class="ordenavel" @click="ordenarPor('municipio')">Cidade {{ setaOrdem('municipio') }}</th>
              <th>Telefone</th>
              <th>Sinais</th>
              <th class="ordenavel" @click="ordenarPor('estado')">Estado {{ setaOrdem('estado') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="i in itens"
              :key="i.id"
              class="linha"
              :class="{ marcada: estaMarcado(selecao, i.id) }"
              tabindex="0"
              @click="itemAberto = i.id"
              @keydown.enter="itemAberto = i.id"
            >
              <td class="sel" @click.stop>
                <input
                  type="checkbox"
                  :aria-label="`Selecionar ${i.nome_fantasia || i.razao_social}`"
                  :checked="estaMarcado(selecao, i.id)"
                  @change="alternarLinha(i.id)"
                />
              </td>
              <td><NotaBadge :nota="i.nota" :motivos="i.nota_motivos" /></td>
              <td class="empresa">
                <div class="nome">{{ i.nome_fantasia || i.razao_social }}</div>
                <div class="muted pequeno">
                  {{ mascaraCNPJ(i.cnpj) }}<span v-if="i.cnae_descricao"> · {{ i.cnae_descricao }}</span>
                </div>
              </td>
              <td>{{ i.municipio }}/{{ i.uf }}</td>
              <td class="telefone">
                <span v-if="i.telefone_principal">{{ formatarTelefone(i.telefone_principal) }}</span>
                <span v-else class="muted">—</span>
              </td>
              <td class="selos">
                <span v-if="i.tem_celular" class="badge green">celular</span>
                <span v-if="i.tem_whatsapp" class="badge green">WhatsApp</span>
                <span v-if="i.tem_decisor" class="badge blue">decisor</span>
                <span v-if="i.tem_email" class="badge gray">e-mail</span>
                <span v-if="i.contato_compartilhado" class="badge amber" title="O único contato é compartilhado ou de contabilidade">contador</span>
                <span v-if="i.tem_divida" class="badge amber">dívida</span>
                <span v-if="i.pessoa_fisica" class="badge amber" title="MEI ou empresário individual: pessoa física">PF</span>
              </td>
              <td>
                <span class="badge" :class="estadoItemTons[i.estado]" :title="i.erro || undefined">{{ estadoItemLabels[i.estado] }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <PaginationBar v-model:page="page" v-model:per-page="perPage" :total="totalItens" :per-page-options="[25, 50, 100]" rotulo="empresas" />
      </div>

      <ItemDrawer :item-id="itemAberto" @close="itemAberto = null" />

      <EnriquecerDialog
        v-if="enriquecerAberto"
        :open="enriquecerAberto"
        :lista-id="lista.id"
        :selecao="selecaoApi"
        @close="enriquecerAberto = false"
        @iniciado="trabalhoIniciado($event, 'Enriquecimento iniciado')"
        @job-ativo="jobJaAtivo"
      />
      <EnviarCrmWizard
        v-if="enviarAberto"
        :open="enviarAberto"
        :lista-id="lista.id"
        :selecao="selecaoApi"
        :quantidade="qtdSelecionada"
        @close="enviarAberto = false"
        @iniciado="trabalhoIniciado($event, 'Envio ao CRM iniciado')"
        @job-ativo="jobJaAtivo"
      />
      <ConfigurarListaDialog :open="configurarAberto" :lista="lista" @close="configurarAberto = false" @salvo="listaSalva" />
    </template>
  </div>
</template>

<style scoped>
.voltar {
  margin-bottom: 6px;
  font-size: 13px;
}
.titulo {
  min-width: 0;
}
.sub {
  margin: 4px 0 0;
  font-size: 13px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.descricao {
  margin: 6px 0 0;
  font-size: 13.5px;
  color: var(--ci-text-2);
}
.cabecalho-acoes {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.tentar {
  margin-top: 12px;
}
.bloco {
  margin-bottom: 14px;
}
.contadores {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}
.contador {
  padding: 12px 16px;
  margin: 0;
  display: flex;
  flex-direction: column;
}
.contador .valor {
  font-size: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.contador .rotulo {
  font-size: 12.5px;
  color: var(--ci-text-3);
}
.contador .extra {
  font-size: 12px;
  color: var(--ci-text-2);
}
.contador.pf .valor {
  color: var(--ci-amber, #a86b00);
}
.filtros-itens {
  padding: 12px 16px;
  margin-bottom: 12px;
}
.estados {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}
.qtd {
  opacity: 0.7;
  margin-left: 4px;
  font-variant-numeric: tabular-nums;
}
.filtros-itens .toolbar {
  margin: 0;
  flex-wrap: wrap;
  align-items: center;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13.5px;
  cursor: pointer;
  white-space: nowrap;
}
.barra-acoes {
  position: sticky;
  top: 0;
  z-index: 15;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 10px 14px;
  margin-bottom: 8px;
  border-radius: 10px;
  background: var(--ci-surface);
  border: 1px solid var(--ci-border);
}
.barra-acoes.ativa {
  border-color: var(--ci-purple);
  background: var(--ci-purple-tint);
}
.selecao-info {
  font-size: 13.5px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: baseline;
}
.link {
  border: none;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--ci-purple-dark);
  text-decoration: underline;
  cursor: pointer;
}
.acoes {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.aviso-job {
  margin: 0 0 8px;
  font-size: 12.5px;
}
.table-wrap.carregando {
  opacity: 0.6;
}
.itens .sel {
  width: 36px;
}
.ordenavel {
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
}
.linha {
  cursor: pointer;
}
.linha.marcada td {
  background: var(--ci-purple-tint);
}
.linha:focus-visible {
  outline: 2px solid var(--ci-purple);
  outline-offset: -2px;
}
.empresa .nome {
  font-weight: 500;
}
.pequeno {
  font-size: 12px;
}
.telefone {
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.selos {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.selos .badge {
  font-size: 11px;
}
</style>
