<script setup lang="ts">
import { computed } from 'vue'
import { formatCompetencia, formatNumber } from '../../format'
import ProgressBar from '../ui/ProgressBar.vue'
import { estadoCargaLabels, type EstadoCarga, type ResumoPlataforma } from '../../types-prospeccao'

/**
 * Topo da prospecção: medidor de cota ("1.240 de 5.000 leads usados em
 * novembro"), referência da base ("Receita 2026-11 · PGFN 2026-T3"), faixa de
 * dados de demonstração e aviso de atualização em andamento.
 */
const props = defineProps<{ resumo: ResumoPlataforma }>()

const cota = computed(() => props.resumo.cota)
const usoPct = computed(() => (cota.value.limite > 0 ? Math.round((cota.value.usado / cota.value.limite) * 100) : 0))
const tom = computed(() => (cota.value.restante <= 0 ? 'red' : usoPct.value >= 85 ? 'amber' : ''))

const referencia = computed(() => {
  const b = props.resumo.base
  const partes: string[] = []
  if (b.referencia_cnpj) partes.push(`Receita ${b.referencia_cnpj}`)
  if (b.referencia_pgfn) partes.push(`PGFN ${b.referencia_pgfn}`)
  return partes.join(' · ')
})

/** Com a base pronta, a carga em curso é só um aviso discreto. */
const atualizando = computed(() => {
  const c = props.resumo.base.carga
  return props.resumo.base.pronta && !!c && c.estado !== 'ociosa' && c.estado !== 'erro' && !!c.estado
})
const rotuloCarga = computed(() => estadoCargaLabels[props.resumo.base.carga?.estado as EstadoCarga] ?? props.resumo.base.carga?.estado ?? '')
</script>

<template>
  <div class="plataforma">
    <div class="card cota" :class="tom">
      <ProgressBar :valor="cota.usado" :total="cota.limite" :mostrar-numeros="false" :tom="tom" fina>
        <span class="cota-texto">
          <strong>{{ formatNumber(cota.usado) }}</strong> de <strong>{{ formatNumber(cota.limite) }}</strong> leads usados em
          {{ formatCompetencia(cota.mes) }}
        </span>
      </ProgressBar>
      <div class="cota-rodape">
        <span v-if="cota.restante > 0">Restam {{ formatNumber(cota.restante) }} no mês. Contar e ver a prévia não gastam cota.</span>
        <span v-else class="esgotada">Cota do mês esgotada. Contar e ver a prévia continuam livres; a cota renova no mês que vem.</span>
      </div>
    </div>
    <div class="base">
      <span v-if="referencia" class="referencia" title="Referência da base pública usada nas buscas">{{ referencia }}</span>
      <span v-if="resumo.base.empresas" class="muted">· {{ formatNumber(resumo.base.empresas) }} empresas ativas</span>
      <span v-if="atualizando" class="badge blue atualizando" :title="resumo.base.carga.etapa">
        Atualização em andamento ({{ rotuloCarga.toLowerCase() }}, {{ Math.round(resumo.base.carga.percentual) }}%)
      </span>
    </div>
  </div>
  <div v-if="resumo.base.amostra" class="faixa amber amostra" role="note">
    <strong>Dados de demonstração.</strong> As empresas, telefones e e-mails desta base são fictícios; servem para conhecer a ferramenta.
  </div>
</template>

<style scoped>
.plataforma {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px 20px;
  margin-bottom: 14px;
}
.cota {
  flex: 1 1 360px;
  max-width: 520px;
  padding: 12px 16px;
  margin: 0;
}
.cota-texto {
  font-size: 13.5px;
}
.cota-rodape {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--ci-text-3);
}
.esgotada {
  color: var(--ci-red);
}
.base {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ci-text-2);
}
.referencia {
  font-weight: 500;
}
.atualizando {
  font-size: 11.5px;
}
.amostra {
  margin-bottom: 14px;
}
</style>
