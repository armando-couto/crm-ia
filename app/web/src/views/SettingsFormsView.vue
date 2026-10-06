<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { formatDateTime } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import type { ContactList, PublicForm, PublicFormField, User } from '../types'

const toast = useToastStore()

const forms = ref<PublicForm[]>([])
const users = ref<User[]>([])
const lists = ref<ContactList[]>([])
const defaultFields = ref<PublicFormField[]>([])
const embedBase = ref('')
const loading = ref(true)
const saving = ref(false)

const editing = ref<PublicForm | null>(null)
const submissions = ref<{ form: PublicForm; items: any[] } | null>(null)

const fieldTypes = [
  { value: 'texto', label: 'Texto' },
  { value: 'email', label: 'E-mail' },
  { value: 'telefone', label: 'Telefone' },
  { value: 'textarea', label: 'Texto longo' },
  { value: 'selecao', label: 'Seleção' }
]

const stages = ['lead', 'mql', 'sql', 'oportunidade', 'cliente']

const embedCode = computed(() => {
  if (!editing.value?.slug) return ''
  return `<iframe src="${embedBase.value}/f/${editing.value.slug}" width="100%" height="620" frameborder="0" style="border:0"></iframe>`
})

const publicUrl = computed(() =>
  editing.value?.slug ? `${embedBase.value}/f/${editing.value.slug}` : ''
)

