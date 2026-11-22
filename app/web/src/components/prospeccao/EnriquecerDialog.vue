<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, ApiError } from '../../api'
import { formatCentavos, formatNumber } from '../../format'
import type { EnriquecerEstimativa, EnriquecerRequest, Job, JobResposta, Selecao } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'

/**
 * Enriquecer em lote: mostra antes o custo estimado, o saldo do provedor e o
 * uso do mês, e só envia com a confirmação do valor. Se o custo mudar entre a
 * estimativa e a confirmação (409 `custo_mudou`), mostra o novo e pede outra
 * confirmação.
 */
const props = defineProps<{ open: boolean; listaId: number; selecao: Selecao }>()
const emit = defineEmits<{ close: []; iniciado: [job: Job]; jobAtivo: [job: Job] }>()

const estimativa = ref<EnriquecerEstimativa | null>(null)
const carregando = ref(false)
const enviando = ref(false)
const reprocessar = ref(false)
const erro = ref('')
const aviso = ref('')
const confirmado = ref(false)

async function estimar() {
  carregando.value = true
  erro.value = ''
  try {
    estimativa.value = await api.post<EnriquecerEstimativa>(`/prospeccao/listas/${props.listaId}/enriquecer/estimativa`, {
      selecao: props.selecao,
      reprocessar: reprocessar.value
    })
  } catch (e: any) {
    estimativa.value = null
    erro.value = e?.message || 'não foi possível calcular o custo'
  } finally {
    carregando.value = false
  }
}

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    reprocessar.value = false
    confirmado.value = false
    aviso.value = ''
    estimativa.value = null
    void estimar()
  },
  { immediate: true }
)

watch(reprocessar, () => {
  if (!props.open) return
  confirmado.value = false
  void estimar()
})

const provedor = computed(() => estimativa.value?.provedor ?? null)
const usoDepois = computed(() => (estimativa.value ? estimativa.value.uso_mes.custo_centavos + estimativa.value.custo_estimado_centavos : 0))
const estouraLimite = computed(() => {
  const e = estimativa.value
  return !!e && e.uso_mes.limite_centavos > 0 && usoDepois.value > e.uso_mes.limite_centavos
})
const semFranquia = computed(() => {
  const s = estimativa.value?.saldo
  return !!s && s.franquia_restante !== null && s.franquia_restante < (estimativa.value?.a_enriquecer ?? 0)
})
const temCusto = computed(() => (estimativa.value?.custo_estimado_centavos ?? 0) > 0)
const podeEnviar = computed(() => {
  const e = estimativa.value
  if (!e || carregando.value || enviando.value) return false
  if (!e.provedor.pronto || e.a_enriquecer <= 0 || estouraLimite.value) return false
  return !temCusto.value || confirmado.value
})

async function enviar() {
  const e = estimativa.value
  if (!e || !podeEnviar.value) return
  enviando.value = true
  erro.value = ''
  aviso.value = ''
  const corpo: EnriquecerRequest = { selecao: props.selecao, reprocessar: reprocessar.value, custo_confirmado_centavos: e.custo_estimado_centavos }
  try {
    const r = await api.post<JobResposta>(`/prospeccao/listas/${props.listaId}/enriquecer`, corpo)
    emit('iniciado', r.job)
  } catch (err: any) {
    if (err instanceof ApiError && err.codigo === 'custo_mudou') {
      if (err.data?.estimativa) estimativa.value = err.data.estimativa
      else await estimar()
      confirmado.value = false
      aviso.value = 'O custo mudou desde a estimativa. Confira o novo valor e confirme de novo.'
    } else if (err instanceof ApiError && err.codigo === 'job_ativo' && err.data?.job) {
      emit('jobAtivo', err.data.job)
    } else {
      erro.value = err?.message || 'não foi possível iniciar o enriquecimento'
    }
  } finally {
    enviando.value = false
  }
}
</script>

