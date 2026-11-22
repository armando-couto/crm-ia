<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, ApiError } from '../../api'
import { formatCompetencia, formatContagem, formatNumber } from '../../format'
import type { Contagem, Cota, CriarLista, CriarListaResposta, Filtros, Ordem } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'
import { tetoQuantidade } from './filtros'

/**
 * Confirmação de "Gerar lista": nome, quantidade (limitada ao total e à cota
 * restante), exclusões e quanto da cota do mês a lista vai usar.
 */
const props = defineProps<{
  open: boolean
  filtros: Filtros
  ordem: Ordem
  nomeSugerido: string
  contagem: Contagem | null
  cota: Cota
  segmentoId?: number | null
}>()

const emit = defineEmits<{
  close: []
  criada: [resposta: CriarListaResposta]
  /** A API devolveu a cota atualizada (429 de cota esgotada). */
  cota: [cota: Cota]
}>()

const nome = ref('')
const descricao = ref('')
const quantidade = ref(0)
const excluirNoCrm = ref(true)
const excluirEmListas = ref(true)
const salvando = ref(false)
const erro = ref('')

const teto = computed(() => tetoQuantidade(props.contagem, props.cota))
const atalhos = computed(() => [100, 500, 1000, 5000].filter((n) => n < teto.value.maximo))

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    nome.value = props.nomeSugerido
    descricao.value = ''
    quantidade.value = Math.min(teto.value.maximo, 500)
    excluirNoCrm.value = true
    excluirEmListas.value = true
    erro.value = ''
  }
)

const quantidadeValida = computed(() => Number.isInteger(quantidade.value) && quantidade.value >= 1 && quantidade.value <= teto.value.maximo)
const podeGerar = computed(() => !salvando.value && nome.value.trim().length > 0 && quantidadeValida.value)

const avisoQuantidade = computed(() => {
  const q = quantidade.value
  if (teto.value.maximo <= 0) return ''
  if (!Number.isInteger(q) || q < 1) return 'Informe quantas empresas a lista deve ter.'
  if (q > teto.value.maximo) return `O máximo agora é ${formatNumber(teto.value.maximo)}.`
  return ''
})

async function gerar() {
  if (!podeGerar.value) return
  salvando.value = true
  erro.value = ''
  const corpo: CriarLista = {
    nome: nome.value.trim(),
    origem: 'base',
    filtros: props.filtros,
    ordem: props.ordem,
    quantidade: quantidade.value,
    excluir_no_crm: excluirNoCrm.value,
    excluir_em_listas: excluirEmListas.value
  }
  if (descricao.value.trim()) corpo.descricao = descricao.value.trim()
  if (props.segmentoId) corpo.segmento_id = props.segmentoId
  try {
    emit('criada', await api.post<CriarListaResposta>('/prospeccao/listas', corpo))
  } catch (e: any) {
    if (e instanceof ApiError && e.data?.cota) emit('cota', e.data.cota)
    erro.value = e?.message || 'não foi possível gerar a lista'
  } finally {
    salvando.value = false
  }
}
</script>

<template>
  <ModalDialog title="Gerar lista" :open="open" @close="emit('close')">
    <form id="form-gerar-lista" @submit.prevent="gerar">
      <p class="resumo">
        <strong>{{ formatContagem(contagem) }}</strong> empresas nos filtros escolhidos. Você decide quantas entram na lista.
      </p>

      <div v-if="teto.maximo <= 0" class="faixa red">
        Sua cota de {{ formatCompetencia(cota.mes) }} acabou ({{ formatNumber(cota.usado) }} de {{ formatNumber(cota.limite) }} leads usados).
        Contar e ver a prévia continuam livres.
      </div>

      <template v-else>
        <div class="field">
          <label for="gl-nome">Nome da lista</label>
          <input id="gl-nome" v-model="nome" type="text" maxlength="120" required placeholder="Ex.: Restaurantes de Fortaleza" />
        </div>

        <div class="field">
          <label for="gl-quantidade">Quantas empresas?</label>
          <div class="quantidade">
            <input id="gl-quantidade" v-model.number="quantidade" type="number" min="1" :max="teto.maximo" step="1" required />
            <button v-for="n in atalhos" :key="n" type="button" class="chip" :class="{ active: quantidade === n }" @click="quantidade = n">
              {{ formatNumber(n) }}
            </button>
            <button type="button" class="chip" :class="{ active: quantidade === teto.maximo }" @click="quantidade = teto.maximo">
              Máximo ({{ formatNumber(teto.maximo) }})
            </button>
          </div>
          <span v-if="avisoQuantidade" class="error-text">{{ avisoQuantidade }}</span>
          <span v-else-if="teto.podeSairMenor" class="muted nota">A lista pode sair menor que o pedido: o total exato deste filtro não é conhecido.</span>
        </div>

        <fieldset class="exclusoes">
          <legend>Não gastar cota com quem você já tem</legend>
          <label class="check"><input v-model="excluirNoCrm" type="checkbox" /> Pular empresas que já estão no CRM</label>
          <label class="check"><input v-model="excluirEmListas" type="checkbox" /> Pular empresas de outras listas</label>
        </fieldset>

        <div class="field">
          <label for="gl-descricao">Observação (opcional)</label>
          <input id="gl-descricao" v-model="descricao" type="text" maxlength="240" placeholder="Para que serve esta lista" />
        </div>

        <p v-if="quantidadeValida" class="uso">
          Esta lista usa <strong>{{ formatNumber(quantidade) }}</strong> dos seus
          <strong>{{ formatNumber(cota.restante) }}</strong> leads restantes no mês.
        </p>
      </template>

      <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>
    </form>

    <template #footer>
      <button type="button" class="btn btn-outline" @click="emit('close')">Cancelar</button>
      <button type="submit" form="form-gerar-lista" class="btn btn-primary" :disabled="!podeGerar">
        {{ salvando ? 'Gerando…' : 'Gerar lista' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.resumo {
  margin: 0 0 14px;
  color: var(--ci-text-2);
}
.quantidade {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.quantidade input {
  width: 120px;
}
.nota {
  font-size: 12.5px;
}
.exclusoes {
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  padding: 10px 14px 12px;
  margin: 0 0 14px;
}
.exclusoes legend {
  font-size: 13px;
  font-weight: 500;
  color: var(--ci-text-2);
  padding: 0 6px;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
  cursor: pointer;
}
.uso {
  margin: 4px 0 0;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
}
.faixa {
  margin-bottom: 4px;
}
</style>
