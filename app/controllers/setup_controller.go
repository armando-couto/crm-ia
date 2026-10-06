package controllers

import (
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// -----------------------------------------------------------------------------
// Assistente de configuração inicial. O administrador escolhe um modelo de
// CRM, dá a identidade da empresa, configura o e-mail e as regras de disparo e
// convida a equipe — tudo em uma tela, no primeiro acesso.
// -----------------------------------------------------------------------------

// SetupStatus diz em que passo o cliente está e o que já foi configurado.
func SetupStatus(ctx iris.Context) {
	ctx.JSON(services.LoadSetupStatus(utils.DB))
}

// SetupTemplates lista o catálogo de modelos.
func SetupTemplates(ctx iris.Context) {
	lista := make([]services.ResumoTemplate, 0, len(services.Catalogo))
	for _, t := range services.Catalogo {
		lista = append(lista, t.Resumo())
	}
	ctx.JSON(lista)
}

// SetupTemplateDetail mostra o conteúdo que um modelo vai criar.
func SetupTemplateDetail(ctx iris.Context) {
	t, ok := services.TemplatePorCodigo(ctx.Params().Get("codigo"))
	if !ok {
		notFound(ctx)
		return
	}
	pipelines := []iris.Map{}
	for _, p := range t.Pipelines {
		etapas := []string{}
		for _, e := range p.Etapas {
			etapas = append(etapas, e.Nome)
		}
		pipelines = append(pipelines, iris.Map{"nome": p.Nome, "etapas": etapas})
	}
	emails := []string{}
	for _, e := range t.Emails {
		emails = append(emails, e.Nome)
	}
	cadencias := []iris.Map{}
	for _, c := range t.Cadencias {
		cadencias = append(cadencias, iris.Map{"nome": c.Nome, "descricao": c.Descricao, "passos": len(c.Passos)})
	}
	campos := []iris.Map{}
	for _, p := range t.Propriedades {
		campos = append(campos, iris.Map{"entidade": p.Entidade, "rotulo": p.Rotulo, "tipo": p.Tipo})
	}
	ctx.JSON(iris.Map{"resumo": t.Resumo(), "pipelines": pipelines, "emails": emails, "cadencias": cadencias, "campos": campos})
}

// SetupApplyTemplate semeia o modelo escolhido no ambiente.
func SetupApplyTemplate(ctx iris.Context) {
	var req struct {
		Template string `json:"template"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.Template == "" {
		badRequest(ctx, "escolha um modelo")
		return
	}
	res, err := services.AplicarTemplate(utils.DB, req.Template, userIDPtr(ctx))
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	audit(ctx, "setup", "configuracao", 0, "modelo de CRM aplicado: "+req.Template)
	ctx.JSON(res)
}

// SetupStep marca um passo como visto (para o assistente lembrar onde parou).
func SetupStep(ctx iris.Context) {
	var req struct {
		Step string `json:"step"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.Step == "" {
		badRequest(ctx, "informe o passo")
		return
	}
	if err := services.MarcarPassoSetup(utils.DB, req.Step, userIDPtr(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(services.LoadSetupStatus(utils.DB))
}

// SetupComplete encerra o assistente.
func SetupComplete(ctx iris.Context) {
	if err := services.ConcluirSetup(utils.DB, userIDPtr(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, "setup", "configuracao", 0, "assistente de configuração concluído")
	ctx.JSON(services.LoadSetupStatus(utils.DB))
}
