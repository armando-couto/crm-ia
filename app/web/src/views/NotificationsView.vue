<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useToastStore } from '../stores/toast'

const toast = useToastStore()

interface Prefs {
  negocio_atribuido: boolean
  tarefa_atribuida: boolean
  ticket_atribuido: boolean
}

const prefs = ref<Prefs>({ negocio_atribuido: true, tarefa_atribuida: true, ticket_atribuido: true })
const loading = ref(true)
const saving = ref(false)

const options: { key: keyof Prefs; label: string; hint: string }[] = [
  {
    key: 'negocio_atribuido',
    label: 'Negócio atribuído a você',
    hint: 'Receba um e-mail quando um colega criar um negócio com você como proprietário.'
  },
  {
    key: 'tarefa_atribuida',
    label: 'Tarefa atribuída a você',
    hint: 'Receba um e-mail quando uma tarefa for criada tendo você como responsável.'
  },
  {
    key: 'ticket_atribuido',
    label: 'Ticket atribuído a você',
    hint: 'Receba um e-mail quando um ticket for aberto com você como proprietário.'
  }
]

async function load() {
  loading.value = true
  try {
    prefs.value = await api.get<Prefs>('/me/notifications')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    prefs.value = await api.put<Prefs>('/me/notifications', prefs.value)
    toast.push('Preferências de notificação salvas')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>Notificações</h1>
        <p class="muted" style="margin: 4px 0 0">
          Escolha quais avisos você recebe por e-mail (enviados via Mandrill). Válido apenas para você.
        </p>
      </div>
    </div>

    <div class="card" style="max-width: 620px">
      <div v-for="o in options" :key="o.key" class="pref-row">
        <label class="switch">
          <input type="checkbox" v-model="prefs[o.key]" />
          <span class="track"><span class="thumb"></span></span>
        </label>
        <div class="pref-info">
          <strong>{{ o.label }}</strong>
          <span class="muted">{{ o.hint }}</span>
        </div>
      </div>

      <button class="btn btn-primary" type="button" :disabled="saving || loading" style="margin-top: 16px" @click="save">
        {{ saving ? 'Salvando…' : 'Salvar preferências' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.pref-row {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  padding: 14px 0;
  border-bottom: 1px solid var(--ci-bg);
}

.pref-row:last-of-type {
  border-bottom: none;
}

.pref-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 14px;
}

.pref-info .muted {
  font-size: 13px;
}

/* Toggle */
.switch {
  position: relative;
  display: inline-block;
  flex-shrink: 0;
  cursor: pointer;
  margin-top: 2px;
}

.switch input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.track {
  display: block;
  width: 40px;
  height: 22px;
  background: var(--ci-border);
  border-radius: 999px;
  transition: background 0.15s;
  position: relative;
}

.thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 18px;
  height: 18px;
  background: #fff;
  border-radius: 50%;
  transition: transform 0.15s;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.25);
}

.switch input:checked + .track {
  background: var(--ci-purple);
}

.switch input:checked + .track .thumb {
  transform: translateX(18px);
}
</style>
