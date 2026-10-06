// CRM IA — aplicação do ambiente de um cliente.
//
// Uma imagem, muitos clientes: cada empresa roda a sua cópia desta aplicação,
// com PostgreSQL e Redis próprios, em https://<dominio>/<slug>. A identidade do
// ambiente (slug, nome, tema, segredos, licença) chega por variáveis de
// ambiente, injetadas pelo provisionador da plataforma.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/armando-couto/crm-ia/app/migrations"
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/routes"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
	_ "github.com/lib/pq"
)

// versaoBuild é injetada no build (-ldflags "-X main.versaoBuild=1.2.0").
var versaoBuild = "dev"

func main() {
	resetAdmin := flag.Bool("reset-admin", false, "redefine a senha do administrador com ADMIN_EMAIL/ADMIN_SENHA e sai")
	healthcheck := flag.Bool("healthcheck", false, "consulta /health e sai (usado pelo Docker)")
	flag.Parse()

	utils.LoadConfig()
	if utils.Cfg.Versao == "dev" && versaoBuild != "dev" {
		utils.Cfg.Versao = versaoBuild
	}

	if *healthcheck {
		os.Exit(checarSaude())
	}
	if utils.JWTSecret == "" {
		log.Fatal("JWT_SECRET não configurado")
	}

	//-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_ Banco de dados -_-_-_-_-_-_-_-_-_-_-_-_-_-_-_
	db, err := utils.ConectarBanco(utils.Cfg.DSN())
	if err != nil {
		log.Fatalf("Banco de dados: %v", err)
	}
	utils.DB = db
	if err := migrations.Run(utils.DB); err != nil {
		log.Fatalf("Erro ao aplicar migrations: %v", err)
	}

	if *resetAdmin {
		if err := services.ResetAdmin(utils.DB); err != nil {
			log.Fatalf("Erro ao redefinir o administrador: %v", err)
		}
		return
	}
	if err := services.SeedAdmin(utils.DB); err != nil {
		log.Fatalf("Erro ao criar administrador inicial: %v", err)
	}
	if _, err := models.LoadPermissions(utils.DB); err != nil {
		log.Fatalf("Erro ao carregar as permissões dos perfis: %v", err)
	}

	//-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_ Redis (cache) -_-_-_-_-_-_-_-_-_-_-_-_-_-_-_
	if utils.Cfg.RedisURL != "" {
		if err := utils.ConectarRedis(utils.Cfg.RedisURL); err != nil {
			log.Printf("Aviso: Redis indisponível (%v); seguindo com cache em memória", err)
		} else {
			log.Printf("Redis conectado (cache do ambiente)")
		}
	}

	//-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_ E-mail -_-_-_-_-_-_-_-_-_-_-_-_-_-_-_
	if descricao, err := services.ConfigurarRemetente(models.LoadEmailSettings(utils.DB)); err != nil {
		log.Printf("Aviso: envio de e-mails desabilitado (%v)", err)
	} else {
		log.Printf("E-mail: %s", descricao)
	}

	// Automações: retoma as sequências em espera e varre os gatilhos por tempo.
	services.StartAutomationWorker(utils.DB)

	//-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_ HTTP -_-_-_-_-_-_-_-_-_-_-_-_-_-_-_
	app := iris.New()
	app.Use(iris.Compression)
	app.SetRoutesNoLog(true)
	routes.Register(app)

	log.Printf("CRM IA %s · ambiente %q em %s (porta %s)", utils.Cfg.Versao, utils.Cfg.TenantSlug, utils.Cfg.AppURL, utils.Cfg.Porta)
	app.Listen(":"+utils.Cfg.Porta,
		iris.WithPostMaxMemory(models.MaxAttachmentBytes+(1<<20)),
		iris.WithoutStartupLog,
	)
}

// checarSaude é o healthcheck do container: 0 se a API responde.
func checarSaude() int {
	cli := &http.Client{Timeout: 4 * time.Second}
	resp, err := cli.Get(fmt.Sprintf("http://127.0.0.1:%s/health", utils.Cfg.Porta))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "status", resp.StatusCode)
		return 1
	}
	return 0
}
