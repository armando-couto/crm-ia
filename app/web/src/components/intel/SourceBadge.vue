<script setup lang="ts">
// Marca ✦ discreta: "este campo veio de uma fonte externa", com fonte e data no tooltip.
// Aceita `fonte` (objeto de `fontes` da API) ou `provedor` + `data`; sem nenhum, não renderiza.
import { computed } from 'vue'
import type { FonteCampo } from '../../types-intel'
import { dataCurta, nomeProvedor } from './util'

const props = defineProps<{
  provedor?: string
  data?: string | null
  fonte?: FonteCampo | null
  /** Mostra o nome da fonte ao lado do ✦ (tabela de diff). */
  comNome?: boolean
}>()

const prov = computed(() => props.fonte?.provedor ?? props.provedor ?? '')
const quando = computed(() => props.fonte?.fetched_at ?? props.data ?? '')
const nome = computed(() => nomeProvedor(prov.value))
const titulo = computed(() => (quando.value ? `${nome.value} · ${dataCurta(quando.value)}` : nome.value))
</script>

<template>
  <span v-if="prov" class="source" :class="[prov, { named: comNome }]" :title="titulo" :aria-label="titulo">
    <span class="star" aria-hidden="true">✦</span>
    <span v-if="comNome" class="name">{{ nome }}</span>
  </span>
</template>

<style scoped>
.source {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--ci-purple);
  font-size: 11px;
  line-height: 1;
  vertical-align: middle;
  cursor: help;
}

.star {
  font-size: 11px;
}

.named {
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
  font-weight: 500;
  cursor: default;
}

.named.bigdatacorp {
  background: var(--ci-blue-tint);
  color: var(--ci-blue);
}

.named.serasa {
  background: var(--ci-amber-tint);
  color: var(--ci-amber);
}
</style>
