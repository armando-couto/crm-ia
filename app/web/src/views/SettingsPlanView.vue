<script setup lang="ts">
// Meu plano: licença, faturas e forma de pagamento. Os dados vêm da
// plataforma; o cliente vê só o que importa para ele — plano, valor, próximo
// vencimento e o cartão cadastrado.
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'
import { useToastStore } from '../stores/toast'
import { formatDate, formatDateTime } from '../format'
import type { MyPlan, PlanOffer } from '../types'

const auth = useAuthStore()
const toast = useToastStore()
const plan = ref<MyPlan | null>(null)
const loading = ref(true)
const error = ref('')
const sending = ref(false)

const brl = (c: number) => (c / 100).toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
const pendente = computed(() => plan.value?.solicitacao?.status === 'pendente')
const enabled = computed(() => auth.workspace?.plan_enabled ?? false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    plan.value = await api.get<MyPlan>('/plano')
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function solicitar(body: { tipo: 'upgrade' | 'cancelamento'; plano_id?: number; mensagem: string }, ok: string) {
  sending.value = true
  try {
    await api.post('/plano/solicitacoes', body)
    toast.push(ok)
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    sending.value = false
  }
}

function pedirPlano(p: PlanOffer) {
  if (!confirm(`Pedir a troca para o plano ${p.nome} (${brl(p.valor_centavos)}/mês)? Nossa equipe confirma e a mudança aparece aqui.`)) return
  solicitar({ tipo: 'upgrade', plano_id: p.id, mensagem: '' }, 'Pedido enviado! A resposta aparece nesta tela.')
}

function pedirCancelamento() {
  const motivo = prompt('Pedir o cancelamento do plano? Conte o motivo (opcional):')
  if (motivo === null) return
  solicitar({ tipo: 'cancelamento', mensagem: motivo.trim() }, 'Pedido de cancelamento enviado.')
}

onMounted(() => {
  if (enabled.value) load()
  else loading.value = false
})
</script>

<template>
  <div class="page">
    <div class="page-head"><h1>Meu plano</h1></div>

    <div v-if="!enabled" class="card"><p class="muted">A gestão do plano não está disponível neste ambiente. Fale com o nosso suporte.</p></div>
    <div v-else-if="loading" class="muted">Carregando…</div>
    <div v-else-if="error" class="card"><p class="error-text">{{ error }}</p><button class="btn btn-outline btn-sm" type="button" @click="load">Tentar de novo</button></div>

    <template v-else-if="plan">
      <div class="card" style="margin-bottom: 16px">
        <template v-if="plan.plano">
          <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap">
            <h2>Plano {{ plan.plano.nome }}</h2>
            <button class="btn btn-danger btn-sm" type="button" :disabled="pendente" @click="pedirCancelamento">Pedir cancelamento</button>
          </div>
          <div class="kpis">
            <div><span>Valor</span><strong>{{ brl(plan.plano.valor_centavos) }}</strong><small>{{ plan.plano.periodicidade }}</small></div>
            <div><span>Usuários</span><strong>até {{ plan.plano.usuarios_max }}</strong></div>
            <div><span>Válido até</span><strong>{{ formatDate(plan.plano.fim) }}</strong></div>
            <div v-if="plan.pagamento"><span>Pagamento</span><strong>{{ plan.pagamento.cartao_final ? `cartão •••• ${plan.pagamento.cartao_final}` : plan.pagamento.status }}</strong></div>
          </div>
        </template>
        <p v-else class="muted">Nenhum plano ativo neste ambiente. Escolha um plano abaixo ou fale com o nosso suporte.</p>

        <div v-if="plan.pagamento?.link" class="pay">
          <p>{{ plan.pagamento.status === 'aguardando_cartao' ? 'Cadastre o cartão de crédito para ativar a cobrança mensal automática.' : 'Para trocar o cartão da cobrança mensal, use o link abaixo.' }}</p>
          <a class="btn btn-primary" :href="plan.pagamento.link" target="_blank" rel="noopener">{{ plan.pagamento.status === 'aguardando_cartao' ? 'Cadastrar cartão' : 'Atualizar cartão' }}</a>
        </div>

        <div v-if="plan.solicitacao" class="request" :class="pendente ? 'pending' : ''">
          Pedido de {{ plan.solicitacao.tipo === 'cancelamento' ? 'cancelamento' : `troca para o plano ${plan.solicitacao.plano_nome}` }} —
          <strong>{{ plan.solicitacao.status }}</strong> (enviado em {{ formatDateTime(plan.solicitacao.criado_em) }}).
          <span v-if="plan.solicitacao.resposta"> Resposta: {{ plan.solicitacao.resposta }}</span>
        </div>
      </div>

      <div class="card" style="margin-bottom: 16px">
        <h2 style="margin-bottom: 10px">Faturas</h2>
        <div v-if="!plan.faturas.length" class="muted">Nenhuma fatura emitida ainda.</div>
        <div v-else class="table-wrap">
          <table class="data">
            <thead><tr><th>Competência</th><th>Vencimento</th><th>Situação</th><th style="text-align: right">Valor</th></tr></thead>
            <tbody>
              <tr v-for="f in plan.faturas" :key="f.id">
                <td>{{ f.competencia }}</td>
                <td>{{ formatDate(f.vencimento) }}</td>
                <td><span class="pill" :class="f.status">{{ f.status }}</span><small v-if="f.pago_em" class="muted"> em {{ formatDate(f.pago_em) }}</small></td>
                <td style="text-align: right">{{ brl(f.valor_centavos) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-if="plan.planos.length" class="card">
        <h2 style="margin-bottom: 10px">Planos disponíveis</h2>
        <div class="offers">
          <div v-for="p in plan.planos" :key="p.id" class="offer" :class="{ current: p.atual }">
            <strong>{{ p.nome }}</strong>
            <span class="price">{{ brl(p.valor_centavos) }}<small>/mês</small></span>
            <small class="muted">{{ p.faixa }}</small>
            <p class="muted">{{ p.descricao }}</p>
            <ul><li v-for="r in p.recursos" :key="r">{{ r }}</li></ul>
            <button v-if="!p.atual" class="btn btn-outline btn-sm" type="button" :disabled="pendente || sending" @click="pedirPlano(p)">Pedir este plano</button>
            <span v-else class="pill paga">plano atual</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; margin-top: 14px; }
.kpis div { background: var(--ci-bg); border-radius: 10px; padding: 12px; display: flex; flex-direction: column; }
.kpis span { font-size: 12px; color: var(--ci-text-3); text-transform: uppercase; letter-spacing: 0.05em; }
.kpis strong { font-size: 18px; }
.kpis small { color: var(--ci-text-3); }
.pay { margin-top: 14px; padding: 14px; border-radius: 10px; background: var(--ci-purple-tint); display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.request { margin-top: 12px; padding: 10px 12px; border-radius: 8px; background: var(--ci-amber-tint); font-size: 14px; }
.request.pending { background: var(--ci-blue-tint); }
.pill { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--ci-amber-tint); color: var(--ci-amber); font-weight: 600; }
.pill.paga { background: var(--ci-green-tint); color: var(--ci-green); }
.pill.vencida { background: var(--ci-red-tint); color: var(--ci-red); }
.offers { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
.offer { border: 1px solid var(--ci-border); border-radius: 12px; padding: 14px; display: flex; flex-direction: column; gap: 6px; }
.offer.current { border-color: var(--ci-purple); }
.offer ul { margin: 0; padding-left: 18px; font-size: 13px; flex: 1; }
.price { font-size: 20px; font-weight: 700; }
.price small { font-size: 12px; font-weight: 400; color: var(--ci-text-3); }
</style>
