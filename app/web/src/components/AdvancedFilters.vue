<script setup lang="ts">
import { ref, watch } from 'vue'
import type { FilterCondition, FilterFieldDef, FilterGroup } from '../types'

const props = defineProps<{
  open: boolean
  fields: FilterFieldDef[]
  modelValue: FilterGroup[]
}>()

const emit = defineEmits<{
  close: []
  apply: [groups: FilterGroup[]]
}>()

const groups = ref<FilterGroup[]>([])

watch(
  () => props.open,
  (open) => {
    if (open) {
      groups.value = JSON.parse(JSON.stringify(props.modelValue || []))
      if (!groups.value.length) addGroup()
      editing.value = null
    }
  }
)

// ===== Edição de condição =====
interface Draft {
  groupIndex: number
  field: string
  op: string
  value: string
  values: string[]
}

const editing = ref<Draft | null>(null)

const opsByKind: Record<string, { value: string; label: string }[]> = {
  text: [
    { value: 'contains', label: 'contém' },
    { value: 'eq', label: 'é igual a' },
    { value: 'empty', label: 'está vazio' },
    { value: 'not_empty', label: 'não está vazio' }
  ],
  enum: [
    { value: 'any_of', label: 'é qualquer de' },
    { value: 'none_of', label: 'não é nenhum de' }
  ],
  ref: [
    { value: 'any_of', label: 'é qualquer de' },
    { value: 'none_of', label: 'não é nenhum de' },
    { value: 'empty', label: 'está vazio' },
    { value: 'not_empty', label: 'não está vazio' }
  ],
  date: [
    { value: 'last_days', label: 'nos últimos X dias' },
    { value: 'older_than', label: 'há mais de X dias (ou nunca)' },
    { value: 'empty', label: 'nunca ocorreu' },
    { value: 'not_empty', label: 'já ocorreu' }
  ],
  number: [
    { value: 'gte', label: 'é maior ou igual a' },
    { value: 'lte', label: 'é menor ou igual a' }
  ]
}

function fieldDef(key: string): FilterFieldDef | undefined {
  return props.fields.find((f) => f.key === key)
}

function startAdding(groupIndex: number) {
  editing.value = { groupIndex, field: '', op: '', value: '', values: [] }
}

function onFieldChange() {
  if (!editing.value) return
  const def = fieldDef(editing.value.field)
  editing.value.op = def ? opsByKind[def.kind][0].value : ''
  editing.value.value = ''
  editing.value.values = []
}

function needsValue(op: string): boolean {
  return op === 'contains' || op === 'eq' || op === 'last_days' || op === 'older_than' || op === 'gte' || op === 'lte'
}

function numericOp(op: string): boolean {
  return op === 'last_days' || op === 'older_than' || op === 'gte' || op === 'lte'
}

function needsValues(op: string): boolean {
  return op === 'any_of' || op === 'none_of'
}

function toggleValue(v: string) {
  if (!editing.value) return
  const i = editing.value.values.indexOf(v)
  if (i >= 0) {
    editing.value.values.splice(i, 1)
  } else {
    editing.value.values.push(v)
  }
}

function draftValid(): boolean {
  if (!editing.value?.field || !editing.value.op) return false
  if (needsValue(editing.value.op) && !editing.value.value.trim()) return false
  if (needsValues(editing.value.op) && !editing.value.values.length) return false
  return true
}

function confirmDraft() {
  if (!editing.value || !draftValid()) return
  const cond: FilterCondition = { field: editing.value.field, op: editing.value.op }
  if (needsValue(editing.value.op)) cond.value = editing.value.value.trim()
  if (needsValues(editing.value.op)) cond.values = [...editing.value.values]
  groups.value[editing.value.groupIndex].conditions.push(cond)
  editing.value = null
}

// ===== Grupos =====
function addGroup() {
  groups.value.push({ conditions: [] })
}

