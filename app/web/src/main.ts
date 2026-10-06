import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { setPasswordChangeHandler, setUnauthorizedHandler } from './api'
import { useAuthStore } from './stores/auth'
import './styles.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)

setUnauthorizedHandler(() => {
  router.push({ name: 'login' })
})

// A API responde 403 com must_change_password enquanto a senha temporária
// não for trocada: leva o usuário direto para a tela de troca.
setPasswordChangeHandler(() => {
  useAuthStore().mustChangePassword = true
  if (router.currentRoute.value.name !== 'change-password') {
    router.push({ name: 'change-password' })
  }
})

app.mount('#app')
