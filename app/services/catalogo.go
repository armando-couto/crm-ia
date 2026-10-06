package services

import "github.com/armando-couto/crm-ia/app/models"

// -----------------------------------------------------------------------------
// Catálogo de modelos de CRM. No setup o cliente escolhe o que mais parece com
// o negócio dele e o ambiente nasce com pipeline, etapas, modelos de e-mail,
// uma cadência de prospecção e campos próprios já no lugar — em vez de uma
// tela vazia. Tudo é editável depois; o modelo só poupa a primeira hora.
// -----------------------------------------------------------------------------

type EtapaModelo struct {
	Nome          string
	Probabilidade int
	Ganho, Perda  bool
}

type PipelineModelo struct {
	Nome   string
	Etapas []EtapaModelo
}

type EmailModelo struct {
	Nome, Assunto, Corpo string
}

type PassoModelo struct {
	Tipo       string // models.StepEmailAuto | task_call | task_email | task_general | task_linkedin
	DiasDepois int
	Assunto    string
	Corpo      string
	Titulo     string
	Nota       string
}

type CadenciaModelo struct {
	Nome, Descricao string
	Passos          []PassoModelo
}

type PropriedadeModelo struct {
	Entidade, Rotulo, Tipo, Grupo string
	Opcoes                        []string
}

type SnippetModelo struct {
	Nome, Atalho, Corpo string
}

// CRMTemplate é um pacote completo de configuração inicial.
type CRMTemplate struct {
	Codigo       string   `json:"codigo"`
	Nome         string   `json:"nome"`
	Descricao    string   `json:"descricao"`
	Icone        string   `json:"icone"`
	Segmentos    []string `json:"segmentos"`
	Destaques    []string `json:"destaques"`
	Pipelines    []PipelineModelo
	Emails       []EmailModelo
	Cadencias    []CadenciaModelo
	Propriedades []PropriedadeModelo
	Snippets     []SnippetModelo
}

// Resumo é a forma pública do catálogo (sem o conteúdo inteiro).
type ResumoTemplate struct {
	Codigo    string   `json:"codigo"`
	Nome      string   `json:"nome"`
	Descricao string   `json:"descricao"`
	Icone     string   `json:"icone"`
	Segmentos []string `json:"segmentos"`
	Destaques []string `json:"destaques"`
	Pipelines int      `json:"pipelines"`
	Etapas    int      `json:"etapas"`
	Emails    int      `json:"emails"`
	Cadencias int      `json:"cadencias"`
	Campos    int      `json:"campos"`
}

func (t CRMTemplate) Resumo() ResumoTemplate {
	etapas := 0
	for _, p := range t.Pipelines {
		etapas += len(p.Etapas)
	}
	return ResumoTemplate{Codigo: t.Codigo, Nome: t.Nome, Descricao: t.Descricao, Icone: t.Icone,
		Segmentos: t.Segmentos, Destaques: t.Destaques, Pipelines: len(t.Pipelines), Etapas: etapas,
		Emails: len(t.Emails), Cadencias: len(t.Cadencias), Campos: len(t.Propriedades)}
}

// TemplatePorCodigo localiza um modelo do catálogo.
func TemplatePorCodigo(codigo string) (CRMTemplate, bool) {
	for _, t := range Catalogo {
		if t.Codigo == codigo {
			return t, true
		}
	}
	return CRMTemplate{}, false
}

func funilPadrao(nome string, etapas ...string) PipelineModelo {
	p := PipelineModelo{Nome: nome}
	n := len(etapas)
	for i, e := range etapas {
		prob := int(float64(i+1) / float64(n+1) * 100)
		p.Etapas = append(p.Etapas, EtapaModelo{Nome: e, Probabilidade: prob})
	}
	p.Etapas = append(p.Etapas,
		EtapaModelo{Nome: "Fechado ganho", Probabilidade: 100, Ganho: true},
		EtapaModelo{Nome: "Fechado perdido", Probabilidade: 0, Perda: true})
	return p
}

