<script setup lang="ts">
// Enriquecer empresa em três passos: fontes → diff → resumo.
import { computed, ref, watch } from 'vue'
import { api } from '../../api'
import { useAuthStore } from '../../stores/auth'
import type { Company } from '../../types'
import type {
  DiffCampo,
  EnrichApply,
  EnrichPreview,
  EnrichmentSettings,
  EnrichmentSettingsResposta,
  FonteCatalogo,
  Provedor,
  UsoMes
} from '../../types-intel'
import ModalDialog from '../ModalDialog.vue'
import SourceBadge from './SourceBadge.vue'
import { centavos, datasetsSugeridos, nomeProvedor, nomesDatasets, valorLegivel } from './util'

const props = defineProps<{
  open: boolean
  company: Company
  usoMes?: UsoMes | null
}>()

const emit = defineEmits<{ close: []; applied: [company: Company, aplicados: string[]] }>()

const auth = useAuthStore()

const passo = ref<1 | 2 | 3>(1)
const selecionados = ref<Record<Provedor, boolean>>({ receita: true, bigdatacorp: false, serasa: false })
const datasets = ref<string[]>([...datasetsSugeridos])
const ajustarDatasets = ref(false)
const forcar = ref(false)

const settings = ref<EnrichmentSettings | null>(null)
const catalogo = ref<FonteCatalogo[]>([])
const usoConfig = ref<UsoMes | null>(null)

const consultando = ref(false)
const erro = ref('')
const preview = ref<EnrichPreview | null>(null)
const marcados = ref<string[]>([])
const aplicando = ref(false)
const resultado = ref<EnrichApply | null>(null)

const descricoes: Record<Provedor, string> = {
  receita: 'Razão social, situação cadastral, CNAE, natureza, porte, endereço, telefones e sócios (QSA).',
  bigdatacorp: 'Indicadores de atividade, contatos validados, sócios com nível, presença digital, processos, KYC e mídia.',
  serasa: 'Score positivo, pendências, protestos, cheques, consultas recentes e limite de crédito sugerido.'
}

function catalogoDe(codigo: Provedor): FonteCatalogo | undefined {
  return catalogo.value.find((f) => f.codigo === codigo)
}

function descricao(codigo: Provedor): string {
  return catalogoDe(codigo)?.descricao || descricoes[codigo]
}

async function carregarConfig() {
  if (!auth.can('settings.integrations')) return
  try {
    const r = await api.get<EnrichmentSettingsResposta>('/settings/enrichment')
    settings.value = r.settings
    catalogo.value = r.fontes ?? []
    usoConfig.value = r.uso_mes ?? null
    if (r.settings?.bigdatacorp?.datasets_padrao?.length) datasets.value = [...r.settings.bigdatacorp.datasets_padrao]
  } catch {
    /* sem catálogo: mostra os cartões sem preços */
  }
}

function reiniciar() {
  passo.value = 1
  selecionados.value = { receita: true, bigdatacorp: false, serasa: false }
  datasets.value = [...datasetsSugeridos]
  ajustarDatasets.value = false
  forcar.value = false
  erro.value = ''
  preview.value = null
  marcados.value = []
  resultado.value = null
}

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    reiniciar()
    carregarConfig()
  },
  { immediate: true }
)

// ===== Passo 1: fontes e custos =====
const precos = computed<Record<string, number>>(() => {
  const dos = settings.value?.bigdatacorp?.precos_centavos
  if (dos && Object.keys(dos).length) return dos
  const out: Record<string, number> = {}
  for (const d of catalogoDe('bigdatacorp')?.datasets ?? []) if (typeof d.preco_centavos === 'number') out[d.codigo] = d.preco_centavos
  return out
})

const temPrecos = computed(() => Object.keys(precos.value).length > 0)

const datasetsDisponiveis = computed(() => {
  const doCatalogo = catalogoDe('bigdatacorp')?.datasets
  if (doCatalogo?.length) return doCatalogo.map((d) => ({ codigo: d.codigo, nome: d.nome || nomesDatasets[d.codigo] || d.codigo }))
  return Object.keys(nomesDatasets).map((codigo) => ({ codigo, nome: nomesDatasets[codigo] }))
})

const bdcHabilitado = computed(() => (settings.value ? settings.value.bigdatacorp.habilitado : true))
const bdcDemo = computed(() => !!settings.value?.bigdatacorp.demo)
const serasaHabilitado = computed(() => (settings.value ? settings.value.serasa.habilitado : true))
const serasaDemo = computed(() => !!settings.value?.serasa.demo)

