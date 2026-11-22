<script setup lang="ts">
// Configurações → Prospecção: janela de contato, cadência, meta, WhatsApp,
// destino padrão da conversão, roteiro, motivos de descarte e a lista de
// bloqueios ("não contatar"). Contrato: docs/PROSPECCAO.md 8.3, 8.4 e 10.5.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, ApiError } from '../api'
import { cnpjValido, mascaraCNPJ, normalizarCNPJ } from '../cnpj'
import ModalDialog from '../components/ModalDialog.vue'
import PaginationBar from '../components/ui/PaginationBar.vue'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import { formatarTelefone, normalizarTelefone, telefoneValido } from '../telefone'
import type { Pipeline, Playbook } from '../types'
import {
  CONFIG_PROSPECCAO_PADRAO,
  motivoBloqueioLabels,
  tipoBloqueioLabels,
  type Bloqueio,
  type BloqueiosResposta,
  type ConfigProspeccao,
  type ImportarBloqueiosResposta,
  type MotivoBloqueio,
  type TipoBloqueio
} from '../types-prospeccao'

const toast = useToastStore()

// ===== Configurações =====
const form = ref<ConfigProspeccao>(clonar(CONFIG_PROSPECCAO_PADRAO))
const carregado = ref(false)
const salvando = ref(false)
const erros = ref<string[]>([])
const pipelines = ref<Pipeline[]>([])
const playbooks = ref<Playbook[]>([])
const intervalosTexto = ref(CONFIG_PROSPECCAO_PADRAO.intervalos_horas.join(', '))
const novoMotivo = ref('')

/** Teto do sistema para a janela de contato (8h às 20h). */
const HORA_MIN = 8
const HORA_MAX = 20
const horas = Array.from({ length: HORA_MAX - HORA_MIN + 1 }, (_, i) => HORA_MIN + i)

function clonar<T>(v: T): T {
  return JSON.parse(JSON.stringify(v))
}

function aplicar(c: Partial<ConfigProspeccao> | null | undefined) {
  const base = clonar(CONFIG_PROSPECCAO_PADRAO)
  const novo = { ...base, ...(c ?? {}) }
  if (!Array.isArray(novo.intervalos_horas)) novo.intervalos_horas = base.intervalos_horas
  if (!Array.isArray(novo.motivos_descarte)) novo.motivos_descarte = base.motivos_descarte
  form.value = novo
  intervalosTexto.value = novo.intervalos_horas.join(', ')
}

const etapas = computed(() => pipelines.value.find((p) => p.id === form.value.pipeline_id)?.stages ?? [])

function aoTrocarFunil() {
  if (!etapas.value.some((s) => s.id === form.value.stage_id)) form.value.stage_id = null
}

function lerIntervalos(): number[] | null {
  const partes = intervalosTexto.value
    .split(/[\s,;]+/)
    .map((p) => p.trim())
    .filter(Boolean)
  const nums = partes.map((p) => Number(p.replace(/h$/i, '')))
  if (!nums.length || nums.some((n) => !Number.isFinite(n) || n <= 0)) return null
  return nums.map((n) => Math.round(n))
}

function adicionarMotivo() {
  const texto = novoMotivo.value.trim()
  if (!texto) return
  if (form.value.motivos_descarte.some((m) => m.toLowerCase() === texto.toLowerCase())) {
    toast.error('Esse motivo já está na lista')
    return
  }
  form.value.motivos_descarte = [...form.value.motivos_descarte, texto]
  novoMotivo.value = ''
}

function removerMotivo(i: number) {
  form.value.motivos_descarte = form.value.motivos_descarte.filter((_, idx) => idx !== i)
}

