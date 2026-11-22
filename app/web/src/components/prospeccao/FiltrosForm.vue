<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../../api'
import { formatNumber } from '../../format'
import {
  dividaFiltroLabels,
  matrizLabels,
  ORDENS,
  ordemLabels,
  porteLabels,
  simNaoLabels,
  type Catalogo,
  type CatalogoNatureza,
  type CatalogoUf,
  type Porte
} from '../../types-prospeccao'
import ChipsTexto from './ChipsTexto.vue'
import CidadePicker from './CidadePicker.vue'
import CnaePicker from './CnaePicker.vue'
import NaturezaPicker from './NaturezaPicker.vue'
import { ATALHOS_ABERTURA, localFinoPermitido, ufsSemCidade, usadosNaQualidade, usadosNoPerfil, type FormFiltros } from './filtros'

/**
 * Formulário do gerador de listas em quatro blocos numerados, do macro ao
 * micro: (1) que tipo de empresa, (2) onde, (3) perfil e (4) qualidade do
 * contato e ordem. Os dois últimos começam recolhidos e mostram quantos
 * filtros estão em uso.
 */
const form = defineModel<FormFiltros>({ required: true })
defineProps<{ disabled?: boolean }>()

/** Siglas na ordem em que o vendedor procura (regiões), para quando o catálogo não responde. */
const UFS_PADRAO = [
  'AC', 'AL', 'AM', 'AP', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MG', 'MS', 'MT', 'PA',
  'PB', 'PE', 'PI', 'PR', 'RJ', 'RN', 'RO', 'RR', 'RS', 'SC', 'SE', 'SP', 'TO'
]
const PORTES: Porte[] = ['me', 'epp', 'demais', 'nao_informado']

const ufsCatalogo = ref<CatalogoUf[]>([])
const naturezas = ref<CatalogoNatureza[]>([])
let naturezasPedidas = false

const abertoLocalFino = ref(false)
const abertoPerfil = ref(false)
const abertoQualidade = ref(false)

const ufs = computed<CatalogoUf[]>(() => {
  if (ufsCatalogo.value.length) return [...ufsCatalogo.value].sort((a, b) => a.uf.localeCompare(b.uf))
  return UFS_PADRAO.map((uf) => ({ uf, nome: uf, empresas: 0 }))
})

onMounted(async () => {
  try {
    ufsCatalogo.value = (await api.get<Catalogo<CatalogoUf>>('/prospeccao/catalogos/ufs'))?.itens ?? []
  } catch {
    /* segue com as siglas fixas */
  }
  // Segmento carregado com filtros recolhidos: abre o bloco para mostrar o que está em uso.
  if (usadosNoPerfil(form.value) > 0) abrirPerfil()
  if (form.value.bairros.length || form.value.ceps.length) abertoLocalFino.value = true
})

async function carregarNaturezas() {
  if (naturezasPedidas) return
  naturezasPedidas = true
  try {
    naturezas.value = (await api.get<Catalogo<CatalogoNatureza>>('/prospeccao/catalogos/naturezas'))?.itens ?? []
  } catch {
    naturezasPedidas = false
  }
}

function abrirPerfil() {
  abertoPerfil.value = true
  void carregarNaturezas()
}

function alternarPerfil() {
  if (abertoPerfil.value) abertoPerfil.value = false
  else abrirPerfil()
}

watch(
  () => form.value.naturezas.length,
  (n) => {
    if (n) void carregarNaturezas()
  },
  { immediate: true }
)

function alternarUf(uf: string) {
  const f = form.value
  if (f.ufs.includes(uf)) {
    f.ufs = f.ufs.filter((u) => u !== uf)
    // Cidades do estado desmarcado saem junto.
    f.municipios = f.municipios.filter((m) => m.uf !== uf)
  } else {
    f.ufs = [...f.ufs, uf]
  }
}

function alternarPorte(p: Porte) {
  const f = form.value
  f.portes = f.portes.includes(p) ? f.portes.filter((x) => x !== p) : [...f.portes, p]
}

