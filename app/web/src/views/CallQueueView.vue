<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '../api'
import { formatDuracao, formatNumber, mascaraCNPJ } from '../format'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { chaveLocal } from '../tenant'
import { useAtalhos } from '../composables/useAtalhos'
import { useModoFoco } from '../composables/useModoFoco'
import {
  CONFIG_PROSPECCAO_PADRAO,
  RESULTADOS,
  type ConfigProspeccao,
  type Janela,
  type Lead,
  type LeadCompleto,
  type LeadLista,
  type MotivoFilaVazia,
  type ProximoRequest,
  type ProximoResposta,
  type ResultadoResposta,
  type ResumoFila
} from '../types-prospeccao'
import ProgressBar from '../components/ui/ProgressBar.vue'
import LeadHistorico from '../components/leads/LeadHistorico.vue'
import LeadQuemE from '../components/leads/LeadQuemE.vue'
import ResultadoForm from '../components/leads/ResultadoForm.vue'
import TelefonesLead from '../components/leads/TelefonesLead.vue'
import { baseDeDemonstracao, carregarConfig, leadDeDemonstracao, textoReabre } from '../components/leads/util'

/**
 * Fila de ligações (10.4): modo foco, um lead por vez, três colunas (Quem é,
 * Ligar, Histórico e progresso). Depois do resultado avança sozinho; ao sair
 * solta a trava. Atalhos: 1 a 0 (resultados), S (pular) e Esc (sair).
 */
useModoFoco()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()
/** Query da rota atual (a tela também monta fora do roteador, nos testes da fundação). */
const consulta = computed<Record<string, any>>(() => route?.query ?? {})

/** Renovação da trava (20 min no servidor) durante uma ligação longa. */
const RENOVAR_TRAVA_MS = 8 * 60 * 1000

const config = ref<ConfigProspeccao>({ ...CONFIG_PROSPECCAO_PADRAO })
const completo = ref<LeadCompleto | null>(null)
const motivoVazio = ref<MotivoFilaVazia>('')
const janela = ref<Janela | null>(null)
const resumo = ref<ResumoFila | null>(null)
const listas = ref<LeadLista[]>([])
const listaId = ref<number | null>(null)
const carregando = ref(false)
const erro = ref('')
const amostraBase = ref(false)
/** Leads pulados nesta sessão (vão todos em `pular_ids`). */
const pulados = ref<number[]>([])
const formulario = ref<InstanceType<typeof ResultadoForm> | null>(null)

const lead = computed<Lead | null>(() => completo.value?.lead ?? null)
const amostra = computed(() => amostraBase.value || leadDeDemonstracao(lead.value))
const decisores = computed(() => completo.value?.resumo?.decisores ?? [])

// ===== Ligação: número, cronômetro e notas =====

const numeroDiscado = ref('')
const segundos = ref(0)
let inicioLigacao = 0
let relogio: ReturnType<typeof setInterval> | null = null
const notas = ref('')

function pararRelogio() {
  if (relogio) clearInterval(relogio)
  relogio = null
}

function aoLigar(numero: string) {
  numeroDiscado.value = numero
  // Nova discagem recomeça o cronômetro.
  pararRelogio()
  inicioLigacao = Date.now()
  segundos.value = 0
  relogio = setInterval(() => {
    segundos.value = Math.floor((Date.now() - inicioLigacao) / 1000)
  }, 1000)
}

function zerarLigacao() {
  pararRelogio()
  numeroDiscado.value = ''
  segundos.value = 0
  inicioLigacao = 0
}

const contexto = computed(() => ({ numero: numeroDiscado.value, duracao: segundos.value, notas: notas.value }))

function chaveNotas(id: number): string {
  return chaveLocal('fila_notas_' + id)
}

function lerRascunho(id: number): string {
  try {
    return localStorage.getItem(chaveNotas(id)) ?? ''
  } catch {
    return ''
  }
}

function guardarRascunho() {
  if (!lead.value) return
  try {
    if (notas.value) localStorage.setItem(chaveNotas(lead.value.id), notas.value)
    else localStorage.removeItem(chaveNotas(lead.value.id))
  } catch {
    /* navegador sem armazenamento: o rascunho fica só na tela */
  }
}

