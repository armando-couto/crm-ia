<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { api, ApiError } from '../../api'
import { useAuthStore } from '../../stores/auth'
import { formatDateTime, resultadoLabels } from '../../format'
import { formatarTelefone, normalizarTelefone, telefonePrincipal } from '../../telefone'
import {
  ESTADOS_LEAD_ABERTOS,
  RESULTADOS,
  temperaturaLabels,
  type ConfigProspeccao,
  type LeadCompleto,
  type Resultado,
  type ResultadoRequest,
  type ResultadoResposta,
  type Temperatura
} from '../../types-prospeccao'
import MeetingSlotPicker from './MeetingSlotPicker.vue'
import { atalhosRetorno, campoParaIso, novaChave } from './util'

/**
 * Botões de resultado da ligação e o mínimo que cada um pede a mais: data do
 * retorno, motivo, novo decisor, número errado ou a reunião. Uma chamada a
 * `POST /leads/{id}/resultado` grava tudo. Usado pelo cockpit da fila (botões
 * grandes, com atalho) e pelo painel de /leads.
 */
const props = withDefaults(
  defineProps<{
    completo: LeadCompleto
    config: ConfigProspeccao
    /** O que a tela sabe da ligação: último número discado, cronômetro e notas. */
    contexto?: { numero: string; duracao: number; notas: string }
    /** Botões grandes com a tecla de atalho (cockpit). */
    grande?: boolean
  }>(),
  { contexto: () => ({ numero: '', duracao: 0, notas: '' }), grande: false }
)

const emit = defineEmits<{
  salvo: [resposta: ResultadoResposta, resultado: Resultado]
  /** 423 (lead com outra pessoa) ou 409 `estado` (lead já encerrado). */
  conflito: [erro: ApiError]
}>()

const auth = useAuthStore()

/** Resultados gravados no clique, sem pedir mais nada. */
const DIRETOS: Resultado[] = ['nao_atendeu', 'caixa_postal', 'ocupado']

const tons: Record<Resultado, string> = {
  atendeu: 'verde',
  reuniao: 'verde',
  retornar: 'azul',
  sem_interesse: 'cinza',
  nao_e_decisor: 'azul',
  nao_atendeu: 'ambar',
  caixa_postal: 'ambar',
  ocupado: 'ambar',
  numero_errado: 'vermelho',
  nao_ligar_mais: 'vermelho'
}

const escolhido = ref<Resultado | null>(null)
const salvando = ref(false)
const erro = ref('')
const chave = ref('')
const caixa = ref<HTMLElement | null>(null)
const seletor = ref<InstanceType<typeof MeetingSlotPicker> | null>(null)

const campos = reactive({
  retornarEm: '',
  temperatura: '' as Temperatura,
  motivo: '',
  outroMotivo: '',
  contato: { nome: '', cargo: '', telefone: '', email: '' },
  numeroErrado: '',
  reuniao: { titulo: '', inicio: '', userId: null as number | null, duracao: 30, local: '', notas: '', valor: '' }
})
/** Horário recusado por 409 `agenda_ocupada`: libera "Marcar mesmo assim". */
const inicioOcupado = ref('')

const lead = computed(() => props.completo.lead)
const aberto = computed(() => ESTADOS_LEAD_ABERTOS.includes(lead.value.estado))
const telefonesValidos = computed(() => (lead.value.telefones ?? []).filter((t) => !t.invalido))
const atalhos = computed(() => atalhosRetorno(new Date(), props.config.janela_inicio))
const motivos = computed(() => props.config.motivos_descarte ?? [])

const titulos: Partial<Record<Resultado, string>> = {
  atendeu: 'Atendeu: quando falar de novo?',
  retornar: 'Retornar: quando?',
  sem_interesse: 'Sem interesse: qual o motivo?',
  nao_e_decisor: 'Quem é o decisor?',
  numero_errado: 'Qual número está errado?',
  nao_ligar_mais: 'Não ligar mais para esta empresa?',
  reuniao: 'Marcar a reunião'
}

