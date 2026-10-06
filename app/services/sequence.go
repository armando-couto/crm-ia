package services

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// ProcessDueSequences roda as inscrições cujo prazo venceu. É chamada pelo
// worker a cada minuto, junto das automações.
func ProcessDueSequences(db *sql.DB) {
	pending, err := models.DueMembers(db, time.Now(), 100)
	if err != nil {
		log.Printf("sequências: falha ao buscar inscrições pendentes: %v", err)
		return
	}
	for i := range pending {
		runMemberStep(db, &pending[i])
	}
}

// runMemberStep executa a etapa em que a inscrição parou.
func runMemberStep(db *sql.DB, member *models.SequenceMember) {
	seq, err := models.SequenceByID(db, member.SequenceID)
	if err != nil {
		_ = models.FinishMember(db, member.ID, models.MemberExited, "manual")
		return
	}
	if !seq.Active {
		// Sequência pausada: a inscrição espera sem avançar.
		return
	}

	step := member.Step
	// Sequência dinâmica: assim que o contato engaja, pula o que sobrou de
	// automático e vai para a primeira etapa manual.
	if seq.Dynamic && member.Engaged && step < len(seq.Steps) &&
		!models.IsManualStep(seq.Steps[step].Kind) {
		if manual := models.FirstManualStep(seq.Steps, step); manual >= 0 {
			step = manual
		}
	}

	if step >= len(seq.Steps) {
		_ = models.FinishMember(db, member.ID, models.MemberDone, "fim")
		return
	}

	current := seq.Steps[step]
	if models.IsManualStep(current.Kind) {
		if err := createSequenceTask(db, seq, member, current, step); err != nil {
			log.Printf("sequências: falha ao criar a tarefa da etapa %d: %v", step+1, err)
		}
		return
	}

	if err := sendSequenceEmail(db, seq, member, current); err != nil {
		log.Printf("sequências: falha no e-mail da etapa %d para o contato %d: %v",
			step+1, member.ContactID, err)
		if Mail == nil {
			// Sem serviço de e-mail configurado a inscrição espera: quando a
			// chave do Mandrill entrar, a cadência continua de onde parou.
			return
		}
		// Erro real de envio encerra a inscrição: insistir mandaria duplicado.
		_ = models.FinishMember(db, member.ID, models.MemberExited, "manual")
		return
	}
	scheduleNext(db, seq, member, step+1)
}

// scheduleNext marca a próxima etapa respeitando o intervalo dela.
func scheduleNext(db *sql.DB, seq *models.Sequence, member *models.SequenceMember, next int) {
	if next >= len(seq.Steps) {
		_ = models.FinishMember(db, member.ID, models.MemberDone, "fim")
		return
	}
	runAt := time.Now().AddDate(0, 0, seq.Steps[next].DelayDays)
	if err := models.AdvanceMember(db, member.ID, next, runAt); err != nil {
		log.Printf("sequências: falha ao agendar a etapa %d: %v", next+1, err)
	}
}