function apagarRascunho(id: number) {
  try {
    localStorage.removeItem(chaveNotas(id))
  } catch {
    /* sem armazenamento */
  }
}

// ===== Roteiro =====

const roteiro = computed(() => {
  const r = completo.value?.roteiro
  if (!r) return null
  const l = lead.value
  const cad = completo.value?.resumo?.cadastro
  const valores: Record<string, string> = {
    empresa: l?.titulo || cad?.nome_fantasia || cad?.razao_social || '',
    razao_social: cad?.razao_social || l?.company_name || '',
    cidade: l?.cidade || cad?.municipio || '',
    uf: l?.uf || cad?.uf || '',
    segmento: cad?.cnae || '',
    decisor: decisores.value[0]?.nome || l?.contact_name || '',
    vendedor: auth.user?.name || '',
    minha_empresa: auth.workspaceName || ''
  }
  // O servidor já troca as variáveis; aqui só completa o que tenha sobrado.
  const trocar = (texto: string) => texto.replace(/\{\{\s*([a-z_]+)\s*\}\}/g, (todo, nome: string) => valores[nome] ?? todo)
  return { titulo: trocar(r.titulo || ''), corpo: trocar(r.corpo || '') }
})

// ===== Fila =====

function normalizar(r: LeadCompleto): LeadCompleto {
  return {
    ...r,
    lead: { ...r.lead, telefones: r.lead.telefones ?? [], etiquetas: r.lead.etiquetas ?? [], emails: r.lead.emails ?? [] },
    tentativas: r.tentativas ?? [],
    alertas: r.alertas ?? [],
    contatos: r.contatos ?? []
  }
}

function mostrar(novo: LeadCompleto | null) {
  zerarLigacao()
  completo.value = novo ? normalizar(novo) : null
  notas.value = novo ? lerRascunho(novo.lead.id) : ''
}

let pedido = 0

async function proximo() {
  const meu = ++pedido
  carregando.value = true
  erro.value = ''
  const corpo: ProximoRequest = {}
  if (listaId.value) corpo.lista_id = listaId.value
  if (pulados.value.length) corpo.pular_ids = pulados.value.slice(-200)
  try {
    const r = await api.post<ProximoResposta>('/leads/fila/proximo', corpo)
    if (meu !== pedido) return
    mostrar(r?.lead ?? null)
    motivoVazio.value = r?.lead ? '' : r?.motivo_vazio || 'fila_vazia'
    janela.value = r?.janela ?? null
  } catch (e: any) {
    if (meu !== pedido) return
    mostrar(null)
    erro.value = e?.message || 'não foi possível buscar o próximo lead'
  } finally {
    if (meu === pedido) carregando.value = false
  }
}

async function carregarResumo() {
  try {
    resumo.value = await api.get<ResumoFila>('/leads/fila/resumo')
  } catch {
    /* a barra do dia é opcional */
  }
}

function pular() {
  if (!lead.value || carregando.value || formulario.value?.salvando) return
  guardarRascunho()
  if (!pulados.value.includes(lead.value.id)) pulados.value = [...pulados.value, lead.value.id]
  void proximo()
}

function recomecarPulados() {
  pulados.value = []
  void proximo()
}

async function aoSalvar(r: ResultadoResposta) {
  if (lead.value) apagarRascunho(lead.value.id)
  toast.push(r?.proximo_passo?.texto || 'Resultado registrado')
  void carregarResumo()
  await proximo()
}

function aoConflito(e: ApiError) {
  toast.error(e.message || 'este lead já não está com você')
  void proximo()
}

// ===== WhatsApp =====

const salvandoOptin = ref(false)
async function registrarOptin() {
  if (!lead.value || !completo.value) return
  salvandoOptin.value = true
  try {
    const novo = await api.put<Lead>(`/leads/${lead.value.id}`, { whatsapp_optin: true })
    completo.value = { ...completo.value, lead: { ...completo.value.lead, ...(novo ?? {}), whatsapp_optin: true } }
    toast.push('Autorização de WhatsApp registrada')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvandoOptin.value = false
  }
}

// ===== Trava =====

let renovacao: ReturnType<typeof setInterval> | null = null

function liberar(id: number | undefined) {
  if (!id) return
  api.post(`/leads/${id}/liberar`, undefined, { keepalive: true }).catch(() => {})
}

