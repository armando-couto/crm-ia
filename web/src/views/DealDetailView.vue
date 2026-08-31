<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, formatDateTime, formatMoney, initials, relativeDate } from '../format'
import { useToastStore } from '../stores/toast'
import CustomProperties from '../components/CustomProperties.vue'
import AttachmentsPanel from '../components/AttachmentsPanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Activity, Company, Contact, Deal, Meeting, Paginated, Pipeline, Task, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const deal = ref<Deal | null>(null)
const pipelines = ref<Pipeline[]>([])
const users = ref<User[]>([])
const contacts = ref<Contact[]>([])
const companies = ref<Company[]>([])
const tasks = ref<Task[]>([])
const meetings = ref<Meeting[]>([])
const dealContact = ref<Contact | null>(null)

const centerTab = ref<'visao' | 'atividades'>('visao')

const pipeline = computed(() => pipelines.value.find((p) => p.id === deal.value?.pipeline_id))
const stages = computed(() => pipeline.value?.stages.filter((s) => !s.is_won && !s.is_lost) ?? [])
const pendingTasks = computed(() => tasks.value.filter((t) => !t.completed_at))

const temperatureLabels: Record<string, string> = { quente: '🔴 Quente', media: '🟡 Média', fria: '🔵 Fria' }

// ===== Central de atividades =====
type ActivityTab = 'todas' | 'nota' | 'email' | 'ligacao' | 'tarefas' | 'reunioes'
const activityTab = ref<ActivityTab>('todas')
const activities = ref<Activity[]>([])
const activitySearch = ref('')
const loadingActivities = ref(false)
const noteContent = ref('')

const activityTabs: { key: ActivityTab; label: string }[] = [
  { key: 'todas', label: 'Todas as atividades' },
  { key: 'nota', label: 'Observações' },
  { key: 'email', label: 'E-mails' },
  { key: 'ligacao', label: 'Chamadas' },
  { key: 'tarefas', label: 'Tarefas' },
  { key: 'reunioes', label: 'Reuniões' }
]

let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(activitySearch, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(loadActivities, 300)
})

async function loadActivities() {
  if (activityTab.value === 'tarefas' || activityTab.value === 'reunioes') return
  loadingActivities.value = true
  try {
    const params = new URLSearchParams({ deal_id: String(id), limit: '200' })
    if (activityTab.value !== 'todas') params.set('kind', activityTab.value)
    if (activitySearch.value.trim()) params.set('q', activitySearch.value.trim())
    const resp = await api.get<{ data: Activity[] }>(`/activities?${params}`)
    activities.value = resp.data
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loadingActivities.value = false
  }
}

function selectActivityTab(tab: ActivityTab) {
  activityTab.value = tab
  activitySearch.value = ''
  loadActivities()
}

const monthNames = ['Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho', 'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro']

const groupedActivities = computed(() => {
  const groups: { month: string; items: Activity[] }[] = []
  for (const a of activities.value) {
    const d = new Date(a.created_at)
    const month = `${monthNames[d.getMonth()]} ${d.getFullYear()}`
    const last = groups[groups.length - 1]
    if (last && last.month === month) {
      last.items.push(a)
    } else {
      groups.push({ month, items: [a] })
    }
  }
  return groups
})

