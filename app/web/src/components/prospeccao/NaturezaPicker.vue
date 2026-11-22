<script setup lang="ts">
import { computed, ref } from 'vue'
import type { CatalogoNatureza } from '../../types-prospeccao'
import BuscaChips from './BuscaChips.vue'

/** Seletor de natureza jurídica: a lista é curta e vem inteira do catálogo. */
const props = defineProps<{ modelValue: number[]; catalogo: CatalogoNatureza[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [codigos: number[]] }>()

const texto = ref('')

function semAcento(v: string): string {
  return v.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
}

const opcoes = computed(() => {
  const q = semAcento(texto.value)
  if (!q) return props.catalogo
  return props.catalogo.filter((n) => semAcento(n.descricao).includes(q) || String(n.codigo).includes(q))
})

const selecionadas = computed<CatalogoNatureza[]>(() =>
  props.modelValue.map((codigo) => props.catalogo.find((n) => n.codigo === codigo) ?? { codigo, descricao: `Natureza ${codigo}`, pessoa_fisica: false })
)

const chave = (n: CatalogoNatureza) => String(n.codigo)
const rotulo = (n: CatalogoNatureza) => n.descricao
</script>

<template>
  <BuscaChips
    :model-value="selecionadas"
    :opcoes="opcoes"
    :chave="chave"
    :rotulo="rotulo"
    :disabled="disabled"
    nome="Natureza jurídica"
    placeholder="Todas. Buscar: limitada, anônima, cooperativa…"
    vazio="Nenhuma natureza jurídica com esse nome."
    @update:model-value="emit('update:modelValue', $event.map((n) => n.codigo))"
    @buscar="texto = $event"
  >
    <template #opcao="{ item }">
      {{ item.descricao }}
      <span v-if="item.pessoa_fisica" class="badge amber pf">PF</span>
    </template>
  </BuscaChips>
</template>

<style scoped>
.pf {
  font-size: 11px;
  padding: 1px 7px;
  margin-left: 6px;
}
</style>
