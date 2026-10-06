<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatDate, relativeDate } from '../format'
import type { Contact, MessageTemplate, Paginated, Sequence, SequenceMember, SequenceStep, User } from '../types'

const props = defineProps<{ sequenceId: number }>()
const emit = defineEmits<{ back: [] }>()

const auth = useAuthStore()
const toast = useToastStore()

type TabKey = 'etapas' | 'inscritos' | 'configuracoes' | 'automatizar'
const tab = ref<TabKey>('etapas')

const seq = ref<Sequence | null>(null)
const members = ref<SequenceMember[]>([])
const templates = ref<MessageTemplate[]>([])
const users = ref<User[]>([])
const loading = ref(true)
const saving = ref(false)

// Inscrição de contatos
const enrollOpen = ref(false)
const searchTerm = ref('')
const searchResults = ref<Contact[]>([])
const selected = ref<number[]>([])

const canManage = computed(() => auth.can('automations.manage'))

const stepKinds = [
  { key: 'email_auto', label: 'E-mail automático', icon: '✉', auto: true },
  { key: 'task_email', label: 'Tarefa de e-mail manual', icon: '✎', auto: false },
  { key: 'task_call', label: 'Tarefa de chamada', icon: '☎', auto: false },
  { key: 'task_general', label: 'Tarefa geral', icon: '✓', auto: false },
  { key: 'task_linkedin', label: 'Tarefa no LinkedIn', icon: 'in', auto: false }
]

const statusLabels: Record<string, string> = {
  ativa: 'em andamento',
  aguardando_tarefa: 'aguardando tarefa',
  concluida: 'concluída',
  cancelada: 'saiu'
}

const exitLabels: Record<string, string> = {
  respondeu: 'respondeu',
  reuniao: 'agendou reunião',
  manual: 'removido',
  fim: 'terminou a cadência'
}

function kindOf(kind: string) {
  return stepKinds.find((k) => k.key === kind) ?? stepKinds[0]
}

