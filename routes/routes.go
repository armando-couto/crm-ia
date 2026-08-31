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

	// can encurta a proteção por permissão nas rotas abaixo.
	can := middleware.RequirePermission

	auth.Get("/me", controllers.Me)
	auth.Put("/me", controllers.UpdateMe)
	auth.Get("/me/notifications", controllers.GetMyNotifications)
	auth.Put("/me/notifications", controllers.UpdateMyNotifications)
	auth.Get("/me/permissions", controllers.MyPermissions)

	// Usuários e equipes: visíveis a todos (para atribuir donos), gestão restrita.
	auth.Get("/users", controllers.ListUsers)
	auth.Post("/users", can(models.PermSettingsUsers), controllers.CreateUser)
	auth.Put("/users/{id:int64}", can(models.PermSettingsUsers), controllers.UpdateUserByID)
	auth.Delete("/users/{id:int64}", can(models.PermSettingsUsers), controllers.DeactivateUser)

	auth.Get("/teams", controllers.ListTeams)
	auth.Post("/teams", can(models.PermSettingsUsers), controllers.CreateTeam)
	auth.Put("/teams/{id:int64}", can(models.PermSettingsUsers), controllers.UpdateTeam)
	auth.Delete("/teams/{id:int64}", can(models.PermSettingsUsers), controllers.DeleteTeam)

	auth.Get("/contacts", can(models.PermContactsView), controllers.ListContacts)
	auth.Post("/contacts", can(models.PermContactsEdit), controllers.CreateContact)
	auth.Get("/contacts/stats", can(models.PermContactsView), controllers.ContactStatsHandler)
	auth.Post("/contacts/bulk", can(models.PermContactsEdit), controllers.BulkContacts)
	auth.Get("/contacts/export", can(models.PermContactsExport), controllers.ExportContacts)
	auth.Post("/contacts/import", can(models.PermContactsImport), controllers.ImportContacts)
	auth.Get("/contacts/{id:int64}", can(models.PermContactsView), controllers.GetContact)
	auth.Put("/contacts/{id:int64}", can(models.PermContactsEdit), controllers.UpdateContact)
	auth.Delete("/contacts/{id:int64}", can(models.PermContactsDelete), controllers.DeleteContact)

	auth.Get("/companies", can(models.PermCompaniesView), controllers.ListCompanies)
	auth.Post("/companies", can(models.PermCompaniesEdit), controllers.CreateCompany)
	auth.Get("/companies/{id:int64}", can(models.PermCompaniesView), controllers.GetCompany)
	auth.Put("/companies/{id:int64}", can(models.PermCompaniesEdit), controllers.UpdateCompany)
	auth.Delete("/companies/{id:int64}", can(models.PermCompaniesDelete), controllers.DeleteCompany)

	auth.Get("/pipelines", controllers.ListPipelines)
	pipelines := auth.Party("", can(models.PermSettingsPipelines))
	pipelines.Post("/pipelines", controllers.CreatePipeline)
	pipelines.Put("/pipelines/{id:int64}", controllers.UpdatePipeline)
	pipelines.Delete("/pipelines/{id:int64}", controllers.DeletePipeline)
	pipelines.Post("/pipelines/{id:int64}/stages", controllers.CreateStage)
	pipelines.Post("/pipelines/{id:int64}/stages/reorder", controllers.ReorderStages)
	pipelines.Put("/stages/{id:int64}", controllers.UpdateStage)
	pipelines.Delete("/stages/{id:int64}", controllers.DeleteStage)

	auth.Get("/deals", can(models.PermDealsView), controllers.ListDeals)
	auth.Get("/deals/board", can(models.PermDealsView), controllers.DealsBoard)
	auth.Get("/deals/export", can(models.PermDealsExport), controllers.ExportDeals)
	auth.Post("/deals", can(models.PermDealsEdit), controllers.CreateDeal)
	auth.Get("/deals/{id:int64}", can(models.PermDealsView), controllers.GetDeal)
	auth.Put("/deals/{id:int64}", can(models.PermDealsEdit), controllers.UpdateDeal)
	auth.Patch("/deals/{id:int64}/stage", can(models.PermDealsEdit), controllers.MoveDeal)
	auth.Patch("/deals/{id:int64}/close", can(models.PermDealsEdit), controllers.CloseDealHandler)
	auth.Delete("/deals/{id:int64}", can(models.PermDealsDelete), controllers.DeleteDeal)

	auth.Get("/tasks", can(models.PermTasksView), controllers.ListTasks)
	auth.Post("/tasks", can(models.PermTasksEdit), controllers.CreateTask)
	auth.Put("/tasks/{id:int64}", can(models.PermTasksEdit), controllers.UpdateTask)
	auth.Patch("/tasks/{id:int64}/toggle", can(models.PermTasksEdit), controllers.ToggleTask)
	auth.Delete("/tasks/{id:int64}", can(models.PermTasksEdit), controllers.DeleteTask)

	auth.Get("/activities", controllers.ListActivities)
	auth.Post("/activities", controllers.CreateActivityHandler)
	auth.Post("/emails", can(models.PermEmailSend), controllers.SendEmail)

	auth.Get("/tickets", can(models.PermTicketsView), controllers.ListTickets)
	auth.Post("/tickets", can(models.PermTicketsEdit), controllers.CreateTicket)
	auth.Get("/tickets/{id:int64}", can(models.PermTicketsView), controllers.GetTicket)
	auth.Put("/tickets/{id:int64}", can(models.PermTicketsEdit), controllers.UpdateTicket)
	auth.Delete("/tickets/{id:int64}", can(models.PermTicketsDelete), controllers.DeleteTicket)

	auth.Get("/lists", can(models.PermListsView), controllers.ListContactLists)
	auth.Post("/lists", can(models.PermListsManage), controllers.CreateContactList)
	auth.Get("/lists/{id:int64}", can(models.PermListsView), controllers.GetContactList)
	auth.Put("/lists/{id:int64}", can(models.PermListsManage), controllers.UpdateContactList)
	auth.Delete("/lists/{id:int64}", can(models.PermListsManage), controllers.DeleteContactList)
	auth.Get("/lists/{id:int64}/contacts", can(models.PermListsView), controllers.ListMembers)
	auth.Post("/lists/{id:int64}/contacts", can(models.PermListsManage), controllers.AddMember)
	auth.Delete("/lists/{id:int64}/contacts/{contactId:int64}", can(models.PermListsManage), controllers.RemoveMember)

	auth.Get("/projects", can(models.PermProjectsView), controllers.ListProjects)
	auth.Post("/projects", can(models.PermProjectsEdit), controllers.CreateProject)
	auth.Get("/projects/{id:int64}", can(models.PermProjectsView), controllers.GetProject)
	auth.Put("/projects/{id:int64}", can(models.PermProjectsEdit), controllers.UpdateProject)
	auth.Delete("/projects/{id:int64}", can(models.PermProjectsEdit), controllers.DeleteProject)

	auth.Get("/conversations", can(models.PermInboxView), controllers.ListConversations)
	auth.Get("/conversations/{id:int64}", can(models.PermInboxView), controllers.GetConversation)
	auth.Post("/conversations/{id:int64}/reply", can(models.PermInboxReply), controllers.ReplyConversation)
	auth.Patch("/conversations/{id:int64}/status", can(models.PermInboxReply), controllers.SetConversationStatus)

	auth.Get("/calls", can(models.PermCallsView), controllers.ListCalls)
	auth.Post("/calls", can(models.PermCallsLog), controllers.CreateCall)
	auth.Delete("/calls/{id:int64}", can(models.PermCallsLog), controllers.DeleteCall)

	auth.Get("/meetings", can(models.PermMeetingsView), controllers.ListMeetings)
	auth.Post("/meetings", can(models.PermMeetingsManage), controllers.CreateMeeting)
	auth.Put("/meetings/{id:int64}", can(models.PermMeetingsManage), controllers.UpdateMeeting)
	auth.Delete("/meetings/{id:int64}", can(models.PermMeetingsManage), controllers.DeleteMeeting)

	auth.Get("/playbooks", can(models.PermLibraryView), controllers.ListPlaybooks)
	auth.Post("/playbooks", can(models.PermLibraryManage), controllers.CreatePlaybook)
	auth.Put("/playbooks/{id:int64}", can(models.PermLibraryManage), controllers.UpdatePlaybook)
	auth.Delete("/playbooks/{id:int64}", can(models.PermLibraryManage), controllers.DeletePlaybook)

	auth.Get("/templates", can(models.PermLibraryView), controllers.ListMessageTemplates)
	auth.Post("/templates", can(models.PermLibraryManage), controllers.CreateMessageTemplate)
	auth.Put("/templates/{id:int64}", can(models.PermLibraryManage), controllers.UpdateMessageTemplate)
	auth.Delete("/templates/{id:int64}", can(models.PermLibraryManage), controllers.DeleteMessageTemplate)
	auth.Get("/templates/{id:int64}/render", can(models.PermLibraryView), controllers.RenderMessageTemplate)

	auth.Get("/snippets", can(models.PermLibraryView), controllers.ListSnippets)
	auth.Post("/snippets", can(models.PermLibraryManage), controllers.CreateSnippet)
	auth.Put("/snippets/{id:int64}", can(models.PermLibraryManage), controllers.UpdateSnippet)
	auth.Delete("/snippets/{id:int64}", can(models.PermLibraryManage), controllers.DeleteSnippet)

	auth.Get("/settings/contact-form", controllers.GetContactForm)
	auth.Put("/settings/contact-form", can(models.PermSettingsForms), controllers.UpdateContactForm)
	auth.Get("/settings/deal-form", controllers.GetDealForm)
	auth.Put("/settings/deal-form", can(models.PermSettingsForms), controllers.UpdateDealForm)

	auth.Get("/properties", controllers.ListProperties)
	auth.Get("/properties/values", controllers.GetPropertyValues)
	auth.Put("/properties/values", controllers.SavePropertyValues)
	auth.Post("/properties", can(models.PermSettingsProperties), controllers.CreateProperty)
	auth.Put("/properties/{id:int64}", can(models.PermSettingsProperties), controllers.UpdateProperty)
	auth.Delete("/properties/{id:int64}", can(models.PermSettingsProperties), controllers.DeleteProperty)

	// Permissões por perfil (matriz configurável).
	auth.Get("/permissions", can(models.PermSettingsPermissions), controllers.GetPermissions)
	auth.Put("/permissions", can(models.PermSettingsPermissions), controllers.UpdatePermissions)

	// Trilha de auditoria (quem fez o quê).
	auth.Get("/audit", can(models.PermSettingsAudit), controllers.ListAuditLog)

	auth.Get("/views", controllers.ListViews)
	auth.Post("/views", can(models.PermViewsManage), controllers.CreateView)
	auth.Put("/views/{id:int64}", can(models.PermViewsManage), controllers.RenameView)
	auth.Delete("/views/{id:int64}", can(models.PermViewsManage), controllers.DeleteView)

	auth.Get("/dashboard", can(models.PermDashboardView), controllers.Dashboard)
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
