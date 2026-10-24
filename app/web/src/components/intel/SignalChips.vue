<script setup lang="ts">
// Resumo da inteligência: chips coloridos por nível (ok / atenção / risco).
import type { Sinal } from '../../types-intel'

defineProps<{ sinais: Sinal[] }>()

const icones: Record<string, string> = { ok: '✓', atencao: '!', risco: '✕' }
</script>

<template>
  <ul v-if="sinais.length" class="signals" aria-label="Sinais">
    <li v-for="(s, i) in sinais" :key="`${s.tipo}-${i}`" class="chip" :class="s.nivel" :data-tipo="s.tipo">
      <span class="ico" aria-hidden="true">{{ icones[s.nivel] ?? '•' }}</span>
      {{ s.texto }}
    </li>
  </ul>
</template>

<style scoped>
.signals {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 12px 5px 8px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  border: 1px solid transparent;
}

.ico {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  color: #fff;
}

.chip.ok {
  background: var(--ci-green-tint);
  color: var(--ci-green);
}

.chip.ok .ico {
  background: var(--ci-green);
}

.chip.atencao {
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
}

.chip.atencao .ico {
  background: var(--ci-amber);
}

.chip.risco {
  background: var(--ci-red-tint);
  color: var(--ci-red);
}

.chip.risco .ico {
  background: var(--ci-red);
}
</style>