<template>
  <ModalDialog title="Enriquecer contatos" :open="open" @close="emit('close')">
    <div v-if="carregando && !estimativa" class="muted vazio">Calculando o custo…</div>
    <template v-else-if="estimativa">
      <p class="intro">
        Busca telefones (com WhatsApp e "não perturbe"), e-mails, sócios e decisores no provedor
        <strong>{{ provedor?.nome }}</strong>
        <span v-if="provedor?.demo" class="badge amber">demonstração: custo zero, contatos fictícios</span>.
      </p>

      <div v-if="!provedor?.pronto" class="faixa red">
        {{ provedor?.motivo || 'O provedor de enriquecimento não está configurado.' }}
        Configure em Configurações → Integrações de dados.
      </div>

      <dl class="numeros" :class="{ atualizando: carregando }">
        <dt>Selecionadas</dt>
        <dd>{{ formatNumber(estimativa.selecionados) }}</dd>
        <dt>A enriquecer</dt>
        <dd><strong>{{ formatNumber(estimativa.a_enriquecer) }}</strong></dd>
        <template v-if="estimativa.ja_enriquecidos">
          <dt>Já enriquecidas</dt>
          <dd>
            {{ formatNumber(estimativa.ja_enriquecidos) }}
            <span class="muted">{{ reprocessar ? '(serão consultadas de novo)' : '(ficam de fora)' }}</span>
          </dd>
        </template>
        <dt>Custo por empresa</dt>
        <dd>{{ formatCentavos(estimativa.custo_unitario_centavos) }} <span class="muted">(estimativa)</span></dd>
        <dt>Custo estimado</dt>
        <dd class="custo">{{ formatCentavos(estimativa.custo_estimado_centavos) }}</dd>
        <dt>Uso no mês</dt>
        <dd>
          {{ formatCentavos(estimativa.uso_mes.custo_centavos) }}
          <template v-if="estimativa.uso_mes.limite_centavos > 0">
            de {{ formatCentavos(estimativa.uso_mes.limite_centavos) }}
            <span class="muted">· depois deste lote: {{ formatCentavos(usoDepois) }}</span>
          </template>
          <span v-else class="muted">(sem limite mensal)</span>
        </dd>
        <template v-if="estimativa.saldo">
          <dt>Saldo no provedor</dt>
          <dd>
            {{ estimativa.saldo.plano || 'Plano' }}
            <template v-if="estimativa.saldo.franquia_restante !== null">
              · {{ formatNumber(estimativa.saldo.franquia_restante) }}
              <template v-if="estimativa.saldo.franquia_total !== null">de {{ formatNumber(estimativa.saldo.franquia_total) }}</template>
              consultas restantes
            </template>
            <span v-else class="muted">· pré-pago</span>
          </dd>
        </template>
      </dl>

      <label v-if="estimativa.ja_enriquecidos" class="check">
        <input v-model="reprocessar" type="checkbox" />
        Consultar de novo as já enriquecidas (paga outra vez)
      </label>

      <div v-if="estouraLimite" class="faixa red">
        Este lote passa do limite mensal de enriquecimento. Selecione menos empresas ou peça ao gestor para aumentar o limite.
      </div>
      <div v-else-if="semFranquia" class="faixa amber">
        A franquia do provedor não cobre todas as empresas: as que passarem do saldo podem falhar e ficam como estavam.
      </div>
      <p v-if="estimativa.a_enriquecer <= 0" class="muted">Nenhuma empresa da seleção precisa de enriquecimento.</p>

      <label v-if="temCusto && provedor?.pronto && estimativa.a_enriquecer > 0 && !estouraLimite" class="check confirmar">
        <input v-model="confirmado" type="checkbox" />
        Confirmo o custo estimado de <strong>{{ formatCentavos(estimativa.custo_estimado_centavos) }}</strong>, cobrado pelo provedor.
      </label>
      <p v-if="aviso" class="faixa amber" role="alert">{{ aviso }}</p>
    </template>
    <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>

    <template #footer>
      <button type="button" class="btn btn-outline" @click="emit('close')">Cancelar</button>
      <button type="button" class="btn btn-primary" :disabled="!podeEnviar" @click="enviar">
        {{ enviando ? 'Iniciando…' : estimativa && estimativa.a_enriquecer > 0 ? `Enriquecer ${formatNumber(estimativa.a_enriquecer)}` : 'Enriquecer' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.vazio {
  padding: 18px 0;
}
.intro {
  margin: 0 0 12px;
  font-size: 13.5px;
  color: var(--ci-text-2);
}
.intro .badge {
  font-size: 11px;
  margin-left: 4px;
}
.numeros {
  display: grid;
  grid-template-columns: 150px 1fr;
  gap: 6px 12px;
  margin: 12px 0;
  font-size: 13.5px;
}
.numeros.atualizando {
  opacity: 0.55;
}
.numeros dt {
  color: var(--ci-text-3);
}
.numeros dd {
  margin: 0;
}
.custo {
  font-size: 17px;
  font-weight: 700;
}
.check {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 4px 0;
  cursor: pointer;
  font-size: 13.5px;
}
.confirmar {
  margin-top: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--ci-purple-tint);
}
.faixa {
  margin: 10px 0;
}
</style>
