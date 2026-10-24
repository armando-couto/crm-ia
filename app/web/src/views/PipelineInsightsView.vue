<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, formatMoney } from '../format'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import FunnelChart from '../components/charts/FunnelChart.vue'
import WaterfallChart from '../components/charts/WaterfallChart.vue'
import GoalBar from '../components/charts/GoalBar.vue'
import Sparkline from '../components/charts/Sparkline.vue'
import { formatDias, formatMoneyShort, formatPct } from '../components/charts/chart-utils'
import type { Pipeline, User } from '../types'
import type {
  FunilModo,
  FunilNegocio,
  FunilPeriodo,
  FunilResposta,
  FunilSelecao,
  MotivoRisco,
  MovimentacaoChave,
  SelecaoTipo,
  SparkPonto
} from '../types-funil'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

// ===== Filtros (espelhados na query string) =====
const PERIODOS: { value: FunilPeriodo; label: string }[] = [
  { value: '7', label: '7 dias' },
  { value: '30', label: '30 dias' },
  { value: '90', label: '90 dias' },
  { value: '180', label: '6 meses' },
  { value: '365', label: '12 meses' },
  { value: 'mes', label: 'Mês' },
  { value: 'trimestre', label: 'Trimestre' },
  { value: 'ano', label: 'Ano' }
]
const PERIODO_PADRAO: FunilPeriodo = '30'
const MODO_PADRAO: FunilModo = 'fluxo'

function lerPeriodo(v: unknown): FunilPeriodo {
  const s = String(v ?? '')
  return PERIODOS.some((p) => p.value === s) ? (s as FunilPeriodo) : PERIODO_PADRAO
}

function lerModo(v: unknown): FunilModo {
  return v === 'coorte' ? 'coorte' : MODO_PADRAO
}

const pipelineId = ref(Number(route.query.pipeline_id) || 0)
const periodo = ref<FunilPeriodo>(lerPeriodo(route.query.periodo))
const modo = ref<FunilModo>(lerModo(route.query.modo))
const ownerId = ref(Number(route.query.owner_id) || 0)

const pipelines = ref<Pipeline[]>([])
const users = ref<User[]>([])
const dados = ref<FunilResposta | null>(null)
const loading = ref(true)
const erro = ref(false)
const atualizadoEm = ref<Date | null>(null)

/** Vendedor sem forecast.view só vê os próprios negócios: o filtro de dono não aparece. */
const podeFiltrarDono = computed(() => auth.can('forecast.view'))

const donos = computed(() => {
  if (users.value.length) return users.value.filter((u) => u.active !== false).map((u) => ({ id: u.id, nome: u.name }))
  return (dados.value?.por_dono ?? []).map((d) => ({ id: d.owner_id, nome: d.nome }))
})

function queryFunil(): string {
  const params = new URLSearchParams({ periodo: periodo.value, modo: modo.value })
  if (pipelineId.value) params.set('pipeline_id', String(pipelineId.value))
  if (ownerId.value) params.set('owner_id', String(ownerId.value))
  return params.toString()
}

function sincronizarUrl() {
  const query: Record<string, string> = {}
  if (pipelineId.value) query.pipeline_id = String(pipelineId.value)
  if (periodo.value !== PERIODO_PADRAO) query.periodo = periodo.value
  if (modo.value !== MODO_PADRAO) query.modo = modo.value
  if (ownerId.value) query.owner_id = String(ownerId.value)
  router.replace({ query })
}

async function carregar(silencioso = false) {
  if (!silencioso) loading.value = true
  erro.value = false
  try {
    dados.value = await api.get<FunilResposta>(`/funil?${queryFunil()}`)
    atualizadoEm.value = new Date()
  } catch (e: any) {
    erro.value = true
    toast.error(e?.message || 'Não foi possível carregar o funil')
  } finally {
    loading.value = false
  }
}

let pronto = false
watch([pipelineId, periodo, modo, ownerId], () => {
  if (!pronto) return
  limparSelecao()
  sincronizarUrl()
  carregar()
})

onMounted(async () => {
  try {
    pipelines.value = await api.get<Pipeline[]>('/pipelines')
    if (!pipelineId.value || !pipelines.value.some((p) => p.id === pipelineId.value)) {
      pipelineId.value = pipelines.value[0]?.id ?? 0
    }
  } catch (e: any) {
    toast.error(e?.message || 'Não foi possível carregar os pipelines')
  }
  if (podeFiltrarDono.value) {
    try {
      users.value = await api.get<User[]>('/users')
    } catch {
      /* lista de donos vem do próprio funil (por_dono) */
    }
  }
  await carregar()
  pronto = true
})

// ===== Seleção (drill-down) =====
const selecao = ref<FunilSelecao>({ tipo: null, valor: null })