function validar(): string[] {
  const f = form.value
  const out: string[] = []
  if (f.janela_inicio < HORA_MIN || f.janela_fim > HORA_MAX) out.push('A janela de contato precisa ficar entre 8h e 20h.')
  if (f.janela_inicio >= f.janela_fim) out.push('O início da janela precisa ser antes do fim.')
  if (!lerIntervalos()) out.push('Informe os intervalos entre tentativas em horas, separados por vírgula (ex.: 3, 24, 48).')
  if (!(f.max_tentativas >= 1)) out.push('O máximo de tentativas por lead precisa ser pelo menos 1.')
  if (!(f.tentativas_dia >= 1)) out.push('O limite de tentativas por dia precisa ser pelo menos 1.')
  if (!(f.retorno_padrao_horas >= 1)) out.push('O retorno padrão precisa ser de pelo menos 1 hora.')
  if (!(f.meta_ligacoes_dia >= 0)) out.push('A meta diária não pode ser negativa.')
  if (!f.motivos_descarte.length) out.push('Deixe pelo menos um motivo de descarte.')
  return out
}

async function carregar() {
  try {
    aplicar(await api.get<ConfigProspeccao>('/settings/prospeccao'))
    carregado.value = true
  } catch (e: any) {
    toast.error(e.message)
  }
  // Seletores opcionais: quem não tem acesso a funis ou a manuais ainda configura o resto.
  try {
    pipelines.value = (await api.get<Pipeline[]>('/pipelines')) ?? []
  } catch {
    pipelines.value = []
  }
  try {
    playbooks.value = ((await api.get<Playbook[]>('/playbooks')) ?? []).filter((p) => p.active !== false || p.id === form.value.playbook_id)
  } catch {
    playbooks.value = []
  }
}

async function salvar() {
  erros.value = validar()
  if (erros.value.length) return
  salvando.value = true
  try {
    const corpo: ConfigProspeccao = { ...form.value, intervalos_horas: lerIntervalos() ?? form.value.intervalos_horas }
    const r = await api.put<ConfigProspeccao>('/settings/prospeccao', corpo)
    aplicar(r && typeof r === 'object' && 'janela_inicio' in r ? r : corpo)
    toast.push('Configurações de prospecção salvas')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    salvando.value = false
  }
}

// ===== Bloqueios =====
const bloqueios = ref<Bloqueio[]>([])
const total = ref(0)
const page = ref(1)
const perPage = ref(25)
const busca = ref('')
const filtroTipo = ref<TipoBloqueio | ''>('')
const carregandoBloqueios = ref(false)
let pedido = 0
let timer: ReturnType<typeof setTimeout> | null = null

const tipos = Object.keys(tipoBloqueioLabels) as TipoBloqueio[]
const motivos = Object.keys(motivoBloqueioLabels) as MotivoBloqueio[]

async function carregarBloqueios() {
  const meu = ++pedido
  carregandoBloqueios.value = true
  try {
    const params = new URLSearchParams({ page: String(page.value), per_page: String(perPage.value) })
    if (busca.value.trim()) params.set('q', busca.value.trim())
    if (filtroTipo.value) params.set('tipo', filtroTipo.value)
    const r = await api.get<BloqueiosResposta>(`/bloqueios?${params}`)
    if (meu !== pedido) return // resposta atrasada
    bloqueios.value = r?.data ?? []
    total.value = r?.pagination?.total ?? bloqueios.value.length
  } catch (e: any) {
    if (meu === pedido) toast.error(e.message)
  } finally {
    if (meu === pedido) carregandoBloqueios.value = false
  }
}

watch([busca, filtroTipo], () => {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => {
    if (page.value !== 1) page.value = 1
    else carregarBloqueios()
  }, 300)
})
watch([page, perPage], carregarBloqueios)

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
})

function valorFormatado(b: Pick<Bloqueio, 'tipo' | 'valor'>): string {
  if (b.tipo === 'telefone') return formatarTelefone(b.valor) || b.valor
  if (b.tipo === 'cnpj') return mascaraCNPJ(b.valor) || b.valor
  return b.valor
}

function rotuloMotivo(m: string): string {
  return (motivoBloqueioLabels as Record<string, string>)[m] ?? m
}