function duplicateGroup(index: number) {
  groups.value.splice(index + 1, 0, JSON.parse(JSON.stringify(groups.value[index])))
}

function removeGroup(index: number) {
  groups.value.splice(index, 1)
  if (!groups.value.length) addGroup()
}

function removeCondition(groupIndex: number, condIndex: number) {
  groups.value[groupIndex].conditions.splice(condIndex, 1)
}

// ===== Resumo da condição =====
function conditionLabel(c: FilterCondition): string {
  const def = fieldDef(c.field)
  if (!def) return c.field
  const op = opsByKind[def.kind].find((o) => o.value === c.op)?.label ?? c.op
  let value = ''
  if (c.values?.length) {
    const labels = c.values.map((v) => def.options?.find((o) => o.value === v)?.label ?? v)
    value = labels.join(', ')
  } else if (c.value) {
    value = c.op === 'last_days' || c.op === 'older_than' ? `${c.value} dias` : c.value
    if (c.op === 'gte' || c.op === 'lte') value = `R$ ${c.value}`
  }
  return value ? `${def.label} ${op} ${value}` : `${def.label} ${op}`
}

function apply() {
  const cleaned = groups.value.filter((g) => g.conditions.length > 0)
  emit('apply', cleaned)
  emit('close')
}

function clearAll() {
  groups.value = [{ conditions: [] }]
  emit('apply', [])
  emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="af-overlay" @mousedown.self="emit('close')">
      <aside class="af-drawer">
        <header>
          <h2>Todos os filtros</h2>
          <button type="button" class="af-close" aria-label="Fechar" @click="emit('close')">×</button>
        </header>

        <div class="af-body">
          <p class="af-sub">Filtros avançados</p>

          <template v-for="(group, gi) in groups" :key="gi">
            <div class="af-group">
              <div class="af-group-head">
                <strong>Grupo {{ gi + 1 }}</strong>
                <span class="af-group-actions">
                  <button type="button" title="Duplicar grupo" @click="duplicateGroup(gi)">⧉</button>
                  <button type="button" title="Excluir grupo" @click="removeGroup(gi)">🗑</button>
                </span>
              </div>

              <template v-for="(cond, ci) in group.conditions" :key="ci">
                <div class="af-cond">
                  <span>{{ conditionLabel(cond) }}</span>
                  <button type="button" aria-label="Remover condição" @click="removeCondition(gi, ci)">×</button>
                </div>
                <div class="af-and">e</div>
              </template>

              <div v-if="editing?.groupIndex === gi" class="af-editor">
                <select v-model="editing.field" @change="onFieldChange">
                  <option value="" disabled>Escolha a propriedade…</option>
                  <option v-for="f in fields" :key="f.key" :value="f.key">{{ f.label }}</option>
                </select>

                <select v-if="editing.field" v-model="editing.op">
                  <option v-for="o in opsByKind[fieldDef(editing.field)!.kind]" :key="o.value" :value="o.value">
                    {{ o.label }}
                  </option>
                </select>

                <input
                  v-if="editing.field && needsValue(editing.op)"
                  v-model="editing.value"
                  :type="numericOp(editing.op) ? 'number' : 'text'"
                  :placeholder="editing.op === 'last_days' || editing.op === 'older_than' ? 'dias' : 'valor'"
                  :min="editing.op === 'gte' || editing.op === 'lte' ? undefined : 1"
                />

                <div v-if="editing.field && needsValues(editing.op)" class="af-options">
                  <label v-for="o in fieldDef(editing.field)!.options" :key="o.value">
                    <input type="checkbox" :checked="editing.values.includes(o.value)" @change="toggleValue(o.value)" />
                    {{ o.label }}
                  </label>
                </div>

                <div class="af-editor-actions">
                  <button type="button" class="btn btn-primary btn-sm" :disabled="!draftValid()" @click="confirmDraft">
                    Adicionar
                  </button>
                  <button type="button" class="btn btn-sm" @click="editing = null">Cancelar</button>
                </div>
              </div>

              <button v-else type="button" class="af-add" @click="startAdding(gi)">+ Adicionar filtro</button>
            </div>

            <div v-if="gi < groups.length - 1" class="af-or">ou</div>
          </template>

          <div class="af-or-row">
            <span class="af-or-chip">ou</span>
            <button type="button" class="af-add-group" @click="addGroup">+ Adicionar grupo de filtros</button>
          </div>
        </div>

        <footer>
          <button type="button" class="btn btn-primary" @click="apply">Aplicar filtros</button>
          <button type="button" class="btn btn-outline" @click="clearAll">Limpar tudo</button>
        </footer>
      </aside>
    </div>
  </Teleport>
</template>

<style scoped>
.af-overlay {
  position: fixed;
  inset: 0;
  background: rgba(34, 20, 60, 0.35);
  z-index: 160;
  display: flex;
  justify-content: flex-end;
}

.af-drawer {
  width: min(440px, 100vw);
  background: var(--ci-surface);
  height: 100%;
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-lg);
  animation: af-in 0.2s ease-out;
}

