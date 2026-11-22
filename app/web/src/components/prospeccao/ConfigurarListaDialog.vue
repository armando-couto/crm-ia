<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '../../api'
import type { Pipeline, Playbook } from '../../types'
import type { AtualizarLista, Lista } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'

/**
 * Nome da lista, roteiro de ligação (manual de atividades) e funil de destino
 * da conversão. Vazio usa o padrão das configurações de prospecção.
 */
const props = defineProps<{ open: boolean; lista: Lista }>()
const emit = defineEmits<{ close: []; salvo: [lista: Lista] }>()

const nome = ref('')
const descricao = ref('')
const playbookId = ref<number | null>(null)
const pipelineId = ref<number | null>(null)
const stageId = ref<number | null>(null)
const playbooks = ref<Playbook[]>([])
const pipelines = ref<Pipeline[]>([])
const salvando = ref(false)
const erro = ref('')
let catalogosPedidos = false

async function carregarCatalogos() {
  if (catalogosPedidos) return
  catalogosPedidos = true
  const [pb, pl] = await Promise.allSettled([api.get<Playbook[]>('/playbooks'), api.get<Pipeline[]>('/pipelines')])
  if (pb.status === 'fulfilled') playbooks.value = (pb.value ?? []).filter((p) => p.active !== false || p.id === props.lista.playbook_id)
  if (pl.status === 'fulfilled') pipelines.value = pl.value ?? []
}

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    nome.value = props.lista.nome
    descricao.value = props.lista.descricao ?? ''
    playbookId.value = props.lista.playbook_id
    pipelineId.value = props.lista.pipeline_id
    stageId.value = props.lista.stage_id
    erro.value = ''
    void carregarCatalogos()
  },
  { immediate: true }
)

const etapas = computed(() => pipelines.value.find((p) => p.id === pipelineId.value)?.stages?.filter((s) => !s.is_won && !s.is_lost) ?? [])

watch(pipelineId, () => {
  if (stageId.value && !etapas.value.some((s) => s.id === stageId.value)) stageId.value = null
})

async function salvar() {
  if (!nome.value.trim() || salvando.value) return
  salvando.value = true
  erro.value = ''
  const corpo: AtualizarLista = {
    nome: nome.value.trim(),
    descricao: descricao.value.trim(),
    playbook_id: playbookId.value,
    pipeline_id: pipelineId.value,
    stage_id: pipelineId.value ? stageId.value : null
  }
  try {
    emit('salvo', await api.put<Lista>(`/prospeccao/listas/${props.lista.id}`, corpo))
  } catch (e: any) {
    erro.value = e?.message || 'não foi possível salvar'
  } finally {
    salvando.value = false
  }
}
</script>

<template>
  <ModalDialog title="Configurar a lista" :open="open" @close="emit('close')">
    <form id="form-config-lista" @submit.prevent="salvar">
      <div class="field">
        <label for="cl-nome">Nome</label>
        <input id="cl-nome" v-model="nome" type="text" maxlength="120" required />
      </div>
      <div class="field">
        <label for="cl-desc">Observação</label>
        <input id="cl-desc" v-model="descricao" type="text" maxlength="240" />
      </div>
      <div class="field">
        <label for="cl-roteiro">Roteiro da ligação</label>
        <select id="cl-roteiro" v-model="playbookId">
          <option :value="null">Padrão das configurações</option>
          <option v-for="p in playbooks" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <span class="muted pequeno">Aparece na fila de ligações para quem trabalhar os leads desta lista.</span>
      </div>
      <div class="form-row">
        <div class="field">
          <label for="cl-funil">Funil do negócio</label>
          <select id="cl-funil" v-model="pipelineId">
            <option :value="null">Padrão das configurações</option>
            <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <div class="field">
          <label for="cl-etapa">Etapa</label>
          <select id="cl-etapa" v-model="stageId" :disabled="!pipelineId">
            <option :value="null">Primeira etapa de reunião</option>
            <option v-for="s in etapas" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </div>
      </div>
      <p class="muted pequeno">O negócio nasce quando o vendedor marca a reunião com um lead desta lista.</p>
      <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-outline" @click="emit('close')">Cancelar</button>
      <button type="submit" form="form-config-lista" class="btn btn-primary" :disabled="salvando || !nome.trim()">
        {{ salvando ? 'Salvando…' : 'Salvar' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.pequeno {
  font-size: 12.5px;
}
</style>
