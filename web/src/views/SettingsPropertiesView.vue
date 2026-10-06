<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate } from '../format'
import { useToastStore } from '../stores/toast'
import ModalDialog from '../components/ModalDialog.vue'
import type { CustomProperty, PropertyOption } from '../types'

const toast = useToastStore()

const entity = ref<CustomProperty['entity']>('contacts')
const properties = ref<CustomProperty[]>([])
const search = ref('')
const loading = ref(false)

const entityLabels: Record<string, string> = {
  contacts: 'Propriedades de Contato',
  companies: 'Propriedades de Empresa',
  deals: 'Propriedades de Negócio',
  tickets: 'Propriedades de Ticket'
}

interface FieldTypeDef {
  value: CustomProperty['field_type']
  label: string
  hint: string
}

const fieldTypes: FieldTypeDef[] = [
  { value: 'texto', label: 'Texto de uma linha', hint: 'Nomes, códigos, identificadores' },
  { value: 'texto_longo', label: 'Texto de várias linhas', hint: 'Observações e descrições' },
  { value: 'numero', label: 'Número', hint: 'Valores, quantidades, TPV' },
  { value: 'data', label: 'Data', hint: 'Datas como credenciamento ou vencimento' },
  { value: 'selecao', label: 'Selecionar uma opção', hint: 'Lista suspensa com uma escolha' },
  { value: 'multipla', label: 'Selecionar múltiplas opções', hint: 'Várias escolhas ao mesmo tempo' },
  { value: 'booleano', label: 'Sim / Não', hint: 'Marcação simples' }
]

const fieldTypeLabels = computed<Record<string, string>>(() =>
  Object.fromEntries(fieldTypes.map((t) => [t.value, t.label]))
)

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return properties.value
  return properties.value.filter(
    (p) => p.label.toLowerCase().includes(term) || p.key.includes(term) || p.group_name.toLowerCase().includes(term)
  )
})

