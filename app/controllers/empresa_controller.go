package controllers

import (
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/middleware"
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// -----------------------------------------------------------------------------
// Identidade do ambiente, e-mail e disparo — as configurações que o cliente
// faz no setup e revisita em Configurações.
// -----------------------------------------------------------------------------

// Workspace (público para quem está logado) devolve nome, cor, logo e versão
// do ambiente — é o que o shell do app usa para se vestir com a marca do cliente.
func Workspace(ctx iris.Context) {
	ctx.JSON(services.Tenant(utils.DB))
}

// GetWorkspaceSettings devolve a identidade editável da empresa.
func GetWorkspaceSettings(ctx iris.Context) {
	c := utils.Cfg
	ctx.JSON(models.LoadWorkspaceSettings(utils.DB, c.TenantNome, c.CorPrimaria, c.LogoURL))
}

// UpdateWorkspaceSettings grava nome, segmento, cor e logo.
func UpdateWorkspaceSettings(ctx iris.Context) {
	var w models.WorkspaceSettings
	if err := ctx.ReadJSON(&w); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if err := w.Validate(); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	if w.Color == "" {
		w.Color = utils.Cfg.CorPrimaria
	}
	uid := userIDPtr(ctx)
	if err := models.SetSetting(utils.DB, models.SettingWorkspace, w, uid); err != nil {
		serverError(ctx, err)
		return
	}
	_ = services.MarcarPassoSetup(utils.DB, "empresa", uid)
	audit(ctx, models.AuditUpdate, "configuracao", 0, "identidade da empresa atualizada")
	ctx.JSON(w)
}

// GetEmailSettings devolve o provedor configurado (sem o segredo) e os presets.
func GetEmailSettings(ctx iris.Context) {
	e := models.LoadEmailSettings(utils.DB)
	var pub *models.EmailSettings
	if e != nil {
		p := e.Publica()
		pub = &p
	}
	presets := map[string]iris.Map{}
	for k, p := range models.EmailPresets {
		presets[k] = iris.Map{"nome": p.Nome, "host": p.Host, "port": p.Port, "dica": p.Dica}
	}
	ctx.JSON(iris.Map{
		"settings":         pub,
		"presets":          presets,
		"platform_default": utils.Cfg.MandrillChave != "" || utils.Cfg.SMTPHost != "",
		"active":           services.Remetente() != nil,
		"inbound_webhook":  utils.Cfg.MandrillWebhookURL,
	})
}

type emailSettingsRequest struct {
	models.EmailSettings
	// Secret é a chave de API ou a senha SMTP; vazio mantém a já gravada.
	Secret string `json:"secret"`
}

// UpdateEmailSettings grava o provedor e já o coloca em uso.
func UpdateEmailSettings(ctx iris.Context) {
	var req emailSettingsRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	cfg := req.EmailSettings
	if err := cfg.Validate(); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	atual := models.LoadEmailSettings(utils.DB)
	if strings.TrimSpace(req.Secret) != "" {
		cifrado, err := utils.Cifrar(strings.TrimSpace(req.Secret))
		if err != nil {
			serverError(ctx, err)
			return
		}
		cfg.SecretCifrado = cifrado
	} else if atual != nil && atual.Provider == cfg.Provider {
		cfg.SecretCifrado = atual.SecretCifrado
	}
	if cfg.Provider != models.EmailProviderPlatform && cfg.SecretCifrado == "" {
		badRequest(ctx, "informe a chave de API ou a senha do provedor")
		return
	}
	descricao, err := services.ConfigurarRemetente(&cfg)
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	cfg.TestedAt, cfg.TestResult = nil, ""
	uid := userIDPtr(ctx)
	if err := models.SetSetting(utils.DB, models.SettingEmail, cfg, uid); err != nil {
		serverError(ctx, err)
		return
	}
	_ = services.MarcarPassoSetup(utils.DB, "email", uid)
	audit(ctx, models.AuditUpdate, "configuracao", 0, "provedor de e-mail: "+descricao)
	ctx.JSON(iris.Map{"settings": cfg.Publica(), "description": descricao})
}

// TestEmailSettings envia uma mensagem de teste para o usuário logado com o
// provedor ATIVO e registra o resultado.
func TestEmailSettings(ctx iris.Context) {
	user := middleware.CurrentUser(ctx)
	mm := services.Remetente()
	if mm == nil {
		badRequest(ctx, "nenhum provedor de e-mail ativo")
		return
	}
	var req struct {
		To string `json:"to"`
	}
	_ = ctx.ReadJSON(&req)
	destino := models.NormalizeEmail(req.To)
	if destino == "" && user != nil {
		destino = user.Email
	}
	if destino == "" {
		badRequest(ctx, "informe o destinatário do teste")
		return
	}
	descricao := "configuração do ambiente"
	cfg := models.LoadEmailSettings(utils.DB)
	if cfg != nil && cfg.Provider != models.EmailProviderPlatform {
		descricao = models.EmailPresets[cfg.Provider].Nome
	}
	subject, html := services.TestEmail(descricao)
	err := mm.Send(destino, "", subject, html)
	agora := time.Now()
	if cfg != nil {
		cfg.TestedAt = &agora
		if err != nil {
			cfg.TestResult = "falhou: " + err.Error()
		} else {
			cfg.TestResult = "ok para " + destino
		}
		_ = models.SetSetting(utils.DB, models.SettingEmail, cfg, userIDPtr(ctx))
	}
	if err != nil {
		ctx.StopWithJSON(iris.StatusBadGateway, iris.Map{"error": "o envio falhou: " + err.Error()})
		return
	}
	ctx.JSON(iris.Map{"ok": true, "to": destino, "sent_at": agora})
}

// GetSendingSettings devolve as regras de disparo e o resumo do dia.
func GetSendingSettings(ctx iris.Context) {
	provedor := "nenhum"
	if e := models.LoadEmailSettings(utils.DB); e != nil && e.Provider != models.EmailProviderPlatform {
		provedor = models.EmailPresets[e.Provider].Nome
	} else if services.Remetente() != nil {
		provedor = "padrão da plataforma"
	}
	ctx.JSON(iris.Map{"settings": models.LoadSendingSettings(utils.DB), "summary": services.ResumoDoDisparo(utils.DB, provedor)})
}

// UpdateSendingSettings grava janela, teto e assinatura dos disparos.
func UpdateSendingSettings(ctx iris.Context) {
	var s models.SendingSettings
	if err := ctx.ReadJSON(&s); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if err := s.Validate(); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	uid := userIDPtr(ctx)
	if err := models.SetSetting(utils.DB, models.SettingSending, s, uid); err != nil {
		serverError(ctx, err)
		return
	}
	_ = services.MarcarPassoSetup(utils.DB, "disparo", uid)
	audit(ctx, models.AuditUpdate, "configuracao", 0, "regras de disparo atualizadas")
	ctx.JSON(s)
}

func userIDPtr(ctx iris.Context) *int64 {
	if claims := middlewareClaims(ctx); claims != nil {
		return &claims.UserID
	}
	return nil
}
