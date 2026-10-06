<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const email = ref('')
const password = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    if (auth.mustChangePassword) {
      router.push('/trocar-senha')
      return
    }
    router.push(typeof route.query.r === 'string' ? route.query.r : '/')
  } catch (e: any) {
    error.value = e.message || 'não foi possível entrar'
  }
}
</script>

<template>
  <div class="login-page">
    <div class="panel">
      <div class="brand">
        <span class="brand-mark">F</span>
        <div>
          <h1>CRM IA</h1>
          <p class="muted">Seu CRM, do seu jeito</p>
        </div>
      </div>

      <form @submit.prevent="submit">
        <div class="field">
          <label for="email">E-mail</label>
          <input id="email" v-model="email" type="email" autocomplete="username" required placeholder="voce@exemplo.com.br" />
        </div>
        <div class="field">
          <label for="password">Senha</label>
          <input id="password" v-model="password" type="password" autocomplete="current-password" required placeholder="••••••••" />
        </div>

        <p v-if="error" class="error-text">{{ error }}</p>

        <button class="btn btn-primary submit" type="submit" :disabled="auth.loading">
          {{ auth.loading ? 'Entrando…' : 'Entrar' }}
        </button>

        <router-link class="forgot" to="/esqueci-senha">Esqueci minha senha</router-link>
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
  background: var(--ci-purple);
  color: #fff;
  font-size: 24px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
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

.forgot {
  display: block;
  text-align: center;
  margin-top: 16px;
  font-size: 13px;
}
</style>
