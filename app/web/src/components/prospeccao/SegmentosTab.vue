<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../../api'
import { formatDate, formatNumber } from '../../format'
import { useToastStore } from '../../stores/toast'
import { ordemLabels, type Segmento } from '../../types-prospeccao'
import SalvarSegmentoDialog from './SalvarSegmentoDialog.vue'
import { resumoFiltros } from './filtros'

/** Aba "Segmentos salvos": filtros guardados para contar de novo a cada base. */
defineProps<{ podeGerenciar: boolean }>()
const emit = defineEmits<{ usar: [segmento: Segmento] }>()

const toast = useToastStore()
const segmentos = ref<Segmento[]>([])
const carregando = ref(false)
const erro = ref('')
const editando = ref<Segmento | null>(null)

async function carregar() {
  carregando.value = true
  erro.value = ''
  try {
    segmentos.value = (await api.get<Segmento[]>('/prospeccao/segmentos')) ?? []
  } catch (e: any) {
    erro.value = e?.message || 'não foi possível carregar os segmentos'
  } finally {
    carregando.value = false
  }
}

async function excluir(s: Segmento) {
  if (!confirm(`Excluir o segmento "${s.nome}"? As listas geradas com ele continuam.`)) return
  try {
    await api.delete(`/prospeccao/segmentos/${s.id}`)
    toast.push('Segmento excluído')
    await carregar()
  } catch (e: any) {
    toast.error(e?.message || 'não foi possível excluir o segmento')
  }
}

function salvo(s: Segmento) {
  editando.value = null
  toast.push('Segmento atualizado')
  const i = segmentos.value.findIndex((x) => x.id === s.id)
  if (i >= 0) segmentos.value[i] = s
  else void carregar()
}

onMounted(carregar)
defineExpose({ carregar })
</script>

<template>
  <div>
    <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>
    <div v-else-if="!carregando && !segmentos.length" class="card empty-state">
      <strong>Nenhum segmento salvo</strong>
      Monte os filtros em "Nova lista" e use "Salvar segmento" para reaproveitar o recorte: a cada base nova você conta de novo e vê o que entrou.
    </div>
    <div v-else class="card table-wrap">
      <table class="data">
        <thead>
          <tr>
            <th>Segmento</th>
            <th>Filtros</th>
            <th class="num">Última contagem</th>
            <th>Criado por</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in segmentos" :key="s.id">
            <td>
              <strong>{{ s.nome }}</strong>
              <div class="muted pequeno">
                {{ ordemLabels[s.ordem] ?? s.ordem }}<span v-if="s.compartilhado"> · <span class="badge blue">equipe</span></span>
              </div>
            </td>
            <td class="pequeno resumo">{{ resumoFiltros(s.filtros) }}</td>
            <td class="num">
              <template v-if="s.ultima_contagem !== null">
                {{ formatNumber(s.ultima_contagem) }}
                <div class="muted pequeno">{{ formatDate(s.ultima_contagem_em) }}</div>
              </template>
              <span v-else class="muted">—</span>
            </td>
            <td class="pequeno">{{ s.criado_por_nome || '—' }}</td>
            <td class="acoes">
              <button type="button" class="btn btn-primary btn-sm" @click="emit('usar', s)">Usar</button>
              <template v-if="podeGerenciar">
                <button type="button" class="btn btn-outline btn-sm" @click="editando = s">Editar</button>
                <button type="button" class="btn btn-outline btn-sm" @click="excluir(s)">Excluir</button>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <SalvarSegmentoDialog
      :open="!!editando"
      :filtros="editando?.filtros ?? {}"
      :ordem="editando?.ordem ?? 'relevancia'"
      :segmento="editando"
      so-editar
      @close="editando = null"
      @salvo="salvo"
    />
  </div>
</template>

<style scoped>
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.pequeno {
  font-size: 12.5px;
}
.resumo {
  max-width: 380px;
  color: var(--ci-text-2);
}
.acoes {
  text-align: right;
  white-space: nowrap;
}
.acoes .btn + .btn {
  margin-left: 6px;
}
</style>