const MOVIMENTACAO_ROTULOS: Record<MovimentacaoChave, string> = {
  inicio: 'Abertos no início do período',
  criados: 'Criados no período',
  aumentos: 'Aumentos de valor (últimos 7 dias)',
  reducoes: 'Reduções de valor (últimos 7 dias)',
  adiados: 'Adiados',
  ganhos: 'Ganhos',
  perdidos: 'Perdidos',
  fim: 'Abertos agora'
}

const MOTIVO_ROTULOS: Record<MotivoRisco, string> = {
  estagnado: 'Estagnado',
  sem_atividade: 'Sem atividade',
  adiado: 'Adiado',
  vencido: 'Vencido'
}

function selecionar(tipo: SelecaoTipo, valor: number | string) {
  if (selecao.value.tipo === tipo && selecao.value.valor === valor) {
    limparSelecao()
    return
  }
  selecao.value = { tipo, valor }
}

function limparSelecao() {
  selecao.value = { tipo: null, valor: null }
}

const etapaSelecionada = computed(() => (selecao.value.tipo === 'etapa' ? Number(selecao.value.valor) : null))
const movimentacaoSelecionada = computed(() =>
  selecao.value.tipo === 'movimentacao' ? (selecao.value.valor as MovimentacaoChave) : null
)
const donoSelecionado = computed(() => (selecao.value.tipo === 'dono' ? Number(selecao.value.valor) : null))
const riscoSelecionado = computed(() => (selecao.value.tipo === 'risco' ? String(selecao.value.valor) : null))

const negocios = computed(() => dados.value?.negocios ?? [])

function nomeEtapa(stageId: number): string {
  return (
    dados.value?.funil.find((f) => f.stage_id === stageId)?.nome ??
    dados.value?.pipeline.stages.find((s) => s.id === stageId)?.name ??
    `Etapa ${stageId}`
  )
}

function filtrarMovimentacao(lista: FunilNegocio[], chave: MovimentacaoChave): FunilNegocio[] {
  const de = dados.value?.periodo.de ?? ''
  const dia = (iso: string | null | undefined) => (iso ?? '').slice(0, 10)
  switch (chave) {
    case 'inicio':
      return lista.filter((n) => dia(n.created_at) < de && (!n.closed_at || dia(n.closed_at) >= de))
    case 'criados':
      return lista.filter((n) => dia(n.created_at) >= de)
    case 'aumentos':
      return lista.filter((n) => (n.variacao_7d?.valor ?? 0) > 0)
    case 'reducoes':
      return lista.filter((n) => (n.variacao_7d?.valor ?? 0) < 0)
    case 'adiados':
      return lista.filter((n) => n.motivos?.includes('adiado') || n.adiamentos > 0)
    case 'ganhos':
      return lista.filter((n) => n.status === 'ganho')
    case 'perdidos':
      return lista.filter((n) => n.status === 'perdido')
    case 'fim':
      return lista.filter((n) => n.status === 'aberto')
  }
}

const lista = computed<FunilNegocio[]>(() => {
  const s = selecao.value
  const todos = negocios.value
  switch (s.tipo) {
    case 'etapa':
      return todos.filter((n) => n.stage_id === Number(s.valor))
    case 'dono':
      return todos.filter((n) => n.owner_id === Number(s.valor))
    case 'risco':
      return s.valor === 'todos'
        ? todos.filter((n) => (n.motivos?.length ?? 0) > 0)
        : todos.filter((n) => n.motivos?.includes(s.valor as MotivoRisco))
    case 'movimentacao':
      return filtrarMovimentacao(todos, s.valor as MovimentacaoChave)
    default:
      return todos
  }
})

const rotuloSelecao = computed(() => {
  const s = selecao.value
  switch (s.tipo) {
    case 'etapa':
      return `Etapa ${nomeEtapa(Number(s.valor))}`
    case 'movimentacao':
      return MOVIMENTACAO_ROTULOS[s.valor as MovimentacaoChave]
    case 'dono':
      return donos.value.find((d) => d.id === Number(s.valor))?.nome ?? dados.value?.por_dono.find((d) => d.owner_id === Number(s.valor))?.nome ?? 'Vendedor'
    case 'risco':
      return s.valor === 'todos' ? 'Em risco' : MOTIVO_ROTULOS[s.valor as MotivoRisco]
    default:
      return ''
  }
})

const totalLista = computed(() => lista.value.reduce((s, n) => s + n.amount, 0))

// ===== Blocos =====
const resumo = computed(() => dados.value?.resumo ?? null)
const temMeta = computed(() => (resumo.value?.meta ?? 0) > 0)

function semaforo(cobertura: number): 'red' | 'amber' | 'green' {
  if (cobertura < 2) return 'red'
  if (cobertura <= 3) return 'amber'
  return 'green'
}

interface Tile {
  key: string
  label: string
  valor: string
  hint?: string
  tom?: 'ok' | 'danger' | 'warn' | 'purple' | 'muted'
  acao?: () => void
}

