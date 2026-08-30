package services

import (
	"database/sql"
	"fmt"
	"log"

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
