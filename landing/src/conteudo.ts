// Conteúdo institucional da landing. Fica aqui, e não dentro do componente,
// porque é usado em dois lugares que não podem divergir: a página e os dados
// estruturados (JSON-LD) gerados na pré-renderização.

export interface Item { ic: string; t: string; d: string }
export interface Grupo { id: string; sobre: string; titulo: string; resumo: string; itens: Item[] }
export interface Beneficio { ic: string; t: string; d: string }
export interface Pergunta { p: string; r: string }
export interface Segmento { ic: string; nome: string; desc: string }

export const segmentos: Segmento[] = [
  { ic: 'pasta', nome: 'Vendas B2B', desc: 'Funil de qualificação a fechamento, cadência de prospecção e campos de decisão de compra.' },
  { ic: 'loja', nome: 'Serviços e atendimento local', desc: 'Clínicas, estúdios, escritórios e prestadores: do contato ao serviço feito, com lembretes.' },
  { ic: 'casa', nome: 'Imobiliárias', desc: 'Visitas, propostas e documentação, com os campos que o corretor precisa na mão.' },
  { ic: 'carrinho', nome: 'Varejo e e-commerce', desc: 'Relacionamento e recompra: quem comprou, quando e o que voltar a oferecer.' },
  { ic: 'lampada', nome: 'Agências e consultorias', desc: 'Briefing, proposta, aprovação e kickoff — projetos e retainers em funis separados.' },
  { ic: 'faisca', nome: 'Do seu jeito', desc: 'Comece com um funil simples e monte etapas, campos e automações como preferir.' }
]

export const grupos: Grupo[] = [
  {
    id: 'organizar',
    sobre: 'Organização',
    titulo: 'Toda a sua carteira em um lugar, do primeiro contato ao fechamento',
    resumo: 'Contatos, empresas e negócios ligados entre si, com a história completa de cada relação na timeline.',
    itens: [
      { ic: 'pessoas', t: 'Contatos e empresas', d: 'Cadastro completo, estágios de ciclo de vida, importação de planilhas com histórico e detecção de duplicados.' },
      { ic: 'kanban', t: 'Negócios em kanban', d: 'Vários funis com etapas e probabilidade próprias; arraste o card e a previsão se recalcula.' },
      { ic: 'check', t: 'Tarefas, ligações e reuniões', d: 'O que cada pessoa precisa fazer hoje, com atrasadas em destaque e página pública de agendamento.' },
      { ic: 'relogio', t: 'Timeline', d: 'Notas, e-mails, ligações e eventos do sistema por contato, empresa e negócio.' }
    ]
  },
  {
    id: 'automatizar',
    sobre: 'Automação',
    titulo: 'Sequências que prospectam enquanto a equipe fecha',
    resumo: 'Cadências de e-mail com tarefas intercaladas, automações por gatilho e regras de disparo que respeitam horário e volume.',
    itens: [
      { ic: 'seta', t: 'Sequências de prospecção', d: 'E-mails automáticos e tarefas manuais numa cadência que para sozinha quando o contato responde ou agenda.' },
      { ic: 'raio', t: 'Automações com gatilhos', d: 'Contato criado, negócio parado, formulário enviado: ações encadeadas com espera entre passos.' },
      { ic: 'calendario', t: 'Janela de disparo e teto diário', d: 'Nada sai de madrugada ou em excesso: você define horário, dias e limite por dia.' },
      { ic: 'grafico', t: 'Rastreio de aberturas e cliques', d: 'Cada e-mail enviado pelo CRM mostra quem abriu e quem clicou, por período.' }
    ]
  },
  {
    id: 'email',
    sobre: 'E-mail no seu domínio',
    titulo: 'Os e-mails saem com o nome da sua empresa, do seu provedor',
    resumo: 'Conecte Mandrill ou Maileroo em dois minutos. Respostas caem na caixa de entrada compartilhada do CRM.',
    itens: [
      { ic: 'email', t: 'Mandrill ou Maileroo', d: 'Escolha o provedor, informe a chave e teste o envio na mesma tela. O remetente é o seu domínio.' },
      { ic: 'caixa', t: 'Caixa de entrada compartilhada', d: 'Respostas dos contatos viram conversas com fila, dono e comentários internos.' },
      { ic: 'modelo', t: 'Modelos e snippets', d: 'Biblioteca de e-mails com variáveis do contato e atalhos de texto para responder rápido.' },
      { ic: 'formulario', t: 'Formulários de captura', d: 'Construtor com endereço próprio e código de incorporação; cada envio vira contato.' }
    ]
  },
  {
    id: 'medir',
    sobre: 'Gestão',
    titulo: 'Previsão, metas e relatórios sem exportar nada',
    resumo: 'O gestor acompanha a equipe em tempo real; o vendedor vê o próprio dia numa fila só.',
    itens: [
      { ic: 'alvo', t: 'Previsão e metas', d: 'Previsão por etapa e por categoria, metas por pessoa e equipe, envio de previsão pelo vendedor.' },
      { ic: 'grafico', t: 'Relatórios customizáveis', d: 'Conversão de funil, duração por etapa, progresso mensal e painéis montados pela equipe.' },
      { ic: 'escudo', t: 'Permissões e auditoria', d: 'Perfis com matriz de permissões configurável e trilha de tudo o que foi feito.' },
      { ic: 'nuvem', t: 'Ambiente só seu', d: 'Banco de dados, cache e aplicação separados por empresa, no seu endereço, com versão controlada.' }
    ]
  }
]

