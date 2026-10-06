<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'

const email = ref('')
const sent = ref(false)
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await api.post('/auth/forgot', { email: email.value })
    sent.value = true
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
      <h1>Recuperar acesso</h1>

      <template v-if="!sent">
        <p class="muted">Informe seu e-mail e enviaremos um link para redefinir a senha.</p>
        <form @submit.prevent="submit">
          <div class="field">
            <label for="email">E-mail</label>
            <input id="email" v-model="email" type="email" required placeholder="voce@exemplo.com.br" />
          </div>
          <p v-if="error" class="error-text">{{ error }}</p>
          <button class="btn btn-primary submit" type="submit" :disabled="loading">
            {{ loading ? 'Enviando…' : 'Enviar link' }}
          </button>
        </form>
      </template>

      <p v-else class="success-text">
        Se o e-mail existir no CRM IA, você receberá o link de redefinição em instantes.
      </p>

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
  margin-bottom: 8px;
}

p.muted {
  margin: 0 0 20px;
  font-size: 14px;
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
