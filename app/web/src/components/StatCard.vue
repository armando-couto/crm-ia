<script setup lang="ts">
defineProps<{
  label: string
  value: string
  hint?: string
  tone?: 'purple' | 'green' | 'red' | 'blue'
}>()
</script>

<template>
  <div class="stat" :class="tone || 'purple'">
    <span class="label">{{ label }}</span>
    <span class="value">{{ value }}</span>
    <span v-if="hint" class="hint">{{ hint }}</span>
  </div>
</template>

<style scoped>
.stat {
  background: var(--ci-surface);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-top: 3px solid var(--ci-purple);
}

.stat.green {
  border-top-color: var(--ci-green);
}

.stat.red {
  border-top-color: var(--ci-red);
}

.stat.blue {
  border-top-color: var(--ci-blue);
}

.label {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--ci-text-3);
  font-weight: 600;
}

.value {
  font-size: 24px;
  font-weight: 700;
  color: var(--ci-text);
}

.hint {
  font-size: 12px;
  color: var(--ci-text-3);
}
</style>