function aoSairDaPagina() {
  guardarRascunho()
  liberar(lead.value?.id)
}

// O dono da trava a renova ao ler o lead.
function renovarTrava() {
  if (!lead.value) return
  api.get(`/leads/${lead.value.id}`).catch(() => {})
}

// ===== Navegação =====

function sair() {
  void router?.push('/leads')
}

function lerLista(): number | null {
  const v = Number(consulta.value.lista)
  return Number.isFinite(v) && v > 0 ? v : null
}

function trocarLista(event: Event) {
  const v = (event.target as HTMLSelectElement).value
  const query = { ...consulta.value }
  if (v) query.lista = v
  else delete query.lista
  void router?.replace({ query })
}

watch(
  () => consulta.value.lista,
  () => {
    const nova = lerLista()
    if (nova === listaId.value) return
    listaId.value = nova
    pulados.value = []
    void proximo()
  }
)

useAtalhos(
  {
    ...Object.fromEntries(RESULTADOS.map((r) => [r.tecla, () => formulario.value?.escolher(r.resultado)])),
    s: pular,
    escape: () => {
      if (formulario.value?.escolhido) formulario.value.cancelar()
      else sair()
    }
  },
  { ativo: () => !formulario.value?.salvando }
)

const progressoDia = computed(() => ({
  feitas: resumo.value?.feitas_hoje ?? 0,
  meta: resumo.value?.meta_dia || config.value.meta_ligacoes_dia || 0
}))

onMounted(async () => {
  listaId.value = lerLista()
  window.addEventListener('pagehide', aoSairDaPagina)
  renovacao = setInterval(renovarTrava, RENOVAR_TRAVA_MS)
  void carregarConfig().then((c) => (config.value = c))
  void baseDeDemonstracao(auth.can('prospecting.view')).then((a) => (amostraBase.value = a))
  void api
    .get<LeadLista[] | null>('/leads/listas')
    .then((r) => (listas.value = Array.isArray(r) ? r : []))
    .catch(() => (listas.value = []))
  void carregarResumo()
  await proximo()
})

onBeforeUnmount(() => {
  window.removeEventListener('pagehide', aoSairDaPagina)
  if (renovacao) clearInterval(renovacao)
  pararRelogio()
  aoSairDaPagina()
})
</script>

