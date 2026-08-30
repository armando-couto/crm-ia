<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatDate, roleLabels } from '../format'
import { useToastStore } from '../stores/toast'
import { useAuthStore } from '../stores/auth'
import ModalDialog from '../components/ModalDialog.vue'
import type { User } from '../types'

const toast = useToastStore()
const auth = useAuthStore()

const users = ref<User[]>([])
const loading = ref(false)

const modalOpen = ref(false)
const saving = ref(false)
const editing = ref<User | null>(null)
const form = ref({ name: '', email: '', role: 'vendedor', active: true })

async function load() {
  loading.value = true
  try {
    users.value = await api.get<User[]>('/users')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

function openNew() {
  editing.value = null
  form.value = { name: '', email: '', role: 'vendedor', active: true }
  modalOpen.value = true
}

function openEdit(user: User) {
  editing.value = user
  form.value = { name: user.name, email: user.email, role: user.role, active: user.active }
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
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Usuários</h1>
      <button class="btn btn-primary" type="button" @click="openNew">+ Novo usuário</button>
    </div>

    <div class="table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Nome</th>
            <th>E-mail</th>
            <th>Papel</th>
            <th>Status</th>
            <th>Criado</th>
            <th style="width: 150px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" style="cursor: default">
            <td><strong>{{ u.name }}</strong></td>
            <td>{{ u.email }}</td>
            <td><span class="badge" :class="u.role === 'admin' ? '' : u.role === 'gestor' ? 'blue' : 'gray'">{{ roleLabels[u.role] }}</span></td>
            <td><span class="badge" :class="u.active ? 'green' : 'red'">{{ u.active ? 'Ativo' : 'Inativo' }}</span></td>
            <td class="muted">{{ formatDate(u.created_at) }}</td>
            <td>
              <button class="btn btn-outline btn-sm" type="button" @click="openEdit(u)">Editar</button>
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

    <ModalDialog :title="editing ? 'Editar usuário' : 'Novo usuário'" :open="modalOpen" @close="modalOpen = false">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome *</label>
          <input v-model="form.name" required />
        </div>
        <div class="field">
          <label>E-mail *</label>
          <input v-model="form.email" type="email" required />
        </div>
        <div class="field">
          <label>Papel</label>
          <select v-model="form.role">
            <option value="vendedor">Vendedor</option>
            <option value="gestor">Gestor</option>
            <option value="admin">Administrador</option>
          </select>
        </div>
        <div class="field" v-if="editing">
          <label class="check-inline">
            <input v-model="form.active" type="checkbox" />
            Usuário ativo
          </label>
        </div>
        <p class="muted" v-if="!editing" style="font-size: 13px">
          O novo usuário receberá um e-mail (via Mandrill) com a senha temporária de acesso.
        </p>
        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : editing ? 'Salvar alterações' : 'Criar e enviar convite' }}
        </button>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.check-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
</style>