export const beneficios: Beneficio[] = [
  { ic: 'faisca', t: 'Pronto em minutos', d: 'Escolha um modelo para o seu negócio e o CRM nasce com funil, e-mails e campos no lugar.' },
  { ic: 'escudo', t: 'Dados isolados', d: 'Cada empresa tem o próprio banco de dados. Nada é compartilhado com outros clientes.' },
  { ic: 'email', t: 'E-mail que chega', d: 'Envio pelo seu domínio, com SPF e DKIM, por Mandrill ou Maileroo. Sem remetente genérico.' },
  { ic: 'pessoas', t: 'Preço por equipe', d: 'Planos por faixa de usuários, cobrança mensal no cartão e cancelamento quando quiser.' },
  { ic: 'nuvem', t: 'Sem instalação', d: 'Funciona no navegador do computador e do celular. Atualizações sem parar a operação.' },
  { ic: 'check', t: 'Sem fidelidade', d: 'Teste grátis, sem taxa de adesão. Peça upgrade ou cancelamento dentro do próprio CRM.' }
]

export function criarFaq(dominio: string): Pergunta[] {
  return [
    { p: 'Para que tipo de empresa o CRM IA serve?', r: 'Para equipes que vendem e atendem por relacionamento: vendas B2B, serviços locais, imobiliárias, varejo, agências e consultorias. No primeiro acesso você escolhe um modelo e o CRM já vem com funil, e-mails e campos adequados ao seu negócio.' },
    { p: 'Preciso instalar algum programa?', r: 'Não. O sistema roda na nuvem e é acessado pelo navegador, no computador ou no celular.' },
    { p: 'Meus dados ficam misturados com os de outras empresas?', r: `Não. Cada empresa tem um ambiente isolado, com banco de dados próprio, no endereço ${dominio}/suaempresa.` },
    { p: 'Como os e-mails são enviados?', r: 'Pelo seu provedor de e-mail transacional: Mandrill (API ou SMTP) ou Maileroo (SMTP). Você informa a chave em Configurações, testa o envio e pronto: os e-mails saem com o seu domínio, e as respostas caem na caixa de entrada do CRM.' },
    { p: 'Como funciona a cobrança?', r: 'Por assinatura mensal no cartão de crédito, com valor fixo conforme o plano. Você cadastra o cartão numa página segura e a cobrança acontece todo mês, automaticamente. Pode pedir upgrade ou cancelar dentro do CRM, em “Meu plano”.' },
    { p: 'Tem taxa de adesão ou fidelidade?', r: 'Não. Você paga a mensalidade do plano escolhido e pode cancelar quando quiser; o acesso vale até o fim do período já pago.' },
    { p: 'Como funciona o período de teste?', r: 'Você usa o sistema completo durante o período de teste, sem cartão. Se gostar, escolhe o plano e cadastra o cartão; se não, o acesso se encerra.' },
    { p: 'Dá para importar a minha base atual?', r: 'Sim. O centro de importações aceita CSV e XLSX, atualiza registros por chave (e-mail ou CNPJ) e guarda o histórico de cada carga.' }
  ]
}
