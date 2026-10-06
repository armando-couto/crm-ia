package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
)

// InboundEvent é o formato dos eventos do webhook de e-mail de entrada do Mandrill
// (campo mandrill_events do POST).
type InboundEvent struct {
	Event string `json:"event"`
	Msg   struct {
		FromEmail string `json:"from_email"`
		FromName  string `json:"from_name"`
		Email     string `json:"email"` // destinatário (nossa caixa)
		Subject   string `json:"subject"`
		Text      string `json:"text"`
		HTML      string `json:"html"`
	} `json:"msg"`
}

// ParseInboundEvents decodifica o JSON do campo mandrill_events.
func ParseInboundEvents(payload string) ([]InboundEvent, error) {
	var events []InboundEvent
	if err := json.Unmarshal([]byte(payload), &events); err != nil {
		return nil, fmt.Errorf("payload mandrill_events inválido: %w", err)
	}
	return events, nil
}

// ProcessInboundEvents grava os e-mails recebidos na caixa de entrada,
// agrupando por conversa e vinculando ao contato pelo remetente.
// Retorna quantas mensagens foram criadas.
func ProcessInboundEvents(db *sql.DB, events []InboundEvent) (int, error) {
	created := 0
	for _, e := range events {
		if e.Event != "inbound" {
			continue
		}
		from := models.NormalizeEmail(e.Msg.FromEmail)
		if from == "" {
			continue
		}
		body := strings.TrimSpace(e.Msg.Text)
		if body == "" {
			body = strings.TrimSpace(e.Msg.HTML)
		}
		if body == "" {
			body = "(mensagem vazia)"
		}

		conv, err := models.FindOrCreateConversation(db, from, e.Msg.Subject)
		if err != nil {
			return created, err
		}
		msg := &models.ConversationMessage{
			ConversationID: conv.ID,
			Direction:      "recebida",
			FromEmail:      from,
			ToEmail:        models.NormalizeEmail(e.Msg.Email),
			Subject:        strings.TrimSpace(e.Msg.Subject),
			Body:           body,
		}
		if err := models.AddConversationMessage(db, msg); err != nil {
			return created, err
		}

		// Registra na timeline do contato quando o remetente é conhecido.
		if conv.ContactID != nil {
			meta, _ := json.Marshal(map[string]string{"from": from, "subject": msg.Subject})
			_ = models.CreateActivity(db, &models.Activity{
				Kind:      models.ActivityEmail,
				Content:   fmt.Sprintf("E-mail recebido: %s", models.NormalizeSubject(e.Msg.Subject)),
				Metadata:  meta,
				ContactID: conv.ContactID,
			})

			// Responder é a saída natural da cadência: tira o contato das
			// sequências que têm a regra ligada e alimenta a taxa de resposta.
			SequenceExitOnReply(db, *conv.ContactID)
		}
		created++
	}
	return created, nil
}
