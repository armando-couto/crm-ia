<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, formatDateTime, formatMoney, initials, lifecycleLabels, relativeDate } from '../format'
import { useToastStore } from '../stores/toast'
import CustomProperties from '../components/CustomProperties.vue'
import TimelinePanel from '../components/TimelinePanel.vue'
import AttachmentsPanel from '../components/AttachmentsPanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Activity, Company, Contact, Deal, Paginated, Pipeline, Task, Ticket, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const contact = ref<Contact | null>(null)
const deals = ref<Deal[]>([])
const tasks = ref<Task[]>([])
const tickets = ref<Ticket[]>([])
const recentActivities = ref<Activity[]>([])
const users = ref<User[]>([])
const companies = ref<Company[]>([])
const pipelines = ref<Pipeline[]>([])

const centerTab = ref<'visao' | 'atividades'>('visao')
const timeline = ref<InstanceType<typeof TimelinePanel> | null>(null)

// ===== Modais =====
const editOpen = ref(false)
const callOpen = ref(false)
const taskOpen = ref(false)
const meetingOpen = ref(false)
const dealOpen = ref(false)
const ticketOpen = ref(false)
const saving = ref(false)

const form = ref<any>({})

// Rótulos do papel do contato na decisão de compra (contas-alvo).
const buyingRoleLabels: Record<string, string> = {
  decisor: 'Decisor',
  influenciador: 'Influenciador',
  usuario: 'Usuário',
  financeiro: 'Financeiro',
  bloqueador: 'Bloqueador',
  campeao: 'Campeão'
}
const callForm = ref({ direction: 'saida', outcome: 'conectada', duration_seconds: 0, notes: '' })
const taskForm = ref({ title: '', type: 'tarefa', priority: 'media', due_date: '' })
const meetingForm = ref({ title: '', starts_at: '', location: '' })
const dealForm = ref({ name: '', amount: 0, pipeline_id: 0, stage_id: 0 })
const ticketForm = ref({ subject: '', description: '', priority: 'media' })

const fullName = computed(() => (contact.value ? `${contact.value.first_name} ${contact.value.last_name}`.trim() : ''))
const pendingTasks = computed(() => tasks.value.filter((t) => !t.completed_at))
const openDeals = computed(() => deals.value.filter((d) => d.status === 'aberto'))
const openTickets = computed(() => tickets.value.filter((t) => t.status === 'aberto' || t.status === 'pendente'))
const dealStages = computed(() => {
  const p = pipelines.value.find((x) => x.id === dealForm.value.pipeline_id)
  return p?.stages.filter((s) => !s.is_won && !s.is_lost) ?? []
})

async function load() {
  try {
    contact.value = await api.get<Contact>(`/contacts/${id}`)
    const [dealsResp, tasksResp, ticketsResp, activitiesResp] = await Promise.all([
      api.get<Paginated<Deal>>(`/deals?contact_id=${id}&per_page=50`),
      api.get<Paginated<Task>>(`/tasks?contact_id=${id}&per_page=50`),
      api.get<Paginated<Ticket>>(`/tickets?contact_id=${id}&per_page=50`),
      api.get<{ data: Activity[] }>(`/activities?contact_id=${id}&limit=5`)
    ])
    deals.value = dealsResp.data
    tasks.value = tasksResp.data
    tickets.value = ticketsResp.data
    recentActivities.value = activitiesResp.data
  } catch (e: any) {
    toast.error(e.message)
    router.push('/contatos')
  }
}

async function refreshAll() {
  await load()
  timeline.value?.reload()
}

// ===== Ações rápidas =====
function quickNote() {
  centerTab.value = 'atividades'
}

