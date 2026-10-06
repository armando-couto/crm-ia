<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatDateTime, lifecycleLabels, relativeDate } from '../format'
import type { Contact, Conversation, ConversationMessage, User } from '../types'

const auth = useAuthStore()
const toast = useToastStore()

type QueueKey = 'nao_atribuido' | 'minhas' | 'todas' | 'fechadas'

const queue = ref<QueueKey>('todas')
const conversations = ref<Conversation[]>([])
const counters = ref({ unassigned: 0, mine: 0, open: 0, closed: 0 })
const users = ref<User[]>([])
const loading = ref(true)

const current = ref<Conversation | null>(null)
const messages = ref<ConversationMessage[]>([])
const contact = ref<Contact | null>(null)

// Composer com as abas Responder / Comentário (interno).
const composerTab = ref<'responder' | 'comentario'>('responder')
const body = ref('')
const sending = ref(false)

const queues: { key: QueueKey; label: string }[] = [
  { key: 'nao_atribuido', label: 'Não atribuído' },
  { key: 'minhas', label: 'Atribuído a mim' },
  { key: 'todas', label: 'Tudo aberto' },
  { key: 'fechadas', label: 'Fechadas' }
]

function counterOf(key: QueueKey): number {
  if (key === 'nao_atribuido') return counters.value.unassigned
  if (key === 'minhas') return counters.value.mine
  if (key === 'fechadas') return counters.value.closed
  return counters.value.open
}

const canReply = computed(() => auth.can('inbox.reply'))

async function load() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (queue.value === 'fechadas') params.set('status', 'fechada')
    else {
      params.set('status', 'aberta')
      if (queue.value !== 'todas') params.set('queue', queue.value)
    }
    const resp = await api.get<{ data: Conversation[]; counters: typeof counters.value }>(
      `/conversations?${params.toString()}`
    )
    conversations.value = resp.data ?? []
    counters.value = resp.counters
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function switchQueue(key: QueueKey) {
  queue.value = key
  current.value = null
  load()
}