function limpar() {
  campos.retornarEm = ''
  campos.temperatura = lead.value.temperatura ?? ''
  campos.motivo = ''
  campos.outroMotivo = ''
  campos.contato = { nome: '', cargo: '', telefone: '', email: '' }
  campos.numeroErrado = ''
  campos.reuniao = {
    titulo: `Reunião com ${lead.value.titulo}`,
    inicio: '',
    userId: lead.value.owner_id ?? auth.user?.id ?? null,
    duracao: 30,
    local: '',
    notas: '',
    valor: lead.value.valor_estimado != null ? String(lead.value.valor_estimado) : ''
  }
  inicioOcupado.value = ''
  erro.value = ''
}

/** Número da ligação: o último discado; sem clique em "ligar", o principal. */
function numeroDaLigacao(): string {
  return props.contexto.numero || telefonePrincipal(lead.value.telefones)?.numero || ''
}

function escolher(resultado: Resultado) {
  if (salvando.value || !aberto.value) return
  escolhido.value = resultado
  chave.value = novaChave()
  limpar()
  if (resultado === 'numero_errado') {
    const discado = props.contexto.numero
    campos.numeroErrado =
      telefonesValidos.value.find((t) => t.numero === discado)?.numero ??
      (telefonesValidos.value.length === 1 ? telefonesValidos.value[0].numero : '')
  }
  if (DIRETOS.includes(resultado)) {
    void enviar()
    return
  }
  nextTick(() => {
    caixa.value?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })
    const foco = caixa.value?.querySelector<HTMLElement>('[data-foco]')
    foco?.focus?.()
  })
}

function cancelar() {
  if (salvando.value) return
  escolhido.value = null
  erro.value = ''
  inicioOcupado.value = ''
}

function montar(forcar: boolean): ResultadoRequest | null {
  const resultado = escolhido.value
  if (!resultado) return null
  const notas = props.contexto.notas.trim()
  const corpo: ResultadoRequest = {
    resultado,
    chave: chave.value,
    duracao_segundos: Math.max(0, Math.round(props.contexto.duracao || 0))
  }
  const numero = numeroDaLigacao()
  if (numero) corpo.numero = numero
  if (notas) corpo.notas = notas

  if (resultado === 'atendeu' || resultado === 'retornar') {
    const iso = campoParaIso(campos.retornarEm)
    if (resultado === 'retornar' && !iso) {
      erro.value = 'Informe a data e a hora do retorno.'
      return null
    }
    if (iso && new Date(iso).getTime() <= Date.now()) {
      erro.value = 'O retorno precisa ser no futuro.'
      return null
    }
    if (iso) corpo.retornar_em = iso
    if (resultado === 'atendeu' && campos.temperatura !== (lead.value.temperatura ?? '')) corpo.temperatura = campos.temperatura
  }

  if (resultado === 'sem_interesse') {
    const motivo = (campos.motivo === '__outro' ? campos.outroMotivo : campos.motivo).trim()
    if (!motivo) {
      erro.value = 'Escolha o motivo.'
      return null
    }
    corpo.motivo = motivo
  }

  if (resultado === 'nao_e_decisor') {
    const c = campos.contato
    const temAlgo = c.cargo.trim() || c.telefone.trim() || c.email.trim()
    if (!c.nome.trim() && temAlgo) {
      erro.value = 'Informe o nome do decisor.'
      return null
    }
    if (c.nome.trim()) {
      corpo.contato = { nome: c.nome.trim() }
      if (c.cargo.trim()) corpo.contato.cargo = c.cargo.trim()
      if (c.telefone.trim()) corpo.contato.telefone = normalizarTelefone(c.telefone)
      if (c.email.trim()) corpo.contato.email = c.email.trim()
    }
  }

  if (resultado === 'numero_errado') {
    if (!campos.numeroErrado) {
      erro.value = 'Escolha o número errado.'
      return null
    }
    corpo.numero = campos.numeroErrado
  }

  if (resultado === 'reuniao') {
    const r = campos.reuniao
    const inicio = forcar ? inicioOcupado.value || r.inicio : r.inicio
    if (!inicio) {
      erro.value = 'Escolha o dia e o horário da reunião.'
      return null
    }
    corpo.reuniao = { inicio, duracao_min: r.duracao }
    if (r.titulo.trim()) corpo.reuniao.titulo = r.titulo.trim()
    if (r.userId) corpo.reuniao.user_id = r.userId
    if (r.local.trim()) corpo.reuniao.local = r.local.trim()
    if (r.notas.trim()) corpo.reuniao.notas = r.notas.trim()
    const valor = Number(String(r.valor).replace(',', '.'))
    if (r.valor !== '' && Number.isFinite(valor) && valor >= 0) corpo.reuniao.valor = valor
    if (forcar) corpo.reuniao.forcar = true
  }
  return corpo
}

