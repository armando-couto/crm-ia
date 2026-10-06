package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Tipos de etapa da sequência. A automática roda sozinha; as manuais viram
// tarefa na fila de quem vende e seguram a cadência até serem concluídas.
const (
	StepEmailAuto    = "email_auto"
	StepTaskEmail    = "task_email"
	StepTaskCall     = "task_call"
	StepTaskGeneral  = "task_general"
	StepTaskLinkedIn = "task_linkedin"
)

var sequenceStepKinds = []string{
	StepEmailAuto, StepTaskEmail, StepTaskCall, StepTaskGeneral, StepTaskLinkedIn,
}

// StepLabels alimenta o construtor de sequências.
var StepLabels = map[string]string{
	StepEmailAuto:    "E-mail automático",
	StepTaskEmail:    "Tarefa de e-mail manual",
	StepTaskCall:     "Tarefa de chamada",
	StepTaskGeneral:  "Tarefa geral",
	StepTaskLinkedIn: "Tarefa no LinkedIn",
}

// IsManualStep informa se a etapa exige ação de uma pessoa.
func IsManualStep(kind string) bool {
	return kind != StepEmailAuto
}

// Status da inscrição.
const (
	MemberActive  = "ativa"
	MemberWaiting = "aguardando_tarefa"
	MemberDone    = "concluida"
	MemberExited  = "cancelada"
)

// SequenceStep é uma etapa da cadência.
type SequenceStep struct {
	Kind      string `json:"kind"`
	DelayDays int    `json:"delay_days"`
	// E-mail automático:
	Subject    string `json:"subject,omitempty"`
	Body       string `json:"body,omitempty"`
	TemplateID int64  `json:"template_id,omitempty"`
	// Tarefas manuais:
	Title string `json:"title,omitempty"`
	Note  string `json:"note,omitempty"`
}

type Sequence struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Steps         []SequenceStep `json:"steps"`
	Active        bool           `json:"active"`
	Dynamic       bool           `json:"dynamic"`
	ExitOnReply   bool           `json:"exit_on_reply"`
	ExitOnMeeting bool           `json:"exit_on_meeting"`
	OwnerID       *int64         `json:"owner_id"`
	OwnerName     string         `json:"owner_name,omitempty"`
	CreatedBy     *int64         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`

	// Métricas da listagem.
	Enrolled  int     `json:"enrolled"`
	Active_   int     `json:"active_members"`
	OpenRate  float64 `json:"open_rate"`
	ReplyRate float64 `json:"reply_rate"`
}

// SequenceMember é a inscrição de um contato na cadência.
type SequenceMember struct {
	ID          int64      `json:"id"`
	SequenceID  int64      `json:"sequence_id"`
	ContactID   int64      `json:"contact_id"`
	ContactName string     `json:"contact_name,omitempty"`
	ContactMail string     `json:"contact_email,omitempty"`
	Step        int        `json:"step"`
	StepLabel   string     `json:"step_label,omitempty"`
	NextRunAt   time.Time  `json:"next_run_at"`
	Status      string     `json:"status"`
	ExitReason  string     `json:"exit_reason"`
	TaskID      *int64     `json:"task_id"`
	Engaged     bool       `json:"engaged"`
	EnrolledAt  time.Time  `json:"enrolled_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

// ValidateSequence confere nome e etapas.
func ValidateSequence(s *Sequence) error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("informe o nome da sequência")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("a sequência precisa de pelo menos uma etapa")
	}
	if len(s.Steps) > 30 {
		return fmt.Errorf("no máximo 30 etapas por sequência")
	}

	temManual := false
	for i := range s.Steps {
		step := &s.Steps[i]
		if !slices.Contains(sequenceStepKinds, step.Kind) {
			return fmt.Errorf("etapa inválida: %s", step.Kind)
		}
		if step.DelayDays < 0 || step.DelayDays > 365 {
			return fmt.Errorf("o intervalo da etapa %d deve ficar entre 0 e 365 dias", i+1)
		}
		if step.Kind == StepEmailAuto {
			if step.TemplateID == 0 && (strings.TrimSpace(step.Subject) == "" ||
				strings.TrimSpace(step.Body) == "") {
				return fmt.Errorf("a etapa %d precisa de assunto e mensagem, ou de um modelo", i+1)
			}
		} else {
			temManual = true
			if strings.TrimSpace(step.Title) == "" {
				step.Title = StepLabels[step.Kind]
			}
		}
	}

	// A sequência dinâmica troca para as manuais quando o contato engaja: sem
	// nenhuma etapa manual ela não teria para onde ir.
	if s.Dynamic && !temManual {
		return fmt.Errorf("a sequência dinâmica precisa de pelo menos uma etapa manual")
	}
	return nil
}

const sequenceSelect = `
	SELECT s.id, s.name, s.description, s.steps, s.active, s.dynamic,
	       s.exit_on_reply, s.exit_on_meeting, s.owner_id, COALESCE(u.name,''),
	       s.created_by, s.created_at, s.updated_at
	FROM sequences s
	LEFT JOIN users u ON u.id = s.owner_id`

