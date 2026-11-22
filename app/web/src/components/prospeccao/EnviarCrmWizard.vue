<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, ApiError } from '../../api'
import { formatNumber } from '../../format'
import { useAuthStore } from '../../stores/auth'
import type { User } from '../../types'
import {
  duplicadosLabels,
  modoDistribuicaoLabels,
  temperaturaLabels,
  type EnviarPrevia,
  type EnviarRequest,
  type Job,
  type JobResposta,
  type ModoDistribuicao,
  type Selecao,
  type Temperatura
} from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'
import StepperSteps from '../ui/StepperSteps.vue'
import ChipsTexto from './ChipsTexto.vue'

/**
 * Enviar ao CRM em três passos: (1) o que criar e como tratar duplicados,
 * (2) para quem vão os leads, com a prévia "≈ N por pessoa" e a carga diária
 * opcional, (3) o resumo calculado pelo servidor e a confirmação.
 */
const props = defineProps<{ open: boolean; listaId: number; selecao: Selecao; quantidade: number }>()
const emit = defineEmits<{ close: []; iniciado: [job: Job]; jobAtivo: [job: Job] }>()

const PASSOS = ['O que criar', 'Distribuição', 'Confirmar']
const auth = useAuthStore()

const passo = ref(0)
const duplicados = ref<EnviarRequest['duplicados']>('pular')
const criarContatos = ref(true)
const temperatura = ref<Temperatura>('')
const etiquetas = ref<string[]>([])
const tier = ref<0 | 1 | 2 | 3>(0)
const modo = ref<ModoDistribuicao>('unico')
const unico = ref<number | null>(null)
const rodizio = ref<number[]>([])
const usarPorDia = ref(false)
const porDia = ref<number>(20)

const usuarios = ref<User[]>([])
const carregandoUsuarios = ref(false)
const previa = ref<EnviarPrevia | null>(null)
const carregandoPrevia = ref(false)
const enviando = ref(false)
const erro = ref('')

async function carregarUsuarios() {
  if (usuarios.value.length) return
  carregandoUsuarios.value = true
  try {
    usuarios.value = ((await api.get<User[]>('/users')) ?? []).filter((u) => u.active !== false)
  } catch (e: any) {
    erro.value = e?.message || 'não foi possível carregar os usuários'
  } finally {
    carregandoUsuarios.value = false
  }
  if (unico.value === null) {
    const eu = usuarios.value.find((u) => u.id === auth.user?.id)
    unico.value = eu?.id ?? usuarios.value[0]?.id ?? null
  }
  if (!rodizio.value.length) rodizio.value = usuarios.value.filter((u) => u.role === 'seller').map((u) => u.id)
}

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    passo.value = 0
    previa.value = null
    erro.value = ''
    void carregarUsuarios()
  },
  { immediate: true }
)

const escolhidos = computed<number[]>(() => (modo.value === 'unico' ? (unico.value ? [unico.value] : []) : rodizio.value))
const porPessoa = computed(() => (escolhidos.value.length ? Math.ceil(props.quantidade / escolhidos.value.length) : 0))
const porDiaValido = computed(() => !usarPorDia.value || (Number.isInteger(porDia.value) && porDia.value >= 1 && porDia.value <= 500))
const diasUteis = computed(() => (usarPorDia.value && porDia.value > 0 ? Math.ceil(porPessoa.value / porDia.value) : 0))
const passo2Ok = computed(() => escolhidos.value.length > 0 && porDiaValido.value)

function nomeDe(id: number): string {
  return usuarios.value.find((u) => u.id === id)?.name ?? `Usuário ${id}`
}

function alternarRodizio(id: number) {
  rodizio.value = rodizio.value.includes(id) ? rodizio.value.filter((x) => x !== id) : [...rodizio.value, id]
}

function corpo(): EnviarRequest {
  const req: EnviarRequest = {
    selecao: props.selecao,
    duplicados: duplicados.value,
    criar_contatos: criarContatos.value,
    distribuicao: { modo: modo.value, usuarios: escolhidos.value }
  }
  if (usarPorDia.value) req.distribuicao.por_dia = porDia.value
  if (temperatura.value) req.temperatura = temperatura.value
  if (etiquetas.value.length) req.etiquetas = [...etiquetas.value]
  if (tier.value) req.conta_alvo_tier = tier.value
  return req
}

async function carregarPrevia() {
  carregandoPrevia.value = true
  erro.value = ''
  previa.value = null
  try {
    previa.value = await api.post<EnviarPrevia>(`/prospeccao/listas/${props.listaId}/enviar/previa`, corpo())
  } catch (e: any) {
    if (e instanceof ApiError && e.codigo === 'job_ativo' && e.data?.job) emit('jobAtivo', e.data.job)
    else erro.value = e?.message || 'não foi possível calcular a prévia'
  } finally {
    carregandoPrevia.value = false
  }
}

