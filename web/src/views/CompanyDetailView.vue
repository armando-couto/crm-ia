<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, formatDateTime, formatMoney, initials, lifecycleLabels, relativeDate } from '../format'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import CustomProperties from '../components/CustomProperties.vue'
import AttachmentsPanel from '../components/AttachmentsPanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Activity, Company, Contact, Deal, Meeting, Paginated, Pipeline, Task, Ticket, User } from '../types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

const id = Number(route.params.id)
const company = ref<Company | null>(null)

// Conta-alvo: prioridade da empresa na estratégia de vendas.
const targetTier = ref(2)
const targetNotes = ref('')

async function saveTarget() {
  if (!company.value) return
  try {
    company.value = await api.put<Company>(`/companies/${id}/target`, {
      is_target: company.value.is_target,
      target_tier: targetTier.value,
      target_notes: targetNotes.value
    })
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function toggleTarget(event: Event) {
  if (!company.value) return
  const marcar = (event.target as HTMLInputElement).checked
  try {
    company.value = await api.put<Company>(`/companies/${id}/target`, {
      is_target: marcar,
      target_tier: targetTier.value,
      target_notes: targetNotes.value
    })
    targetTier.value = company.value.target_tier || 2
    targetNotes.value = company.value.target_notes
    toast.push(marcar ? 'Marcada como conta-alvo' : 'Removida das contas-alvo')
  } catch (e: any) {
    toast.error(e.message)
  }
}
const contacts = ref<Contact[]>([])
const deals = ref<Deal[]>([])
const tickets = ref<Ticket[]>([])
const tasks = ref<Task[]>([])
const meetings = ref<Meeting[]>([])
const users = ref<User[]>([])
const pipelines = ref<Pipeline[]>([])

const centerTab = ref<'visao' | 'atividades'>('visao')

// ===== Central de atividades (sub-abas) =====
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
    const params = new URLSearchParams({ company_id: String(id), limit: '200' })
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

// Timeline agrupada por mês, como no HubSpot ("Junho 2026").
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
    await api.post('/activities', { kind: 'nota', content: noteContent.value.trim(), company_id: id })
    noteContent.value = ''
    toast.push('Observação registrada')
    await loadActivities()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Carregamento geral =====
async function load() {
  try {
    company.value = await api.get<Company>(`/companies/${id}`)
    targetTier.value = company.value.target_tier || 2
    targetNotes.value = company.value.target_notes ?? ''
    const [contactsResp, dealsResp, ticketsResp, tasksResp, meetingsResp] = await Promise.all([
      api.get<Paginated<Contact>>(`/contacts?company_id=${id}&per_page=100`),
      api.get<Paginated<Deal>>(`/deals?company_id=${id}&per_page=50`),
      api.get<Paginated<Ticket>>(`/tickets?company_id=${id}&per_page=50`),
      api.get<Paginated<Task>>(`/tasks?company_id=${id}&per_page=50`),
      api.get<Paginated<Meeting>>(`/meetings?company_id=${id}&per_page=50`)
    ])
    contacts.value = contactsResp.data
    deals.value = dealsResp.data
    tickets.value = ticketsResp.data
    tasks.value = tasksResp.data
    meetings.value = meetingsResp.data
  } catch (e: any) {
    toast.error(e.message)
    router.push('/empresas')
  }
}

const openDeals = computed(() => deals.value.filter((d) => d.status === 'aberto'))
const pendingTasks = computed(() => tasks.value.filter((t) => !t.completed_at))
const openTickets = computed(() => tickets.value.filter((t) => t.status === 'aberto' || t.status === 'pendente'))
const emailContact = computed(() => contacts.value.find((c) => c.email))

// ===== Modais =====
const editOpen = ref(false)
const callOpen = ref(false)
const taskOpen = ref(false)
const meetingOpen = ref(false)
const dealOpen = ref(false)
const ticketOpen = ref(false)
const emailOpen = ref(false)
const saving = ref(false)

const form = ref<any>({})
const callForm = ref({ contact_id: 0 as number | 0, direction: 'saida', outcome: 'conectada', duration_seconds: 0, notes: '' })
const taskForm = ref({ title: '', type: 'tarefa', priority: 'media', due_date: '' })
const meetingForm = ref({ title: '', starts_at: '', location: '', contact_id: null as number | null })
const dealForm = ref({ name: '', amount: 0, pipeline_id: 0, stage_id: 0, contact_id: null as number | null })
const ticketForm = ref({ subject: '', description: '', priority: 'media', contact_id: null as number | null })
const emailForm = ref({ contact_id: 0, subject: '', body: '' })

const dealStages = computed(() => {
  const p = pipelines.value.find((x) => x.id === dealForm.value.pipeline_id)
  return p?.stages.filter((s) => !s.is_won && !s.is_lost) ?? []
})

function openEdit() {
  if (!company.value) return
  form.value = {
    ...company.value,
    accredited_at: company.value.accredited_at ? company.value.accredited_at.slice(0, 10) : '',
    products_text: (company.value.products || []).join(', ')
  }
  editOpen.value = true
}

async function saveEdit() {
  saving.value = true
  try {
    const payload = {
      ...form.value,
      products: String(form.value.products_text || '')
        .split(',')
        .map((p: string) => p.trim())
        .filter(Boolean)
    }
    company.value = await api.put<Company>(`/companies/${id}`, payload)
    toast.push('Empresa atualizada')
    editOpen.value = false
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!confirm('Remover esta empresa? Os contatos serão mantidos, sem vínculo.')) return
  try {
    await api.delete(`/companies/${id}`)
    toast.push('Empresa removida')
    router.push('/empresas')
  } catch (e: any) {
    toast.error(e.message)
  }
}

function quickNote() {
  centerTab.value = 'atividades'
  selectActivityTab('nota')
}

function openCall() {
  callForm.value = { contact_id: contacts.value[0]?.id ?? 0, direction: 'saida', outcome: 'conectada', duration_seconds: 0, notes: '' }
  callOpen.value = true
}

function openEmail() {
  emailForm.value = { contact_id: emailContact.value?.id ?? 0, subject: '', body: '' }
  emailOpen.value = true
}

function openTask() {
  taskForm.value = { title: '', type: 'tarefa', priority: 'media', due_date: '' }
  taskOpen.value = true
}

function openMeeting() {
  meetingForm.value = { title: '', starts_at: '', location: '', contact_id: contacts.value[0]?.id ?? null }
  meetingOpen.value = true
}

function openDeal() {
  const first = pipelines.value[0]
  dealForm.value = {
    name: '',
    amount: 0,
    pipeline_id: first?.id ?? 0,
    stage_id: first?.stages.find((s) => !s.is_won && !s.is_lost)?.id ?? 0,
    contact_id: contacts.value[0]?.id ?? null
  }
  dealOpen.value = true
}

function openTicket() {
  ticketForm.value = { subject: '', description: '', priority: 'media', contact_id: contacts.value[0]?.id ?? null }
  ticketOpen.value = true
}

async function post(path: string, payload: Record<string, unknown>, message: string, close: () => void) {
  saving.value = true
  try {
    await api.post(path, payload)
    toast.push(message)
    close()
    await Promise.all([load(), loadActivities()])
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

const saveCall = () =>
  post('/calls', { ...callForm.value, contact_id: callForm.value.contact_id || null, company_id: id }, 'Chamada registrada', () => (callOpen.value = false))
const saveTask = () => post('/tasks', { ...taskForm.value, company_id: id }, 'Tarefa criada', () => (taskOpen.value = false))
const saveMeeting = () => post('/meetings', { ...meetingForm.value, company_id: id }, 'Reunião agendada', () => (meetingOpen.value = false))
const saveDeal = () =>
  post('/deals', { ...dealForm.value, amount: Number(dealForm.value.amount) || 0, company_id: id }, 'Negócio criado', () => (dealOpen.value = false))
const saveTicket = () => post('/tickets', { ...ticketForm.value, company_id: id }, 'Ticket criado', () => (ticketOpen.value = false))
const sendEmail = () =>
  post('/emails', { ...emailForm.value }, 'E-mail enviado', () => (emailOpen.value = false))

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
    users.value = await api.get<User[]>('/users')
    pipelines.value = await api.get<Pipeline[]>('/pipelines')
  } catch {
    /* opcional */
  }
})

const kindIcons: Record<string, string> = { nota: '✎', email: '✉', ligacao: '☎', reuniao: '⚑', sistema: '⚙' }
const kindLabels: Record<string, string> = { nota: 'Observação', email: 'E-mail', ligacao: 'Chamada', reuniao: 'Reunião', sistema: 'Sistema' }
const ticketBadge: Record<string, string> = { aberto: 'blue', pendente: 'amber', resolvido: 'green', fechado: 'gray' }
const meetingBadge: Record<string, string> = { agendada: 'blue', realizada: 'green', cancelada: 'gray', nao_compareceu: 'red' }
</script>

<template>
  <div class="record" v-if="company">
    <!-- ===== Coluna esquerda ===== -->
    <aside class="left">
      <router-link to="/empresas" class="back">‹ Empresas</router-link>

      <div class="profile">
        <span class="avatar-xl">{{ initials(company.name) }}</span>
        <h1>{{ company.name }}</h1>
        <p class="muted" v-if="company.domain || company.industry">
          {{ company.industry }}<template v-if="company.industry && company.domain"> · </template>{{ company.domain }}
        </p>
        <span v-if="company.is_client" class="badge green" style="margin-top: 6px">Cliente</span>
        <span v-if="company.do_not_disturb" class="badge red" style="margin-top: 6px">Não perturbe</span>
        <span v-if="company.is_target" class="badge" style="margin-top: 6px">
          Conta-alvo · tier {{ company.target_tier }}
        </span>
      </div>

      <!-- Conta-alvo: prioridade da empresa na estratégia de vendas. -->
      <div v-if="auth.can('companies.edit')" class="target-box">
        <label class="target-toggle">
          <input type="checkbox" :checked="company.is_target" @change="toggleTarget" />
          <span>Conta-alvo</span>
        </label>
        <template v-if="company.is_target">
          <select v-model.number="targetTier" @change="saveTarget">
            <option :value="1">Tier 1 — prioridade máxima</option>
            <option :value="2">Tier 2</option>
            <option :value="3">Tier 3</option>
          </select>
          <textarea
            v-model="targetNotes"
            rows="2"
            placeholder="Por que esta conta importa?"
            @blur="saveTarget"
          ></textarea>
        </template>
      </div>

      <div class="quick-actions">
        <button type="button" @click="quickNote"><span>✎</span>Observ.</button>
        <button type="button" :disabled="!emailContact" @click="openEmail"><span>✉</span>E-mail</button>
        <button type="button" :disabled="!contacts.length" @click="openCall"><span>☎</span>Chamada</button>
        <button type="button" @click="openTask"><span>✓</span>Tarefa</button>
        <button type="button" @click="openMeeting"><span>⚑</span>Reunião</button>
      </div>

      <div class="about card">
        <div class="about-head">
          <h2>Sobre essa empresa</h2>
          <div>
            <button class="btn btn-outline btn-sm" type="button" @click="openEdit">Editar</button>
            <button class="btn btn-danger btn-sm" type="button" @click="remove">✕</button>
          </div>
        </div>
        <dl>
          <dt>Número do EC</dt>
          <dd>{{ company.ec_number || '—' }}</dd>
          <dt>Grupo econômico</dt>
          <dd>{{ company.economic_group || '—' }}</dd>
          <dt>CNPJ/CPF</dt>
          <dd>{{ company.cnpj || '—' }}</dd>
          <dt>Data do credenciamento</dt>
          <dd>{{ formatDate(company.accredited_at) }}</dd>
          <dt>Representante da empresa</dt>
          <dd>{{ company.representative || '—' }}</dd>
          <dt>Telefone</dt>
          <dd>{{ company.phone || '—' }}</dd>
          <dt>Cidade / Estado</dt>
          <dd>{{ company.city ? `${company.city}${company.state ? '/' + company.state : ''}` : company.state || '—' }}</dd>
          <dt>Instagram</dt>
          <dd>
            <a v-if="company.instagram" :href="`https://instagram.com/${company.instagram}`" target="_blank">@{{ company.instagram }}</a>
            <template v-else>—</template>
          </dd>
          <dt>Produtos contratados</dt>
          <dd>
            <span v-if="!company.products?.length">—</span>
            <span v-else class="product-tags">
              <span v-for="p in company.products" :key="p" class="badge">{{ p }}</span>
            </span>
          </dd>
          <dt>Quantidade de máquinas</dt>
          <dd>{{ company.machines_count }}</dd>
          <dt>Cliente ou Não é Cliente?</dt>
          <dd>{{ company.is_client ? 'Cliente' : 'Não é cliente' }}</dd>
          <dt>Modalidade de antecipação</dt>
          <dd style="text-transform: capitalize">{{ company.anticipation_mode || '—' }}</dd>
          <dt>Validador</dt>
          <dd>{{ company.validator ? 'Sim' : 'Não' }}</dd>
          <dt>Proprietário</dt>
          <dd :class="{ muted: !company.owner_name }">{{ company.owner_name || 'Nenhum proprietário' }}</dd>
          <dt>Criada em</dt>
          <dd>{{ formatDate(company.created_at) }}</dd>
        </dl>
      </div>

      <CustomProperties entity="companies" :record-id="id" />
    </aside>

    <!-- ===== Coluna central ===== -->
    <section class="center">
      <div class="center-tabs">
        <button type="button" :class="{ active: centerTab === 'visao' }" @click="centerTab = 'visao'">Visão geral</button>
        <button type="button" :class="{ active: centerTab === 'atividades' }" @click="centerTab = 'atividades'">Atividades</button>
      </div>

      <div v-if="centerTab === 'visao'" class="overview">
        <div class="card">
          <h2>Destaques</h2>
          <div class="highlights">
            <div class="highlight">
              <span class="hl-value">{{ contacts.length }}</span>
              <span class="hl-label">contatos</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ openDeals.length }}</span>
              <span class="hl-label">negócios abertos</span>
              <span class="hl-sub">{{ formatMoney(openDeals.reduce((s, d) => s + d.amount, 0)) }}</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ openTickets.length }}</span>
              <span class="hl-label">tickets abertos</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ pendingTasks.length }}</span>
              <span class="hl-label">tarefas pendentes</span>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-head">
            <h2>Interações recentes</h2>
            <button class="btn btn-outline btn-sm" type="button" @click="centerTab = 'atividades'">Ver atividades</button>
          </div>
          <p v-if="!activities.length" class="empty-state">
            <strong>Nenhuma atividade neste registro.</strong>
          </p>
          <ul v-else class="recent">
            <li v-for="a in activities.slice(0, 5)" :key="a.id">
              <span class="dot">{{ kindIcons[a.kind] || '•' }}</span>
              <span class="recent-content">{{ a.content }}</span>
              <span class="muted">{{ relativeDate(a.created_at) }}</span>
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
          <div v-else-if="!activities.length" class="empty-state">
            <strong>Nenhuma atividade aqui</strong>
          </div>

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
            <h3 class="sub-title">Tarefas da empresa</h3>
            <button class="btn btn-outline btn-sm" type="button" @click="openTask">+ Tarefa</button>
          </div>
          <p v-if="!tasks.length" class="empty-state"><strong>Nenhuma tarefa</strong></p>
          <div v-for="t in tasks" :key="t.id" class="activity-item card task-row">
            <label class="task-check">
              <input type="checkbox" :checked="!!t.completed_at" @change="toggleTask(t)" />
              <span :class="{ done: t.completed_at }">{{ t.title }}</span>
            </label>
            <span class="muted" v-if="t.due_date">{{ formatDateTime(t.due_date) }}</span>
          </div>
        </div>

        <div v-else>
          <div class="card-head" style="margin-bottom: 10px">
            <h3 class="sub-title">Reuniões da empresa</h3>
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
          <h2>Contatos ({{ contacts.length }})</h2>
        </div>
        <p v-if="!contacts.length" class="muted">Veja as pessoas associadas a este registro.</p>
        <div v-for="c in contacts.slice(0, 5)" :key="c.id" class="contact-card">
          <router-link :to="`/contatos/${c.id}`"><strong>{{ c.first_name }} {{ c.last_name }}</strong></router-link>
          <span class="muted" v-if="c.job_title">{{ c.job_title }}</span>
          <a v-if="c.email" class="muted small" :href="`mailto:${c.email}`">{{ c.email }}</a>
          <a v-if="c.phone" class="muted small" :href="`tel:${c.phone}`">{{ c.phone }}</a>
        </div>
      </div>

      <div class="assoc card">
        <div class="assoc-head">
          <h2>Negócios ({{ deals.length }})</h2>
          <button type="button" class="add-link" @click="openDeal">+ Adicionar</button>
        </div>
        <p v-if="!deals.length" class="muted">Acompanhe as oportunidades de receita associadas a este registro.</p>
        <ul class="assoc-list">
          <li v-for="d in deals" :key="d.id">
            <router-link :to="`/negocios/${d.id}`">{{ d.name }}</router-link>
            <span class="assoc-meta">
              {{ formatMoney(d.amount) }}
              <span class="badge" :class="d.status === 'ganho' ? 'green' : d.status === 'perdido' ? 'red' : 'blue'">
                {{ d.status === 'aberto' ? d.stage_name : d.status }}
              </span>
            </span>
          </li>
        </ul>
      </div>

      <div class="assoc card">
        <div class="assoc-head">
          <h2>Tickets ({{ tickets.length }})</h2>
          <button type="button" class="add-link" @click="openTicket">+ Adicionar</button>
        </div>
        <p v-if="!tickets.length" class="muted">Acompanhe as solicitações dos clientes associadas a este registro.</p>
        <ul class="assoc-list">
          <li v-for="t in tickets" :key="t.id">
            <router-link :to="`/tickets/${t.id}`">#{{ t.id }} {{ t.subject }}</router-link>
            <span class="assoc-meta"><span class="badge" :class="ticketBadge[t.status]">{{ t.status }}</span></span>
          </li>
        </ul>
      </div>
      <AttachmentsPanel entity="empresa" :entity-id="id" />
    </aside>

    <!-- ===== Modais ===== -->
    <ModalDialog title="Editar empresa" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="saveEdit">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.name" required />
          </div>
          <div class="field">
            <label>Número do EC</label>
            <input v-model="form.ec_number" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>CNPJ/CPF</label>
            <input v-model="form.cnpj" />
          </div>
          <div class="field">
            <label>Grupo econômico</label>
            <input v-model="form.economic_group" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Data do credenciamento</label>
            <input v-model="form.accredited_at" type="date" />
          </div>
          <div class="field">
            <label>Representante</label>
            <input v-model="form.representative" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Domínio</label>
            <input v-model="form.domain" />
          </div>
          <div class="field">
            <label>Telefone</label>
            <input v-model="form.phone" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Segmento</label>
            <input v-model="form.industry" />
          </div>
          <div class="field">
            <label>Instagram</label>
            <input v-model="form.instagram" placeholder="@perfil" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Cidade</label>
            <input v-model="form.city" />
          </div>
          <div class="field">
            <label>UF</label>
            <input v-model="form.state" maxlength="2" />
          </div>
        </div>
        <div class="field">
          <label>Produtos contratados (separados por vírgula)</label>
          <input v-model="form.products_text" placeholder="Pix, Link de pagamento, Safe Link" />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Quantidade de máquinas</label>
            <input v-model.number="form.machines_count" type="number" min="0" />
          </div>
          <div class="field">
            <label>Modalidade de antecipação</label>
            <select v-model="form.anticipation_mode">
              <option value="">—</option>
              <option value="pontual">Pontual</option>
              <option value="automatica">Automática</option>
              <option value="nenhuma">Nenhuma</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label class="check-inline"><input v-model="form.is_client" type="checkbox" /> É cliente</label>
          </div>
          <div class="field">
            <label class="check-inline"><input v-model="form.validator" type="checkbox" /> Validador</label>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label class="check-inline"><input v-model="form.do_not_disturb" type="checkbox" /> Não perturbe</label>
          </div>
          <div class="field">
            <label>Proprietário</label>
            <select v-model="form.owner_id">
              <option :value="null">Sem proprietário</option>
              <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
            </select>
          </div>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar alterações' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Registrar chamada" :open="callOpen" @close="callOpen = false">
      <form @submit.prevent="saveCall">
        <div class="field">
          <label>Contato *</label>
          <select v-model.number="callForm.contact_id" required>
            <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
          </select>
        </div>
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
        <button class="btn btn-primary" type="submit" :disabled="saving || !callForm.contact_id" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Registrar' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Enviar e-mail" :open="emailOpen" @close="emailOpen = false">
      <form @submit.prevent="sendEmail">
        <div class="field">
          <label>Contato *</label>
          <select v-model.number="emailForm.contact_id" required>
            <option v-for="c in contacts.filter((x) => x.email)" :key="c.id" :value="c.id">
              {{ c.first_name }} {{ c.last_name }} ({{ c.email }})
            </option>
          </select>
        </div>
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
            <label>Vencimento</label>
            <input v-model="taskForm.due_date" type="datetime-local" />
          </div>
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
        <div class="field">
          <label>Contato</label>
          <select v-model="meetingForm.contact_id">
            <option :value="null">Sem contato</option>
            <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Agendar' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Novo negócio" :open="dealOpen" @close="dealOpen = false">
      <form @submit.prevent="saveDeal">
        <div class="field">
          <label>Nome *</label>
          <input v-model="dealForm.name" required />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Valor (R$)</label>
            <input v-model.number="dealForm.amount" type="number" min="0" step="0.01" />
          </div>
          <div class="field">
            <label>Etapa</label>
            <select v-model.number="dealForm.stage_id">
              <option v-for="s in dealStages" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </div>
        </div>
        <div class="field">
          <label>Contato</label>
          <select v-model="dealForm.contact_id">
            <option :value="null">Sem contato</option>
            <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar negócio' }}
        </button>
      </form>
    </ModalDialog>

    <ModalDialog title="Novo ticket" :open="ticketOpen" @close="ticketOpen = false">
      <form @submit.prevent="saveTicket">
        <div class="field">
          <label>Assunto *</label>
          <input v-model="ticketForm.subject" required />
        </div>
        <div class="field">
          <label>Descrição</label>
          <textarea v-model="ticketForm.description" rows="3"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Prioridade</label>
            <select v-model="ticketForm.priority">
              <option value="baixa">Baixa</option>
              <option value="media">Média</option>
              <option value="alta">Alta</option>
            </select>
          </div>
          <div class="field">
            <label>Contato</label>
            <select v-model="ticketForm.contact_id">
              <option :value="null">Sem contato</option>
              <option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.first_name }} {{ c.last_name }}</option>
            </select>
          </div>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar ticket' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.target-box {
  background: var(--fix-purple-tint);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.target-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--fix-purple-dark);
  cursor: pointer;
}

