<script setup lang="ts">
import { computed } from 'vue'
import { useToastStore } from '../../stores/toast'
import {
  agruparTelefones,
  formatarTelefone,
  linkWhatsApp,
  opcoesDiscagem,
  podeLigar,
  podeWhatsApp
} from '../../telefone'
import { tipoTelefoneLabels, type Decisor, type Lead, type Telefone } from '../../types-prospeccao'
import { copiarTexto } from './util'

/**
 * Coluna "Ligar": só `lead.telefones`, na ordem recebida, agrupados por
 * pessoa. Cada número tem ligar (`tel:`), copiar e WhatsApp (com a regra do
 * opt-in). Telefone inválido aparece riscado e sem ações.
 */
const props = withDefaults(
  defineProps<{
    lead: Lead
    decisores?: Decisor[]
    /** `ConfigProspeccao.exigir_optin_whatsapp`. */
    exigirOptin?: boolean
    /** Base de demonstração: não liga nem abre WhatsApp. */
    amostra?: boolean
    /** Número da ligação em curso (destaque). */
    emLigacao?: string
    /** Pode marcar a autorização de WhatsApp (`leads.edit`). */
    podeEditar?: boolean
    salvandoOptin?: boolean
  }>(),
  { decisores: () => [], exigirOptin: true, amostra: false, emLigacao: '', podeEditar: true, salvandoOptin: false }
)

const emit = defineEmits<{
  /** Clique em "ligar": `numero` é sempre o `numero` original do telefone. */
  ligar: [numero: string]
  /** Caixa "Cliente autorizou WhatsApp". */
  optin: []
}>()

const toast = useToastStore()

const grupos = computed(() => agruparTelefones(props.lead.telefones, props.decisores))
const temCelular = computed(() => (props.lead.telefones ?? []).some((t) => t.tipo === 'celular' && !t.invalido))
const regras = computed(() => ({
  exigirOptin: props.exigirOptin,
  optin: props.lead.whatsapp_optin,
  amostra: props.amostra
}))

function dicaWhatsApp(tel: Telefone): string {
  if (props.amostra) return 'Dados de demonstração'
  if (tel.tipo !== 'celular') return 'WhatsApp só em celular'
  if (tel.whatsapp === false) return 'Este número não tem WhatsApp'
  if (props.exigirOptin && !props.lead.whatsapp_optin) return 'Peça a autorização do cliente antes de chamar no WhatsApp'
  return 'Abrir conversa no WhatsApp'
}

async function copiar(tel: Telefone) {
  const ok = await copiarTexto(formatarTelefone(tel.numero))
  if (ok) toast.push('Número copiado')
  else toast.error('Não foi possível copiar o número')
}
</script>

<template>
  <div class="telefones">
    <p v-if="!grupos.length" class="muted">Este lead não tem telefone.</p>

    <div v-for="grupo in grupos" :key="grupo.pessoa" class="grupo">
      <div class="grupo-head">
        <strong>{{ grupo.pessoa }}</strong>
        <span v-if="grupo.cargo" class="muted">{{ grupo.cargo }}</span>
      </div>

      <div
        v-for="tel in grupo.telefones"
        :key="tel.numero"
        class="telefone"
        :class="{ ativo: emLigacao === tel.numero, invalido: tel.invalido }"
        :data-numero="tel.numero"
      >
        <div class="numero">
          <span :class="{ 'telefone-invalido': tel.invalido }">{{ formatarTelefone(tel.numero) }}</span>
          <span class="selos">
            <span class="badge gray">{{ tipoTelefoneLabels[tel.tipo] ?? tel.tipo }}</span>
            <span v-if="tel.invalido" class="badge red">Inválido</span>
            <span v-if="tel.nao_perturbe" class="badge red" title="Número em cadastro de não perturbe">Não perturbe</span>
            <span v-if="tel.compartilhado" class="badge amber" title="Número usado por várias empresas (contador)">Compartilhado</span>
          </span>
        </div>

        <div v-if="!tel.invalido" class="acoes">
          <template v-if="podeLigar(tel, { amostra })">
            <a
              v-for="(op, i) in opcoesDiscagem(tel)"
              :key="op.numero"
              class="btn btn-primary acao ligar"
              :href="op.href"
              :title="`Ligar para ${op.rotulo}`"
              @click="emit('ligar', tel.numero)"
            >
              {{ opcoesDiscagem(tel).length > 1 ? (i === 0 ? 'Ligar sem o 9' : 'Ligar com o 9') : 'Ligar' }}
            </a>
          </template>
          <button v-else type="button" class="btn btn-primary acao ligar" disabled title="Dados de demonstração">
            Ligar
          </button>

          <button type="button" class="btn btn-outline acao copiar" title="Copiar o número" @click="copiar(tel)">Copiar</button>

          <a
            v-if="podeWhatsApp(tel, regras)"
            class="btn btn-outline acao whatsapp"
            :href="linkWhatsApp(tel.numero)"
            target="_blank"
            rel="noopener noreferrer"
            :title="dicaWhatsApp(tel)"
          >
            WhatsApp
          </a>
          <button v-else-if="tel.tipo === 'celular'" type="button" class="btn btn-outline acao whatsapp" disabled :title="dicaWhatsApp(tel)">
            WhatsApp
          </button>
        </div>
      </div>
    </div>

    <p v-if="amostra" class="faixa amber demo">Dados de demonstração: ligar e WhatsApp ficam desabilitados.</p>

    <label v-if="temCelular && exigirOptin && !amostra" class="optin">
      <input
        type="checkbox"
        :checked="lead.whatsapp_optin"
        :disabled="lead.whatsapp_optin || !podeEditar || salvandoOptin"
        @change="emit('optin')"
      />
      <span>Cliente autorizou WhatsApp</span>
    </label>
  </div>
</template>

<style scoped>
.telefones {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.grupo-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 13px;
  margin-bottom: 6px;
}

.telefone {
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  padding: 10px 12px;
  margin-bottom: 8px;
  background: var(--ci-surface);
}

.telefone.ativo {
  border-color: var(--ci-green);
  box-shadow: 0 0 0 2px var(--ci-green-tint);
}

.telefone.invalido {
  background: var(--ci-bg);
}

.numero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 17px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.selos {
  display: inline-flex;
  gap: 4px;
  flex-wrap: wrap;
}

.selos .badge {
  font-size: 11px;
  padding: 2px 8px;
}

.acoes {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 10px;
}

.acao {
  min-height: 40px;
  justify-content: center;
  text-decoration: none;
}

.acao.ligar {
  flex: 1 1 120px;
}

.demo {
  margin: 0;
}

.optin {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--ci-text-2);
  cursor: pointer;
}

@media (max-width: 720px) {
  .acao {
    min-height: 48px;
    flex: 1 1 30%;
  }

  .acao.ligar {
    flex-basis: 100%;
    font-size: 16px;
  }
}
</style>