async function enviar(forcar = false) {
  if (salvando.value || !escolhido.value) return
  erro.value = ''
  const corpo = montar(forcar)
  if (!corpo) return
  const resultado = escolhido.value
  salvando.value = true
  try {
    const resposta = await api.post<ResultadoResposta>(`/leads/${lead.value.id}/resultado`, corpo)
    escolhido.value = null
    inicioOcupado.value = ''
    emit('salvo', resposta, resultado)
  } catch (e: any) {
    if (e instanceof ApiError && e.status === 409 && e.codigo === 'agenda_ocupada') {
      // Mantém o formulário, recarrega a grade e oferece "Marcar mesmo assim".
      inicioOcupado.value = corpo.reuniao?.inicio ?? ''
      erro.value = e.message || 'este horário já está ocupado'
      void seletor.value?.recarregar()
    } else if (e instanceof ApiError && (e.status === 423 || (e.status === 409 && e.codigo === 'estado'))) {
      escolhido.value = null
      emit('conflito', e)
    } else {
      erro.value = e?.message || 'não foi possível salvar o resultado'
      // Recusa definitiva: a próxima tentativa é outro pedido.
      if (e instanceof ApiError && e.status >= 400 && e.status < 500) chave.value = novaChave()
      if (DIRETOS.includes(resultado)) nextTick(() => caixa.value?.scrollIntoView?.({ block: 'nearest' }))
    }
  } finally {
    salvando.value = false
  }
}

function escolherHorario(valor: string) {
  campos.reuniao.inicio = valor
  if (valor) {
    inicioOcupado.value = ''
    erro.value = ''
  }
}

// Outro lead na tela: nada do anterior pode sobrar.
watch(
  () => lead.value.id,
  () => {
    escolhido.value = null
    salvando.value = false
    limpar()
  }
)

defineExpose({ escolher, cancelar, escolhido, salvando })
</script>

