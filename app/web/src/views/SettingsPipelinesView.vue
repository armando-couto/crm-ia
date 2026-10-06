<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { Pipeline, PipelineStage } from '../types'

const toast = useToastStore()

const pipelines = ref<Pipeline[]>([])
const selectedId = ref(0)
const loading = ref(false)
const saving = ref(false)

const pipelineModal = ref(false)
const pipelineName = ref('')

const pipeline = computed(() => pipelines.value.find((p) => p.id === selectedId.value))
const stages = computed(() => pipeline.value?.stages ?? [])

// Opções de probabilidade no estilo HubSpot: percentuais + fases de fechamento.
const probabilityOptions = [
  { value: 'won', label: 'Fechado ganho (100%)' },
  { value: 'lost', label: 'Fechado perdido (0%)' },
  ...[90, 80, 75, 60, 50, 40, 25, 20, 10].map((p) => ({ value: String(p), label: `${p}%` }))
]

function stageProbabilityValue(stage: PipelineStage): string {
  if (stage.is_won) return 'won'
  if (stage.is_lost) return 'lost'
  return String(stage.probability)
}

async function load(keepSelection = true) {
  loading.value = true
  try {
    pipelines.value = await api.get<Pipeline[]>('/pipelines')
    if (!keepSelection || !pipelines.value.some((p) => p.id === selectedId.value)) {
      selectedId.value = pipelines.value[0]?.id ?? 0
    }
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

// ===== Pipelines =====
async function createPipeline() {
  if (!pipelineName.value.trim()) return
  saving.value = true
  try {
    const created = await api.post<Pipeline>('/pipelines', {
      name: pipelineName.value.trim(),
      position: pipelines.value.length
    })
    toast.push('Pipeline criado — adicione as fases')
    pipelineModal.value = false
    pipelineName.value = ''
    await load(false)
    selectedId.value = created.id
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function renamePipeline() {
  if (!pipeline.value) return
  const name = prompt('Novo nome do pipeline:', pipeline.value.name)
  if (!name?.trim() || name.trim() === pipeline.value.name) return
  try {
    await api.put(`/pipelines/${pipeline.value.id}`, { name: name.trim(), position: pipeline.value.position })
    toast.push('Pipeline renomeado')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removePipeline() {
  if (!pipeline.value) return
  if (!confirm(`Excluir o pipeline "${pipeline.value.name}" e todas as suas fases?`)) return
  try {
    await api.delete(`/pipelines/${pipeline.value.id}`)
    toast.push('Pipeline excluído')
    await load(false)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Fases =====
async function saveStage(stage: PipelineStage) {
  try {
    await api.put(`/stages/${stage.id}`, {
      name: stage.name,
      position: stage.position,
      probability: stage.probability,
      is_won: stage.is_won,
      is_lost: stage.is_lost
    })
  } catch (e: any) {
    toast.error(e.message)
    await load()
  }
}

async function renameStage(stage: PipelineStage, event: Event) {
  const name = (event.target as HTMLInputElement).value.trim()
  if (!name || name === stage.name) return
  stage.name = name
  await saveStage(stage)
  toast.push('Fase atualizada')
}

async function changeProbability(stage: PipelineStage, event: Event) {
  const value = (event.target as HTMLSelectElement).value
  stage.is_won = value === 'won'
  stage.is_lost = value === 'lost'
  if (value === 'won') {
    stage.probability = 100
  } else if (value === 'lost') {
    stage.probability = 0
  } else {
    stage.probability = Number(value)
  }
  await saveStage(stage)
  toast.push('Probabilidade atualizada')
}

async function moveStage(index: number, delta: number) {
  if (!pipeline.value) return
  const target = index + delta
  if (target < 0 || target >= stages.value.length) return
  const order = stages.value.map((s) => s.id)
  ;[order[index], order[target]] = [order[target], order[index]]
  try {
    await api.post(`/pipelines/${pipeline.value.id}/stages/reorder`, { stage_ids: order })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function addStage() {
  if (!pipeline.value) return
  try {
    await api.post(`/pipelines/${pipeline.value.id}/stages`, {
      name: 'Nova fase',
      position: stages.value.length,
      probability: 50,
      is_won: false,
      is_lost: false
    })
    toast.push('Fase adicionada — edite o nome na tabela')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removeStage(stage: PipelineStage) {
  if ((stage.deals_count ?? 0) > 0) {
    toast.error(`Mova os ${stage.deals_count} negócio(s) desta fase antes de excluí-la`)
    return
  }
  if (!confirm(`Excluir a fase "${stage.name}"?`)) return
  try {
    await api.delete(`/stages/${stage.id}`)
    toast.push('Fase excluída')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(() => load(false))
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Pipelines</h1>
        <p class="muted" style="margin: 4px 0 0">
          Defina as fases que os negócios percorrem e a probabilidade de fechamento de cada uma.
        </p>
      </div>
      <button class="btn btn-primary" type="button" @click="pipelineModal = true">+ Criar pipeline</button>
    </div>

    <div class="toolbar picker">
      <select v-model.number="selectedId">
        <option v-for="p in pipelines" :key="p.id" :value="p.id">
          {{ p.name }} ({{ p.deals_count ?? 0 }} negócios)
        </option>
      </select>
      <button class="btn btn-outline btn-sm" type="button" :disabled="!pipeline" @click="renamePipeline">Renomear</button>
      <button class="btn btn-danger btn-sm" type="button" :disabled="!pipeline" @click="removePipeline">Excluir</button>
    </div>

    <div class="table-wrap" v-if="pipeline">
      <table class="data">
        <thead>
          <tr>
            <th style="width: 70px">Ordem</th>
            <th>Nome da fase</th>
            <th style="width: 220px">Probabilidade da fase</th>
            <th style="width: 110px">Usado por</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(s, i) in stages" :key="s.id" style="cursor: default">
            <td>
              <span class="order-btns">
                <button type="button" :disabled="i === 0" @click="moveStage(i, -1)">↑</button>
                <button type="button" :disabled="i === stages.length - 1" @click="moveStage(i, 1)">↓</button>
              </span>
            </td>
            <td>
              <input class="stage-name-input" :value="s.name" @change="renameStage(s, $event)" />
            </td>
            <td>
              <select class="prob-select" :value="stageProbabilityValue(s)" @change="changeProbability(s, $event)">
                <option v-for="o in probabilityOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
              </select>
            </td>
            <td>
              <span class="badge" :class="s.deals_count ? 'blue' : 'gray'">{{ s.deals_count ?? 0 }} negócio(s)</span>
            </td>
            <td>
              <button class="btn btn-danger btn-sm" type="button" title="Excluir fase" @click="removeStage(s)">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <button class="add-stage" type="button" @click="addStage">+ Adicionar fase</button>
    </div>

    <div v-else-if="!loading" class="empty-state">
      <strong>Nenhum pipeline criado</strong>
      Crie o primeiro pipeline para organizar os negócios.
    </div>

    <ModalDialog title="Criar pipeline" :open="pipelineModal" @close="pipelineModal = false">
      <form @submit.prevent="createPipeline">
        <div class="field">
          <label>Nome *</label>
          <input v-model="pipelineName" required placeholder="ex.: Prospecção, Pós-venda…" />
        </div>
        <p class="muted" style="font-size: 13px">
          Depois de criar, adicione as fases e defina a probabilidade de cada uma na tabela.
        </p>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Criando…' : 'Criar pipeline' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.picker {
  margin-bottom: 14px;
}

.picker select {
  min-width: 280px;
}

.order-btns {
  display: inline-flex;
  gap: 2px;
}

.order-btns button {
  border: 1px solid var(--ci-border);
  background: var(--ci-surface);
  border-radius: 5px;
  cursor: pointer;
  font-size: 11px;
  width: 24px;
  height: 24px;
}

.order-btns button:disabled {
  opacity: 0.3;
  cursor: default;
}

.stage-name-input {
  width: 100%;
  padding: 7px 10px;
  border: 1px solid transparent;
  border-radius: 7px;
  font-size: 14px;
  font-weight: 500;
  background: transparent;
  outline: none;
}

.stage-name-input:hover {
  border-color: var(--ci-border);
}

.stage-name-input:focus {
  border-color: var(--ci-purple);
  background: var(--ci-surface);
}

.prob-select {
  width: 100%;
  padding: 7px 10px;
  border: 1px solid var(--ci-border);
  border-radius: 7px;
  font-size: 13px;
  background: var(--ci-surface);
  outline: none;
}

.add-stage {
  width: 100%;
  border: none;
  border-top: 1px solid var(--ci-border);
  background: none;
  padding: 12px;
  font-size: 13px;
  color: var(--ci-purple);
  cursor: pointer;
  font-weight: 500;
}

.add-stage:hover {
  background: var(--ci-purple-tint);
}
</style>
