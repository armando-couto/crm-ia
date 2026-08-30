<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Pipeline, PipelineStage } from '../types'

const toast = useToastStore()

const pipelines = ref<Pipeline[]>([])
const loading = ref(false)

const pipelineModal = ref(false)
const pipelineName = ref('')

const stageModal = ref(false)
const stagePipelineId = ref(0)
const editingStage = ref<PipelineStage | null>(null)
const stageForm = ref({ name: '', position: 0, probability: 0, is_won: false, is_lost: false })
const saving = ref(false)

async function load() {
  loading.value = true
  try {
    pipelines.value = await api.get<Pipeline[]>('/pipelines')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function createPipeline() {
  if (!pipelineName.value.trim()) return
  try {
    await api.post('/pipelines', { name: pipelineName.value.trim(), position: pipelines.value.length })
    toast.push('Pipeline criado')
    pipelineModal.value = false
    pipelineName.value = ''
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removePipeline(p: Pipeline) {
  if (!confirm(`Remover o pipeline "${p.name}" e todas as suas etapas?`)) return
  try {
    await api.delete(`/pipelines/${p.id}`)
    toast.push('Pipeline removido')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

function openNewStage(pipeline: Pipeline) {
  editingStage.value = null
  stagePipelineId.value = pipeline.id
  stageForm.value = { name: '', position: pipeline.stages.length, probability: 50, is_won: false, is_lost: false }
  stageModal.value = true
}

function openEditStage(stage: PipelineStage) {
  editingStage.value = stage
  stagePipelineId.value = stage.pipeline_id
  stageForm.value = {
    name: stage.name,
    position: stage.position,
    probability: stage.probability,
    is_won: stage.is_won,
    is_lost: stage.is_lost
  }
  stageModal.value = true
}

async function saveStage() {
  saving.value = true
  try {
    if (editingStage.value) {
      await api.put(`/stages/${editingStage.value.id}`, stageForm.value)
      toast.push('Etapa atualizada')
    } else {
      await api.post(`/pipelines/${stagePipelineId.value}/stages`, stageForm.value)
      toast.push('Etapa criada')
    }
    stageModal.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function removeStage(stage: PipelineStage) {
  if (!confirm(`Remover a etapa "${stage.name}"?`)) return
  try {
    await api.delete(`/stages/${stage.id}`)
    toast.push('Etapa removida')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Pipelines</h1>
      <button class="btn btn-primary" type="button" @click="pipelineModal = true">+ Novo pipeline</button>
    </div>

    <div class="pipelines">
      <div v-for="p in pipelines" :key="p.id" class="card">
        <div class="pipeline-head">
          <h2>{{ p.name }}</h2>
          <div class="toolbar">
            <button class="btn btn-outline btn-sm" type="button" @click="openNewStage(p)">+ Etapa</button>
            <button class="btn btn-danger btn-sm" type="button" @click="removePipeline(p)">Remover</button>
          </div>
        </div>

        <table class="data">
          <thead>
            <tr>
              <th>Etapa</th>
              <th>Ordem</th>
              <th>Probabilidade</th>
              <th>Tipo</th>
              <th style="width: 150px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in p.stages" :key="s.id" style="cursor: default">
              <td><strong>{{ s.name }}</strong></td>
              <td>{{ s.position + 1 }}</td>
              <td>{{ s.probability }}%</td>
              <td>
                <span v-if="s.is_won" class="badge green">Ganho</span>
                <span v-else-if="s.is_lost" class="badge red">Perdido</span>
                <span v-else class="badge gray">Aberta</span>
              </td>
              <td>
                <button class="btn btn-outline btn-sm" type="button" @click="openEditStage(s)">Editar</button>
                <button class="btn btn-danger btn-sm" type="button" @click="removeStage(s)">✕</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <ModalDialog title="Novo pipeline" :open="pipelineModal" @close="pipelineModal = false">
      <form @submit.prevent="createPipeline">
        <div class="field">
          <label>Nome *</label>
          <input v-model="pipelineName" required placeholder="ex.: Pipeline de Parcerias" />
        </div>
        <button class="btn btn-primary" type="submit" style="width: 100%; justify-content: center">Criar pipeline</button>
      </form>
    </ModalDialog>

    <ModalDialog :title="editingStage ? 'Editar etapa' : 'Nova etapa'" :open="stageModal" @close="stageModal = false">
      <form @submit.prevent="saveStage">
        <div class="field">
          <label>Nome *</label>
          <input v-model="stageForm.name" required />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Ordem</label>
            <input v-model.number="stageForm.position" type="number" min="0" />
          </div>
          <div class="field">
            <label>Probabilidade (%)</label>
            <input v-model.number="stageForm.probability" type="number" min="0" max="100" />
          </div>
        </div>
        <div class="field">
          <label class="check-inline">
            <input v-model="stageForm.is_won" type="checkbox" :disabled="stageForm.is_lost" />
            Etapa de ganho (fecha o negócio como ganho)
          </label>
        </div>
        <div class="field">
          <label class="check-inline">
            <input v-model="stageForm.is_lost" type="checkbox" :disabled="stageForm.is_won" />
            Etapa de perda (fecha o negócio como perdido)
          </label>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar etapa' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.pipelines {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.pipeline-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.pipeline-head h2 {
  font-size: 16px;
}

.check-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
</style>
