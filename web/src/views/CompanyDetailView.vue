<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { formatDate, initials, lifecycleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import TimelinePanel from '../components/TimelinePanel.vue'
import ModalDialog from '../components/ModalDialog.vue'
import type { Company, Contact, Deal, Paginated, User } from '../types'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const id = Number(route.params.id)
const company = ref<Company | null>(null)
const contacts = ref<Contact[]>([])
const deals = ref<Deal[]>([])
const users = ref<User[]>([])

const editOpen = ref(false)
const saving = ref(false)
const form = ref<any>({})

async function load() {
  try {
    company.value = await api.get<Company>(`/companies/${id}`)
    const [contactsResp, dealsResp] = await Promise.all([
      api.get<Paginated<Contact>>(`/contacts?company_id=${id}&per_page=100`),
      api.get<Paginated<Deal>>(`/deals?company_id=${id}&per_page=50`)
    ])
    contacts.value = contactsResp.data
    deals.value = dealsResp.data
  } catch (e: any) {
    toast.error(e.message)
    router.push('/empresas')
  }
}

function openEdit() {
  if (!company.value) return
  form.value = { ...company.value }
  editOpen.value = true
}

async function save() {
  saving.value = true
  try {
    company.value = await api.put<Company>(`/companies/${id}`, form.value)
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

onMounted(async () => {
  await load()
  try {
    users.value = await api.get<User[]>('/users')
  } catch {
    /* opcional */
  }
})
</script>

<template>
  <div class="page" v-if="company">
    <div class="page-head">
      <div class="who">
        <span class="avatar-lg">{{ initials(company.name) }}</span>
        <div>
          <h1>{{ company.name }}</h1>
          <p class="muted">
            {{ company.industry || 'Sem segmento' }}
            <template v-if="company.domain"> · {{ company.domain }}</template>
          </p>
        </div>
      </div>
      <div class="toolbar">
        <button class="btn btn-outline" @click="openEdit">Editar</button>
        <button class="btn btn-danger" @click="remove">Remover</button>
      </div>
    </div>

    <div class="layout">
      <div class="side">
        <div class="card">
          <h2>Informações</h2>
          <dl>
            <dt>Telefone</dt>
            <dd>{{ company.phone || '—' }}</dd>
            <dt>Cidade/UF</dt>
            <dd>{{ company.city ? `${company.city}${company.state ? '/' + company.state : ''}` : '—' }}</dd>
            <dt>Dono</dt>
            <dd>{{ company.owner_name || '—' }}</dd>
            <dt>Criada em</dt>
            <dd>{{ formatDate(company.created_at) }}</dd>
          </dl>
        </div>

        <div class="card">
          <h2>Contatos ({{ contacts.length }})</h2>
          <p v-if="!contacts.length" class="muted">Nenhum contato vinculado.</p>
          <ul class="mini-list">
            <li v-for="c in contacts" :key="c.id">
              <router-link :to="`/contatos/${c.id}`">{{ c.first_name }} {{ c.last_name }}</router-link>
              <span class="badge gray">{{ lifecycleLabels[c.lifecycle_stage] || c.lifecycle_stage }}</span>
            </li>
          </ul>
        </div>

        <div class="card">
          <h2>Negócios ({{ deals.length }})</h2>
          <p v-if="!deals.length" class="muted">Nenhum negócio vinculado.</p>
          <ul class="mini-list">
            <li v-for="d in deals" :key="d.id">
              <router-link :to="`/negocios/${d.id}`">{{ d.name }}</router-link>
              <span class="badge" :class="d.status === 'ganho' ? 'green' : d.status === 'perdido' ? 'red' : 'gray'">
                {{ d.stage_name }}
              </span>
            </li>
          </ul>
        </div>
      </div>

      <div class="main-col">
        <TimelinePanel :company-id="id" />
      </div>
    </div>

    <ModalDialog title="Editar empresa" :open="editOpen" wide @close="editOpen = false">
      <form @submit.prevent="save">
        <div class="form-row">
          <div class="field">
            <label>Nome *</label>
            <input v-model="form.name" required />
          </div>
          <div class="field">
            <label>Domínio</label>
            <input v-model="form.domain" />
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>Telefone</label>
            <input v-model="form.phone" />
          </div>
          <div class="field">
            <label>Segmento</label>
            <input v-model="form.industry" />
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
          <label>Dono</label>
          <select v-model="form.owner_id">
            <option :value="null">Sem dono</option>
            <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
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
.who {
  display: flex;
  align-items: center;
  gap: 14px;
}

.avatar-lg {
  width: 52px;
  height: 52px;
  border-radius: 12px;
  background: var(--fix-purple);
  color: #fff;
  font-size: 18px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
}

.who p {
  margin: 2px 0 0;
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

.side {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
  word-break: break-word;
}

.mini-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
}

.mini-list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
</style>
