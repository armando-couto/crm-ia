package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// Gatilhos disponíveis. Os "por evento" são disparados pelos controllers no
// momento da ação; os "por tempo" são varridos periodicamente pelo worker.
const (
	TriggerContactCreated = "contato_criado"
	TriggerContactStage   = "contato_estagio"
	TriggerDealCreated    = "negocio_criado"
	TriggerDealStage      = "negocio_etapa"
	TriggerDealWon        = "negocio_ganho"
	TriggerDealLost       = "negocio_perdido"
	TriggerTicketCreated  = "ticket_criado"
	TriggerFormSubmitted  = "formulario_enviado"

	// Por tempo (worker).
	TriggerDealIdle    = "negocio_parado"
	TriggerTaskOverdue = "tarefa_atrasada"
	TriggerContactIdle = "contato_sem_contato"
)

// TimeTriggers são os gatilhos varridos pelo worker, não por evento.
var TimeTriggers = []string{TriggerDealIdle, TriggerTaskOverdue, TriggerContactIdle}

// Ações disponíveis.
const (
	ActionSendEmail    = "enviar_email"
	ActionCreateTask   = "criar_tarefa"
	ActionSetOwner     = "mudar_dono"
	ActionSetStage     = "mudar_etapa"
	ActionSetLifecycle = "mudar_estagio"
	ActionAddToList    = "adicionar_lista"
	ActionAddNote      = "adicionar_nota"
	ActionNotifyUser   = "notificar"
	ActionWait         = "aguardar"
)

var automationTriggers = []string{
	TriggerContactCreated, TriggerContactStage, TriggerDealCreated, TriggerDealStage,
	TriggerDealWon, TriggerDealLost, TriggerTicketCreated, TriggerFormSubmitted,
	TriggerDealIdle, TriggerTaskOverdue, TriggerContactIdle,
}

var automationActions = []string{
	ActionSendEmail, ActionCreateTask, ActionSetOwner, ActionSetStage,
	ActionSetLifecycle, ActionAddToList, ActionAddNote, ActionNotifyUser, ActionWait,
}

// TriggerLabels e ActionLabels alimentam a tela de automações.
var TriggerLabels = map[string]string{
	TriggerContactCreated: "Contato criado",
	TriggerContactStage:   "Contato mudou de estágio",
	TriggerDealCreated:    "Negócio criado",
	TriggerDealStage:      "Negócio mudou de etapa",
	TriggerDealWon:        "Negócio ganho",
	TriggerDealLost:       "Negócio perdido",
	TriggerTicketCreated:  "Ticket aberto",
	TriggerFormSubmitted:  "Formulário enviado",
	TriggerDealIdle:       "Negócio parado há X dias",
	TriggerTaskOverdue:    "Tarefa atrasada há X dias",
	TriggerContactIdle:    "Contato sem interação há X dias",
}

var ActionLabels = map[string]string{
	ActionSendEmail:    "Enviar e-mail",
	ActionCreateTask:   "Criar tarefa",
	ActionSetOwner:     "Mudar o dono",
	ActionSetStage:     "Mover de etapa",
	ActionSetLifecycle: "Mudar o estágio do contato",
	ActionAddToList:    "Adicionar à lista",
	ActionAddNote:      "Registrar observação",
	ActionNotifyUser:   "Avisar alguém da equipe",
	ActionWait:         "Aguardar X dias",
}

// AutomationAction é um passo da automação. Config carrega os parâmetros
// específicos de cada tipo (template_id, stage_id, days...).
type AutomationAction struct {
	Kind   string          `json:"kind"`
	Config json.RawMessage `json:"config,omitempty"`
}

