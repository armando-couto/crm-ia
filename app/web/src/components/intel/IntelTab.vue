<script setup lang="ts">
// Aba "Inteligência" da ficha da empresa: sinais no topo, cartões por tema,
// histórico de consultas e limpeza (LGPD).
import { computed, ref } from 'vue'
import { api } from '../../api'
import { formatDateTime, formatMoney } from '../../format'
import { useToastStore } from '../../stores/toast'
import type { Company } from '../../types'
import type { IntelEmpresa, Perfil, Provedor } from '../../types-intel'
import ScoreArc from './ScoreArc.vue'
import SignalChips from './SignalChips.vue'
import { centavos, dataCurta, enderecoCompleto, formatarCEP, nomeProvedor } from './util'

const props = defineProps<{
  company: Company
  data: IntelEmpresa | null
  loading: boolean
  canEnrich: boolean
}>()

const emit = defineEmits<{ enrich: []; refresh: []; updated: [company: Company] }>()

const toast = useToastStore()

const perfil = computed<Perfil | null>(() => props.data?.intel ?? null)
const campos = computed(() => perfil.value?.campos ?? {})
const historico = computed(() => props.data?.historico ?? [])

const temIntel = computed(() => {
  const p = perfil.value
  if (!p) return false
  return (
    Object.keys(campos.value).length > 0 ||
    (p.sinais?.length ?? 0) > 0 ||
    (p.socios?.length ?? 0) > 0 ||
    !!p.risco ||
    !!p.indicadores ||
    !!p.processos ||
    !!p.conformidade ||
    !!p.midia
  )
})

const demo = computed(() => !!perfil.value?.demo || historico.value.some((h) => h.status === 'ok' && h.demo))

// ===== Fonte · data por cartão =====
function ultimaOk(provedor: Provedor): { provedor: string; data: string } | null {
  const h = historico.value.find((x) => x.provedor === provedor && x.status === 'ok')
  return h ? { provedor, data: h.created_at } : null
}

const fontePadrao = computed(() =>
  perfil.value?.provedor || perfil.value?.consultado_em ? { provedor: perfil.value?.provedor ?? 'receita', data: perfil.value?.consultado_em ?? '' } : null
)

type Secao = 'cadastro' | 'endereco' | 'socios' | 'contatos' | 'atividade' | 'presenca' | 'processos' | 'conformidade' | 'midia' | 'risco'

function fonte(secao: Secao): string {
  let f: { provedor: string; data: string } | null = null
  if (secao === 'cadastro' || secao === 'endereco' || secao === 'socios') f = ultimaOk('receita')
  else if (secao === 'risco') f = ultimaOk('serasa') ?? ultimaOk('bigdatacorp')
  else f = ultimaOk('bigdatacorp') ?? ultimaOk('receita')
  f = f ?? fontePadrao.value
  if (!f) return ''
  return f.data ? `${nomeProvedor(f.provedor)} · ${dataCurta(f.data)}` : nomeProvedor(f.provedor)
}

// ===== Visibilidade dos cartões =====
function tem(...valores: unknown[]): boolean {
  return valores.some((v) => v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0))
}

const mostraCadastro = computed(() => {
  const c = campos.value
  return tem(c.legal_name, c.trade_name, c.registration_status, c.founded_at, c.legal_nature, c.company_size, c.capital, c.cnae_code, c.cnae_description, c.simples, c.mei)
})
const endereco = computed(() => enderecoCompleto(campos.value as any))
const mostraEndereco = computed(() => !!endereco.value)
const mapaUrl = computed(() => `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(endereco.value)}`)

const telefones = computed(() => perfil.value?.contatos?.telefones ?? [])
const emails = computed(() => perfil.value?.contatos?.emails ?? [])
const mostraContatos = computed(() => telefones.value.length > 0 || emails.value.length > 0)

const socios = computed(() => perfil.value?.socios ?? [])

