<script setup lang="ts">
import { formatDateTime, formatDuracao, resultadoLabels } from '../../format'
import { formatarTelefone } from '../../telefone'
import type { Tentativa } from '../../types-prospeccao'

/** Tentativas anteriores do lead, da mais recente para a mais antiga. */
defineProps<{ tentativas: Tentativa[] }>()

const conectados = ['atendeu', 'reuniao', 'retornar', 'sem_interesse', 'nao_e_decisor', 'nao_ligar_mais']
</script>

<template>
  <div class="historico">
    <p v-if="!tentativas.length" class="muted vazio">Primeira ligação para este lead.</p>
    <ol v-else>
      <li v-for="t in tentativas" :key="t.id">
        <div class="linha">
          <span class="badge" :class="conectados.includes(t.resultado) ? 'green' : 'amber'">
            {{ resultadoLabels[t.resultado] ?? t.resultado }}
          </span>
          <span class="muted quando">{{ formatDateTime(t.criado_em) }}</span>
        </div>
        <div class="muted detalhe">
          <span v-if="t.user_name">{{ t.user_name }}</span>
          <span v-if="t.numero"> · {{ formatarTelefone(t.numero) }}</span>
          <span v-if="t.duracao_segundos"> · {{ formatDuracao(t.duracao_segundos) }}</span>
        </div>
        <p v-if="t.notas" class="notas">{{ t.notas }}</p>
      </li>
    </ol>
  </div>
</template>

<style scoped>
ol {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

li {
  padding-bottom: 10px;
  border-bottom: 1px solid var(--ci-bg);
}

li:last-child {
  border-bottom: none;
}

.linha {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.quando,
.detalhe {
  font-size: 12px;
}

.notas {
  margin: 4px 0 0;
  font-size: 13px;
  white-space: pre-wrap;
}

.vazio {
  margin: 0;
  font-size: 13px;
}
</style>