type Automation struct {
	ID            int64              `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	TriggerKind   string             `json:"trigger_kind"`
	TriggerConfig json.RawMessage    `json:"trigger_config"`
	Actions       []AutomationAction `json:"actions"`
	Active        bool               `json:"active"`
	Runs          int                `json:"runs"`
	LastRunAt     *time.Time         `json:"last_run_at"`
	CreatedBy     *int64             `json:"created_by"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type AutomationRun struct {
	ID           int64     `json:"id"`
	AutomationID int64     `json:"automation_id"`
	Entity       string    `json:"entity"`
	EntityID     *int64    `json:"entity_id"`
	Status       string    `json:"status"`
	Detail       string    `json:"detail"`
	CreatedAt    time.Time `json:"created_at"`
}

// SequenceEnrollment guarda onde o contato parou numa sequência com espera.
type SequenceEnrollment struct {
	ID           int64     `json:"id"`
	AutomationID int64     `json:"automation_id"`
	ContactID    int64     `json:"contact_id"`
	DealID       *int64    `json:"deal_id"`
	Step         int       `json:"step"`
	NextRunAt    time.Time `json:"next_run_at"`
	Status       string    `json:"status"`
}

func ValidAutomationTrigger(kind string) bool {
	return slices.Contains(automationTriggers, kind)
}

func IsTimeTrigger(kind string) bool {
	return slices.Contains(TimeTriggers, kind)
}

// ValidateAutomation confere gatilho, ações e a ordem dos passos.
func ValidateAutomation(a *Automation) error {
	if a.Name == "" {
		return fmt.Errorf("informe o nome da automação")
	}
	if !ValidAutomationTrigger(a.TriggerKind) {
		return fmt.Errorf("gatilho inválido")
	}
	if len(a.Actions) == 0 {
		return fmt.Errorf("adicione pelo menos uma ação")
	}
	if len(a.Actions) > 20 {
		return fmt.Errorf("no máximo 20 ações por automação")
	}
	for i, action := range a.Actions {
		if !slices.Contains(automationActions, action.Kind) {
			return fmt.Errorf("ação inválida: %s", action.Kind)
		}
		// Terminar em "aguardar" deixa a sequência parada sem fazer nada.
		if action.Kind == ActionWait && i == len(a.Actions)-1 {
			return fmt.Errorf("a última ação não pode ser uma espera")
		}
	}
	return nil
}

const automationSelect = `
	SELECT id, name, description, trigger_kind, trigger_config, actions, active,
	       runs, last_run_at, created_by, created_at, updated_at
	FROM automations`

func scanAutomation(row interface{ Scan(...any) error }) (*Automation, error) {
	var a Automation
	var trigger, actions []byte
	err := row.Scan(&a.ID, &a.Name, &a.Description, &a.TriggerKind, &trigger, &actions,
		&a.Active, &a.Runs, &a.LastRunAt, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.TriggerConfig = json.RawMessage(trigger)
	if len(actions) > 0 {
		if err := json.Unmarshal(actions, &a.Actions); err != nil {
			return nil, err
		}
	}
	return &a, nil
}

func ListAutomations(db *sql.DB) ([]Automation, error) {
	rows, err := db.Query(automationSelect + ` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Automation{}
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, rows.Err()
}

// ActiveAutomationsByTrigger devolve as automações ligadas para um gatilho.
func ActiveAutomationsByTrigger(db *sql.DB, kind string) ([]Automation, error) {
	rows, err := db.Query(automationSelect+
		` WHERE trigger_kind = $1 AND active = TRUE ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Automation{}
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, rows.Err()
}

func AutomationByID(db *sql.DB, id int64) (*Automation, error) {
	return scanAutomation(db.QueryRow(automationSelect+` WHERE id = $1`, id))
}

func CreateAutomation(db *sql.DB, a *Automation) error {
	actions, err := json.Marshal(a.Actions)
	if err != nil {
		return err
	}
	if len(a.TriggerConfig) == 0 {
		a.TriggerConfig = json.RawMessage(`{}`)
	}
	return db.QueryRow(`
		INSERT INTO automations (name, description, trigger_kind, trigger_config, actions, active, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at, updated_at`,
		a.Name, a.Description, a.TriggerKind, string(a.TriggerConfig), actions, a.Active, a.CreatedBy,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func UpdateAutomation(db *sql.DB, a *Automation) error {
	actions, err := json.Marshal(a.Actions)
	if err != nil {
		return err
	}
	if len(a.TriggerConfig) == 0 {
		a.TriggerConfig = json.RawMessage(`{}`)
	}
	_, err = db.Exec(`
		UPDATE automations SET name = $1, description = $2, trigger_kind = $3,
		    trigger_config = $4, actions = $5, active = $6, updated_at = NOW()
		WHERE id = $7`,
		a.Name, a.Description, a.TriggerKind, string(a.TriggerConfig), actions, a.Active, a.ID)
	return err
}

func DeleteAutomation(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM automations WHERE id = $1`, id)
	return err
}

// MarkAutomationRun soma uma execução e guarda o histórico.
func MarkAutomationRun(db *sql.DB, run *AutomationRun) error {
	if _, err := db.Exec(`
		UPDATE automations SET runs = runs + 1, last_run_at = NOW() WHERE id = $1`,
		run.AutomationID); err != nil {
		return err
	}
	if len(run.Detail) > 500 {
		run.Detail = run.Detail[:500]
	}
	_, err := db.Exec(`
		INSERT INTO automation_runs (automation_id, entity, entity_id, status, detail)
		VALUES ($1, $2, $3, $4, $5)`,
		run.AutomationID, run.Entity, run.EntityID, run.Status, run.Detail)
	return err
}

func ListAutomationRuns(db *sql.DB, automationID int64, limit int) ([]AutomationRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, automation_id, entity, entity_id, status, detail, created_at
		FROM automation_runs WHERE automation_id = $1 ORDER BY created_at DESC LIMIT $2`,
		automationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AutomationRun{}
	for rows.Next() {
		var r AutomationRun
		if err := rows.Scan(&r.ID, &r.AutomationID, &r.Entity, &r.EntityID,
			&r.Status, &r.Detail, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// EnrollInSequence agenda a retomada da automação a partir do passo informado.
// O mesmo contato não entra duas vezes na mesma automação.
func EnrollInSequence(db *sql.DB, automationID, contactID int64, dealID *int64, step int, runAt time.Time) error {
	_, err := db.Exec(`
		INSERT INTO sequence_enrollments (automation_id, contact_id, deal_id, step, next_run_at, status)
		VALUES ($1, $2, $3, $4, $5, 'ativa')
		ON CONFLICT (automation_id, contact_id) DO UPDATE
		SET step = EXCLUDED.step, next_run_at = EXCLUDED.next_run_at,
		    deal_id = EXCLUDED.deal_id, status = 'ativa', updated_at = NOW()`,
		automationID, contactID, dealID, step, runAt)
	return err
}

// DueEnrollments devolve as sequências que já podem retomar.
func DueEnrollments(db *sql.DB, now time.Time, limit int) ([]SequenceEnrollment, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT id, automation_id, contact_id, deal_id, step, next_run_at, status
		FROM sequence_enrollments
		WHERE status = 'ativa' AND next_run_at <= $1
		ORDER BY next_run_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []SequenceEnrollment{}
	for rows.Next() {
		var e SequenceEnrollment
		if err := rows.Scan(&e.ID, &e.AutomationID, &e.ContactID, &e.DealID,
			&e.Step, &e.NextRunAt, &e.Status); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func FinishEnrollment(db *sql.DB, id int64, status string) error {
	_, err := db.Exec(`
		UPDATE sequence_enrollments SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id)
	return err
}

// CountActiveEnrollments informa quantos contatos estão no meio da sequência.
func CountActiveEnrollments(db *sql.DB, automationID int64) (int, error) {
	var total int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM sequence_enrollments
		WHERE automation_id = $1 AND status = 'ativa'`, automationID).Scan(&total)
	return total, err
}

// ===== Consultas dos gatilhos por tempo =====

// IdleDeals devolve negócios abertos sem atividade há N dias.
func IdleDeals(db *sql.DB, days int) ([]Deal, error) {
	rows, err := db.Query(dealSelect+`
		WHERE d.status = 'aberto'
		  AND NOT EXISTS (
		      SELECT 1 FROM activities a
		      WHERE a.deal_id = d.id AND a.created_at >= NOW() - make_interval(days => $1))
		  AND d.created_at < NOW() - make_interval(days => $1)
		LIMIT 200`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Deal{}
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, rows.Err()
}

// IdleContacts devolve contatos sem nenhuma atividade há N dias.
func IdleContacts(db *sql.DB, days int) ([]Contact, error) {
	rows, err := db.Query(contactSelect+`
		WHERE NOT EXISTS (
		      SELECT 1 FROM activities a
		      WHERE a.contact_id = c.id AND a.created_at >= NOW() - make_interval(days => $1))
		  AND c.created_at < NOW() - make_interval(days => $1)
		LIMIT 200`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

// OverdueTasks devolve tarefas em aberto vencidas há pelo menos N dias.
func OverdueTasks(db *sql.DB, days int) ([]Task, error) {
	rows, err := db.Query(taskSelect+`
		WHERE t.completed_at IS NULL
		  AND t.due_date IS NOT NULL
		  AND t.due_date <= NOW() - make_interval(days => $1)
		LIMIT 200`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *task)
	}
	return list, rows.Err()
}

// AlreadyRanFor evita repetir a mesma automação no mesmo registro dentro da
// janela (os gatilhos por tempo rodam várias vezes ao dia).
func AlreadyRanFor(db *sql.DB, automationID int64, entity string, entityID int64, within time.Duration) (bool, error) {
	var total int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM automation_runs
		WHERE automation_id = $1 AND entity = $2 AND entity_id = $3
		  AND created_at >= $4`,
		automationID, entity, entityID, time.Now().Add(-within)).Scan(&total)
	return total > 0, err
}
