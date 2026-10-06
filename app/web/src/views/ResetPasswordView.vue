<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'

const route = useRoute()
const router = useRouter()

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const password = ref('')
const confirm = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  if (password.value.length < 8) {
    error.value = 'a senha deve ter pelo menos 8 caracteres'
    return
  }
  if (password.value !== confirm.value) {
    error.value = 'as senhas não conferem'
    return
  }
  loading.value = true
  try {
    await api.post('/auth/reset', { token: token.value, password: password.value })
    router.push({ name: 'login' })
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <div class="panel">
      <h1>Definir nova senha</h1>

      <p v-if="!token" class="error-text">Link inválido: acesse novamente pelo e-mail recebido.</p>

      <form v-else @submit.prevent="submit">
        <div class="field">
          <label for="password">Nova senha</label>
          <input id="password" v-model="password" type="password" autocomplete="new-password" required minlength="8" />
        </div>
        <div class="field">
          <label for="confirm">Confirmar senha</label>
          <input id="confirm" v-model="confirm" type="password" autocomplete="new-password" required />
        </div>
        <p v-if="error" class="error-text">{{ error }}</p>
        <button class="btn btn-primary submit" type="submit" :disabled="loading">
          {{ loading ? 'Salvando…' : 'Salvar nova senha' }}
        </button>
      </form>

      <router-link class="back" to="/login">← Voltar ao login</router-link>
    </div>
  </div>
</template>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--ci-purple-deep) 0%, var(--ci-purple-dark) 100%);
  padding: 24px;
}

.panel {
  background: var(--ci-surface);
  border-radius: 16px;
  box-shadow: var(--shadow-lg);
  padding: 36px;
  width: 100%;
  max-width: 400px;
}

h1 {
  font-size: 20px;
  margin-bottom: 16px;
}

.submit {
  width: 100%;
  justify-content: center;
  padding: 11px;
}

.back {
  display: block;
  text-align: center;
  margin-top: 18px;
  font-size: 13px;
}
</style>
