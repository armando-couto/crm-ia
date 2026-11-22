package controllers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/middleware"
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// -----------------------------------------------------------------------------
// Rotas internas: só o painel da plataforma as alcança, pela rede do Docker,
// com o TOKEN_INTERNO do ambiente. Nenhuma delas tem rota pública no Traefik.
// -----------------------------------------------------------------------------

// RequireInternalToken barra quem não apresenta o token do ambiente.
func RequireInternalToken(ctx iris.Context) {
	tok, ok := strings.CutPrefix(ctx.GetHeader("Authorization"), "Bearer ")
	if utils.Cfg.TokenInterno == "" || !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(utils.Cfg.TokenInterno)) != 1 {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"erro": "não autorizado"})
		return
	}
	ctx.Next()
}

// InternoUso: o que o painel mostra na ficha do cliente (uso da licença).
func InternoUso(ctx iris.Context) {
	ativos, _ := models.CountActiveUsers(utils.DB)
	var contatos, negocios int
	_ = utils.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM contacts), (SELECT COUNT(*) FROM deals)`).Scan(&contatos, &negocios)
	st := models.LoadSetupState(utils.DB)
	ctx.JSON(iris.Map{
		"slug": utils.Cfg.TenantSlug, "versao": utils.Cfg.Versao,
		"usuarios_ativos": ativos, "usuarios_max": utils.Cfg.UsuariosMax,
		"contatos": contatos, "negocios": negocios,
		"setup_concluido": st.Completed, "modelo": st.Template,
		"cache": utils.Cache.Nome(),
	})
}

// InternoDesempenho: agregados do período para a visão de rede do painel.
// Só números — nenhum dado de contato sai do ambiente do cliente.
func InternoDesempenho(ctx iris.Context) {
	de, ate := periodoDe(ctx)
	var (
		ganhos, perdidos, criados, contatosNovos, emails int
		valorGanho                                       float64
	)
	_ = utils.DB.QueryRow(`
		SELECT
		  (SELECT COUNT(*) FROM deals WHERE status = 'ganho' AND closed_at >= $1 AND closed_at < $2),
		  (SELECT COALESCE(SUM(amount),0) FROM deals WHERE status = 'ganho' AND closed_at >= $1 AND closed_at < $2),
		  (SELECT COUNT(*) FROM deals WHERE status = 'perdido' AND closed_at >= $1 AND closed_at < $2),
		  (SELECT COUNT(*) FROM deals WHERE created_at >= $1 AND created_at < $2),
		  (SELECT COUNT(*) FROM contacts WHERE created_at >= $1 AND created_at < $2),
		  (SELECT COUNT(*) FROM email_messages WHERE sent_at >= $1 AND sent_at < $2)`,
		de, ate).Scan(&ganhos, &valorGanho, &perdidos, &criados, &contatosNovos, &emails)
	taxa := 0.0
	if ganhos+perdidos > 0 {
		taxa = float64(ganhos) / float64(ganhos+perdidos) * 100
	}
	ticket := 0.0
	if ganhos > 0 {
		ticket = valorGanho / float64(ganhos)
	}
	ctx.JSON(iris.Map{
		"slug": utils.Cfg.TenantSlug, "nome": utils.Cfg.TenantNome,
		"negocios_ganhos": ganhos, "negocios_perdidos": perdidos, "negocios_criados": criados,
		"valor_ganho_centavos": int64(valorGanho*100 + 0.5), "ticket_medio_centavos": int64(ticket*100 + 0.5),
		"taxa_conversao_pct": taxa, "contatos_novos": contatosNovos, "emails_enviados": emails,
	})
}

func periodoDe(ctx iris.Context) (time.Time, time.Time) {
	agora := time.Now()
	de := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, agora.Location())
	ate := de.AddDate(0, 1, 0)
	if v := ctx.URLParam("de"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			de = t
		}
	}
	if v := ctx.URLParam("ate"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			ate = t
		}
	}
	return de, ate
}

// InternoRedefinirSenhaAdmin gera uma senha provisória para o administrador
// que perdeu o acesso — só pela rota interna e só para perfil admin.
func InternoRedefinirSenhaAdmin(ctx iris.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StopWithJSON(iris.StatusBadRequest, iris.Map{"erro": "corpo inválido"})
		return
	}
	user, err := models.UserByEmail(utils.DB, models.NormalizeEmail(req.Email))
	if err != nil {
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"erro": "não existe usuário com este e-mail"})
		return
	}
	if user.Role != models.RoleAdmin {
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"erro": "o usuário não é administrador deste ambiente"})
		return
	}
	senha, err := services.RandomToken()
	if err != nil {
		serverError(ctx, err)
		return
	}
	senha = senha[:12]
	hash, err := services.HashPassword(senha)
	if err != nil {
		serverError(ctx, err)
		return
	}
	expira := time.Now().Add(InviteTTL)
	if err := models.ResetUserInvite(utils.DB, user.ID, hash, expira); err != nil {
		serverError(ctx, err)
		return
	}
	if !user.Active {
		user.Active = true
		_ = models.UpdateUser(utils.DB, user)
	}
	middleware.InvalidateUser(user.ID)
	_ = models.RecordAudit(utils.DB, &models.AuditEntry{UserName: "plataforma", Action: models.AuditUpdate, Entity: "usuario",
		EntityID: &user.ID, Summary: "senha provisória do administrador gerada pela plataforma", IP: clientIP(ctx)})
	ctx.JSON(iris.Map{"email": user.Email, "senha": senha})
}

// -----------------------------------------------------------------------------
// Meu plano: o app repassa ao painel (rede interna, TOKEN_INTERNO). O cliente
// vê plano, faturas e o link para atualizar o cartão — nunca quem processa.
// -----------------------------------------------------------------------------

// repassarAoPainel copia status e corpo da resposta do painel para a tela. O
// transporte é o cliente da plataforma (services.ChamarPainelBruto), o mesmo
// que os jobs usam fora dos controllers.
func repassarAoPainel(ctx iris.Context, metodo, caminho string, corpo any) {
	resp, err := services.ChamarPainelBruto(ctx.Request().Context(), metodo, caminho, nil, corpo)
	switch {
	case errors.Is(err, services.ErrPlataformaNaoConfigurada):
		ctx.StopWithJSON(iris.StatusServiceUnavailable, iris.Map{"error": "a gestão do plano ainda não está disponível neste ambiente"})
		return
	case errors.Is(err, services.ErrPlataformaInacessivel):
		ctx.StopWithJSON(iris.StatusBadGateway, iris.Map{"error": services.ErrPlataformaInacessivel.Error()})
		return
	case err != nil:
		serverError(ctx, err)
		return
	}
	if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
		ctx.StopWithJSON(iris.StatusBadGateway, iris.Map{"error": "a plataforma não reconheceu este ambiente"})
		return
	}
	ctx.StatusCode(resp.Status)
	ctx.ContentType("application/json; charset=utf-8")
	_, _ = ctx.Write(resp.Corpo)
}

// MeuPlano devolve plano, faturas, planos disponíveis e o link de pagamento.
func MeuPlano(ctx iris.Context) { repassarAoPainel(ctx, http.MethodGet, "/plano", nil) }

// SolicitarPlano registra pedido de upgrade/cancelamento.
func SolicitarPlano(ctx iris.Context) {
	var in struct {
		Tipo     string `json:"tipo"`
		PlanoID  *int64 `json:"plano_id"`
		Mensagem string `json:"mensagem"`
	}
	if err := ctx.ReadJSON(&in); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	repassarAoPainel(ctx, http.MethodPost, "/plano/solicitacoes", in)
}
