<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'

/**
 * Gaveta lateral: ficha do item da lista e painel contextual do lead. Abre
 * sobre a tela, à direita, sem tirar o usuário da lista.
 *
 * Não é modal (`aria-modal="false"`): os atalhos da tela por baixo (J e K no
 * painel de leads) continuam valendo. Esc fecha, a menos que haja um
 * ModalDialog aberto por cima.
 */
const props = withDefaults(
  defineProps<{
    open: boolean
    title?: string
    /** Largura em px (limitada à largura da janela). */
    largura?: number
    /** Escurece o fundo e fecha ao clicar fora. */
    fundo?: boolean
    /** Esc fecha a gaveta. */
    fecharComEsc?: boolean
  }>(),
  { title: '', largura: 520, fundo: true, fecharComEsc: true }
)

const emit = defineEmits<{ close: [] }>()

function onTecla(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !props.fecharComEsc) return
  // Um modal aberto por cima da gaveta trata o próprio Esc.
  if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
  emit('close')
}

watch(
  () => props.open,
  (aberta) => {
    if (aberta) window.addEventListener('keydown', onTecla)
    else window.removeEventListener('keydown', onTecla)
  },
  { immediate: true }
)

onBeforeUnmount(() => window.removeEventListener('keydown', onTecla))
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="drawer-holder">
      <div v-if="fundo" class="drawer-fundo" @mousedown.self="emit('close')"></div>
      <aside class="drawer" role="dialog" aria-modal="false" :aria-label="title || 'Painel'" :style="{ width: `${largura}px` }">
        <header class="drawer-head">
          <div class="drawer-title">
            <slot name="title">
              <h3>{{ title }}</h3>
            </slot>
          </div>
          <div class="drawer-actions">
            <slot name="actions" />
            <button type="button" class="drawer-close" aria-label="Fechar" title="Fechar (Esc)" @click="emit('close')">×</button>
          </div>
        </header>
        <div class="drawer-body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="drawer-footer">
          <slot name="footer" />
        </footer>
      </aside>
    </div>
  </Teleport>
</template>

<style scoped>
.drawer-holder {
  position: fixed;
  inset: 0;
  z-index: 140;
  pointer-events: none;
}

.drawer-fundo {
  position: absolute;
  inset: 0;
  background: rgba(34, 20, 60, 0.28);
  pointer-events: auto;
}

.drawer {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  max-width: 100vw;
  background: var(--ci-surface);
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
  pointer-events: auto;
  animation: drawer-in 0.18s ease-out;
}

.drawer-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 20px 14px;
  border-bottom: 1px solid var(--ci-border);
  flex-shrink: 0;
}

.drawer-title {
  min-width: 0;
}

.drawer-title h3 {
  font-size: 17px;
}

.drawer-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.drawer-close {
  border: none;
  background: none;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  color: var(--ci-text-3);
  padding: 4px 8px;
  border-radius: 6px;
}

.drawer-close:hover {
  background: var(--ci-bg);
  color: var(--ci-text);
}

.drawer-body {
  flex: 1;
  overflow-y: auto;
  padding: 18px 20px;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 20px;
  border-top: 1px solid var(--ci-border);
  background: var(--ci-bg);
  flex-shrink: 0;
}

@keyframes drawer-in {
  from {
    transform: translateX(24px);
    opacity: 0;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}
</style>
