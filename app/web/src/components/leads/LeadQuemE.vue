<script setup lang="ts">
import { computed } from 'vue'
import { formatDate, formatMoney, mascaraCNPJ } from '../../format'
import { nivelTons, porteLabels, tipoAlertaLabels, tipoDividaLabels, type LeadCompleto } from '../../types-prospeccao'

/**
 * "Quem é": frases do resumo, sinais, alertas, cadastro, sócios e dívida
 * ativa. Usado na primeira coluna do cockpit e no painel de /leads.
 */
const props = withDefaults(defineProps<{ completo: LeadCompleto; compacto?: boolean }>(), { compacto: false })

const cadastro = computed(() => props.completo.resumo?.cadastro ?? null)
const sinais = computed(() => props.completo.resumo?.sinais ?? [])
const frases = computed(() => props.completo.resumo?.frases ?? [])
const alertas = computed(() => props.completo.alertas ?? [])
const socios = computed(() => cadastro.value?.socios ?? [])
const divida = computed(() => cadastro.value?.divida ?? null)
</script>

<template>
  <div class="quem-e">
    <ul v-if="alertas.length" class="alertas">
      <li v-for="(a, i) in alertas" :key="i" class="faixa" :class="{ amber: a.nivel === 'atencao', red: a.nivel === 'risco' }" :data-alerta="a.tipo">
        <strong>{{ tipoAlertaLabels[a.tipo] ?? a.tipo }}</strong>
        <span>{{ a.texto }}</span>
      </li>
    </ul>

    <ul v-if="frases.length" class="frases">
      <li v-for="(f, i) in frases" :key="i">{{ f }}</li>
    </ul>

    <div v-if="sinais.length" class="sinais">
      <span v-for="(s, i) in sinais" :key="i" class="badge" :class="nivelTons[s.nivel]">{{ s.texto }}</span>
    </div>

    <template v-if="cadastro">
      <component :is="compacto ? 'details' : 'div'" class="bloco">
        <component :is="compacto ? 'summary' : 'h4'">Cadastro</component>
        <dl>
          <template v-if="cadastro.razao_social">
            <dt>Razão social</dt>
            <dd>{{ cadastro.razao_social }}</dd>
          </template>
          <template v-if="completo.lead.cnpj">
            <dt>CNPJ</dt>
            <dd>{{ mascaraCNPJ(completo.lead.cnpj) }}</dd>
          </template>
          <template v-if="cadastro.cnae">
            <dt>Atividade</dt>
            <dd>{{ cadastro.cnae }}</dd>
          </template>
          <dt>Porte</dt>
          <dd>
            {{ porteLabels[cadastro.porte] ?? cadastro.porte }}
            <span v-if="cadastro.mei" class="badge gray">MEI</span>
            <span v-else-if="cadastro.simples" class="badge gray">Simples</span>
          </dd>
          <template v-if="cadastro.abertura">
            <dt>Abertura</dt>
            <dd>{{ formatDate(cadastro.abertura) }}</dd>
          </template>
          <template v-if="cadastro.capital != null">
            <dt>Capital</dt>
            <dd>{{ formatMoney(cadastro.capital) }}</dd>
          </template>
          <template v-if="cadastro.endereco">
            <dt>Endereço</dt>
            <dd>{{ cadastro.endereco }}</dd>
          </template>
          <template v-if="cadastro.email">
            <dt>E-mail</dt>
            <dd><a :href="`mailto:${cadastro.email}`">{{ cadastro.email }}</a></dd>
          </template>
          <template v-if="cadastro.site">
            <dt>Site</dt>
            <dd>{{ cadastro.site }}</dd>
          </template>
        </dl>
      </component>

      <component :is="compacto ? 'details' : 'div'" v-if="socios.length" class="bloco">
        <component :is="compacto ? 'summary' : 'h4'">Sócios ({{ socios.length }})</component>
        <ul class="socios">
          <li v-for="(s, i) in socios" :key="i">
            <strong>{{ s.nome }}</strong>
            <span class="muted">{{ s.qualificacao }}</span>
          </li>
        </ul>
      </component>

      <div v-if="divida" class="bloco divida">
        <h4>Dívida ativa</h4>
        <p>
          <strong>{{ formatMoney(divida.total) }}</strong>
          em {{ divida.inscricoes }} inscrição(ões)<template v-if="divida.ajuizada">, com cobrança judicial</template>
        </p>
        <p class="muted">
          {{ divida.tipos.map((t) => tipoDividaLabels[t] ?? t).join(', ') }}
          <template v-if="divida.referencia"> · referência {{ divida.referencia }}</template>
        </p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.quem-e {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.alertas,
.frases,
.socios {
  list-style: none;
  margin: 0;
  padding: 0;
}

.alertas {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.alertas .faixa {
  align-items: baseline;
  flex-wrap: wrap;
  padding: 8px 12px;
}

.frases li {
  font-size: 14px;
  color: var(--ci-text-2);
  padding: 2px 0;
}

.sinais {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.sinais .badge {
  white-space: normal;
}

.bloco h4,
.bloco summary {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--ci-text-3);
}

.bloco summary {
  cursor: pointer;
  padding: 6px 0;
}

dl {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 4px 12px;
  margin: 0;
  font-size: 13px;
}

dt {
  color: var(--ci-text-3);
}

dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.socios li {
  display: flex;
  flex-direction: column;
  font-size: 13px;
  padding: 4px 0;
}

.divida p {
  margin: 0;
  font-size: 13px;
}
</style>