func scanSequence(row interface{ Scan(...any) error }) (*Sequence, error) {
	var s Sequence
	var steps []byte
	err := row.Scan(&s.ID, &s.Name, &s.Description, &steps, &s.Active, &s.Dynamic,
		&s.ExitOnReply, &s.ExitOnMeeting, &s.OwnerID, &s.OwnerName,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Steps = []SequenceStep{}
	if len(steps) > 0 {
		if err := json.Unmarshal(steps, &s.Steps); err != nil {
			return nil, err
		}
	}
	return &s, nil
}

// ListSequences devolve as sequências com inscritos, taxa de abertura e de
// resposta — as colunas que a equipe usa para comparar cadências.
func ListSequences(db *sql.DB) ([]Sequence, error) {
	rows, err := db.Query(sequenceSelect + ` ORDER BY s.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Sequence{}
	for rows.Next() {
		s, err := scanSequence(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range list {
		if err := loadSequenceMetrics(db, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// loadSequenceMetrics conta inscritos e mede abertura e resposta dos e-mails
// que a sequência mandou.
func loadSequenceMetrics(db *sql.DB, s *Sequence) error {
	if err := db.QueryRow(`
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status IN ('ativa','aguardando_tarefa'))
		FROM sequence_members WHERE sequence_id = $1`, s.ID,
	).Scan(&s.Enrolled, &s.Active_); err != nil {
		return err
	}

	var sent, opened, replied int
	if err := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE opens > 0),
		       COUNT(*) FILTER (WHERE replied_at IS NOT NULL)
		FROM email_messages WHERE sequence_id = $1`, s.ID,
	).Scan(&sent, &opened, &replied); err != nil {
		return err
	}
	if sent > 0 {
		s.OpenRate = float64(opened) / float64(sent) * 100
		s.ReplyRate = float64(replied) / float64(sent) * 100
	}
	return nil
}

func SequenceByID(db *sql.DB, id int64) (*Sequence, error) {
	s, err := scanSequence(db.QueryRow(sequenceSelect+` WHERE s.id = $1`, id))
	if err != nil {
		return nil, err
	}
	if err := loadSequenceMetrics(db, s); err != nil {
		return nil, err
	}
	return s, nil
}

