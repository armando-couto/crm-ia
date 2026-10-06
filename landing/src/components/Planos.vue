<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ planos: any[]; diasTeste: string }>()
const brl = (c: number) => (c / 100).toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: c % 100 ? 2 : 0, maximumFractionDigits: 2 })

// Fallback se o painel não responder. Os valores de verdade são os do painel.
const padrao = [
  { codigo: 'essencial', nome: 'Essencial', descricao: 'Para equipes pequenas saindo da planilha.', faixa: '1 a 3 usuários', preco_mensal_centavos: 9900, destaque: false, recursos: ['Até 3 usuários', 'Contatos, empresas e negócios em kanban', 'Tarefas, reuniões e timeline', 'Modelos de e-mail e envio pelo CRM', 'Formulários de captura', 'Dashboard de vendas'] },
  { codigo: 'profissional', nome: 'Profissional', descricao: 'Para times de vendas que vivem de cadência.', faixa: '4 a 10 usuários', preco_mensal_centavos: 24900, destaque: true, recursos: ['Tudo do Essencial', 'Até 10 usuários', 'Sequências de prospecção automáticas', 'Automações com gatilhos e esperas', 'Caixa de entrada compartilhada', 'Previsão, metas e relatórios'] },
  { codigo: 'escala', nome: 'Escala', descricao: 'Para operações maiores, com várias equipes ou unidades.', faixa: '11 a 30 usuários', preco_mensal_centavos: 59900, destaque: false, recursos: ['Tudo do Profissional', 'Até 30 usuários', 'Grupo de CNPJs (filiais e franquias)', 'Importações em massa com histórico', 'Relatórios customizáveis e painéis', 'Gerente de contas'] },
  { codigo: 'sob-medida', nome: 'Sob medida', descricao: 'Acima de 30 usuários ou integrações específicas.', faixa: 'a partir de 31 usuários', preco_mensal_centavos: 0, sob_consulta: true, destaque: false, recursos: ['Tudo do Escala', 'Usuários conforme a operação', 'Implantação acompanhada'] }
]
const lista = computed(() => (props.planos?.length ? props.planos : padrao))
</script>
<template>
  <div class="planos">
    <div v-for="p in lista" :key="p.codigo" class="plano" :class="{ 'plano--destaque': p.destaque }">
      <span v-if="p.destaque" class="plano__tag">MAIS ESCOLHIDO</span>
      <h3>{{ p.nome }}</h3>
      <span class="plano__faixa">{{ p.faixa }}</span>
      <p class="plano__desc">{{ p.descricao }}</p>
      <div v-if="p.sob_consulta || !p.preco_mensal_centavos" class="plano__preco"><b class="plano__consulta">Sob consulta</b></div>
      <div v-else class="plano__preco"><b>{{ brl(p.preco_mensal_centavos) }}</b><span>/mês</span></div>
      <ul><li v-for="r in p.recursos" :key="r">{{ r }}</li></ul>
      <a class="btn" :class="p.destaque ? 'btn--p' : 'btn--contorno'" href="#contato">{{ p.sob_consulta || !p.preco_mensal_centavos ? 'Falar com o comercial' : `Testar ${diasTeste} dias grátis` }}</a>
    </div>
  </div>
  <p class="planos__nota">Preços em reais, por empresa, cobrados mensalmente no cartão de crédito. Sem taxa de adesão e sem fidelidade.</p>
</template>