const tiles = computed<Tile[]>(() => {
  const r = resumo.value
  if (!r) return []
  const cobertura = temMeta.value ? semaforo(r.cobertura) : null
  return [
    { key: 'ganho', label: 'Ganho', valor: formatMoney(r.valor_ganho), hint: `${r.ganhos} negócio(s)`, tom: 'ok', acao: () => selecionar('movimentacao', 'ganhos') },
    { key: 'ponderado', label: 'Ponderado', valor: formatMoney(r.ponderado), hint: `${r.abertos} aberto(s)`, tom: 'purple', acao: () => selecionar('movimentacao', 'fim') },
    { key: 'aberto', label: 'Aberto', valor: formatMoney(r.valor_aberto), hint: 'valor cheio em aberto' },
    {
      key: 'gap',
      label: 'Gap',
      valor: temMeta.value ? formatMoney(r.gap) : '—',
      hint: temMeta.value ? `meta ${formatMoneyShort(r.meta)}` : 'sem meta no período',
      tom: temMeta.value ? (r.gap > 0 ? 'warn' : 'ok') : 'muted'
    },
    {
      key: 'cobertura',
      label: 'Cobertura',
      valor: temMeta.value ? `${r.cobertura.toFixed(1).replace('.', ',')}×` : '—',
      hint: temMeta.value ? 'ponderado ÷ gap' : 'sem meta no período',
      tom: cobertura === 'red' ? 'danger' : cobertura === 'amber' ? 'warn' : cobertura === 'green' ? 'ok' : 'muted'
    },
    { key: 'taxa', label: 'Taxa de ganho', valor: formatPct(r.taxa_ganho), hint: `${r.ganhos} ganhos · ${r.perdidos} perdidos` },
    { key: 'ticket', label: 'Ticket médio', valor: formatMoney(r.ticket_medio), hint: 'dos negócios ganhos' },
    { key: 'ciclo', label: 'Ciclo mediano', valor: `${r.ciclo_mediano_dias} dias`, hint: 'da criação ao ganho' },
    {
      key: 'risco',
      label: 'Em risco',
      valor: String(r.em_risco),
      hint: formatMoneyShort(r.valor_em_risco),
      tom: r.em_risco > 0 ? 'danger' : 'ok',
      acao: () => selecionar('risco', 'todos')
    }
  ]
})

const sparkPontos = computed<SparkPonto[]>(() =>
  (dados.value?.ganhos_por_semana ?? []).map((g) => ({ rotulo: formatDate(g.semana), valor: g.valor }))
)

const funilModo = ref<'periodo' | 'agora'>('periodo')

/** Etapas do funil sem as de "perdido" (uma faixa de perdidos não é conversão). */
const etapasFunil = computed(() => {
  const perdidas = new Set((dados.value?.pipeline.stages ?? []).filter((s) => s.is_lost).map((s) => s.id))
  return (dados.value?.funil ?? []).filter((f) => !perdidas.has(f.stage_id))
})

const emRisco = computed(() =>
  negocios.value.filter((n) => (n.motivos?.length ?? 0) > 0 && n.status === 'aberto').sort((a, b) => b.amount - a.amount)
)
const mostrarTodosRisco = ref(false)
const riscoVisivel = computed(() => (mostrarTodosRisco.value ? emRisco.value : emRisco.value.slice(0, 8)))

const porDono = computed(() => [...(dados.value?.por_dono ?? [])].sort((a, b) => b.ponderado - a.ponderado))
const maiorDono = computed(() => Math.max(1, ...porDono.value.map((d) => Math.max(d.meta, d.valor_ganho + d.ponderado))))

function barraDono(d: { meta: number; valor_ganho: number; ponderado: number }) {
  const base = d.meta > 0 ? Math.max(d.meta, d.valor_ganho + d.ponderado) : maiorDono.value
  const ganho = Math.min(100, (d.valor_ganho / base) * 100)
  const pond = Math.min(100 - ganho, (d.ponderado / base) * 100)
  const meta = d.meta > 0 ? Math.min(100, (d.meta / base) * 100) : null
  return { ganho, pond, meta }
}

const vazio = computed(() => {
  const r = resumo.value
  return !!dados.value && !!r && negocios.value.length === 0 && r.abertos === 0 && r.ganhos === 0 && r.perdidos === 0
})

function ultimaAtividade(dias: number | null | undefined): string {
  if (dias == null) return '—'
  if (dias <= 0) return 'hoje'
  if (dias === 1) return 'ontem'
  return `há ${dias} d`
}

function classeDias(n: FunilNegocio): string {
  if (n.status !== 'aberto' || !n.prazo_dias) return ''
  if (n.dias_na_etapa > n.prazo_dias) return 'vencido'
  if (n.dias_na_etapa >= n.prazo_dias * 0.7) return 'atencao'
  return ''
}

const statusBadge: Record<string, string> = { aberto: 'blue', ganho: 'green', perdido: 'red' }

// ===== Modo reunião =====
const SLIDES = ['meta', 'movimentacao', 'funil', 'risco', 'donos', 'lista'] as const
type Slide = (typeof SLIDES)[number]
const reuniao = ref(false)
const slide = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

