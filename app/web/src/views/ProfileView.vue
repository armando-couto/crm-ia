<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'

const auth = useAuthStore()
const toast = useToastStore()

const name = ref('')
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const saving = ref(false)

onMounted(() => {
  name.value = auth.user?.name ?? ''
})

async function save() {
  if (newPassword.value && newPassword.value !== confirmPassword.value) {
    toast.error('as senhas não conferem')
    return
  }
  saving.value = true
  try {
    if (newPassword.value) {
      // changePassword guarda o token novo: trocar a senha invalida o anterior.
      await auth.changePassword(currentPassword.value, newPassword.value, name.value)
    } else {
      await api.put('/me', { name: name.value })
      await auth.fetchMe()
    }
    toast.push('Dados atualizados')
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <h1>Minha conta</h1>
    </div>

    <div class="card" style="max-width: 480px">
      <form @submit.prevent="save">
        <div class="field">
          <label>Nome</label>
          <input v-model="name" required />
        </div>
        <div class="field">
          <label>E-mail</label>
          <input :value="auth.user?.email" disabled />
        </div>

        <h2 class="section">Alterar senha</h2>
        <div class="field">
          <label>Senha atual</label>
          <input v-model="currentPassword" type="password" autocomplete="current-password" />
        </div>
        <div class="form-row">
          <div class="field">
            <label>Nova senha</label>
            <input v-model="newPassword" type="password" autocomplete="new-password" minlength="8" />
          </div>
          <div class="field">
            <label>Confirmar</label>
            <input v-model="confirmPassword" type="password" autocomplete="new-password" />
          </div>
        </div>

        <button class="btn btn-primary" type="submit" :disabled="saving" style="width: 100%; justify-content: center">
          {{ saving ? 'Salvando…' : 'Salvar' }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.section {
  font-size: 14px;
  margin: 18px 0 12px;
  padding-top: 14px;
  border-top: 1px solid var(--ci-border);
}
</style>