const indicadores = computed(() => perfil.value?.indicadores ?? null)
const mostraAtividade = computed(() => {
  const i = indicadores.value
  return (
    (!!i && tem(i.nivel_atividade, i.tem_dominio_ativo, i.tem_ssl, i.tem_email_corporativo, i.funcionarios_faixa, i.faturamento_faixa)) ||
    tem(campos.value.employees_range, campos.value.revenue_range)
  )
})
const nivelAtividade = computed(() => {
  const n = indicadores.value?.nivel_atividade
  if (n === null || n === undefined) return null
  return Math.round(Math.max(0, Math.min(1, Number(n))) * 100)
})
const flags = computed(() => {
  const i = indicadores.value
  if (!i) return []
  const lista: { rotulo: string; valor: boolean | null | undefined }[] = [
    { rotulo: 'Domínio ativo', valor: i.tem_dominio_ativo },
    { rotulo: 'Certificado SSL', valor: i.tem_ssl },
    { rotulo: 'E-mail corporativo', valor: i.tem_email_corporativo }
  ]
  return lista.filter((f) => f.valor !== null && f.valor !== undefined)
})

const dominios = computed(() => perfil.value?.presenca_digital?.dominios ?? [])
const redes = computed(() => Object.entries(perfil.value?.presenca_digital?.redes ?? {}).filter(([, v]) => !!v))
const mostraPresenca = computed(() => dominios.value.length > 0 || redes.value.length > 0)