const custoBdc = computed(() => {
  if (!temPrecos.value) return null
  return datasets.value.reduce((s, d) => s + (precos.value[d] ?? 0), 0)
})
const custoSerasa = computed(() => settings.value?.serasa.preco_centavos ?? null)

const custoEstimado = computed(() => {
  let total = 0
  if (selecionados.value.bigdatacorp && !bdcDemo.value) total += custoBdc.value ?? 0
  if (selecionados.value.serasa && !serasaDemo.value) total += custoSerasa.value ?? 0
  return total
})

function rotuloCusto(codigo: Provedor): string {
  if (codigo === 'receita') return 'grátis'
  if (codigo === 'bigdatacorp') {
    if (!bdcHabilitado.value) return 'não configurado'
    if (bdcDemo.value) return 'demonstração'
    const n = `${datasets.value.length} ${datasets.value.length === 1 ? 'conjunto' : 'conjuntos'}`
    return custoBdc.value !== null ? `≈ ${centavos(custoBdc.value)} · ${n}` : n
  }
  if (!serasaHabilitado.value) return 'não configurado'
  if (serasaDemo.value) return 'demonstração'
  return custoSerasa.value !== null ? `≈ ${centavos(custoSerasa.value)} · por consulta` : 'por contrato'
}

function classeCusto(codigo: Provedor): string {
  const r = rotuloCusto(codigo)
  if (r === 'grátis') return 'free'
  if (r === 'demonstração') return 'demo'
  if (r === 'não configurado') return 'off'
  return ''
}

const uso = computed(() => props.usoMes ?? usoConfig.value)

const algumSelecionado = computed(() => Object.values(selecionados.value).some(Boolean))

const demoNoPasso1 = computed(() => (selecionados.value.bigdatacorp && bdcDemo.value) || (selecionados.value.serasa && serasaDemo.value))

function alternarDataset(codigo: string) {
  datasets.value = datasets.value.includes(codigo) ? datasets.value.filter((d) => d !== codigo) : [...datasets.value, codigo]
}

// ===== Passo 2: consulta e diff =====
async function consultar() {
  if (!algumSelecionado.value) return
  consultando.value = true
  erro.value = ''
  try {
    const provedores = (Object.keys(selecionados.value) as Provedor[]).filter((p) => selecionados.value[p])
    const body: Record<string, unknown> = { provedores, forcar: forcar.value }
    if (selecionados.value.bigdatacorp) body.datasets = datasets.value
    const r = await api.post<EnrichPreview>(`/companies/${props.company.id}/enrich/preview`, body)
    preview.value = r
    marcados.value = (r.diff ?? []).filter((d) => d.sugerido).map((d) => d.campo)
    passo.value = 2
  } catch (e: any) {
    erro.value = e.message
  } finally {
    consultando.value = false
  }
}

const diff = computed<DiffCampo[]>(() => preview.value?.diff ?? [])
const consultas = computed(() => preview.value?.consultas ?? [])
const consultasOk = computed(() => consultas.value.filter((c) => c.status === 'ok'))
const demoNoResultado = computed(() => consultas.value.some((c) => c.demo))

function estaMarcado(campo: string): boolean {
  return marcados.value.includes(campo)
}

function alternarCampo(campo: string) {
  marcados.value = estaMarcado(campo) ? marcados.value.filter((c) => c !== campo) : [...marcados.value, campo]
}

function marcarSoVazios() {
  marcados.value = diff.value.filter((d) => valorLegivel(d.campo, d.atual) === '').map((d) => d.campo)
}

function marcarTodos() {
  marcados.value = diff.value.map((d) => d.campo)
}

const statusConsulta: Record<string, { rotulo: string; classe: string }> = {
  ok: { rotulo: 'ok', classe: 'green' },
  sem_resultado: { rotulo: 'sem resultado', classe: 'amber' },
  erro: { rotulo: 'erro', classe: 'red' }
}

// ===== Passo 3: aplicar e resumir =====
async function aplicar() {
  if (!preview.value) return
  aplicando.value = true
  erro.value = ''
  try {
    const r = await api.post<EnrichApply>(`/companies/${props.company.id}/enrich/apply`, {
      enrichment_ids: preview.value.enrichment_ids,
      campos: marcados.value
    })
    resultado.value = r
    passo.value = 3
    emit('applied', r.company, r.aplicados ?? [])
  } catch (e: any) {
    erro.value = e.message
  } finally {
    aplicando.value = false
  }
}

