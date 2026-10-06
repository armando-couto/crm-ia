package routes

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// -----------------------------------------------------------------------------
// SPA do cliente. O build do Vue é um só para todos os ambientes; na hora de
// servir o index.html, a aplicação injeta a identidade do tenant (slug, nome,
// cor, logo, base path, versão) — é isso que permite uma imagem servir todos
// os clientes, cada um com a própria marca e no próprio endereço.
// -----------------------------------------------------------------------------

const distDir = "./web/dist"

func registerSPA(app *iris.Application) {
	indexBruto, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		log.Printf("Aviso: web/dist/index.html não encontrado (%v) — só a API será servida", err)
		app.OnErrorCode(iris.StatusNotFound, naoEncontrado)
		return
	}
	index := string(indexBruto)

	// Assets com hash no nome: cache longo.
	app.HandleDir("/assets", iris.Dir(filepath.Join(distDir, "assets")), iris.DirOptions{
		Cache: iris.DirCacheOptions{Enable: false},
	})
	app.Get("/favicon.svg", func(ctx iris.Context) { ctx.ServeFile(filepath.Join(distDir, "favicon.svg")) })
	app.Get("/favicon.ico", func(ctx iris.Context) { ctx.StatusCode(http.StatusNoContent) })

	servirIndex := func(ctx iris.Context) {
		ctx.Header("Cache-Control", "no-store, must-revalidate")
		ctx.Header("X-Frame-Options", "DENY")
		ctx.ContentType("text/html; charset=utf-8")
		_, _ = ctx.WriteString(indexComTenant(index))
	}
	app.Get("/", servirIndex)
	app.OnErrorCode(iris.StatusNotFound, func(ctx iris.Context) {
		if strings.HasPrefix(ctx.Path(), "/api/") {
			naoEncontrado(ctx)
			return
		}
		// Arquivo estático que existe no dist (robots, manifest…)?
		limpo := filepath.Clean("/" + ctx.Path())
		if info, err := os.Stat(filepath.Join(distDir, limpo)); err == nil && !info.IsDir() && limpo != "/index.html" {
			ctx.StatusCode(http.StatusOK)
			ctx.ServeFile(filepath.Join(distDir, limpo))
			return
		}
		ctx.StatusCode(http.StatusOK)
		servirIndex(ctx)
	})
}

func naoEncontrado(ctx iris.Context) {
	ctx.JSON(iris.Map{"error": "rota não encontrada"})
}

// indexComTenant preenche os placeholders do index.html. A identidade é lida a
// cada requisição (nome, cor e logo podem mudar em Configurações) com cache de
// alguns segundos para não bater no banco em toda navegação.
func indexComTenant(index string) string {
	t := tenantCacheado()
	b, _ := json.Marshal(t)
	base := utils.Cfg.BasePath + "/"
	r := strings.NewReplacer(
		"__TENANT_BASE__", base,
		"__TENANT_NOME__", htmlEscape(t.Name),
		"__TENANT_SLUG__", t.Slug,
		"__TEMA_COR_PRIMARIA__", t.Color,
		"__TENANT_JSON__", string(b),
	)
	return r.Replace(index)
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

const chaveTenantCache = "tenant:identidade"

func tenantCacheado() services.TenantInfo {
	if raw, ok := utils.Cache.Get(chaveTenantCache); ok {
		var t services.TenantInfo
		if json.Unmarshal([]byte(raw), &t) == nil {
			return t
		}
	}
	t := services.Tenant(utils.DB)
	if b, err := json.Marshal(t); err == nil {
		utils.Cache.Set(chaveTenantCache, string(b), 15*time.Second)
	}
	return t
}

// InvalidarTenantCache é chamado quando a identidade muda em Configurações.
func InvalidarTenantCache() { utils.Cache.Del(chaveTenantCache) }
