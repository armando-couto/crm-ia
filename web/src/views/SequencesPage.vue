<script setup lang="ts">
import { ref } from 'vue'
import SequencesView from './SequencesView.vue'
import SequenceEditorView from './SequenceEditorView.vue'

// Alterna entre a listagem e o editor sem trocar de rota, para o botão
// "‹ Sequências" voltar direto para a lista atualizada.
const editing = ref<number | null>(null)
</script>

<template>
  <div class="page">
    <SequenceEditorView
      v-if="editing !== null"
      :sequence-id="editing"
      :key="editing"
      @back="editing = null"
    />
    <SequencesView v-else @open="editing = $event" />
  </div>
</template>
