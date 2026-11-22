<script setup lang="ts">
import { computed, watch } from 'vue'
import { api } from '../../api'
import { useConsultaAdiada } from '../../composables/useConsultaAdiada'
import { formatNumber } from '../../format'
import type { Catalogo, CatalogoMunicipio } from '../../types-prospeccao'
import BuscaChips from './BuscaChips.vue'

/**
 * Seletor de cidades dependente da UF: com estados marcados, a lista abre já
 * com as maiores cidades deles; sem estado, busca no Brasil inteiro pelo nome.
 */
const props = defineProps<{ modelValue: CatalogoMunicipio[]; ufs: string[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [itens: CatalogoMunicipio[]] }>()

/** Acima disso a busca vai sem UF e o recorte é feito aqui. */
const MAX_UFS_POR_BUSCA = 6

interface Pedido {
  q: string
  ufs: string[]
}

async function consultar(p: Pedido, signal: AbortSignal): Promise<CatalogoMunicipio[]> {
  const q = p.q ? `&q=${encodeURIComponent(p.q)}` : ''
  if (!p.ufs.length || p.ufs.length > MAX_UFS_POR_BUSCA) {
    const r = await api.get<Catalogo<CatalogoMunicipio>>(`/prospeccao/catalogos/municipios?limite=50${q}`, { signal })
    const itens = r?.itens ?? []
    return p.ufs.length ? itens.filter((m) => p.ufs.includes(m.uf)) : itens
  }
  const partes = await Promise.all(
    p.ufs.map((uf) =>
      api.get<Catalogo<CatalogoMunicipio>>(`/prospeccao/catalogos/municipios?uf=${encodeURIComponent(uf)}&limite=50${q}`, { signal })
    )
  )
  return partes
    .flatMap((r) => r?.itens ?? [])
    .sort((a, b) => b.empresas - a.empresas || a.nome.localeCompare(b.nome))
    .slice(0, 50)
}

const busca = useConsultaAdiada(consultar, { atraso: 250 })
const opcoes = computed(() => busca.dados.value ?? [])

let ultimoTexto = ''
function buscar(texto: string) {
  ultimoTexto = texto
  if (!props.ufs.length && texto.length < 2) busca.limpar()
  else busca.pedir({ q: texto, ufs: [...props.ufs] })
}

// Trocar os estados muda o que a lista pode mostrar.
watch(
  () => props.ufs.join(','),
  () => {
    if (busca.dados.value) buscar(ultimoTexto)
  }
)

const chave = (m: CatalogoMunicipio) => String(m.codigo)
const rotulo = (m: CatalogoMunicipio) => (m.uf ? `${m.nome}/${m.uf}` : m.nome)

const dica = computed(() =>
  props.ufs.length ? 'Nenhuma cidade encontrada nos estados escolhidos.' : 'Digite o nome da cidade, ou escolha um estado acima para ver as maiores.'
)
</script>

<template>
  <BuscaChips
    :model-value="modelValue"
    :opcoes="opcoes"
    :chave="chave"
    :rotulo="rotulo"
    :carregando="busca.carregando.value"
    :disabled="disabled"
    nome="Buscar cidade"
    :placeholder="ufs.length ? `Buscar cidade em ${ufs.join(', ')}…` : 'Buscar cidade em todo o Brasil…'"
    vazio="Nenhuma cidade encontrada com esse nome."
    :dica="dica"
    @update:model-value="emit('update:modelValue', $event)"
    @buscar="buscar"
  >
    <template #opcao="{ item }">
      <span class="linha">
        <span class="nome">{{ item.nome }}<span class="uf">/{{ item.uf }}</span></span>
        <span v-if="item.empresas" class="qtd">{{ formatNumber(item.empresas) }} empresas</span>
      </span>
    </template>
    <template v-if="busca.erro.value" #rodape>
      <span class="error-text">{{ busca.erro.value }}</span>
    </template>
  </BuscaChips>
</template>

<style scoped>
.linha {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}
.uf {
  color: var(--ci-text-3);
}
.qtd {
  color: var(--ci-text-3);
  font-size: 12px;
  white-space: nowrap;
}
</style>