async function load() {
  loading.value = true
  try {
    properties.value = await api.get<CustomProperty[]>(`/properties?entity=${entity.value}`)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

// ===== Criar / editar =====
const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<CustomProperty | null>(null)
const form = ref({
  label: '',
  description: '',
  field_type: 'texto' as CustomProperty['field_type'],
  group_name: '',
  options: [] as PropertyOption[]
})

const needsOptions = computed(() => form.value.field_type === 'selecao' || form.value.field_type === 'multipla')

function openNew() {
  editing.value = null
  form.value = { label: '', description: '', field_type: 'texto', group_name: '', options: [] }
  modalOpen.value = true
}

function openEdit(p: CustomProperty) {
  editing.value = p
  form.value = {
    label: p.label,
    description: p.description,
    field_type: p.field_type,
    group_name: p.group_name,
    options: p.options.map((o) => ({ ...o }))
  }
  modalOpen.value = true
}

function addOption() {
  form.value.options.push({ value: '', label: '' })
}

function removeOption(index: number) {
  form.value.options.splice(index, 1)
}

function moveOption(index: number, delta: number) {
  const target = index + delta
  if (target < 0 || target >= form.value.options.length) return
  const next = [...form.value.options]
  ;[next[index], next[target]] = [next[target], next[index]]
  form.value.options = next
}

async function save() {
  saving.value = true
  try {
    const payload = {
      entity: entity.value,
      label: form.value.label,
      description: form.value.description,
      field_type: form.value.field_type,
      group_name: form.value.group_name,
      options: needsOptions.value ? form.value.options : []
    }
    if (editing.value) {
      await api.put(`/properties/${editing.value.id}`, payload)
      toast.push('Propriedade atualizada')
    } else {
      await api.post('/properties', payload)
      toast.push('Propriedade criada')
    }
    modalOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(p: CustomProperty) {
  const extra = p.used_count > 0 ? ` Os valores preenchidos em ${p.used_count} registro(s) serão apagados.` : ''
  if (!confirm(`Excluir a propriedade "${p.label}"?${extra}`)) return
  try {
    await api.delete(`/properties/${p.id}`)
    toast.push('Propriedade removida')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

function selectEntity(value: CustomProperty['entity']) {
  entity.value = value
  search.value = ''
  load()
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Propriedades</h1>
        <p class="muted" style="margin: 4px 0 0">
          Crie campos próprios para guardar as informações da Fix Pay em cada objeto do CRM.
        </p>
      </div>
      <button class="btn btn-primary" type="button" @click="openNew">+ Criar propriedade</button>
    </div>

    <div class="toolbar filters">
      <select v-model="entity" @change="selectEntity(entity)">
        <option v-for="(label, key) in entityLabels" :key="key" :value="key">{{ label }}</option>
      </select>
      <input v-model="search" type="search" placeholder="Pesquisar propriedades…" />
      <span class="muted">{{ filtered.length }} propriedade(s)</span>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>Grupo</th>
            <th>Tipo de campo</th>
            <th>Criado por</th>
            <th>Usado em</th>
            <th>Criada</th>
            <th style="width: 150px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in filtered" :key="p.id" style="cursor: default">
            <td>
              <strong>{{ p.label }}</strong>
              <div class="muted key">{{ p.key }}</div>
            </td>
            <td class="muted">{{ p.group_name }}</td>
            <td>{{ fieldTypeLabels[p.field_type] || p.field_type }}</td>
            <td :class="{ muted: !p.creator_name }">{{ p.creator_name || '—' }}</td>
            <td><span class="badge" :class="p.used_count ? 'blue' : 'gray'">{{ p.used_count }} registro(s)</span></td>
            <td class="muted">{{ formatDate(p.created_at) }}</td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="openEdit(p)">Editar</button>
              <button class="btn btn-danger btn-sm" type="button" @click="remove(p)">Excluir</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !filtered.length" class="empty-state">
        <strong>Nenhuma propriedade personalizada aqui</strong>
        Crie campos como "Número do EC", "Origem do lead" ou "Produtos contratados" para este objeto.
      </div>
    </div>

    <ModalDialog
      :title="editing ? 'Editar propriedade' : 'Criar propriedade'"
      :open="modalOpen"
      wide
      @close="modalOpen = false"
    >
      <form @submit.prevent="save">
        <div class="field">
          <label>Rótulo *</label>
          <input v-model="form.label" required placeholder="ex.: Número do EC" />
          <small class="muted" v-if="!editing">O nome interno é gerado automaticamente a partir do rótulo.</small>
          <small class="muted key" v-else>Nome interno: {{ editing.key }} (não pode ser alterado)</small>
        </div>

        <div class="field">
          <label>Descrição</label>
          <input v-model="form.description" placeholder="Explique para que serve este campo" />
        </div>

        <div class="field">
          <label>Grupo</label>
          <input v-model="form.group_name" placeholder="Informações personalizadas" />
        </div>

        <div class="field">
          <label>Escolha um tipo de campo *</label>
          <select v-model="form.field_type" :disabled="!!editing">
            <option v-for="t in fieldTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
          </select>
          <small class="muted">
            {{ fieldTypes.find((t) => t.value === form.field_type)?.hint }}
            <template v-if="editing"> · o tipo não pode ser alterado depois de criado.</template>
          </small>
        </div>

        <div class="field" v-if="needsOptions">
          <label>Opções *</label>
          <ul class="options-list">
            <li v-for="(o, i) in form.options" :key="i">
              <span class="opt-order">
                <button type="button" :disabled="i === 0" @click="moveOption(i, -1)">↑</button>
                <button type="button" :disabled="i === form.options.length - 1" @click="moveOption(i, 1)">↓</button>
              </span>
              <input v-model="o.label" placeholder="Rótulo (ex.: Link de pagamento)" />
              <input v-model="o.value" placeholder="valor interno (opcional)" class="opt-value" />
              <button type="button" class="opt-remove" title="Remover opção" @click="removeOption(i)">×</button>
            </li>
          </ul>
          <button type="button" class="add-option" @click="addOption">+ Adicionar opção</button>
        </div>

        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : editing ? 'Salvar alterações' : 'Criar propriedade' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.filters {
  margin-bottom: 14px;
}

.key {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.field small {
  font-size: 12px;
}

.options-list {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.options-list li {
  display: flex;
  align-items: center;
  gap: 6px;
}

.options-list input {
  flex: 1;
  padding: 7px 10px;
  border: 1px solid var(--fix-border);
  border-radius: 7px;
  font-size: 13px;
  outline: none;
}

.options-list input:focus {
  border-color: var(--fix-purple);
}

.opt-value {
  max-width: 190px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px !important;
}

.opt-order {
  display: flex;
  gap: 2px;
}

.opt-order button {
  border: 1px solid var(--fix-border);
  background: var(--fix-surface);
  border-radius: 5px;
  cursor: pointer;
  font-size: 10px;
  width: 20px;
  height: 20px;
}

.opt-order button:disabled {
  opacity: 0.3;
  cursor: default;
}

.opt-remove {
  border: none;
  background: none;
  color: var(--fix-red);
  font-size: 18px;
  cursor: pointer;
  line-height: 1;
  padding: 0 4px;
}

.add-option {
  border: 1px dashed var(--fix-border);
  background: none;
  border-radius: 8px;
  padding: 8px;
  width: 100%;
  color: var(--fix-purple);
  cursor: pointer;
  font-size: 13px;
}

.add-option:hover {
  border-color: var(--fix-purple);
  background: var(--fix-purple-tint);
}
</style>
