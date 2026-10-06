<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Planos from './components/Planos.vue'
import Contato from './components/Contato.vue'
import Icone from './components/Icone.vue'
import { beneficios, criarFaq, grupos, segmentos } from './conteudo'

const dominio = import.meta.env.VITE_DOMINIO || 'crmia.com.br'
const emailComercial = import.meta.env.VITE_EMAIL_COMERCIAL || 'contato@crmia.com.br'

// Textos e planos vêm do painel em tempo real; sem resposta, ficam os padrões.
const dados = ref<any>({ planos: [], textos: {} })
onMounted(async () => {
  try {
    dados.value = await (await fetch('/painel/api/planos')).json()
  } catch {
    /* fica com os textos padrão */
  }
})
const titulo = computed(() => dados.value.textos?.titulo || 'O CRM que a sua equipe realmente usa, com o seu domínio e os seus dados.')
const subtitulo = computed(() => dados.value.textos?.subtitulo || 'Contatos, negócios, sequências de e-mail e automações em um ambiente só seu — banco de dados isolado, e-mail com o seu domínio e pronto para usar em minutos.')
const diasTeste = computed(() => dados.value.textos?.dias_teste || '14')
const whatsapp = computed(() => dados.value.textos?.whatsapp || import.meta.env.VITE_WHATSAPP || '')
const linkWhats = computed(() => `https://wa.me/${whatsapp.value}?text=${encodeURIComponent('Olá! Quero conhecer o CRM IA.')}`)

const faq = criarFaq(dominio)
const menuAberto = ref(false)
const links = [
  { h: '#segmentos', t: 'Para quem' },
  { h: '#funcionalidades', t: 'Funcionalidades' },
  { h: '#como-funciona', t: 'Como funciona' },
  { h: '#planos', t: 'Planos' },
  { h: '#faq', t: 'Dúvidas' }
]
const varEmpresa = '{{empresa}}'
const etapas = [
  { n: 'Lead novo', q: 12 },
  { n: 'Qualificado', q: 8 },
  { n: 'Reunião', q: 5 },
  { n: 'Proposta', q: 4 },
  { n: 'Negociação', q: 2 }
]
</script>

