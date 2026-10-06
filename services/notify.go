package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"
)

// Tipos de atribuição notificáveis.
const (
	NotifyDeal   = "negocio"
	NotifyTask   = "tarefa"
	NotifyTicket = "ticket"
)

const notificationPrefsKey = "notifications"

// notificationEnabled verifica a preferência do usuário para o tipo de aviso
// (tudo ligado por padrão quando nunca configurou).
func notificationEnabled(db *sql.DB, userID int64, kind string) bool {
	prefs := models.DefaultNotificationPrefs()
	if _, err := models.GetUserSetting(db, userID, notificationPrefsKey, &prefs); err != nil {
		return true
	}
	switch kind {
	case NotifyDeal:
		return prefs.DealAssigned
	case NotifyTask:
		return prefs.TaskAssigned
	case NotifyTicket:
		return prefs.TicketAssigned
	}
	return true
}

// NotifyAssignment avisa por e-mail (Mandrill) quando um registro é atribuído
// a outro usuário. Não bloqueia a requisição: rode em goroutine.
func NotifyAssignment(db *sql.DB, ownerID *int64, actorID int64, kind, title, path string) {
	if Mail == nil || ownerID == nil || *ownerID == actorID {
		return
	}
	if !notificationEnabled(db, *ownerID, kind) {
		return
	}

	owner, err := models.UserByID(db, *ownerID)
	if err != nil || !owner.Active || owner.Email == "" {
		return
	}
	actor, err := models.UserByID(db, actorID)
	actorName := "um colega"
	if err == nil {
		actorName = actor.Name
	}

	kindLabels := map[string]string{
		NotifyDeal:   "um negócio",
		NotifyTask:   "uma tarefa",
		NotifyTicket: "um ticket",
	}
	subject := fmt.Sprintf("Fix CRM: %s foi atribuído(a) a você", kindLabels[kind])
	link := fmt.Sprintf("%s%s", utils.AppURL, path)
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>!</p>
		<p>%s atribuiu %s a você no Fix CRM:</p>
		<p style="font-size:16px;font-weight:bold;">%s</p>
		<p><a href="%s" style="display:inline-block;background:#9B52DF;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">Abrir no Fix CRM</a></p>
		<p style="color:#8E8E8E;font-size:12px;">Você pode desativar estes avisos em Configurações &gt; Notificações.</p>`,
		owner.Name, actorName, kindLabels[kind], title, link)

	if err := Mail.Send(owner.Email, owner.Name, subject, emailLayout(subject, body)); err != nil {
		log.Printf("falha ao notificar atribuição (%s -> %s): %v", kind, owner.Email, err)
	}
}

// NotifyNewLead avisa o dono do formulário que chegou um lead novo. Roda em
// goroutine a partir do handler público, então não bloqueia o envio do site.
func NotifyNewLead(db *sql.DB, ownerID int64, formName, contactName, contactEmail string, contactID int64) {
	if Mail == nil {
		return
	}
	owner, err := models.UserByID(db, ownerID)
	if err != nil || !owner.Active || owner.Email == "" {
		return
	}

	subject := "Fix CRM: novo lead pelo formulário " + formName
	link := fmt.Sprintf("%s/contatos/%d", utils.AppURL, contactID)
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>!</p>
		<p>O formulário <strong>%s</strong> acabou de receber um lead:</p>
		<p style="font-size:16px;font-weight:bold;">%s</p>
		<p>%s</p>
		<p><a href="%s" style="display:inline-block;background:#9B52DF;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">Abrir o contato</a></p>`,
		owner.Name, formName, contactName, contactEmail, link)

	if err := Mail.Send(owner.Email, owner.Name, subject, emailLayout(subject, body)); err != nil {
		log.Printf("falha ao avisar sobre o lead do formulário %s: %v", formName, err)
	}
}

// NotifyBooking confirma o agendamento para quem marcou e avisa quem vai
// atender. Roda em goroutine a partir da rota pública.
func NotifyBooking(db *sql.DB, page *models.BookingPage, contact *models.Contact, start time.Time) {
	if Mail == nil {
		return
	}
	quando := start.Format("02/01/2006 às 15:04")
	local := page.Location
	if local == "" {
		local = "a combinar"
	}

	// Confirmação para o cliente.
	if contact.Email != "" {
		subject := "Reunião confirmada: " + quando
		body := fmt.Sprintf(`
			<p>Olá, <strong>%s</strong>!</p>
			<p>Sua conversa com <strong>%s</strong> está marcada para <strong>%s</strong>.</p>
			<p>Local: %s</p>
			<p>Se precisar remarcar, é só responder este e-mail.</p>`,
			contact.FirstName, page.UserName, quando, local)
		if err := Mail.Send(contact.Email, contact.FirstName, subject, emailLayout(subject, body)); err != nil {
			log.Printf("falha ao confirmar agendamento para %s: %v", contact.Email, err)
		}
	}

	// Aviso para quem atende.
	host, err := models.UserByID(db, page.UserID)
	if err != nil || !host.Active || host.Email == "" {
		return
	}
	subject := "Fix CRM: nova reunião agendada"
	link := fmt.Sprintf("%s/contatos/%d", utils.AppURL, contact.ID)
	body := fmt.Sprintf(`
		<p>Olá, <strong>%s</strong>!</p>
		<p><strong>%s %s</strong> (%s) agendou uma conversa com você para <strong>%s</strong>.</p>
		<p>Local: %s</p>
		<p><a href="%s" style="display:inline-block;background:#9B52DF;color:#FFFFFF;padding:10px 24px;border-radius:8px;text-decoration:none;">Abrir o contato</a></p>`,
		host.Name, contact.FirstName, contact.LastName, contact.Email, quando, local, link)
	if err := Mail.Send(host.Email, host.Name, subject, emailLayout(subject, body)); err != nil {
		log.Printf("falha ao avisar %s sobre o agendamento: %v", host.Email, err)
	}
}
