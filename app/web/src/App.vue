<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { useToastStore } from './stores/toast'
import AppShell from './components/AppShell.vue'

const route = useRoute()
const auth = useAuthStore()
const toasts = useToastStore()

onMounted(() => {
  auth.fetchMe()
})
</script>

<template>
  <component :is="route.meta.public ? 'div' : AppShell">
    <router-view />
  </component>

  <div class="toast-holder">
    <div v-for="t in toasts.toasts" :key="t.id" class="toast" :class="{ error: t.kind === 'error' }" @click="toasts.dismiss(t.id)">
      {{ t.message }}
    </div>
  </div>
</template>