/** Normaliza como o servidor guarda; vazio = valor inválido para o tipo. */
function normalizar(tipo: TipoBloqueio, bruto: string): string {
  const v = bruto.trim()
  if (tipo === 'telefone') {
    const d = normalizarTelefone(v)
    return telefoneValido(d) ? d : ''
  }
  if (tipo === 'cnpj') {
    const d = normalizarCNPJ(v)
    return cnpjValido(d) ? d : ''
  }
  const e = v.toLowerCase()
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e) ? e : ''
}

const exemplos: Record<TipoBloqueio, string> = {
  telefone: '(85) 99999-0001',
  email: 'pessoa@empresa.com.br',
  cnpj: '00.000.000/0000-00'
}

// Adicionar um
const modalNovo = ref(false)
const novo = ref<{ tipo: TipoBloqueio; valor: string; motivo: MotivoBloqueio; observacao: string }>({ tipo: 'telefone', valor: '', motivo: 'manual', observacao: '' })
const erroNovo = ref('')
const gravando = ref(false)

function abrirNovo() {
  novo.value = { tipo: filtroTipo.value || 'telefone', valor: '', motivo: 'manual', observacao: '' }
  erroNovo.value = ''
  modalNovo.value = true
}

async function criarBloqueio() {
  const valor = normalizar(novo.value.tipo, novo.value.valor)
  if (!valor) {
    erroNovo.value = `${tipoBloqueioLabels[novo.value.tipo]} inválido. Exemplo: ${exemplos[novo.value.tipo]}`
    return
  }
  gravando.value = true
  erroNovo.value = ''
  try {
    const corpo: Record<string, unknown> = { tipo: novo.value.tipo, valor, motivo: novo.value.motivo }
    if (novo.value.observacao.trim()) corpo.observacao = novo.value.observacao.trim()
    await api.post('/bloqueios', corpo)
    modalNovo.value = false
    toast.push('Bloqueio adicionado')
    await carregarBloqueios()
  } catch (e: any) {
    erroNovo.value = e instanceof ApiError && e.status === 409 ? 'Esse valor já está bloqueado.' : e.message
  } finally {
    gravando.value = false
  }
}

// Importar colando
const LIMITE_IMPORTACAO = 5000
const modalImportar = ref(false)
const importar = ref<{ tipo: TipoBloqueio; motivo: MotivoBloqueio; texto: string }>({ tipo: 'telefone', motivo: 'lista_externa', texto: '' })
const importando = ref(false)
const erroImportar = ref('')
const resultadoImportar = ref<ImportarBloqueiosResposta | null>(null)

/** Telefone pode ter espaço no meio ("(85) 99999-0001"): só quebra em linha, vírgula e ponto e vírgula. */
const colados = computed(() => {
  const separador = importar.value.tipo === 'telefone' ? /[\n\r,;|\t]+/ : /[\s,;|]+/
  const validos: string[] = []
  const vistos = new Set<string>()
  let invalidos = 0
  let repetidos = 0
  for (const trecho of importar.value.texto.split(separador)) {
    if (!trecho.trim()) continue
    const v = normalizar(importar.value.tipo, trecho)
    if (!v) invalidos++
    else if (vistos.has(v)) repetidos++
    else {
      vistos.add(v)
      validos.push(v)
    }
  }
  return { validos, invalidos, repetidos }
})

const passouDoLimite = computed(() => colados.value.validos.length > LIMITE_IMPORTACAO)

function abrirImportar() {
  importar.value = { tipo: filtroTipo.value || 'telefone', motivo: 'lista_externa', texto: '' }
  erroImportar.value = ''
  resultadoImportar.value = null
  modalImportar.value = true
}