async function open(conv: Conversation) {
  try {
    const resp = await api.get<{ conversation: Conversation; messages: ConversationMessage[] }>(
      `/conversations/${conv.id}`
    )
    current.value = resp.conversation
    messages.value = resp.messages ?? []
    conv.unread = false
    body.value = ''
    composerTab.value = 'responder'

    // A ficha do contato no painel direito, quando o remetente é conhecido.
    contact.value = null
    if (resp.conversation.contact_id) {
      try {
        contact.value = await api.get<Contact>(`/contacts/${resp.conversation.contact_id}`)
      } catch {
        contact.value = null
      }
    }
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function assign(ownerId: number | null) {
  if (!current.value) return
  try {
    current.value = await api.patch<Conversation>(`/conversations/${current.value.id}/owner`, {
      owner_id: ownerId
    })
    toast.push(ownerId ? 'Conversa atribuída' : 'Conversa devolvida para a fila')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function send() {
  if (!current.value || !body.value.trim()) return
  sending.value = true
  try {
    if (composerTab.value === 'comentario') {
      const msg = await api.post<ConversationMessage>(
        `/conversations/${current.value.id}/comments`,
        { body: body.value.trim() }
      )
      messages.value.push(msg)
      toast.push('Comentário registrado (só a equipe vê)')
    } else {
      const msg = await api.post<ConversationMessage>(
        `/conversations/${current.value.id}/reply`,
        { body: body.value.trim() }
      )
      messages.value.push(msg)
      toast.push('Resposta enviada')
    }
    body.value = ''
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    sending.value = false
  }
}

async function toggleStatus() {
  if (!current.value) return
  const status = current.value.status === 'aberta' ? 'fechada' : 'aberta'
  try {
    current.value = await api.patch<Conversation>(`/conversations/${current.value.id}/status`, {
      status
    })
    toast.push(status === 'fechada' ? 'Conversa fechada' : 'Conversa reaberta')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(async () => {
  try {
    users.value = await api.get<User[]>('/users')
  } catch {
    users.value = []
  }
  await load()
})
</script>

<template>
  <div class="inbox">
    <!-- ===== Filas ===== -->
    <aside class="queues">
      <h1>Caixa de entrada</h1>
      <button
        v-for="q in queues"
        :key="q.key"
        type="button"
        :class="{ active: queue === q.key }"
        @click="switchQueue(q.key)"
      >
        <span>{{ q.label }}</span>
        <span class="count">{{ counterOf(q.key) }}</span>
      </button>
    </aside>

    <!-- ===== Lista ===== -->
    <section class="list">
      <p v-if="loading" class="muted pad">Carregando…</p>
      <p v-else-if="!conversations.length" class="muted pad">Nenhuma conversa nesta fila.</p>
      <button
        v-for="conv in conversations"
        :key="conv.id"
        type="button"
        class="conv"
        :class="{ active: current?.id === conv.id, unread: conv.unread }"
        @click="open(conv)"
      >
        <div class="conv-head">
          <strong>{{ conv.contact_name || conv.peer_email }}</strong>
          <span class="muted when">{{ relativeDate(conv.last_message_at) }}</span>
        </div>
        <span class="subject">{{ conv.subject }}</span>
        <span class="muted preview">{{ conv.last_preview }}</span>
        <span v-if="conv.owner_name" class="muted owner-tag">→ {{ conv.owner_name }}</span>
      </button>
    </section>

    <!-- ===== Conversa ===== -->
    <section class="thread">
      <div v-if="!current" class="muted empty-thread">
        Escolha uma conversa para ler e responder.
      </div>

      <template v-else>
        <header class="thread-head">
          <div>
            <strong>{{ current.contact_name || current.peer_email }}</strong>
            <div class="owner-row">
              <span class="muted small">Proprietário</span>
              <select
                :value="current.owner_id ?? ''"
                :disabled="!canReply"
                @change="assign(Number(($event.target as HTMLSelectElement).value) || null)"
              >
                <option value="">Nenhum proprietário</option>
                <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
              </select>
            </div>
          </div>
          <button v-if="canReply" class="btn" type="button" @click="toggleStatus">
            {{ current.status === 'aberta' ? '✓ Fechar conversa' : 'Reabrir' }}
          </button>
        </header>

        <div class="msgs">
          <div
            v-for="m in messages"
            :key="m.id"
            class="msg"
            :class="{ out: m.direction === 'enviada', note: m.direction === 'comentario' }"
          >
            <div class="msg-head">
              <strong>
                {{ m.direction === 'recebida' ? (current.contact_name || m.from_email) : m.user_name || 'Equipe' }}
              </strong>
              <span v-if="m.direction === 'comentario'" class="badge amber">comentário interno</span>
              <span class="muted small">{{ formatDateTime(m.created_at) }}</span>
            </div>
            <p>{{ m.body }}</p>
          </div>
        </div>

        <div v-if="canReply" class="composer">
          <nav class="composer-tabs">
            <button type="button" :class="{ active: composerTab === 'responder' }" @click="composerTab = 'responder'">
              ✉ Responder
            </button>
            <button type="button" :class="{ active: composerTab === 'comentario' }" @click="composerTab = 'comentario'">
              ✎ Comentário
            </button>
          </nav>
          <textarea
            v-model="body"
            rows="3"
            :placeholder="composerTab === 'comentario' ? 'Nota interna: o contato não vê…' : 'Escreva a resposta…'"
            :class="{ 'note-bg': composerTab === 'comentario' }"
          ></textarea>
          <div class="composer-actions">
            <button class="btn btn-primary" type="button" :disabled="sending || !body.trim()" @click="send">
              {{ sending ? 'Enviando…' : composerTab === 'comentario' ? 'Registrar comentário' : 'Enviar resposta' }}
            </button>
          </div>
        </div>
      </template>
    </section>

    <!-- ===== Ficha do contato ===== -->
    <aside v-if="current" class="profile">
      <template v-if="contact">
        <router-link :to="`/contatos/${contact.id}`" class="profile-name">
          {{ contact.first_name }} {{ contact.last_name }} ↗
        </router-link>
        <h2>Sobre esse contato</h2>
        <dl>
          <dt>E-mail</dt>
          <dd>{{ contact.email || '—' }}</dd>
          <dt>Telefone</dt>
          <dd>{{ contact.phone || '—' }}</dd>
          <dt>Proprietário</dt>
          <dd>{{ contact.owner_name || 'Nenhum proprietário' }}</dd>
          <dt>Último contato</dt>
          <dd>{{ contact.last_activity_at ? relativeDate(contact.last_activity_at) : '—' }}</dd>
          <dt>Fase do ciclo de vida</dt>
          <dd><span class="badge">{{ lifecycleLabels[contact.lifecycle_stage] || contact.lifecycle_stage }}</span></dd>
          <dt>Fonte do registro</dt>
          <dd>{{ contact.source || '—' }}</dd>
          <dt v-if="contact.company_name">Empresa</dt>
          <dd v-if="contact.company_name">
            <router-link :to="`/empresas/${contact.company_id}`">{{ contact.company_name }}</router-link>
          </dd>
        </dl>
      </template>
      <template v-else>
        <h2>Remetente</h2>
        <p class="muted">{{ current.peer_email }}</p>
        <p class="muted small">
          Este e-mail ainda não é um contato do CRM. Crie o contato para ver a ficha aqui.
        </p>
      </template>
    </aside>
  </div>
</template>

<style scoped>
.inbox {
  display: grid;
  grid-template-columns: 180px 300px minmax(0, 1fr) 260px;
  height: 100%;
  min-height: calc(100vh - 56px);
}

.queues {
  border-right: 1px solid var(--fix-border);
  padding: 18px 10px;
  background: var(--fix-surface);
}

.queues h1 {
  font-size: 15px;
  padding: 0 8px 12px;
}

.queues button {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: none;
  border: none;
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 13px;
  color: var(--fix-text-2);
  cursor: pointer;
  margin-bottom: 2px;
}

.queues button:hover {
  background: var(--fix-purple-tint);
}

.queues button.active {
  background: var(--fix-purple);
  color: #fff;
  font-weight: 500;
}

.count {
  font-variant-numeric: tabular-nums;
  font-size: 12px;
}

.list {
  border-right: 1px solid var(--fix-border);
  overflow-y: auto;
  background: var(--fix-surface);
}

.pad {
  padding: 18px;
}

.conv {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  border-bottom: 1px solid var(--fix-border);
  padding: 12px 14px;
  cursor: pointer;
  font-size: 13px;
  color: var(--fix-text);
}

.conv:hover {
  background: var(--fix-bg);
}

.conv.active {
  background: var(--fix-purple-tint);
}

.conv.unread strong {
  color: var(--fix-purple-dark);
}

.conv-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.when {
  font-size: 11px;
  white-space: nowrap;
}

.subject {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.owner-tag {
  font-size: 11px;
}

.thread {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.empty-thread {
  margin: auto;
}

.thread-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--fix-border);
  background: var(--fix-surface);
}

.owner-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
}

.owner-row select {
  padding: 4px 8px;
  border: 1px solid var(--fix-border);
  border-radius: 6px;
  background: var(--fix-surface);
  font-size: 12px;
}

.msgs {
  flex: 1;
  overflow-y: auto;
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.msg {
  max-width: 640px;
  background: var(--fix-surface);
  border: 1px solid var(--fix-border);
  border-radius: 12px;
  padding: 12px 14px;
}

.msg.out {
  align-self: flex-end;
  background: var(--fix-purple-tint);
}

.msg.note {
  background: var(--fix-amber-tint, #fdf3d7);
  border-color: var(--fix-amber, #d99e2b);
}

.msg-head {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: 12px;
  margin-bottom: 4px;
  flex-wrap: wrap;
}

.msg p {
  margin: 0;
  font-size: 14px;
  white-space: pre-wrap;
}

.small {
  font-size: 11px;
}

.composer {
  border-top: 1px solid var(--fix-border);
  background: var(--fix-surface);
  padding: 0 18px 14px;
}

.composer-tabs {
  display: flex;
  gap: 4px;
}

.composer-tabs button {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 10px 12px;
  font-size: 13px;
  color: var(--fix-text-2);
  cursor: pointer;
}

.composer-tabs button.active {
  color: var(--fix-purple-dark);
  border-bottom-color: var(--fix-purple);
  font-weight: 500;
}

.composer textarea {
  width: 100%;
  margin-top: 8px;
}

.composer textarea.note-bg {
  background: var(--fix-amber-tint, #fdf3d7);
}

.composer-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 8px;
}

.profile {
  border-left: 1px solid var(--fix-border);
  padding: 18px 16px;
  background: var(--fix-surface);
  overflow-y: auto;
}

.profile-name {
  font-weight: 600;
  color: var(--fix-purple-dark);
  display: block;
  margin-bottom: 14px;
}

.profile h2 {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--fix-text-3);
  margin-bottom: 10px;
}

.profile dl {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  font-size: 13px;
}

.profile dt {
  color: var(--fix-text-3);
  font-size: 11px;
  margin-top: 8px;
}

.profile dd {
  margin: 0;
}

@media (max-width: 1100px) {
  .inbox {
    grid-template-columns: 160px 260px minmax(0, 1fr);
  }
  .profile {
    display: none;
  }
}
</style>