function urlRede(rede: string, valor: string): string {
  if (/^https?:\/\//i.test(valor)) return valor
  const handle = valor.replace(/^@/, '')
  const bases: Record<string, string> = {
    instagram: 'https://instagram.com/',
    linkedin: 'https://www.linkedin.com/company/',
    facebook: 'https://facebook.com/',
    twitter: 'https://x.com/',
    x: 'https://x.com/',
    youtube: 'https://youtube.com/',
    tiktok: 'https://tiktok.com/@'
  }
  return (bases[rede.toLowerCase()] ?? 'https://') + handle
}

const risco = computed(() => perfil.value?.risco ?? null)
const mostraRisco = computed(() => {
  const r = risco.value
  return !!r && tem(r.score, r.faixa, r.pendencias_qtd, r.protestos_qtd, r.acoes_qtd, r.consultas_90d, r.limite_sugerido)
})

const processos = computed(() => perfil.value?.processos ?? null)
const mostraProcessos = computed(() => {
  const p = processos.value
  return !!p && tem(p.total, p.ativos, p.como_reu, p.ultimos)
})

const conformidade = computed(() => perfil.value?.conformidade ?? null)
const mostraConformidade = computed(() => {
  const c = conformidade.value
  return !!c && tem(c.pep, c.sancionado, c.listas)
})

const midia = computed(() => perfil.value?.midia ?? null)
const mostraMidia = computed(() => {
  const m = midia.value
  return !!m && tem(m.exposicao, m.noticias)
})

const sentimentoClasse: Record<string, string> = { positivo: 'green', neutro: 'gray', negativo: 'red' }
const statusClasse: Record<string, string> = { ok: 'green', sem_resultado: 'amber', erro: 'red' }
const statusRotulo: Record<string, string> = { ok: 'ok', sem_resultado: 'sem resultado', erro: 'erro' }

// ===== Ações =====
const salvandoCampo = ref('')

async function usar(campo: 'phone' | 'email', valor: string) {
  salvandoCampo.value = `${campo}:${valor}`
  try {
    const atualizado = await api.put<Company>(`/companies/${props.company.id}`, { ...props.company, [campo]: valor })
    emit('updated', atualizado)
    toast.push(campo === 'phone' ? 'Telefone da empresa atualizado' : 'E-mail da empresa atualizado')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvandoCampo.value = ''
  }
}

const removendo = ref(false)

async function remover() {
  if (!confirm('Remover os dados de inteligência desta empresa? O histórico de consultas e as fontes são apagados; os campos já aplicados na ficha ficam.')) return
  removendo.value = true
  try {
    await api.delete(`/companies/${props.company.id}/intel`)
    toast.push('Dados de inteligência removidos')
    emit('refresh')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    removendo.value = false
  }
}
</script>

<template>
  <div class="intel">
    <p v-if="loading && !data" class="muted">Carregando inteligência…</p>

    <!-- ===== Estado vazio ===== -->
    <div v-else-if="!temIntel" class="empty card">
      <div class="empty-icon" aria-hidden="true">✦</div>
      <h3>Ainda não há inteligência sobre esta empresa</h3>
      <p class="muted">
        A partir do CNPJ, o CRM IA busca o que é público na Receita Federal (grátis) e, sob demanda, dados de provedores pagos:
        sócios, indicadores de atividade, contatos validados, risco de crédito e processos. Nada é gravado sem a sua revisão e cada
        dado mostra a fonte e a data.
      </p>
      <button v-if="canEnrich" type="button" class="btn btn-primary" :disabled="!company.cnpj" @click="emit('enrich')">✦ Enriquecer</button>
      <p v-else class="muted small">Peça a alguém com a permissão "Enriquecer empresas" para consultar as fontes.</p>
      <p v-if="canEnrich && !company.cnpj" class="muted small">Informe o CNPJ da empresa para enriquecer.</p>
    </div>

    <template v-else>
      <div v-if="demo" class="demo-banner">Dados de demonstração — uma das fontes está em modo demonstração; os valores são fictícios e rotulados.</div>

      <div class="top">
        <SignalChips :sinais="perfil?.sinais ?? []" />
        <button v-if="canEnrich" type="button" class="btn btn-outline btn-sm" @click="emit('enrich')">✦ Enriquecer novamente</button>
      </div>

      <div class="cards">
        <!-- Cadastro -->
        <article v-if="mostraCadastro" class="intel-card card" data-card="cadastro">
          <h3>Cadastro</h3>
          <dl>
            <template v-if="campos.legal_name"><dt>Razão social</dt><dd>{{ campos.legal_name }}</dd></template>
            <template v-if="campos.trade_name"><dt>Nome fantasia</dt><dd>{{ campos.trade_name }}</dd></template>
            <template v-if="campos.registration_status">
              <dt>Situação cadastral</dt>
              <dd>
                <span class="badge" :class="String(campos.registration_status).toUpperCase() === 'ATIVA' ? 'green' : 'red'">{{ campos.registration_status }}</span>
                <span v-if="campos.registration_status_at" class="muted"> desde {{ dataCurta(campos.registration_status_at) }}</span>
              </dd>
            </template>
            <template v-if="campos.founded_at"><dt>Abertura</dt><dd>{{ dataCurta(campos.founded_at) }}</dd></template>
            <template v-if="campos.legal_nature"><dt>Natureza jurídica</dt><dd>{{ campos.legal_nature }}</dd></template>
            <template v-if="campos.company_size"><dt>Porte</dt><dd>{{ campos.company_size }}</dd></template>
            <template v-if="campos.capital !== undefined && campos.capital !== null"><dt>Capital social</dt><dd>{{ formatMoney(Number(campos.capital)) }}</dd></template>
            <template v-if="campos.simples !== undefined || campos.mei !== undefined">
              <dt>Regime</dt>
              <dd class="tags">
                <span class="badge" :class="campos.simples ? 'green' : 'gray'">Simples {{ campos.simples ? 'sim' : 'não' }}</span>
                <span class="badge" :class="campos.mei ? 'green' : 'gray'">MEI {{ campos.mei ? 'sim' : 'não' }}</span>
              </dd>
            </template>
            <template v-if="campos.cnae_code || campos.cnae_description">
              <dt>CNAE principal</dt>
              <dd>
                <span v-if="campos.cnae_code" class="mono">{{ campos.cnae_code }}</span>
                {{ campos.cnae_description }}
                <details v-if="perfil?.cnaes_secundarios?.length" class="secondary">
                  <summary>{{ perfil!.cnaes_secundarios!.length }} CNAEs secundários</summary>
                  <ul>
                    <li v-for="c in perfil!.cnaes_secundarios" :key="c.codigo"><span class="mono">{{ c.codigo }}</span> {{ c.descricao }}</li>
                  </ul>
                </details>
              </dd>
            </template>
          </dl>
          <footer>Fonte · {{ fonte('cadastro') }}</footer>
        </article>

        <!-- Endereço -->
        <article v-if="mostraEndereco" class="intel-card card" data-card="endereco">
          <h3>Endereço</h3>
          <p class="address">{{ endereco }}</p>
          <p v-if="campos.zip" class="muted small">CEP {{ formatarCEP(String(campos.zip)) }}</p>
          <a class="btn btn-outline btn-sm" :href="mapaUrl" target="_blank" rel="noopener">Ver no mapa ↗</a>
          <footer>Fonte · {{ fonte('endereco') }}</footer>
        </article>

        <!-- Contatos encontrados -->
        <article v-if="mostraContatos" class="intel-card card" data-card="contatos">
          <h3>Contatos encontrados</h3>
          <ul class="found">
            <li v-for="t in telefones" :key="t.numero">
              <span class="ico">☎</span>
              <span class="value">{{ t.numero }}</span>
              <span v-if="t.tipo" class="muted small">{{ t.tipo }}</span>
              <span v-if="t.principal" class="badge">principal</span>
              <span v-if="t.validado" class="valid" title="Validado pela fonte">✓</span>
              <span class="spacer"></span>
              <button
                v-if="canEnrich && company.phone !== t.numero"
                type="button"
                class="use"
                :disabled="salvandoCampo === `phone:${t.numero}`"
                @click="usar('phone', t.numero)"
              >
                usar
              </button>
              <span v-else-if="company.phone === t.numero" class="muted small">em uso</span>
            </li>
            <li v-for="e in emails" :key="e.email">
              <span class="ico">✉</span>
              <span class="value">{{ e.email }}</span>
              <span v-if="e.tipo" class="muted small">{{ e.tipo }}</span>
              <span v-if="e.validado" class="valid" title="Validado pela fonte">✓</span>
              <span class="spacer"></span>
              <button
                v-if="canEnrich && company.email !== e.email"
                type="button"
                class="use"
                :disabled="salvandoCampo === `email:${e.email}`"
                @click="usar('email', e.email)"
              >
                usar
              </button>
              <span v-else-if="company.email === e.email" class="muted small">em uso</span>
            </li>
          </ul>
          <footer>Fonte · {{ fonte('contatos') }}</footer>
        </article>

        <!-- Sócios -->
        <article v-if="socios.length" class="intel-card card" data-card="socios">
          <h3>Sócios e administradores</h3>
          <table class="mini">
            <thead>
              <tr><th>Nome</th><th>Qualificação</th><th>Desde</th><th>Documento</th></tr>
            </thead>
            <tbody>
              <tr v-for="(s, i) in socios" :key="`${s.nome}-${i}`">
                <td>{{ s.nome }}<span v-if="s.tipo" class="muted small"> · {{ s.tipo }}</span></td>
                <td>{{ s.qualificacao || '—' }}</td>
                <td>{{ s.desde ? dataCurta(s.desde) : '—' }}</td>
                <td class="mono">{{ s.documento || '—' }}</td>
              </tr>
            </tbody>
          </table>
          <footer>Fonte · {{ fonte('socios') }}</footer>
        </article>

        <!-- Atividade e porte -->
        <article v-if="mostraAtividade" class="intel-card card" data-card="atividade">
          <h3>Atividade e porte</h3>
          <dl>
            <template v-if="nivelAtividade !== null">
              <dt>Nível de atividade</dt>
              <dd>
                <div class="bar"><span :style="{ width: nivelAtividade + '%' }" :class="nivelAtividade >= 70 ? 'ok' : nivelAtividade >= 40 ? 'warn' : 'risk'"></span></div>
                <span class="small">{{ nivelAtividade }}%</span>
              </dd>
            </template>
            <template v-if="indicadores?.funcionarios_faixa || campos.employees_range">
              <dt>Funcionários</dt><dd>{{ indicadores?.funcionarios_faixa || campos.employees_range }}</dd>
            </template>
            <template v-if="indicadores?.faturamento_faixa || campos.revenue_range">
              <dt>Faturamento presumido</dt><dd>{{ indicadores?.faturamento_faixa || campos.revenue_range }}</dd>
            </template>
            <template v-if="flags.length">
              <dt>Sinais digitais</dt>
              <dd class="tags">
                <span v-for="f in flags" :key="f.rotulo" class="badge" :class="f.valor ? 'green' : 'gray'">{{ f.valor ? '✓' : '✕' }} {{ f.rotulo }}</span>
              </dd>
            </template>
          </dl>
          <footer>Fonte · {{ fonte('atividade') }}</footer>
        </article>

        <!-- Presença digital -->
        <article v-if="mostraPresenca" class="intel-card card" data-card="presenca">
          <h3>Presença digital</h3>
          <ul v-if="dominios.length" class="found">
            <li v-for="d in dominios" :key="d.dominio">
              <span class="ico">⌂</span>
              <a :href="`https://${d.dominio}`" target="_blank" rel="noopener" class="value">{{ d.dominio }}</a>
              <span class="badge" :class="d.ativo ? 'green' : 'gray'">{{ d.ativo ? 'ativo' : 'inativo' }}</span>
              <span v-if="d.ssl" class="badge blue">SSL</span>
            </li>
          </ul>
          <div v-if="redes.length" class="tags" style="margin-top: 8px">
            <a v-for="[rede, valor] in redes" :key="rede" class="badge" :href="urlRede(rede, valor)" target="_blank" rel="noopener">{{ rede }} ↗</a>
          </div>
          <footer>Fonte · {{ fonte('presenca') }}</footer>
        </article>

        <!-- Risco de crédito -->
        <article v-if="mostraRisco && risco" class="intel-card card risk" data-card="risco">
          <h3>Risco de crédito</h3>
          <div class="risk-body">
            <ScoreArc :score="risco.score" :faixa="risco.faixa" :nivel="risco.nivel" />
            <dl class="risk-facts">
              <template v-if="risco.pendencias_qtd !== undefined">
                <dt>Pendências</dt>
                <dd :class="{ bad: risco.pendencias_qtd }">
                  {{ risco.pendencias_qtd }}<span v-if="risco.pendencias_valor" class="muted small"> · {{ formatMoney(risco.pendencias_valor) }}</span>
                </dd>
              </template>
              <template v-if="risco.protestos_qtd !== undefined"><dt>Protestos</dt><dd :class="{ bad: risco.protestos_qtd }">{{ risco.protestos_qtd }}</dd></template>
              <template v-if="risco.cheques_sem_fundo !== undefined"><dt>Cheques sem fundo</dt><dd :class="{ bad: risco.cheques_sem_fundo }">{{ risco.cheques_sem_fundo }}</dd></template>
              <template v-if="risco.acoes_qtd !== undefined"><dt>Ações judiciais</dt><dd :class="{ bad: risco.acoes_qtd }">{{ risco.acoes_qtd }}</dd></template>
              <template v-if="risco.consultas_90d !== undefined"><dt>Consultas em 90 dias</dt><dd>{{ risco.consultas_90d }}</dd></template>
              <template v-if="risco.limite_sugerido !== undefined && risco.limite_sugerido !== null">
                <dt>Limite sugerido</dt><dd><strong>{{ formatMoney(risco.limite_sugerido) }}</strong></dd>
              </template>
            </dl>
          </div>
          <p v-if="risco.observacao" class="muted small">{{ risco.observacao }}</p>
          <footer>Fonte · {{ fonte('risco') }}</footer>
        </article>

        <!-- Processos -->
        <article v-if="mostraProcessos && processos" class="intel-card card" data-card="processos">
          <h3>Processos</h3>
          <div class="facts">
            <div v-if="processos.total !== undefined"><strong>{{ processos.total }}</strong><span>total</span></div>
            <div v-if="processos.ativos !== undefined"><strong>{{ processos.ativos }}</strong><span>ativos</span></div>
            <div v-if="processos.como_reu !== undefined"><strong>{{ processos.como_reu }}</strong><span>como réu</span></div>
            <div v-if="processos.valor_total !== undefined"><strong>{{ formatMoney(processos.valor_total) }}</strong><span>em disputa</span></div>
          </div>
          <table v-if="processos.ultimos?.length" class="mini">
            <thead>
              <tr><th>Processo</th><th>Tipo</th><th>Tribunal</th><th>Polo</th><th>Valor</th><th>Data</th></tr>
            </thead>
            <tbody>
              <tr v-for="(p, i) in processos.ultimos" :key="p.numero ?? i">
                <td class="mono">{{ p.numero || '—' }}<span v-if="p.status" class="badge gray" style="margin-left: 6px">{{ p.status }}</span></td>
                <td>{{ p.tipo || '—' }}</td>
                <td>{{ p.tribunal || '—' }}</td>
                <td>{{ p.polo || '—' }}</td>
                <td>{{ p.valor ? formatMoney(p.valor) : '—' }}</td>
                <td>{{ p.data ? dataCurta(p.data) : '—' }}</td>
              </tr>
            </tbody>
          </table>
          <footer>Fonte · {{ fonte('processos') }}</footer>
        </article>

        <!-- Conformidade -->
        <article v-if="mostraConformidade && conformidade" class="intel-card card" data-card="conformidade">
          <h3>Conformidade</h3>
          <div class="tags">
            <span class="badge" :class="conformidade.pep ? 'red' : 'green'">{{ conformidade.pep ? 'PEP encontrada' : 'Sem PEP' }}</span>
            <span class="badge" :class="conformidade.sancionado ? 'red' : 'green'">{{ conformidade.sancionado ? 'Sancionada' : 'Sem sanções' }}</span>
          </div>
          <ul v-if="conformidade.listas?.length" class="plain">
            <li v-for="l in conformidade.listas" :key="l">{{ l }}</li>
          </ul>
          <footer>Fonte · {{ fonte('conformidade') }}</footer>
        </article>

        <!-- Mídia -->
        <article v-if="mostraMidia && midia" class="intel-card card" data-card="midia">
          <h3>Mídia</h3>
          <p v-if="midia.exposicao" class="small">Exposição <strong>{{ midia.exposicao }}</strong></p>
          <ul v-if="midia.noticias?.length" class="news">
            <li v-for="(n, i) in midia.noticias" :key="n.url ?? i">
              <a v-if="n.url" :href="n.url" target="_blank" rel="noopener">{{ n.titulo }}</a>
              <span v-else>{{ n.titulo }}</span>
              <span class="meta">
                <span v-if="n.sentimento" class="badge" :class="sentimentoClasse[n.sentimento] ?? 'gray'">{{ n.sentimento }}</span>
                <span v-if="n.data" class="muted small">{{ dataCurta(n.data) }}</span>
              </span>
            </li>
          </ul>
          <footer>Fonte · {{ fonte('midia') }}</footer>
        </article>
      </div>
    </template>

    <!-- ===== Histórico ===== -->
    <section v-if="historico.length" class="history">
      <div class="history-head">
        <h3>Histórico de consultas</h3>
        <button v-if="canEnrich" type="button" class="btn btn-danger btn-sm" :disabled="removendo" @click="remover">
          {{ removendo ? 'Removendo…' : 'Remover dados de inteligência' }}
        </button>
      </div>
      <div class="table-wrap">
        <table class="data history-table">
          <thead>
            <tr><th>Provedor</th><th>Status</th><th>Custo</th><th>Usuário</th><th>Data</th><th>Aplicado</th></tr>
          </thead>
          <tbody>
            <tr v-for="h in historico" :key="h.id">
              <td>
                {{ nomeProvedor(h.provedor) }}
                <span v-if="h.demo" class="badge amber" style="margin-left: 6px">demo</span>
                <div v-if="h.datasets" class="muted small">{{ h.datasets }}</div>
              </td>
              <td><span class="badge" :class="statusClasse[h.status] ?? 'gray'">{{ statusRotulo[h.status] ?? h.status }}</span></td>
              <td>{{ h.custo_centavos ? centavos(h.custo_centavos) : 'grátis' }}</td>
              <td>{{ h.user_name || '—' }}</td>
              <td class="muted">{{ formatDateTime(h.created_at) }}</td>
              <td>
                <span v-if="h.aplicado_em" class="valid">✓ {{ dataCurta(h.aplicado_em) }}</span>
                <span v-else class="muted">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<style scoped>
.intel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.small {
  font-size: 12px;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.empty {
  text-align: center;
  padding: 36px 28px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}

.empty-icon {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--ci-purple-tint);
  color: var(--ci-purple);
  font-size: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty h3 {
  font-size: 16px;
}

.empty p {
  max-width: 520px;
  margin: 0;
  font-size: 13px;
}

.demo-banner {
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
  border-left: 3px solid var(--ci-amber);
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
}

.top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
  align-items: start;
}

.intel-card {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.intel-card h3 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
}

.intel-card footer {
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--ci-bg);
  font-size: 11px;
  color: var(--ci-text-3);
}