// createSequenceTask põe a etapa manual na fila de quem vende e trava a
// cadência até a tarefa ser concluída.
func createSequenceTask(db *sql.DB, seq *models.Sequence, member *models.SequenceMember,
	step models.SequenceStep, index int) error {

	contact, err := models.ContactByID(db, member.ContactID)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(step.Title)
	if title == "" {
		title = models.StepLabels[step.Kind]
	}
	nome := strings.TrimSpace(contact.FirstName + " " + contact.LastName)

	due := time.Now()
	task := &models.Task{
		Title:       fmt.Sprintf("%s: %s", title, nome),
		Description: step.Note,
		Type:        taskTypeForStep(step.Kind),
		Priority:    "media",
		DueDate:     &due,
		ContactID:   &member.ContactID,
	}
	// A tarefa vai para o dono da sequência; sem dono, para quem inscreveu.
	if seq.OwnerID != nil {
		task.OwnerID = seq.OwnerID
	} else {
		task.OwnerID = contact.OwnerID
	}

	if err := models.CreateTask(db, task); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE tasks SET sequence_member_id = $1 WHERE id = $2`,
		member.ID, task.ID); err != nil {
		return err
	}
	if err := models.HoldForTask(db, member.ID, task.ID); err != nil {
		return err
	}

	return models.CreateActivity(db, &models.Activity{
		Kind:      models.ActivitySistema,
		Content:   fmt.Sprintf("Sequência \"%s\": etapa %d na fila — %s", seq.Name, index+1, title),
		ContactID: &member.ContactID,
	})
}

// taskTypeForStep traduz a etapa para o tipo de tarefa do CRM.
func taskTypeForStep(kind string) string {
	switch kind {
	case models.StepTaskCall:
		return "ligacao"
	case models.StepTaskEmail:
		return "email"
	}
	return "tarefa"
}

// sendSequenceEmail manda o e-mail automático da etapa, já rastreado e ligado
// à sequência (é isso que alimenta as taxas de abertura e resposta).
func sendSequenceEmail(db *sql.DB, seq *models.Sequence, member *models.SequenceMember,
	step models.SequenceStep) error {

	if Remetente() == nil {
		return fmt.Errorf("serviço de e-mail não configurado")
	}
	if err := EnvioAutomaticoPermitido(db, time.Now()); err != nil {
		return err
	}
	contact, err := models.ContactByID(db, member.ContactID)
	if err != nil {
		return err
	}
	if contact.Email == "" {
		return fmt.Errorf("contato sem e-mail")
	}

	subject, body := step.Subject, step.Body
	if step.TemplateID > 0 {
		tpl, err := models.MessageTemplateByID(db, step.TemplateID)
		if err != nil {
			return fmt.Errorf("modelo de e-mail não encontrado")
		}
		subject, body = tpl.Subject, tpl.Body
	}
	subject = models.RenderTemplate(subject, contact)
	body = models.RenderTemplate(body, contact)

	html := strings.ReplaceAll(body, "\n", "<br>")
	msg := &models.EmailMessage{
		Subject:   subject,
		ToEmail:   contact.Email,
		Body:      html,
		ContactID: &contact.ID,
		UserID:    seq.OwnerID,
		Source:    models.EmailSourceAutomation,
	}
	tracked, err := TrackOutgoingEmail(db, msg)
	if err != nil {
		log.Printf("sequências: rastreio indisponível, enviando sem ele: %v", err)
		tracked = html
	} else if _, err := db.Exec(`UPDATE email_messages SET sequence_id = $1 WHERE id = $2`,
		seq.ID, msg.ID); err != nil {
		log.Printf("sequências: falha ao ligar o e-mail à sequência: %v", err)
	}

	nome := strings.TrimSpace(contact.FirstName + " " + contact.LastName)
	if err := Remetente().Send(contact.Email, nome, subject, tracked); err != nil {
		return err
	}
	RegistrarEnvio(time.Now())

	return models.CreateActivity(db, &models.Activity{
		Kind:      models.ActivityEmail,
		Content:   fmt.Sprintf("Sequência \"%s\": %s", seq.Name, subject),
		ContactID: &contact.ID,
	})
}

// CompleteSequenceTask é chamado quando uma tarefa é concluída: se ela segurava
// uma sequência, a cadência volta a andar.
func CompleteSequenceTask(db *sql.DB, taskID int64) {
	member, err := models.MemberByTask(db, taskID)
	if err != nil {
		return // A tarefa não pertence a nenhuma sequência.
	}
	if member.Status != models.MemberWaiting {
		return
	}
	seq, err := models.SequenceByID(db, member.SequenceID)
	if err != nil {
		return
	}
	scheduleNext(db, seq, member, member.Step+1)
}

// ===== Regras de saída =====

// SequenceExitOnReply tira o contato das sequências que saem na resposta.
// Chamado quando chega e-mail de entrada do contato.
func SequenceExitOnReply(db *sql.DB, contactID int64) {
	if err := models.MarkEmailReplied(db, contactID); err != nil {
		log.Printf("sequências: falha ao marcar a resposta: %v", err)
	}
	if err := models.MarkEngaged(db, contactID); err != nil {
		log.Printf("sequências: falha ao marcar o engajamento: %v", err)
	}

	saiu, err := models.ExitByRule(db, contactID, "respondeu")
	if err != nil {
		log.Printf("sequências: falha ao encerrar por resposta: %v", err)
		return
	}
	if saiu > 0 {
		registerExit(db, contactID, "o contato respondeu")
	}
}

// SequenceExitOnMeeting tira o contato das sequências que saem no agendamento.
func SequenceExitOnMeeting(db *sql.DB, contactID int64) {
	if err := models.MarkEngaged(db, contactID); err != nil {
		log.Printf("sequências: falha ao marcar o engajamento: %v", err)
	}

	saiu, err := models.ExitByRule(db, contactID, "reuniao")
	if err != nil {
		log.Printf("sequências: falha ao encerrar por reunião: %v", err)
		return
	}
	if saiu > 0 {
		registerExit(db, contactID, "a reunião foi agendada")
	}
}

// SequenceMarkEngaged registra abertura ou clique: é o que faz a sequência
// dinâmica trocar para as etapas manuais.
func SequenceMarkEngaged(db *sql.DB, contactID int64) {
	if err := models.MarkEngaged(db, contactID); err != nil {
		log.Printf("sequências: falha ao marcar o engajamento: %v", err)
	}
}

func registerExit(db *sql.DB, contactID int64, motivo string) {
	if err := models.CreateActivity(db, &models.Activity{
		Kind:      models.ActivitySistema,
		Content:   "Saiu da sequência: " + motivo,
		ContactID: &contactID,
	}); err != nil {
		log.Printf("sequências: falha ao registrar a saída: %v", err)
	}
}