function quickEmail() {
  centerTab.value = 'atividades'
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

function openDeal() {
  const first = pipelines.value[0]
  dealForm.value = {
    name: '',
    amount: 0,
    pipeline_id: first?.id ?? 0,
    stage_id: first?.stages.find((s) => !s.is_won && !s.is_lost)?.id ?? 0
  }
  dealOpen.value = true
}

function openTicket() {
  ticketForm.value = { subject: '', description: '', priority: 'media' }
  ticketOpen.value = true
}

async function saveCall() {
  saving.value = true
  try {
    await api.post('/calls', { ...callForm.value, contact_id: id, company_id: contact.value?.company_id })
    toast.push('Chamada registrada')
    callOpen.value = false
    await refreshAll()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function saveTask() {
  saving.value = true
  try {
    await api.post('/tasks', { ...taskForm.value, contact_id: id })
    toast.push('Tarefa criada')
    taskOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function saveMeeting() {
  saving.value = true
  try {
    await api.post('/meetings', { ...meetingForm.value, contact_id: id, company_id: contact.value?.company_id })
    toast.push('Reunião agendada')
    meetingOpen.value = false
    await refreshAll()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function saveDeal() {
  saving.value = true
  try {
    await api.post('/deals', {
      ...dealForm.value,
      amount: Number(dealForm.value.amount) || 0,
      contact_id: id,
      company_id: contact.value?.company_id
    })
    toast.push('Negócio criado')
    dealOpen.value = false
    await refreshAll()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function saveTicket() {
  saving.value = true
  try {
    await api.post('/tickets', { ...ticketForm.value, contact_id: id, company_id: contact.value?.company_id })
    toast.push('Ticket criado')
    ticketOpen.value = false
    await refreshAll()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

// ===== Edição / remoção =====
function openEdit() {
  if (!contact.value) return
  form.value = { ...contact.value }
  editOpen.value = true
}

async function saveEdit() {
  saving.value = true
  try {
    contact.value = await api.put<Contact>(`/contacts/${id}`, form.value)
    toast.push('Contato atualizado')
    editOpen.value = false
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!confirm('Remover este contato? As atividades e tarefas vinculadas também serão removidas.')) return
  try {
    await api.delete(`/contacts/${id}`)
    toast.push('Contato removido')
    router.push('/contatos')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function toggleTask(task: Task) {
  try {
    const updated = await api.patch<Task>(`/tasks/${task.id}/toggle`, { done: !task.completed_at })
    tasks.value = tasks.value.map((t) => (t.id === task.id ? updated : t))
  } catch (e: any) {
    toast.error(e.message)
  }
}

function copyEmail() {
  if (!contact.value?.email) return
  navigator.clipboard?.writeText(contact.value.email)
  toast.push('E-mail copiado')
}

onMounted(async () => {
  await load()
  try {
    const [usersResp, companiesResp, pipelinesResp] = await Promise.all([
      api.get<User[]>('/users'),
      api.get<Paginated<Company>>('/companies?per_page=100'),
      api.get<Pipeline[]>('/pipelines')
    ])
    users.value = usersResp
    companies.value = companiesResp.data
    pipelines.value = pipelinesResp
  } catch {
    /* opcional */
  }
})

const kindIcons: Record<string, string> = { nota: '✎', email: '✉', ligacao: '☎', reuniao: '⚑', sistema: '⚙' }
const ticketBadge: Record<string, string> = { aberto: 'blue', pendente: 'amber', resolvido: 'green', fechado: 'gray' }
</script>

<template>
  <div class="record" v-if="contact">
    <!-- ===== Coluna esquerda: perfil ===== -->
    <aside class="left">
      <router-link to="/contatos" class="back">‹ Contatos</router-link>

      <div class="profile">
        <span class="avatar-xl">{{ initials(fullName) }}</span>
        <h1>{{ fullName }}</h1>
        <p class="muted" v-if="contact.job_title || contact.company_name">
          {{ contact.job_title }}<template v-if="contact.job_title && contact.company_name"> · </template>{{ contact.company_name }}
        </p>
        <button v-if="contact.email" type="button" class="email-line" @click="copyEmail" title="Copiar e-mail">
          {{ contact.email }} ⧉
        </button>
      </div>

      <div class="quick-actions">
        <button type="button" @click="quickNote"><span>✎</span>Nota</button>
        <button type="button" @click="quickEmail" :disabled="!contact.email"><span>✉</span>E-mail</button>
        <button type="button" @click="openCall"><span>☎</span>Chamada</button>
        <button type="button" @click="openTask"><span>✓</span>Tarefa</button>
        <button type="button" @click="openMeeting"><span>⚑</span>Reunião</button>
      </div>

      <div class="about card">
        <div class="about-head">
          <h2>Sobre esse contato</h2>
          <div>
            <button class="btn btn-outline btn-sm" type="button" @click="openEdit">Editar</button>
            <button class="btn btn-danger btn-sm" type="button" @click="remove">✕</button>
          </div>
        </div>
        <dl>
          <dt>E-mail</dt>
          <dd>{{ contact.email || '—' }}</dd>
          <dt>Telefone</dt>
          <dd>
            <a v-if="contact.phone" :href="`tel:${contact.phone}`">{{ contact.phone }}</a>
            <template v-else>—</template>
          </dd>
          <dt>Proprietário</dt>
          <dd :class="{ muted: !contact.owner_name }">{{ contact.owner_name || 'Nenhum proprietário' }}</dd>
          <dt>Último contato</dt>
          <dd>{{ contact.last_activity_at ? relativeDate(contact.last_activity_at) : '—' }}</dd>
          <dt>Fase do ciclo de vida</dt>
          <dd><span class="badge">{{ lifecycleLabels[contact.lifecycle_stage] || contact.lifecycle_stage }}</span></dd>
          <dt>Fonte do registro</dt>
          <dd>{{ contact.source || '—' }}</dd>
          <dt>Papel na decisão</dt>
          <dd>
            <span v-if="contact.buying_role" class="badge" :class="contact.buying_role === 'decisor' ? 'green' : ''">
              {{ buyingRoleLabels[contact.buying_role] || contact.buying_role }}
            </span>
            <span v-else class="muted">—</span>
          </dd>
          <dt>Criado em</dt>
          <dd>{{ formatDate(contact.created_at) }}</dd>
        </dl>
      </div>

      <CustomProperties entity="contacts" :record-id="id" />
    </aside>

    <!-- ===== Coluna central: abas ===== -->
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
              <span class="hl-value">{{ openDeals.length }}</span>
              <span class="hl-label">negócios abertos</span>
              <span class="hl-sub">{{ formatMoney(openDeals.reduce((s, d) => s + d.amount, 0)) }}</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ pendingTasks.length }}</span>
              <span class="hl-label">tarefas pendentes</span>
            </div>
            <div class="highlight">
              <span class="hl-value">{{ openTickets.length }}</span>
              <span class="hl-label">tickets abertos</span>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card-head">
            <h2>Interações recentes</h2>
            <button class="btn btn-outline btn-sm" type="button" @click="centerTab = 'atividades'">Criar atividade</button>
          </div>
          <p v-if="!recentActivities.length" class="empty-state">
            <strong>Nenhuma atividade neste registro.</strong>
            Registre uma nota, chamada ou reunião pelas ações rápidas.
          </p>
          <ul v-else class="recent">
            <li v-for="a in recentActivities" :key="a.id">
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

      <TimelinePanel
        v-else
        ref="timeline"
        :contact-id="id"
        :email-contact-id="contact.email ? id : undefined"
      />
    </section>

    <!-- ===== Coluna direita: associações ===== -->
    <aside class="right">
      <div class="assoc card">
        <div class="assoc-head">
          <h2>Empresas ({{ contact.company_id ? 1 : 0 }})</h2>
        </div>
        <div v-if="contact.company_id" class="assoc-item company">
          <router-link :to="`/empresas/${contact.company_id}`"><strong>{{ contact.company_name }}</strong></router-link>
          <span class="badge green">Principal</span>
        </div>
        <p v-else class="muted">Vincule uma empresa editando o contato.</p>
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
        <p v-if="!tickets.length" class="muted">Acompanhe as solicitações do cliente associadas a este registro.</p>
        <ul class="assoc-list">
          <li v-for="t in tickets" :key="t.id">
            <router-link :to="`/tickets/${t.id}`">#{{ t.id }} {{ t.subject }}</router-link>
            <span class="assoc-meta">
              <span class="badge" :class="ticketBadge[t.status]">{{ t.status }}</span>
            </span>
          </li>
        </ul>
      </div>

      <div class="assoc card">
        <div class="assoc-head">
          <h2>Tarefas ({{ pendingTasks.length }})</h2>
          <button type="button" class="add-link" @click="openTask">+ Adicionar</button>
        </div>
        <p v-if="!pendingTasks.length" class="muted">Nenhuma tarefa pendente para este contato.</p>
        <ul class="assoc-list">
          <li v-for="t in pendingTasks.slice(0, 5)" :key="t.id">
            <span>{{ t.title }}</span>
            <span class="assoc-meta muted" v-if="t.due_date">{{ formatDate(t.due_date) }}</span>
          </li>
        </ul>
      </div>
      <AttachmentsPanel entity="contato" :entity-id="id" />
    </aside>

    <!-- ===== Modais ===== -->
    <ModalDialog title="Editar contato" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="saveEdit">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.first_name" required />
          </div>
          <div class="field">
            <label>Sobrenome</label>
            <input v-model="form.last_name" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>E-mail</label>
            <input v-model="form.email" type="email" />
          </div>
          <div class="field">
            <label>Telefone</label>
            <input v-model="form.phone" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Cargo</label>
            <input v-model="form.job_title" />
          </div>
          <div class="field">
            <label>Origem</label>
            <input v-model="form.source" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Estágio</label>
            <select v-model="form.lifecycle_stage">
              <option v-for="(label, key) in lifecycleLabels" :key="key" :value="key">{{ label }}</option>
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
            <label>Papel na decisão</label>
            <select v-model="form.buying_role">
              <option value="">Não definido</option>
              <option v-for="(label, key) in buyingRoleLabels" :key="key" :value="key">{{ label }}</option>
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
        <div class="form-row">
          <div class="field">
            <label>Direção</label>
            <select v-model="callForm.direction">
              <option value="saida">Saída (liguei)</option>
              <option value="entrada">Entrada (recebi)</option>
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
          <label>Duração (segundos)</label>
          <input v-model.number="callForm.duration_seconds" type="number" min="0" />
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
        <div class="field">
          <label>Prioridade</label>
          <select v-model="ticketForm.priority">
            <option value="baixa">Baixa</option>
            <option value="media">Média</option>
            <option value="alta">Alta</option>
          </select>
        </div>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Criar ticket' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.record {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr) 300px;
  gap: 0;
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
  .left {
    border-right: none !important;
  }
}

/* ===== Coluna esquerda ===== */
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
}

.avatar-xl {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: var(--fix-purple);
  color: #fff;
  font-size: 26px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 10px;
}

.profile h1 {
  font-size: 20px;
}

.profile p {
  margin: 4px 0 0;
  font-size: 13px;
}

.email-line {
  border: none;
  background: none;
  color: var(--fix-purple);
  font-size: 13px;
  cursor: pointer;
  margin-top: 6px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-actions {
  display: flex;
  justify-content: center;
  gap: 8px;
  padding-bottom: 18px;
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
  margin-bottom: 12px;
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

/* ===== Coluna central ===== */
.center {
  padding: 0 22px 22px;
  min-width: 0;
}

.center-tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--fix-border);
  margin-bottom: 18px;
  background: var(--fix-bg);
  position: sticky;
  top: 0;
  z-index: 5;
  padding-top: 14px;
}

.center-tabs button {
  padding: 10px 18px;
  border: none;
  background: none;
  font-size: 14px;
  color: var(--fix-text-2);
  cursor: pointer;
  border-bottom: 2px solid transparent;
}

.center-tabs button.active {
  color: var(--fix-purple);
  border-bottom-color: var(--fix-purple);
  font-weight: 600;
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
  margin-bottom: 8px;
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
  display: flex;
  flex-direction: column;
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

.recent .dot {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
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

.task-check {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

/* ===== Coluna direita ===== */
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

.assoc-item.company {
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
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 12px;
  color: var(--fix-text-3);
}
</style>
