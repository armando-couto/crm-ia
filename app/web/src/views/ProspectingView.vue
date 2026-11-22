<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '../api'
import { formatContagem, formatNumber } from '../format'
import { useConsultaAdiada } from '../composables/useConsultaAdiada'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import {
  estadoCargaLabels,
  temFiltroObrigatorio,
  type Contagem,
  type ContagemRequest,
  type CriarListaResposta,
  type Cota,
  type EstadoCarga,
  type ResumoPlataforma,
  type Segmento
} from '../types-prospeccao'
import ProgressBar from '../components/ui/ProgressBar.vue'
import PlataformaCabecalho from '../components/prospeccao/PlataformaCabecalho.vue'
import FiltrosForm from '../components/prospeccao/FiltrosForm.vue'
import PreviaDialog from '../components/prospeccao/PreviaDialog.vue'
import GerarListaDialog from '../components/prospeccao/GerarListaDialog.vue'
import SalvarSegmentoDialog from '../components/prospeccao/SalvarSegmentoDialog.vue'
import ColarCnpjsDialog from '../components/prospeccao/ColarCnpjsDialog.vue'
import ListasTab from '../components/prospeccao/ListasTab.vue'
import SegmentosTab from '../components/prospeccao/SegmentosTab.vue'
import { carregarRotulos, deFiltros, formVazio, paraFiltros, sugerirNome, type FormFiltros } from '../components/prospeccao/filtros'

type Aba = 'nova' | 'listas' | 'segmentos'
const ABAS: { valor: Aba; rotulo: string }[] = [
  { valor: 'nova', rotulo: 'Nova lista' },
  { valor: 'listas', rotulo: 'Listas de prospecção' },
  { valor: 'segmentos', rotulo: 'Segmentos salvos' }
]
/** Base ainda não pronta: confere de novo a cada 30 s. */
const INTERVALO_BASE = 30000

// A tela também monta fora do roteador (testes da fundação): tudo com `?.`.
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

const podeGerenciar = computed(() => auth.can('prospecting.manage'))

// ===== Plataforma (cota e base) =====

const resumo = ref<ResumoPlataforma | null>(null)
const carregandoResumo = ref(true)
const erroResumo = ref('')
let timerBase: ReturnType<typeof setTimeout> | null = null

async function carregarResumo(silencioso = false) {
  if (!silencioso) carregandoResumo.value = true
  try {
    resumo.value = await api.get<ResumoPlataforma>('/prospeccao')
    erroResumo.value = ''
  } catch (e: any) {
    if (!silencioso || !resumo.value) erroResumo.value = e?.message || 'não foi possível falar com a plataforma'
  } finally {
    carregandoResumo.value = false
  }
  agendarBase()
}

function agendarBase() {
  if (timerBase) clearTimeout(timerBase)
  timerBase = null
  if (resumo.value && resumo.value.habilitada && !resumo.value.base?.pronta) {
    timerBase = setTimeout(() => void carregarResumo(true), INTERVALO_BASE)
  }
}

onBeforeUnmount(() => {
  if (timerBase) clearTimeout(timerBase)
})

const habilitada = computed(() => !!resumo.value?.habilitada)
const basePronta = computed(() => !!resumo.value?.base?.pronta)
/** Formulário só funciona com a plataforma habilitada e a base pronta. */
const bloqueado = computed(() => !habilitada.value || !basePronta.value)
const cota = computed<Cota>(() => resumo.value?.cota ?? { mes: '', limite: 0, usado: 0, restante: 0 })
const semCota = computed(() => cota.value.restante <= 0)
const rotuloCarga = computed(() => {
  const e = resumo.value?.base?.carga?.estado
  return (e && estadoCargaLabels[e as EstadoCarga]) || e || ''
})

// ===== Abas =====

function abaDaUrl(): Aba {
  const a = route?.query?.aba
  return a === 'listas' || a === 'segmentos' ? a : 'nova'
}
const aba = ref<Aba>(abaDaUrl())
watch(aba, (a) => {
  if (!router || !route) return
  if ((route.query?.aba ?? 'nova') === a) return
  void router.replace({ query: { ...route.query, aba: a === 'nova' ? undefined : a } })
})
watch(
  () => route?.query?.aba,
  () => {
    aba.value = abaDaUrl()
  }
)

// ===== Formulário e contagem ao vivo =====

