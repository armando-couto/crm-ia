<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { relativeDate } from '../format'
import type { Activity } from '../types'

const props = defineProps<{
  contactId?: number
  companyId?: number
  dealId?: number
  /** Habilita o formulário de envio de e-mail (requer contato com e-mail). */
  emailContactId?: number
}>()

const toast = useToastStore()
const activities = ref<Activity[]>([])
const loading = ref(false)

const noteContent = ref('')
const noteKind = ref('nota')
const emailOpen = ref(false)
const emailSubject = ref('')
const emailBody = ref('')
const sending = ref(false)

const kindIcons: Record<string, string> = {
  nota: '✎',
  email: '✉',
  ligacao: '☎',
  reuniao: '⚑',
  sistema: '⚙'
}

const kindLabels: Record<string, string> = {
  nota: 'Nota',
  email: 'E-mail',
  ligacao: 'Ligação',
  reuniao: 'Reunião',
  sistema: 'Sistema'
}

function query(): string {
  const params = new URLSearchParams()
  if (props.contactId) params.set('contact_id', String(props.contactId))
  if (props.companyId) params.set('company_id', String(props.companyId))
  if (props.dealId) params.set('deal_id', String(props.dealId))
  return params.toString()
}

async function load() {
  loading.value = true
  try {
    const resp = await api.get<{ data: Activity[] }>(`/activities?${query()}`)
    activities.value = resp.data
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function addNote() {
  if (!noteContent.value.trim()) return
  try {
    await api.post('/activities', {
      kind: noteKind.value,
      content: noteContent.value.trim(),
      contact_id: props.contactId || null,
      company_id: props.companyId || null,
      deal_id: props.dealId || null
    })
    noteContent.value = ''
    toast.push('Atividade registrada')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function sendEmail() {
  if (!props.emailContactId || !emailSubject.value.trim() || !emailBody.value.trim()) return
  sending.value = true
  try {
    await api.post('/emails', {
      contact_id: props.emailContactId,
      subject: emailSubject.value.trim(),
      body: emailBody.value.trim(),
      deal_id: props.dealId || null
    })
    toast.push('E-mail enviado com sucesso')
    emailOpen.value = false
    emailSubject.value = ''
    emailBody.value = ''
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    sending.value = false
  }
}

onMounted(load)
defineExpose({ reload: load })
</script>

<template>
  <div class="timeline">
    <div class="composer card">
      <div class="composer-head">
        <select v-model="noteKind">
          <option value="nota">Nota</option>
          <option value="ligacao">Ligação</option>
          <option value="reuniao">Reunião</option>
        </select>
        <button v-if="emailContactId" class="btn btn-outline btn-sm" type="button" @click="emailOpen = !emailOpen">
          ✉ Enviar e-mail
        </button>
      </div>

      <template v-if="!emailOpen">
        <textarea v-model="noteContent" rows="2" placeholder="Registrar uma nota, ligação ou reunião…"></textarea>
        <div class="composer-actions">
          <button class="btn btn-primary btn-sm" type="button" :disabled="!noteContent.trim()" @click="addNote">
            Registrar
          </button>
        </div>
      </template>

      <template v-else>
        <input v-model="emailSubject" type="text" placeholder="Assunto" />
        <textarea v-model="emailBody" rows="4" placeholder="Mensagem para o contato…"></textarea>
        <div class="composer-actions">
          <button class="btn btn-sm" type="button" @click="emailOpen = false">Cancelar</button>
          <button
            class="btn btn-primary btn-sm"
            type="button"
            :disabled="sending || !emailSubject.trim() || !emailBody.trim()"
            @click="sendEmail"
          >
            {{ sending ? 'Enviando…' : 'Enviar via Mandrill' }}
          </button>
        </div>
      </template>
    </div>

    <div v-if="loading" class="muted">Carregando atividades…</div>
    <div v-else-if="!activities.length" class="empty-state">
      <strong>Nenhuma atividade ainda</strong>
      Registre a primeira interação acima.
    </div>

    <ul v-else class="items">
      <li v-for="a in activities" :key="a.id">
        <span class="dot" :class="a.kind">{{ kindIcons[a.kind] || '•' }}</span>
        <div class="item-body">
          <div class="item-head">
            <span class="kind">{{ kindLabels[a.kind] || a.kind }}</span>
            <span class="muted" v-if="a.user_name">por {{ a.user_name }}</span>
            <span class="muted when">{{ relativeDate(a.created_at) }}</span>
          </div>
          <p>{{ a.content }}</p>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.timeline {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.composer {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.composer-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
}

.composer select,
.composer input,
.composer textarea {
  padding: 8px 10px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  font-size: 14px;
  font-family: inherit;
  outline: none;
  resize: vertical;
}

.composer select:focus,
.composer input:focus,
.composer textarea:focus {
  border-color: var(--fix-purple);
}

.composer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.items {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.items li {
  display: flex;
  gap: 12px;
  padding: 10px 4px;
}

.dot {
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: var(--fix-purple-tint);
  color: var(--fix-purple-dark);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  flex-shrink: 0;
}

.dot.email {
  background: var(--fix-blue-tint);
  color: var(--fix-blue);
}

.dot.sistema {
  background: var(--fix-bg);
  color: var(--fix-text-3);
}

.item-body {
  flex: 1;
  border-bottom: 1px solid var(--fix-bg);
  padding-bottom: 10px;
  min-width: 0;
}

.item-head {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: 13px;
}

.kind {
  font-weight: 600;
}

.when {
  margin-left: auto;
  font-size: 12px;
}

.item-body p {
  margin: 4px 0 0;
  font-size: 14px;
  color: var(--fix-text-2);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
