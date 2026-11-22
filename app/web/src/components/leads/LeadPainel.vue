<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { api, ApiError, foiAbortado } from '../../api'
import { useAuthStore } from '../../stores/auth'
import { useToastStore } from '../../stores/toast'
import {
  estadoLeadLabels,
  estadoLeadTons,
  formatDateTime,
  formatMoney,
  mascaraCNPJ,
  motivoDescarteLabel,
  urgenciaLabels,
  urgenciaTons
} from '../../format'
import type { Pipeline, User } from '../../types'
import {
  ESTADOS_LEAD_ABERTOS,
  origemLeadLabels,
  temperaturaLabels,
  type AtualizarLead,
  type ConfigProspeccao,
  type ConverterResposta,
  type DescartarRequest,
  type Lead,
  type LeadCompleto,
  type ResultadoResposta,
  type Temperatura
} from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'
import LeadHistorico from './LeadHistorico.vue'
import LeadQuemE from './LeadQuemE.vue'
import ResultadoForm from './ResultadoForm.vue'
import TelefonesLead from './TelefonesLead.vue'

/**
 * Painel contextual do lead em /leads: ficha resumida, telefones, registrar
 * resultado (mesma rota da fila, sem trava prévia), converter em negócio,
 * descartar, reabrir e editar.
 */
const props = withDefaults(
  defineProps<{
    leadId: number
    config: ConfigProspeccao
    /** `leads.assign`: pode trocar o dono. */
    podeAtribuir?: boolean
    usuarios?: User[]
    amostra?: boolean
  }>(),
  { podeAtribuir: false, usuarios: () => [], amostra: false }
)

const emit = defineEmits<{
  /** O lead mudou (resultado, edição, conversão...): a tabela recarrega. */
  atualizado: [lead: Lead]
  /** Título para o cabeçalho da gaveta. */
  titulo: [texto: string]
}>()

const auth = useAuthStore()
const toast = useToastStore()

const completo = ref<LeadCompleto | null>(null)
const carregando = ref(false)
const erro = ref('')
let controle: AbortController | null = null

const lead = computed(() => completo.value?.lead ?? null)
const aberto = computed(() => !!lead.value && ESTADOS_LEAD_ABERTOS.includes(lead.value.estado))
const podeEditar = computed(() => auth.can('leads.edit'))
const decisores = computed(() => completo.value?.resumo?.decisores ?? [])

async function carregar() {
  controle?.abort()
  controle = new AbortController()
  carregando.value = true
  erro.value = ''
  try {
    const r = await api.get<LeadCompleto>(`/leads/${props.leadId}`, { signal: controle.signal })
    completo.value = normalizar(r)
    emit('titulo', r.lead.titulo)
  } catch (e: any) {
    if (foiAbortado(e)) return
    completo.value = null
    erro.value = e instanceof ApiError && e.status === 404 ? 'Lead não encontrado ou fora da sua carteira.' : e?.message || 'não foi possível abrir o lead'
  } finally {
    carregando.value = false
  }
}

function normalizar(r: LeadCompleto): LeadCompleto {
  return {
    ...r,
    lead: { ...r.lead, telefones: r.lead.telefones ?? [], etiquetas: r.lead.etiquetas ?? [], emails: r.lead.emails ?? [] },
    tentativas: r.tentativas ?? [],
    alertas: r.alertas ?? [],
    contatos: r.contatos ?? []
  }
}

function aplicar(novo: Lead) {
  if (completo.value) completo.value = { ...completo.value, lead: { ...completo.value.lead, ...novo } }
  emit('atualizado', novo)
}

// ===== Resultado =====

async function aoSalvarResultado(r: ResultadoResposta) {
  toast.push(r.proximo_passo?.texto || 'Resultado registrado')
  emit('atualizado', r.lead)
  await carregar()
}

function aoConflito(e: ApiError) {
  toast.error(e.message || 'este lead mudou: recarregando')
  void carregar()
}

// ===== WhatsApp =====

const salvandoOptin = ref(false)
async function registrarOptin() {
  if (!lead.value) return
  salvandoOptin.value = true
  try {
    const novo = await api.put<Lead>(`/leads/${lead.value.id}`, { whatsapp_optin: true } satisfies AtualizarLead)
    aplicar(novo ?? { ...lead.value, whatsapp_optin: true })
    toast.push('Autorização de WhatsApp registrada')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvandoOptin.value = false
  }
}

// ===== Assumir =====