const form = ref<FormFiltros>(formVazio())
/** Muda a cada segmento carregado: remonta o formulário (blocos abertos conforme o uso). */
const versaoForm = ref(0)
const segmentoAtual = ref<Segmento | null>(null)
/** Filtros do segmento como foram carregados: a contagem só atualiza o segmento se nada mudou. */
let filtrosDoSegmento = ''

const filtros = computed(() => paraFiltros(form.value))
const chaveFiltros = computed(() => JSON.stringify(filtros.value))
const podeContar = computed(() => temFiltroObrigatorio(filtros.value))
const segmentoIntacto = computed(() => !!segmentoAtual.value && chaveFiltros.value === filtrosDoSegmento)

const contagem = useConsultaAdiada(
  (req: ContagemRequest, signal: AbortSignal) => api.post<Contagem>('/prospeccao/contagem', req, { signal }),
  {
    // A base saiu do ar ou ainda não foi carregada: o topo e o formulário refletem isso.
    aoErro: (e) => {
      if (e instanceof ApiError && (e.codigo === 'base_em_carga' || e.status === 503)) void carregarResumo(true)
    }
  }
)

function recontar() {
  if (bloqueado.value || !podeContar.value) {
    contagem.limpar()
    return
  }
  const req: ContagemRequest = { filtros: filtros.value }
  if (segmentoIntacto.value && segmentoAtual.value) req.segmento_id = segmentoAtual.value.id
  contagem.pedir(req)
}

watch([chaveFiltros, bloqueado], recontar)

const parciais = computed(() => {
  const c = contagem.dados.value
  if (!c?.parciais) return [] as string[]
  const p = c.parciais
  const saida: string[] = []
  if (p.com_celular !== null && p.com_celular !== undefined) saida.push(`${formatNumber(p.com_celular)} com celular`)
  if (p.com_email !== null && p.com_email !== undefined) saida.push(`${formatNumber(p.com_email)} com e-mail`)
  if (p.com_divida !== null && p.com_divida !== undefined && form.value.divida !== 'sem') saida.push(`${formatNumber(p.com_divida)} com dívida ativa`)
  return saida
})
const totalZero = computed(() => contagem.dados.value?.total_modo === 'exato' && contagem.dados.value.total === 0)
const acoesDesabilitadas = computed(() => bloqueado.value || !podeContar.value)

function limparFiltros() {
  form.value = formVazio()
  segmentoAtual.value = null
  filtrosDoSegmento = ''
  versaoForm.value++
}

// ===== Segmentos =====

const segmentosTab = ref<InstanceType<typeof SegmentosTab> | null>(null)
const salvarAberto = ref(false)

async function usarSegmento(s: Segmento) {
  const rotulos = await carregarRotulos(s.filtros)
  form.value = deFiltros(s.filtros, s.ordem, rotulos)
  segmentoAtual.value = s
  filtrosDoSegmento = JSON.stringify(paraFiltros(form.value))
  versaoForm.value++
  aba.value = 'nova'
}

function segmentoSalvo(s: Segmento) {
  salvarAberto.value = false
  segmentoAtual.value = s
  filtrosDoSegmento = chaveFiltros.value
  toast.push(`Segmento "${s.nome}" salvo`)
  void segmentosTab.value?.carregar()
}

// ===== Prévia, gerar lista e colar CNPJs =====

const previaAberta = ref(false)
const gerarAberto = ref(false)
const colarAberto = ref(false)

function abrirGerar() {
  previaAberta.value = false
  gerarAberto.value = true
}

function listaCriada(r: CriarListaResposta) {
  gerarAberto.value = false
  colarAberto.value = false
  toast.push(`Lista "${r.lista.nome}" criada; gerando…`)
  if (router) void router.push({ name: 'prospect-list', params: { id: String(r.lista.id) } })
}

function cotaAtualizada(c: Cota) {
  if (resumo.value) resumo.value = { ...resumo.value, cota: c }
}

const nomeSugerido = computed(() => (segmentoIntacto.value && segmentoAtual.value ? segmentoAtual.value.nome : sugerirNome(form.value)))

function abrirLista(id: number) {
  if (router) void router.push({ name: 'prospect-list', params: { id: String(id) } })
}

onMounted(async () => {
  await carregarResumo()
  recontar()
})
</script>