function avancar() {
  if (passo.value === 0) passo.value = 1
  else if (passo.value === 1 && passo2Ok.value) {
    passo.value = 2
    void carregarPrevia()
  }
}

function irPara(i: number) {
  if (i < passo.value) passo.value = i
}

async function enviar() {
  if (!previa.value || previa.value.aptos <= 0 || enviando.value) return
  enviando.value = true
  erro.value = ''
  try {
    const r = await api.post<JobResposta>(`/prospeccao/listas/${props.listaId}/enviar`, corpo())
    emit('iniciado', r.job)
  } catch (e: any) {
    if (e instanceof ApiError && e.codigo === 'job_ativo' && e.data?.job) emit('jobAtivo', e.data.job)
    else erro.value = e?.message || 'não foi possível enviar ao CRM'
  } finally {
    enviando.value = false
  }
}
</script>

<template>
  <ModalDialog title="Enviar ao CRM" :open="open" wide @close="emit('close')">
    <StepperSteps :passos="PASSOS" :atual="passo" @update:atual="irPara" />

    <div class="passo">
      <!-- 1. O que criar -->
      <template v-if="passo === 0">
        <p class="intro">
          Cada empresa vira um <strong>lead</strong> na caixa do vendedor (não um negócio: o negócio nasce quando a reunião é marcada).
          {{ formatNumber(quantidade) }} empresa{{ quantidade === 1 ? '' : 's' }} selecionada{{ quantidade === 1 ? '' : 's' }}.
        </p>
        <fieldset class="grupo">
          <legend>Empresa que já está no CRM</legend>
          <label v-for="(r, v) in duplicadosLabels" :key="v" class="check">
            <input v-model="duplicados" type="radio" :value="v" /> {{ r }}
          </label>
        </fieldset>
        <label class="check">
          <input v-model="criarContatos" type="checkbox" />
          Criar contatos dos decisores e sócios-administradores (até 3 por empresa)
        </label>
        <div class="form-row">
          <div class="field">
            <label for="ec-temp">Temperatura</label>
            <select id="ec-temp" v-model="temperatura">
              <option v-for="(r, v) in temperaturaLabels" :key="v" :value="v">{{ r }}</option>
            </select>
          </div>
          <div class="field">
            <label for="ec-tier">Conta-alvo</label>
            <select id="ec-tier" v-model.number="tier">
              <option :value="0">Não marcar</option>
              <option :value="1">Nível 1</option>
              <option :value="2">Nível 2</option>
              <option :value="3">Nível 3</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Etiquetas</label>
          <ChipsTexto v-model="etiquetas" nome="Etiquetas" placeholder="Ex.: feira-novembro (Enter para acrescentar)" :maximo="10" />
        </div>
      </template>

      <!-- 2. Distribuição -->
      <template v-else-if="passo === 1">
        <fieldset class="grupo">
          <legend>Para quem vão os leads</legend>
          <label v-for="(r, v) in modoDistribuicaoLabels" :key="v" class="check">
            <input v-model="modo" type="radio" :value="v" /> {{ r }}
          </label>
        </fieldset>

        <div v-if="carregandoUsuarios" class="muted">Carregando a equipe…</div>
        <div v-else-if="modo === 'unico'" class="field">
          <label for="ec-dono">Dono</label>
          <select id="ec-dono" v-model.number="unico">
            <option v-for="u in usuarios" :key="u.id" :value="u.id">{{ u.name }}</option>
          </select>
        </div>
        <div v-else class="usuarios" role="group" aria-label="Pessoas do rodízio">
          <label v-for="u in usuarios" :key="u.id" class="check">
            <input type="checkbox" :checked="rodizio.includes(u.id)" @change="alternarRodizio(u.id)" />
            {{ u.name }}
          </label>
          <p class="muted pequeno">O rodízio segue esta ordem e pula quem estiver inativo.</p>
        </div>

        <label class="check">
          <input v-model="usarPorDia" type="checkbox" />
          Distribuir aos poucos: a próxima ação de cada lead cai em dias úteis
        </label>
        <div v-if="usarPorDia" class="field por-dia">
          <label for="ec-por-dia">Leads por pessoa por dia</label>
          <input id="ec-por-dia" v-model.number="porDia" type="number" min="1" max="500" step="1" />
          <span v-if="!porDiaValido" class="error-text">Entre 1 e 500.</span>
        </div>

        <div v-if="escolhidos.length" class="estimativa">
          ≈ <strong>{{ formatNumber(porPessoa) }}</strong> lead{{ porPessoa === 1 ? '' : 's' }} por pessoa
          <template v-if="escolhidos.length > 1">entre {{ escolhidos.length }} pessoas</template>
          <template v-if="usarPorDia && diasUteis">, em cerca de {{ diasUteis }} dia{{ diasUteis === 1 ? '' : 's' }} úte{{ diasUteis === 1 ? 'il' : 'is' }}</template>
          <span class="muted"> (antes de tirar duplicados e bloqueados)</span>
        </div>
        <p v-else class="error-text">Escolha ao menos uma pessoa.</p>
      </template>

      <!-- 3. Confirmar -->
      <template v-else>
        <div v-if="carregandoPrevia" class="muted vazio">Conferindo duplicados e bloqueios…</div>
        <template v-else-if="previa">
          <div class="aptos">
            <strong>{{ formatNumber(previa.aptos) }}</strong> lead{{ previa.aptos === 1 ? '' : 's' }} ser{{ previa.aptos === 1 ? 'á criado' : 'ão criados' }}
            de {{ formatNumber(previa.selecionados) }} selecionada{{ previa.selecionados === 1 ? '' : 's' }}
          </div>
          <ul class="resumo">
            <li v-if="previa.duplicados_no_crm">
              {{ formatNumber(previa.duplicados_no_crm) }} já estão no CRM
              <span class="muted">{{ duplicados === 'pular' ? '(ficam de fora)' : '(o lead fica ligado à empresa existente)' }}</span>
            </li>
            <li v-if="previa.bloqueados">{{ formatNumber(previa.bloqueados) }} bloqueadas na lista de não contatar <span class="muted">(ficam de fora)</span></li>
            <li v-if="previa.ja_enviados">{{ formatNumber(previa.ja_enviados) }} já enviadas antes <span class="muted">(ficam de fora)</span></li>
            <li v-if="previa.descartados">{{ formatNumber(previa.descartados) }} descartadas <span class="muted">(ficam de fora)</span></li>
            <li v-if="previa.sem_telefone" class="atencao">{{ formatNumber(previa.sem_telefone) }} sem telefone</li>
            <li v-if="previa.pessoa_fisica" class="atencao">
              {{ formatNumber(previa.pessoa_fisica) }} pessoa{{ previa.pessoa_fisica === 1 ? '' : 's' }} física{{ previa.pessoa_fisica === 1 ? '' : 's' }} (MEI ou empresário individual): a LGPD se aplica ao contato
            </li>
          </ul>
          <table v-if="previa.por_usuario?.length" class="data por-usuario">
            <thead>
              <tr>
                <th>Vendedor</th>
                <th class="num">Leads</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="u in previa.por_usuario" :key="u.user_id">
                <td>{{ u.nome || nomeDe(u.user_id) }}</td>
                <td class="num">{{ formatNumber(u.quantidade) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="previa.aptos <= 0" class="faixa amber">Nenhuma empresa apta: nada a enviar com estas opções.</p>
          <p class="muted pequeno">
            Não dispara automações de "contato criado" nem e-mails. O envio roda no servidor; você pode fechar esta tela.
          </p>
        </template>
      </template>
    </div>

    <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>

    <template #footer>
      <button v-if="passo > 0" type="button" class="btn btn-outline" :disabled="enviando" @click="passo--">Voltar</button>
      <button v-else type="button" class="btn btn-outline" @click="emit('close')">Cancelar</button>
      <button v-if="passo < 2" type="button" class="btn btn-primary" :disabled="passo === 1 && !passo2Ok" @click="avancar">Continuar</button>
      <button v-else type="button" class="btn btn-primary" :disabled="!previa || previa.aptos <= 0 || enviando || carregandoPrevia" @click="enviar">
        {{ enviando ? 'Enviando…' : previa ? `Enviar ${formatNumber(previa.aptos)} ao CRM` : 'Enviar' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.passo {
  margin-top: 16px;
  min-height: 200px;
}
.intro {
  margin: 0 0 12px;
  font-size: 13.5px;
  color: var(--ci-text-2);
}
.grupo {
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  padding: 8px 14px 10px;
  margin: 0 0 12px;
}
.grupo legend {
  font-size: 13px;
  font-weight: 500;
  color: var(--ci-text-2);
  padding: 0 6px;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  cursor: pointer;
  font-size: 13.5px;
}
.usuarios {
  max-height: 220px;
  overflow-y: auto;
  margin-bottom: 8px;
}
.por-dia {
  max-width: 220px;
}
.estimativa {
  margin-top: 12px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
}
.vazio {
  padding: 24px 0;
}
.aptos {
  font-size: 15px;
  margin-bottom: 8px;
}
.aptos strong {
  font-size: 20px;
}
.resumo {
  margin: 0 0 12px;
  padding-left: 18px;
  font-size: 13.5px;
  line-height: 1.7;
}
.atencao {
  color: var(--ci-amber, #a86b00);
}
.por-usuario {
  margin-bottom: 10px;
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.pequeno {
  font-size: 12.5px;
}
</style>