async function assumir() {
  if (!lead.value || !auth.user) return
  try {
    const novo = await api.put<Lead>(`/leads/${lead.value.id}`, { owner_id: auth.user.id } satisfies AtualizarLead)
    aplicar(novo)
    toast.push('Lead assumido')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Editar =====

const editarAberto = ref(false)
const salvando = ref(false)
const edicao = reactive({
  titulo: '',
  temperatura: '' as Temperatura,
  etiquetas: '',
  valor: '',
  ownerId: null as number | null
})

function abrirEditar() {
  if (!lead.value) return
  edicao.titulo = lead.value.titulo
  edicao.temperatura = lead.value.temperatura ?? ''
  edicao.etiquetas = (lead.value.etiquetas ?? []).join(', ')
  edicao.valor = lead.value.valor_estimado != null ? String(lead.value.valor_estimado) : ''
  edicao.ownerId = lead.value.owner_id
  editarAberto.value = true
}

async function salvarEdicao() {
  if (!lead.value) return
  const titulo = edicao.titulo.trim()
  if (!titulo) {
    toast.error('Informe o título do lead')
    return
  }
  const corpo: AtualizarLead = {
    titulo,
    temperatura: edicao.temperatura,
    etiquetas: edicao.etiquetas
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean),
    valor_estimado: edicao.valor === '' ? null : Number(String(edicao.valor).replace(',', '.'))
  }
  if (props.podeAtribuir && edicao.ownerId !== lead.value.owner_id) corpo.owner_id = edicao.ownerId
  salvando.value = true
  try {
    const novo = await api.put<Lead>(`/leads/${lead.value.id}`, corpo)
    aplicar(novo)
    emit('titulo', novo?.titulo ?? titulo)
    editarAberto.value = false
    toast.push('Lead atualizado')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

// ===== Descartar =====

const descartarAberto = ref(false)
const descarte = reactive({ motivo: '', outro: '', observacao: '', bloquear: false })

function abrirDescartar() {
  descarte.motivo = ''
  descarte.outro = ''
  descarte.observacao = ''
  descarte.bloquear = false
  descartarAberto.value = true
}

async function descartar() {
  if (!lead.value) return
  const motivo = (descarte.motivo === '__outro' ? descarte.outro : descarte.motivo).trim()
  if (!motivo) {
    toast.error('Escolha o motivo do descarte')
    return
  }
  const corpo: DescartarRequest = { motivo }
  if (descarte.observacao.trim()) corpo.observacao = descarte.observacao.trim()
  if (descarte.bloquear) corpo.bloquear = true
  salvando.value = true
  try {
    const novo = await api.post<Lead>(`/leads/${lead.value.id}/descartar`, corpo)
    aplicar(novo)
    descartarAberto.value = false
    toast.push('Lead descartado')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

async function reabrir() {
  if (!lead.value) return
  try {
    const novo = await api.post<Lead>(`/leads/${lead.value.id}/reabrir`)
    aplicar(novo)
    toast.push('Lead reaberto')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Converter =====

const converterAberto = ref(false)
const funis = ref<Pipeline[]>([])
const conversao = reactive({ pipelineId: null as number | null, stageId: null as number | null, nome: '', valor: '' })
const etapas = computed(() =>
  (funis.value.find((p) => p.id === conversao.pipelineId)?.stages ?? []).filter((s) => !s.is_won && !s.is_lost)
)

async function abrirConverter() {
  if (!lead.value) return
  conversao.nome = lead.value.titulo
  conversao.valor = lead.value.valor_estimado != null ? String(lead.value.valor_estimado) : ''
  converterAberto.value = true
  if (!funis.value.length) {
    try {
      const lista = await api.get<Pipeline[] | null>('/pipelines')
      funis.value = Array.isArray(lista) ? lista : []
    } catch {
      funis.value = []
    }
  }
  conversao.pipelineId = props.config.pipeline_id ?? funis.value[0]?.id ?? null
  conversao.stageId = props.config.stage_id ?? null
  if (!etapas.value.some((s) => s.id === conversao.stageId)) conversao.stageId = etapas.value[0]?.id ?? null
}

watch(
  () => conversao.pipelineId,
  () => {
    if (!etapas.value.some((s) => s.id === conversao.stageId)) conversao.stageId = etapas.value[0]?.id ?? null
  }
)

async function converter() {
  if (!lead.value) return
  const corpo: Record<string, unknown> = {}
  if (conversao.pipelineId) corpo.pipeline_id = conversao.pipelineId
  if (conversao.stageId) corpo.stage_id = conversao.stageId
  if (conversao.nome.trim()) corpo.nome = conversao.nome.trim()
  const valor = Number(String(conversao.valor).replace(',', '.'))
  if (conversao.valor !== '' && Number.isFinite(valor)) corpo.valor = valor
  salvando.value = true
  try {
    const r = await api.post<ConverterResposta>(`/leads/${lead.value.id}/converter`, corpo)
    aplicar(r.lead)
    converterAberto.value = false
    toast.push('Lead convertido em negócio')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

watch(() => props.leadId, carregar, { immediate: true })

defineExpose({ recarregar: carregar, completo })
</script>

<template>
  <div class="lead-painel">
    <p v-if="carregando && !completo" class="muted">Carregando…</p>
    <p v-else-if="erro" class="error-text">{{ erro }}</p>

    <template v-else-if="completo && lead">
      <div class="topo">
        <div class="selos">
          <span class="badge" :class="estadoLeadTons[lead.estado]">{{ estadoLeadLabels[lead.estado] }}</span>
          <span v-if="aberto" class="urgencia">
            <span class="semaforo" :class="urgenciaTons[lead.urgencia]"></span>
            {{ urgenciaLabels[lead.urgencia] }}
            <template v-if="lead.proxima_acao_em"> · {{ formatDateTime(lead.proxima_acao_em) }}</template>
          </span>
          <span v-if="lead.temperatura" class="badge gray">{{ temperaturaLabels[lead.temperatura] }}</span>
          <span v-if="lead.pessoa_fisica" class="badge amber">PF</span>
          <span v-for="t in lead.etiquetas" :key="t" class="badge gray">{{ t }}</span>
        </div>
        <p class="muted linha">
          <template v-if="lead.cnpj">{{ mascaraCNPJ(lead.cnpj) }} · </template>
          <template v-if="lead.cidade">{{ lead.cidade }}/{{ lead.uf }} · </template>
          {{ origemLeadLabels[lead.origem] ?? lead.origem }}<template v-if="lead.lista_nome"> ({{ lead.lista_nome }})</template>
        </p>
        <p class="muted linha">
          Dono: <strong>{{ lead.owner_name || 'sem dono' }}</strong>
          · {{ lead.tentativas }} tentativa(s)
          <template v-if="lead.valor_estimado != null"> · {{ formatMoney(lead.valor_estimado) }}</template>
        </p>
        <p v-if="lead.estado === 'descartado' && lead.motivo_descarte" class="muted linha">
          Motivo do descarte: {{ motivoDescarteLabel(lead.motivo_descarte) }}
        </p>
        <p v-if="lead.deal_id" class="linha">
          <router-link :to="`/negocios/${lead.deal_id}`">Abrir o negócio</router-link>
        </p>

        <div v-if="podeEditar" class="acoes">
          <button v-if="!lead.owner_id && auth.user" type="button" class="btn btn-outline btn-sm" @click="assumir">Assumir</button>
          <button v-if="aberto" type="button" class="btn btn-primary btn-sm acao-converter" @click="abrirConverter">Converter em negócio</button>
          <button v-if="aberto" type="button" class="btn btn-outline btn-sm acao-descartar" @click="abrirDescartar">Descartar</button>
          <button v-if="lead.estado === 'descartado'" type="button" class="btn btn-outline btn-sm acao-reabrir" @click="reabrir">Reabrir</button>
          <button type="button" class="btn btn-outline btn-sm acao-editar" @click="abrirEditar">Editar</button>
          <router-link v-if="lead.company_id" :to="`/empresas/${lead.company_id}`" class="btn btn-outline btn-sm">Empresa</router-link>
        </div>
      </div>

      <section class="bloco">
        <h4>Telefones</h4>
        <TelefonesLead
          :lead="lead"
          :decisores="decisores"
          :exigir-optin="config.exigir_optin_whatsapp"
          :amostra="amostra"
          :pode-editar="podeEditar"
          :salvando-optin="salvandoOptin"
          @optin="registrarOptin"
        />
      </section>

      <section v-if="podeEditar && aberto" class="bloco">
        <h4>Registrar resultado</h4>
        <ResultadoForm :completo="completo" :config="config" @salvo="aoSalvarResultado" @conflito="aoConflito" />
      </section>

      <section class="bloco">
        <h4>Quem é</h4>
        <LeadQuemE :completo="completo" compacto />
      </section>

      <section class="bloco">
        <h4>Histórico</h4>
        <LeadHistorico :tentativas="completo.tentativas" />
      </section>
    </template>

    <ModalDialog title="Editar lead" :open="editarAberto" @close="editarAberto = false">
      <form class="form-modal" @submit.prevent="salvarEdicao">
        <div class="field">
          <label for="lead-titulo">Título</label>
          <input id="lead-titulo" v-model="edicao.titulo" type="text" required />
        </div>
        <div class="field">
          <label for="lead-temperatura">Temperatura</label>
          <select id="lead-temperatura" v-model="edicao.temperatura">
            <option v-for="(rotulo, valor) in temperaturaLabels" :key="valor" :value="valor">{{ rotulo }}</option>
          </select>
        </div>
        <div class="field">
          <label for="lead-etiquetas">Etiquetas (separe por vírgula)</label>
          <input id="lead-etiquetas" v-model="edicao.etiquetas" type="text" />
        </div>
        <div class="field">
          <label for="lead-valor">Valor estimado (R$)</label>
          <input id="lead-valor" v-model="edicao.valor" type="number" min="0" step="0.01" inputmode="decimal" />
        </div>
        <div v-if="podeAtribuir" class="field">
          <label for="lead-dono">Dono</label>
          <select id="lead-dono" v-model="edicao.ownerId">
            <option :value="null">Sem dono</option>
            <option v-for="u in usuarios" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>
        <div class="rodape">
          <button type="button" class="btn btn-outline" @click="editarAberto = false">Cancelar</button>
          <button type="submit" class="btn btn-primary" :disabled="salvando">{{ salvando ? 'Salvando…' : 'Salvar' }}</button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog title="Descartar lead" :open="descartarAberto" @close="descartarAberto = false">
      <form class="form-modal" @submit.prevent="descartar">
        <div class="field">
          <label for="descarte-motivo">Motivo</label>
          <select id="descarte-motivo" v-model="descarte.motivo" required>
            <option value="" disabled>Escolha…</option>
            <option v-for="m in config.motivos_descarte" :key="m" :value="m">{{ m }}</option>
            <option value="__outro">Outro</option>
          </select>
        </div>
        <div v-if="descarte.motivo === '__outro'" class="field">
          <label for="descarte-outro">Qual?</label>
          <input id="descarte-outro" v-model="descarte.outro" type="text" maxlength="120" />
        </div>
        <div class="field">
          <label for="descarte-obs">Observação (opcional)</label>
          <textarea id="descarte-obs" v-model="descarte.observacao" rows="2"></textarea>
        </div>
        <label class="check">
          <input v-model="descarte.bloquear" type="checkbox" />
          <span>Não contatar mais (CNPJ e telefones entram na lista de bloqueios)</span>
        </label>
        <div class="rodape">
          <button type="button" class="btn btn-outline" @click="descartarAberto = false">Cancelar</button>
          <button type="submit" class="btn btn-danger" :disabled="salvando">{{ salvando ? 'Salvando…' : 'Descartar' }}</button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog title="Converter em negócio" :open="converterAberto" @close="converterAberto = false">
      <form class="form-modal" @submit.prevent="converter">
        <div class="field">
          <label for="conv-nome">Nome do negócio</label>
          <input id="conv-nome" v-model="conversao.nome" type="text" />
        </div>
        <div class="field">
          <label for="conv-funil">Funil</label>
          <select id="conv-funil" v-model="conversao.pipelineId">
            <option v-for="p in funis" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <div class="field">
          <label for="conv-etapa">Etapa</label>
          <select id="conv-etapa" v-model="conversao.stageId">
            <option v-for="s in etapas" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </div>
        <div class="field">
          <label for="conv-valor">Valor (R$)</label>
          <input id="conv-valor" v-model="conversao.valor" type="number" min="0" step="0.01" inputmode="decimal" />
        </div>
        <div class="rodape">
          <button type="button" class="btn btn-outline" @click="converterAberto = false">Cancelar</button>
          <button type="submit" class="btn btn-primary" :disabled="salvando">{{ salvando ? 'Convertendo…' : 'Converter' }}</button>
        </div>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.lead-painel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.selos {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}

.urgencia {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ci-text-2);
}

.linha {
  margin: 0 0 4px;
  font-size: 13px;
}

.acoes {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}

.acoes a.btn {
  text-decoration: none;
}

.bloco h4 {
  margin: 0 0 8px;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
}

.form-modal .rodape {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
}

.check {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}
</style>