const resumoPorFonte = computed(() => {
  const aplicados = new Set(resultado.value?.aplicados ?? [])
  const contagem: Record<string, number> = {}
  for (const d of diff.value) if (aplicados.has(d.campo)) contagem[d.provedor] = (contagem[d.provedor] ?? 0) + 1
  return Object.entries(contagem).map(([provedor, n]) => ({ provedor, n }))
})

const titulo = computed(() => {
  if (passo.value === 1) return 'Enriquecer empresa · 1 de 3 — Fontes'
  if (passo.value === 2) return 'Enriquecer empresa · 2 de 3 — Resultado'
  return 'Enriquecer empresa · 3 de 3 — Resumo'
})

function fechar() {
  emit('close')
}
</script>

<template>
  <ModalDialog :title="titulo" :open="open" wide @close="fechar">
    <ol class="steps" aria-label="Passos">
      <li :class="{ on: passo === 1, done: passo > 1 }">Fontes</li>
      <li :class="{ on: passo === 2, done: passo > 2 }">Resultado</li>
      <li :class="{ on: passo === 3 }">Resumo</li>
    </ol>

    <!-- ===== Passo 1 ===== -->
    <section v-if="passo === 1" class="step step-1">
      <p class="intro">
        Consultar <strong>{{ company.name }}</strong> <span class="muted">({{ company.cnpj }})</span>. Nada é gravado sem a sua revisão no próximo passo.
      </p>

      <div v-if="demoNoPasso1" class="demo-banner">Modo demonstração: a fonte paga devolve dados fictícios, rotulados, só para apresentar o recurso.</div>

      <div class="sources">
        <label class="source" :class="{ selected: selecionados.receita }">
          <input v-model="selecionados.receita" type="checkbox" />
          <span class="src-head">
            <strong>Receita Federal</strong>
            <span class="cost" :class="classeCusto('receita')">{{ rotuloCusto('receita') }}</span>
          </span>
          <small>{{ descricao('receita') }}</small>
        </label>

        <label class="source" :class="{ selected: selecionados.bigdatacorp, disabled: !bdcHabilitado }">
          <input v-model="selecionados.bigdatacorp" type="checkbox" :disabled="!bdcHabilitado" />
          <span class="src-head">
            <strong>BigDataCorp</strong>
            <span class="cost" :class="classeCusto('bigdatacorp')">{{ rotuloCusto('bigdatacorp') }}</span>
          </span>
          <small>{{ descricao('bigdatacorp') }}</small>
        </label>

        <label class="source" :class="{ selected: selecionados.serasa, disabled: !serasaHabilitado }">
          <input v-model="selecionados.serasa" type="checkbox" :disabled="!serasaHabilitado" />
          <span class="src-head">
            <strong>Serasa Experian</strong>
            <span class="cost" :class="classeCusto('serasa')">{{ rotuloCusto('serasa') }}</span>
          </span>
          <small>{{ descricao('serasa') }}</small>
        </label>
      </div>

      <div v-if="selecionados.bigdatacorp" class="datasets">
        <button type="button" class="link" @click="ajustarDatasets = !ajustarDatasets">
          {{ ajustarDatasets ? 'Ocultar conjuntos' : 'Ajustar conjuntos da BigDataCorp' }}
        </button>
        <ul v-if="ajustarDatasets" class="dataset-list">
          <li v-for="d in datasetsDisponiveis" :key="d.codigo">
            <label>
              <input type="checkbox" :checked="datasets.includes(d.codigo)" @change="alternarDataset(d.codigo)" />
              <span>{{ d.nome }}</span>
              <span v-if="precos[d.codigo] !== undefined" class="muted price">{{ centavos(precos[d.codigo]) }}</span>
            </label>
          </li>
        </ul>
      </div>

      <div class="options">
        <label class="check"><input v-model="forcar" type="checkbox" /> Forçar nova consulta (ignorar o cache de 30 dias)</label>
        <p v-if="uso" class="usage">
          Uso neste mês: <strong>{{ uso.consultas }}</strong> {{ uso.consultas === 1 ? 'consulta' : 'consultas' }} ·
          <strong>{{ centavos(uso.custo_centavos) }}</strong>
          <template v-if="uso.limite_centavos"> de {{ centavos(uso.limite_centavos) }}</template>
        </p>
        <p v-if="custoEstimado" class="estimate">Custo estimado desta consulta: <strong>≈ {{ centavos(custoEstimado) }}</strong></p>
      </div>

      <p v-if="erro" class="error-text">{{ erro }}</p>
    </section>

    <!-- ===== Passo 2 ===== -->
    <section v-else-if="passo === 2" class="step step-2">
      <div v-if="demoNoResultado" class="demo-banner">Dados de demonstração — uma ou mais fontes estão em modo demonstração.</div>

      <ul class="queries">
        <li v-for="c in consultas" :key="c.provedor" :class="c.status">
          <SourceBadge :provedor="c.provedor" com-nome />
          <span class="badge" :class="statusConsulta[c.status]?.classe ?? 'gray'">{{ statusConsulta[c.status]?.rotulo ?? c.status }}</span>
          <span v-if="c.cache" class="badge gray">cache</span>
          <span v-if="c.demo" class="badge amber">demo</span>
          <span v-if="c.custo_centavos" class="muted">{{ centavos(c.custo_centavos) }}</span>
          <span v-if="c.mensagem" class="query-msg" :class="{ 'error-text': c.status === 'erro' }">{{ c.mensagem }}</span>
        </li>
      </ul>

      <template v-if="diff.length">
        <div class="diff-tools">
          <span class="muted">{{ marcados.length }} de {{ diff.length }} campos marcados</span>
          <span class="spacer"></span>
          <button type="button" class="btn btn-outline btn-sm" @click="marcarSoVazios">Marcar só vazios</button>
          <button type="button" class="btn btn-outline btn-sm" @click="marcarTodos">Marcar todos</button>
        </div>
        <div class="diff-wrap">
          <table class="diff">
            <thead>
              <tr>
                <th></th>
                <th>Campo</th>
                <th>Atual</th>
                <th>Proposto</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in diff" :key="d.campo" :class="{ checked: estaMarcado(d.campo) }" @click="alternarCampo(d.campo)">
                <td class="chk">
                  <input type="checkbox" :checked="estaMarcado(d.campo)" :data-campo="d.campo" @click.stop @change="alternarCampo(d.campo)" />
                </td>
                <td class="label">{{ d.label }}</td>
                <td class="current" :class="{ empty: valorLegivel(d.campo, d.atual) === '' }">{{ valorLegivel(d.campo, d.atual) || 'vazio' }}</td>
                <td class="proposed">
                  <span class="value">{{ valorLegivel(d.campo, d.proposto) }}</span>
                  <SourceBadge :provedor="d.provedor" com-nome />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
      <div v-else class="empty-state no-diff">
        <strong>Nenhuma diferença nos campos da ficha</strong>
        <template v-if="consultasOk.length">A ficha já está em dia; ainda assim você pode guardar a inteligência consultada (sócios, indicadores, risco).</template>
        <template v-else>Nenhuma fonte devolveu dados para este CNPJ.</template>
      </div>

      <p v-if="erro" class="error-text">{{ erro }}</p>
    </section>

    <!-- ===== Passo 3 ===== -->
    <section v-else class="step step-3">
      <div class="done-icon" aria-hidden="true">✦</div>
      <h4 v-if="resumoPorFonte.length">
        <template v-for="(r, i) in resumoPorFonte" :key="r.provedor">
          <template v-if="i > 0"><br /></template>
          {{ r.n }} {{ r.n === 1 ? 'campo atualizado' : 'campos atualizados' }} pela {{ nomeProvedor(r.provedor) }}
        </template>
      </h4>
      <h4 v-else>Inteligência guardada, nenhum campo da ficha alterado</h4>
      <p class="muted">
        A aba <strong>Inteligência</strong> mostra o que foi encontrado; os campos aplicados ganham a marca ✦ com fonte e data.
        <template v-if="preview?.custo_centavos"> Custo desta consulta: {{ centavos(preview.custo_centavos) }}.</template>
      </p>
      <div v-if="demoNoResultado" class="demo-banner">Dados de demonstração — ative a fonte real em Configurações → Integrações.</div>
    </section>

    <template #footer>
      <template v-if="passo === 1">
        <button type="button" class="btn btn-outline" @click="fechar">Cancelar</button>
        <button type="button" class="btn btn-primary" :disabled="!algumSelecionado || consultando" @click="consultar">
          {{ consultando ? 'Consultando…' : 'Consultar' }}
        </button>
      </template>
      <template v-else-if="passo === 2">
        <button type="button" class="btn btn-outline" :disabled="aplicando" @click="passo = 1">Voltar</button>
        <button type="button" class="btn btn-primary" :disabled="aplicando || !consultasOk.length" @click="aplicar">
          <template v-if="aplicando">Aplicando…</template>
          <template v-else-if="marcados.length">Aplicar {{ marcados.length }} {{ marcados.length === 1 ? 'campo' : 'campos' }}</template>
          <template v-else>Guardar sem alterar campos</template>
        </button>
      </template>
      <template v-else>
        <button type="button" class="btn btn-primary" @click="fechar">Fechar</button>
      </template>
    </template>
  </ModalDialog>
