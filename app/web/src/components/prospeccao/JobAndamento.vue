<script setup lang="ts">
import { computed } from 'vue'
import { formatCentavos, formatNumber } from '../../format'
import {
  jobAtivo,
  statusJobLabels,
  tipoJobLabels,
  type Job,
  type ResultadoEnviarCrm,
  type ResultadoGerarLista
} from '../../types-prospeccao'
import ProgressBar from '../ui/ProgressBar.vue'

/**
 * Faixa do trabalho da lista: barra de progresso enquanto roda (com
 * "Cancelar"), mensagem quando falha ou é cancelado e o resumo quando termina.
 */
const props = defineProps<{ job: Job; podeCancelar?: boolean; cancelando?: boolean; erroConsulta?: string }>()
const emit = defineEmits<{ cancelar: []; fechar: [] }>()

const ativo = computed(() => jobAtivo(props.job))
const tom = computed(() => {
  if (props.job.status === 'falhou') return 'red'
  if (props.job.status === 'cancelado') return 'amber'
  if (props.job.status === 'concluido') return 'green'
  return ''
})

const titulos: Record<Job['tipo'], string> = {
  gerar_lista: 'Geração da lista',
  enriquecer: 'Enriquecimento',
  enviar_crm: 'Envio ao CRM'
}

const resumo = computed<string[]>(() => {
  const r = props.job.resultado as Record<string, any> | null
  if (!r || ativo.value) return []
  const n = (v: unknown) => formatNumber(Number(v) || 0)
  if (props.job.tipo === 'gerar_lista') {
    const g = r as Partial<ResultadoGerarLista>
    const partes = [`${n(g.entregues)} empresas na lista`]
    if (g.pulados_no_crm) partes.push(`${n(g.pulados_no_crm)} já estavam no CRM`)
    if (g.pulados_em_listas) partes.push(`${n(g.pulados_em_listas)} já estavam em outras listas`)
    if (g.pulados_bloqueio) partes.push(`${n(g.pulados_bloqueio)} bloqueadas`)
    if (g.pulados_pf) partes.push(`${n(g.pulados_pf)} de pessoa física puladas`)
    if (g.nao_encontrados) partes.push(`${n(g.nao_encontrados)} não encontradas na base`)
    return partes
  }
  if (props.job.tipo === 'enviar_crm') {
    const e = r as Partial<ResultadoEnviarCrm>
    const partes = [`${n(e.leads)} leads criados`]
    if (e.empresas_criadas) partes.push(`${n(e.empresas_criadas)} empresas novas`)
    if (e.empresas_existentes) partes.push(`${n(e.empresas_existentes)} ligadas a empresas existentes`)
    if (e.contatos) partes.push(`${n(e.contatos)} contatos`)
    if (e.duplicados) partes.push(`${n(e.duplicados)} duplicadas puladas`)
    if (e.bloqueados) partes.push(`${n(e.bloqueados)} bloqueadas`)
    for (const u of e.por_usuario ?? []) partes.push(`${u.nome}: ${n(u.quantidade)}`)
    return partes
  }
  const partes = [`${n(props.job.feitos)} consultadas`]
  if (props.job.erros) partes.push(`${n(props.job.erros)} com erro`)
  if (props.job.custo_centavos) partes.push(`custo ${formatCentavos(props.job.custo_centavos)}`)
  return partes
})
</script>

<template>
  <div class="card job" :class="tom" role="status" aria-live="polite">
    <template v-if="ativo">
      <div class="linha">
        <ProgressBar
          :valor="job.feitos"
          :total="job.total"
          :indeterminado="job.status === 'pendente' || !job.total"
          :mostrar-numeros="job.total > 0"
        >
          <strong>{{ tipoJobLabels[job.tipo] }}</strong>
          <span class="muted"> · {{ statusJobLabels[job.status] }}</span>
          <span v-if="job.erros" class="erros"> · {{ formatNumber(job.erros) }} com erro</span>
          <span v-if="job.custo_centavos" class="muted"> · {{ formatCentavos(job.custo_centavos) }}</span>
        </ProgressBar>
        <button v-if="podeCancelar" type="button" class="btn btn-outline btn-sm" :disabled="cancelando" @click="emit('cancelar')">
          {{ cancelando ? 'Cancelando…' : 'Cancelar' }}
        </button>
      </div>
      <p class="muted nota">Roda no servidor: pode fechar ou recarregar a página que o trabalho continua.</p>
      <p v-if="erroConsulta" class="error-text nota">{{ erroConsulta }}</p>
    </template>
    <template v-else>
      <div class="linha">
        <div>
          <strong>{{ titulos[job.tipo] }}: {{ statusJobLabels[job.status].toLowerCase() }}</strong>
          <p v-if="job.mensagem" class="mensagem">{{ job.mensagem }}</p>
          <p v-if="resumo.length" class="mensagem muted">{{ resumo.join(' · ') }}</p>
        </div>
        <button type="button" class="fechar" aria-label="Fechar aviso" @click="emit('fechar')">×</button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.job {
  padding: 12px 16px;
  margin: 0 0 14px;
  border-left: 4px solid var(--ci-purple);
}
.job.green {
  border-left-color: var(--ci-green);
}
.job.red {
  border-left-color: var(--ci-red);
}
.job.amber {
  border-left-color: var(--ci-amber, #d99100);
}
.linha {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}
.linha > :first-child {
  flex: 1;
  min-width: 0;
}
.nota {
  margin: 6px 0 0;
  font-size: 12.5px;
}
.erros {
  color: var(--ci-red);
}
.mensagem {
  margin: 4px 0 0;
  font-size: 13px;
}
.fechar {
  border: none;
  background: none;
  font-size: 20px;
  line-height: 1;
  color: var(--ci-text-3);
  cursor: pointer;
}
</style>