var cadenciaB2B = CadenciaModelo{
	Nome:      "Prospecção outbound (5 toques)",
	Descricao: "Dois e-mails, uma ligação, um toque no LinkedIn e o e-mail de encerramento. Sai sozinha quando o contato responde.",
	Passos: []PassoModelo{
		{Tipo: models.StepEmailAuto, DiasDepois: 0, Assunto: "{{nome}}, uma ideia para a {{empresa}}",
			Corpo: "Olá {{nome}},\n\nVi o trabalho da {{empresa}} e acredito que podemos ajudar a equipe a ganhar tempo no dia a dia.\n\nFaz sentido uma conversa de 15 minutos esta semana?\n\nAbraço,"},
		{Tipo: models.StepTaskCall, DiasDepois: 2, Titulo: "Ligar para {{nome}}", Nota: "Mencionar o e-mail enviado e perguntar quem cuida do tema na empresa."},
		{Tipo: models.StepEmailAuto, DiasDepois: 4, Assunto: "Re: uma ideia para a {{empresa}}",
			Corpo: "{{nome}}, retomando o assunto: separei um exemplo rápido de como outras empresas do seu segmento estão fazendo.\n\nPosso te mostrar em 15 minutos?"},
		{Tipo: models.StepTaskLinkedIn, DiasDepois: 7, Titulo: "Conectar com {{nome}} no LinkedIn", Nota: "Convite curto, sem pitch."},
		{Tipo: models.StepEmailAuto, DiasDepois: 11, Assunto: "Encerrando por aqui, {{nome}}",
			Corpo: "{{nome}}, não quero ocupar sua caixa de entrada. Se o tema voltar a ser prioridade, é só responder este e-mail.\n\nObrigado!"},
	},
}

var cadenciaReativacao = CadenciaModelo{
	Nome:      "Reativação de clientes inativos",
	Descricao: "Para quem comprou e sumiu: um lembrete, uma oferta e uma pergunta aberta.",
	Passos: []PassoModelo{
		{Tipo: models.StepEmailAuto, DiasDepois: 0, Assunto: "Sentimos sua falta, {{nome}}",
			Corpo: "Olá {{nome}}, faz um tempo que não nos falamos. Tem algo em que possamos ajudar hoje?"},
		{Tipo: models.StepEmailAuto, DiasDepois: 5, Assunto: "Uma condição especial para você voltar",
			Corpo: "{{nome}}, preparamos uma condição exclusiva para clientes que já conhecem nosso trabalho. Quer saber mais?"},
		{Tipo: models.StepTaskCall, DiasDepois: 9, Titulo: "Ligar para {{nome}} (reativação)", Nota: "Entender o motivo do afastamento."},
	},
}

var emailsComuns = []EmailModelo{
	{Nome: "Primeiro contato", Assunto: "{{nome}}, podemos conversar?", Corpo: "Olá {{nome}},\n\nMeu nome é [seu nome] e trabalho na [sua empresa]. Ajudamos empresas como a {{empresa}} a [resultado].\n\nTeria 15 minutos esta semana para uma conversa rápida?\n\nAbraço,"},
	{Nome: "Follow-up após reunião", Assunto: "Resumo da nossa conversa", Corpo: "Olá {{nome}},\n\nObrigado pelo tempo hoje. Como combinado, seguem os próximos passos:\n\n1. [passo]\n2. [passo]\n\nQualquer dúvida, é só responder.\n\nAbraço,"},
	{Nome: "Envio de proposta", Assunto: "Proposta para a {{empresa}}", Corpo: "Olá {{nome}},\n\nSegue a proposta que conversamos. Preparei com base no que você me contou sobre [necessidade].\n\nPodemos revisar juntos amanhã?\n\nAbraço,"},
	{Nome: "Cobrança gentil de retorno", Assunto: "Ainda faz sentido, {{nome}}?", Corpo: "Olá {{nome}},\n\nNão tive retorno sobre a proposta. Mudou algo por aí? Se preferir, me diga a melhor forma de seguir.\n\nAbraço,"},
}