function visivel(bloco: Slide): boolean {
  return !reuniao.value || SLIDES[slide.value] === bloco
}

function proximoSlide() {
  slide.value = Math.min(SLIDES.length - 1, slide.value + 1)
}

function slideAnterior() {
  slide.value = Math.max(0, slide.value - 1)
}

function onTecla(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    sairReuniao()
  } else if (e.key === 'ArrowRight' || e.key === 'PageDown') {
    e.preventDefault()
    proximoSlide()
  } else if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
    e.preventDefault()
    slideAnterior()
  }
}

function onFullscreenChange() {
  if (reuniao.value && typeof document !== 'undefined' && !document.fullscreenElement) sairReuniao()
}

async function entrarReuniao() {
  reuniao.value = true
  slide.value = 0
  document.body.classList.add('modo-reuniao')
  window.addEventListener('keydown', onTecla)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  timer = setInterval(() => carregar(true), 60_000)
  const el = document.documentElement
  try {
    if (typeof el.requestFullscreen === 'function' && !document.fullscreenElement) await el.requestFullscreen()
  } catch {
    /* tela cheia indisponível (iframe, permissão): segue só com o layout de reunião */
  }
}

function sairReuniao() {
  reuniao.value = false
  document.body.classList.remove('modo-reuniao')
  window.removeEventListener('keydown', onTecla)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  if (timer) clearInterval(timer)
  timer = null
  try {
    if (document.fullscreenElement && typeof document.exitFullscreen === 'function') {
      document.exitFullscreen().catch(() => {})
    }
  } catch {
    /* já fora da tela cheia */
  }
}

onBeforeUnmount(() => {
  if (reuniao.value) sairReuniao()
})

const horaAtualizacao = computed(() =>
  atualizadoEm.value ? atualizadoEm.value.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) : '—'
)

const pipelineNome = computed(() => dados.value?.pipeline.name ?? pipelines.value.find((p) => p.id === pipelineId.value)?.name ?? '')
</script>

