package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"
)

// Subject é o registro sobre o qual a automação age. Nem todo gatilho traz
// tudo: um ticket pode não ter negócio, um negócio pode não ter contato.
type Subject struct {
	Entity    string // contato | negocio | ticket | tarefa
	EntityID  int64
	ContactID *int64
	DealID    *int64
	TicketID  *int64
	OwnerID   *int64
	Label     string
}

// actionConfig junta os parâmetros possíveis de todas as ações.
type actionConfig struct {
	TemplateID int64  `json:"template_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	UserID     int64  `json:"user_id"`
	StageID    int64  `json:"stage_id"`
	ListID     int64  `json:"list_id"`
	Lifecycle  string `json:"lifecycle_stage"`
	Days       int    `json:"days"`
	Priority   string `json:"priority"`
}

// triggerConfig são os parâmetros do gatilho.
type triggerConfig struct {
	StageID   int64  `json:"stage_id"`
	Lifecycle string `json:"lifecycle_stage"`
	FormID    int64  `json:"form_id"`
	Days      int    `json:"days"`
}

func parseActionConfig(raw json.RawMessage) actionConfig {
	var cfg actionConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	return cfg
}

func parseTriggerConfig(raw json.RawMessage) triggerConfig {
	var cfg triggerConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	return cfg
}

// AutomationsEnabled liga o motor. A aplicação liga na subida, junto com o
// worker; nos testes de controller ele fica desligado para os gatilhos em
// goroutine não caírem no mock de banco de outro teste.
var AutomationsEnabled bool

// FireTrigger roda todas as automações ativas do gatilho. É chamada em
// goroutine pelos controllers: falha de automação nunca derruba a requisição
// que a disparou.
func FireTrigger(db *sql.DB, kind string, subject Subject, match triggerConfig) {
	if !AutomationsEnabled {
		return
	}

	automations, err := models.ActiveAutomationsByTrigger(db, kind)
	if err != nil {
		log.Printf("automações: falha ao carregar gatilho %s: %v", kind, err)
		return
	}

	for i := range automations {
		auto := &automations[i]
		cfg := parseTriggerConfig(auto.TriggerConfig)

		// Filtros do gatilho: etapa, estágio ou formulário específico.
		if cfg.StageID != 0 && cfg.StageID != match.StageID {
			continue
		}
		if cfg.Lifecycle != "" && cfg.Lifecycle != match.Lifecycle {
			continue
		}
		if cfg.FormID != 0 && cfg.FormID != match.FormID {
			continue
		}
		RunAutomation(db, auto, subject, 0)
	}
}

// FireContactCreated e companhia são os atalhos usados pelos controllers.
func FireContactCreated(db *sql.DB, contact *models.Contact) {
	FireTrigger(db, models.TriggerContactCreated, SubjectFromContact(contact), triggerConfig{})
}

func FireContactStage(db *sql.DB, contact *models.Contact) {
	FireTrigger(db, models.TriggerContactStage, SubjectFromContact(contact),
		triggerConfig{Lifecycle: contact.LifecycleStage})
}

func FireFormSubmitted(db *sql.DB, contact *models.Contact, formID int64) {
	FireTrigger(db, models.TriggerFormSubmitted, SubjectFromContact(contact),
		triggerConfig{FormID: formID})
}

func FireDealCreated(db *sql.DB, deal *models.Deal) {
	FireTrigger(db, models.TriggerDealCreated, SubjectFromDeal(deal),
		triggerConfig{StageID: deal.StageID})
}

func FireDealStage(db *sql.DB, deal *models.Deal) {
	FireTrigger(db, models.TriggerDealStage, SubjectFromDeal(deal),
		triggerConfig{StageID: deal.StageID})
}

func FireDealClosed(db *sql.DB, deal *models.Deal) {
	kind := models.TriggerDealWon
	if deal.Status == "perdido" {
		kind = models.TriggerDealLost
	}
	FireTrigger(db, kind, SubjectFromDeal(deal), triggerConfig{})
}

func FireTicketCreated(db *sql.DB, ticket *models.Ticket) {
	FireTrigger(db, models.TriggerTicketCreated, Subject{
		Entity:    "ticket",
		EntityID:  ticket.ID,
		ContactID: ticket.ContactID,
		TicketID:  &ticket.ID,
		OwnerID:   ticket.OwnerID,
		Label:     ticket.Subject,
	}, triggerConfig{})
}

// SubjectFromContact monta o alvo a partir de um contato.
func SubjectFromContact(c *models.Contact) Subject {
	return Subject{
		Entity:    "contato",
		EntityID:  c.ID,
		ContactID: &c.ID,
		OwnerID:   c.OwnerID,
		Label:     strings.TrimSpace(c.FirstName + " " + c.LastName),
	}
}

// SubjectFromDeal monta o alvo a partir de um negócio.
func SubjectFromDeal(d *models.Deal) Subject {
	return Subject{
		Entity:    "negocio",
		EntityID:  d.ID,
		ContactID: d.ContactID,
		DealID:    &d.ID,
		OwnerID:   d.OwnerID,
		Label:     d.Name,
	}
}

// RunAutomation executa as ações a partir de startStep. Ao encontrar uma
// espera, agenda a retomada e para: o worker continua depois.
func RunAutomation(db *sql.DB, auto *models.Automation, subject Subject, startStep int) {
	executed := []string{}

	for step := startStep; step < len(auto.Actions); step++ {
		action := auto.Actions[step]

		if action.Kind == models.ActionWait {
			cfg := parseActionConfig(action.Config)
			if cfg.Days <= 0 {
				cfg.Days = 1
			}
			// Sem contato não há como retomar depois: encerra aqui.
			if subject.ContactID == nil {
				recordRun(db, auto, subject, "erro",
					"a espera precisa de um contato para retomar a sequência")
				return
			}
			runAt := time.Now().AddDate(0, 0, cfg.Days)
			if err := models.EnrollInSequence(db, auto.ID, *subject.ContactID,
				subject.DealID, step+1, runAt); err != nil {
				recordRun(db, auto, subject, "erro", "falha ao agendar a espera: "+err.Error())
				return
			}
			detail := fmt.Sprintf("%s · aguardando %d dia(s)",
				strings.Join(executed, ", "), cfg.Days)
			recordRun(db, auto, subject, "aguardando", strings.TrimPrefix(detail, " · "))
			return
		}

		if err := runAction(db, action, subject); err != nil {
			recordRun(db, auto, subject, "erro",
				fmt.Sprintf("%s: %v", models.ActionLabels[action.Kind], err))
			return
		}
		executed = append(executed, models.ActionLabels[action.Kind])
	}

	recordRun(db, auto, subject, "sucesso", strings.Join(executed, ", "))
}

func recordRun(db *sql.DB, auto *models.Automation, subject Subject, status, detail string) {
	entityID := subject.EntityID
	run := &models.AutomationRun{
		AutomationID: auto.ID,
		Entity:       subject.Entity,
		Status:       status,
		Detail:       detail,
	}
	if entityID > 0 {
		run.EntityID = &entityID
	}
	if subject.Label != "" {
		run.Detail = subject.Label + " — " + detail
	}
	if err := models.MarkAutomationRun(db, run); err != nil {
		log.Printf("automações: falha ao registrar execução: %v", err)
	}
}

// runAction executa um passo. Erros voltam para o chamador, que grava o
// histórico e interrompe a automação daquele registro.
func runAction(db *sql.DB, action models.AutomationAction, subject Subject) error {
	cfg := parseActionConfig(action.Config)

	switch action.Kind {
	case models.ActionSendEmail:
		return actionSendEmail(db, cfg, subject)

	case models.ActionCreateTask:
		title := strings.TrimSpace(cfg.Title)
		if title == "" {
			title = "Tarefa criada por automação"
		}
		due := time.Now().AddDate(0, 0, max(cfg.Days, 0))
		task := &models.Task{
			Title:     title,
			Type:      "tarefa",
			Priority:  orDefault(cfg.Priority, "media"),
			DueDate:   &due,
			ContactID: subject.ContactID,
			DealID:    subject.DealID,
		}
		if cfg.UserID > 0 {
			task.OwnerID = &cfg.UserID
		} else {
			task.OwnerID = subject.OwnerID
		}
		return models.CreateTask(db, task)

	case models.ActionSetOwner:
		if cfg.UserID == 0 {
			return fmt.Errorf("escolha o novo dono")
		}
		return setOwner(db, subject, cfg.UserID)

	case models.ActionSetStage:
		if subject.DealID == nil {
			return fmt.Errorf("só negócios têm etapa")
		}
		if cfg.StageID == 0 {
			return fmt.Errorf("escolha a etapa de destino")
		}
		_, err := db.Exec(`UPDATE deals SET stage_id = $1, updated_at = NOW() WHERE id = $2`,
			cfg.StageID, *subject.DealID)
		return err

	case models.ActionSetLifecycle:
		if subject.ContactID == nil {
			return fmt.Errorf("só contatos têm estágio")
		}
		if !models.ValidLifecycleStage(cfg.Lifecycle) {
			return fmt.Errorf("estágio inválido")
		}
		_, err := db.Exec(`UPDATE contacts SET lifecycle_stage = $1, updated_at = NOW() WHERE id = $2`,
			cfg.Lifecycle, *subject.ContactID)
		return err

	case models.ActionAddToList:
		if subject.ContactID == nil {
			return fmt.Errorf("só contatos entram em listas")
		}
		if cfg.ListID == 0 {
			return fmt.Errorf("escolha a lista")
		}
		_, err := db.Exec(`
			INSERT INTO list_members (list_id, contact_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, cfg.ListID, *subject.ContactID)
		return err

	case models.ActionAddNote:
		content := strings.TrimSpace(cfg.Content)
		if content == "" {
			return fmt.Errorf("escreva a observação")
		}
		return models.CreateActivity(db, &models.Activity{
			Kind:      models.ActivitySistema,
			Content:   content,
			ContactID: subject.ContactID,
			DealID:    subject.DealID,
			TicketID:  subject.TicketID,
		})

	case models.ActionNotifyUser:
		if cfg.UserID == 0 {
			return fmt.Errorf("escolha quem deve ser avisado")
		}
		NotifyAutomation(db, cfg.UserID, subject.Label, cfg.Content, subject.Entity, subject.EntityID)
		return nil
	}

	return fmt.Errorf("ação desconhecida: %s", action.Kind)
}