async function enviarImportacao() {
  if (!colados.value.validos.length || passouDoLimite.value) return
  importando.value = true
  erroImportar.value = ''
  try {
    const r = await api.post<ImportarBloqueiosResposta>('/bloqueios/importar', {
      tipo: importar.value.tipo,
      motivo: importar.value.motivo,
      valores: colados.value.validos
    })
    resultadoImportar.value = { criados: r?.criados ?? 0, repetidos: (r?.repetidos ?? 0) + colados.value.repetidos, invalidos: (r?.invalidos ?? 0) + colados.value.invalidos }
    toast.push(`${resultadoImportar.value.criados} ${resultadoImportar.value.criados === 1 ? 'bloqueio criado' : 'bloqueios criados'}`)
    importar.value.texto = ''
    await carregarBloqueios()
  } catch (e: any) {
    erroImportar.value = e.message
  } finally {
    importando.value = false
  }
}

// Remover
const removendo = ref<number | null>(null)

async function remover(b: Bloqueio) {
  if (!confirm(`Remover o bloqueio de ${valorFormatado(b)}? Esse contato volta a poder entrar em listas e na fila de ligações.`)) return
  removendo.value = b.id
  try {
    await api.delete(`/bloqueios/${b.id}`)
    toast.push('Bloqueio removido')
    if (bloqueios.value.length === 1 && page.value > 1) page.value--
    else await carregarBloqueios()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    removendo.value = null
  }
}

