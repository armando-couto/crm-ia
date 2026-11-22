package controllers_test

import (
	"net/http"
	stdhttptest "net/http/httptest"
	"testing"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/routes"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// rotasDeProspeccao é o contrato de rotas das seções 6 a 9 de
// docs/PROSPECCAO.md (com o adendo): método, caminho e quem pode chamar.
// perm vazio = basta estar autenticado; "*" = qualquer uma das de leitura de bloqueios.
var rotasDeProspeccao = []struct{ metodo, caminho, perm string }{
	{"GET", "/prospeccao", models.PermProspectingView},
	{"GET", "/prospeccao/catalogos/{nome:string}", models.PermProspectingView},
	{"POST", "/prospeccao/contagem", models.PermProspectingView},
	{"POST", "/prospeccao/previa", models.PermProspectingView},
	{"GET", "/prospeccao/segmentos", models.PermProspectingView},
	{"POST", "/prospeccao/segmentos", models.PermProspectingManage},
	{"PUT", "/prospeccao/segmentos/{id:int64}", models.PermProspectingManage},
	{"DELETE", "/prospeccao/segmentos/{id:int64}", models.PermProspectingManage},
	{"GET", "/prospeccao/listas", models.PermProspectingView},
	{"POST", "/prospeccao/listas", models.PermProspectingManage},
	{"GET", "/prospeccao/listas/{id:int64}", models.PermProspectingView},
	{"PUT", "/prospeccao/listas/{id:int64}", models.PermProspectingManage},
	{"DELETE", "/prospeccao/listas/{id:int64}", models.PermProspectingManage},
	{"GET", "/prospeccao/listas/{id:int64}/itens", models.PermProspectingView},
	{"GET", "/prospeccao/itens/{id:int64}", models.PermProspectingView},
	{"POST", "/prospeccao/listas/{id:int64}/itens/descartar", models.PermProspectingManage},
	{"POST", "/prospeccao/listas/{id:int64}/itens/restaurar", models.PermProspectingManage},
	{"GET", "/prospeccao/listas/{id:int64}/exportar", models.PermProspectingView},
	{"GET", "/prospeccao/jobs/{id:int64}", models.PermProspectingView},
	{"POST", "/prospeccao/jobs/{id:int64}/cancelar", models.PermProspectingManage},
	{"POST", "/prospeccao/listas/{id:int64}/enriquecer/estimativa", models.PermProspectingManage},
	{"POST", "/prospeccao/listas/{id:int64}/enriquecer", models.PermProspectingManage},
	{"POST", "/prospeccao/listas/{id:int64}/enviar/previa", models.PermProspectingManage},
	{"POST", "/prospeccao/listas/{id:int64}/enviar", models.PermProspectingManage},
	{"GET", "/leads", models.PermLeadsView},
	{"POST", "/leads", models.PermLeadsEdit},
	{"GET", "/leads/listas", models.PermLeadsView},
	{"GET", "/leads/{id:int64}", models.PermLeadsView},
	{"PUT", "/leads/{id:int64}", models.PermLeadsEdit},
	{"DELETE", "/leads/{id:int64}", models.PermLeadsAssign},
	{"POST", "/leads/bulk", models.PermLeadsEdit},
	{"POST", "/leads/{id:int64}/converter", models.PermLeadsEdit},
	{"POST", "/leads/{id:int64}/descartar", models.PermLeadsEdit},
	{"POST", "/leads/{id:int64}/reabrir", models.PermLeadsEdit},
	{"POST", "/leads/{id:int64}/resultado", models.PermLeadsEdit},
	{"POST", "/leads/{id:int64}/liberar", models.PermLeadsEdit},
	{"GET", "/leads/fila/resumo", models.PermLeadsEdit},
	{"POST", "/leads/fila/proximo", models.PermLeadsEdit},
	{"GET", "/agenda/disponibilidade", models.PermLeadsEdit},
	{"GET", "/bloqueios", "*"},
	{"POST", "/bloqueios", models.PermProspectingManage},
	{"POST", "/bloqueios/importar", models.PermProspectingManage},
	{"DELETE", "/bloqueios/{id:int64}", models.PermProspectingManage},
	{"GET", "/settings/prospeccao", models.PermLeadsView},
	{"PUT", "/settings/prospeccao", models.PermProspectingManage},
	{"GET", "/settings/motivos-perda", ""},
	{"PUT", "/settings/motivos-perda", models.PermSettingsPipelines},
}

func TestRotasDeProspeccaoRegistradas(t *testing.T) {
	app := iris.New()
	routes.Register(app)
	registradas := map[string]bool{}
	for _, r := range app.GetRoutes() {
		registradas[r.Method+" "+r.Tmpl().Src] = true
	}
	for _, r := range rotasDeProspeccao {
		if !registradas[r.metodo+" /api/v1"+r.caminho] {
			t.Errorf("rota do contrato não registrada: %s /api/v1%s", r.metodo, r.caminho)
		}
	}
}

// caminhoConcreto troca os parâmetros do padrão por valores de exemplo.
func caminhoConcreto(padrao string) string {
	out := ""
	for i := 0; i < len(padrao); i++ {
		if padrao[i] != '{' {
			out += string(padrao[i])
			continue
		}
		fim := i
		for padrao[fim] != '}' {
			fim++
		}
		if padrao[i:fim+1] == "{nome:string}" {
			out += "ufs"
		} else {
			out += "1"
		}
		i = fim
	}
	return "/api/v1" + out
}

// Sem sessão toda rota responde 401; com sessão, 403 exatamente para quem não
// tem a permissão da rota no perfil padrão. O que o handler faz depois de
// passar pela permissão (hoje 501; amanhã a resposta de verdade) não é
// assunto deste teste.
func TestRotasDeProspeccaoExigemPermissao(t *testing.T) {
	e, _, _ := newTestApp(t)
	perfis := map[string]string{
		models.RoleSeller:  tokenFor(t, &models.User{ID: 21, Name: "Vendedora", Email: "vendedora@exemplo.com.br", Role: models.RoleSeller}),
		models.RoleManager: tokenFor(t, &models.User{ID: 22, Name: "Gestor", Email: "gestor@exemplo.com.br", Role: models.RoleManager}),
	}
	padrao := models.DefaultPermissions()

	for _, r := range rotasDeProspeccao {
		caminho := caminhoConcreto(r.caminho)
		e.Request(r.metodo, caminho).Expect().Status(http.StatusUnauthorized)

		for perfil, token := range perfis {
			pode := r.perm == "" || padrao[perfil][r.perm]
			if r.perm == "*" {
				pode = padrao[perfil][models.PermProspectingView] || padrao[perfil][models.PermLeadsView]
			}
			status := e.Request(r.metodo, caminho).WithHeader("Authorization", "Bearer "+token).Expect().Raw().StatusCode
			if pode && (status == http.StatusForbidden || status == http.StatusUnauthorized || status == http.StatusNotFound) {
				t.Errorf("%s %s: %s deveria passar pela permissão, veio %d", r.metodo, caminho, perfil, status)
			}
			if !pode && status != http.StatusForbidden {
				t.Errorf("%s %s: %s deveria receber 403, veio %d", r.metodo, caminho, perfil, status)
			}
		}
	}
}

// "Meu plano" continua um repasse fiel do painel, agora pelo cliente da
// plataforma (services.ChamarPainelBruto).
func TestMeuPlanoRepassaPeloClienteDaPlataforma(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)
	antes := utils.Cfg
	utils.Cfg.TokenInterno = "token-interno-de-teste"
	t.Cleanup(func() { utils.Cfg = antes })

	status, corpo := http.StatusOK, `{"plano":{"nome":"Profissional"},"prospeccao":{"mes":"2026-11","limite":5000,"usado":10,"restante":4990}}`
	var recebido string
	painel := stdhttptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebido = r.Method + " " + r.URL.Path + " " + r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}))
	t.Cleanup(painel.Close)
	t.Cleanup(services.UsarPlataformaDeTeste(painel.URL))

	resp := e.GET("/api/v1/plano").WithHeader("Authorization", "Bearer "+token).Expect().Status(http.StatusOK).JSON().Object()
	resp.Value("plano").Object().Value("nome").IsEqual("Profissional")
	resp.Value("prospeccao").Object().Value("restante").IsEqual(4990)
	if recebido != "GET /plano Bearer token-interno-de-teste" {
		t.Fatalf("chamada ao painel: %q", recebido)
	}

	// Status e corpo do painel são copiados como vieram (ex.: 422 de validação).
	status, corpo = http.StatusUnprocessableEntity, `{"erro":"plano inválido","error":"plano inválido"}`
	e.POST("/api/v1/plano/solicitacoes").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "upgrade", "plano_id": 3}).
		Expect().Status(http.StatusUnprocessableEntity).JSON().Object().Value("error").IsEqual("plano inválido")
	if recebido != "POST /plano/solicitacoes Bearer token-interno-de-teste" {
		t.Fatalf("chamada ao painel: %q", recebido)
	}

	// O painel não reconhece o ambiente: a tela recebe 502, não um 401 que derrubaria a sessão.
	status, corpo = http.StatusUnauthorized, `{"erro":"não autorizado"}`
	e.GET("/api/v1/plano").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusBadGateway).JSON().Object().Value("error").String().Contains("não reconheceu")

	// Painel fora do ar: 502 com mensagem amigável.
	painel.Close()
	e.GET("/api/v1/plano").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusBadGateway).JSON().Object().Value("error").String().Contains("plataforma")

	// Ambiente sem painel: 503 com a mensagem de sempre.
	services.UsarPlataformaDeTeste("")
	utils.Cfg.PainelURL = ""
	e.GET("/api/v1/plano").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(http.StatusServiceUnavailable).JSON().Object().Value("error").String().Contains("gestão do plano")
}
