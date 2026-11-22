<script setup lang="ts">
// Diálogo único de "negócio perdido": o mesmo no botão "Perdido" (quadro e
// ficha) e ao soltar o cartão em uma etapa de perda. Lê os motivos em
// GET /settings/motivos-perda (só exige estar autenticado) e devolve
// { motivo, observacao } para quem abriu fazer a chamada.
import { computed, ref, watch } from 'vue'
import { api } from '../../api'
import type { MotivosPerda, PerdaRequest } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'
import { MOTIVOS_PERDA_PADRAO } from './perda'

const props = defineProps<{
  open: boolean
  /** Nome do negócio, para o texto do diálogo. */
  nome?: string
  /** Chamada em andamento em quem abriu o diálogo. */
  salvando?: boolean
  /** Erro devolvido pela chamada (ex.: 422 "motivo obrigatório"). */
  erro?: string
}>()

const emit = defineEmits<{ confirm: [perda: PerdaRequest]; cancel: [] }>()

const config = ref<MotivosPerda>({ motivos: [...MOTIVOS_PERDA_PADRAO], exigir: true })
const carregando = ref(false)
const motivo = ref('')
const observacao = ref('')
const tentou = ref(false)

const faltaMotivo = computed(() => config.value.exigir && !motivo.value)

async function carregar() {
  carregando.value = true
  try {
    const r = await api.get<MotivosPerda>('/settings/motivos-perda')
    config.value = {
      motivos: Array.isArray(r?.motivos) && r.motivos.length ? r.motivos : [...MOTIVOS_PERDA_PADRAO],
      exigir: r?.exigir !== false
    }
  } catch {
    // Sem a lista, vale o padrão do contrato; o servidor continua validando.
    config.value = { motivos: [...MOTIVOS_PERDA_PADRAO], exigir: true }
  } finally {
    carregando.value = false
  }
}

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    motivo.value = ''
    observacao.value = ''
    tentou.value = false
    carregar()
  },
  { immediate: true }
)

function confirmar() {
  tentou.value = true
  if (faltaMotivo.value || props.salvando) return
  const perda: PerdaRequest = {}
  if (motivo.value) perda.motivo = motivo.value
  if (observacao.value.trim()) perda.observacao = observacao.value.trim()
  emit('confirm', perda)
}
</script>

<template>
  <ModalDialog title="Marcar como perdido" :open="open" @close="emit('cancel')">
    <form id="form-perda" class="perda" data-dialogo="perda" @submit.prevent="confirmar">
      <p class="intro">
        Por que <strong>{{ nome || 'este negócio' }}</strong> foi perdido?
        <span class="muted">O motivo alimenta o relatório de perdas do funil.</span>
      </p>

      <fieldset class="motivos" :disabled="carregando">
        <legend>Motivo<span v-if="!config.exigir" class="muted"> (opcional)</span></legend>
        <label v-for="m in config.motivos" :key="m" class="motivo" :class="{ selected: motivo === m }">
          <input v-model="motivo" type="radio" name="motivo-perda" :value="m" />
          {{ m }}
        </label>
      </fieldset>

      <div class="field">
        <label for="perda-observacao">Observação (opcional)</label>
        <textarea id="perda-observacao" v-model="observacao" rows="3" maxlength="1000" placeholder="Ex.: fechou com o concorrente por prazo de entrega"></textarea>
      </div>

      <p v-if="tentou && faltaMotivo" class="error-text" role="alert">Escolha o motivo da perda.</p>
      <p v-else-if="erro" class="error-text" role="alert">{{ erro }}</p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-outline" data-acao="cancelar" @click="emit('cancel')">Cancelar</button>
      <button type="submit" form="form-perda" class="btn btn-danger" data-acao="confirmar" :disabled="salvando">
        {{ salvando ? 'Salvando…' : 'Marcar como perdido' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.intro {
  margin: 0 0 14px;
  font-size: 14px;
}

.intro .muted {
  display: block;
  margin-top: 2px;
  font-size: 12px;
}

.motivos {
  border: none;
  margin: 0 0 14px;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.motivos legend {
  padding: 0;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--ci-text-2);
}

.motivo {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid var(--ci-border);
  border-radius: 999px;
  font-size: 13px;
  cursor: pointer;
  background: var(--ci-surface);
}

.motivo input {
  margin: 0;
}

.motivo.selected {
  border-color: var(--ci-purple);
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  font-weight: 500;
}
</style>