<template>
  <div class="cockpit">
    <header class="barra">
      <div class="barra-titulo">
        <h1>Fila de ligações</h1>
        <select class="lista" :value="listaId ?? ''" aria-label="Lista" @change="trocarLista">
          <option value="">Todas as listas</option>
          <option v-for="l in listas" :key="l.id" :value="l.id">{{ l.nome }} ({{ l.abertos }})</option>
        </select>
      </div>
      <div class="barra-dia">
        <ProgressBar
          class="dia"
          :valor="progressoDia.feitas"
          :total="progressoDia.meta || undefined"
          :tom="progressoDia.meta && progressoDia.feitas >= progressoDia.meta ? 'green' : ''"
          fina
        >
          {{ formatNumber(progressoDia.feitas) }}<template v-if="progressoDia.meta"> de {{ formatNumber(progressoDia.meta) }}</template> ligações hoje
        </ProgressBar>
        <span v-if="resumo" class="muted na-fila">{{ formatNumber(resumo.na_fila) }} na fila</span>
      </div>
      <div class="barra-acoes">
        <button v-if="lead" type="button" class="btn btn-outline pular" :disabled="carregando" title="Pular (S)" @click="pular">
          Pular <span class="tecla">S</span>
        </button>
        <button type="button" class="btn btn-outline sair" title="Sair da fila (Esc)" @click="sair">
          Sair da fila <span class="tecla">Esc</span>
        </button>
      </div>
    </header>

    <p v-if="carregando && !completo" class="muted carregando">Buscando o próximo lead…</p>

    <div v-else-if="erro && !completo" class="card vazio">
      <strong>Não foi possível carregar a fila</strong>
      <p class="muted">{{ erro }}</p>
      <button type="button" class="btn btn-primary" @click="proximo">Tentar de novo</button>
    </div>

    <div v-else-if="!completo && motivoVazio === 'fora_da_janela'" class="card vazio fora-da-janela">
      <strong>Fora da janela de contato</strong>
      <p>
        {{ textoReabre(janela?.reabre_em) ? `A fila ${textoReabre(janela?.reabre_em)}.` : 'A fila reabre no próximo horário permitido.' }}
      </p>
      <p v-if="janela?.texto" class="muted">Ligações {{ janela.texto }}.</p>
      <button type="button" class="btn btn-outline" @click="sair">Ver os leads</button>
    </div>

    <div v-else-if="!completo" class="card vazio fila-em-dia">
      <strong>Fila em dia</strong>
      <p class="muted">Nenhum lead para ligar agora.</p>
      <p v-if="resumo" class="muted">
        Hoje: {{ resumo.feitas_hoje }} ligação(ões), {{ resumo.conectadas_hoje }} conectada(s), {{ resumo.reunioes_hoje }} reunião(ões).
      </p>
      <div class="vazio-acoes">
        <button v-if="pulados.length" type="button" class="btn btn-primary" @click="recomecarPulados">
          Voltar aos {{ pulados.length }} pulado(s)
        </button>
        <button type="button" class="btn btn-outline" @click="sair">Ver os leads</button>
      </div>
    </div>

    <main v-else-if="lead" class="colunas" :class="{ trocando: carregando }" :data-lead="lead.id">
      <!-- Quem é -->
      <section class="coluna quem">
        <div class="cabeca">
          <h2>{{ lead.titulo }}</h2>
          <p class="muted">
            <template v-if="lead.cidade">{{ lead.cidade }}/{{ lead.uf }}</template>
            <template v-if="lead.cnpj"> · {{ mascaraCNPJ(lead.cnpj) }}</template>
          </p>
          <p class="muted">
            Tentativa {{ lead.tentativas + 1 }}<template v-if="config.max_tentativas"> de {{ config.max_tentativas }}</template>
            <template v-if="lead.lista_nome"> · {{ lead.lista_nome }}</template>
          </p>
        </div>
        <LeadQuemE :completo="completo" />
      </section>

      <!-- Ligar -->
      <section class="coluna ligar">
        <div class="cabeca-ligar">
          <h2 class="so-celular">{{ lead.titulo }}</h2>
          <div class="cronometro" :class="{ rodando: !!numeroDiscado }" aria-live="off">
            <span class="tempo">{{ formatDuracao(segundos) }}</span>
            <span class="muted">{{ numeroDiscado ? 'em ligação' : 'clique em ligar para começar' }}</span>
            <button v-if="numeroDiscado" type="button" class="btn btn-outline btn-sm" @click="zerarLigacao">Zerar</button>
          </div>
        </div>

        <TelefonesLead
          :lead="lead"
          :decisores="decisores"
          :exigir-optin="config.exigir_optin_whatsapp"
          :amostra="amostra"
          :em-ligacao="numeroDiscado"
          :pode-editar="auth.can('leads.edit')"
          :salvando-optin="salvandoOptin"
          @ligar="aoLigar"
          @optin="registrarOptin"
        />

        <details v-if="roteiro" class="roteiro" open>
          <summary>{{ roteiro.titulo || 'Roteiro' }}</summary>
          <div class="roteiro-corpo">{{ roteiro.corpo }}</div>
        </details>

        <div class="field notas">
          <label for="fila-notas">Notas da ligação</label>
          <textarea
            id="fila-notas"
            v-model="notas"
            rows="3"
            placeholder="O que foi dito, quem atendeu, próximo passo…"
            @input="guardarRascunho"
          ></textarea>
        </div>

        <ResultadoForm
          ref="formulario"
          :completo="completo"
          :config="config"
          :contexto="contexto"
          grande
          @salvo="aoSalvar"
          @conflito="aoConflito"
        />
      </section>

      <!-- Histórico e progresso -->
      <section class="coluna historico">
        <div class="progresso card-leve">
          <h3>Hoje</h3>
          <dl class="numeros">
            <div>
              <dt>Feitas</dt>
              <dd>{{ resumo?.feitas_hoje ?? 0 }}</dd>
            </div>
            <div>
              <dt>Conectadas</dt>
              <dd>{{ resumo?.conectadas_hoje ?? 0 }}</dd>
            </div>
            <div>
              <dt>Reuniões</dt>
              <dd>{{ resumo?.reunioes_hoje ?? 0 }}</dd>
            </div>
          </dl>
          <p v-if="resumo" class="muted pequeno">
            {{ resumo.retornos_vencidos }} retorno(s) vencido(s) · {{ resumo.novos }} novo(s)
          </p>
        </div>
        <div>
          <h3>Tentativas anteriores</h3>
          <LeadHistorico :tentativas="completo.tentativas" />
        </div>
      </section>
    </main>
  </div>
