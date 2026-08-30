<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDateTime, initials, relativeDate } from '../format'
import { useToastStore } from '../stores/toast'
import type { Conversation, ConversationMessage } from '../types'

const toast = useToastStore()

const conversations = ref<Conversation[]>([])
const unread = ref(0)
const statusFilter = ref('aberta')
const selected = ref<Conversation | null>(null)
const messages = ref<ConversationMessage[]>([])
const reply = ref('')
const sending = ref(false)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const query = statusFilter.value ? `?status=${statusFilter.value}` : ''
    const resp = await api.get<{ data: Conversation[]; unread: number }>(`/conversations${query}`)
    conversations.value = resp.data
    unread.value = resp.unread
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function open(conv: Conversation) {
  try {
    const resp = await api.get<{ conversation: Conversation; messages: ConversationMessage[] }>(
      `/conversations/${conv.id}`
    )
    selected.value = resp.conversation
    messages.value = resp.messages
    conversations.value = conversations.value.map((c) => (c.id === conv.id ? { ...c, unread: false } : c))
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function sendReply() {
  if (!selected.value || !reply.value.trim()) return
  sending.value = true
  try {
    const msg = await api.post<ConversationMessage>(`/conversations/${selected.value.id}/reply`, {
      body: reply.value.trim()
    })
    messages.value.push(msg)
    reply.value = ''
    toast.push('Resposta enviada')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    sending.value = false
  }
}

async function setStatus(status: 'aberta' | 'fechada') {
  if (!selected.value) return
  try {
    selected.value = await api.patch<Conversation>(`/conversations/${selected.value.id}/status`, { status })
    toast.push(status === 'fechada' ? 'Conversa fechada' : 'Conversa reaberta')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="inbox">
    <aside class="conv-list">
      <div class="conv-head">
        <h1>Caixa de entrada <span v-if="unread" class="badge red">{{ unread }}</span></h1>
        <select v-model="statusFilter" @change="load">
          <option value="aberta">Abertas</option>
          <option value="fechada">Fechadas</option>
          <option value="">Todas</option>
        </select>
      </div>

      <div v-if="!loading && !conversations.length" class="empty-state">
        <strong>Nenhuma conversa</strong>
        E-mails recebidos pelo Mandrill (rota de entrada) aparecem aqui.
      </div>

      <button
        v-for="c in conversations"
        :key="c.id"
        type="button"
        class="conv-item"
        :class="{ active: selected?.id === c.id, unread: c.unread }"
        @click="open(c)"
      >
        <span class="avatar">{{ initials(c.contact_name || c.peer_email) }}</span>
        <span class="conv-info">
          <span class="conv-top">
            <strong>{{ c.contact_name || c.peer_email }}</strong>
            <small class="muted">{{ relativeDate(c.last_message_at) }}</small>
          </span>
          <span class="conv-subject">{{ c.subject }}</span>
          <span class="conv-preview muted">{{ c.last_preview }}</span>
        </span>
      </button>
    </aside>

    <section class="thread" v-if="selected">
      <header class="thread-head">
        <div>
          <h2>{{ selected.subject }}</h2>
          <p class="muted">
            {{ selected.peer_email }}
            <router-link v-if="selected.contact_id" :to="`/contatos/${selected.contact_id}`">
              · ver contato
            </router-link>
          </p>
        </div>
        <button v-if="selected.status === 'aberta'" class="btn btn-outline btn-sm" @click="setStatus('fechada')">
          Fechar conversa
        </button>
        <button v-else class="btn btn-outline btn-sm" @click="setStatus('aberta')">Reabrir</button>
      </header>

      <div class="messages">
        <article v-for="m in messages" :key="m.id" class="message" :class="m.direction">
          <div class="message-meta muted">
            <strong>{{ m.direction === 'recebida' ? m.from_email : m.user_name || 'Fix CRM' }}</strong>
            {{ formatDateTime(m.created_at) }}
          </div>
          <p>{{ m.body }}</p>
        </article>
      </div>

      <footer class="reply-box">
        <textarea v-model="reply" rows="3" placeholder="Escreva sua resposta… (enviada por e-mail via Mandrill)"></textarea>
        <button class="btn btn-primary" type="button" :disabled="sending || !reply.trim()" @click="sendReply">
          {{ sending ? 'Enviando…' : 'Responder' }}
        </button>
      </footer>
    </section>

    <section v-else class="thread empty">
      <div class="empty-state">
        <strong>Selecione uma conversa</strong>
        As mensagens do thread aparecem aqui.
      </div>
    </section>
  </div>
</template>

<style scoped>
.inbox {
  display: grid;
  grid-template-columns: 340px 1fr;
  height: 100%;
}

.conv-list {
  border-right: 1px solid var(--fix-border);
  overflow-y: auto;
  background: var(--fix-surface);
}

.conv-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 18px 16px 12px;
}

.conv-head h1 {
  font-size: 17px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.conv-head select {
  padding: 6px 8px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  font-size: 13px;
}

.conv-item {
  display: flex;
  gap: 10px;
  width: 100%;
  padding: 12px 16px;
  border: none;
  border-bottom: 1px solid var(--fix-bg);
  background: none;
  cursor: pointer;
  text-align: left;
}

.conv-item:hover {
  background: var(--fix-purple-tint);
}

.conv-item.active {
  background: var(--fix-purple-tint);
  border-left: 3px solid var(--fix-purple);
}

.avatar {
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

.conv-info {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.conv-top {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
}

.conv-subject {
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.conv-item.unread .conv-subject {
  font-weight: 700;
}

.conv-preview {
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.thread {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.thread.empty {
  align-items: center;
  justify-content: center;
}

.thread-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 22px;
  border-bottom: 1px solid var(--fix-border);
  background: var(--fix-surface);
}

.thread-head h2 {
  font-size: 16px;
}

.thread-head p {
  margin: 2px 0 0;
  font-size: 13px;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 20px 22px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.message {
  max-width: 70%;
  padding: 12px 14px;
  border-radius: 12px;
  background: var(--fix-surface);
  box-shadow: var(--shadow);
}

.message.enviada {
  align-self: flex-end;
  background: var(--fix-purple-tint);
}

.message-meta {
  font-size: 12px;
  margin-bottom: 4px;
  display: flex;
  gap: 8px;
}

.message p {
  margin: 0;
  font-size: 14px;
  white-space: pre-wrap;
  word-break: break-word;
}

.reply-box {
  display: flex;
  gap: 10px;
  padding: 14px 22px;
  border-top: 1px solid var(--fix-border);
  background: var(--fix-surface);
}

.reply-box textarea {
  flex: 1;
  padding: 10px 12px;
  border: 1px solid var(--fix-border);
  border-radius: 10px;
  font-family: inherit;
  font-size: 14px;
  resize: vertical;
  outline: none;
}

.reply-box textarea:focus {
  border-color: var(--fix-purple);
}

@media (max-width: 900px) {
  .inbox {
    grid-template-columns: 1fr;
  }
  .thread {
    display: none;
  }
  .thread:has(.messages) {
    display: flex;
  }
}
</style>