async function addNote() {
  if (!noteContent.value.trim()) return
  try {
    await api.post('/activities', {
      kind: 'nota',
      content: noteContent.value.trim(),
      deal_id: id,
      contact_id: deal.value?.contact_id || null
    })
    noteContent.value = ''
    toast.push('Observação registrada')
    await loadActivities()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Carregamento =====
async function load() {
  try {
    deal.value = await api.get<Deal>(`/deals/${id}`)
    const [tasksResp, meetingsResp] = await Promise.all([
      api.get<Paginated<Task>>(`/tasks?deal_id=${id}&per_page=50`),
      api.get<Paginated<Meeting>>(`/meetings?deal_id=${id}&per_page=50`)
    ])
    tasks.value = tasksResp.data
    meetings.value = meetingsResp.data
    if (deal.value.contact_id) {
      dealContact.value = await api.get<Contact>(`/contacts/${deal.value.contact_id}`)
    } else {
      dealContact.value = null
    }
  } catch (e: any) {
    toast.error(e.message)
    router.push('/negocios')
  }
}

async function refreshAll() {
  await Promise.all([load(), loadActivities()])
}

// ===== Etapas / fechamento =====
async function moveTo(stageId: number) {
  if (!deal.value || deal.value.stage_id === stageId) return
  try {
    deal.value = await api.patch<Deal>(`/deals/${id}/stage`, { stage_id: stageId, position: 0 })
    toast.push('Etapa atualizada')
    await loadActivities()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function close(won: boolean) {
  try {
    deal.value = await api.patch<Deal>(`/deals/${id}/close`, { won })
    toast.push(won ? 'Negócio ganho! 🎉' : 'Negócio marcado como perdido')
    await loadActivities()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Modais =====
const editOpen = ref(false)
const callOpen = ref(false)
const taskOpen = ref(false)
const meetingOpen = ref(false)
const emailOpen = ref(false)
const saving = ref(false)

const form = ref<any>({})
const callForm = ref({ direction: 'saida', outcome: 'conectada', duration_seconds: 0, notes: '' })
const taskForm = ref({ title: '', type: 'tarefa', priority: 'media', due_date: '' })
const meetingForm = ref({ title: '', starts_at: '', location: '' })
const emailForm = ref({ subject: '', body: '' })

function openEdit() {
  if (!deal.value) return
  form.value = {
    name: deal.value.name,
    amount: deal.value.amount,
    contact_id: deal.value.contact_id,
    company_id: deal.value.company_id,
    owner_id: deal.value.owner_id,
    temperature: deal.value.temperature || '',
    close_date: deal.value.close_date ? deal.value.close_date.slice(0, 10) : ''
  }
  editOpen.value = true
}

async function saveEdit() {
  saving.value = true
  try {
    deal.value = await api.put<Deal>(`/deals/${id}`, { ...form.value, amount: Number(form.value.amount) || 0 })
    toast.push('Negócio atualizado')
    editOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!confirm('Remover este negócio?')) return
  try {
    await api.delete(`/deals/${id}`)
    toast.push('Negócio removido')
    router.push('/negocios')
  } catch (e: any) {
    toast.error(e.message)
  }
}

function quickNote() {
  centerTab.value = 'atividades'
  selectActivityTab('nota')
}

function openCall() {
  callForm.value = { direction: 'saida', outcome: 'conectada', duration_seconds: 0, notes: '' }
  callOpen.value = true
}

function openTask() {
  taskForm.value = { title: '', type: 'tarefa', priority: 'media', due_date: '' }
  taskOpen.value = true
}

function openMeeting() {
  meetingForm.value = { title: '', starts_at: '', location: '' }
  meetingOpen.value = true
}

function openEmail() {
  emailForm.value = { subject: '', body: '' }
  emailOpen.value = true
}

async function post(path: string, payload: Record<string, unknown>, message: string, closeFn: () => void) {
  saving.value = true
  try {
    await api.post(path, payload)
    toast.push(message)
    closeFn()
    await refreshAll()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

const saveCall = () =>
  post(
    '/calls',
    { ...callForm.value, contact_id: deal.value?.contact_id, company_id: deal.value?.company_id, deal_id: id },
    'Chamada registrada',
    () => (callOpen.value = false)
  )
const saveTask = () =>
  post('/tasks', { ...taskForm.value, deal_id: id, contact_id: deal.value?.contact_id }, 'Tarefa criada', () => (taskOpen.value = false))
const saveMeeting = () =>
  post(
    '/meetings',
    { ...meetingForm.value, deal_id: id, contact_id: deal.value?.contact_id, company_id: deal.value?.company_id },
    'Reunião agendada',
    () => (meetingOpen.value = false)
  )
const sendEmail = () =>
  post('/emails', { ...emailForm.value, contact_id: deal.value?.contact_id, deal_id: id }, 'E-mail enviado', () => (emailOpen.value = false))

async function toggleTask(task: Task) {
  try {
    const updated = await api.patch<Task>(`/tasks/${task.id}/toggle`, { done: !task.completed_at })
    tasks.value = tasks.value.map((t) => (t.id === task.id ? updated : t))
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(async () => {
  await Promise.all([load(), loadActivities()])
  try {
    const [pipelinesResp, usersResp, contactsResp, companiesResp] = await Promise.all([
      api.get<Pipeline[]>('/pipelines'),
      api.get<User[]>('/users'),
      api.get<Paginated<Contact>>('/contacts?per_page=100'),
      api.get<Paginated<Company>>('/companies?per_page=100')
    ])
    pipelines.value = pipelinesResp
    users.value = usersResp
    contacts.value = contactsResp.data ?? []
    companies.value = companiesResp.data ?? []
  } catch {
    /* opcional */
  }
})

const kindIcons: Record<string, string> = { nota: '✎', email: '✉', ligacao: '☎', reuniao: '⚑', sistema: '⚙' }
const kindLabels: Record<string, string> = { nota: 'Observação', email: 'E-mail', ligacao: 'Chamada', reuniao: 'Reunião', sistema: 'Sistema' }
const meetingBadge: Record<string, string> = { agendada: 'blue', realizada: 'green', cancelada: 'gray', nao_compareceu: 'red' }
</script>

<template>
  <div class="record" v-if="deal">
    <!-- ===== Coluna esquerda ===== -->
    <aside class="left">
      <router-link to="/negocios" class="back">‹ Negócios</router-link>

      <div class="profile">
        <h1>{{ deal.name }}</h1>
        <span class="deal-amount">{{ formatMoney(deal.amount) }}</span>
        <div class="badges">
          <span class="badge" :class="deal.status === 'ganho' ? 'green' : deal.status === 'perdido' ? 'red' : 'blue'">
            {{ deal.status === 'aberto' ? deal.stage_name : deal.status }}
          </span>
          <span v-if="deal.temperature" class="badge amber">{{ temperatureLabels[deal.temperature] }}</span>
        </div>
      </div>

      <div class="quick-actions">
        <button type="button" @click="quickNote"><span>✎</span>Observ.</button>
        <button type="button" :disabled="!dealContact?.email" @click="openEmail"><span>✉</span>E-mail</button>
        <button type="button" :disabled="!deal.contact_id" @click="openCall"><span>☎</span>Chamada</button>
        <button type="button" @click="openTask"><span>✓</span>Tarefa</button>
        <button type="button" @click="openMeeting"><span>⚑</span>Reunião</button>
      </div>

      <div class="close-actions" v-if="deal.status === 'aberto'">
        <button class="btn btn-outline" style="color: var(--fix-green)" @click="close(true)">✓ Ganho</button>
        <button class="btn btn-outline" style="color: var(--fix-red)" @click="close(false)">✕ Perdido</button>
      </div>

      <div class="about card">
        <div class="about-head">
          <h2>Sobre esse negócio</h2>
          <div>
            <button class="btn btn-outline btn-sm" type="button" @click="openEdit">Editar</button>
            <button class="btn btn-danger btn-sm" type="button" @click="remove">✕</button>
          </div>
        </div>
        <dl>
          <dt>Pipeline</dt>
          <dd>{{ pipeline?.name || '—' }}</dd>
          <dt>Etapa do negócio</dt>
          <dd>{{ deal.stage_name }}</dd>
          <dt>Valor</dt>
          <dd>{{ formatMoney(deal.amount) }}</dd>
          <dt>Temperatura do deal</dt>
          <dd>{{ deal.temperature ? temperatureLabels[deal.temperature] : '—' }}</dd>
          <dt>Proprietário do negócio</dt>
          <dd :class="{ muted: !deal.owner_name }">{{ deal.owner_name || 'Nenhum proprietário' }}</dd>
          <dt>Data de fechamento (previsão)</dt>
          <dd>{{ formatDate(deal.close_date) }}</dd>
          <dt>Última atividade</dt>
          <dd>{{ deal.last_activity_at ? relativeDate(deal.last_activity_at) : '—' }}</dd>
          <dt>Criado em</dt>
          <dd>{{ formatDate(deal.created_at) }}</dd>
          <dt>Fechado em</dt>
          <dd>{{ formatDate(deal.closed_at) }}</dd>
        </dl>
      </div>

      <CustomProperties entity="deals" :record-id="id" />
    </aside>

    <!-- ===== Coluna central ===== -->
    <section class="center">
      <div v-if="deal.status === 'aberto' && stages.length" class="stage-track card">
        <button
          v-for="(s, i) in stages"
          :key="s.id"
          type="button"
          class="stage-step"
          :class="{ current: s.id === deal.stage_id, past: stages.findIndex((x) => x.id === deal?.stage_id) > i }"
          @click="moveTo(s.id)"
        >
          {{ s.name }}
        </button>
      </div>

      <div class="center-tabs">
        <button type="button" :class="{ active: centerTab === 'visao' }" @click="centerTab = 'visao'">Visão geral</button>
        <button type="button" :class="{ active: centerTab === 'atividades' }" @click="centerTab = 'atividades'">Atividades</button>
      </div>

      <div v-if="centerTab === 'visao'" class="overview">
        <div class="card">
          <h2>Destaques</h2>
          <div class="highlights">
            <div class="highlight">
              <span class="hl-value">{{ pendingTasks.length }}</span>
              <span class="hl-label">tarefas pendentes</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ meetings.filter((m) => m.status === 'agendada').length }}</span>
              <span class="hl-label">reuniões agendadas</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ deal.last_activity_at ? relativeDate(deal.last_activity_at) : 'nunca' }}</span>
              <span class="hl-label">última atividade</span>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-head">
            <h2>Interações recentes</h2>
            <button class="btn btn-outline btn-sm" type="button" @click="centerTab = 'atividades'">Ver atividades</button>
          </div>
          <p v-if="!activities.length" class="empty-state"><strong>Nenhuma atividade neste registro.</strong></p>
          <ul v-else class="recent">
            <li v-for="a in activities.slice(0, 5)" :key="a.id">
              <span class="dot">{{ kindIcons[a.kind] || '•' }}</span>
              <span class="recent-content">{{ a.content }}</span>
              <span class="muted">{{ relativeDate(a.created_at) }}</span>
            </li>
          </ul>
        </div>

        <div class="card">
          <div class="card-head">
            <h2>Próximas tarefas</h2>
            <button class="btn btn-outline btn-sm" type="button" @click="openTask">+ Tarefa</button>
          </div>
          <p v-if="!pendingTasks.length" class="muted">Nenhuma tarefa pendente.</p>
          <ul v-else class="task-list">
            <li v-for="t in pendingTasks" :key="t.id">
              <label class="task-check">
                <input type="checkbox" :checked="!!t.completed_at" @change="toggleTask(t)" />
                <span>{{ t.title }}</span>
              </label>
              <span class="muted" v-if="t.due_date">{{ formatDateTime(t.due_date) }}</span>
            </li>
          </ul>
        </div>
      </div>

      <div v-else class="activity-center">
        <div class="sub-tabs">
          <button
            v-for="t in activityTabs"
            :key="t.key"
            type="button"
            :class="{ active: activityTab === t.key }"
            @click="selectActivityTab(t.key)"
          >
            {{ t.label }}
          </button>
        </div>

        <div v-if="activityTab !== 'tarefas' && activityTab !== 'reunioes'">
          <div class="activity-toolbar">
            <input v-model="activitySearch" type="search" placeholder="Pesquisar atividades…" />
          </div>

          <div v-if="activityTab === 'todas' || activityTab === 'nota'" class="note-composer card">
            <textarea v-model="noteContent" rows="2" placeholder="Registrar uma observação…"></textarea>
            <button class="btn btn-primary btn-sm" type="button" :disabled="!noteContent.trim()" @click="addNote">Registrar</button>
          </div>

          <p v-if="loadingActivities" class="muted">Carregando…</p>
          <div v-else-if="!activities.length" class="empty-state"><strong>Nenhuma atividade aqui</strong></div>

          <div v-for="group in groupedActivities" :key="group.month" class="month-group">
            <h3>{{ group.month }}</h3>
            <div v-for="a in group.items" :key="a.id" class="activity-item card">
              <div class="activity-head">
                <span class="dot">{{ kindIcons[a.kind] || '•' }}</span>
                <strong>{{ kindLabels[a.kind] || a.kind }}</strong>
                <span class="muted" v-if="a.user_name">por {{ a.user_name }}</span>
                <span class="muted when">{{ formatDateTime(a.created_at) }}</span>
              </div>
              <p>{{ a.content }}</p>
            </div>
          </div>
        </div>

        <div v-else-if="activityTab === 'tarefas'">
          <div class="card-head" style="margin-bottom: 10px">
            <h3 class="sub-title">Tarefas do negócio</h3>
            <button class="btn btn-outline btn-sm" type="button" @click="openTask">+ Tarefa</button>
          </div>
          <p v-if="!tasks.length" class="empty-state"><strong>Nenhuma tarefa</strong></p>
          <div v-for="t in tasks" :key="t.id" class="activity-item card task-row">
            <label class="task-check">
              <input type="checkbox" :checked="!!t.completed_at" @change="toggleTask(t)" />
              <span :class="{ done: t.completed_at }">{{ t.title }}</span>
            </label>
            <span class="task-meta">
              <span class="badge" :class="t.priority === 'alta' ? 'red' : t.priority === 'baixa' ? 'gray' : 'blue'">{{ t.priority }}</span>
              <span class="muted" v-if="t.due_date">{{ formatDateTime(t.due_date) }}</span>
            </span>
          </div>
        </div>

        <div v-else>
          <div class="card-head" style="margin-bottom: 10px">
            <h3 class="sub-title">Reuniões do negócio</h3>
            <button class="btn btn-outline btn-sm" type="button" @click="openMeeting">+ Reunião</button>
          </div>
          <p v-if="!meetings.length" class="empty-state"><strong>Nenhuma reunião</strong></p>
          <div v-for="m in meetings" :key="m.id" class="activity-item card">
            <div class="activity-head">
              <span class="dot">⚑</span>
              <strong>{{ m.title }}</strong>
              <span class="badge" :class="meetingBadge[m.status]">{{ m.status }}</span>
              <span class="muted when">{{ formatDateTime(m.starts_at) }}</span>
            </div>
            <p v-if="m.location" class="muted">{{ m.location }}</p>
          </div>
        </div>
      </div>
    </section>

    <!-- ===== Coluna direita ===== -->
    <aside class="right">
      <div class="assoc card">
        <div class="assoc-head">
          <h2>Contatos ({{ deal.contact_id ? 1 : 0 }})</h2>
        </div>
        <div v-if="dealContact" class="contact-card">
          <span class="assoc-avatar">{{ initials(`${dealContact.first_name} ${dealContact.last_name}`) }}</span>
          <div class="contact-info">
            <router-link :to="`/contatos/${dealContact.id}`">
              <strong>{{ dealContact.first_name }} {{ dealContact.last_name }}</strong>
            </router-link>
            <span class="muted" v-if="dealContact.job_title">{{ dealContact.job_title }}</span>
            <a v-if="dealContact.email" class="muted small" :href="`mailto:${dealContact.email}`">{{ dealContact.email }}</a>
            <a v-if="dealContact.phone" class="muted small" :href="`tel:${dealContact.phone}`">{{ dealContact.phone }}</a>
          </div>
        </div>
        <p v-else class="muted">Vincule um contato editando o negócio.</p>
      </div>

      <div class="assoc card">
        <div class="assoc-head">
          <h2>Empresas ({{ deal.company_id ? 1 : 0 }})</h2>
        </div>
        <div v-if="deal.company_id" class="assoc-item">
          <router-link :to="`/empresas/${deal.company_id}`"><strong>{{ deal.company_name }}</strong></router-link>
          <span class="badge green">Principal</span>
        </div>
        <p v-else class="muted">Vincule uma empresa editando o negócio.</p>
      </div>

      <div class="assoc card">
        <div class="assoc-head">
          <h2>Tarefas ({{ pendingTasks.length }})</h2>
          <button type="button" class="add-link" @click="openTask">+ Adicionar</button>
        </div>
        <p v-if="!pendingTasks.length" class="muted">Nenhuma tarefa pendente para este negócio.</p>
        <ul class="assoc-list">
          <li v-for="t in pendingTasks.slice(0, 5)" :key="t.id">
            <span>{{ t.title }}</span>
            <span class="assoc-meta muted" v-if="t.due_date">{{ formatDate(t.due_date) }}</span>
          </li>
        </ul>
      </div>

      <div class="assoc card">
        <div class="assoc-head">
          <h2>Reuniões ({{ meetings.length }})</h2>
          <button type="button" class="add-link" @click="openMeeting">+ Adicionar</button>
        </div>
        <p v-if="!meetings.length" class="muted">Agende reuniões para avançar este negócio.</p>
        <ul class="assoc-list">
          <li v-for="m in meetings.slice(0, 5)" :key="m.id">
            <span>{{ m.title }}</span>
            <span class="assoc-meta muted">{{ formatDateTime(m.starts_at) }}</span>
          </li>
        </ul>
      </div>
      <AttachmentsPanel entity="negocio" :entity-id="id" />
    </aside>

    <!-- ===== Modais ===== -->
    <ModalDialog title="Editar negócio" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="saveEdit">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.name" required />
          </div>
          <div class="field">
            <label>Valor (R$)</label>
            <input v-model.number="form.amount" type="number" min="0" step="0.01" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Contato</label>
            <select v-model="form.contact_id">
              <option :value="null">Sem contato</option>
              <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Empresa</label>
            <select v-model="form.company_id">
              <option :value="null">Sem empresa</option>
              <option v-for="co in companies" :key="co.id" :value="co.id">{{ co.name }}</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Proprietário</label>
            <select v-model="form.owner_id">
              <option :value="null">Sem proprietário</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
          <div class="field">
            <label>Previsão de fechamento</label>
            <input v-model="form.close_date" type="date" />
          </div>
        </div>
        <div class="field">
          <label>Temperatura do deal</label>
          <select v-model="form.temperature">
            <option value="">—</option>
            <option value="quente">🔴 Quente</option>
            <option value="media">🟡 Média</option>
            <option value="fria">🔵 Fria</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar alterações' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Registrar chamada" :open="callOpen" @close="callOpen = false">
      <form @submit.prevent="saveCall">
        <div class="form-row">
          <div class="field">
            <label>Direção</label>
            <select v-model="callForm.direction">
              <option value="saida">Saída</option>
              <option value="entrada">Entrada</option>
            </select>
          </div>
          <div class="field">
            <label>Resultado</label>
            <select v-model="callForm.outcome">
              <option value="conectada">Conectada</option>
              <option value="sem_resposta">Sem resposta</option>
              <option value="caixa_postal">Caixa postal</option>
              <option value="ocupado">Ocupado</option>
              <option value="numero_errado">Número errado</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Notas</label>
          <textarea v-model="callForm.notes" rows="3"></textarea>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Registrar' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Nova tarefa" :open="taskOpen" @close="taskOpen = false">
      <form @submit.prevent="saveTask">
        <div class="field">
          <label>Título *</label>
          <input v-model="taskForm.title" required />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Tipo</label>
            <select v-model="taskForm.type">
              <option value="tarefa">Tarefa</option>
              <option value="ligacao">Ligação</option>
              <option value="email">E-mail</option>
              <option value="reuniao">Reunião</option>
            </select>
          </div>
          <div class="field">
            <label>Prioridade</label>
            <select v-model="taskForm.priority">
              <option value="baixa">Baixa</option>
              <option value="media">Média</option>
              <option value="alta">Alta</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Vencimento</label>
          <input v-model="taskForm.due_date" type="datetime-local" />
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar tarefa' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Agendar reunião" :open="meetingOpen" @close="meetingOpen = false">
      <form @submit.prevent="saveMeeting">
        <div class="field">
          <label>Título *</label>
          <input v-model="meetingForm.title" required />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Início *</label>
            <input v-model="meetingForm.starts_at" type="datetime-local" required />
          </div>
          <div class="field">
            <label>Local ou link</label>
            <input v-model="meetingForm.location" />
          </div>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Agendar' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Enviar e-mail" :open="emailOpen" @close="emailOpen = false">
      <form @submit.prevent="sendEmail">
        <p class="muted" style="margin-top: 0; font-size: 13px" v-if="dealContact">
          Para: {{ dealContact.first_name }} {{ dealContact.last_name }} ({{ dealContact.email }})
        </p>
        <div class="field">
          <label>Assunto *</label>
          <input v-model="emailForm.subject" required />
        </div>
        <div class="field">
          <label>Mensagem *</label>
          <textarea v-model="emailForm.body" rows="5" required></textarea>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Enviando…' : 'Enviar via Mandrill' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.record {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr) 300px;
  min-height: 100%;
  align-items: start;
}

@media (max-width: 1200px) {
  .record {
    grid-template-columns: 300px minmax(0, 1fr);
  }
  .right {
    display: none;
  }
}

@media (max-width: 900px) {
  .record {
    grid-template-columns: 1fr;
  }
}

.left {
  background: var(--fix-surface);
  border-right: 1px solid var(--fix-border);
  padding: 18px;
  min-height: 100%;
}

.back {
  font-size: 13px;
  font-weight: 500;
}

.profile {
  text-align: center;
  padding: 18px 0 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.profile h1 {
  font-size: 19px;
}

.deal-amount {
  font-size: 22px;
  font-weight: 700;
  color: var(--fix-purple-dark);
}

.badges {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: center;
}

.quick-actions {
  display: flex;
  justify-content: center;
  gap: 8px;
  padding: 12px 0 14px;
  flex-wrap: wrap;
}

.quick-actions button {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  border: none;
  background: none;
  cursor: pointer;
  font-size: 11px;
  color: var(--fix-text-2);
  min-width: 52px;
}

.quick-actions button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.quick-actions button span {
  width: 38px;
  height: 38px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  transition: background 0.15s, color 0.15s;
}

.quick-actions button:hover:not(:disabled) span {
  background: var(--fix-purple);
  color: #fff;
}

.close-actions {
  display: flex;
  gap: 8px;
  justify-content: center;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 16px;
}

.about {
  box-shadow: none;
  border: 1px solid var(--fix-border);
  padding: 14px;
}

.about-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.about h2 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--fix-text-3);
}

.about dl {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
}

.about dt {
  color: var(--fix-text-3);
  font-size: 12px;
  margin-top: 10px;
}

.about dd {
  margin: 0;
}

.center {
  padding: 14px 22px 22px;
  min-width: 0;
}

.stage-track {
  display: flex;
  gap: 6px;
  padding: 10px;
  margin-bottom: 14px;
  overflow-x: auto;
}

.stage-step {
  flex: 1;
  padding: 8px 10px;
  border: none;
  background: var(--fix-bg);
  color: var(--fix-text-3);
  font-size: 13px;
  cursor: pointer;
  border-radius: 6px;
  white-space: nowrap;
  transition: background 0.15s, color 0.15s;
}

.stage-step.past {
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
}

.stage-step.current {
  background: var(--fix-purple);
  color: #fff;
  font-weight: 600;
}

.center-tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 16px;
}

.center-tabs > button {
  padding: 10px 18px;
  border: none;
  background: none;
  font-size: 14px;
  color: var(--fix-text-2);
  cursor: pointer;
  border-bottom: 2px solid transparent;
}

.center-tabs > button.active {
  color: var(--fix-purple);
  border-bottom-color: var(--fix-purple);
  font-weight: 600;
}

.sub-tabs {
  display: flex;
  gap: 2px;
  margin-bottom: 14px;
  overflow-x: auto;
  border-bottom: 1px solid var(--fix-border);
}

.sub-tabs button {
  padding: 8px 12px;
  border: none;
  background: none;
  font-size: 13px;
  color: var(--fix-text-2);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  white-space: nowrap;
}

.sub-tabs button.active {
  color: var(--fix-purple);
  border-bottom-color: var(--fix-purple);
  font-weight: 600;
}

.activity-toolbar {
  margin-bottom: 12px;
}

.activity-toolbar input {
  width: min(320px, 100%);
  padding: 8px 12px;
  border: 1px solid var(--fix-border);
  border-radius: 999px;
  font-size: 13px;
  outline: none;
}

.activity-toolbar input:focus {
  border-color: var(--fix-purple);
}

.note-composer {
  display: flex;
  gap: 10px;
  padding: 12px;
  margin-bottom: 14px;
  align-items: flex-start;
}

.note-composer textarea {
  flex: 1;
  padding: 8px 10px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  font-family: inherit;
  font-size: 13px;
  resize: vertical;
  outline: none;
}

.note-composer textarea:focus {
  border-color: var(--fix-purple);
}

.month-group h3 {
  font-size: 14px;
  margin: 18px 0 10px;
  color: var(--fix-text-2);
}

.activity-item {
  padding: 12px 14px;
  margin-bottom: 8px;
}

.activity-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  flex-wrap: wrap;
}

.activity-head .when {
  margin-left: auto;
  font-size: 12px;
}

.activity-item p {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--fix-text-2);
  white-space: pre-wrap;
  word-break: break-word;
}

.dot {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}

.task-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
}