<template>
  <a class="pular" href="#conteudo">Ir para o conteúdo</a>
  <header class="cab">
    <div class="container cab__linha">
      <a href="#" class="logo" aria-label="CRM IA — início">
        <svg viewBox="0 0 64 64" aria-hidden="true"><rect width="64" height="64" rx="16" fill="#6d5df6" /><path d="M20 40c0-8 5-14 12-14s12 6 12 14" fill="none" stroke="#fff" stroke-width="6" stroke-linecap="round" /><circle cx="32" cy="22" r="6" fill="#fff" /><circle cx="46" cy="18" r="4" fill="#c7c2ff" /></svg>
        <span>CRM IA<small>vendas com método</small></span>
      </a>
      <nav id="menu" class="nav" :class="{ 'nav--aberto': menuAberto }" aria-label="Seções da página">
        <a v-for="l in links" :key="l.h" :href="l.h" @click="menuAberto = false">{{ l.t }}</a>
        <a class="nav__entrar" href="/painel/login" @click="menuAberto = false">Área da equipe</a>
      </nav>
      <div class="acoes">
        <a class="btn btn--contorno btn--peq acoes__entrar" href="/painel/login">Entrar</a>
        <a class="btn btn--p btn--peq" href="#contato">Teste grátis</a>
        <button class="hamb" type="button" :aria-expanded="menuAberto" aria-controls="menu" :aria-label="menuAberto ? 'Fechar menu' : 'Abrir menu'" @click="menuAberto = !menuAberto">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path :d="menuAberto ? 'M6 6l12 12M18 6L6 18' : 'M4 7h16M4 12h16M4 17h16'" /></svg>
        </button>
      </div>
    </div>
  </header>

  <main id="conteudo">
    <section class="hero">
      <div class="container hero__grid">
        <div>
          <span class="selo"><b>Ambiente isolado</b> banco de dados e e-mail só da sua empresa</span>
          <h1>{{ titulo }}</h1>
          <p class="sub">{{ subtitulo }}</p>
          <div class="hero__acoes">
            <a class="btn btn--p btn--g" href="#contato">Testar {{ diasTeste }} dias grátis</a>
            <a class="btn btn--contorno btn--g" href="#planos">Ver planos</a>
          </div>
          <ul class="hero__prova">
            <li><Icone nome="check" />A partir de R$ 99 por mês</li>
            <li><Icone nome="check" />Sem taxa de adesão</li>
            <li><Icone nome="check" />Cancele quando quiser</li>
          </ul>
        </div>
        <div class="mock" aria-hidden="true">
          <div class="mock__barra"><i></i><i></i><i></i><span>{{ dominio }}/suaempresa</span></div>
          <div class="mock__corpo">
            <div class="mock__menu"><b>Negócios</b><span>Contatos</span><span>Sequências</span><span>Caixa de entrada</span><span>Relatórios</span></div>
            <div class="mock__main">
              <div class="mock__kpis">
                <div class="mock__kpi"><b>31</b><span>negócios abertos</span></div>
                <div class="mock__kpi"><b>R$ 148 mil</b><span>previsão ponderada</span></div>
                <div class="mock__kpi"><b>62%</b><span>taxa de abertura</span></div>
              </div>
              <div class="mock__kanban">
                <div v-for="e in etapas" :key="e.n" class="mock__col"><b>{{ e.n }}</b><small>{{ e.q }}</small><i v-for="k in Math.min(e.q, 3)" :key="k"></i></div>
              </div>
              <div class="mock__linha"><span><b>Sequência</b> “Prospecção outbound” · e-mail 2 de 5 enviado para 14 contatos</span><span class="pill">dentro da janela</span></div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section id="segmentos" class="secao secao--cinza">
      <div class="container">
        <div class="centro"><p class="sobre">Para quem</p><h2 class="titulo">Comece com um modelo pronto para o seu negócio</h2><p class="sub">No primeiro acesso você escolhe o modelo e o CRM nasce com funil, etapas, modelos de e-mail, uma cadência e campos próprios. Tudo editável depois.</p></div>
        <ul class="segs">
          <li v-for="s in segmentos" :key="s.nome" class="seg"><span class="icbox"><Icone :nome="s.ic" /></span><div><h3>{{ s.nome }}</h3><p>{{ s.desc }}</p></div></li>
        </ul>
      </div>
    </section>

    <section id="funcionalidades" class="secao">
      <div class="container">
        <div class="centro"><p class="sobre">Funcionalidades</p><h2 class="titulo">Tudo o que um time de vendas precisa, sem a complexidade</h2><p class="sub">Quatro frentes que conversam entre si: o que você cadastra uma vez aparece no funil, na sequência, na caixa de entrada e no relatório.</p></div>
        <article v-for="(g, i) in grupos" :id="g.id" :key="g.id" class="grupo" :class="{ 'grupo--inv': i % 2 === 1 }">
          <div class="grupo__txt">
            <p class="sobre">{{ g.sobre }}</p>
            <h3 class="grupo__titulo">{{ g.titulo }}</h3>
            <p class="grupo__resumo">{{ g.resumo }}</p>
            <ul class="itens">
              <li v-for="it in g.itens" :key="it.t"><span class="icbox icbox--p"><Icone :nome="it.ic" /></span><div><h4>{{ it.t }}</h4><p>{{ it.d }}</p></div></li>
            </ul>
          </div>
          <div class="grupo__arte" aria-hidden="true">
            <div v-if="g.id === 'organizar'" class="quadro">
              <div class="quadro__cab"><b>Funil de vendas</b><span>este mês</span></div>
              <div class="funil"><div v-for="e in etapas" :key="e.n" class="funil__lin"><span>{{ e.n }}</span><i :style="{ width: (e.q / 12) * 100 + '%' }"></i><small>{{ e.q }}</small></div></div>
            </div>
            <div v-else-if="g.id === 'automatizar'" class="quadro">
              <div class="quadro__cab"><b>Prospecção outbound</b><span>5 toques</span></div>
              <ol class="cadencia">
                <li><b>Dia 0</b> E-mail automático: “uma ideia para a {{ varEmpresa }}”</li>
                <li><b>Dia 2</b> Tarefa: ligar para o contato</li>
                <li><b>Dia 4</b> E-mail automático: retomada</li>
                <li><b>Dia 7</b> Tarefa: conectar no LinkedIn</li>
                <li><b>Dia 11</b> E-mail de encerramento</li>
              </ol>
              <span class="quadro__btn">Sai sozinha quando o contato responde</span>
            </div>
            <div v-else-if="g.id === 'email'" class="quadro">
              <div class="quadro__cab"><b>Envio de e-mails</b><span class="pill">ativo</span></div>
              <div class="provedores"><span class="prov prov--at">Mandrill · API</span><span class="prov">Mandrill · SMTP</span><span class="prov">Maileroo · SMTP</span></div>
              <div class="campo-mock"><small>Remetente</small><b>vendas@suaempresa.com.br</b></div>
              <div class="campo-mock"><small>Janela de disparo</small><b>seg–sex, 8h às 19h · até 500/dia</b></div>
              <span class="quadro__btn">Enviar e-mail de teste</span>
            </div>
            <div v-else class="quadro">
              <div class="quadro__cab"><b>Previsão do mês</b><span>equipe</span></div>
              <div class="prev"><div><small>Fechado</small><b>R$ 62 mil</b></div><div><small>Comprometido</small><b>R$ 48 mil</b></div><div><small>Melhor caso</small><b>R$ 110 mil</b></div></div>
              <div class="meta"><span>Meta do mês · R$ 150 mil</span><i style="width: 41%"></i></div>
            </div>
          </div>
        </article>
      </div>
    </section>

    <section id="como-funciona" class="secao secao--roxa">
      <div class="container">
        <div class="centro"><p class="sobre">Como funciona</p><h2 class="titulo">Do pedido ao CRM no ar em três passos</h2></div>
        <ol class="passos">
          <li><h3>Peça o teste</h3><p>Conte como é a sua empresa. Criamos o seu ambiente exclusivo em {{ dominio }}/suaempresa, com banco de dados próprio.</p></li>
          <li><h3>Configure em 5 minutos</h3><p>Escolha o modelo do seu negócio, dê a identidade da empresa, conecte o e-mail e convide a equipe no assistente inicial.</p></li>
          <li><h3>Venda com método</h3><p>Importe a base, ligue as sequências e acompanhe o funil. Quando decidir ficar, cadastre o cartão e a assinatura mensal começa.</p></li>
        </ol>
      </div>
    </section>

    <section id="beneficios" class="secao">
      <div class="container">
        <div class="centro"><p class="sobre">Por que o CRM IA</p><h2 class="titulo">Menos planilha, mais negócio fechado</h2></div>
        <ul class="benef">
          <li v-for="b in beneficios" :key="b.t"><span class="icbox icbox--p"><Icone :nome="b.ic" /></span><h3>{{ b.t }}</h3><p>{{ b.d }}</p></li>
        </ul>
      </div>
    </section>

    <section id="planos" class="secao secao--cinza">
      <div class="container">
        <div class="centro"><p class="sobre">Planos</p><h2 class="titulo">Um preço por equipe, cobrado no cartão</h2><p class="sub">Sem taxa de adesão, sem fidelidade e com {{ diasTeste }} dias grátis para testar com a sua operação de verdade.</p></div>
        <Planos :planos="dados.planos" :dias-teste="diasTeste" />
      </div>
    </section>

    <section id="faq" class="secao">
      <div class="container">
        <div class="centro"><p class="sobre">Perguntas frequentes</p><h2 class="titulo">O que costumam perguntar</h2></div>
        <div class="faq"><details v-for="f in faq" :key="f.p"><summary>{{ f.p }}</summary><p>{{ f.r }}</p></details></div>
      </div>
    </section>

    <section id="contato" class="secao secao--cinza">
      <div class="container duas">
        <div>
          <p class="sobre">Contato</p>
          <h2 class="titulo">Teste grátis por {{ diasTeste }} dias</h2>
          <p class="sub">Conte como é a sua empresa e preparamos o seu ambiente. Sem cartão de crédito e sem compromisso.</p>
          <ul class="lista-check">
            <li><Icone nome="check" />Ambiente próprio, com banco de dados isolado</li>
            <li><Icone nome="check" />Modelo de CRM pronto para o seu segmento</li>
            <li><Icone nome="check" />Ajuda para importar a base e configurar o e-mail</li>
          </ul>
          <p class="contato__canais">Prefere falar direto? <a v-if="whatsapp" :href="linkWhats" target="_blank" rel="noopener">WhatsApp</a><template v-if="whatsapp"> · </template><a :href="'mailto:' + emailComercial">{{ emailComercial }}</a></p>
        </div>
        <Contato :planos="dados.planos" />
      </div>
    </section>
  </main>

  <footer class="rodape">
    <div class="container rodape__grade">
      <div><a href="#" class="logo"><span>CRM IA<small>vendas com método</small></span></a><p class="rodape__sobre">CRM na nuvem com ambiente isolado por empresa, e-mail no seu domínio e sequências que trabalham por você.</p></div>
      <div><h3>Produto</h3><a href="#segmentos">Para quem</a><a href="#funcionalidades">Funcionalidades</a><a href="#como-funciona">Como funciona</a><a href="#planos">Planos</a></div>
      <div><h3>Acesso</h3><a href="/painel/login">Área da equipe</a><a href="#contato">Teste grátis</a><a href="#faq">Dúvidas</a></div>
      <div><h3>Contato</h3><a :href="'mailto:' + emailComercial">{{ emailComercial }}</a><a v-if="whatsapp" :href="linkWhats" target="_blank" rel="noopener">WhatsApp</a></div>
      <div class="rodape__fim"><span>© {{ new Date().getFullYear() }} CRM IA. Pagamento seguro no cartão de crédito.</span><span>{{ dominio }}</span></div>
    </div>
  </footer>
</template>
