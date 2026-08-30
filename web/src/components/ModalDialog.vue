<script setup lang="ts">
defineProps<{
  title: string
  open: boolean
  wide?: boolean
}>()

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="overlay" @mousedown.self="emit('close')">
      <div class="modal" :class="{ wide }" role="dialog" aria-modal="true">
        <header>
          <h3>{{ title }}</h3>
          <button type="button" class="close" aria-label="Fechar" @click="emit('close')">×</button>
        </header>
        <div class="body">
          <slot />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(34, 20, 60, 0.45);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 8vh 16px 16px;
  z-index: 150;
  overflow-y: auto;
}

.modal {
  background: var(--fix-surface);
  border-radius: 14px;
  box-shadow: var(--shadow-lg);
  width: 100%;
  max-width: 480px;
  animation: modal-in 0.18s ease-out;
}

.modal.wide {
  max-width: 680px;
}

header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 22px 0;
}

h3 {
  font-size: 17px;
}

.close {
  border: none;
  background: none;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  color: var(--fix-text-3);
  padding: 4px 8px;
  border-radius: 6px;
}

.close:hover {
  background: var(--fix-bg);
  color: var(--fix-text);
}

.body {
  padding: 16px 22px 22px;
}

@keyframes modal-in {
  from {
    transform: translateY(-8px);
    opacity: 0;
  }
  to {
    transform: translateY(0);
    opacity: 1;
  }
}
</style>
