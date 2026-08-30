package routes

import (
	"strings"

	"fixpay/fix-crm/controllers"
	"fixpay/fix-crm/middleware"
	"fixpay/fix-crm/models"

	"github.com/kataras/iris/v12"
)

// Register configura todas as rotas da API e o serviço do front (SPA Vue).
func Register(app *iris.Application) {
	api := app.Party("/api/v1")

	// Rotas públicas de autenticação.
	api.Post("/auth/login", controllers.Login)
	api.Post("/auth/forgot", controllers.ForgotPassword)
	api.Post("/auth/reset", controllers.ResetPassword)

	// Rotas autenticadas.
	auth := api.Party("", middleware.JWTAuth)

	auth.Get("/me", controllers.Me)
	auth.Put("/me", controllers.UpdateMe)

	auth.Get("/users", controllers.ListUsers)
	admin := auth.Party("/users", middleware.RequireRoles(models.RoleAdmin))
	admin.Post("", controllers.CreateUser)
	admin.Put("/{id:int64}", controllers.UpdateUserByID)
	admin.Delete("/{id:int64}", controllers.DeactivateUser)

	auth.Get("/contacts", controllers.ListContacts)
	auth.Post("/contacts", controllers.CreateContact)
	auth.Get("/contacts/export", controllers.ExportContacts)
	auth.Post("/contacts/import", controllers.ImportContacts)
	auth.Get("/contacts/{id:int64}", controllers.GetContact)
	auth.Put("/contacts/{id:int64}", controllers.UpdateContact)
	auth.Delete("/contacts/{id:int64}", controllers.DeleteContact)

	auth.Get("/companies", controllers.ListCompanies)
	auth.Post("/companies", controllers.CreateCompany)
	auth.Get("/companies/{id:int64}", controllers.GetCompany)
	auth.Put("/companies/{id:int64}", controllers.UpdateCompany)
	auth.Delete("/companies/{id:int64}", controllers.DeleteCompany)

	auth.Get("/pipelines", controllers.ListPipelines)
	gestao := auth.Party("", middleware.RequireRoles(models.RoleGestor))
	gestao.Post("/pipelines", controllers.CreatePipeline)
	gestao.Put("/pipelines/{id:int64}", controllers.UpdatePipeline)
	gestao.Delete("/pipelines/{id:int64}", controllers.DeletePipeline)
	gestao.Post("/pipelines/{id:int64}/stages", controllers.CreateStage)
	gestao.Put("/stages/{id:int64}", controllers.UpdateStage)
	gestao.Delete("/stages/{id:int64}", controllers.DeleteStage)

	auth.Get("/deals", controllers.ListDeals)
	auth.Get("/deals/board", controllers.DealsBoard)
	auth.Post("/deals", controllers.CreateDeal)
	auth.Get("/deals/{id:int64}", controllers.GetDeal)
	auth.Put("/deals/{id:int64}", controllers.UpdateDeal)
	auth.Patch("/deals/{id:int64}/stage", controllers.MoveDeal)
	auth.Patch("/deals/{id:int64}/close", controllers.CloseDealHandler)
	auth.Delete("/deals/{id:int64}", controllers.DeleteDeal)

	auth.Get("/tasks", controllers.ListTasks)
	auth.Post("/tasks", controllers.CreateTask)
	auth.Put("/tasks/{id:int64}", controllers.UpdateTask)
	auth.Patch("/tasks/{id:int64}/toggle", controllers.ToggleTask)
	auth.Delete("/tasks/{id:int64}", controllers.DeleteTask)

	auth.Get("/activities", controllers.ListActivities)
	auth.Post("/activities", controllers.CreateActivityHandler)
	auth.Post("/emails", controllers.SendEmail)

	auth.Get("/dashboard", controllers.Dashboard)
	auth.Get("/search", controllers.Search)

	// Healthcheck para o orquestrador.
	app.Get("/health", func(ctx iris.Context) {
		ctx.JSON(iris.Map{"status": "ok"})
	})

	registerSPA(app)
}

// registerSPA serve o build do Vue (web/dist) com fallback para o index.html
// (SPA: rotas do front resolvidas no cliente). A compressão fica por conta do
// middleware global (iris.Compression); Compress aqui causaria dupla codificação.
func registerSPA(app *iris.Application) {
	app.HandleDir("/", iris.Dir("./web/dist"), iris.DirOptions{
		IndexName: "index.html",
		SPA:       true,
	})

	app.OnErrorCode(iris.StatusNotFound, func(ctx iris.Context) {
		if strings.HasPrefix(ctx.Path(), "/api/") {
			ctx.JSON(iris.Map{"error": "rota não encontrada"})
			return
		}
		ctx.WriteString("página não encontrada")
	})
}
