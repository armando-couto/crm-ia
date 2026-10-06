<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import type { CustomProperty } from '../types'

const props = defineProps<{
  entity: 'contacts' | 'companies' | 'deals' | 'tickets'
  recordId: number
}>()

const toast = useToastStore()

const definitions = ref<CustomProperty[]>([])
const values = ref<Record<string, any>>({})
const draft = ref<Record<string, any>>({})
const editing = ref(false)
const saving = ref(false)
const loading = ref(true)

const hasProperties = computed(() => definitions.value.length > 0)

async function load() {
  loading.value = true
  try {
    const [defs, vals] = await Promise.all([
      api.get<CustomProperty[]>(`/properties?entity=${props.entity}`),
      api.get<{ values: Record<string, any> }>(`/properties/values?entity=${props.entity}&record_id=${props.recordId}`)
    ])
    definitions.value = defs ?? []
    values.value = vals.values ?? {}
  } catch {
    /* seção some quando não há propriedades */
  } finally {
    loading.value = false
  }
}

function startEdit() {
  draft.value = {}
  for (const def of definitions.value) {
    const current = values.value[def.key]
    if (def.field_type === 'multipla') {
      draft.value[def.key] = Array.isArray(current) ? [...current] : []
    } else if (def.field_type === 'booleano') {
      draft.value[def.key] = current === true
    } else {
      draft.value[def.key] = current ?? ''
    }
  }
  editing.value = true
}

function toggleMulti(key: string, value: string) {
  const list: string[] = draft.value[key] ?? []
  const i = list.indexOf(value)
  if (i >= 0) {
    list.splice(i, 1)
  } else {
    list.push(value)
  }
  draft.value[key] = [...list]
}

async function save() {
  saving.value = true
  try {
    const resp = await api.put<{ values: Record<string, any> }>('/properties/values', {
      entity: props.entity,
      record_id: props.recordId,
      values: draft.value
    })
    values.value = resp.values ?? {}
    editing.value = false
    toast.push('Propriedades atualizadas')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

// Texto legível do valor gravado, conforme o tipo do campo.
function displayValue(def: CustomProperty): string {
  const v = values.value[def.key]
  if (v === undefined || v === null || v === '') return '—'
  switch (def.field_type) {
    case 'booleano':
      return v ? 'Sim' : 'Não'
    case 'data':
      return formatDate(String(v))
    case 'numero':
      return new Intl.NumberFormat('pt-BR').format(Number(v))
    case 'selecao':
      return def.options.find((o) => o.value === v)?.label ?? String(v)
    case 'multipla':
      return (Array.isArray(v) ? v : [])
        .map((item) => def.options.find((o) => o.value === item)?.label ?? item)
        .join(', ')
    default:
      return String(v)
  }
}

onMounted(load)
defineExpose({ reload: load })
</script>

<template>
  <div class="custom-props card" v-if="hasProperties">
    <div class="props-head">
      <h2>Propriedades personalizadas</h2>
      <button v-if="!editing" class="btn btn-outline btn-sm" type="button" @click="startEdit">Editar</button>
    </div>

    <dl v-if="!editing">
      <template v-for="def in definitions" :key="def.id">
        <dt :title="def.description">{{ def.label }}</dt>
        <dd :class="{ muted: displayValue(def) === '—' }">{{ displayValue(def) }}</dd>
      </template>
    </dl>

    <form v-else @submit.prevent="save">
      <div v-for="def in definitions" :key="def.id" class="field">
        <label>{{ def.label }}</label>

        <textarea v-if="def.field_type === 'texto_longo'" v-model="draft[def.key]" rows="3"></textarea>

        <input v-else-if="def.field_type === 'numero'" v-model="draft[def.key]" type="number" step="any" />

        <input v-else-if="def.field_type === 'data'" v-model="draft[def.key]" type="date" />

        <label v-else-if="def.field_type === 'booleano'" class="check-inline">
          <input type="checkbox" v-model="draft[def.key]" />
          Sim
        </label>

        <select v-else-if="def.field_type === 'selecao'" v-model="draft[def.key]">
          <option value="">—</option>
          <option v-for="o in def.options" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>

        <div v-else-if="def.field_type === 'multipla'" class="multi-options">
          <label v-for="o in def.options" :key="o.value" class="check-inline">
            <input
              type="checkbox"
              :checked="(draft[def.key] ?? []).includes(o.value)"
              @change="toggleMulti(def.key, o.value)"
            />
            {{ o.label }}
          </label>
        </div>

        <input v-else v-model="draft[def.key]" type="text" />

        <small v-if="def.description" class="muted">{{ def.description }}</small>
      </div>

      <div class="props-actions">
        <button class="btn btn-primary btn-sm" type="submit" :disabled="saving">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
        <button class="btn btn-sm" type="button" @click="editing = false">Cancelar</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.custom-props {
  padding: 14px;
}

.props-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.props-head h2 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
}

dl {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
}

dt {
  color: var(--ci-text-3);
  font-size: 12px;
  margin-top: 10px;
}

dd {
  margin: 0;
  word-break: break-word;
}

.field small {
  font-size: 12px;
}

.multi-options {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  padding: 10px;
  max-height: 160px;
  overflow-y: auto;
}

.check-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 400 !important;
}

.props-actions {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}
</style>