// actionSendEmail manda o e-mail do modelo (ou do texto avulso) para o contato,
// já com o rastreio de abertura e clique.
func actionSendEmail(db *sql.DB, cfg actionConfig, subject Subject) error {
	if Mail == nil {
		return fmt.Errorf("serviço de e-mail não configurado")
	}
	if subject.ContactID == nil {
		return fmt.Errorf("a ação de e-mail precisa de um contato")
	}

	contact, err := models.ContactByID(db, *subject.ContactID)
	if err != nil {
		return err
	}
	if contact.Email == "" {
		return fmt.Errorf("o contato não tem e-mail")
	}

	subjectText, bodyText := cfg.Subject, cfg.Body
	if cfg.TemplateID > 0 {
		tpl, err := models.MessageTemplateByID(db, cfg.TemplateID)
		if err != nil {
			return fmt.Errorf("modelo de e-mail não encontrado")
		}
		subjectText, bodyText = tpl.Subject, tpl.Body
	}
	subjectText = models.RenderTemplate(subjectText, contact)
	bodyText = models.RenderTemplate(bodyText, contact)
	if strings.TrimSpace(subjectText) == "" || strings.TrimSpace(bodyText) == "" {
		return fmt.Errorf("assunto e mensagem são obrigatórios")
	}

	html := strings.ReplaceAll(bodyText, "\n", "<br>")
	msg := &models.EmailMessage{
		Subject:   subjectText,
		ToEmail:   contact.Email,
		Body:      html,
		ContactID: &contact.ID,
		DealID:    subject.DealID,
		Source:    models.EmailSourceAutomation,
	}
	tracked, err := TrackOutgoingEmail(db, msg)
	if err != nil {
		log.Printf("automações: rastreio indisponível, enviando sem ele: %v", err)
		tracked = html
	}

	name := strings.TrimSpace(contact.FirstName + " " + contact.LastName)
	if err := Mail.Send(contact.Email, name, subjectText, tracked); err != nil {
		return err
	}

	return models.CreateActivity(db, &models.Activity{
		Kind:      models.ActivityEmail,
		Content:   "E-mail enviado por automação: " + subjectText,
		ContactID: &contact.ID,
		DealID:    subject.DealID,
	})
}

