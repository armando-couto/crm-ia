<script setup lang="ts">
import { ref, watch } from 'vue'
import { api } from '../../api'
import type { Filtros, Ordem, Segmento, SegmentoRequest } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'

/**
 * Salva os filtros como segmento (o filtro, não o resultado). Com um segmento
 * já carregado, deixa escolher entre atualizá-lo e salvar como novo. Também
 * serve para renomear e compartilhar um segmento da aba "Segmentos salvos".
 */
const props = defineProps<{
  open: boolean
  filtros: Filtros
  ordem: Ordem
  /** Segmento em uso no formulário (ou em edição), se houver. */
  segmento?: Segmento | null
  nomeSugerido?: string
  /** Só renomear e compartilhar: não oferece "salvar como novo". */
  soEditar?: boolean
}>()

const emit = defineEmits<{ close: []; salvo: [segmento: Segmento] }>()

const nome = ref('')
const compartilhado = ref(false)
const modo = ref<'novo' | 'atualizar'>('novo')
const salvando = ref(false)
const erro = ref('')

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    modo.value = props.segmento ? 'atualizar' : 'novo'
    nome.value = props.segmento?.nome ?? props.nomeSugerido ?? ''
    compartilhado.value = props.segmento?.compartilhado ?? false
    erro.value = ''
  }
)

watch(modo, (m) => {
  if (m === 'novo' && props.segmento && nome.value === props.segmento.nome) nome.value = `${props.segmento.nome} (cópia)`
  if (m === 'atualizar' && props.segmento) nome.value = props.segmento.nome
})

async function salvar() {
  if (!nome.value.trim() || salvando.value) return
  salvando.value = true
  erro.value = ''
  const corpo: SegmentoRequest = { nome: nome.value.trim(), filtros: props.filtros, ordem: props.ordem, compartilhado: compartilhado.value }
  try {
    const salvo =
      modo.value === 'atualizar' && props.segmento
        ? await api.put<Segmento>(`/prospeccao/segmentos/${props.segmento.id}`, corpo)
        : await api.post<Segmento>('/prospeccao/segmentos', corpo)
    emit('salvo', salvo)
  } catch (e: any) {
    erro.value = e?.message || 'não foi possível salvar o segmento'
  } finally {
    salvando.value = false
  }
}
</script>

<template>
  <ModalDialog :title="soEditar ? 'Editar segmento' : 'Salvar segmento'" :open="open" @close="emit('close')">
    <form id="form-salvar-segmento" @submit.prevent="salvar">
      <p v-if="!soEditar" class="muted intro">
        O segmento guarda os filtros, não as empresas: a cada nova base você conta de novo e vê o que entrou.
      </p>

      <div v-if="segmento && !soEditar" class="modos" role="radiogroup" aria-label="Como salvar">
        <label class="check"><input v-model="modo" type="radio" value="atualizar" /> Atualizar "{{ segmento.nome }}"</label>
        <label class="check"><input v-model="modo" type="radio" value="novo" /> Salvar como novo segmento</label>
      </div>

      <div class="field">
        <label for="ss-nome">Nome do segmento</label>
        <input id="ss-nome" v-model="nome" type="text" maxlength="120" required placeholder="Ex.: Restaurantes do Ceará com celular" />
      </div>

      <label class="check">
        <input v-model="compartilhado" type="checkbox" />
        Compartilhar com a equipe
      </label>

      <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>
    </form>

    <template #footer>
      <button type="button" class="btn btn-outline" @click="emit('close')">Cancelar</button>
      <button type="submit" form="form-salvar-segmento" class="btn btn-primary" :disabled="salvando || !nome.trim()">
        {{ salvando ? 'Salvando…' : 'Salvar' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.intro {
  margin: 0 0 14px;
}
.modos {
  margin-bottom: 14px;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
  cursor: pointer;
}
</style>