async function load() {
  loading.value = true
  try {
    const [resp, userList, listResp] = await Promise.all([
      api.get<{ data: PublicForm[]; default_fields: PublicFormField[]; embed_base: string }>('/forms'),
      api.get<User[]>('/users'),
      api.get<{ data: ContactList[] }>('/lists')
    ])
    forms.value = resp.data ?? []
    defaultFields.value = resp.default_fields ?? []
    embedBase.value = resp.embed_base ?? window.location.origin
    users.value = userList
    lists.value = listResp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function novo() {
  editing.value = {
    id: 0,
    slug: '',
    name: '',
    headline: 'Fale com a nossa equipe',
    description: '',
    fields: JSON.parse(JSON.stringify(defaultFields.value)),
    submit_label: 'Enviar',
    success_message: 'Recebemos seus dados. Em breve entraremos em contato.',
    redirect_url: '',
    owner_id: null,
    list_id: null,
    lifecycle_stage: 'lead',
    source: 'formulario',
    active: true,
    submissions: 0,
    created_by: null,
    created_at: '',
    updated_at: ''
  }
}

function edit(form: PublicForm) {
  editing.value = JSON.parse(JSON.stringify(form))
}

function addField() {
  editing.value?.fields.push({ key: '', label: '', type: 'texto', required: false })
}

function removeField(index: number) {
  editing.value?.fields.splice(index, 1)
}

function moveField(index: number, delta: number) {
  const fields = editing.value?.fields
  if (!fields) return
  const target = index + delta
  if (target < 0 || target >= fields.length) return
  const [item] = fields.splice(index, 1)
  fields.splice(target, 0, item)
}

/** As opções da seleção são editadas como uma linha separada por vírgula. */
function optionsText(field: PublicFormField): string {
  return (field.options ?? []).join(', ')
}

function setOptions(field: PublicFormField, value: string) {
  field.options = value
    .split(',')
    .map((o) => o.trim())
    .filter(Boolean)
}

async function save() {
  if (!editing.value) return
  saving.value = true
  try {
    const payload = { ...editing.value }
    if (editing.value.id) {
      await api.put(`/forms/${editing.value.id}`, payload)
    } else {
      await api.post('/forms', payload)
    }
    toast.push('Formulário salvo')
    editing.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove(form: PublicForm) {
  if (!confirm(`Excluir o formulário "${form.name}"? Os envios já recebidos também somem.`)) return
  try {
    await api.delete(`/forms/${form.id}`)
    toast.push('Formulário removido')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function openSubmissions(form: PublicForm) {
  try {
    const resp = await api.get<{ data: any[] }>(`/forms/${form.id}/submissions`)
    submissions.value = { form, items: resp.data ?? [] }
  } catch (e: any) {
    toast.error(e.message)
  }
}

function copy(text: string) {
  navigator.clipboard?.writeText(text)
  toast.push('Copiado')
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Formulários</h1>
        <p class="muted" style="margin: 4px 0 0">
          Crie um formulário, embuta no site e cada envio vira contato no CRM.
        </p>
      </div>
      <button class="btn btn-primary" type="button" @click="novo">Novo formulário</button>
    </div>

    <p v-if="loading" class="muted">Carregando formulários…</p>

    <div v-else-if="!forms.length" class="card empty">
      <h2>Nenhum formulário ainda</h2>
      <p class="muted">
        Um formulário publicado captura leads direto do site, sem ninguém digitar nada aqui dentro.
      </p>
    </div>

    <div v-else class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th style="width: 200px">Endereço</th>
            <th style="width: 130px">Dono do lead</th>
            <th style="width: 100px">Envios</th>
            <th style="width: 100px">Situação</th>
            <th style="width: 190px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="f in forms" :key="f.id">
            <td><strong>{{ f.name }}</strong></td>
            <td class="slug">/f/{{ f.slug }}</td>
            <td class="muted">{{ f.owner_name || '—' }}</td>
            <td>{{ f.submissions }}</td>
            <td>
              <span class="badge" :class="f.active ? 'green' : 'gray'">
                {{ f.active ? 'ativo' : 'pausado' }}
              </span>
            </td>
            <td class="actions">
              <button class="btn btn-sm" type="button" @click="openSubmissions(f)">Envios</button>
              <button class="btn btn-sm" type="button" @click="edit(f)">Editar</button>
              <button class="btn btn-sm danger" type="button" @click="remove(f)">Excluir</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- ===== Editor ===== -->
    <ModalDialog
      :title="editing?.id ? 'Editar formulário' : 'Novo formulário'"
      :open="!!editing"
      wide
      @close="editing = null"
    >
      <template v-if="editing">
        <div class="form-row">
          <div class="field">
            <label>Nome interno *</label>
            <input v-model="editing.name" placeholder="Contato do site" />
          </div>
          <div class="field">
            <label>Endereço (slug)</label>
            <input v-model="editing.slug" placeholder="gerado a partir do nome" />
          </div>
        </div>

        <div class="field">
          <label>Título exibido</label>
          <input v-model="editing.headline" placeholder="Fale com a nossa equipe" />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="editing.description" rows="2"></textarea>
        </div>

        <h3 class="section">Campos</h3>
        <div v-for="(field, i) in editing.fields" :key="i" class="field-row">
          <div class="field-grid">
            <input v-model="field.label" placeholder="Rótulo" />
            <input v-model="field.key" placeholder="chave" class="mono" />
            <select v-model="field.type">
              <option v-for="t in fieldTypes" :key="t.value" :value="t.value">{{ t.label }}</option>
            </select>
            <label class="inline">
              <input type="checkbox" v-model="field.required" /> obrigatório
            </label>
          </div>
          <div v-if="field.type === 'selecao'" class="field">
            <input
              :value="optionsText(field)"
              placeholder="Opções separadas por vírgula"
              @input="setOptions(field, ($event.target as HTMLInputElement).value)"
            />
          </div>
          <div class="field-actions">
            <button class="icon-btn" type="button" title="Subir" @click="moveField(i, -1)">↑</button>
            <button class="icon-btn" type="button" title="Descer" @click="moveField(i, 1)">↓</button>
            <button class="icon-btn danger" type="button" title="Remover" @click="removeField(i)">✕</button>
          </div>
        </div>
        <button class="btn btn-sm" type="button" @click="addField">+ Adicionar campo</button>
        <p class="muted hint">
          As chaves <code>first_name</code>, <code>last_name</code>, <code>email</code>,
          <code>phone</code> e <code>job_title</code> preenchem o cadastro do contato.
          O resto vira observação na timeline. O campo <code>email</code> é obrigatório.
        </p>

        <h3 class="section">O que fazer com o lead</h3>
        <div class="form-row">
          <div class="field">
            <label>Dono</label>
            <select v-model="editing.owner_id">
              <option :value="null">Sem dono</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Adicionar à lista</label>
            <select v-model="editing.list_id">
              <option :value="null">Nenhuma</option>
              <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Estágio</label>
            <select v-model="editing.lifecycle_stage">
              <option v-for="s in stages" :key="s" :value="s">{{ s }}</option>
            </select>
          </div>
          <div class="field">
            <label>Origem</label>
            <input v-model="editing.source" placeholder="formulario" />
          </div>
        </div>

        <h3 class="section">Depois do envio</h3>
        <div class="field">
          <label>Mensagem de sucesso</label>
          <input v-model="editing.success_message" />
        </div>
        <div class="field">
          <label>Ou redirecionar para</label>
          <input v-model="editing.redirect_url" placeholder="https://suaempresa.com.br/obrigado" />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Texto do botão</label>
            <input v-model="editing.submit_label" />
          </div>
          <div class="field">
            <label>Situação</label>
            <select v-model="editing.active">
              <option :value="true">Ativo</option>
              <option :value="false">Pausado</option>
            </select>
          </div>
        </div>

        <template v-if="editing.id">
          <h3 class="section">Publicar no site</h3>
          <div class="field">
            <label>Link direto</label>
            <div class="copy-row">
              <input :value="publicUrl" readonly class="mono" />
              <button class="btn btn-sm" type="button" @click="copy(publicUrl)">Copiar</button>
            </div>
          </div>
          <div class="field">
            <label>Código para embutir</label>
            <div class="copy-row">
              <textarea :value="embedCode" readonly rows="3" class="mono"></textarea>
              <button class="btn btn-sm" type="button" @click="copy(embedCode)">Copiar</button>
            </div>
          </div>
        </template>
      </template>

      <template #footer>
        <button class="btn" type="button" @click="editing = null">Cancelar</button>
        <button class="btn btn-primary" type="button" :disabled="saving" @click="save">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
      </template>
    </ModalDialog>

    <!-- ===== Envios recebidos ===== -->
    <ModalDialog
      :title="`Envios de ${submissions?.form.name ?? ''}`"
      :open="!!submissions"
      wide
      @close="submissions = null"
    >
      <template v-if="submissions">
        <p v-if="!submissions.items.length" class="muted">Nenhum envio ainda.</p>
        <ul v-else class="submissions">
          <li v-for="s in submissions.items" :key="s.id">
            <div class="sub-head">
              <router-link v-if="s.contact_id" :to="`/contatos/${s.contact_id}`">
                Ver contato #{{ s.contact_id }}
              </router-link>
              <span class="muted">{{ formatDateTime(s.created_at) }}</span>
            </div>
            <dl>
              <template v-for="(value, key) in s.payload" :key="key">
                <dt v-if="value">{{ key }}</dt>
                <dd v-if="value">{{ value }}</dd>
              </template>
            </dl>
          </li>
        </ul>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.empty {
  text-align: center;
  padding: 36px;
}

.empty h2 {
  font-size: 16px;
  margin-bottom: 6px;
}

.slug,
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}

.actions {
  display: flex;
  gap: 6px;
  justify-content: flex-end;
}

.danger {
  color: var(--ci-red);
}

.section {
  font-size: 14px;
  margin: 20px 0 12px;
  padding-top: 14px;
  border-top: 1px solid var(--ci-border);
}

.field-row {
  border: 1px solid var(--ci-border);
  border-radius: 10px;
  padding: 10px;
  margin-bottom: 8px;
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.field-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1.4fr 1fr 1fr auto;
  gap: 8px;
  align-items: center;
}

@media (max-width: 700px) {
  .field-grid {
    grid-template-columns: 1fr;
  }
}

.inline {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  white-space: nowrap;
}

.field-actions {
  display: flex;
  gap: 2px;
}

.icon-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--ci-text-3);
  padding: 4px 6px;
  border-radius: 6px;
}

.icon-btn:hover {
  background: var(--ci-bg);
}

.icon-btn.danger:hover {
  color: var(--ci-red);
}

.hint {
  font-size: 12px;
  margin-top: 8px;
}

.copy-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.copy-row input,
.copy-row textarea {
  flex: 1;
}

.submissions {
  list-style: none;
  margin: 0;
  padding: 0;
}

.submissions li {
  border-top: 1px solid var(--ci-border);
  padding: 12px 0;
}

.submissions li:first-child {
  border-top: none;
}

.sub-head {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  margin-bottom: 6px;
}

.submissions dl {
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: 3px 12px;
  margin: 0;
  font-size: 13px;
}

.submissions dt {
  color: var(--ci-text-3);
}

.submissions dd {
  margin: 0;
  white-space: pre-wrap;
}
</style>