var snippetsComuns = []SnippetModelo{
	{Nome: "Agradecimento", Atalho: "#obrigado", Corpo: "Obrigado pelo retorno, {{nome}}! Fico à disposição."},
	{Nome: "Pedir horário", Atalho: "#horario", Corpo: "Qual o melhor horário para conversarmos: amanhã às 10h ou às 15h?"},
	{Nome: "Assinatura", Atalho: "#ass", Corpo: "[Seu nome]\n[Cargo] · [Empresa]\n[Telefone]"},
}

// Catalogo é a lista oferecida no setup.
var Catalogo = []CRMTemplate{
	{
		Codigo: "vendas_b2b", Nome: "Vendas B2B", Icone: "briefcase",
		Descricao: "Para equipes que vendem para empresas: funil de qualificação a fechamento, cadência de prospecção e campos de decisão de compra.",
		Segmentos: []string{"Software", "Serviços", "Consultoria", "Indústria"},
		Destaques: []string{"Funil com 6 etapas e probabilidade", "Cadência outbound de 5 toques", "Campos de decisor e orçamento"},
		Pipelines: []PipelineModelo{funilPadrao("Funil de vendas", "Lead novo", "Qualificado", "Reunião agendada", "Proposta enviada", "Negociação")},
		Emails:    emailsComuns,
		Cadencias: []CadenciaModelo{cadenciaB2B, cadenciaReativacao},
		Propriedades: []PropriedadeModelo{
			{Entidade: "deals", Rotulo: "Orçamento disponível", Tipo: models.FieldNumero, Grupo: "Qualificação"},
			{Entidade: "deals", Rotulo: "Decisor identificado", Tipo: models.FieldBooleano, Grupo: "Qualificação"},
			{Entidade: "deals", Rotulo: "Concorrente", Tipo: models.FieldTexto, Grupo: "Qualificação"},
			{Entidade: "companies", Rotulo: "Porte", Tipo: models.FieldSelecao, Grupo: "Perfil", Opcoes: []string{"Micro", "Pequena", "Média", "Grande"}},
			{Entidade: "contacts", Rotulo: "Papel na decisão", Tipo: models.FieldSelecao, Grupo: "Perfil", Opcoes: []string{"Decisor", "Influenciador", "Usuário", "Comprador"}},
		},
		Snippets: snippetsComuns,
	},
	{
		Codigo: "servicos_locais", Nome: "Serviços e atendimento local", Icone: "store",
		Descricao: "Clínicas, estúdios, salões, escritórios e prestadores: do primeiro contato ao serviço feito, com lembretes e pós-atendimento.",
		Segmentos: []string{"Clínicas", "Beleza", "Escritórios", "Prestadores"},
		Destaques: []string{"Funil curto, do contato ao serviço", "E-mails de confirmação e pós-atendimento", "Campos de origem e preferência"},
		Pipelines: []PipelineModelo{funilPadrao("Atendimentos", "Contato recebido", "Orçamento enviado", "Agendado", "Em atendimento")},
		Emails: append([]EmailModelo{
			{Nome: "Confirmação de agendamento", Assunto: "Seu horário está confirmado, {{nome}}", Corpo: "Olá {{nome}},\n\nSeu atendimento está confirmado para [data e hora]. Se precisar remarcar, responda este e-mail.\n\nAté lá!"},
			{Nome: "Pós-atendimento", Assunto: "Como foi sua experiência, {{nome}}?", Corpo: "Olá {{nome}},\n\nObrigado pela visita! Conte para nós como foi: sua opinião ajuda a melhorar o atendimento.\n\nAbraço,"},
		}, emailsComuns[:2]...),
		Cadencias: []CadenciaModelo{cadenciaReativacao},
		Propriedades: []PropriedadeModelo{
			{Entidade: "contacts", Rotulo: "Como nos conheceu", Tipo: models.FieldSelecao, Grupo: "Origem", Opcoes: []string{"Indicação", "Instagram", "Google", "Passou na frente", "Outro"}},
			{Entidade: "contacts", Rotulo: "Preferência de contato", Tipo: models.FieldSelecao, Grupo: "Atendimento", Opcoes: []string{"WhatsApp", "Telefone", "E-mail"}},
			{Entidade: "deals", Rotulo: "Serviço de interesse", Tipo: models.FieldTexto, Grupo: "Atendimento"},
		},
		Snippets: snippetsComuns,
	},
	{
		Codigo: "imobiliaria", Nome: "Imobiliária e incorporação", Icone: "home",
		Descricao: "Captação, visitas e propostas de imóveis, com os campos que o corretor precisa na mão.",
		Segmentos: []string{"Imobiliárias", "Incorporadoras", "Corretores"},
		Destaques: []string{"Funil de visita a escritura", "Campos de imóvel, faixa de valor e financiamento", "Cadência pós-visita"},
		Pipelines: []PipelineModelo{funilPadrao("Vendas de imóveis", "Interesse", "Visita agendada", "Visita realizada", "Proposta", "Documentação")},
		Emails: append([]EmailModelo{
			{Nome: "Confirmação de visita", Assunto: "Visita confirmada: {{nome}}", Corpo: "Olá {{nome}},\n\nSua visita está confirmada para [data e hora], no endereço [endereço]. Te encontro lá!\n\nAbraço,"},
			{Nome: "Imóveis parecidos", Assunto: "Separei opções parecidas para você", Corpo: "Olá {{nome}},\n\nCom base no que você procura, separei [N] imóveis parecidos. Quer visitar algum deles esta semana?"},
		}, emailsComuns[1:3]...),
		Cadencias: []CadenciaModelo{{
			Nome: "Pós-visita", Descricao: "Depois da visita: feedback, opções parecidas e fechamento.",
			Passos: []PassoModelo{
				{Tipo: models.StepEmailAuto, DiasDepois: 1, Assunto: "O que achou do imóvel, {{nome}}?", Corpo: "Olá {{nome}}, o que você achou da visita de ontem? Me conta o que mais gostou e o que faltou."},
				{Tipo: models.StepTaskCall, DiasDepois: 3, Titulo: "Ligar para {{nome}} sobre a visita"},
				{Tipo: models.StepEmailAuto, DiasDepois: 7, Assunto: "Novas opções para você", Corpo: "{{nome}}, entraram imóveis novos no perfil que você procura. Quer que eu envie?"},
			},
		}, cadenciaReativacao},
		Propriedades: []PropriedadeModelo{
			{Entidade: "deals", Rotulo: "Tipo de imóvel", Tipo: models.FieldSelecao, Grupo: "Imóvel", Opcoes: []string{"Apartamento", "Casa", "Terreno", "Comercial"}},
			{Entidade: "deals", Rotulo: "Faixa de valor", Tipo: models.FieldSelecao, Grupo: "Imóvel", Opcoes: []string{"Até 300 mil", "300 a 600 mil", "600 mil a 1 milhão", "Acima de 1 milhão"}},
			{Entidade: "deals", Rotulo: "Precisa de financiamento", Tipo: models.FieldBooleano, Grupo: "Imóvel"},
			{Entidade: "contacts", Rotulo: "Bairros de interesse", Tipo: models.FieldTexto, Grupo: "Busca"},
		},
		Snippets: snippetsComuns,
	},
	{
		Codigo: "varejo_ecommerce", Nome: "Varejo e e-commerce", Icone: "cart",
		Descricao: "Relacionamento com clientes de loja física ou online: recompra, campanhas e atendimento de pedidos.",
		Segmentos: []string{"Lojas", "E-commerce", "Distribuidores"},
		Destaques: []string{"Funil de recompra e atacado", "Cadência de reativação", "Campos de ticket e canal"},
		Pipelines: []PipelineModelo{funilPadrao("Vendas e recompra", "Interesse", "Carrinho / cotação", "Pedido confirmado", "Entregue")},
		Emails: append([]EmailModelo{
			{Nome: "Pedido confirmado", Assunto: "Recebemos seu pedido, {{nome}}", Corpo: "Olá {{nome}},\n\nSeu pedido foi confirmado e já está em preparação. Avisamos assim que sair para entrega.\n\nObrigado pela preferência!"},
			{Nome: "Novidades da semana", Assunto: "Chegaram novidades, {{nome}}", Corpo: "Olá {{nome}},\n\nSeparei as novidades da semana que combinam com suas últimas compras. Dá uma olhada!"},
		}, emailsComuns[3:]...),
		Cadencias: []CadenciaModelo{cadenciaReativacao},
		Propriedades: []PropriedadeModelo{
			{Entidade: "contacts", Rotulo: "Canal preferido", Tipo: models.FieldSelecao, Grupo: "Compra", Opcoes: []string{"Loja física", "Site", "WhatsApp", "Marketplace"}},
			{Entidade: "contacts", Rotulo: "Última compra", Tipo: models.FieldData, Grupo: "Compra"},
			{Entidade: "deals", Rotulo: "Número do pedido", Tipo: models.FieldTexto, Grupo: "Pedido"},
		},
		Snippets: snippetsComuns,
	},
	{
		Codigo: "agencia_consultoria", Nome: "Agência e consultoria", Icone: "lightbulb",
		Descricao: "Projetos vendidos por proposta: briefing, proposta, aprovação e kickoff, com acompanhamento de retainer.",
		Segmentos: []string{"Agências", "Consultorias", "Estúdios", "Freelancers"},
		Destaques: []string{"Dois funis: projetos e retainers", "E-mails de briefing e aprovação", "Campos de escopo e prazo"},
		Pipelines: []PipelineModelo{
			funilPadrao("Projetos", "Briefing", "Proposta", "Aprovação", "Kickoff"),
			funilPadrao("Retainers", "Interesse", "Diagnóstico", "Proposta mensal"),
		},
		Emails: append([]EmailModelo{
			{Nome: "Pedido de briefing", Assunto: "Vamos alinhar o briefing, {{nome}}?", Corpo: "Olá {{nome}},\n\nPara montar a proposta certa preciso entender melhor o objetivo do projeto. Pode me contar em poucas linhas o que esperam alcançar e até quando?"},
		}, emailsComuns[1:]...),
		Cadencias: []CadenciaModelo{cadenciaB2B},
		Propriedades: []PropriedadeModelo{
			{Entidade: "deals", Rotulo: "Escopo resumido", Tipo: models.FieldTextoLongo, Grupo: "Projeto"},
			{Entidade: "deals", Rotulo: "Prazo desejado", Tipo: models.FieldData, Grupo: "Projeto"},
			{Entidade: "deals", Rotulo: "Tipo de contrato", Tipo: models.FieldSelecao, Grupo: "Projeto", Opcoes: []string{"Projeto fechado", "Retainer mensal", "Por hora"}},
		},
		Snippets: snippetsComuns,
	},
	{
		Codigo: "em_branco", Nome: "Começar do zero", Icone: "sparkles",
		Descricao: "Só o essencial: um funil simples de 4 etapas e os modelos de e-mail básicos. Você monta o resto do seu jeito.",
		Segmentos: []string{"Qualquer segmento"},
		Destaques: []string{"Funil simples", "Modelos básicos de e-mail", "Sem campos extras"},
		Pipelines: []PipelineModelo{funilPadrao("Funil", "Novo", "Em contato", "Proposta")},
		Emails:    emailsComuns[:2],
		Snippets:  snippetsComuns[:1],
	},
}
