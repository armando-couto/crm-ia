<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, foiAbortado } from '../../api'
import { formatDate, formatDateTime, formatMoney, mascaraCNPJ } from '../../format'
import { formatarTelefone } from '../../telefone'
import {
  emailTipoLabels,
  estadoItemLabels,
  estadoItemTons,
  origemContatoLabels,
  papelPessoaLabels,
  porteLabels,
  tipoDividaLabels,
  tipoSocioLabels,
  tipoTelefoneLabels,
  type Item,
  type Telefone
} from '../../types-prospeccao'
import SideDrawer from '../ui/SideDrawer.vue'
import NotaBadge from './NotaBadge.vue'
import { formatarCnae } from './filtros'

/**
 * Ficha do item da lista: cadastro, contatos (com a origem de cada um),
 * sócios, dívida ativa e enriquecimento. CPF nunca aparece: os documentos já
 * chegam mascarados.
 */
const props = defineProps<{ itemId: number | null }>()
const emit = defineEmits<{ close: [] }>()

const item = ref<Item | null>(null)
const carregando = ref(false)
const erro = ref('')
let controle: AbortController | null = null

async function carregar(id: number) {
  controle?.abort()
  controle = new AbortController()
  carregando.value = true
  erro.value = ''
  try {
    item.value = await api.get<Item>(`/prospeccao/itens/${id}`, { signal: controle.signal })
  } catch (e: any) {
    if (foiAbortado(e)) return
    erro.value = e?.message || 'não foi possível abrir a ficha'
    item.value = null
  } finally {
    carregando.value = false
  }
}

watch(
  () => props.itemId,
  (id) => {
    if (id) void carregar(id)
    else {
      controle?.abort()
      item.value = null
    }
  },
  { immediate: true }
)

const dados = computed(() => item.value?.dados ?? null)
const perfil = computed(() => item.value?.enriquecimento ?? null)

/** Telefones da base e do enriquecimento, sem repetir número, com a origem de cada um. */
const telefones = computed<Telefone[]>(() => {
  const vistos = new Set<string>()
  const saida: Telefone[] = []
  const juntar = (lista: Telefone[] | undefined, origem: Telefone['origem']) => {
    for (const t of lista ?? []) {
      if (!t?.numero || vistos.has(t.numero)) continue
      vistos.add(t.numero)
      saida.push({ ...t, origem: t.origem ?? origem })
    }
  }
  juntar(perfil.value?.telefones, 'wholead')
  juntar(dados.value?.telefones, 'receita')
  return saida
})

const emails = computed(() => {
  const saida: { email: string; tipo: string; origem: string; pessoa?: string }[] = []
  const vistos = new Set<string>()
  for (const e of perfil.value?.emails ?? []) {
    const k = e.email.toLowerCase()
    if (vistos.has(k)) continue
    vistos.add(k)
    saida.push({ email: e.email, tipo: e.tipo ?? '', origem: 'wholead', pessoa: e.pessoa })
  }
  const d = dados.value
  if (d?.email && !vistos.has(d.email.toLowerCase())) {
    saida.push({ email: d.email, tipo: emailTipoLabels[d.email_tipo] ?? '', origem: 'receita' })
  }
  return saida
})

const endereco = computed(() => {
  const e = dados.value?.endereco
  if (!e) return ''
  const rua = [e.tipo_logradouro, e.logradouro].filter(Boolean).join(' ')
  const linha1 = [rua, e.numero, e.complemento].filter(Boolean).join(', ')
  const cep = e.cep ? `CEP ${e.cep.replace(/^(\d{5})(\d{3})$/, '$1-$2')}` : ''
  return [linha1, e.bairro, `${e.municipio}/${e.uf}`, cep].filter(Boolean).join(' · ')
})

const titulo = computed(() => item.value?.nome_fantasia || item.value?.razao_social || 'Empresa')
</script>