.task-meta {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 12px;
}

.task-check {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 13px;
}

.done {
  text-decoration: line-through;
  color: var(--fix-text-3);
}

.sub-title {
  font-size: 14px;
}

.overview {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.card h2 {
  font-size: 15px;
  margin-bottom: 14px;
}

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-head h2 {
  margin-bottom: 0;
}

.highlights {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}

.highlight {
  background: var(--fix-bg);
  border-radius: 10px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  text-align: center;
}

.hl-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--fix-purple-dark);
}

.hl-label {
  font-size: 12px;
  color: var(--fix-text-2);
}

.recent {
  list-style: none;
  margin: 0;
  padding: 0;
}

.recent li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 0;
  border-bottom: 1px solid var(--fix-bg);
  font-size: 13px;
}

.recent li:last-child {
  border-bottom: none;
}

.recent-content {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
}

.task-list li {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
}

.right {
  padding: 18px 18px 22px 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.assoc {
  padding: 14px;
}

.assoc-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.assoc h2 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--fix-text-3);
}

.add-link {
  border: none;
  background: none;
  color: var(--fix-purple);
  font-size: 13px;
  cursor: pointer;
  font-weight: 500;
}

.add-link:hover {
  text-decoration: underline;
}

.assoc .muted {
  font-size: 12px;
}

.contact-card {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.assoc-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--fix-purple);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.contact-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  min-width: 0;
}

.contact-info .small {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.assoc-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
}

.assoc-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
}

.assoc-list li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--fix-bg);
}

.assoc-list li:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.assoc-meta {
  font-size: 12px;
}
</style>