<template>
  <div class="page prospeccao">
    <div class="page-head">
      <h1>Gerar listas</h1>
      <div v-if="resumo && habilitada && podeGerenciar" class="cabecalho-acoes">
        <button type="button" class="btn btn-outline" :disabled="bloqueado" @click="colarAberto = true">Colar CNPJs</button>
      </div>
    </div>

    <div v-if="carregandoResumo && !resumo" class="card empty-state">Carregando a prospecção…</div>

    <div v-else-if="erroResumo && !resumo" class="card empty-state">
      <strong>Não foi possível abrir a prospecção</strong>
      {{ erroResumo }}
      <div class="tentar"><button type="button" class="btn btn-outline btn-sm" @click="carregarResumo()">Tentar de novo</button></div>
    </div>

    <div v-else-if="resumo && !habilitada" class="card empty-state desabilitada">
      <strong>Prospecção indisponível</strong>
      {{ resumo.motivo || 'A prospecção não está habilitada neste ambiente.' }}
    </div>

    <template v-else-if="resumo">
      <PlataformaCabecalho :resumo="resumo" />

      <div class="abas" role="tablist" aria-label="Prospecção">
        <button
          v-for="a in ABAS"
          :key="a.valor"
          type="button"
          role="tab"
          class="aba"
          :class="{ ativa: aba === a.valor }"
          :aria-selected="aba === a.valor"
          @click="aba = a.valor"
        >
          {{ a.rotulo }}
        </button>
      </div>

      <!-- Nova lista -->
      <section v-show="aba === 'nova'" role="tabpanel" aria-label="Nova lista">
        <div v-if="!basePronta" class="card carga" role="status">
          <strong>A base de empresas ainda está sendo carregada.</strong>
          <p class="muted">
            Os filtros ficam disponíveis assim que a primeira carga terminar. Esta tela confere sozinha a cada 30 segundos.
          </p>
          <ProgressBar
            :valor="Math.round(resumo.base.carga?.percentual ?? 0)"
            :total="100"
            :mostrar-numeros="false"
            :indeterminado="!resumo.base.carga?.percentual"
          >
            Base em carga, {{ Math.round(resumo.base.carga?.percentual ?? 0) }}%
            <span v-if="rotuloCarga" class="muted"> · {{ rotuloCarga }}</span>
            <span v-if="resumo.base.carga?.etapa" class="muted"> · {{ resumo.base.carga.etapa }}</span>
          </ProgressBar>
          <p v-if="resumo.base.carga?.erro" class="error-text">{{ resumo.base.carga.erro }}</p>
        </div>

        <div v-if="segmentoAtual" class="segmento-atual">
          <span>
            Segmento: <strong>{{ segmentoAtual.nome }}</strong>
            <span v-if="!segmentoIntacto" class="muted">(filtros alterados)</span>
          </span>
          <button type="button" class="btn btn-outline btn-sm" @click="limparFiltros">Começar do zero</button>
        </div>

        <FiltrosForm :key="versaoForm" v-model="form" :disabled="bloqueado" />

        <div class="rodape-contagem card" :class="{ inativo: acoesDesabilitadas }" aria-live="polite">
          <div class="contagem">
            <template v-if="bloqueado">
              <span class="muted">A contagem fica disponível quando a base estiver pronta.</span>
            </template>
            <template v-else-if="!podeContar">
              <span class="muted">Escolha um segmento (CNAE) ou um lugar (UF ou cidade)</span>
            </template>
            <template v-else-if="contagem.dados.value">
              <span class="total" :class="{ atualizando: contagem.carregando.value }">
                <strong>{{ formatContagem(contagem.dados.value) }}</strong> empresas
              </span>
              <span v-for="p in parciais" :key="p" class="parcial">· {{ p }}</span>
              <span v-if="contagem.carregando.value" class="girando" aria-label="Contando"></span>
            </template>
            <template v-else-if="contagem.erro.value">
              <span class="error-text">{{ contagem.erro.value }}</span>
            </template>
            <template v-else>
              <span class="muted">Contando…</span>
              <span class="girando" aria-hidden="true"></span>
            </template>
            <div v-if="contagem.erro.value && contagem.dados.value" class="error-text erro-contagem">{{ contagem.erro.value }}</div>
          </div>
          <div class="botoes">
            <button type="button" class="btn btn-outline btn-sm" :disabled="bloqueado" @click="limparFiltros">Limpar</button>
            <button type="button" class="btn btn-outline" :disabled="acoesDesabilitadas || totalZero" @click="previaAberta = true">Ver prévia</button>
            <button v-if="podeGerenciar" type="button" class="btn btn-outline" :disabled="acoesDesabilitadas" @click="salvarAberto = true">
              Salvar segmento
            </button>
            <button
              v-if="podeGerenciar"
              type="button"
              class="btn btn-primary"
              :disabled="acoesDesabilitadas || totalZero || semCota"
              :title="semCota ? 'Cota do mês esgotada' : undefined"
              @click="abrirGerar"
            >
              {{ semCota ? 'Cota esgotada' : 'Gerar lista' }}
            </button>
          </div>
        </div>
      </section>

      <section v-if="aba === 'listas'" role="tabpanel" aria-label="Listas de prospecção">
        <ListasTab :pode-gerenciar="podeGerenciar" @abrir="abrirLista" @nova="aba = 'nova'" />
      </section>

      <section v-show="aba === 'segmentos'" role="tabpanel" aria-label="Segmentos salvos">
        <SegmentosTab ref="segmentosTab" :pode-gerenciar="podeGerenciar" @usar="usarSegmento" />
      </section>

      <PreviaDialog
        v-if="previaAberta"
        :open="previaAberta"
        :filtros="filtros"
        :ordem="form.ordem"
        :pode-gerar="podeGerenciar && !semCota"
        @close="previaAberta = false"
        @gerar="abrirGerar"
      />
      <GerarListaDialog
        :open="gerarAberto"
        :filtros="filtros"
        :ordem="form.ordem"
        :nome-sugerido="nomeSugerido"
        :contagem="contagem.dados.value"
        :cota="cota"
        :segmento-id="segmentoIntacto ? segmentoAtual?.id : null"
        @close="gerarAberto = false"
        @criada="listaCriada"
        @cota="cotaAtualizada"
      />
      <SalvarSegmentoDialog
        :open="salvarAberto"
        :filtros="filtros"
        :ordem="form.ordem"
        :segmento="segmentoAtual"
        :nome-sugerido="sugerirNome(form)"
        @close="salvarAberto = false"
        @salvo="segmentoSalvo"
      />
      <ColarCnpjsDialog :open="colarAberto" :cota="cota" @close="colarAberto = false" @criada="listaCriada" @cota="cotaAtualizada" />
    </template>
  </div>
