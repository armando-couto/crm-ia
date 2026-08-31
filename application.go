package main

import (
	"flag"
	"log"
	"time"

	"fixpay/fix-crm/migrations"
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/routes"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/armando-couto/goutils"
	"github.com/kataras/iris/v12"
	_ "github.com/lib/pq"
)

func main() {
	//-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_ Configuração inicial Iris/APP -_-_-_-_-_-_-_-_-_-_-_-_-_-_-_
	app := iris.Default()
	app.Use(iris.Compression)
	app.SetRoutesNoLog(true)

	utils.LoadConfig()
	if utils.JWTSecret == "" {
		log.Fatal("Chave jwt_secret não encontrada!")
	}

	////////////////////////////////////////////////////////////////////////
	// Banco de dados
	utils.DB = goutils.ConnectionBDPostgreSQL("FixCRM", "disable", false)
	utils.DB.SetMaxOpenConns(40)
	utils.DB.SetMaxIdleConns(20)
	utils.DB.SetConnMaxLifetime(5 * time.Minute)

	if err := migrations.Run(utils.DB); err != nil {
		log.Fatalf("Erro ao aplicar migrations: %v", err)
	}

	// Comando de manutenção: ./fix-crm -reset-admin redefine a senha do
	// administrador com admin_email/admin_password do .env e encerra.
	resetAdmin := flag.Bool("reset-admin", false, "redefine a senha do administrador com admin_email/admin_password do .env e sai")
	flag.Parse()
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
	////////////////////////////////////////////////////////////////////////

	////////////////////////////////////////////////////////////////////////
	// E-mail (Mandrill) — opcional em desenvolvimento
	mailer, err := services.NewMandrillMailer()
	if err != nil {
		log.Printf("Aviso: envio de e-mails desabilitado (%v)", err)
	} else {
		services.Mail = mailer
	}
	////////////////////////////////////////////////////////////////////////

	// Automações: retoma as sequências em espera e varre os gatilhos por tempo.
	services.StartAutomationWorker(utils.DB)

	//-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_ Rotas -_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_
	routes.Register(app)

	// Porta padrão: 9000 em produção (Swarm/stack mapeiam 9000) e 6998 em
	// desenvolvimento; port_server no .env sempre tem prioridade.
	port := goutils.Godotenv("port_server")
	if port == "" {
		if goutils.Godotenv("env") == "production" {
			port = "9000"
		} else {
			port = "6998"
		}
	}
	// PostMaxMemory cobre o upload de anexos (limite por arquivo em models).
	app.Listen(":"+port, iris.WithPostMaxMemory(models.MaxAttachmentBytes+(1<<20)))
}
