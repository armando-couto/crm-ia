<script setup lang="ts">
// Configurações → Integrações de dados: fontes de enriquecimento, regras e uso do mês.
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { somenteDigitos } from '../cnpj'
import { centavos, datasetsSugeridos, nomeProvedor, nomesDatasets } from '../components/intel/util'
import { useToastStore } from '../stores/toast'
import type {
  EnrichmentSettings,
  EnrichmentSettingsResposta,
  FonteCatalogo,
  Provedor,
  TesteFonte,
  UsoHistorico,
  UsoMensal,
  UsoMes
} from '../types-intel'

const toast = useToastStore()

function padrao(): EnrichmentSettings {
  return {
    auto_receita: true,
    sobrescrever: 'vazios',
    limite_mensal_centavos: 20000,
    prazo_cache_dias: 30,
    bigdatacorp: { habilitado: false, demo: false, token_id: '', access_token: '', has_secret: false, datasets_padrao: [...datasetsSugeridos], precos_centavos: {} },
    serasa: { habilitado: false, demo: false, ambiente: 'uat', client_id: '', client_secret: '', has_secret: false, report_name: 'RELATORIO_AVANCADO_PJ_PME', optional_features: '', preco_centavos: 1500 }
  }
}

const form = ref<EnrichmentSettings>(padrao())
const fontes = ref<FonteCatalogo[]>([])
const usoMes = ref<UsoMes | null>(null)
const meses = ref<UsoMensal[]>([])
const carregado = ref(false)
const saving = ref(false)

/** Copia as configurações da API para o formulário, sempre com os segredos em branco. */
function aplicar(s: EnrichmentSettings) {
  const base = padrao()
  form.value = {
    ...base,
    ...s,
    bigdatacorp: { ...base.bigdatacorp, ...(s.bigdatacorp ?? {}), access_token: '' },
    serasa: { ...base.serasa, ...(s.serasa ?? {}), client_secret: '' }
  }
  if (!form.value.bigdatacorp.datasets_padrao) form.value.bigdatacorp.datasets_padrao = [...datasetsSugeridos]
  if (!form.value.bigdatacorp.precos_centavos) form.value.bigdatacorp.precos_centavos = {}
}

async function load() {
  try {
    const r = await api.get<EnrichmentSettingsResposta>('/settings/enrichment')
    if (r.settings) aplicar(r.settings)
    fontes.value = r.fontes ?? []
    usoMes.value = r.uso_mes ?? null
    carregado.value = true
  } catch (e: any) {
    toast.error(e.message)
  }
  try {
    const u = await api.get<UsoHistorico>('/settings/enrichment/usage')
    meses.value = u.meses ?? []
  } catch {
    /* histórico opcional */
  }
}