func setOwner(db *sql.DB, subject Subject, userID int64) error {
	switch subject.Entity {
	case "contato":
		_, err := db.Exec(`UPDATE contacts SET owner_id = $1, updated_at = NOW() WHERE id = $2`,
			userID, subject.EntityID)
		return err
	case "negocio":
		_, err := db.Exec(`UPDATE deals SET owner_id = $1, updated_at = NOW() WHERE id = $2`,
			userID, subject.EntityID)
		return err
	case "ticket":
		_, err := db.Exec(`UPDATE tickets SET owner_id = $1, updated_at = NOW() WHERE id = $2`,
			userID, subject.EntityID)
		return err
	}
	return fmt.Errorf("não dá para mudar o dono de %s", subject.Entity)
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ===== Worker =====

// automationTick é o intervalo de varredura das sequências pendentes.
const automationTick = time.Minute

// timeTriggerEvery limita a varredura dos gatilhos por tempo.
const timeTriggerEvery = time.Hour

// StartAutomationWorker sobe a rotina de fundo que retoma sequências e roda os
// gatilhos por tempo. Deve ser chamada uma vez na subida da aplicação.
func StartAutomationWorker(db *sql.DB) {
	AutomationsEnabled = true

	go func() {
		ticker := time.NewTicker(automationTick)
		defer ticker.Stop()

		lastTimeSweep := time.Time{}
		for range ticker.C {
			ProcessDueEnrollments(db)

			if time.Since(lastTimeSweep) >= timeTriggerEvery {
				lastTimeSweep = time.Now()
				RunTimeTriggers(db)
			}
		}
	}()
	log.Printf("Automações: worker ativo (sequências a cada %s, gatilhos por tempo a cada %s)",
		automationTick, timeTriggerEvery)
}

// ProcessDueEnrollments retoma as sequências cujo prazo de espera venceu.
func ProcessDueEnrollments(db *sql.DB) {
	pending, err := models.DueEnrollments(db, time.Now(), 100)
	if err != nil {
		log.Printf("automações: falha ao buscar sequências pendentes: %v", err)
		return
	}

	for _, enrollment := range pending {
		auto, err := models.AutomationByID(db, enrollment.AutomationID)
		if err != nil || !auto.Active {
			// Automação apagada ou desligada: encerra a inscrição.
			_ = models.FinishEnrollment(db, enrollment.ID, "cancelada")
			continue
		}

		contact, err := models.ContactByID(db, enrollment.ContactID)
		if err != nil {
			_ = models.FinishEnrollment(db, enrollment.ID, "cancelada")
			continue
		}

		subject := SubjectFromContact(contact)
		subject.DealID = enrollment.DealID

		// Marca como concluída antes: se o passo seguinte for outra espera, o
		// RunAutomation reabre a inscrição com o novo prazo.
		if err := models.FinishEnrollment(db, enrollment.ID, "concluida"); err != nil {
			log.Printf("automações: falha ao encerrar inscrição %d: %v", enrollment.ID, err)
			continue
		}
		RunAutomation(db, auto, subject, enrollment.Step)
	}
}

// RunTimeTriggers varre os gatilhos que dependem do relógio (negócio parado,
// tarefa atrasada, contato sem interação).
func RunTimeTriggers(db *sql.DB) {
	for _, kind := range models.TimeTriggers {
		automations, err := models.ActiveAutomationsByTrigger(db, kind)
		if err != nil {
			log.Printf("automações: falha ao carregar %s: %v", kind, err)
			continue
		}

		for i := range automations {
			auto := &automations[i]
			cfg := parseTriggerConfig(auto.TriggerConfig)
			days := cfg.Days
			if days <= 0 {
				days = 7
			}
			runTimeTrigger(db, auto, kind, days)
		}
	}
}

func runTimeTrigger(db *sql.DB, auto *models.Automation, kind string, days int) {
	// Uma vez por registro por dia: a varredura roda de hora em hora.
	const window = 24 * time.Hour

	switch kind {
	case models.TriggerDealIdle:
		deals, err := models.IdleDeals(db, days)
		if err != nil {
			log.Printf("automações: falha ao buscar negócios parados: %v", err)
			return
		}
		for i := range deals {
			deal := &deals[i]
			if ran, _ := models.AlreadyRanFor(db, auto.ID, "negocio", deal.ID, window); ran {
				continue
			}
			RunAutomation(db, auto, SubjectFromDeal(deal), 0)
		}

	case models.TriggerContactIdle:
		contacts, err := models.IdleContacts(db, days)
		if err != nil {
			log.Printf("automações: falha ao buscar contatos sem interação: %v", err)
			return
		}
		for i := range contacts {
			contact := &contacts[i]
			if ran, _ := models.AlreadyRanFor(db, auto.ID, "contato", contact.ID, window); ran {
				continue
			}
			RunAutomation(db, auto, SubjectFromContact(contact), 0)
		}

	case models.TriggerTaskOverdue:
		tasks, err := models.OverdueTasks(db, days)
		if err != nil {
			log.Printf("automações: falha ao buscar tarefas atrasadas: %v", err)
			return
		}
		for i := range tasks {
			task := &tasks[i]
			if ran, _ := models.AlreadyRanFor(db, auto.ID, "tarefa", task.ID, window); ran {
				continue
			}
			RunAutomation(db, auto, Subject{
				Entity:    "tarefa",
				EntityID:  task.ID,
				ContactID: task.ContactID,
				DealID:    task.DealID,
				OwnerID:   task.OwnerID,
				Label:     task.Title,
			}, 0)
		}
	}
}

// NotifyAutomation avisa alguém da equipe que a automação agiu.
func NotifyAutomation(db *sql.DB, userID int64, label, note, entity string, entityID int64) {
	if Mail == nil {
		return
	}
	user, err := models.UserByID(db, userID)
	if err != nil || !user.Active || user.Email == "" {
		return
	}

	path := map[string]string{
		"contato": "/contatos/", "negocio": "/negocios/", "ticket": "/tickets/",
	}[entity]
	link := utils.AppURL
	if path != "" {
		link = fmt.Sprintf("%s%s%d", utils.AppURL, path, entityID)
	}

	subject := "Fix CRM: automação precisa da sua atenção"
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>!</p>
		<p>Uma automação do CRM sinalizou este registro: <strong>%s</strong></p>
		<p>%s</p>
		<p><a href="%s" style="display:inline-block;background:#9B52DF;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">Abrir no Fix CRM</a></p>`,
		user.Name, label, note, link)

	if err := Mail.Send(user.Email, user.Name, subject, emailLayout(subject, body)); err != nil {
		log.Printf("automações: falha ao avisar %s: %v", user.Email, err)
	}
}
