<script setup lang="ts">
import { ref, watch } from 'vue'
import { api, foiAbortado } from '../../api'
import { formatDate, formatMoney, mascaraCNPJ } from '../../format'
import { porteLabels, type EmpresaPrevia, type Filtros, type Ordem, type PreviaResposta } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'
import { formatarCnae } from './filtros'

/**
 * Prévia gratuita: até 25 empresas dos filtros, sem contatos e sem nomes de
 * sócios (só se tem celular, telefone, e-mail ou dívida), para conferir se o
 * recorte tem a cara do cliente antes de gastar cota.
 */
const props = defineProps<{ open: boolean; filtros: Filtros; ordem: Ordem; podeGerar?: boolean }>()
const emit = defineEmits<{ close: []; gerar: [] }>()

const itens = ref<EmpresaPrevia[]>([])
const carregando = ref(false)
const erro = ref('')
const ordemIgnorada = ref(false)
let controle: AbortController | null = null

async function carregar() {
  controle?.abort()
  controle = new AbortController()
  carregando.value = true
  erro.value = ''
  ordemIgnorada.value = false
  try {
    const r = await api.post<PreviaResposta>('/prospeccao/previa', { filtros: props.filtros, ordem: props.ordem }, { signal: controle.signal })
    itens.value = r?.itens ?? []
    ordemIgnorada.value = !!r?.ordem_aplicada && r.ordem_aplicada !== props.ordem
  } catch (e: any) {
    if (foiAbortado(e)) return
    erro.value = e?.message || 'não foi possível carregar a prévia'
    itens.value = []
  } finally {
    carregando.value = false
  }
}

watch(
  () => props.open,
  (aberto) => {
    if (aberto) void carregar()
    else controle?.abort()
  },
  { immediate: true }
)
</script>

<template>
  <ModalDialog title="Prévia das empresas" :open="open" wide @close="emit('close')">
    <p class="muted intro">
      Até 25 empresas dos filtros, sem os contatos. Telefones, e-mails e sócios aparecem quando a lista é gerada.
    </p>
    <div v-if="ordemIgnorada" class="faixa amber aviso">Muitas empresas para esta ordenação; mostrando os melhores contatos.</div>
    <div v-if="carregando" class="muted vazio">Carregando a prévia…</div>
    <p v-else-if="erro" class="error-text" role="alert">{{ erro }}</p>
    <div v-else-if="!itens.length" class="empty-state"><strong>Nenhuma empresa</strong> Nenhuma empresa casa com estes filtros.</div>
    <div v-else class="table-wrap">
      <table class="data previa">
        <thead>
          <tr>
            <th>Empresa</th>
            <th>Cidade</th>
            <th>Atividade</th>
            <th>Porte</th>
            <th>Abertura</th>
            <th>Contato</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in itens" :key="e.cnpj">
            <td>
              <div class="nome">{{ e.nome_fantasia || e.razao_social }}</div>
              <div class="muted pequeno">
                <span v-if="e.nome_fantasia">{{ e.razao_social }} · </span>{{ mascaraCNPJ(e.cnpj) }}
              </div>
            </td>
            <td>{{ e.municipio }}/{{ e.uf }}</td>
            <td>
              <span class="pequeno">{{ e.cnae.descricao }}</span>
              <div class="muted pequeno">{{ formatarCnae(e.cnae.codigo) }}</div>
            </td>
            <td>
              {{ porteLabels[e.porte] }}
              <div v-if="e.capital" class="muted pequeno">{{ formatMoney(e.capital) }}</div>
            </td>
            <td>{{ formatDate(e.abertura) }}</td>
            <td class="selos">
              <span v-if="e.tem_celular" class="badge green">celular</span>
              <span v-else-if="e.tem_telefone" class="badge blue">fixo</span>
              <span v-if="e.tem_email" class="badge blue">e-mail</span>
              <span v-if="!e.tem_celular && !e.tem_telefone && !e.tem_email" class="badge gray">sem contato</span>
              <span v-if="e.socios_qtd" class="badge gray">{{ e.socios_qtd }} sócio{{ e.socios_qtd > 1 ? 's' : '' }}</span>
              <span v-if="e.tem_divida" class="badge amber">dívida</span>
              <span v-if="e.pessoa_fisica" class="badge amber" title="MEI ou empresário individual: pessoa física">PF</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <template #footer>
      <button type="button" class="btn btn-outline" @click="emit('close')">Voltar aos filtros</button>
      <button v-if="podeGerar" type="button" class="btn btn-primary" :disabled="carregando || !itens.length" @click="emit('gerar')">Gerar lista</button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.intro {
  margin: 0 0 12px;
}
.aviso {
  margin-bottom: 12px;
}
.vazio {
  padding: 24px 0;
  text-align: center;
}
.previa td {
  vertical-align: top;
}
.nome {
  font-weight: 500;
}
.pequeno {
  font-size: 12px;
}
.selos {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.selos .badge {
  font-size: 11px;
}
</style>