</template>

<style scoped>
.steps {
  list-style: none;
  margin: 0 0 16px;
  padding: 0;
  display: flex;
  gap: 6px;
  counter-reset: passo;
}

.steps li {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--ci-text-3);
  padding-bottom: 8px;
  border-bottom: 2px solid var(--ci-border);
}

.steps li::before {
  counter-increment: passo;
  content: counter(passo);
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--ci-bg);
  font-weight: 600;
  font-size: 11px;
}

.steps li.on {
  color: var(--ci-purple);
  border-bottom-color: var(--ci-purple);
  font-weight: 600;
}

.steps li.on::before {
  background: var(--ci-purple);
  color: #fff;
}

.steps li.done {
  color: var(--ci-green);
  border-bottom-color: var(--ci-green);
}

.steps li.done::before {
  content: '✓';
  background: var(--ci-green-tint);
  color: var(--ci-green);
}

.intro {
  margin: 0 0 12px;
  font-size: 13px;
}

.demo-banner {
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
  border-left: 3px solid var(--ci-amber);
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  margin-bottom: 12px;
}

.sources {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}

.source {
  border: 2px solid var(--ci-border);
  border-radius: 10px;
  padding: 12px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 6px;
  position: relative;
}

.source input {
  position: absolute;
  top: 10px;
  right: 10px;
  margin: 0;
}