async function load() {
  loading.value = true
  try {
    if (props.sequenceId) {
      seq.value = await api.get<Sequence>(`/sequences/${props.sequenceId}`)
      const resp = await api.get<{ data: SequenceMember[] }>(`/sequences/${props.sequenceId}/members`)
      members.value = resp.data ?? []
    } else {
      seq.value = {
        id: 0,
        name: '',
        description: '',
        steps: [{ kind: 'email_auto', delay_days: 0, subject: '', body: '' }],
        active: true,
        dynamic: false,
        exit_on_reply: true,
        exit_on_meeting: true,
        owner_id: auth.user?.id ?? null,
        created_by: null,
        created_at: '',
        updated_at: '',
        enrolled: 0,
        active_members: 0,
        open_rate: 0,
        reply_rate: 0
      }
      members.value = []
    }
    templates.value = await api.get<MessageTemplate[]>('/templates')
    users.value = await api.get<User[]>('/users')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function addStep(kind: string) {
  seq.value?.steps.push({
    kind,
    delay_days: seq.value.steps.length ? 2 : 0,
    subject: '',
    body: '',
    title: '',
    note: ''
  } as SequenceStep)
}

function removeStep(index: number) {
  seq.value?.steps.splice(index, 1)
}

function moveStep(index: number, delta: number) {
  const steps = seq.value?.steps
  if (!steps) return
  const target = index + delta
  if (target < 0 || target >= steps.length) return
  const [item] = steps.splice(index, 1)
  steps.splice(target, 0, item)
}

async function save() {
  if (!seq.value) return
  saving.value = true
  try {
    if (seq.value.id) {
      seq.value = await api.put<Sequence>(`/sequences/${seq.value.id}`, seq.value)
    } else {
      seq.value = await api.post<Sequence>('/sequences', seq.value)
    }
    toast.push('Sequência salva')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function searchContacts() {
  if (searchTerm.value.trim().length < 2) return
  try {
    const resp = await api.get<Paginated<Contact>>(`/contacts?q=${encodeURIComponent(searchTerm.value)}&per_page=10`)
    searchResults.value = resp.data ?? []
  } catch (e: any) {
    toast.error(e.message)
  }
}

function toggleSelect(id: number) {
  const i = selected.value.indexOf(id)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(id)
}

async function enroll() {
  if (!seq.value?.id || !selected.value.length) return
  try {
    const resp = await api.post<{ enrolled: number; skipped: string[] }>(
      `/sequences/${seq.value.id}/enroll`,
      { contact_ids: selected.value }
    )
    toast.push(`${resp.enrolled} contato(s) inscritos`)
    if (resp.skipped?.length) toast.error(resp.skipped[0])
    enrollOpen.value = false
    selected.value = []
    searchResults.value = []
    searchTerm.value = ''
    await load()
    tab.value = 'inscritos'
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function unenroll(member: SequenceMember) {
  if (!confirm(`Tirar ${member.contact_name} da sequência?`)) return
  try {
    await api.delete(`/sequences/${seq.value!.id}/members/${member.id}`)
    toast.push('Contato removido da sequência')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="editor-head">
      <button class="back" type="button" @click="emit('back')">‹ Sequências</button>
      <input
        v-if="seq"
        v-model="seq.name"
        class="name-input"
        placeholder="Nome da sequência"
        :disabled="!canManage"
      />
      <div class="head-actions">
        <button
          v-if="canManage && seq?.id"
          class="btn"
          type="button"
          @click="enrollOpen = true"
        >
          Inscrever contatos
        </button>
        <button v-if="canManage" class="btn btn-primary" type="button" :disabled="saving" @click="save">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
      </div>
    </div>

    <nav class="seq-tabs">
      <button type="button" :class="{ active: tab === 'etapas' }" @click="tab = 'etapas'">Etapas</button>
      <button type="button" :class="{ active: tab === 'inscritos' }" @click="tab = 'inscritos'">
        Inscritos <span v-if="members.length">({{ members.length }})</span>
      </button>
      <button type="button" :class="{ active: tab === 'configuracoes' }" @click="tab = 'configuracoes'">
        Configurações
      </button>
      <button type="button" :class="{ active: tab === 'automatizar' }" @click="tab = 'automatizar'">
        Automatizar
      </button>
    </nav>

    <p v-if="loading" class="muted">Carregando…</p>

    <!-- ===== Etapas ===== -->
    <template v-else-if="tab === 'etapas' && seq">
      <div class="steps">
        <div v-for="(step, i) in seq.steps" :key="i" class="step-wrap">
          <div v-if="i > 0" class="delay">
            <span class="delay-line"></span>
            <label>
              aguardar
              <input v-model.number="step.delay_days" type="number" min="0" max="365" :disabled="!canManage" />
              dia(s)
            </label>
            <span class="delay-line"></span>
          </div>

          <div class="card step" :class="{ manual: kindOf(step.kind) && !kindOf(step.kind).auto }">
            <div class="step-head">
              <span class="step-num">{{ i + 1 }}</span>
              <span class="step-kind" :class="{ auto: kindOf(step.kind).auto }">
                {{ kindOf(step.kind).icon }} {{ kindOf(step.kind).label }}
              </span>
              <span v-if="!kindOf(step.kind).auto" class="badge amber">pausa a cadência</span>
              <div v-if="canManage" class="step-actions">
                <button class="icon-btn" type="button" title="Subir" @click="moveStep(i, -1)">↑</button>
                <button class="icon-btn" type="button" title="Descer" @click="moveStep(i, 1)">↓</button>
                <button class="icon-btn danger" type="button" title="Remover" @click="removeStep(i)">✕</button>
              </div>
            </div>

            <template v-if="step.kind === 'email_auto'">
              <div class="field">
                <label>Modelo da biblioteca</label>
                <select v-model.number="step.template_id" :disabled="!canManage">
                  <option :value="0">Escrever aqui mesmo</option>
                  <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
                </select>
              </div>
              <template v-if="!step.template_id">
                <div class="field">
                  <label>Assunto</label>
                  <input v-model="step.subject" placeholder="Olá {{nome}}, tudo bem?" :disabled="!canManage" />
                </div>
                <div class="field">
                  <label>Mensagem</label>
                  <textarea v-model="step.body" rows="4" :disabled="!canManage"></textarea>
                </div>
              </template>
            </template>

            <template v-else>
              <div class="field">
                <label>Título da tarefa</label>
                <input v-model="step.title" :placeholder="kindOf(step.kind).label" :disabled="!canManage" />
              </div>
              <div class="field">
                <label>Instruções para quem vai executar</label>
                <textarea v-model="step.note" rows="2" :disabled="!canManage"></textarea>
              </div>
              <p class="muted hint">
                Esta etapa cria uma tarefa na fila do dono da sequência e a cadência só continua
                quando a tarefa for concluída.
              </p>
            </template>
          </div>
        </div>
      </div>

      <div v-if="canManage" class="add-step card">
        <span class="muted">Adicionar etapa:</span>
        <button
          v-for="k in stepKinds"
          :key="k.key"
          class="btn btn-sm"
          type="button"
          @click="addStep(k.key)"
        >
          {{ k.icon }} {{ k.label }}
        </button>
      </div>
    </template>

    <!-- ===== Inscritos ===== -->
    <template v-else-if="tab === 'inscritos' && seq">
      <div v-if="!members.length" class="card empty">
        <p class="muted">
          Ninguém inscrito ainda.
          <template v-if="seq.id"> Use "Inscrever contatos" para começar.</template>
          <template v-else> Salve a sequência antes de inscrever contatos.</template>
        </p>
      </div>
      <div v-else class="table-wrap">
        <table class="data">
          <thead>
            <tr>
              <th>Contato</th>
              <th style="width: 200px">Etapa atual</th>
              <th style="width: 150px">Situação</th>
              <th style="width: 130px">Inscrito</th>
              <th style="width: 90px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in members" :key="m.id">
              <td>
                <router-link :to="`/contatos/${m.contact_id}`"><strong>{{ m.contact_name }}</strong></router-link>
                <div class="muted small">{{ m.contact_email }}</div>
              </td>
              <td>
                <template v-if="m.status === 'ativa' || m.status === 'aguardando_tarefa'">
                  {{ m.step + 1 }} de {{ seq.steps.length }}
                  <div v-if="m.step_label" class="muted small">{{ m.step_label }}</div>
                </template>
                <span v-else class="muted">—</span>
              </td>
              <td>
                <span
                  class="badge"
                  :class="m.status === 'concluida' ? 'green' : m.status === 'cancelada' ? 'gray' : m.status === 'aguardando_tarefa' ? 'amber' : 'blue'"
                >
                  {{ statusLabels[m.status] }}
                </span>
                <div v-if="m.exit_reason" class="muted small">{{ exitLabels[m.exit_reason] }}</div>
              </td>
              <td class="muted">{{ formatDate(m.enrolled_at) }}</td>
              <td>
                <button
                  v-if="canManage && (m.status === 'ativa' || m.status === 'aguardando_tarefa')"
                  class="btn btn-sm"
                  type="button"
                  @click="unenroll(m)"
                >
                  Tirar
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- ===== Configurações ===== -->
    <template v-else-if="tab === 'configuracoes' && seq">
      <div class="card settings">
        <div class="field">
          <label>Descrição</label>
          <input v-model="seq.description" placeholder="Para que serve esta cadência" :disabled="!canManage" />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Dono (recebe as tarefas manuais)</label>
            <select v-model="seq.owner_id" :disabled="!canManage">
              <option :value="null">Dono do contato</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Situação</label>
            <select v-model="seq.active" :disabled="!canManage">
              <option :value="true">Ativa</option>
              <option :value="false">Pausada</option>
            </select>
          </div>
        </div>

        <label class="toggle">
          <input type="checkbox" v-model="seq.dynamic" :disabled="!canManage" />
          <div>
            <strong>Sequência dinâmica</strong>
            <p class="muted">
              Enquanto o contato não engaja, seguem os e-mails automáticos. Assim que ele abre,
              clica ou responde, a cadência pula direto para as etapas manuais — a ligação vai
              para a sua fila em vez de mais um e-mail.
            </p>
          </div>
        </label>
      </div>
    </template>

    <!-- ===== Automatizar ===== -->
    <template v-else-if="tab === 'automatizar' && seq">
      <div class="card settings">
        <p class="muted" style="margin-top: 0">
          Escolha o que tira o contato da sequência automaticamente.
        </p>

        <label class="toggle">
          <input type="checkbox" v-model="seq.exit_on_reply" :disabled="!canManage" />
          <div>
            <strong>Quando o contato responde a qualquer e-mail</strong>
            <p class="muted">Cancelar a inscrição — a conversa continua na caixa de entrada.</p>
          </div>
        </label>

        <label class="toggle">
          <input type="checkbox" v-model="seq.exit_on_meeting" :disabled="!canManage" />
          <div>
            <strong>Quando uma reunião é agendada com o contato</strong>
            <p class="muted">Cancelar a inscrição — o objetivo da cadência foi cumprido.</p>
          </div>
        </label>
      </div>
    </template>

    <!-- ===== Inscrever contatos ===== -->
    <div v-if="enrollOpen" class="overlay" @mousedown.self="enrollOpen = false">
      <div class="modal">
        <header>
          <h3>Inscrever contatos</h3>
          <button type="button" class="close" @click="enrollOpen = false">×</button>
        </header>
        <div class="modal-body">
          <div class="field">
            <input
              v-model="searchTerm"
              placeholder="Buscar por nome ou e-mail…"
              @keyup.enter="searchContacts"
            />
          </div>
          <button class="btn" type="button" @click="searchContacts">Buscar</button>

          <ul class="results">
            <li v-for="c in searchResults" :key="c.id">
              <label>
                <input type="checkbox" :checked="selected.includes(c.id)" @change="toggleSelect(c.id)" />
                <span>
                  <strong>{{ c.first_name }} {{ c.last_name }}</strong>
                  <span class="muted small"> · {{ c.email || 'sem e-mail' }}</span>
                </span>
              </label>
            </li>
          </ul>

          <button
            class="btn btn-primary"
            type="button"
            style="width: 100%; justify-content: center"
            :disabled="!selected.length"
            @click="enroll"
          >
            Inscrever {{ selected.length }} contato(s)
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.editor-head {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 8px;
}

.back {
  background: none;
  border: none;
  color: var(--fix-purple-dark);
  cursor: pointer;
  font-size: 14px;
  white-space: nowrap;
}

.name-input {
  flex: 1;
  border: none;
  background: transparent;
  font-size: 20px;
  font-weight: 600;
  outline: none;
  padding: 4px 0;
  border-bottom: 2px solid transparent;
}

.name-input:focus {
  border-bottom-color: var(--fix-purple);
}

.head-actions {
  display: flex;
  gap: 8px;
}

.seq-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 20px;
}

.seq-tabs button {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 10px 14px;
  font-size: 14px;
  color: var(--fix-text-2);
  cursor: pointer;
}

.seq-tabs button.active {
  color: var(--fix-purple-dark);
  border-bottom-color: var(--fix-purple);
  font-weight: 500;
}

.steps {
  max-width: 640px;
  margin: 0 auto;
}

.delay {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 0;
  justify-content: center;
}

.delay label {
  font-size: 13px;
  color: var(--fix-text-3);
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.delay input {
  width: 60px;
  padding: 4px 8px;
  text-align: center;
}

.delay-line {
  flex: 1;
  height: 1px;
  background: var(--fix-border);
}

.step {
  border-left: 3px solid var(--fix-purple);
}

.step.manual {
  border-left-color: var(--fix-amber);
}

.step-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.step-num {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--fix-purple);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}

.step-kind {
  font-weight: 600;
  font-size: 14px;
  flex: 1;
}

.step-actions {
  display: flex;
  gap: 2px;
}

.icon-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--fix-text-3);
  padding: 4px 6px;
  border-radius: 6px;
}

.icon-btn:hover {
  background: var(--fix-bg);
}

.icon-btn.danger:hover {
  color: var(--fix-red);
}

.hint {
  font-size: 12px;
  margin: 8px 0 0;
}

.add-step {
  max-width: 640px;
  margin: 14px auto 0;
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.empty {
  text-align: center;
  padding: 32px;
}

.small {
  font-size: 12px;
}

.settings {
  max-width: 640px;
}

.toggle {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 14px 0;
  border-top: 1px solid var(--fix-border);
  cursor: pointer;
}

.toggle input {
  margin-top: 3px;
}

.toggle p {
  margin: 4px 0 0;
  font-size: 13px;
}

.overlay {
  position: fixed;
  inset: 0;
  background: rgba(34, 20, 60, 0.45);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 8vh 16px;
  z-index: 150;
}

.modal {
  background: var(--fix-surface);
  border-radius: 14px;
  width: 100%;
  max-width: 440px;
  box-shadow: var(--shadow-lg);
}

.modal header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 18px 22px 0;
}

.modal h3 {
  font-size: 17px;
}

.close {
  border: none;
  background: none;
  font-size: 22px;
  cursor: pointer;
  color: var(--fix-text-3);
}

.modal-body {
  padding: 14px 22px 22px;
}

.results {
  list-style: none;
  margin: 12px 0;
  padding: 0;
  max-height: 260px;
  overflow-y: auto;
}

.results li {
  padding: 8px 0;
  border-top: 1px solid var(--fix-border);
}

.results label {
  display: flex;
  gap: 10px;
  align-items: center;
  cursor: pointer;
}
</style>
