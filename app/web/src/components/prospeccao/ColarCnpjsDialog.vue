<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, ApiError } from '../../api'
import { formatCompetencia, formatNumber, separarCNPJs } from '../../format'
import type { Cota, CriarLista, CriarListaResposta } from '../../types-prospeccao'
import ModalDialog from '../ModalDialog.vue'
import { MAX_CNPJS } from './filtros'

/**
 * "Colar CNPJs": lista específica validada no navegador (aceita o CNPJ
 * alfanumérico). Mostra "N válidos, M inválidos, K repetidos", recusa mais de
 * 5.000 e envia só os válidos.
 */
const props = defineProps<{ open: boolean; cota: Cota }>()
const emit = defineEmits<{ close: []; criada: [resposta: CriarListaResposta]; cota: [cota: Cota] }>()

const texto = ref('')
const nome = ref('')
const incluirPf = ref(false)
const excluirNoCrm = ref(true)
const excluirEmListas = ref(true)
const salvando = ref(false)
const erro = ref('')

watch(
  () => props.open,
  (aberto) => {
    if (!aberto) return
    texto.value = ''
    nome.value = ''
    incluirPf.value = false
    excluirNoCrm.value = true
    excluirEmListas.value = true
    erro.value = ''
  }
)

const colados = computed(() => separarCNPJs(texto.value))
const validos = computed(() => colados.value.validos.length)
const demais = computed(() => validos.value > MAX_CNPJS)
const semCota = computed(() => props.cota.restante <= 0)
const acimaDaCota = computed(() => !semCota.value && validos.value > props.cota.restante)
const podeGerar = computed(() => !salvando.value && validos.value > 0 && !demais.value && !semCota.value && nome.value.trim().length > 0)

async function aoEscolherArquivo(event: Event) {
  const input = event.target as HTMLInputElement
  const arquivo = input.files?.[0]
  input.value = ''
  if (!arquivo) return
  try {
    const conteudo = await arquivo.text()
    texto.value = texto.value ? `${texto.value}\n${conteudo}` : conteudo
    if (!nome.value) nome.value = arquivo.name.replace(/\.[^.]+$/, '')
  } catch {
    erro.value = 'não foi possível ler o arquivo'
  }
}

async function gerar() {
  if (!podeGerar.value) return
  salvando.value = true
  erro.value = ''
  const corpo: CriarLista = {
    nome: nome.value.trim(),
    origem: 'cnpjs',
    filtros: { cnpjs: colados.value.validos, ...(incluirPf.value ? { incluir_pf: true } : {}) },
    quantidade: validos.value,
    excluir_no_crm: excluirNoCrm.value,
    excluir_em_listas: excluirEmListas.value
  }
  try {
    emit('criada', await api.post<CriarListaResposta>('/prospeccao/listas', corpo))
  } catch (e: any) {
    if (e instanceof ApiError && e.data?.cota) emit('cota', e.data.cota)
    erro.value = e?.message || 'não foi possível gerar a lista'
  } finally {
    salvando.value = false
  }
}
</script>

<template>
  <ModalDialog title="Colar CNPJs" :open="open" wide @close="emit('close')">
    <form id="form-colar-cnpjs" @submit.prevent="gerar">
      <p class="muted intro">
        Cole os CNPJs de uma planilha ou de um texto (um por linha, ou separados por vírgula ou ponto e vírgula), com ou sem pontuação.
      </p>

      <div class="field">
        <label for="cc-texto">CNPJs</label>
        <textarea id="cc-texto" v-model="texto" rows="8" spellcheck="false" placeholder="11.222.333/0001-81&#10;12.ABC.345/01DE-35"></textarea>
        <label class="arquivo">
          ou escolha um arquivo .csv ou .txt
          <input type="file" accept=".csv,.txt,text/csv,text/plain" @change="aoEscolherArquivo" />
        </label>
      </div>

      <div v-if="texto.trim()" class="conferencia" :class="{ ruim: demais || !validos }" aria-live="polite">
        <strong>{{ formatNumber(validos) }} válidos</strong>, {{ formatNumber(colados.invalidos.length) }} inválidos,
        {{ formatNumber(colados.repetidos) }} repetidos
        <details v-if="colados.invalidos.length" class="invalidos">
          <summary>Ver os inválidos</summary>
          <code>{{ colados.invalidos.slice(0, 50).join(', ') }}</code>
          <span v-if="colados.invalidos.length > 50"> e mais {{ formatNumber(colados.invalidos.length - 50) }}</span>
        </details>
      </div>
      <p v-if="demais" class="error-text" role="alert">
        São {{ formatNumber(validos) }} CNPJs válidos; o limite é de {{ formatNumber(MAX_CNPJS) }} por lista. Divida em mais de uma lista.
      </p>

      <div class="field">
        <label for="cc-nome">Nome da lista</label>
        <input id="cc-nome" v-model="nome" type="text" maxlength="120" required placeholder="Ex.: Expositores da feira de novembro" />
      </div>

      <label class="check"><input v-model="excluirNoCrm" type="checkbox" /> Pular empresas que já estão no CRM</label>
      <label class="check"><input v-model="excluirEmListas" type="checkbox" /> Pular empresas de outras listas</label>
      <label class="check">
        <input v-model="incluirPf" type="checkbox" />
        Incluir MEI e empresário individual
        <span class="muted">(são pessoas físicas: a LGPD se aplica ao contato)</span>
      </label>

      <div v-if="semCota" class="faixa red espaco">
        Sua cota de {{ formatCompetencia(cota.mes) }} acabou ({{ formatNumber(cota.usado) }} de {{ formatNumber(cota.limite) }} leads usados).
      </div>
      <div v-else-if="acimaDaCota" class="faixa amber espaco">
        Restam {{ formatNumber(cota.restante) }} leads na cota do mês: a lista sai com no máximo {{ formatNumber(cota.restante) }} empresas.
      </div>
      <p v-else-if="validos && !demais" class="uso">
        Esta lista usa até <strong>{{ formatNumber(validos) }}</strong> dos seus <strong>{{ formatNumber(cota.restante) }}</strong> leads restantes no mês.
      </p>

      <p v-if="erro" class="error-text" role="alert">{{ erro }}</p>
    </form>

    <template #footer>
      <button type="button" class="btn btn-outline" @click="emit('close')">Cancelar</button>
      <button type="submit" form="form-colar-cnpjs" class="btn btn-primary" :disabled="!podeGerar">
        {{ salvando ? 'Gerando…' : validos && !demais ? `Gerar lista com ${formatNumber(validos)} CNPJs` : 'Gerar lista' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.intro {
  margin: 0 0 14px;
}
textarea {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace !important;
  font-size: 13px !important;
  resize: vertical;
}
.arquivo {
  font-size: 12.5px !important;
  font-weight: 400 !important;
  color: var(--ci-text-3) !important;
}
.arquivo input {
  padding: 0 !important;
  border: none !important;
  font-size: 12.5px !important;
  margin-left: 6px;
}
.conferencia {
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--ci-green-tint);
  color: var(--ci-green);
  margin-bottom: 14px;
}
.conferencia.ruim {
  background: var(--ci-red-tint);
  color: var(--ci-red);
}
.invalidos {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--ci-text-2);
}
.invalidos code {
  word-break: break-all;
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
  cursor: pointer;
  flex-wrap: wrap;
}
.espaco {
  margin-top: 12px;
}
.uso {
  margin: 12px 0 0;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--ci-purple-tint);
  color: var(--ci-purple-dark);
}
</style>