function escolherAbertura(dias: number | null) {
  const f = form.value
  if (dias === null) {
    f.abertura_modo = ''
    f.abertura_ultimos_dias = null
  } else {
    f.abertura_modo = 'dias'
    f.abertura_ultimos_dias = dias
  }
}

// MEI = sim só existe com pessoa física incluída (o serviço responde 422 sem isso).
watch(
  () => form.value.mei,
  (mei) => {
    if (mei === 'sim') form.value.incluir_pf = true
  }
)
watch(
  () => form.value.incluir_pf,
  (pf) => {
    if (!pf && form.value.mei === 'sim') form.value.mei = 'todos'
  }
)

// Cidades novas trazem a UF para os chips de estado.
watch(
  () => form.value.municipios.map((m) => m.uf).join(','),
  () => {
    const f = form.value
    const faltam = [...new Set(f.municipios.map((m) => m.uf).filter((uf) => uf && !f.ufs.includes(uf)))]
    if (faltam.length) f.ufs = [...f.ufs, ...faltam]
  }
)

const ufsIgnoradas = computed(() => ufsSemCidade(form.value))
const podeLocalFino = computed(() => localFinoPermitido(form.value))
const nPerfil = computed(() => usadosNoPerfil(form.value))
const nQualidade = computed(() => usadosNaQualidade(form.value))
const nomeCurto = computed(() => {
  const n = form.value.nome.trim().length
  return n > 0 && n < 3
})

function normalizarBairro(v: string): string | null {
  const s = v
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toUpperCase()
    .replace(/\s+/g, ' ')
    .trim()
  return s.length >= 2 ? s : null
}

function normalizarCep(v: string): string | null {
  const d = v.replace(/\D/g, '')
  return d.length >= 3 && d.length <= 8 ? d : null
}

function numeroOuNulo(v: string): number | null {
  const n = Number(String(v).replace(/\./g, '').replace(',', '.'))
  return v === '' || !Number.isFinite(n) ? null : n
}
</script>