onMounted(() => {
  carregar()
  carregarBloqueios()
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Prospecção</h1>
        <p class="muted" style="margin: 4px 0 0">Regras da fila de ligações, destino dos leads que viram negócio e a lista de quem não deve ser contatado.</p>
      </div>
    </div>

    <form class="blocos" data-bloco="config" @submit.prevent="salvar">
      <section class="card" data-secao="janela">
        <h2>Janela de contato</h2>
        <p class="muted">A fila só entrega leads dentro deste horário, em dias úteis. Domingos e feriados nacionais nunca entram; o sistema não permite antes das 8h nem depois das 20h.</p>
        <div class="form-row">
          <div class="field">
            <label for="janela-inicio">Começa às</label>
            <select id="janela-inicio" v-model.number="form.janela_inicio">
              <option v-for="h in horas.slice(0, -1)" :key="h" :value="h">{{ h }}h</option>
            </select>
          </div>
          <div class="field">
            <label for="janela-fim">Termina às</label>
            <select id="janela-fim" v-model.number="form.janela_fim">
              <option v-for="h in horas.slice(1)" :key="h" :value="h">{{ h }}h</option>
            </select>
          </div>
        </div>
        <label class="switch">
          <input v-model="form.sabado" type="checkbox" data-campo="sabado" />
          <span>Ligar também aos sábados</span>
        </label>
      </section>

      <section class="card" data-secao="cadencia">
        <h2>Cadência de tentativas</h2>
        <p class="muted">Quantas vezes insistir em um lead que não atende e quanto esperar entre uma tentativa e outra.</p>
        <div class="form-row">
          <div class="field">
            <label for="max-tentativas">Máximo de tentativas por lead</label>
            <input id="max-tentativas" v-model.number="form.max_tentativas" type="number" min="1" max="30" />
            <small class="muted">Depois disso o lead é descartado como "sem contato após as tentativas".</small>
          </div>
          <div class="field">
            <label for="tentativas-dia">Tentativas por dia, por lead</label>
            <input id="tentativas-dia" v-model.number="form.tentativas_dia" type="number" min="1" max="10" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label for="intervalos">Espera entre tentativas (horas)</label>
            <input id="intervalos" v-model="intervalosTexto" placeholder="3, 24, 48, 72, 120" />
            <small class="muted">Depois da 1ª, da 2ª, da 3ª tentativa sem contato, e assim por diante. O último valor vale para as seguintes.</small>
          </div>
          <div class="field">
            <label for="retorno">Retorno padrão (horas)</label>
            <input id="retorno" v-model.number="form.retorno_padrao_horas" type="number" min="1" />
            <small class="muted">Sugestão de data quando o resultado é "Retornar".</small>
          </div>
        </div>
      </section>

      <section class="card" data-secao="fila">
        <h2>Fila e meta</h2>
        <div class="form-row">
          <div class="field">
            <label for="meta">Meta diária de ligações por vendedor</label>
            <input id="meta" v-model.number="form.meta_ligacoes_dia" type="number" min="0" />
          </div>
          <div class="field"></div>
        </div>
        <label class="switch">
          <input v-model="form.incluir_sem_dono_na_fila" type="checkbox" data-campo="incluir_sem_dono_na_fila" />
          <span>Entregar leads sem dono na fila de quem estiver ligando</span>
        </label>
        <label class="switch">
          <input v-model="form.exigir_optin_whatsapp" type="checkbox" data-campo="exigir_optin_whatsapp" />
          <span>WhatsApp só com consentimento registrado no lead</span>
        </label>
        <p class="hint">Desligado, o botão de WhatsApp aparece para qualquer celular. Mensagem sem pedido prévio pode ser tratada como abordagem indevida: a decisão é da sua empresa.</p>
      </section>

      <section class="card" data-secao="destino">
        <h2>Destino padrão da conversão</h2>
        <p class="muted">Onde nasce o negócio quando um lead vira reunião ou oportunidade e a lista não define outro funil.</p>
        <div class="form-row">
          <div class="field">
            <label for="funil">Funil</label>
            <select id="funil" v-model="form.pipeline_id" @change="aoTrocarFunil">
              <option :value="null">Automático (etapa de reunião ou a primeira)</option>
              <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
          </div>
          <div class="field">
            <label for="etapa">Etapa</label>
            <select id="etapa" v-model="form.stage_id" :disabled="!form.pipeline_id">
              <option :value="null">Automática</option>
              <option v-for="s in etapas" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </div>
        </div>
        <div class="field" style="max-width: 420px">
          <label for="roteiro">Roteiro padrão da ligação</label>
          <select id="roteiro" v-model="form.playbook_id">
            <option :value="null">Sem roteiro</option>
            <option v-for="p in playbooks" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
          <small class="muted">Um manual de atividades. Cada lista pode escolher outro.</small>
        </div>
      </section>

      <section class="card" data-secao="motivos">
        <h2>Motivos de descarte</h2>
        <p class="muted">Opções oferecidas ao descartar um lead ou marcar "Sem interesse".</p>
        <div class="chips">
          <span v-for="(m, i) in form.motivos_descarte" :key="m" class="chip" data-motivo>
            {{ m }}
            <button type="button" class="chip-x" :aria-label="`Remover ${m}`" @click="removerMotivo(i)">×</button>
          </span>
        </div>
        <div class="add-motivo">
          <input v-model="novoMotivo" placeholder="Novo motivo" maxlength="80" data-campo="novo-motivo" @keydown.enter.prevent="adicionarMotivo" />
          <button type="button" class="btn btn-outline btn-sm" :disabled="!novoMotivo.trim()" @click="adicionarMotivo">Adicionar</button>
        </div>
      </section>

      <ul v-if="erros.length" class="erros error-text" role="alert">
        <li v-for="e in erros" :key="e">{{ e }}</li>
      </ul>

      <div class="actions">
        <button class="btn btn-primary" type="submit" :disabled="salvando || !carregado">{{ salvando ? 'Salvando…' : 'Salvar configurações' }}</button>
      </div>
    </form>

    <section class="card" data-bloco="bloqueios">
      <div class="bloqueios-head">
        <div>
          <h2>Bloqueios ("não contatar")</h2>
          <p class="muted">Telefones, e-mails e CNPJs que ficam fora das listas, do envio ao CRM e da fila. O bloqueio continua valendo mesmo que o lead seja excluído.</p>
        </div>
        <div class="bloqueios-acoes">
          <button type="button" class="btn btn-outline" data-acao="importar" @click="abrirImportar">Importar colando</button>
          <button type="button" class="btn btn-primary" data-acao="adicionar" @click="abrirNovo">Adicionar</button>
        </div>
      </div>

      <div class="toolbar">
        <input v-model="busca" type="search" placeholder="Buscar por telefone, e-mail ou CNPJ" aria-label="Buscar bloqueio" />
        <select v-model="filtroTipo" aria-label="Tipo">
          <option value="">Todos os tipos</option>
          <option v-for="t in tipos" :key="t" :value="t">{{ tipoBloqueioLabels[t] }}</option>
        </select>
      </div>

      <div v-if="bloqueios.length" class="table-wrap">
        <table class="data">
          <thead>
            <tr><th>Tipo</th><th>Valor</th><th>Motivo</th><th>Observação</th><th>Quem bloqueou</th><th>Quando</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="b in bloqueios" :key="b.id" data-bloqueio>
              <td>{{ tipoBloqueioLabels[b.tipo] ?? b.tipo }}</td>
              <td class="valor">{{ valorFormatado(b) }}</td>
              <td>{{ rotuloMotivo(b.motivo) }}<span v-if="b.canal" class="muted small"> · {{ b.canal }}</span></td>
              <td class="obs">{{ b.observacao || '—' }}</td>
              <td>{{ b.criado_por_nome || 'Sistema' }}</td>
              <td>{{ formatDate(b.criado_em) }}</td>
              <td class="right">
                <button type="button" class="btn btn-outline btn-sm" :disabled="removendo === b.id" @click="remover(b)">Remover</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty-state">
        <template v-if="carregandoBloqueios">Carregando…</template>
        <template v-else-if="busca.trim() || filtroTipo">
          <strong>Nada encontrado</strong>
          Nenhum bloqueio combina com a busca.
        </template>
        <template v-else>
          <strong>Nenhum bloqueio ainda</strong>
          Quando alguém pedir para não ser contatado, o número entra aqui sozinho. Você também pode colar uma lista.
        </template>
      </div>

      <PaginationBar v-if="total > perPage || page > 1" v-model:page="page" v-model:perPage="perPage" :total="total" :per-page-options="[25, 50, 100]" :disabled="carregandoBloqueios" rotulo="bloqueios" />
    </section>

    <!-- Adicionar um bloqueio -->
    <ModalDialog title="Adicionar bloqueio" :open="modalNovo" @close="modalNovo = false">
      <form id="form-bloqueio" @submit.prevent="criarBloqueio">
        <div class="form-row">
          <div class="field">
            <label for="novo-tipo">Tipo</label>
            <select id="novo-tipo" v-model="novo.tipo">
              <option v-for="t in tipos" :key="t" :value="t">{{ tipoBloqueioLabels[t] }}</option>
            </select>
          </div>
          <div class="field">
            <label for="novo-valor">{{ tipoBloqueioLabels[novo.tipo] }}</label>
            <input id="novo-valor" v-model="novo.valor" :placeholder="exemplos[novo.tipo]" autocomplete="off" />
          </div>
        </div>
        <div class="field">
          <label for="novo-motivo">Motivo</label>
          <select id="novo-motivo" v-model="novo.motivo">
            <option v-for="m in motivos" :key="m" :value="m">{{ motivoBloqueioLabels[m] }}</option>
          </select>
        </div>
        <div class="field">
          <label for="novo-obs">Observação (opcional)</label>
          <textarea id="novo-obs" v-model="novo.observacao" rows="2" maxlength="500"></textarea>
        </div>
        <p v-if="erroNovo" class="error-text" role="alert">{{ erroNovo }}</p>
      </form>
      <template #footer>
        <button type="button" class="btn btn-outline" @click="modalNovo = false">Cancelar</button>
        <button type="submit" form="form-bloqueio" class="btn btn-primary" :disabled="gravando || !novo.valor.trim()">{{ gravando ? 'Salvando…' : 'Bloquear' }}</button>
      </template>
    </ModalDialog>

    <!-- Importar colando -->
    <ModalDialog title="Importar bloqueios" :open="modalImportar" wide @close="modalImportar = false">
      <form id="form-importar" @submit.prevent="enviarImportacao">
        <div class="form-row">
          <div class="field">
            <label for="imp-tipo">Tipo</label>
            <select id="imp-tipo" v-model="importar.tipo">
              <option v-for="t in tipos" :key="t" :value="t">{{ tipoBloqueioLabels[t] }}</option>
            </select>
          </div>
          <div class="field">
            <label for="imp-motivo">Motivo</label>
            <select id="imp-motivo" v-model="importar.motivo">
              <option v-for="m in motivos" :key="m" :value="m">{{ motivoBloqueioLabels[m] }}</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label for="imp-texto">Cole os valores, um por linha</label>
          <textarea id="imp-texto" v-model="importar.texto" rows="8" :placeholder="exemplos[importar.tipo]"></textarea>
          <small class="muted">Até 5.000 por vez. Serve para a lista do Procon do seu estado, do "Não Me Perturbe" ou de clientes atuais.</small>
        </div>
        <p v-if="importar.texto.trim()" class="resumo-colados" data-resumo>
          <strong>{{ colados.validos.length }}</strong> {{ colados.validos.length === 1 ? 'válido' : 'válidos' }}
          <span v-if="colados.invalidos"> · {{ colados.invalidos }} {{ colados.invalidos === 1 ? 'inválido' : 'inválidos' }}</span>
          <span v-if="colados.repetidos"> · {{ colados.repetidos }} {{ colados.repetidos === 1 ? 'repetido' : 'repetidos' }}</span>
        </p>
        <p v-if="passouDoLimite" class="error-text" role="alert">São mais de 5.000 valores. Divida a lista e importe em partes.</p>
        <p v-if="erroImportar" class="error-text" role="alert">{{ erroImportar }}</p>
        <p v-if="resultadoImportar" class="success-text" data-resultado>
          {{ resultadoImportar.criados }} {{ resultadoImportar.criados === 1 ? 'criado' : 'criados' }} · {{ resultadoImportar.repetidos }} já
          {{ resultadoImportar.repetidos === 1 ? 'existia' : 'existiam' }} · {{ resultadoImportar.invalidos }} {{ resultadoImportar.invalidos === 1 ? 'inválido' : 'inválidos' }}
        </p>
      </form>
      <template #footer>
        <button type="button" class="btn btn-outline" @click="modalImportar = false">Fechar</button>
        <button type="submit" form="form-importar" class="btn btn-primary" :disabled="importando || !colados.validos.length || passouDoLimite">
          {{ importando ? 'Importando…' : `Bloquear ${colados.validos.length || ''}`.trim() }}
        </button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.blocos {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 860px;
  margin-bottom: 24px;
}

.card h2 {
  margin: 0 0 4px;
  font-size: 16px;
}

.card > p.muted {
  margin: 0 0 14px;
  font-size: 13px;
}

.switch {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0;
  font-size: 14px;
  cursor: pointer;
}

.hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--ci-text-3);
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.chips .chip {
  cursor: default;
}

.add-motivo {
  display: flex;
  gap: 8px;
  max-width: 420px;
}

.add-motivo input {
  flex: 1;
  padding: 7px 10px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  font: inherit;
}

.erros {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
}

.actions {
  display: flex;
  justify-content: flex-end;
}

.bloqueios-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.bloqueios-head p {
  margin: 0;
  font-size: 13px;
  max-width: 620px;
}

.bloqueios-acoes {
  display: flex;
  gap: 8px;
}

.valor {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.obs {
  max-width: 260px;
  overflow-wrap: anywhere;
}

.right {
  text-align: right;
}

.small {
  font-size: 12px;
}

.resumo-colados {
  margin: 0 0 8px;
  font-size: 13px;
}
</style>