.source.selected {
  border-color: var(--ci-purple);
  background: var(--ci-purple-tint);
}

.source.disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.src-head {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-right: 22px;
}

.cost {
  font-size: 12px;
  font-weight: 600;
  color: var(--ci-text-2);
}

.cost.free {
  color: var(--ci-green);
}

.cost.demo {
  color: var(--ci-amber);
}

.cost.off {
  color: var(--ci-text-3);
  font-weight: 500;
}

.source small {
  color: var(--ci-text-3);
  font-size: 12px;
  line-height: 1.4;
}

.datasets {
  margin-top: 10px;
}

.link {
  border: none;
  background: none;
  color: var(--ci-purple);
  font-size: 13px;
  cursor: pointer;
  padding: 0;
  font-weight: 500;
}

.dataset-list {
  list-style: none;
  margin: 8px 0 0;
  padding: 10px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 14px;
  background: var(--ci-bg);
  border-radius: 8px;
}

.dataset-list label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.dataset-list .price {
  margin-left: auto;
  font-size: 12px;
}

.options {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.check {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.usage,
.estimate {
  margin: 0;
  font-size: 13px;
  color: var(--ci-text-2);
}

.queries {
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.queries li {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
  padding: 6px 10px;
  border-radius: 8px;
  background: var(--ci-bg);
}

.queries li.erro {
  background: var(--ci-red-tint);
}

.query-msg {
  flex-basis: 100%;
  font-size: 12px;
  color: var(--ci-text-2);
}

.diff-tools {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  font-size: 13px;
}

.spacer {
  flex: 1;
}

.diff-wrap {
  max-height: 46vh;
  overflow: auto;
  border: 1px solid var(--ci-border);
  border-radius: 10px;
}

.diff {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.diff th {
  text-align: left;
  padding: 8px 10px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
  border-bottom: 1px solid var(--ci-border);
  position: sticky;
  top: 0;
  background: var(--ci-surface);
}

.diff td {
  padding: 8px 10px;
  border-bottom: 1px solid var(--ci-bg);
  vertical-align: top;
}

.diff tbody tr {
  cursor: pointer;
}

.diff tbody tr:hover {
  background: var(--ci-bg);
}

.diff tbody tr.checked {
  background: var(--ci-purple-tint);
}

.chk {
  width: 32px;
}

.label {
  font-weight: 500;
  white-space: nowrap;
}

.current {
  color: var(--ci-text-2);
  word-break: break-word;
}

.current.empty {
  color: var(--ci-text-3);
  font-style: italic;
}

.proposed {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-start;
  word-break: break-word;
}

.proposed .value {
  font-weight: 500;
}

.no-diff {
  padding: 28px 16px;
}

.step-3 {
  text-align: center;
  padding: 12px 0 4px;
}

.done-icon {
  width: 52px;
  height: 52px;
  margin: 0 auto 12px;
  border-radius: 50%;
  background: var(--ci-purple-tint);
  color: var(--ci-purple);
  font-size: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.step-3 h4 {
  margin: 0 0 8px;
  font-size: 16px;
  line-height: 1.5;
}

.step-3 .muted {
  font-size: 13px;
}
</style>