func CreateSequence(db *sql.DB, s *Sequence) error {
	steps, err := json.Marshal(s.Steps)
	if err != nil {
		return err
	}
	return db.QueryRow(`
		INSERT INTO sequences (name, description, steps, active, dynamic,
		    exit_on_reply, exit_on_meeting, owner_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at`,
		s.Name, s.Description, steps, s.Active, s.Dynamic,
		s.ExitOnReply, s.ExitOnMeeting, s.OwnerID, s.CreatedBy,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func UpdateSequence(db *sql.DB, s *Sequence) error {
	steps, err := json.Marshal(s.Steps)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE sequences SET name = $1, description = $2, steps = $3, active = $4,
		    dynamic = $5, exit_on_reply = $6, exit_on_meeting = $7, owner_id = $8,
		    updated_at = NOW()
		WHERE id = $9`,
		s.Name, s.Description, steps, s.Active, s.Dynamic,
		s.ExitOnReply, s.ExitOnMeeting, s.OwnerID, s.ID)
	return err
}

func DeleteSequence(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM sequences WHERE id = $1`, id)
	return err
}

// ===== Inscrições =====

// EnrollContact inscreve o contato na sequência, começando pela primeira etapa.
// O mesmo contato não entra duas vezes na mesma cadência.
func EnrollContact(db *sql.DB, sequenceID, contactID int64, enrolledBy *int64) error {
	contact, err := ContactByID(db, contactID)
	if err != nil {
		return err
	}
	if contact.Email == "" {
		return fmt.Errorf("o contato precisa de e-mail para entrar na sequência")
	}

	_, err = db.Exec(`
		INSERT INTO sequence_members (sequence_id, contact_id, step, next_run_at, status, enrolled_by)
		VALUES ($1, $2, 0, NOW(), 'ativa', $3)
		ON CONFLICT (sequence_id, contact_id) DO UPDATE
		SET step = 0, next_run_at = NOW(), status = 'ativa', exit_reason = '',
		    engaged = FALSE, task_id = NULL, finished_at = NULL, enrolled_at = NOW()`,
		sequenceID, contactID, enrolledBy)
	return err
}

const memberSelect = `
	SELECT m.id, m.sequence_id, m.contact_id,
	       COALESCE(TRIM(c.first_name || ' ' || c.last_name), ''), COALESCE(c.email,''),
	       m.step, m.next_run_at, m.status, m.exit_reason, m.task_id, m.engaged,
	       m.enrolled_at, m.finished_at
	FROM sequence_members m
	JOIN contacts c ON c.id = m.contact_id`

func scanMember(row interface{ Scan(...any) error }) (*SequenceMember, error) {
	var m SequenceMember
	err := row.Scan(&m.ID, &m.SequenceID, &m.ContactID, &m.ContactName, &m.ContactMail,
		&m.Step, &m.NextRunAt, &m.Status, &m.ExitReason, &m.TaskID, &m.Engaged,
		&m.EnrolledAt, &m.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMembers devolve os inscritos da sequência com a etapa em que estão.
func ListMembers(db *sql.DB, sequenceID int64, status string) ([]SequenceMember, error) {
	query := memberSelect + ` WHERE m.sequence_id = $1`
	args := []any{sequenceID}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND m.status = $%d", len(args))
	}
	query += " ORDER BY m.enrolled_at DESC LIMIT 500"

	// A sequência vem antes de abrir as rows: buscar com o cursor aberto
	// seguraria uma segunda conexão e trava com pool de uma conexão só.
	seq, err := SequenceByID(db, sequenceID)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []SequenceMember{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		if m.Step < len(seq.Steps) {
			m.StepLabel = StepLabels[seq.Steps[m.Step].Kind]
		}
		list = append(list, *m)
	}
	return list, rows.Err()
}

// DueMembers devolve as inscrições prontas para avançar.
func DueMembers(db *sql.DB, now time.Time, limit int) ([]SequenceMember, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(memberSelect+`
		WHERE m.status = 'ativa' AND m.next_run_at <= $1
		ORDER BY m.next_run_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []SequenceMember{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *m)
	}
	return list, rows.Err()
}

func MemberByID(db *sql.DB, id int64) (*SequenceMember, error) {
	return scanMember(db.QueryRow(memberSelect+` WHERE m.id = $1`, id))
}

// MemberByTask acha a inscrição segurada por uma tarefa manual.
func MemberByTask(db *sql.DB, taskID int64) (*SequenceMember, error) {
	return scanMember(db.QueryRow(memberSelect+` WHERE m.task_id = $1`, taskID))
}

// AdvanceMember marca a próxima etapa e quando ela deve rodar.
func AdvanceMember(db *sql.DB, memberID int64, step int, runAt time.Time) error {
	_, err := db.Exec(`
		UPDATE sequence_members
		SET step = $1, next_run_at = $2, status = 'ativa', task_id = NULL
		WHERE id = $3`, step, runAt, memberID)
	return err
}

// HoldForTask trava a inscrição até a tarefa manual ser concluída.
func HoldForTask(db *sql.DB, memberID, taskID int64) error {
	_, err := db.Exec(`
		UPDATE sequence_members SET status = 'aguardando_tarefa', task_id = $1 WHERE id = $2`,
		taskID, memberID)
	return err
}

// FinishMember encerra a inscrição (fim da cadência ou saída por regra).
func FinishMember(db *sql.DB, memberID int64, status, reason string) error {
	_, err := db.Exec(`
		UPDATE sequence_members
		SET status = $1, exit_reason = $2, finished_at = NOW(), task_id = NULL
		WHERE id = $3`, status, reason, memberID)
	return err
}

// MarkEngaged registra que o contato interagiu (abriu, clicou ou respondeu).
func MarkEngaged(db *sql.DB, contactID int64) error {
	_, err := db.Exec(`
		UPDATE sequence_members SET engaged = TRUE
		WHERE contact_id = $1 AND status IN ('ativa','aguardando_tarefa')`, contactID)
	return err
}

// ExitByRule tira o contato das sequências ativas que têm a regra ligada.
// Devolve quantas inscrições foram encerradas.
func ExitByRule(db *sql.DB, contactID int64, rule string) (int64, error) {
	column := "exit_on_reply"
	if rule == "reuniao" {
		column = "exit_on_meeting"
	}

	result, err := db.Exec(fmt.Sprintf(`
		UPDATE sequence_members m
		SET status = 'cancelada', exit_reason = $1, finished_at = NOW(), task_id = NULL
		FROM sequences s
		WHERE m.sequence_id = s.id
		  AND m.contact_id = $2
		  AND m.status IN ('ativa','aguardando_tarefa')
		  AND s.%s = TRUE`, column), rule, contactID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// FirstManualStep devolve o índice da primeira etapa manual a partir de um
// ponto — usado pela sequência dinâmica quando o contato engaja.
func FirstManualStep(steps []SequenceStep, from int) int {
	for i := from; i < len(steps); i++ {
		if IsManualStep(steps[i].Kind) {
			return i
		}
	}
	return -1
}

// MarkEmailReplied liga a resposta ao último e-mail que a sequência mandou,
// para a taxa de resposta sair certa.
func MarkEmailReplied(db *sql.DB, contactID int64) error {
	_, err := db.Exec(`
		UPDATE email_messages
		SET replied_at = NOW()
		WHERE id = (
		    SELECT id FROM email_messages
		    WHERE contact_id = $1 AND sequence_id IS NOT NULL AND replied_at IS NULL
		    ORDER BY sent_at DESC LIMIT 1)`, contactID)
	return err
}