</template>

<style scoped>
.cabecalho-acoes {
  display: flex;
  gap: 8px;
}
.tentar {
  margin-top: 12px;
}
.desabilitada {
  max-width: 640px;
}
.abas {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--ci-border);
  margin-bottom: 16px;
  overflow-x: auto;
}
.aba {
  border: none;
  background: none;
  padding: 10px 14px;
  font: inherit;
  font-size: 14px;
  color: var(--ci-text-2);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  cursor: pointer;
  white-space: nowrap;
}
.aba.ativa {
  color: var(--ci-purple-dark);
  border-bottom-color: var(--ci-purple);
  font-weight: 600;
}
.carga {
  padding: 16px 20px;
  margin-bottom: 14px;
}
.carga p {
  margin: 4px 0 12px;
  font-size: 13px;
}
.segmento-atual {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  padding: 8px 14px;
  margin-bottom: 12px;
  border-radius: 8px;
  background: var(--ci-purple-tint);
  font-size: 13.5px;
}
.rodape-contagem {
  position: sticky;
  bottom: 0;
  z-index: 20;
  margin: 14px 0 0;
  padding: 12px 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  box-shadow: 0 -6px 18px rgba(34, 20, 60, 0.08);
}
.contagem {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 14px;
  min-width: 0;
}
.total strong {
  font-size: 18px;
  font-variant-numeric: tabular-nums;
}
.total.atualizando {
  opacity: 0.55;
}
.parcial {
  color: var(--ci-text-2);
}
.erro-contagem {
  width: 100%;
  font-size: 12.5px;
}
.girando {
  display: inline-block;
  width: 12px;
  height: 12px;
  border: 2px solid var(--ci-border);
  border-top-color: var(--ci-purple);
  border-radius: 50%;
  animation: gira 0.7s linear infinite;
  align-self: center;
}
.botoes {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
@keyframes gira {
  to {
    transform: rotate(360deg);
  }
}
</style>
