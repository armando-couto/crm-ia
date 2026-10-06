package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Origem do e-mail rastreado.
const (
	EmailSourceManual     = "manual"
	EmailSourceAutomation = "automacao"
)

// Tipos de evento rastreado.
const (
	EmailEventOpen  = "abertura"
	EmailEventClick = "clique"
)

// EmailMessage é um e-mail enviado pelo CRM, com os contadores de rastreio.
type EmailMessage struct {
	ID           int64      `json:"id"`
	Token        string     `json:"-"`
	Subject      string     `json:"subject"`
	ToEmail      string     `json:"to_email"`
	Body         string     `json:"body,omitempty"`
	ContactID    *int64     `json:"contact_id"`
	ContactName  string     `json:"contact_name,omitempty"`
	DealID       *int64     `json:"deal_id"`
	UserID       *int64     `json:"user_id"`
	UserName     string     `json:"user_name,omitempty"`
	Source       string     `json:"source"`
	Opens        int        `json:"opens"`
	Clicks       int        `json:"clicks"`
	FirstOpenAt  *time.Time `json:"first_open_at"`
	LastOpenAt   *time.Time `json:"last_open_at"`
	FirstClickAt *time.Time `json:"first_click_at"`
	SentAt       time.Time  `json:"sent_at"`
}

type EmailEvent struct {
	ID        int64     `json:"id"`
	MessageID int64     `json:"message_id"`
	Kind      string    `json:"kind"`
	URL       string    `json:"url,omitempty"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func CreateEmailMessage(db *sql.DB, m *EmailMessage) error {
	if m.Source == "" {
		m.Source = EmailSourceManual
	}
	return db.QueryRow(`
		INSERT INTO email_messages (token, subject, to_email, body, contact_id, deal_id, user_id, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, sent_at`,
		m.Token, m.Subject, m.ToEmail, m.Body, m.ContactID, m.DealID, m.UserID, m.Source,
	).Scan(&m.ID, &m.SentAt)
}

const emailMessageSelect = `
	SELECT m.id, m.subject, m.to_email, m.contact_id,
	       COALESCE(TRIM(c.first_name || ' ' || c.last_name), ''), m.deal_id, m.user_id,
	       COALESCE(u.name, ''), m.source, m.opens, m.clicks,
	       m.first_open_at, m.last_open_at, m.first_click_at, m.sent_at
	FROM email_messages m
	LEFT JOIN contacts c ON c.id = m.contact_id
	LEFT JOIN users u ON u.id = m.user_id`

func scanEmailMessage(row interface{ Scan(...any) error }) (*EmailMessage, error) {
	var m EmailMessage
	err := row.Scan(&m.ID, &m.Subject, &m.ToEmail, &m.ContactID, &m.ContactName, &m.DealID,
		&m.UserID, &m.UserName, &m.Source, &m.Opens, &m.Clicks,
		&m.FirstOpenAt, &m.LastOpenAt, &m.FirstClickAt, &m.SentAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

type EmailFilter struct {
	ContactID int64
	DealID    int64
	UserID    int64
	Days      int
	Limit     int
}

func ListEmailMessages(db *sql.DB, f EmailFilter) ([]EmailMessage, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ContactID > 0 {
		add("m.contact_id = $%d", f.ContactID)
	}
	if f.DealID > 0 {
		add("m.deal_id = $%d", f.DealID)
	}
	if f.UserID > 0 {
		add("m.user_id = $%d", f.UserID)
	}
	if f.Days > 0 {
		add("m.sent_at >= NOW() - make_interval(days => $%d)", f.Days)
	}
	args = append(args, f.Limit)

	query := fmt.Sprintf("%s WHERE %s ORDER BY m.sent_at DESC LIMIT $%d",
		emailMessageSelect, strings.Join(where, " AND "), len(args))
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []EmailMessage{}
	for rows.Next() {
		m, err := scanEmailMessage(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *m)
	}
	return list, rows.Err()
}

func EmailMessageByID(db *sql.DB, id int64) (*EmailMessage, error) {
	return scanEmailMessage(db.QueryRow(emailMessageSelect+" WHERE m.id = $1", id))
}

// EmailMessageIDByToken resolve o token público do pixel/redirect. Devolver só
// o ID evita carregar o e-mail inteiro em cada abertura.
func EmailMessageIDByToken(db *sql.DB, token string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM email_messages WHERE token = $1`, token).Scan(&id)
	return id, err
}

// RecordEmailOpen conta uma abertura e guarda o evento.
func RecordEmailOpen(db *sql.DB, messageID int64, ip, userAgent string) error {
	if _, err := db.Exec(`
		UPDATE email_messages
		SET opens = opens + 1,
		    first_open_at = COALESCE(first_open_at, NOW()),
		    last_open_at = NOW()
		WHERE id = $1`, messageID); err != nil {
		return err
	}
	return recordEmailEvent(db, messageID, EmailEventOpen, "", ip, userAgent)
}

// RecordEmailClick conta um clique (que também vale como abertura do e-mail).
func RecordEmailClick(db *sql.DB, messageID int64, url, ip, userAgent string) error {
	if _, err := db.Exec(`
		UPDATE email_messages
		SET clicks = clicks + 1,
		    first_click_at = COALESCE(first_click_at, NOW()),
		    first_open_at = COALESCE(first_open_at, NOW()),
		    last_open_at = NOW()
		WHERE id = $1`, messageID); err != nil {
		return err
	}
	return recordEmailEvent(db, messageID, EmailEventClick, url, ip, userAgent)
}

func recordEmailEvent(db *sql.DB, messageID int64, kind, url, ip, userAgent string) error {
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	_, err := db.Exec(`
		INSERT INTO email_events (message_id, kind, url, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5)`, messageID, kind, url, ip, userAgent)
	return err
}

func ListEmailEvents(db *sql.DB, messageID int64) ([]EmailEvent, error) {
	rows, err := db.Query(`
		SELECT id, message_id, kind, url, ip, user_agent, created_at
		FROM email_events WHERE message_id = $1 ORDER BY created_at DESC LIMIT 100`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []EmailEvent{}
	for rows.Next() {
		var e EmailEvent
		if err := rows.Scan(&e.ID, &e.MessageID, &e.Kind, &e.URL, &e.IP,
			&e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// EmailStats resume o desempenho dos envios no período.
type EmailStats struct {
	Sent      int     `json:"sent"`
	Opened    int     `json:"opened"`
	Clicked   int     `json:"clicked"`
	OpenRate  float64 `json:"open_rate"`
	ClickRate float64 `json:"click_rate"`
}

func LoadEmailStats(db *sql.DB, days int) (*EmailStats, error) {
	if days <= 0 {
		days = 30
	}
	var s EmailStats
	err := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE opens > 0),
		       COUNT(*) FILTER (WHERE clicks > 0)
		FROM email_messages
		WHERE sent_at >= NOW() - make_interval(days => $1)`, days,
	).Scan(&s.Sent, &s.Opened, &s.Clicked)
	if err != nil {
		return nil, err
	}
	if s.Sent > 0 {
		s.OpenRate = float64(s.Opened) / float64(s.Sent) * 100
		s.ClickRate = float64(s.Clicked) / float64(s.Sent) * 100
	}
	return &s, nil
}