.intel-card dl {
  margin: 0;
  display: grid;
  grid-template-columns: minmax(110px, auto) 1fr;
  gap: 6px 12px;
  font-size: 13px;
}

.intel-card dt {
  color: var(--ci-text-3);
  font-size: 12px;
}

.intel-card dd {
  margin: 0;
  word-break: break-word;
}

.tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.secondary {
  margin-top: 6px;
  font-size: 12px;
}

.secondary summary {
  cursor: pointer;
  color: var(--ci-purple);
}

.secondary ul {
  margin: 6px 0 0;
  padding-left: 16px;
  color: var(--ci-text-2);
}

.address {
  margin: 0;
  font-size: 14px;
}

.found {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
}

.found li {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.found .ico {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  flex-shrink: 0;
}

.found .value {
  font-weight: 500;
}

.valid {
  color: var(--ci-green);
  font-weight: 700;
}

.spacer {
  flex: 1;
}

.use {
  border: 1px solid var(--ci-border);
  background: var(--ci-surface);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 12px;
  color: var(--ci-purple);
  cursor: pointer;
}

.use:hover:not(:disabled) {
  border-color: var(--ci-purple);
  background: var(--ci-purple-tint);
}

.mini {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.mini th {
  text-align: left;
  font-size: 11px;
  color: var(--ci-text-3);
  padding: 4px 6px;
  border-bottom: 1px solid var(--ci-border);
}

.mini td {
  padding: 6px;
  border-bottom: 1px solid var(--ci-bg);
  vertical-align: top;
}

.bar {
  height: 8px;
  border-radius: 999px;
  background: var(--ci-bg);
  overflow: hidden;
  margin: 4px 0;
}

.bar span {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--ci-green);
  transition: width 0.4s;
}

.bar span.warn {
  background: var(--ci-amber);
}

.bar span.risk {
  background: var(--ci-red);
}

.risk-body {
  display: flex;
  gap: 16px;
  align-items: center;
  flex-wrap: wrap;
}

.risk-facts {
  flex: 1;
  min-width: 160px;
}

.bad {
  color: var(--ci-red);
  font-weight: 600;
}

.facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(90px, 1fr));
  gap: 8px;
}

.facts div {
  background: var(--ci-bg);
  border-radius: 8px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.facts strong {
  font-size: 16px;
  color: var(--ci-purple-dark);
}

.facts span {
  font-size: 11px;
  color: var(--ci-text-3);
}

.plain {
  margin: 0;
  padding-left: 16px;
  font-size: 13px;
}

.news {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
}

.news li {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.news .meta {
  display: flex;
  gap: 8px;
  align-items: center;
}

.history-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}

.history-head h3 {
  font-size: 14px;
}

.history-table tbody tr {
  cursor: default;
}

.history-table tbody tr:hover {
  background: transparent;
}
</style>