async function save() {
  saving.value = true
  try {
    const payload: any = JSON.parse(JSON.stringify(form.value))
    // Segredos só vão quando digitados; vazio mantém o atual no servidor.
    if (!payload.bigdatacorp.access_token) delete payload.bigdatacorp.access_token
    if (!payload.serasa.client_secret) delete payload.serasa.client_secret
    delete payload.bigdatacorp.has_secret
    delete payload.serasa.has_secret
    const r = await api.put<EnrichmentSettingsResposta | EnrichmentSettings>('/settings/enrichment', payload)
    const s = (r as EnrichmentSettingsResposta).settings ?? (r as EnrichmentSettings)
    if (s && 'bigdatacorp' in s) aplicar(s)
    if ((r as EnrichmentSettingsResposta).uso_mes) usoMes.value = (r as EnrichmentSettingsResposta).uso_mes
    toast.push('Integrações de dados salvas')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

// ===== Testes por fonte =====
const testCnpj = ref<Record<string, string>>({ bigdatacorp: '', serasa: '' })
const testando = ref<Record<string, boolean>>({})
const testes = ref<Record<string, TesteFonte | null>>({})

async function testar(provedor: Provedor) {
  testando.value = { ...testando.value, [provedor]: true }
  testes.value = { ...testes.value, [provedor]: null }
  try {
    const body: Record<string, unknown> = { provedor }
    const cnpj = somenteDigitos(testCnpj.value[provedor])
    if (cnpj) body.cnpj = cnpj
    const r = await api.post<TesteFonte>('/settings/enrichment/test', body)
    testes.value = { ...testes.value, [provedor]: r }
    if (r.ok) toast.push(`${nomeProvedor(provedor)}: ${r.mensagem || 'conexão ok'}`)
    else toast.error(`${nomeProvedor(provedor)}: ${r.mensagem || 'falhou'}`)
  } catch (e: any) {
    testes.value = { ...testes.value, [provedor]: { ok: false, mensagem: e.message } }
    toast.error(e.message)
  } finally {
    testando.value = { ...testando.value, [provedor]: false }
  }
}

// ===== Datasets BigDataCorp =====
const catalogoBdc = computed(() => fontes.value.find((f) => f.codigo === 'bigdatacorp'))

const datasets = computed(() => {
  const doCatalogo = catalogoBdc.value?.datasets
  const lista = doCatalogo?.length
    ? doCatalogo.map((d) => ({ codigo: d.codigo, nome: d.nome || nomesDatasets[d.codigo] || d.codigo, descricao: d.descricao ?? '', precoCatalogo: d.preco_centavos }))
    : Object.keys(nomesDatasets).map((codigo) => ({ codigo, nome: nomesDatasets[codigo], descricao: '', precoCatalogo: undefined as number | undefined }))
  return lista
})

function preco(codigo: string, precoCatalogo?: number): number {
  const p = form.value.bigdatacorp.precos_centavos?.[codigo]
  return typeof p === 'number' ? p : precoCatalogo ?? 0
}

const totalPadrao = computed(() =>
  form.value.bigdatacorp.datasets_padrao.reduce((s, codigo) => {
    const d = datasets.value.find((x) => x.codigo === codigo)
    return s + preco(codigo, d?.precoCatalogo)
  }, 0)
)

function alternarDataset(codigo: string) {
  const atual = form.value.bigdatacorp.datasets_padrao
  form.value.bigdatacorp.datasets_padrao = atual.includes(codigo) ? atual.filter((d) => d !== codigo) : [...atual, codigo]
}

const editarPrecos = ref(false)

function setPreco(codigo: string, valor: string) {
  const reais = Number(String(valor).replace(',', '.'))
  if (!form.value.bigdatacorp.precos_centavos) form.value.bigdatacorp.precos_centavos = {}
  form.value.bigdatacorp.precos_centavos[codigo] = isNaN(reais) ? 0 : Math.round(reais * 100)
}

// ===== Regras =====
const limiteReais = computed({
  get: () => (form.value.limite_mensal_centavos / 100).toFixed(2).replace('.', ','),
  set: (v: string) => {
    const n = Number(String(v).replace(/\./g, '').replace(',', '.'))
    form.value.limite_mensal_centavos = isNaN(n) ? 0 : Math.round(n * 100)
  }
})

const precoSerasaReais = computed({
  get: () => (form.value.serasa.preco_centavos / 100).toFixed(2).replace('.', ','),
  set: (v: string) => {
    const n = Number(String(v).replace(/\./g, '').replace(',', '.'))
    form.value.serasa.preco_centavos = isNaN(n) ? 0 : Math.round(n * 100)
  }
})

// ===== Uso =====
const mesAtual = new Date().toISOString().slice(0, 7)
const usoDoMes = computed(() => meses.value.find((m) => m.mes === mesAtual) ?? null)

const limite = computed(() => usoMes.value?.limite_centavos || form.value.limite_mensal_centavos || 0)

function pct(custo: number): number {
  if (!limite.value) return 0
  return Math.min(100, Math.round((custo / limite.value) * 100))
}

const porProvedor = computed(() => {
  const pp = usoDoMes.value?.por_provedor ?? {}
  return (['receita', 'bigdatacorp', 'serasa'] as Provedor[]).map((p) => {
    const v = pp[p]
    const consultas = typeof v === 'object' && v ? v.consultas ?? 0 : 0
    const custo = typeof v === 'number' ? v : typeof v === 'object' && v ? v.custo_centavos ?? 0 : 0
    return { provedor: p, consultas, custo, presente: v !== undefined }
  })
})

const temPorProvedor = computed(() => porProvedor.value.some((p) => p.presente))

function nomeMes(mes: string): string {
  const [ano, m] = mes.split('-')
  const nomes = ['jan', 'fev', 'mar', 'abr', 'mai', 'jun', 'jul', 'ago', 'set', 'out', 'nov', 'dez']
  return `${nomes[Number(m) - 1] ?? m}/${ano}`
}

const fontesAtivas = computed(() => 1 + (form.value.bigdatacorp.habilitado ? 1 : 0) + (form.value.serasa.habilitado ? 1 : 0))

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Integrações de dados</h1>
        <p class="muted" style="margin: 4px 0 0">
          Fontes usadas para enriquecer empresas a partir do CNPJ. A Receita Federal é grátis; as fontes pagas mostram o custo estimado antes de cada consulta.
        </p>
      </div>
      <span class="badge ok">{{ fontesAtivas }} {{ fontesAtivas === 1 ? 'fonte ativa' : 'fontes ativas' }}</span>
    </div>

    <div class="layout">
      <form class="forms" @submit.prevent="save">
        <!-- ===== Receita Federal ===== -->
        <section class="card source" data-source="receita">
          <header>
            <div>
              <h2>Receita Federal</h2>
              <p class="muted">BrasilAPI, OpenCNPJ e CNPJ.ws — razão social, situação, CNAE, endereço, sócios. Cache de {{ form.prazo_cache_dias }} dias.</p>
            </div>
            <span class="badge ok">sempre ativa · grátis</span>
          </header>
          <label class="switch">
            <input v-model="form.auto_receita" type="checkbox" />
            <span>Preencher automaticamente ao criar uma empresa com CNPJ</span>
          </label>
          <p class="hint">Ao cadastrar uma empresa com CNPJ válido, o CRM consulta a Receita e preenche só os campos vazios, registrando a fonte de cada um.</p>
        </section>

        <!-- ===== BigDataCorp ===== -->
        <section class="card source" :class="{ off: !form.bigdatacorp.habilitado }" data-source="bigdatacorp">
          <header>
            <div>
              <h2>BigDataCorp</h2>
              <p class="muted">Indicadores de atividade, contatos validados, sócios, presença digital, processos, KYC e mídia — cobrado por conjunto.</p>
            </div>
            <span class="badge" :class="form.bigdatacorp.habilitado ? (form.bigdatacorp.demo ? 'demo' : 'ok') : 'off'">
              {{ form.bigdatacorp.habilitado ? (form.bigdatacorp.demo ? 'demonstração' : 'ativa') : 'desativada' }}
            </span>
          </header>

          <label class="switch">
            <input v-model="form.bigdatacorp.habilitado" type="checkbox" />
            <span>Habilitar BigDataCorp</span>
          </label>

          <template v-if="form.bigdatacorp.habilitado">
            <label class="switch demo-switch">
              <input v-model="form.bigdatacorp.demo" type="checkbox" />
              <span>Modo demonstração</span>
            </label>
            <p v-if="form.bigdatacorp.demo" class="demo-banner">
              Em demonstração a fonte devolve um perfil fictício (determinístico pelo CNPJ) sempre rotulado "dados de demonstração". Nada é cobrado nem consultado de verdade.
            </p>

            <div class="form-row">
              <div class="field">
                <label>Token ID</label>
                <input v-model="form.bigdatacorp.token_id" autocomplete="off" />
              </div>
              <div class="field">
                <label>Access Token <span v-if="form.bigdatacorp.has_secret" class="badge ok mini">guardado</span></label>
                <input v-model="form.bigdatacorp.access_token" type="password" autocomplete="off" :placeholder="form.bigdatacorp.has_secret ? '•••••• (mantida se vazio)' : ''" />
              </div>
            </div>

            <div class="datasets">
              <div class="datasets-head">
                <strong>Conjuntos padrão</strong>
                <span class="total">≈ {{ centavos(totalPadrao) }} por consulta · {{ form.bigdatacorp.datasets_padrao.length }} {{ form.bigdatacorp.datasets_padrao.length === 1 ? 'conjunto' : 'conjuntos' }}</span>
              </div>
              <ul>
                <li v-for="d in datasets" :key="d.codigo">
                  <label>
                    <input type="checkbox" :checked="form.bigdatacorp.datasets_padrao.includes(d.codigo)" :data-dataset="d.codigo" @change="alternarDataset(d.codigo)" />
                    <span class="ds-name">{{ d.nome }}<small v-if="d.descricao" class="muted"> — {{ d.descricao }}</small></span>
                    <span class="ds-price">{{ centavos(preco(d.codigo, d.precoCatalogo)) }}</span>
                  </label>
                  <input
                    v-if="editarPrecos"
                    class="price-input"
                    type="text"
                    inputmode="decimal"
                    :value="(preco(d.codigo, d.precoCatalogo) / 100).toFixed(2).replace('.', ',')"
                    @change="setPreco(d.codigo, ($event.target as HTMLInputElement).value)"
                  />
                </li>
              </ul>
              <button type="button" class="link" @click="editarPrecos = !editarPrecos">
                {{ editarPrecos ? 'Concluir edição de preços' : 'Editar preços de referência (só para estimativa)' }}
              </button>
            </div>

            <div class="test">
              <div class="field" style="margin: 0; flex: 1">
                <label>CNPJ para teste (opcional)</label>
                <input v-model="testCnpj.bigdatacorp" placeholder="00.000.000/0000-00" />
              </div>
              <button class="btn btn-outline" type="button" :disabled="testando.bigdatacorp" @click="testar('bigdatacorp')">
                {{ testando.bigdatacorp ? 'Testando…' : 'Testar' }}
              </button>
            </div>
            <p v-if="testes.bigdatacorp" class="test-result" :class="testes.bigdatacorp.ok ? 'success-text' : 'error-text'">
              {{ testes.bigdatacorp.ok ? '✓' : '✕' }} {{ testes.bigdatacorp.mensagem }}
              <span v-if="testes.bigdatacorp.custo_centavos" class="muted"> · {{ centavos(testes.bigdatacorp.custo_centavos) }}</span>
              <span v-if="testes.bigdatacorp.demo" class="badge demo mini">demo</span>
            </p>
          </template>
        </section>

        <!-- ===== Serasa ===== -->
        <section class="card source" :class="{ off: !form.serasa.habilitado }" data-source="serasa">
          <header>
            <div>
              <h2>Serasa Experian</h2>
              <p class="muted">Relatório PJ: score positivo, pendências, protestos, cheques, consultas recentes e limite sugerido — por contrato.</p>
            </div>
            <span class="badge" :class="form.serasa.habilitado ? (form.serasa.demo ? 'demo' : 'ok') : 'off'">
              {{ form.serasa.habilitado ? (form.serasa.demo ? 'demonstração' : 'ativa') : 'desativada' }}
            </span>
          </header>

          <label class="switch">
            <input v-model="form.serasa.habilitado" type="checkbox" />
            <span>Habilitar Serasa Experian</span>
          </label>

          <template v-if="form.serasa.habilitado">
            <label class="switch demo-switch">
              <input v-model="form.serasa.demo" type="checkbox" />
              <span>Modo demonstração</span>
            </label>
            <p v-if="form.serasa.demo" class="demo-banner">
              Em demonstração o relatório é fictício e rotulado "dados de demonstração". Use para apresentar o recurso antes do contrato.
            </p>

            <div class="field">
              <label>Ambiente</label>
              <div class="radios">
                <label class="radio" :class="{ selected: form.serasa.ambiente === 'uat' }"><input v-model="form.serasa.ambiente" type="radio" value="uat" /> UAT (homologação)</label>
                <label class="radio" :class="{ selected: form.serasa.ambiente === 'prod' }"><input v-model="form.serasa.ambiente" type="radio" value="prod" /> Produção</label>
              </div>
            </div>
            <div class="form-row">
              <div class="field">
                <label>Client ID</label>
                <input v-model="form.serasa.client_id" autocomplete="off" />
              </div>
              <div class="field">
                <label>Client Secret <span v-if="form.serasa.has_secret" class="badge ok mini">guardado</span></label>
                <input v-model="form.serasa.client_secret" type="password" autocomplete="off" :placeholder="form.serasa.has_secret ? '•••••• (mantida se vazio)' : ''" />
              </div>
            </div>
            <div class="form-row">
              <div class="field">
                <label>reportName</label>
                <input v-model="form.serasa.report_name" />
              </div>
              <div class="field">
                <label>optionalFeatures</label>
                <input v-model="form.serasa.optional_features" placeholder="separados por vírgula" />
              </div>
            </div>
            <div class="field" style="max-width: 220px">
              <label>Preço de referência por consulta (R$)</label>
              <input v-model="precoSerasaReais" inputmode="decimal" />
            </div>

            <div class="test">
              <div class="field" style="margin: 0; flex: 1">
                <label>CNPJ para teste (opcional)</label>
                <input v-model="testCnpj.serasa" placeholder="00.000.000/0000-00" />
              </div>
              <button class="btn btn-outline" type="button" :disabled="testando.serasa" @click="testar('serasa')">
                {{ testando.serasa ? 'Testando…' : 'Testar' }}
              </button>
            </div>
            <p v-if="testes.serasa" class="test-result" :class="testes.serasa.ok ? 'success-text' : 'error-text'">
              {{ testes.serasa.ok ? '✓' : '✕' }} {{ testes.serasa.mensagem }}
              <span v-if="testes.serasa.custo_centavos" class="muted"> · {{ centavos(testes.serasa.custo_centavos) }}</span>
              <span v-if="testes.serasa.demo" class="badge demo mini">demo</span>
            </p>
          </template>
        </section>

        <!-- ===== Regras ===== -->
        <section class="card source" data-source="regras">
          <header>
            <div>
              <h2>Regras</h2>
              <p class="muted">Como os dados encontrados entram na ficha e quanto a equipe pode gastar por mês.</p>
            </div>
          </header>
          <div class="field">
            <label>Ao aplicar o enriquecimento</label>
            <div class="radios">
              <label class="radio" :class="{ selected: form.sobrescrever === 'vazios' }">
                <input v-model="form.sobrescrever" type="radio" value="vazios" /> Preencher só os campos vazios
              </label>
              <label class="radio" :class="{ selected: form.sobrescrever === 'sempre' }">
                <input v-model="form.sobrescrever" type="radio" value="sempre" /> Sugerir sobrescrever sempre
              </label>
            </div>
            <small class="muted">No modal de enriquecimento o vendedor ainda revisa campo a campo; esta regra define o que vem marcado por padrão e o que a ação em massa aplica.</small>
          </div>
          <div class="form-row">
            <div class="field">
              <label>Limite mensal (R$)</label>
              <input v-model="limiteReais" inputmode="decimal" />
              <small class="muted">Consultas pagas param ao atingir o limite ({{ centavos(form.limite_mensal_centavos) }}).</small>
            </div>
            <div class="field">
              <label>Prazo do cache (dias)</label>
              <input v-model.number="form.prazo_cache_dias" type="number" min="1" max="365" />
              <small class="muted">Consulta pública com menos dias que isso é reaproveitada sem bater na API.</small>
            </div>
          </div>
        </section>

        <div class="actions">
          <button class="btn btn-primary" type="submit" :disabled="saving || !carregado">{{ saving ? 'Salvando…' : 'Salvar integrações' }}</button>
        </div>
      </form>

      <!-- ===== Uso ===== -->
      <aside class="usage card">
        <h2>Uso neste mês</h2>
        <template v-if="usoMes">
          <div class="usage-total">
            <strong>{{ centavos(usoMes.custo_centavos) }}</strong>
            <span class="muted">de {{ centavos(limite) }} · {{ usoMes.consultas }} {{ usoMes.consultas === 1 ? 'consulta' : 'consultas' }}</span>
          </div>
          <div class="bar total" :class="{ warn: pct(usoMes.custo_centavos) >= 80, full: pct(usoMes.custo_centavos) >= 100 }">
            <span :style="{ width: pct(usoMes.custo_centavos) + '%' }"></span>
          </div>
        </template>
        <p v-else class="muted">Sem consultas registradas ainda.</p>

        <ul v-if="temPorProvedor" class="by-source">
          <li v-for="p in porProvedor" :key="p.provedor">
            <div class="row">
              <span>{{ nomeProvedor(p.provedor) }}</span>
              <span class="muted">{{ p.consultas }} {{ p.consultas === 1 ? 'consulta' : 'consultas' }} · {{ p.custo ? centavos(p.custo) : 'grátis' }}</span>
            </div>
            <div class="bar" :class="p.provedor"><span :style="{ width: pct(p.custo) + '%' }"></span></div>
          </li>
        </ul>

        <template v-if="meses.length">
          <h3>Últimos meses</h3>
          <table class="months">
            <tbody>
              <tr v-for="m in meses.slice(0, 6)" :key="m.mes">
                <td>{{ nomeMes(m.mes) }}</td>
                <td class="muted">{{ m.consultas }} {{ m.consultas === 1 ? 'consulta' : 'consultas' }}</td>
                <td class="right">{{ centavos(m.custo_centavos) }}</td>
              </tr>
            </tbody>
          </table>
        </template>

        <p class="hint small">Os preços são referência da 1ª faixa de cada provedor e servem só para estimativa; a fatura real vem do contrato.</p>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 16px;
  align-items: start;
}

@media (max-width: 1000px) {
  .layout {
    grid-template-columns: 1fr;
  }
}

.forms {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.source header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}

.source h2 {
  font-size: 16px;
}

.source header p {
  margin: 4px 0 0;
  font-size: 13px;
}

.source.off {
  background: color-mix(in srgb, var(--ci-surface) 85%, var(--ci-bg));
}

.badge {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 999px;
  font-weight: 600;
  white-space: nowrap;
}

.badge.ok {
  background: var(--ci-green-tint);
  color: var(--ci-green);
}

.badge.off {
  background: var(--ci-red-tint);
  color: var(--ci-red);
}

.badge.demo {
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
}

.badge.mini {
  font-size: 10px;
  padding: 1px 7px;
  margin-left: 6px;
  vertical-align: middle;
}

.switch {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  cursor: pointer;
  margin-bottom: 12px;
}

.switch input {
  width: 18px;
  height: 18px;
  accent-color: var(--ci-purple);
}

.demo-switch {
  color: var(--ci-amber);
  font-weight: 500;
}

.demo-banner {
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
  border-left: 3px solid var(--ci-amber);
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  margin: 0 0 12px;
}

.hint {
  font-size: 13px;
  color: var(--ci-text-2);
  background: var(--ci-blue-tint);
  padding: 10px 12px;
  border-radius: 8px;
  margin: 0;
}

.hint.small {
  font-size: 12px;
  margin-top: 14px;
}

.datasets {
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 14px;
}

.datasets-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  font-size: 13px;
}

