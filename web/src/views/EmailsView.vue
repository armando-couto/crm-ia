<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'
import { formatDateTime, relativeDate } from '../format'
import ModalDialog from '../components/ModalDialog.vue'
import StatCard from '../components/StatCard.vue'
import type { EmailMessage, EmailEvent } from '../types'

const toast = useToastStore()

const messages = ref<EmailMessage[]>([])
const stats = ref({ sent: 0, opened: 0, clicked: 0, open_rate: 0, click_rate: 0 })
const loading = ref(true)
const days = ref('30')

// Detalhe aberto no modal, com o histórico de aberturas e cliques.
const detail = ref<{ message: EmailMessage; events: EmailEvent[] } | null>(null)

function percent(value: number): string {
  return `${value.toFixed(0)}%`
}

async function load() {
  loading.value = true
  try {
    const [list, resume] = await Promise.all([
      api.get<{ data: EmailMessage[] }>(`/emails/sent?days=${days.value}&limit=100`),
      api.get<typeof stats.value>(`/emails/stats?days=${days.value}`)
    ])
    messages.value = list.data ?? []
    stats.value = resume
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function openDetail(message: EmailMessage) {
  try {
    detail.value = await api.get<{ message: EmailMessage; events: EmailEvent[] }>(
      `/emails/sent/${message.id}`
    )
  } catch (e: any) {
    toast.error(e.message)
  }
}

watch(days, load)
onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>E-mails enviados</h1>
        <p class="muted" style="margin: 4px 0 0">
          Quem abriu e quem clicou nos e-mails que a equipe mandou pelo CRM.
        </p>
      </div>
      <select v-model="days" class="pipeline-select">
        <option value="7">Últimos 7 dias</option>
        <option value="30">Últimos 30 dias</option>
        <option value="90">Últimos 90 dias</option>
        <option value="365">Último ano</option>
      </select>
    </div>

    <div class="stats">
      <StatCard label="Enviados" :value="String(stats.sent)" />
      <StatCard
        label="Abertos"
        :value="String(stats.opened)"
        :hint="`${percent(stats.open_rate)} de abertura`"
        tone="green"
      />
      <StatCard
        label="Com clique"
        :value="String(stats.clicked)"
        :hint="`${percent(stats.click_rate)} de cliques`"
        tone="blue"
      />
    </div>

    <p v-if="loading" class="muted">Carregando envios…</p>

    <div v-else-if="!messages.length" class="card empty">
      <p class="muted">Nenhum e-mail enviado no período.</p>
    </div>

    <div v-else class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Assunto</th>
            <th style="width: 200px">Para</th>
            <th style="width: 130px">Enviado por</th>
            <th style="width: 120px">Situação</th>
            <th style="width: 140px">Quando</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in messages" :key="m.id" class="row" @click="openDetail(m)">
            <td>
              {{ m.subject || '(sem assunto)' }}
              <span v-if="m.source === 'automacao'" class="badge gray">automação</span>
            </td>
            <td>
              <router-link v-if="m.contact_id" :to="`/contatos/${m.contact_id}`" @click.stop>
                {{ m.contact_name || m.to_email }}
              </router-link>
              <template v-else>{{ m.to_email }}</template>
            </td>
            <td class="muted">{{ m.user_name || '—' }}</td>
            <td>
              <span v-if="m.clicks > 0" class="badge blue">{{ m.clicks }} clique(s)</span>
              <span v-else-if="m.opens > 0" class="badge green">Aberto {{ m.opens }}×</span>
              <span v-else class="badge gray">Não aberto</span>
            </td>
            <td class="muted">{{ relativeDate(m.sent_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <ModalDialog title="Detalhes do envio" :open="!!detail" wide @close="detail = null">
      <template v-if="detail">
        <dl class="detail">
          <dt>Assunto</dt>
          <dd>{{ detail.message.subject }}</dd>
          <dt>Para</dt>
          <dd>{{ detail.message.to_email }}</dd>
          <dt>Enviado</dt>
          <dd>{{ formatDateTime(detail.message.sent_at) }}</dd>
          <dt>Aberturas</dt>
          <dd>
            {{ detail.message.opens }}
            <span class="muted" v-if="detail.message.first_open_at">
              · primeira em {{ formatDateTime(detail.message.first_open_at) }}
            </span>
          </dd>
          <dt>Cliques</dt>
          <dd>{{ detail.message.clicks }}</dd>
        </dl>

        <h3 class="events-title">Histórico</h3>
        <p v-if="!detail.events.length" class="muted">
          Nenhuma abertura registrada. Clientes de e-mail que bloqueiam imagens não são contabilizados.
        </p>
        <ul v-else class="events">
          <li v-for="ev in detail.events" :key="ev.id">
            <span class="badge" :class="ev.kind === 'clique' ? 'blue' : 'green'">{{ ev.kind }}</span>
            <span class="muted">{{ formatDateTime(ev.created_at) }}</span>
            <a v-if="ev.url" :href="ev.url" target="_blank" rel="noopener" class="url">{{ ev.url }}</a>
          </li>
        </ul>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.pipeline-select {
  padding: 8px 12px;
  border: 1px solid var(--fix-border);
  border-radius: 8px;
  background: var(--fix-surface);
  font-size: 14px;
  outline: none;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 14px;
  margin-bottom: 20px;
}

.empty {
  text-align: center;
  padding: 32px;
}

.row {
  cursor: pointer;
}

.row:hover {
  background: var(--fix-purple-tint);
}

.detail {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 6px 12px;
  margin: 0;
  font-size: 14px;
}

.detail dt {
  color: var(--fix-text-3);
}

.detail dd {
  margin: 0;
}

.events-title {
  font-size: 14px;
  margin: 18px 0 10px;
  padding-top: 14px;
  border-top: 1px solid var(--fix-border);
}

.events {
  list-style: none;
  margin: 0;
  padding: 0;
  font-size: 13px;
}

.events li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 0;
  border-top: 1px solid var(--fix-border);
}

.events li:first-child {
  border-top: none;
}

.url {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