<template>
  <fieldset class="filtros" :disabled="disabled">
    <!-- 1. Que tipo de empresa? -->
    <section class="bloco card">
      <header class="bloco-head">
        <span class="numero">1</span>
        <div>
          <h2>Que tipo de empresa?</h2>
          <p class="muted">Atividade econômica (CNAE). Escolha uma ou várias; um código curto (ex.: 56) traz a divisão inteira.</p>
        </div>
      </header>
      <CnaePicker v-model="form.cnaes" :disabled="disabled" />
      <label class="chave">
        <input v-model="form.cnae_secundario" type="checkbox" :disabled="disabled || !form.cnaes.length" />
        Incluir CNAE secundário
        <span class="muted">(também quem tem a atividade como secundária: alcança mais empresas, com menos precisão)</span>
      </label>
    </section>

    <!-- 2. Onde? -->
    <section class="bloco card">
      <header class="bloco-head">
        <span class="numero">2</span>
        <div>
          <h2>Onde?</h2>
          <p class="muted">Estados e, se quiser, cidades. Sem estado nem cidade a busca vale para o Brasil todo (exige CNAE).</p>
        </div>
      </header>
      <div class="ufs" role="group" aria-label="Estados">
        <button
          v-for="u in ufs"
          :key="u.uf"
          type="button"
          class="chip uf"
          :class="{ active: form.ufs.includes(u.uf) }"
          :aria-pressed="form.ufs.includes(u.uf)"
          :title="u.empresas ? `${u.nome}: ${formatNumber(u.empresas)} empresas ativas` : u.nome"
          :disabled="disabled"
          @click="alternarUf(u.uf)"
        >
          {{ u.uf }}
        </button>
      </div>
      <div class="field cidades">
        <label>Cidades</label>
        <CidadePicker v-model="form.municipios" :ufs="form.ufs" :disabled="disabled" />
        <span v-if="ufsIgnoradas.length" class="muted aviso-uf">
          Com cidades escolhidas, a busca vale só para elas: {{ ufsIgnoradas.join(', ') }} sem cidade fica{{ ufsIgnoradas.length > 1 ? 'm' : '' }} de fora.
        </span>
      </div>

      <button type="button" class="recolher" :aria-expanded="abertoLocalFino" @click="abertoLocalFino = !abertoLocalFino">
        <span class="seta" :class="{ aberta: abertoLocalFino }">›</span> Bairros e CEP
        <span v-if="form.bairros.length + form.ceps.length" class="badge blue">{{ form.bairros.length + form.ceps.length }}</span>
      </button>
      <div v-if="abertoLocalFino" class="local-fino">
        <p v-if="!podeLocalFino" class="muted">Escolha uma cidade ou um CNAE para filtrar por bairro e CEP.</p>
        <div class="form-row">
          <div class="field">
            <label>Bairros</label>
            <ChipsTexto
              v-model="form.bairros"
              nome="Bairros"
              placeholder="Aldeota, Meireles…"
              :disabled="disabled || !podeLocalFino"
              :normalizar="normalizarBairro"
              invalido="Bairro muito curto"
            />
          </div>
          <div class="field">
            <label>CEP (inteiro ou começo, mínimo 3 dígitos)</label>
            <ChipsTexto
              v-model="form.ceps"
              nome="CEP"
              placeholder="60160, 60150-160…"
              :disabled="disabled || !podeLocalFino"
              :normalizar="normalizarCep"
              invalido="CEP inválido"
            />
          </div>
        </div>
      </div>
    </section>

    <!-- 3. Perfil da empresa -->
    <section class="bloco card" :class="{ recolhido: !abertoPerfil }">
      <button type="button" class="bloco-head clicavel" :aria-expanded="abertoPerfil" @click="alternarPerfil">
        <span class="numero">3</span>
        <div>
          <h2>
            Perfil da empresa <span class="opcional">opcional</span>
            <span v-if="nPerfil" class="badge blue">{{ nPerfil }} em uso</span>
          </h2>
          <p class="muted">Porte, matriz ou filial, natureza jurídica, abertura, capital, Simples, MEI e dívida ativa.</p>
        </div>
        <span class="seta grande" :class="{ aberta: abertoPerfil }" aria-hidden="true">›</span>
      </button>

      <div v-if="abertoPerfil" class="bloco-corpo">
        <div class="field">
          <label>Porte</label>
          <div class="opcoes">
            <button
              v-for="p in PORTES"
              :key="p"
              type="button"
              class="chip"
              :class="{ active: form.portes.includes(p) }"
              :aria-pressed="form.portes.includes(p)"
              @click="alternarPorte(p)"
            >
              {{ porteLabels[p] }}
            </button>
          </div>
        </div>

        <div class="form-row">
          <div class="field">
            <label for="ff-matriz">Matriz ou filial</label>
            <select id="ff-matriz" v-model="form.matriz">
              <option v-for="(r, v) in matrizLabels" :key="v" :value="v">{{ r }}</option>
            </select>
          </div>
          <div class="field">
            <label for="ff-simples">Simples Nacional</label>
            <select id="ff-simples" v-model="form.simples">
              <option v-for="(r, v) in simNaoLabels" :key="v" :value="v">{{ r }}</option>
            </select>
          </div>
          <div class="field">
            <label for="ff-mei">MEI</label>
            <select id="ff-mei" v-model="form.mei">
              <option v-for="(r, v) in simNaoLabels" :key="v" :value="v">{{ r }}</option>
            </select>
          </div>
        </div>

        <div class="field">
          <label>Natureza jurídica</label>
          <NaturezaPicker v-model="form.naturezas" :catalogo="naturezas" :disabled="disabled" />
        </div>

        <div class="field">
          <label>Abertura</label>
          <div class="opcoes">
            <button type="button" class="chip" :class="{ active: form.abertura_modo === '' }" @click="escolherAbertura(null)">Qualquer data</button>
            <button
              v-for="a in ATALHOS_ABERTURA"
              :key="a.dias"
              type="button"
              class="chip"
              :class="{ active: form.abertura_modo === 'dias' && form.abertura_ultimos_dias === a.dias }"
              @click="escolherAbertura(a.dias)"
            >
              Últimos {{ a.rotulo }}
            </button>
            <button type="button" class="chip" :class="{ active: form.abertura_modo === 'intervalo' }" @click="form.abertura_modo = 'intervalo'">
              Intervalo
            </button>
          </div>
          <div v-if="form.abertura_modo === 'intervalo'" class="form-row intervalo">
            <div class="field">
              <label for="ff-ab-de">De</label>
              <input id="ff-ab-de" v-model="form.abertura_de" type="date" />
            </div>
            <div class="field">
              <label for="ff-ab-ate">Até</label>
              <input id="ff-ab-ate" v-model="form.abertura_ate" type="date" />
            </div>
          </div>
        </div>

        <div class="form-row">
          <div class="field">
            <label for="ff-cap-min">Capital social mínimo (R$)</label>
            <input
              id="ff-cap-min"
              type="number"
              min="0"
              step="1000"
              :value="form.capital_min ?? ''"
              placeholder="Sem mínimo"
              @input="form.capital_min = numeroOuNulo(($event.target as HTMLInputElement).value)"
            />
          </div>
          <div class="field">
            <label for="ff-cap-max">Capital social máximo (R$)</label>
            <input
              id="ff-cap-max"
              type="number"
              min="0"
              step="1000"
              :value="form.capital_max ?? ''"
              placeholder="Sem máximo"
              @input="form.capital_max = numeroOuNulo(($event.target as HTMLInputElement).value)"
            />
          </div>
        </div>

        <div class="form-row">
          <div class="field">
            <label for="ff-divida">Dívida ativa (PGFN)</label>
            <select id="ff-divida" v-model="form.divida">
              <option v-for="(r, v) in dividaFiltroLabels" :key="v" :value="v">{{ r }}</option>
            </select>
          </div>
          <template v-if="form.divida !== 'sem'">
            <div class="field">
              <label for="ff-div-min">Dívida mínima (R$)</label>
              <input
                id="ff-div-min"
                type="number"
                min="0"
                step="1000"
                :value="form.divida_min ?? ''"
                placeholder="Sem mínimo"
                @input="form.divida_min = numeroOuNulo(($event.target as HTMLInputElement).value)"
              />
            </div>
            <div class="field">
              <label for="ff-div-max">Dívida máxima (R$)</label>
              <input
                id="ff-div-max"
                type="number"
                min="0"
                step="1000"
                :value="form.divida_max ?? ''"
                placeholder="Sem máximo"
                @input="form.divida_max = numeroOuNulo(($event.target as HTMLInputElement).value)"
              />
            </div>
          </template>
        </div>

        <div class="field">
          <label for="ff-nome">Nome contém</label>
          <input id="ff-nome" v-model="form.nome" type="text" maxlength="80" placeholder="Na razão social ou no nome fantasia" />
          <span v-if="nomeCurto" class="muted">Digite pelo menos 3 letras.</span>
        </div>

        <label class="chave pf" :class="{ ligada: form.incluir_pf }">
          <input v-model="form.incluir_pf" type="checkbox" />
          <span>
            <strong>Incluir MEI e empresário individual</strong>
            <span class="aviso-pf">São pessoas físicas: a LGPD se aplica ao contato. A escolha fica registrada na auditoria.</span>
          </span>
        </label>
      </div>
    </section>

    <!-- 4. Qualidade do contato e ordem -->
    <section class="bloco card" :class="{ recolhido: !abertoQualidade }">
      <button type="button" class="bloco-head clicavel" :aria-expanded="abertoQualidade" @click="abertoQualidade = !abertoQualidade">
        <span class="numero">4</span>
        <div>
          <h2>
            Qualidade do contato e ordem <span class="opcional">opcional</span>
            <span v-if="nQualidade" class="badge blue">{{ nQualidade }} em uso</span>
          </h2>
          <p class="muted">Só empresas com celular, telefone ou e-mail; sem contato de contador; quem vem primeiro.</p>
        </div>
        <span class="seta grande" :class="{ aberta: abertoQualidade }" aria-hidden="true">›</span>
      </button>

      <div v-if="abertoQualidade" class="bloco-corpo">
        <div class="chaves">
          <label class="chave"><input v-model="form.com_celular" type="checkbox" /> Só com celular</label>
          <label class="chave"><input v-model="form.com_telefone" type="checkbox" /> Só com telefone</label>
          <label class="chave"><input v-model="form.com_email" type="checkbox" /> Só com e-mail</label>
          <label class="chave">
            <input v-model="form.ocultar_contador" type="checkbox" />
            Ocultar contato de contador
            <span class="muted">(telefone ou e-mail repetido em muitas empresas, ou de contabilidade)</span>
          </label>
        </div>
        <div class="field ordem">
          <label for="ff-ordem">Ordenar por</label>
          <select id="ff-ordem" v-model="form.ordem">
            <option v-for="o in ORDENS" :key="o" :value="o">{{ ordemLabels[o] }}</option>
          </select>
        </div>
      </div>
    </section>
  </fieldset>