<template>
  <SideDrawer :open="!!itemId" :title="titulo" :largura="560" @close="emit('close')">
    <template #title>
      <div class="titulo">
        <h3>{{ titulo }}</h3>
        <div v-if="item" class="selos">
          <span class="badge" :class="estadoItemTons[item.estado]">{{ estadoItemLabels[item.estado] }}</span>
          <NotaBadge :nota="item.nota" :motivos="item.nota_motivos" />
          <span v-if="item.pessoa_fisica" class="badge amber" title="MEI ou empresário individual: a LGPD se aplica ao contato">PF</span>
          <span v-if="item.tem_divida" class="badge amber">dívida ativa</span>
          <span v-if="item.tem_whatsapp" class="badge green">WhatsApp</span>
          <span v-if="item.contato_compartilhado" class="badge gray" title="O único contato é compartilhado ou de contabilidade">contador</span>
        </div>
      </div>
    </template>

    <div v-if="carregando && !item" class="muted vazio">Carregando a ficha…</div>
    <p v-else-if="erro" class="error-text" role="alert">{{ erro }}</p>

    <div v-else-if="item" class="ficha">
      <p class="sub">
        {{ mascaraCNPJ(item.cnpj) }}
        <span v-if="item.nome_fantasia"> · {{ item.razao_social }}</span>
        <br />
        {{ item.cnae_descricao }} · {{ item.municipio }}/{{ item.uf }}
      </p>

      <div v-if="item.pessoa_fisica" class="faixa amber">
        Pessoa física (MEI ou empresário individual): use o contato só para a oferta à empresa e respeite pedidos de não contato.
      </div>
      <p v-if="item.erro" class="error-text">Último erro: {{ item.erro }}</p>
      <p v-if="item.estado === 'enviado' || item.company_id || item.lead_id" class="enviado">
        <span v-if="item.enviado_em">Enviado ao CRM em {{ formatDateTime(item.enviado_em) }}. </span>
        <RouterLink v-if="item.company_id" :to="`/empresas/${item.company_id}`">Abrir a empresa</RouterLink>
        <template v-if="item.lead_id">
          <span v-if="item.company_id"> · </span>
          <RouterLink :to="{ path: '/leads', query: { lead: String(item.lead_id) } }">Abrir o lead</RouterLink>
        </template>
      </p>

      <section>
        <h4>Contatos</h4>
        <ul v-if="telefones.length" class="lista">
          <li v-for="t in telefones" :key="t.numero" :class="{ 'telefone-invalido': t.invalido }">
            <strong class="numero">{{ formatarTelefone(t.numero) }}</strong>
            <span class="muted">{{ tipoTelefoneLabels[t.tipo] }}</span>
            <span v-if="t.pessoa" class="muted">· {{ t.pessoa }}</span>
            <span class="badge gray origem">{{ origemContatoLabels[t.origem ?? 'receita'] }}</span>
            <span v-if="t.whatsapp" class="badge green">WhatsApp</span>
            <span v-if="t.compartilhado" class="badge amber" title="Mesmo número em 5 ou mais empresas: provável contador">compartilhado</span>
            <span v-if="t.nao_perturbe" class="badge red" title="Bloqueio de telemarketing informado pelo provedor">não perturbe</span>
          </li>
        </ul>
        <p v-else class="muted">Sem telefone na base.<span v-if="!perfil"> O enriquecimento pode encontrar telefones e decisores.</span></p>
        <ul v-if="emails.length" class="lista">
          <li v-for="e in emails" :key="e.email">
            <span class="numero">{{ e.email }}</span>
            <span v-if="e.tipo" class="muted">{{ e.tipo }}</span>
            <span v-if="e.pessoa" class="muted">· {{ e.pessoa }}</span>
            <span class="badge gray origem">{{ origemContatoLabels[e.origem as 'receita'] ?? e.origem }}</span>
          </li>
        </ul>
      </section>

      <section v-if="perfil?.pessoas?.length">
        <h4>Decisores e pessoas</h4>
        <div v-for="(p, i) in perfil.pessoas" :key="i" class="pessoa">
          <div>
            <strong>{{ p.nome }}</strong>
            <span class="muted"> · {{ p.cargo || papelPessoaLabels[p.papel] }}</span>
            <span v-if="p.documento" class="muted doc"> · {{ p.documento }}</span>
          </div>
          <div v-if="p.telefones?.length" class="pessoa-contatos">
            <span v-for="t in p.telefones" :key="t.numero" class="contato" :class="{ 'telefone-invalido': t.invalido }">
              {{ formatarTelefone(t.numero) }}
              <span v-if="t.whatsapp" class="badge green">WhatsApp</span>
              <span v-if="t.nao_perturbe" class="badge red">não perturbe</span>
            </span>
          </div>
          <div v-if="p.emails?.length" class="pessoa-contatos muted">{{ p.emails.join(', ') }}</div>
        </div>
      </section>

      <section v-if="dados?.socios?.length">
        <h4>Sócios ({{ dados.socios.length }})</h4>
        <ul class="lista">
          <li v-for="(s, i) in dados.socios" :key="i">
            <strong>{{ s.nome }}</strong>
            <span class="muted">{{ s.qualificacao }}</span>
            <span v-if="s.desde" class="muted">· desde {{ formatDate(s.desde) }}</span>
            <span v-if="s.tipo !== 'pf'" class="badge gray">{{ tipoSocioLabels[s.tipo] }}</span>
          </li>
        </ul>
      </section>

      <section>
        <h4>Dívida ativa (PGFN)</h4>
        <template v-if="dados?.divida">
          <dl class="campos">
            <dt>Total</dt>
            <dd>
              <strong>{{ formatMoney(dados.divida.total) }}</strong> em {{ dados.divida.inscricoes }} inscriç{{ dados.divida.inscricoes === 1 ? 'ão' : 'ões' }}
            </dd>
            <dt>Em cobrança</dt>
            <dd>{{ formatMoney(dados.divida.em_cobranca) }}</dd>
            <dt>Regularizada</dt>
            <dd>{{ formatMoney(dados.divida.regular) }} <span class="muted">(parcelada, garantida ou suspensa)</span></dd>
            <dt>Tipos</dt>
            <dd>{{ dados.divida.tipos.map((t) => tipoDividaLabels[t]).join(', ') || '—' }}<span v-if="dados.divida.ajuizada"> · ajuizada</span></dd>
            <dt>Mais antiga</dt>
            <dd>{{ formatDate(dados.divida.mais_antiga) }}</dd>
          </dl>
          <p class="muted ref">Referência {{ dados.divida.referencia }}. Só a empresa como devedora principal.</p>
        </template>
        <p v-else class="muted">Sem dívida ativa na referência da base.</p>
      </section>

      <section v-if="dados">
        <h4>Cadastro</h4>
        <dl class="campos">
          <dt>Razão social</dt>
          <dd>{{ dados.razao_social }}</dd>
          <template v-if="dados.nome_fantasia">
            <dt>Nome fantasia</dt>
            <dd>{{ dados.nome_fantasia }}</dd>
          </template>
          <dt>Atividade</dt>
          <dd>{{ formatarCnae(dados.cnae.codigo) }} {{ dados.cnae.descricao }}</dd>
          <template v-if="dados.cnaes_secundarios?.length">
            <dt>Secundárias</dt>
            <dd>
              <span v-for="c in dados.cnaes_secundarios.slice(0, 8)" :key="c.codigo" class="secundario">{{ formatarCnae(c.codigo) }} {{ c.descricao }}</span>
              <span v-if="dados.cnaes_secundarios.length > 8" class="muted">e mais {{ dados.cnaes_secundarios.length - 8 }}</span>
            </dd>
          </template>
          <dt>Natureza</dt>
          <dd>{{ dados.natureza.descricao }}</dd>
          <dt>Porte</dt>
          <dd>{{ porteLabels[dados.porte] }}<span v-if="dados.capital"> · capital {{ formatMoney(dados.capital) }}</span></dd>
          <dt>Abertura</dt>
          <dd>{{ formatDate(dados.abertura) }} · {{ dados.matriz ? 'matriz' : 'filial' }}</dd>
          <dt>Regime</dt>
          <dd>{{ dados.mei ? 'MEI' : dados.simples ? 'Simples Nacional' : 'Fora do Simples' }}</dd>
          <dt>Endereço</dt>
          <dd>{{ endereco }}</dd>
        </dl>
        <p class="muted ref">Receita Federal, referência {{ dados.referencia }}.</p>
      </section>

      <section>
        <h4>Enriquecimento</h4>
        <template v-if="perfil">
          <p>
            {{ item.enriquecimento_provedor || perfil.provedor }}
            <span v-if="perfil.demo" class="badge amber">demonstração</span>
            <span class="muted"> · {{ formatDateTime(perfil.consultado_em || item.enriquecido_em) }}</span>
          </p>
          <dl class="campos">
            <template v-if="perfil.funcionarios_faixa">
              <dt>Funcionários</dt>
              <dd>{{ perfil.funcionarios_faixa }}</dd>
            </template>
            <template v-if="perfil.faturamento_faixa">
              <dt>Faturamento</dt>
              <dd>{{ perfil.faturamento_faixa }}</dd>
            </template>
            <template v-if="perfil.site">
              <dt>Site</dt>
              <dd>{{ perfil.site }}</dd>
            </template>
          </dl>
          <p v-if="perfil.observacao" class="muted">{{ perfil.observacao }}</p>
        </template>
        <p v-else-if="item.estado === 'sem_resultado'" class="muted">O provedor não encontrou contatos para esta empresa.</p>
        <p v-else class="muted">Ainda não enriquecido.</p>
      </section>

      <section v-if="item.nota_motivos?.length">
        <h4>Nota {{ item.nota }}</h4>
        <ul class="lista motivos">
          <li v-for="(m, i) in item.nota_motivos" :key="i">
            <strong :class="m.pontos < 0 ? 'neg' : 'pos'">{{ m.pontos > 0 ? '+' : '' }}{{ m.pontos }}</strong>
            {{ m.texto }}
          </li>
        </ul>
      </section>
    </div>
  </SideDrawer>
