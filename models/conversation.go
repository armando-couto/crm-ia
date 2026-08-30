package models

import (
	"database/sql"
	"strings"
	"time"
)

type Conversation struct {
	ID            int64     `json:"id"`
	Subject       string    `json:"subject"`
	ContactID     *int64    `json:"contact_id"`
	ContactName   string    `json:"contact_name,omitempty"`
	PeerEmail     string    `json:"peer_email"`
	Status        string    `json:"status"` // aberta | fechada
	Unread        bool      `json:"unread"`
	LastMessageAt time.Time `json:"last_message_at"`
	LastPreview   string    `json:"last_preview,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type ConversationMessage struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	Direction      string    `json:"direction"` // recebida | enviada
	FromEmail      string    `json:"from_email"`
	ToEmail        string    `json:"to_email"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	UserID         *int64    `json:"user_id"`
	UserName       string    `json:"user_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func ListConversations(db *sql.DB, status string) ([]Conversation, error) {
	query := `
		SELECT c.id, c.subject, c.contact_id,
		       COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
		       c.peer_email, c.status, c.unread, c.last_message_at,
		       COALESCE((SELECT LEFT(m.body, 120) FROM conversation_messages m
		                 WHERE m.conversation_id = c.id ORDER BY m.created_at DESC LIMIT 1), ''),
		       c.created_at
		FROM conversations c
		LEFT JOIN contacts ct ON ct.id = c.contact_id`
	args := []any{}
	if status != "" {
		query += ` WHERE c.status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY c.last_message_at DESC LIMIT 200`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Subject, &c.ContactID, &c.ContactName, &c.PeerEmail,
			&c.Status, &c.Unread, &c.LastMessageAt, &c.LastPreview, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.ContactName = strings.TrimSpace(c.ContactName)
		list = append(list, c)
	}
	return list, rows.Err()
}

func ConversationByID(db *sql.DB, id int64) (*Conversation, error) {
	var c Conversation
	err := db.QueryRow(`
		SELECT c.id, c.subject, c.contact_id,
		       COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
		       c.peer_email, c.status, c.unread, c.last_message_at, c.created_at
		FROM conversations c
		LEFT JOIN contacts ct ON ct.id = c.contact_id
		WHERE c.id = $1`, id,
	).Scan(&c.ID, &c.Subject, &c.ContactID, &c.ContactName, &c.PeerEmail,
		&c.Status, &c.Unread, &c.LastMessageAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.ContactName = strings.TrimSpace(c.ContactName)
	return &c, nil
}

func ConversationMessages(db *sql.DB, conversationID int64) ([]ConversationMessage, error) {
	rows, err := db.Query(`
		SELECT m.id, m.conversation_id, m.direction, m.from_email, m.to_email,
		       COALESCE(m.subject,''), m.body, m.user_id, COALESCE(u.name,''), m.created_at
		FROM conversation_messages m
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.conversation_id = $1
		ORDER BY m.created_at`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []ConversationMessage{}
	for rows.Next() {
		var m ConversationMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Direction, &m.FromEmail, &m.ToEmail,
			&m.Subject, &m.Body, &m.UserID, &m.UserName, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// NormalizeSubject remove prefixos de resposta/encaminhamento para agrupar o thread.
func NormalizeSubject(subject string) string {
	s := strings.TrimSpace(subject)
	for {
		lower := strings.ToLower(s)
		trimmed := false
		for _, prefix := range []string{"re:", "res:", "fw:", "fwd:", "enc:"} {
			if strings.HasPrefix(lower, prefix) {
				s = strings.TrimSpace(s[len(prefix):])
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	if s == "" {
		s = "(sem assunto)"
	}
	return s
}

// FindOrCreateConversation localiza a conversa aberta do mesmo remetente/assunto
// ou cria uma nova, vinculando ao contato pelo e-mail quando existir.
func FindOrCreateConversation(db *sql.DB, peerEmail, subject string) (*Conversation, error) {
	peerEmail = NormalizeEmail(peerEmail)
	subject = NormalizeSubject(subject)

	var id int64
	err := db.QueryRow(`
		SELECT id FROM conversations
		WHERE peer_email = $1 AND subject = $2 AND status = 'aberta'
		ORDER BY last_message_at DESC LIMIT 1`, peerEmail, subject).Scan(&id)
	if err == nil {
		return ConversationByID(db, id)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	var contactID *int64
	var cid int64
	err = db.QueryRow(`SELECT id FROM contacts WHERE email = $1 LIMIT 1`, peerEmail).Scan(&cid)
	if err == nil {
		contactID = &cid
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	c := &Conversation{Subject: subject, PeerEmail: peerEmail, ContactID: contactID, Status: "aberta", Unread: true}
	err = db.QueryRow(`
		INSERT INTO conversations (subject, contact_id, peer_email)
		VALUES ($1, $2, $3)
		RETURNING id, last_message_at, created_at`,
		c.Subject, c.ContactID, c.PeerEmail,
	).Scan(&c.ID, &c.LastMessageAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// AddConversationMessage grava a mensagem e atualiza o resumo da conversa.
func AddConversationMessage(db *sql.DB, m *ConversationMessage) error {
	err := db.QueryRow(`
		INSERT INTO conversation_messages (conversation_id, direction, from_email, to_email, subject, body, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		m.ConversationID, m.Direction, m.FromEmail, m.ToEmail, m.Subject, m.Body, m.UserID,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return err
	}

	unread := m.Direction == "recebida"
	_, err = db.Exec(`
		UPDATE conversations
		SET last_message_at = NOW(), status = 'aberta',
		    unread = CASE WHEN $1 THEN TRUE ELSE unread END
		WHERE id = $2`, unread, m.ConversationID)
	return err
}

func MarkConversationRead(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE conversations SET unread = FALSE WHERE id = $1`, id)
	return err
}

func SetConversationStatus(db *sql.DB, id int64, status string) error {
	_, err := db.Exec(`UPDATE conversations SET status = $1 WHERE id = $2`, status, id)
	return err
}

func CountUnreadConversations(db *sql.DB) (int, error) {
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE unread = TRUE AND status = 'aberta'`).Scan(&total)
	return total, err
}