.target-box select,
.target-box textarea {
  width: 100%;
  font-size: 13px;
}

.record {
  display: grid;
  grid-template-columns: 340px minmax(0, 1fr) 300px;
  min-height: 100%;
  align-items: start;
}

@media (max-width: 1200px) {
  .record {
    grid-template-columns: 320px minmax(0, 1fr);
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
  gap: 2px;
}

.avatar-xl {
  width: 72px;
  height: 72px;
  border-radius: 16px;
  background: var(--fix-purple);
  color: #fff;
  font-size: 26px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
}

.profile h1 {
  font-size: 20px;
}

.profile p {
  margin: 2px 0 0;
  font-size: 13px;
}

.quick-actions {
  display: flex;
  justify-content: center;
  gap: 8px;
  padding: 14px 0 18px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 16px;
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
  word-break: break-word;
}

.product-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 2px;
}

.center {
  padding: 0 22px 22px;
  min-width: 0;
}

.center-tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 16px;
  background: var(--fix-bg);
  position: sticky;
  top: 0;
  z-index: 5;
  padding-top: 14px;
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
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
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
}

.hl-value {
  font-size: 26px;
  font-weight: 700;
  color: var(--fix-purple-dark);
}

.hl-label {
  font-size: 12px;
  color: var(--fix-text-2);
}

.hl-sub {
  font-size: 12px;
  color: var(--fix-text-3);
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
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  padding-bottom: 10px;
  margin-bottom: 10px;
  border-bottom: 1px solid var(--fix-bg);
}

.contact-card:last-child {
  border-bottom: none;
  margin-bottom: 0;
  padding-bottom: 0;
}

.contact-card .small {
  font-size: 12px;
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
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 12px;
  color: var(--fix-text-3);
}

.check-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
</style>
