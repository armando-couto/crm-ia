<script setup lang="ts">
import { computed } from 'vue'
import { api } from '../../api'
import { useConsultaAdiada } from '../../composables/useConsultaAdiada'
import { nivelCnaeLabels, type Catalogo, type CatalogoCnae } from '../../types-prospeccao'
import BuscaChips from './BuscaChips.vue'
import { formatarCnae } from './filtros'

/**
 * Seletor de CNAE: busca por palavra ("restaurante") ou por código ("5611"),
 * várias escolhas em chips e a opção de marcar um nível inteiro (divisão,
 * grupo ou classe), que vira prefixo no filtro.
 */
const props = defineProps<{ modelValue: CatalogoCnae[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [itens: CatalogoCnae[]] }>()

const busca = useConsultaAdiada(
  (q: string, signal) =>
    api.get<Catalogo<CatalogoCnae>>(`/prospeccao/catalogos/cnaes?q=${encodeURIComponent(q)}&limite=40`, { signal }),
  { atraso: 250 }
)

const opcoes = computed<CatalogoCnae[]>(() => (busca.dados.value?.itens ?? []).map((c) => ({ ...c, codigo: String(c.codigo) })))

function buscar(texto: string) {
  if (texto.length < 2) busca.limpar()
  else busca.pedir(texto)
}

/** Nível inteiro já escolhido que contém este código. */
function coberturaDe(item: CatalogoCnae): CatalogoCnae | undefined {
  return props.modelValue.find((s) => s.nivel !== 'subclasse' && s.codigo !== item.codigo && item.codigo.startsWith(s.codigo))
}

function bloqueada(item: CatalogoCnae): string {
  const pai = coberturaDe(item)
  return pai ? `Já incluído em ${formatarCnae(pai.codigo)} ${pai.descricao}`.trim() : ''
}

function atualizar(itens: CatalogoCnae[]) {
  // Escolher um nível inteiro dispensa os itens dele já marcados um a um.
  const novo = itens.find((i) => !props.modelValue.some((s) => s.codigo === i.codigo))
  if (novo && novo.nivel !== 'subclasse') {
    itens = itens.filter((i) => i.codigo === novo.codigo || !i.codigo.startsWith(novo.codigo))
  }
  emit('update:modelValue', itens)
}

const chave = (c: CatalogoCnae) => c.codigo
const rotulo = (c: CatalogoCnae) => (c.descricao ? `${formatarCnae(c.codigo)} ${c.descricao}` : `CNAE ${formatarCnae(c.codigo)}`)
const detalhe = (c: CatalogoCnae) =>
  c.nivel === 'subclasse' ? 'Subclasse' : `${nivelCnaeLabels[c.nivel]} inteira: todas as atividades que começam com ${c.codigo}`
</script>

<template>
  <BuscaChips
    :model-value="modelValue"
    :opcoes="opcoes"
    :chave="chave"
    :rotulo="rotulo"
    :detalhe="detalhe"
    :bloqueada="bloqueada"
    :carregando="busca.carregando.value"
    :disabled="disabled"
    nome="Buscar atividade (CNAE)"
    placeholder="Digite a atividade ou o código: restaurante, padaria, 5611…"
    vazio="Nenhuma atividade encontrada. Tente outra palavra ou só o começo do código."
    dica="Digite pelo menos 2 letras ou 2 números. Dois números (ex.: 56) trazem a divisão inteira."
    @update:model-value="atualizar"
    @buscar="buscar"
  >
    <template #opcao="{ item }">
      <span class="linha">
        <span class="codigo">{{ formatarCnae(item.codigo) }}</span>
        <span class="descricao">{{ item.descricao }}</span>
        <span v-if="item.nivel !== 'subclasse'" class="badge blue nivel">{{ nivelCnaeLabels[item.nivel] }} inteira</span>
      </span>
      <span v-if="bloqueada(item) && !modelValue.some((s) => s.codigo === item.codigo)" class="coberta">{{ bloqueada(item) }}</span>
    </template>
    <template #chip="{ item }">
      <strong class="chip-codigo">{{ formatarCnae(item.codigo) }}</strong>
      <span>{{ item.descricao }}</span>
      <span v-if="item.nivel !== 'subclasse'" class="chip-nivel">({{ nivelCnaeLabels[item.nivel].toLowerCase() }} inteira)</span>
    </template>
    <template v-if="busca.erro.value" #rodape>
      <span class="error-text">{{ busca.erro.value }}</span>
    </template>
  </BuscaChips>
</template>

<style scoped>
.linha {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.codigo {
  font-variant-numeric: tabular-nums;
  color: var(--ci-text-3);
  white-space: nowrap;
  min-width: 74px;
}
.descricao {
  flex: 1;
  min-width: 0;
}
.nivel {
  font-size: 11px;
  padding: 1px 8px;
}
.coberta {
  display: block;
  font-size: 12px;
  margin-left: 82px;
}
.chip-codigo {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.chip-nivel {
  opacity: 0.75;
}
</style>
