<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, formatMoney } from '../format'
import { useToastStore } from '../stores/toast'
import TimelinePanel from '../components/TimelinePanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, Deal, Paginated, Pipeline, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const deal = ref<Deal | null>(null)
const pipelines = ref<Pipeline[]>([])
const users = ref<User[]>([])
const contacts = ref<Contact[]>([])
const companies = ref<Company[]>([])

const editOpen = ref(false)
const saving = ref(false)
const form = ref<any>({})

const pipeline = computed(() => pipelines.value.find((p) => p.id === deal.value?.pipeline_id))
const stages = computed(() => pipeline.value?.stages.filter((s) => !s.is_won && !s.is_lost) ?? [])

async function load() {
  try {
    deal.value = await api.get<Deal>(`/deals/${id}`)
    pipelines.value = await api.get<Pipeline[]>('/pipelines')
  } catch (e: any) {
    toast.error(e.message)
    router.push('/negocios')
  }
}

async function moveTo(stageId: number) {
  if (!deal.value || deal.value.stage_id === stageId) return
  try {
    deal.value = await api.patch<Deal>(`/deals/${id}/stage`, { stage_id: stageId, position: 0 })
    toast.push('Etapa atualizada')
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function close(won: boolean) {
  try {
    deal.value = await api.patch<Deal>(`/deals/${id}/close`, { won })
    toast.push(won ? 'Negócio ganho! 🎉' : 'Negócio marcado como perdido')
  } catch (e: any) {
    toast.error(e.message)
  }
}

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

async function save() {
  saving.value = true
  try {
    deal.value = await api.put<Deal>(`/deals/${id}`, { ...form.value, amount: Number(form.value.amount) || 0 })
    toast.push('Negócio atualizado')
    editOpen.value = false
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

onMounted(async () => {
  await load()
  try {
    const [usersResp, contactsResp, companiesResp] = await Promise.all([
      api.get<User[]>('/users'),
      api.get<Paginated<Contact>>('/contacts?per_page=100'),
      api.get<Paginated<Company>>('/companies?per_page=100')
    ])
    users.value = usersResp
    contacts.value = contactsResp.data
    companies.value = companiesResp.data
  } catch {
    /* opcional */
  }
})
</script>

<template>
  <div class="page" v-if="deal">
    <div class="page-head">
      <div>
        <h1>{{ deal.name }}</h1>
        <p class="muted head-sub">
          <span class="badge" :class="deal.status === 'ganho' ? 'green' : deal.status === 'perdido' ? 'red' : 'blue'">
            {{ deal.status === 'aberto' ? deal.stage_name : deal.status }}
          </span>
          <strong class="amount">{{ formatMoney(deal.amount) }}</strong>
        </p>
      </div>
      <div class="toolbar">
        <template v-if="deal.status === 'aberto'">
          <button class="btn btn-outline" style="color: var(--fix-green)" @click="close(true)">✓ Ganho</button>
          <button class="btn btn-outline" style="color: var(--fix-red)" @click="close(false)">✕ Perdido</button>
        </template>
        <button class="btn btn-outline" @click="openEdit">Editar</button>
        <button class="btn btn-danger" @click="remove">Remover</button>
      </div>
    </div>

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

    <div class="layout">
      <div class="side">
        <div class="card">
          <h2>Detalhes</h2>
          <dl>
            <dt>Contato</dt>
            <dd>
              <router-link v-if="deal.contact_id" :to="`/contatos/${deal.contact_id}`">{{ deal.contact_name }}</router-link>
              <template v-else>—</template>
            </dd>
            <dt>Empresa</dt>
            <dd>
              <router-link v-if="deal.company_id" :to="`/empresas/${deal.company_id}`">{{ deal.company_name }}</router-link>
              <template v-else>—</template>
            </dd>
            <dt>Dono</dt>
            <dd>{{ deal.owner_name || '—' }}</dd>
            <dt>Temperatura</dt>
            <dd>
              <template v-if="deal.temperature === 'quente'">🔴 Quente</template>
              <template v-else-if="deal.temperature === 'media'">🟡 Média</template>
              <template v-else-if="deal.temperature === 'fria'">🔵 Fria</template>
              <template v-else>—</template>
            </dd>
            <dt>Previsão</dt>
            <dd>{{ formatDate(deal.close_date) }}</dd>
            <dt>Fechado em</dt>
            <dd>{{ formatDate(deal.closed_at) }}</dd>
            <dt>Criado em</dt>
            <dd>{{ formatDate(deal.created_at) }}</dd>
          </dl>
        </div>
      </div>

      <div class="main-col">
        <TimelinePanel :deal-id="id" :email-contact-id="deal.contact_id || undefined" />
      </div>
    </div>

    <ModalDialog title="Editar negócio" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="save">
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
            <label>Dono</label>
            <select v-model="form.owner_id">
              <option :value="null">Sem dono</option>
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
  </div>
</template>

<style scoped>
.head-sub {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0 0;
}

.amount {
  color: var(--fix-purple-dark);
  font-size: 16px;
}

.stage-track {
  display: flex;
  gap: 6px;
  padding: 10px;
  margin-bottom: 16px;
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

.layout {
  display: grid;
  grid-template-columns: 340px 1fr;
  gap: 16px;
  align-items: start;
}

@media (max-width: 980px) {
  .layout {
    grid-template-columns: 1fr;
  }
}

.card h2 {
  font-size: 14px;
  margin-bottom: 12px;
}

dl {
  margin: 0;
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px 14px;
  font-size: 13px;
}

dt {
  color: var(--fix-text-3);
}

dd {
  margin: 0;
}
</style>