.total {
  color: var(--ci-purple-dark);
  font-weight: 600;
}

.datasets ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 16px;
}

@media (max-width: 700px) {
  .datasets ul {
    grid-template-columns: 1fr;
  }
}

.datasets li {
  display: flex;
  align-items: center;
  gap: 8px;
}

.datasets li label {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.ds-name {
  flex: 1;
}

.ds-price {
  color: var(--ci-text-3);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.price-input {
  width: 64px;
  padding: 3px 6px;
  border: 1px solid var(--ci-border);
  border-radius: 6px;
  font-size: 12px;
  text-align: right;
}

.link {
  border: none;
  background: none;
  color: var(--ci-purple);
  font-size: 12px;
  cursor: pointer;
  padding: 0;
  margin-top: 10px;
}

.test {
  display: flex;
  gap: 10px;
  align-items: flex-end;
  flex-wrap: wrap;
}

.test-result {
  margin: 8px 0 0;
}

.radios {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.radio {
  border: 2px solid var(--ci-border);
  border-radius: 10px;
  padding: 8px 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500 !important;
}

.radio.selected {
  border-color: var(--ci-purple);
  background: var(--ci-purple-tint);
}

.radio input {
  display: none;
}

.field small {
  font-size: 12px;
  margin-top: 2px;
}

.actions {
  display: flex;
  justify-content: flex-end;
}

.usage h2 {
  font-size: 15px;
  margin-bottom: 12px;
}

.usage h3 {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
  margin: 16px 0 8px;
}

.usage-total {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 8px;
}

.usage-total strong {
  font-size: 24px;
  color: var(--ci-purple-dark);
}

.usage-total span {
  font-size: 12px;
}

.bar {
  height: 8px;
  border-radius: 999px;
  background: var(--ci-bg);
  overflow: hidden;
}

.bar span {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--ci-purple);
  transition: width 0.4s;
}

.bar.warn span {
  background: var(--ci-amber);
}

.bar.full span {
  background: var(--ci-red);
}

.bar.receita span {
  background: var(--ci-green);
}

.bar.bigdatacorp span {
  background: var(--ci-blue);
}

.bar.serasa span {
  background: var(--ci-amber);
}

.by-source {
  list-style: none;
  margin: 14px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.by-source .row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
  margin-bottom: 4px;
}

.by-source .row .muted {
  font-size: 12px;
}

.months {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.months td {
  padding: 5px 0;
  border-bottom: 1px solid var(--ci-bg);
}

.months .right {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