@keyframes af-in {
  from {
    transform: translateX(30px);
    opacity: 0;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid var(--ci-border);
}

header h2 {
  font-size: 18px;
}

.af-close {
  border: none;
  background: none;
  font-size: 24px;
  color: var(--ci-text-3);
  cursor: pointer;
  line-height: 1;
}

.af-body {
  flex: 1;
  overflow-y: auto;
  padding: 18px 22px;
}

.af-sub {
  font-size: 13px;
  font-weight: 600;
  margin: 0 0 12px;
}

.af-group {
  border: 1px solid var(--ci-border);
  border-radius: 12px;
  padding: 14px;
}

.af-group-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  font-size: 14px;
}

.af-group-actions button {
  border: none;
  background: none;
  cursor: pointer;
  font-size: 14px;
  color: var(--ci-purple);
  padding: 4px 6px;
  border-radius: 5px;
}

.af-group-actions button:hover {
  background: var(--ci-purple-tint);
}

.af-cond {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  background: var(--ci-bg);
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
}

.af-cond button {
  border: none;
  background: none;
  color: var(--ci-purple);
  font-size: 16px;
  cursor: pointer;
  line-height: 1;
}

.af-and {
  font-size: 12px;
  color: var(--ci-text-3);
  padding: 4px 2px;
}

.af-add {
  width: 100%;
  border: none;
  background: var(--ci-bg);
  border-radius: 8px;
  padding: 10px;
  font-size: 13px;
  color: var(--ci-text-2);
  cursor: pointer;
}

.af-add:hover {
  color: var(--ci-purple);
}

.af-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--ci-bg);
  border-radius: 8px;
  padding: 12px;
}

.af-editor select,
.af-editor input {
  padding: 8px 10px;
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  font-size: 13px;
  background: var(--ci-surface);
  outline: none;
}

.af-options {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 180px;
  overflow-y: auto;
  background: var(--ci-surface);
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  padding: 10px;
}

.af-options label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  cursor: pointer;
}

.af-editor-actions {
  display: flex;
  gap: 8px;
}

.af-or {
  padding: 8px 0 8px 18px;
  font-size: 13px;
  font-weight: 600;
  color: var(--ci-text-2);
  border-left: 1px solid var(--ci-border);
  margin-left: 22px;
}

.af-or-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
}

.af-or-chip {
  border: 1px solid var(--ci-border);
  border-radius: 8px;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 600;
  background: var(--ci-surface);
}

.af-add-group {
  border: none;
  background: var(--ci-bg);
  border-radius: 8px;
  padding: 8px 14px;
  font-size: 13px;
  color: var(--ci-text-2);
  cursor: pointer;
}

.af-add-group:hover {
  color: var(--ci-purple);
}

footer {
  display: flex;
  gap: 10px;
  padding: 14px 22px;
  border-top: 1px solid var(--ci-border);
}
</style>