</template>

<style scoped>
.cockpit {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--ci-bg);
}

.barra {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 20px;
  background: var(--ci-surface);
  border-bottom: 1px solid var(--ci-border);
  flex-wrap: wrap;
}

.barra-titulo {
  display: flex;
  align-items: center;
  gap: 12px;
}

.barra-titulo h1 {
  font-size: 17px;
  margin: 0;
  white-space: nowrap;
}

.lista {
  padding: 6px 8px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  background: var(--ci-surface);
  font-family: inherit;
  font-size: 13px;
  max-width: 220px;
}

.barra-dia {
  flex: 1 1 220px;
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.barra-dia .dia {
  flex: 1;
  font-size: 12px;
}

.na-fila {
  font-size: 12px;
  white-space: nowrap;
}

.barra-acoes {
  display: flex;
  gap: 8px;
}

.carregando {
  padding: 40px 20px;
  text-align: center;
}

.vazio {
  max-width: 520px;
  margin: 48px auto;
  padding: 28px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.vazio strong {
  font-size: 18px;
}

.vazio p {
  margin: 0;
}

.vazio-acoes {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
  justify-content: center;
}

.colunas {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) minmax(0, 0.9fr);
  gap: 16px;
  padding: 16px 20px 32px;
  align-items: start;
  transition: opacity 0.15s;
}

.colunas.trocando {
  opacity: 0.5;
  pointer-events: none;
}

.coluna {
  background: var(--ci-surface);
  border: 1px solid var(--ci-border);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}

.cabeca h2 {
  font-size: 20px;
  margin: 0 0 4px;
}

.cabeca p {
  margin: 0 0 2px;
  font-size: 13px;
}

.so-celular {
  display: none;
}

.cronometro {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--ci-bg);
}

.cronometro .tempo {
  font-size: 24px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.cronometro.rodando {
  background: var(--ci-green-tint);
}

.cronometro .muted {
  flex: 1;
  font-size: 12px;
}

.roteiro {
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  padding: 10px 12px;
}

.roteiro summary {
  font-weight: 600;
  cursor: pointer;
}

.roteiro-corpo {
  margin-top: 8px;
  white-space: pre-wrap;
  font-size: 14px;
  line-height: 1.5;
}

.notas {
  margin: 0;
}

.historico h3,
.progresso h3 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
  margin: 0 0 8px;
}

.numeros {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin: 0;
}

.numeros div {
  text-align: center;
}

.numeros dt {
  font-size: 11px;
  color: var(--ci-text-3);
}

.numeros dd {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
}

.pequeno {
  font-size: 12px;
  margin: 8px 0 0;
}

@media (max-width: 1100px) {
  .colunas {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.3fr);
  }

  .historico {
    grid-column: 1 / -1;
  }
}

/* Celular: a coluna "Ligar" vem primeiro; "Quem é" e o histórico abaixo. */
@media (max-width: 720px) {
  .barra {
    padding: 8px 12px;
    gap: 8px;
  }

  .barra-titulo {
    flex: 1 1 100%;
    justify-content: space-between;
  }

  .barra-acoes {
    flex: 1 1 100%;
  }

  .barra-acoes .btn {
    flex: 1;
    justify-content: center;
    min-height: 44px;
  }

  .barra-acoes .tecla {
    display: none;
  }

  .colunas {
    grid-template-columns: 1fr;
    padding: 10px 10px 24px;
    gap: 10px;
  }

  .ligar {
    order: 1;
  }

  .quem {
    order: 2;
  }

  .historico {
    order: 3;
  }

  .quem .cabeca h2 {
    display: none;
  }

  .so-celular {
    display: block;
    font-size: 19px;
    margin: 0 0 8px;
  }

  .coluna {
    padding: 12px;
  }
}
</style>
