<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()

const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const saving = ref(false)

async function submit() {
  error.value = ''
  if (newPassword.value.length < 8) {
    error.value = 'a nova senha deve ter pelo menos 8 caracteres'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'as senhas não conferem'
    return
  }
  saving.value = true
  try {
    await auth.changePassword(currentPassword.value, newPassword.value)
    router.push('/')
  } catch (e: any) {
    error.value = e.message || 'não foi possível alterar a senha'
  } finally {
    saving.value = false
  }
}

function cancel() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="login-page">
    <div class="panel">
      <div class="brand">
        <span class="brand-mark">F</span>
        <div>
          <h1>Defina sua senha</h1>
          <p class="muted">Sua senha temporária precisa ser trocada antes do primeiro acesso</p>
        </div>
      </div>

      <form @submit.prevent="submit">
        <div class="field">
          <label for="current">Senha temporária</label>
          <input id="current" v-model="currentPassword" type="password" autocomplete="current-password" required />
        </div>
        <div class="field">
          <label for="new">Nova senha</label>
          <input id="new" v-model="newPassword" type="password" autocomplete="new-password" minlength="8" required />
        </div>
        <div class="field">
          <label for="confirm">Confirmar nova senha</label>
          <input id="confirm" v-model="confirmPassword" type="password" autocomplete="new-password" required />
        </div>

        <p v-if="error" class="error-text">{{ error }}</p>

        <button class="btn btn-primary submit" type="submit" :disabled="saving">
          {{ saving ? 'Salvando…' : 'Salvar e entrar' }}
        </button>

        <button class="link-btn" type="button" @click="cancel">Sair</button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--fix-purple-deep) 0%, var(--fix-purple-dark) 100%);
  padding: 24px;
}

.panel {
  background: var(--fix-surface);
  border-radius: 16px;
  box-shadow: var(--shadow-lg);
  padding: 36px;
  width: 100%;
  max-width: 400px;
}

.brand {
  display: flex;
  gap: 14px;
  align-items: center;
  margin-bottom: 28px;
}

.brand-mark {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: var(--fix-purple);
  color: #fff;
  font-size: 24px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

h1 {
  font-size: 20px;
}

.brand p {
  margin: 2px 0 0;
  font-size: 13px;
}

.submit {
  width: 100%;
  justify-content: center;
  padding: 11px;
  font-size: 15px;
  margin-top: 6px;
}

.link-btn {
  display: block;
  width: 100%;
  margin-top: 16px;
  background: none;
  border: none;
  color: var(--fix-text-3);
  font-size: 13px;
  cursor: pointer;
}
</style>