</template>

<style scoped>
.titulo h3 {
  margin: 0 0 6px;
}
.selos {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.vazio {
  padding: 24px 0;
}
.sub {
  margin: 0 0 12px;
  color: var(--ci-text-2);
  font-size: 13px;
  line-height: 1.5;
}
.ficha section {
  margin-top: 18px;
}
.ficha h4 {
  margin: 0 0 8px;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--ci-text-3);
}
.lista {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.lista li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 13.5px;
}
.numero {
  font-variant-numeric: tabular-nums;
}
.origem,
.lista .badge,
.selos .badge,
.contato .badge {
  font-size: 11px;
}
.pessoa {
  padding: 8px 0;
  border-bottom: 1px solid var(--ci-bg);
  font-size: 13.5px;
}
.pessoa-contatos {
  margin-top: 4px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 13px;
}
.doc {
  font-size: 12px;
}
.campos {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 6px 12px;
  margin: 0;
  font-size: 13.5px;
}
.campos dt {
  color: var(--ci-text-3);
}
.campos dd {
  margin: 0;
}
.secundario {
  display: block;
}
.ref {
  font-size: 12px;
  margin: 8px 0 0;
}
.enviado {
  font-size: 13px;
}
.motivos .pos {
  color: var(--ci-green);
  min-width: 32px;
}
.motivos .neg {
  color: var(--ci-red);
  min-width: 32px;
}
</style>