<template>
  <div class="resultado" :class="{ grande }">
    <p v-if="!aberto" class="muted encerrado">Este lead já foi encerrado e não recebe novos resultados.</p>

    <div v-else class="botoes" role="group" aria-label="Resultado da ligação">
      <button
        v-for="r in RESULTADOS"
        :key="r.resultado"
        type="button"
        class="resultado-btn"
        :class="[tons[r.resultado], { active: escolhido === r.resultado }]"
        :data-resultado="r.resultado"
        :disabled="salvando"
        :aria-pressed="escolhido === r.resultado"
        @click="escolher(r.resultado)"
      >
        <span v-if="grande" class="tecla">{{ r.tecla }}</span>
        <span class="nome">{{ resultadoLabels[r.resultado] }}</span>
      </button>
    </div>

    <form
      v-if="escolhido && (!DIRETOS.includes(escolhido) || erro)"
      ref="caixa"
      class="complemento"
      :data-complemento="escolhido"
      @submit.prevent="enviar()"
    >
      <template v-if="!DIRETOS.includes(escolhido)">
        <h4>{{ titulos[escolhido] }}</h4>

        <!-- Atendeu / Retornar: data do próximo contato -->
        <template v-if="escolhido === 'atendeu' || escolhido === 'retornar'">
          <div class="atalhos">
            <button
              v-for="a in atalhos"
              :key="a.rotulo"
              type="button"
              class="chip"
              :class="{ active: campos.retornarEm === a.valor }"
              @click="campos.retornarEm = a.valor"
            >
              {{ a.rotulo }}
            </button>
          </div>
          <div class="field">
            <label for="resultado-retorno">
              Data e hora do retorno{{ escolhido === 'atendeu' ? ' (opcional)' : '' }}
            </label>
            <input id="resultado-retorno" v-model="campos.retornarEm" type="datetime-local" data-foco />
            <span v-if="escolhido === 'atendeu' && !campos.retornarEm" class="muted dica">
              Sem data, vale o retorno padrão de {{ config.retorno_padrao_horas }} h.
            </span>
          </div>
          <div v-if="escolhido === 'atendeu'" class="field">
            <label for="resultado-temperatura">Temperatura</label>
            <select id="resultado-temperatura" v-model="campos.temperatura">
              <option v-for="(rotulo, valor) in temperaturaLabels" :key="valor" :value="valor">{{ rotulo }}</option>
            </select>
          </div>
        </template>

        <!-- Sem interesse: motivo -->
        <template v-else-if="escolhido === 'sem_interesse'">
          <div class="atalhos motivos">
            <button
              v-for="m in motivos"
              :key="m"
              type="button"
              class="chip"
              :class="{ active: campos.motivo === m }"
              @click="campos.motivo = m"
            >
              {{ m }}
            </button>
            <button type="button" class="chip" :class="{ active: campos.motivo === '__outro' }" @click="campos.motivo = '__outro'">
              Outro
            </button>
          </div>
          <div v-if="campos.motivo === '__outro'" class="field">
            <label for="resultado-motivo">Motivo</label>
            <input id="resultado-motivo" v-model="campos.outroMotivo" type="text" maxlength="120" />
          </div>
        </template>

        <!-- Não é o decisor: quem é -->
        <template v-else-if="escolhido === 'nao_e_decisor'">
          <p class="muted dica">Opcional: se descobriu quem decide, o telefone dele passa para o topo.</p>
          <div class="form-row">
            <div class="field">
              <label for="decisor-nome">Nome</label>
              <input id="decisor-nome" v-model="campos.contato.nome" type="text" data-foco />
            </div>
            <div class="field">
              <label for="decisor-cargo">Cargo</label>
              <input id="decisor-cargo" v-model="campos.contato.cargo" type="text" />
            </div>
            <div class="field">
              <label for="decisor-telefone">Telefone</label>
              <input id="decisor-telefone" v-model="campos.contato.telefone" type="tel" inputmode="tel" placeholder="(85) 99999-0001" />
            </div>
            <div class="field">
              <label for="decisor-email">E-mail</label>
              <input id="decisor-email" v-model="campos.contato.email" type="email" />
            </div>
          </div>
        </template>

        <!-- Número errado: qual -->
        <template v-else-if="escolhido === 'numero_errado'">
          <p v-if="!telefonesValidos.length" class="muted">Este lead não tem telefone válido.</p>
          <label v-for="t in telefonesValidos" :key="t.numero" class="opcao">
            <input v-model="campos.numeroErrado" type="radio" name="numero-errado" :value="t.numero" />
            <span>{{ formatarTelefone(t.numero) }}</span>
            <span v-if="t.pessoa" class="muted">{{ t.pessoa }}</span>
          </label>
          <p class="muted dica">O número é marcado como inválido e não volta a ser discado.</p>
        </template>

        <!-- Não ligar mais: confirmação -->
        <template v-else-if="escolhido === 'nao_ligar_mais'">
          <p class="dica">
            O lead é descartado, o CNPJ e os telefones entram na lista de bloqueios e a empresa fica marcada como
            "não perturbe".
          </p>
        </template>

        <!-- Reunião -->
        <template v-else-if="escolhido === 'reuniao'">
          <MeetingSlotPicker
            ref="seletor"
            :inicio="campos.reuniao.inicio"
            :user-id="campos.reuniao.userId"
            @update:inicio="escolherHorario"
            @update:user-id="campos.reuniao.userId = $event"
            @duracao="campos.reuniao.duracao = $event"
          />
          <p v-if="campos.reuniao.inicio" class="escolha">Reunião em <strong>{{ formatDateTime(campos.reuniao.inicio) }}</strong></p>
          <div class="field">
            <label for="reuniao-titulo">Título</label>
            <input id="reuniao-titulo" v-model="campos.reuniao.titulo" type="text" />
          </div>
          <div class="form-row">
            <div class="field">
              <label for="reuniao-local">Local ou link</label>
              <input id="reuniao-local" v-model="campos.reuniao.local" type="text" />
            </div>
            <div class="field">
              <label for="reuniao-valor">Valor estimado (R$)</label>
              <input id="reuniao-valor" v-model="campos.reuniao.valor" type="number" min="0" step="0.01" inputmode="decimal" />
            </div>
          </div>
        </template>
      </template>

      <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>

      <div class="acoes">
        <button type="button" class="btn btn-outline" :disabled="salvando" @click="cancelar">Cancelar</button>
        <button
          v-if="escolhido === 'reuniao' && inicioOcupado"
          type="button"
          class="btn btn-outline forcar"
          :disabled="salvando"
          @click="enviar(true)"
        >
          Marcar mesmo assim
        </button>
        <button
          type="submit"
          class="btn"
          :class="escolhido === 'nao_ligar_mais' ? 'btn-danger btn-outline' : 'btn-primary'"
          :disabled="salvando"
          :data-foco="escolhido === 'nao_ligar_mais' || escolhido === 'sem_interesse' ? '' : undefined"
        >
          {{ salvando ? 'Salvando…' : DIRETOS.includes(escolhido) ? 'Tentar de novo' : escolhido === 'reuniao' ? 'Marcar reunião' : 'Salvar resultado' }}
        </button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.botoes {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 8px;
}