<template>
  <div class="funil-page" :class="{ reuniao }">
    <!-- Cabeçalho discreto do modo reunião -->
    <header v-if="reuniao" class="reuniao-head">
      <span class="rh-titulo">{{ pipelineNome }} · {{ dados?.periodo.rotulo ?? '' }}</span>
      <span class="rh-meio">
        <button type="button" class="rh-nav" :disabled="slide === 0" aria-label="Bloco anterior" @click="slideAnterior">‹</button>
        <span class="rh-contador">{{ slide + 1 }} / {{ SLIDES.length }}</span>
        <button type="button" class="rh-nav" :disabled="slide === SLIDES.length - 1" aria-label="Próximo bloco" @click="proximoSlide">›</button>
      </span>
      <span class="rh-direita">
        <span class="muted">atualizado às {{ horaAtualizacao }}</span>
        <button type="button" class="btn btn-outline btn-sm" @click="sairReuniao">Sair (Esc)</button>
      </span>
    </header>

    <!-- Barra de filtros -->
    <div v-else class="filtros">
      <div class="filtros-titulo">
        <h1>Funil</h1>
        <p class="muted">Estamos batendo a meta? O que aconteceu no pipeline? Onde ele vaza e trava? Quem está em risco?</p>
      </div>
      <div class="toolbar filtros-controles">
        <select v-model.number="pipelineId" aria-label="Pipeline" class="sel-pipeline">
          <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>

        <span class="segmentado" role="group" aria-label="Período">
          <button
            v-for="p in PERIODOS"
            :key="p.value"
            type="button"
            :class="{ active: periodo === p.value }"
            @click="periodo = p.value"
          >
            {{ p.label }}
          </button>
        </span>

        <span
          class="segmentado"
          role="group"
          aria-label="Modo"
          title="Fluxo: o que aconteceu no período (criados, ganhos, perdidos, movidos). Coorte: só negócios criados no período e até onde chegaram."
        >
          <button type="button" :class="{ active: modo === 'fluxo' }" @click="modo = 'fluxo'">Fluxo</button>
          <button type="button" :class="{ active: modo === 'coorte' }" @click="modo = 'coorte'">Coorte</button>
          <span class="ajuda" aria-hidden="true">?</span>
        </span>

        <select v-if="podeFiltrarDono" v-model.number="ownerId" aria-label="Dono" class="sel-dono">
          <option :value="0">Todos os vendedores</option>
          <option v-for="d in donos" :key="d.id" :value="d.id">{{ d.nome }}</option>
        </select>

        <button type="button" class="btn btn-primary btn-reuniao" :disabled="!dados" @click="entrarReuniao">▶ Modo reunião</button>
      </div>
    </div>

    <!-- Esqueleto -->
    <div v-if="loading && !dados" class="esqueleto" aria-busy="true" aria-label="Carregando funil">
      <div class="sk-tiles"><span v-for="i in 8" :key="i" class="sk sk-tile"></span></div>
      <span class="sk sk-bloco"></span>
      <span class="sk sk-bloco alto"></span>
    </div>

    <div v-else-if="erro && !dados" class="empty-state">
      <strong>Não foi possível carregar o funil</strong>
      Tente novamente em instantes.
      <div style="margin-top: 12px"><button class="btn btn-outline" type="button" @click="carregar()">Recarregar</button></div>
    </div>

    <div v-else-if="vazio" class="empty-state">
      <strong>Nenhum negócio neste período</strong>
      Mude o período, o pipeline ou o modo para ver movimento no funil.
    </div>

    <template v-else-if="dados && resumo">
      <!-- 1. Meta × realizado -->
      <section v-show="visivel('meta')" class="bloco" data-bloco="meta">
        <header class="bloco-head">
          <h2>Meta × realizado</h2>
          <p class="muted">Estamos batendo a meta? {{ dados.periodo.rotulo }} · {{ modo === 'fluxo' ? 'tudo o que aconteceu no período' : 'negócios criados no período' }}</p>
        </header>
        <div class="tiles">
          <component
            :is="t.acao ? 'button' : 'div'"
            v-for="t in tiles"
            :key="t.key"
            class="tile"
            :class="[t.tom, { clicavel: !!t.acao }]"
            :type="t.acao ? 'button' : undefined"
            @click="t.acao?.()"
          >
            <span class="tile-label">{{ t.label }}</span>
            <span class="tile-valor">{{ t.valor }}</span>
            <span v-if="t.hint" class="tile-hint">{{ t.hint }}</span>
          </component>
        </div>
        <div class="meta-grid">
          <div class="card">
            <h3>Progresso da meta</h3>
            <GoalBar :ganho="resumo.valor_ganho" :ponderado="resumo.ponderado" :meta="resumo.meta" :cobertura="temMeta ? resumo.cobertura : null" />
          </div>
          <div class="card">
            <h3>Ganhos por semana</h3>
            <Sparkline :pontos="sparkPontos" titulo="Ganhos por semana" :altura="64" />
          </div>
        </div>
      </section>

      <!-- 2. Movimentação -->
      <section v-show="visivel('movimentacao')" class="bloco" data-bloco="movimentacao">
        <header class="bloco-head">
          <h2>Movimentação do pipeline</h2>
          <p class="muted">O que aconteceu no período: o que entrou, cresceu, encolheu e saiu do pipeline aberto. Clique numa barra para ver os negócios.</p>
        </header>
        <div class="card">
          <WaterfallChart :movimentacao="dados.movimentacao" :selecionado="movimentacaoSelecionada" @select="selecionar('movimentacao', $event)" />
        </div>
      </section>

      <!-- 3. Funil por etapa -->
      <section v-show="visivel('funil')" class="bloco" data-bloco="funil">
        <header class="bloco-head">
          <div>
            <h2>Funil por etapa</h2>
            <p class="muted">Onde o funil vaza (conversão entre etapas) e onde trava (mediana de dias, estagnados). Clique numa etapa para filtrar.</p>
          </div>
          <span class="segmentado" role="group" aria-label="Base do funil">
            <button type="button" :class="{ active: funilModo === 'periodo' }" @click="funilModo = 'periodo'">No período</button>
            <button type="button" :class="{ active: funilModo === 'agora' }" @click="funilModo = 'agora'">Agora</button>
          </span>
        </header>
        <div class="card">
          <FunnelChart :etapas="etapasFunil" :modo="funilModo" :selecionado="etapaSelecionada" @select="selecionar('etapa', $event)" />
        </div>
      </section>

      <!-- 4. Negócios em risco -->
      <section v-show="visivel('risco')" class="bloco" data-bloco="risco">
        <header class="bloco-head">
          <div>
            <h2>Negócios em risco</h2>
            <p class="muted">Estagnados, sem atividade, adiados ou com previsão vencida — ordenados por valor. Clique num motivo para filtrar a lista.</p>
          </div>
          <span class="badge" :class="emRisco.length ? 'red' : 'green'">{{ emRisco.length }} em risco · {{ formatMoneyShort(resumo.valor_em_risco) }}</span>
        </header>
        <div class="card sem-padding">
          <p v-if="!emRisco.length" class="empty-state">
            <strong>Nenhum negócio em risco</strong>
            Pipeline saudável neste recorte.
          </p>
          <ul v-else class="risco-lista">
            <li v-for="n in riscoVisivel" :key="n.id" class="risco-item">
              <div class="risco-principal">
                <router-link :to="`/negocios/${n.id}`" class="risco-nome">{{ n.name }}</router-link>
                <span class="muted small">{{ n.company_name || '—' }} · {{ n.owner_name || 'sem dono' }} · {{ n.stage_name }}</span>
              </div>
              <div class="risco-motivos">
                <button
                  v-for="m in n.motivos"
                  :key="m"
                  type="button"
                  class="chip-motivo"
                  :class="[m, { on: riscoSelecionado === m }]"
                  @click="selecionar('risco', m)"
                >
                  {{ MOTIVO_ROTULOS[m] }}
                </button>
              </div>
              <div class="risco-metricas">
                <span :class="['dias', classeDias(n)]" :title="`Prazo saudável: ${n.prazo_dias} d`">{{ formatDias(n.dias_na_etapa) }} na etapa</span>
                <span class="muted">atividade {{ ultimaAtividade(n.dias_sem_atividade) }}</span>
                <span v-if="n.variacao_7d?.valor" class="var" :class="n.variacao_7d.valor < 0 ? 'baixa' : 'alta'">
                  {{ n.variacao_7d.valor < 0 ? '↓' : '↑' }} {{ formatMoneyShort(Math.abs(n.variacao_7d.valor)) }}
                </span>
                <span v-if="n.variacao_7d?.close_date_dias" class="var adiada">→ +{{ n.variacao_7d.close_date_dias }} d</span>
                <span v-if="n.variacao_7d?.etapa" class="muted small">{{ n.variacao_7d.etapa }}</span>
              </div>
              <div class="risco-valor">{{ formatMoney(n.amount) }}</div>
            </li>
          </ul>
          <div v-if="emRisco.length > 8" class="risco-rodape">
            <button type="button" class="btn btn-outline btn-sm" @click="mostrarTodosRisco = !mostrarTodosRisco">
              {{ mostrarTodosRisco ? 'Mostrar menos' : `Ver todos (${emRisco.length})` }}
            </button>
          </div>
        </div>
      </section>

      <!-- 5. Por vendedor -->
      <section v-show="visivel('donos')" class="bloco" data-bloco="donos">
        <header class="bloco-head">
          <h2>Por vendedor</h2>
          <p class="muted">Ganho e ponderado contra a meta de cada um. Clique numa linha para ver só os negócios da pessoa.</p>
        </header>
        <div class="table-wrap">
          <table class="data donos">
            <thead>
              <tr>
                <th>Vendedor</th>
                <th style="min-width: 220px">Ponderado × meta</th>
                <th class="num">Ganho</th>
                <th class="num">Ponderado</th>
                <th class="num">Abertos</th>
                <th class="num">Taxa</th>
                <th class="num">Em risco</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="d in porDono"
                :key="d.owner_id"
                :class="{ selecionada: donoSelecionado === d.owner_id }"
                :data-owner-id="d.owner_id"
                @click="selecionar('dono', d.owner_id)"
              >
                <td><strong>{{ d.nome }}</strong></td>
                <td>
                  <div class="barra-dono" :title="d.meta ? `Meta ${formatMoney(d.meta)}` : 'Sem meta'">
                    <span class="bd-ganho" :style="{ width: `${barraDono(d).ganho}%` }"></span>
                    <span class="bd-pond" :style="{ width: `${barraDono(d).pond}%` }"></span>
                    <span v-if="barraDono(d).meta != null" class="bd-meta" :style="{ left: `${barraDono(d).meta}%` }"></span>
                  </div>
                  <span class="muted small">{{ d.meta ? `meta ${formatMoneyShort(d.meta)}` : 'sem meta' }}</span>
                </td>
                <td class="num ok">{{ formatMoneyShort(d.valor_ganho) }} <span class="muted small">({{ d.ganhos }})</span></td>
                <td class="num">{{ formatMoneyShort(d.ponderado) }}</td>
                <td class="num">{{ d.abertos }}</td>
                <td class="num">{{ formatPct(d.taxa_ganho) }}</td>
                <td class="num" :class="{ danger: d.em_risco > 0 }">{{ d.em_risco }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="!porDono.length" class="empty-state">Sem vendedores com negócios neste recorte.</p>
        </div>
      </section>

      <!-- 6. Lista (drill-down) -->
      <section v-show="visivel('lista')" class="bloco" data-bloco="lista">
        <header class="bloco-head">
          <div>
            <h2>Negócios</h2>
            <p class="muted">{{ lista.length }} negócio(s) · {{ formatMoney(totalLista) }}</p>
          </div>
          <span v-if="selecao.tipo" class="chip-filtro">
            Filtrando por: <strong>{{ rotuloSelecao }}</strong>
            <button type="button" class="chip-x" aria-label="Limpar filtro" @click="limparSelecao">✕</button>
          </span>
        </header>
        <div class="table-wrap">
          <table class="data lista">
            <thead>
              <tr>
                <th>Negócio</th>
                <th>Empresa</th>
                <th>Dono</th>
                <th class="num">Valor</th>
                <th class="num">Ponderado</th>
                <th>Etapa</th>
                <th class="num">Dias na etapa</th>
                <th>Última atividade</th>
                <th>Fechamento</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="n in lista" :key="n.id" @click="router.push(`/negocios/${n.id}`)">
                <td>
                  <strong>{{ n.name }}</strong>
                  <span v-if="n.motivos?.length" class="motivos-mini">
                    <i v-for="m in n.motivos" :key="m" class="dot" :class="m" :title="MOTIVO_ROTULOS[m]"></i>
                  </span>
                </td>
                <td class="muted">{{ n.company_name || '—' }}</td>
                <td>{{ n.owner_name || '—' }}</td>
                <td class="num">{{ formatMoney(n.amount) }}</td>
                <td class="num muted">{{ formatMoney(n.ponderado) }}</td>
                <td><span class="badge" :class="statusBadge[n.status]">{{ n.status === 'aberto' ? n.stage_name : n.status }}</span></td>
                <td class="num"><span :class="['dias', classeDias(n)]">{{ n.status === 'aberto' ? formatDias(n.dias_na_etapa) : '—' }}</span></td>
                <td class="muted">{{ ultimaAtividade(n.dias_sem_atividade) }}</td>
                <td class="muted">{{ formatDate(n.close_date) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="!lista.length" class="empty-state">
            <strong>Nenhum negócio neste filtro</strong>
            <button type="button" class="btn btn-outline btn-sm" style="margin-top: 8px" @click="limparSelecao">Limpar filtro</button>
          </p>
        </div>
      </section>
    </template>
  </div>
</template>

<!-- Global, sem scoped: esconde a AppShell quando o body está em modo reunião. -->
<style>
body.modo-reuniao .sidebar,
body.modo-reuniao .topbar {
  display: none !important;
}

body.modo-reuniao .content {
  overflow: hidden !important;
}
</style>

<style scoped>
.funil-page {
  padding: 20px 28px 32px;
  max-width: 1400px;
  margin: 0 auto;
  --ok: var(--ci-green, #1d9e6f);
  --danger: var(--ci-red, #d64550);
  --warn: var(--ci-amber, #c78a1b);
  --teal: #0e9f8a;
}

/* ===== Filtros ===== */
.filtros {
  position: sticky;
  top: 0;
  z-index: 5;
  background: var(--ci-bg);
  padding: 6px 0 12px;
  margin-bottom: 8px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.filtros-titulo h1 {
  font-size: 22px;
}

.filtros-titulo p {
  margin: 2px 0 0;
  font-size: 13px;
}

.filtros-controles {
  gap: 8px;
}

.sel-pipeline {
  min-width: 180px;
}

.segmentado {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--ci-surface);
}

.segmentado button {
  border: none;
  background: transparent;
  padding: 8px 11px;
  font-size: 13px;
  color: var(--ci-text-2);
  cursor: pointer;
  white-space: nowrap;
}

.segmentado button:hover {
  color: var(--ci-purple);
}

.segmentado button.active {
  background: var(--ci-purple);
  color: #fff;
  font-weight: 600;
}

.ajuda {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  font-size: 11px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin: 0 8px 0 4px;
  cursor: help;
}

.btn-reuniao {
  margin-left: auto;
}

/* ===== Esqueleto ===== */
.esqueleto {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.sk-tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
}

.sk {
  display: block;
  border-radius: var(--radius);
  background: linear-gradient(90deg, var(--ci-surface) 0%, var(--ci-purple-tint) 50%, var(--ci-surface) 100%);
  background-size: 200% 100%;
  animation: shimmer 1.2s infinite linear;
}

.sk-tile {
  height: 86px;
}

.sk-bloco {
  height: 160px;
}

.sk-bloco.alto {
  height: 300px;
}

@keyframes shimmer {
  from {
    background-position: 200% 0;
  }
  to {
    background-position: -200% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .sk {
    animation: none;
  }
}

/* ===== Blocos ===== */
.bloco {
  margin-top: 26px;
}

.bloco-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.bloco-head h2 {
  font-size: 17px;
}

.bloco-head p {
  margin: 2px 0 0;
  font-size: 13px;
}

.card h3 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
  margin-bottom: 12px;
}

.card.sem-padding {
  padding: 0;
}

/* ===== Tiles ===== */
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}

.tile {
  background: var(--ci-surface);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-top: 3px solid var(--ci-purple);
  text-align: left;
  font: inherit;
  color: inherit;
  border-left: none;
  border-right: none;
  border-bottom: none;
}

.tile.clicavel {
  cursor: pointer;
  transition: transform 0.12s, box-shadow 0.12s;
}

.tile.clicavel:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-lg);
}

.tile.ok {
  border-top-color: var(--ok);
}

.tile.danger {
  border-top-color: var(--danger);
}

.tile.warn {
  border-top-color: var(--warn);
}

.tile.muted {
  border-top-color: var(--ci-border);
}

.tile-label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--ci-text-3);
  font-weight: 600;
}

.tile-valor {
  font-size: 22px;
  font-weight: 700;
  color: var(--ci-text);
  font-variant-numeric: tabular-nums;
}

.tile.danger .tile-valor {
  color: var(--danger);
}

.tile-hint {
  font-size: 12px;
  color: var(--ci-text-3);
}

.meta-grid {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: 14px;
}

@media (max-width: 900px) {
  .meta-grid {
    grid-template-columns: 1fr;
  }
}

/* ===== Risco ===== */
.risco-lista {
  list-style: none;
  margin: 0;
  padding: 0;
}

.risco-item {
  display: grid;
  grid-template-columns: minmax(200px, 2fr) minmax(160px, 1.4fr) minmax(220px, 2fr) 120px;
  gap: 14px;
  align-items: center;
  padding: 12px 18px;
  border-bottom: 1px solid var(--ci-bg);
}

.risco-item:last-child {
  border-bottom: none;
}

.risco-principal {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.risco-nome {
  font-weight: 600;
  color: var(--ci-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.risco-nome:hover {
  color: var(--ci-purple);
}

.risco-motivos {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.chip-motivo {
  border: 1px solid transparent;
  border-radius: 999px;
  padding: 2px 9px;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  background: var(--ci-bg);
  color: var(--ci-text-2);
}

.chip-motivo.estagnado,
.chip-motivo.vencido {
  background: var(--ci-red-tint);
  color: var(--danger);
}

.chip-motivo.sem_atividade {
  background: var(--ci-amber-tint);
  color: var(--warn);
}

.chip-motivo.adiado {
  background: var(--ci-blue-tint);
  color: var(--ci-blue);
}

.chip-motivo.on {
  border-color: currentColor;
}

.risco-metricas {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
  align-items: center;
}

.dias.atencao {
  color: var(--warn);
  font-weight: 600;
}

.dias.vencido {
  color: var(--danger);
  font-weight: 600;
}

.var {
  font-weight: 600;
}

.var.baixa {
  color: var(--danger);
}

.var.alta {
  color: var(--ok);
}

.var.adiada {
  color: #e07b2a;
}

.risco-valor {
  text-align: right;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.risco-rodape {
  padding: 10px 18px;
  border-top: 1px solid var(--ci-bg);
  text-align: center;
}

@media (max-width: 900px) {
  .risco-item {
    grid-template-columns: 1fr 1fr;
  }
}

/* ===== Por vendedor ===== */
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.ok {
  color: var(--ok);
}

.danger {
  color: var(--danger);
  font-weight: 600;
}

.donos tbody tr.selecionada {
  background: var(--ci-purple-tint);
}

.barra-dono {
  position: relative;
  height: 10px;
  background: var(--ci-bg);
  border-radius: 5px;
  overflow: hidden;
  display: flex;
  margin-bottom: 3px;
}

.bd-ganho {
  background: var(--ok);
  height: 100%;
}

.bd-pond {
  height: 100%;
  background: repeating-linear-gradient(45deg, var(--ci-purple) 0 3px, var(--ci-purple-tint) 3px 6px);
}

.bd-meta {
  position: absolute;
  top: -2px;
  bottom: -2px;
  width: 2px;
  background: var(--ci-text);
}

/* ===== Lista ===== */
.chip-filtro {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  border-radius: 999px;
  padding: 5px 6px 5px 12px;
  font-size: 13px;
}

.chip-x {
  border: none;
  background: var(--ci-purple);
  color: #fff;
  border-radius: 50%;
  width: 20px;
  height: 20px;
  font-size: 11px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.motivos-mini {
  display: inline-flex;
  gap: 3px;
  margin-left: 6px;
  vertical-align: middle;
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  display: inline-block;
  background: var(--ci-text-3);
}

.dot.estagnado,
.dot.vencido {
  background: var(--danger);
}

.dot.sem_atividade {
  background: var(--warn);
}

.dot.adiado {
  background: var(--ci-blue);
}

.small {
  font-size: 12px;
}

.lista tbody tr:hover,
.donos tbody tr:hover {
  cursor: pointer;
}

/* ===== Modo reunião ===== */
.funil-page.reuniao {
  max-width: none;
  padding: 12px 40px 40px;
  min-height: 100vh;
  background: var(--ci-bg);
  /* Fonte 1,25×: zoom escala tudo (gráficos inclusive); fallback só na fonte. */
  zoom: 1.25;
}

@supports not (zoom: 1) {
  .funil-page.reuniao {
    font-size: 1.25em;
  }
}

.funil-page.reuniao .bloco {
  margin-top: 16px;
}

.funil-page.reuniao .bloco-head h2 {
  font-size: 24px;
}

.reuniao-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 8px 0 10px;
  border-bottom: 1px solid var(--ci-border);
  font-size: 14px;
}

.rh-titulo {
  font-weight: 600;
}

.rh-meio {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.rh-nav {
  border: 1px solid var(--ci-border);
  background: var(--ci-surface);
  border-radius: 6px;
  width: 30px;
  height: 30px;
  font-size: 18px;
  cursor: pointer;
  line-height: 1;
}

.rh-nav:disabled {
  opacity: 0.35;
  cursor: default;
}

.rh-contador {
  font-variant-numeric: tabular-nums;
  color: var(--ci-text-3);
}

.rh-direita {
  display: inline-flex;
  align-items: center;
  gap: 12px;
}
</style>
