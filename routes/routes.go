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
	auth.Get("/me/notifications", controllers.GetMyNotifications)
	auth.Put("/me/notifications", controllers.UpdateMyNotifications)

	auth.Get("/users", controllers.ListUsers)
	admin := auth.Party("/users", middleware.RequireRoles(models.RoleAdmin))
	admin.Post("", controllers.CreateUser)
	admin.Put("/{id:int64}", controllers.UpdateUserByID)
	admin.Delete("/{id:int64}", controllers.DeactivateUser)

	auth.Get("/teams", controllers.ListTeams)
	adminTeams := auth.Party("/teams", middleware.RequireRoles(models.RoleAdmin))
	adminTeams.Post("", controllers.CreateTeam)
	adminTeams.Put("/{id:int64}", controllers.UpdateTeam)
	adminTeams.Delete("/{id:int64}", controllers.DeleteTeam)

	auth.Get("/contacts", controllers.ListContacts)
	auth.Post("/contacts", controllers.CreateContact)
	auth.Get("/contacts/stats", controllers.ContactStatsHandler)
	auth.Post("/contacts/bulk", controllers.BulkContacts)
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
	gestao.Post("/pipelines/{id:int64}/stages/reorder", controllers.ReorderStages)
	gestao.Put("/stages/{id:int64}", controllers.UpdateStage)
	gestao.Delete("/stages/{id:int64}", controllers.DeleteStage)

	auth.Get("/deals", controllers.ListDeals)
	auth.Get("/deals/board", controllers.DealsBoard)
	auth.Get("/deals/export", controllers.ExportDeals)
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

	auth.Get("/tickets", controllers.ListTickets)
	auth.Post("/tickets", controllers.CreateTicket)
	auth.Get("/tickets/{id:int64}", controllers.GetTicket)
	auth.Put("/tickets/{id:int64}", controllers.UpdateTicket)
	auth.Delete("/tickets/{id:int64}", controllers.DeleteTicket)

	auth.Get("/lists", controllers.ListContactLists)
	auth.Post("/lists", controllers.CreateContactList)
	auth.Get("/lists/{id:int64}", controllers.GetContactList)
	auth.Put("/lists/{id:int64}", controllers.UpdateContactList)
	auth.Delete("/lists/{id:int64}", controllers.DeleteContactList)
	auth.Get("/lists/{id:int64}/contacts", controllers.ListMembers)
	auth.Post("/lists/{id:int64}/contacts", controllers.AddMember)
	auth.Delete("/lists/{id:int64}/contacts/{contactId:int64}", controllers.RemoveMember)

	auth.Get("/projects", controllers.ListProjects)
	auth.Post("/projects", controllers.CreateProject)
	auth.Get("/projects/{id:int64}", controllers.GetProject)
	auth.Put("/projects/{id:int64}", controllers.UpdateProject)
	auth.Delete("/projects/{id:int64}", controllers.DeleteProject)

	auth.Get("/conversations", controllers.ListConversations)
	auth.Get("/conversations/{id:int64}", controllers.GetConversation)
	auth.Post("/conversations/{id:int64}/reply", controllers.ReplyConversation)
	auth.Patch("/conversations/{id:int64}/status", controllers.SetConversationStatus)

	auth.Get("/calls", controllers.ListCalls)
	auth.Post("/calls", controllers.CreateCall)
	auth.Delete("/calls/{id:int64}", controllers.DeleteCall)

	auth.Get("/meetings", controllers.ListMeetings)
	auth.Post("/meetings", controllers.CreateMeeting)
	auth.Put("/meetings/{id:int64}", controllers.UpdateMeeting)
	auth.Delete("/meetings/{id:int64}", controllers.DeleteMeeting)

	auth.Get("/playbooks", controllers.ListPlaybooks)
	auth.Post("/playbooks", controllers.CreatePlaybook)
	auth.Put("/playbooks/{id:int64}", controllers.UpdatePlaybook)
	auth.Delete("/playbooks/{id:int64}", controllers.DeletePlaybook)

	auth.Get("/templates", controllers.ListMessageTemplates)
	auth.Post("/templates", controllers.CreateMessageTemplate)
	auth.Put("/templates/{id:int64}", controllers.UpdateMessageTemplate)
	auth.Delete("/templates/{id:int64}", controllers.DeleteMessageTemplate)
	auth.Get("/templates/{id:int64}/render", controllers.RenderMessageTemplate)

	auth.Get("/snippets", controllers.ListSnippets)
	auth.Post("/snippets", controllers.CreateSnippet)
	auth.Put("/snippets/{id:int64}", controllers.UpdateSnippet)
	auth.Delete("/snippets/{id:int64}", controllers.DeleteSnippet)

	auth.Get("/settings/contact-form", controllers.GetContactForm)
	gestao.Put("/settings/contact-form", controllers.UpdateContactForm)
	auth.Get("/settings/deal-form", controllers.GetDealForm)
	gestao.Put("/settings/deal-form", controllers.UpdateDealForm)

	auth.Get("/views", controllers.ListViews)
	auth.Post("/views", controllers.CreateView)
	auth.Put("/views/{id:int64}", controllers.RenameView)
	auth.Delete("/views/{id:int64}", controllers.DeleteView)

	auth.Get("/dashboard", controllers.Dashboard)
	auth.Get("/search", controllers.Search)

	// Webhook público do Mandrill (e-mails de entrada da caixa de entrada).
	app.Post("/api/webhooks/mandrill/inbound", controllers.MandrillInboundWebhook)
	app.Head("/api/webhooks/mandrill/inbound", func(ctx iris.Context) { ctx.StatusCode(iris.StatusOK) })

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