.resultado-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  padding: 8px 12px;
  border: 1px solid var(--ci-border);
  border-left-width: 4px;
  border-radius: 10px;
  background: var(--ci-surface);
  color: var(--ci-text);
  font-size: 14px;
  font-weight: 500;
  font-family: inherit;
  text-align: left;
  cursor: pointer;
  transition: background 0.12s, box-shadow 0.12s;
}

.resultado-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.resultado-btn.verde {
  border-left-color: var(--ci-green);
}

.resultado-btn.azul {
  border-left-color: var(--ci-blue);
}

.resultado-btn.ambar {
  border-left-color: var(--ci-amber);
}

.resultado-btn.vermelho {
  border-left-color: var(--ci-red);
}

.resultado-btn.cinza {
  border-left-color: var(--ci-text-3);
}

.resultado-btn:hover:not(:disabled),
.resultado-btn.active {
  background: var(--ci-purple-tint);
  box-shadow: 0 0 0 2px rgba(var(--ci-purple-rgb), 0.25);
}

.grande .botoes {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.grande .resultado-btn {
  min-height: 54px;
  font-size: 15px;
  font-weight: 600;
}

.complemento {
  margin-top: 12px;
  padding: 14px;
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  background: var(--ci-bg);
}

.complemento h4 {
  margin: 0 0 10px;
  font-size: 15px;
}

.atalhos {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}

.atalhos .chip {
  min-height: 34px;
  font-family: inherit;
}

.dica {
  font-size: 13px;
  margin: 0 0 10px;
}

.opcao {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
  font-size: 15px;
  cursor: pointer;
}

.escolha {
  margin: 12px 0;
  font-size: 14px;
}

.acoes {
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 6px;
}

.encerrado {
  margin: 0;
  font-size: 13px;
}

@media (max-width: 720px) {
  .complemento .form-row {
    grid-template-columns: 1fr;
  }

  .acoes .btn {
    flex: 1 1 auto;
    justify-content: center;
    min-height: 46px;
  }

  .atalhos .chip {
    min-height: 40px;
  }
}
</style>
