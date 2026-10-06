<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate, initials, roleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import { useAuthStore } from '../stores/auth'
import ModalDialog from '../components/ModalDialog.vue'
import type { Team, User } from '../types'

const toast = useToastStore()
const auth = useAuthStore()

const tab = ref<'usuarios' | 'equipes'>('usuarios')
const users = ref<User[]>([])
const teams = ref<Team[]>([])
const loading = ref(false)

const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<User | null>(null)
const form = ref({ name: '', email: '', role: 'seller', active: true, team_id: null as number | null })

async function load() {
  loading.value = true
  try {
    const [usersResp, teamsResp] = await Promise.all([api.get<User[]>('/users'), api.get<Team[]>('/teams')])
    users.value = usersResp
    teams.value = teamsResp
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

// ===== Usuários =====
function openNew() {
  editing.value = null
  form.value = { name: '', email: '', role: 'seller', active: true, team_id: null }
  modalOpen.value = true
}

function openEdit(user: User) {
  editing.value = user
  form.value = { name: user.name, email: user.email, role: user.role, active: user.active, team_id: user.team_id ?? null }
  modalOpen.value = true
}

async function save() {
  saving.value = true
  try {
    if (editing.value) {
      await api.put(`/users/${editing.value.id}`, form.value)
      toast.push('Usuário atualizado')
    } else {
      await api.post('/users', form.value)
      toast.push('Usuário criado — a senha temporária foi enviada por e-mail')
    }
    modalOpen.value = false
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

async function resendInvite(user: User) {
  if (
    !confirm(
      `Enviar uma senha nova para ${user.email}?\n\n` +
        'A senha atual deixa de valer na hora e as sessões abertas caem.'
    )
  )
    return
  try {
    const resp = await api.post<{ message: string }>(`/users/${user.id}/resend-invite`)
    toast.push(resp.message)
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function deactivate(user: User) {
  if (!confirm(`Desativar o acesso de ${user.name}? O histórico será mantido.`)) return
  try {
    await api.delete(`/users/${user.id}`)
    toast.push('Usuário desativado')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ===== Equipes =====
function teamMembers(teamId: number): User[] {
  return users.value.filter((u) => u.team_id === teamId && u.active)
}

async function createTeam() {
  const name = prompt('Nome da equipe (ex.: Comercial - Closers):')
  if (!name?.trim()) return
  try {
    await api.post('/teams', { name: name.trim() })
    toast.push('Equipe criada — atribua os membros editando cada usuário')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function renameTeam(team: Team) {
  const name = prompt('Novo nome da equipe:', team.name)
  if (!name?.trim() || name.trim() === team.name) return
  try {
    await api.put(`/teams/${team.id}`, { name: name.trim() })
    toast.push('Equipe renomeada')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function removeTeam(team: Team) {
  if (!confirm(`Excluir a equipe "${team.name}"? Os membros ficam sem equipe.`)) return
  try {
    await api.delete(`/teams/${team.id}`)
    toast.push('Equipe excluída')
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

const activeUsers = computed(() => users.value.filter((u) => u.active).length)

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Usuários e equipes</h1>
        <p class="muted" style="margin: 4px 0 0">{{ activeUsers }} usuário(s) ativo(s) · {{ teams.length }} equipe(s)</p>
      </div>
      <button v-if="tab === 'usuarios'" class="btn btn-primary" type="button" @click="openNew">+ Criar usuário</button>
      <button v-else class="btn btn-primary" type="button" @click="createTeam">+ Criar equipe</button>
    </div>

    <div class="tabs">
      <button type="button" class="tab" :class="{ active: tab === 'usuarios' }" @click="tab = 'usuarios'">Usuários</button>
      <button type="button" class="tab" :class="{ active: tab === 'equipes' }" @click="tab = 'equipes'">Equipes</button>
    </div>

    <!-- ===== Usuários ===== -->
    <div class="table-wrap" v-if="tab === 'usuarios'">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>E-mail</th>
            <th>Papel</th>
            <th>Equipe principal</th>
            <th>Status</th>
            <th>Criado</th>
            <th style="width: 290px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" style="cursor: default">
            <td><strong>{{ u.name }}</strong></td>
            <td>{{ u.email }}</td>
            <td><span class="badge" :class="u.role === 'admin' ? '' : u.role === 'manager' ? 'blue' : 'gray'">{{ roleLabels[u.role] }}</span></td>
            <td :class="{ muted: !u.team_name }">{{ u.team_name || '—' }}</td>
            <td><span class="badge" :class="u.active ? 'green' : 'red'">{{ u.active ? 'Ativo' : 'Inativo' }}</span></td>
            <td class="muted">{{ formatDate(u.created_at) }}</td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="openEdit(u)">Editar</button>
              <button
                v-if="u.active && u.id !== auth.user?.id"
                class="btn btn-outline btn-sm"
                type="button"
                title="Gera uma senha temporária nova e reenvia o e-mail de acesso"
                @click="resendInvite(u)"
              >
                Reenviar senha
              </button>
              <button
                v-if="u.active && u.id !== auth.user?.id"
                class="btn btn-danger btn-sm"
                type="button"
                @click="deactivate(u)"
              >
                Desativar
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- ===== Equipes ===== -->
    <div class="table-wrap" v-else>
      <table class="data">
        <thead>
          <tr>
            <th>Nome da equipe</th>
            <th>Membros</th>
            <th style="width: 200px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in teams" :key="t.id" style="cursor: default">
            <td><strong>{{ t.name }}</strong></td>
            <td>
              <span class="members">
                <span v-for="m in teamMembers(t.id).slice(0, 6)" :key="m.id" class="member-avatar" :title="m.name">
                  {{ initials(m.name) }}
                </span>
                <span class="muted" v-if="!teamMembers(t.id).length">nenhum membro</span>
                <span class="muted" v-else-if="teamMembers(t.id).length > 6">+{{ teamMembers(t.id).length - 6 }}</span>
              </span>
            </td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="renameTeam(t)">Renomear</button>
              <button class="btn btn-danger btn-sm" type="button" @click="removeTeam(t)">Excluir</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!loading && !teams.length" class="empty-state">
        <strong>Nenhuma equipe criada</strong>
        Use equipes para organizar os usuários (ex.: Comercial, CS, Financeiro) — atribua a equipe editando cada usuário.
      </div>
    </div>

    <ModalDialog :title="editing ? 'Editar usuário' : 'Criar um novo usuário'" :open="modalOpen" @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required />
        </div>
        <div class="field">
          <label>Endereço de e-mail *</label>
          <input v-model="form.email" type="email" required />
        </div>
        <div class="field">
          <label>Permissões</label>
          <select v-model="form.role">
            <option value="seller">Seller</option>
            <option value="manager">Manager</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <div class="field">
          <label>Equipe</label>
          <select v-model="form.team_id">
            <option :value="null">Sem equipe</option>
            <option v-for="t in teams" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
        </div>
        <div class="field" v-if="editing">
          <label class="check-inline">
            <input v-model="form.active" type="checkbox" />
            Usuário ativo
          </label>
        </div>
        <p class="muted" v-if="!editing" style="font-size: 13px">
          ✉ O novo usuário receberá um convite por e-mail (via Mandrill) com a senha temporária de acesso.
        </p>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : editing ? 'Salvar alterações' : 'Adicionar usuário' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid var(--ci-border);
  margin-bottom: 14px;
}

.tab {
  padding: 9px 16px;
  border: none;
  background: none;
  font-size: 14px;
  color: var(--ci-text-2);
  cursor: pointer;
  border-bottom: 2px solid transparent;
}

.tab.active {
  color: var(--ci-purple);
  border-bottom-color: var(--ci-purple);
  font-weight: 600;
}

.members {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.member-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  font-size: 11px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.check-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
</style>
