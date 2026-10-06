<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import type { WorkspaceSettings } from '../types'

const auth = useAuthStore()
const toast = useToastStore()
const form = ref<WorkspaceSettings>({ name: '', segment: '', color: '#6d5df6', logo_url: '', website: '', timezone: 'America/Sao_Paulo' })
const saving = ref(false)

onMounted(async () => {
  try {
    form.value = await api.get<WorkspaceSettings>('/settings/workspace')
  } catch (e: any) {
    toast.error(e.message)
  }
})

async function save() {
  saving.value = true
  try {
    form.value = await api.put<WorkspaceSettings>('/settings/workspace', form.value)
    await auth.fetchWorkspace()
    toast.push('Identidade da empresa atualizada')
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
      <div>
        <h1>Empresa</h1>
        <p class="muted" style="margin: 4px 0 0">Nome, cor e logo que aparecem no CRM e nos e-mails do sistema.</p>
      </div>
    </div>
    <form class="card" style="max-width: 640px" @submit.prevent="save">
      <div class="field"><label>Nome da empresa</label><input v-model="form.name" required maxlength="80" /></div>
      <div class="field"><label>Segmento</label><input v-model="form.segment" /></div>
      <div class="field"><label>Site</label><input v-model="form.website" placeholder="https://" /></div>
      <div class="field"><label>Cor principal</label><div style="display: flex; gap: 8px"><input v-model="form.color" type="color" style="width: 52px; padding: 2px" /><input v-model="form.color" maxlength="7" /></div></div>
      <div class="field"><label>URL da logo</label><input v-model="form.logo_url" placeholder="https://…/logo.png" /></div>
      <div class="field"><label>Fuso horário</label><input v-model="form.timezone" /></div>
      <button class="btn btn-primary" type="submit" :disabled="saving">{{ saving ? 'Salvando…' : 'Salvar' }}</button>
    </form>
  </div>
</template>