</template>

<style scoped>
.filtros {
  border: none;
  padding: 0;
  margin: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.filtros:disabled {
  opacity: 0.6;
}
.bloco {
  padding: 18px 20px;
  margin: 0;
}
.bloco.recolhido {
  padding-bottom: 14px;
}
.bloco-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 14px;
  width: 100%;
  text-align: left;
}
.bloco.recolhido .bloco-head {
  margin-bottom: 0;
}
.bloco-head.clicavel {
  border: none;
  background: none;
  padding: 0;
  font: inherit;
  color: inherit;
  cursor: pointer;
}
.bloco-head > div {
  flex: 1;
  min-width: 0;
}
.bloco-head h2 {
  font-size: 15.5px;
  margin: 1px 0 2px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.bloco-head p {
  margin: 0;
  font-size: 13px;
}
.numero {
  flex-shrink: 0;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--ci-purple);
  color: #fff;
  font-weight: 700;
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.opcional {
  font-size: 11.5px;
  font-weight: 400;
  color: var(--ci-text-3);
  text-transform: lowercase;
}
.seta {
  display: inline-block;
  transition: transform 0.15s;
}
.seta.aberta {
  transform: rotate(90deg);
}
.seta.grande {
  font-size: 22px;
  line-height: 1;
  color: var(--ci-text-3);
}
.bloco-corpo {
  margin-top: 16px;
}
.chave {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 4px 0;
  margin-top: 8px;
  cursor: pointer;
  font-size: 13.5px;
  flex-wrap: wrap;
}
.chave .muted {
  font-size: 12.5px;
}
.chaves .chave {
  margin-top: 2px;
}
.chave.pf {
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--ci-border);
  flex-wrap: nowrap;
}
.chave.pf.ligada {
  border-color: var(--ci-amber);
  background: var(--ci-amber-tint, #fff7e6);
}
.aviso-pf {
  display: block;
  font-size: 12.5px;
  color: var(--ci-text-2);
  margin-top: 2px;
}
.ufs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 14px;
}
.chip.uf {
  min-width: 44px;
  justify-content: center;
}
.cidades {
  margin-bottom: 8px;
}
.aviso-uf {
  font-size: 12.5px;
}
.recolher {
  border: none;
  background: none;
  padding: 4px 0;
  font: inherit;
  font-size: 13.5px;
  color: var(--ci-purple-dark);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.local-fino {
  margin-top: 8px;
}
.opcoes {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.intervalo {
  margin-top: 10px;
}
.ordem {
  max-width: 280px;
  margin-top: 12px;
}
</style>
