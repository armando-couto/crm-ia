package services_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/migrations"
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"

	_ "github.com/lib/pq"
)

// O motor de automações mexe em várias tabelas: testar contra Postgres de
// verdade é o único jeito de provar que a sequência anda. Sem FIXCRM_TEST_DSN
// (banco descartável) os testes pulam.

// integrationLockID é o mesmo lock usado pelos testes do pacote models: os dois
// pacotes rodam em paralelo sobre o mesmo banco.
const integrationLockID = 918273

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("FIXCRM_TEST_DSN")
	if dsn == "" {
		t.Skip("FIXCRM_TEST_DSN não definido: pulando teste de integração")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("banco de teste inacessível: %v", err)
	}
	// Os pacotes de teste rodam em paralelo e compartilham este banco: o lock
	// consultivo garante que só um teste de integração mexa nas tabelas por vez.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`SELECT pg_advisory_lock($1)`, integrationLockID); err != nil {
		t.Fatalf("lock do banco de teste: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`SELECT pg_advisory_unlock($1)`, integrationLockID)
		db.Close()
	})

	if err := migrations.Run(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// Estes testes exercitam o motor: ligam explicitamente.
	services.AutomationsEnabled = true
	t.Cleanup(func() { services.AutomationsEnabled = false })

	if _, err := db.Exec(`TRUNCATE automations, automation_runs, sequence_enrollments,
		activities, tasks, email_messages, list_members, lists, contacts RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return db
}

// recordingMailer guarda os envios em vez de falar com o Mandrill.
type recordingMailer struct {
	sent []string
}

func (m *recordingMailer) Send(toEmail, toName, subject, html string) error {
	m.sent = append(m.sent, toEmail+"|"+subject)
	return nil
}

func withMailer(t *testing.T) *recordingMailer {
	t.Helper()
	mailer := &recordingMailer{}
	original := services.Mail
	services.Mail = mailer
	t.Cleanup(func() { services.Mail = original })
	return mailer
}

func cfgJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newContact(t *testing.T, db *sql.DB, email string) *models.Contact {
	t.Helper()
	contact := &models.Contact{FirstName: "Ana", LastName: "Silva", Email: email}
	if err := models.CreateContact(db, contact); err != nil {
		t.Fatal(err)
	}
	return contact
}

func newAutomation(t *testing.T, db *sql.DB, trigger string, actions []models.AutomationAction) *models.Automation {
	t.Helper()
	auto := &models.Automation{
		Name:        "Teste",
		TriggerKind: trigger,
		Actions:     actions,
		Active:      true,
	}
	if err := models.CreateAutomation(db, auto); err != nil {
		t.Fatal(err)
	}
	return auto
}

// Contato criado dispara a automação e a ação roda de verdade.
func TestFireContactCreatedRunsActions(t *testing.T) {
	db := testDB(t)
	withMailer(t)

	newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "veio da automação"})},
	})
	contact := newContact(t, db, "ana@cliente.com")

	services.FireContactCreated(db, contact)

	var notes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM activities WHERE contact_id = $1 AND content = 'veio da automação'`,
		contact.ID).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if notes != 1 {
		t.Fatalf("esperava 1 observação criada pela automação, veio %d", notes)
	}
}

// Automação pausada não roda.
func TestInactiveAutomationDoesNotRun(t *testing.T) {
	db := testDB(t)

	auto := newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "não deveria aparecer"})},
	})
	auto.Active = false
	if err := models.UpdateAutomation(db, auto); err != nil {
		t.Fatal(err)
	}

	contact := newContact(t, db, "pausada@cliente.com")
	services.FireContactCreated(db, contact)

	var notes int
	db.QueryRow(`SELECT COUNT(*) FROM activities WHERE contact_id = $1`, contact.ID).Scan(&notes)
	if notes != 0 {
		t.Fatalf("automação pausada não deveria agir, criou %d atividades", notes)
	}
}

// O e-mail da automação sai rastreado e com origem "automacao".
func TestAutomationSendsTrackedEmail(t *testing.T) {
	db := testDB(t)
	mailer := withMailer(t)

	newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionSendEmail, Config: cfgJSON(t, map[string]any{
			"subject": "Olá {{nome}}", "body": "Bem-vinda à Fix Pay",
		})},
	})
	contact := newContact(t, db, "lead@cliente.com")
	services.FireContactCreated(db, contact)

	if len(mailer.sent) != 1 {
		t.Fatalf("esperava 1 e-mail enviado, veio %d", len(mailer.sent))
	}
	// A variável do modelo é resolvida com os dados do contato.
	if mailer.sent[0] != "lead@cliente.com|Olá Ana" {
		t.Fatalf("assunto inesperado: %s", mailer.sent[0])
	}

	var source string
	if err := db.QueryRow(`SELECT source FROM email_messages WHERE contact_id = $1`,
		contact.ID).Scan(&source); err != nil {
		t.Fatalf("o envio deveria estar registrado para rastreio: %v", err)
	}
	if source != models.EmailSourceAutomation {
		t.Fatalf("origem esperada 'automacao', veio %q", source)
	}
}

// A espera para a sequência e agenda a retomada.
func TestWaitEnrollsAndResumes(t *testing.T) {
	db := testDB(t)
	withMailer(t)

	auto := newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "passo 1"})},
		{Kind: models.ActionWait, Config: cfgJSON(t, map[string]any{"days": 3})},
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "passo 2"})},
	})
	contact := newContact(t, db, "sequencia@cliente.com")
	services.FireContactCreated(db, contact)

	// Só o primeiro passo rodou; o segundo ficou agendado.
	assertActivityCount(t, db, contact.ID, "passo 1", 1)
	assertActivityCount(t, db, contact.ID, "passo 2", 0)

	pendentes, err := models.CountActiveEnrollments(db, auto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pendentes != 1 {
		t.Fatalf("esperava 1 contato em sequência, veio %d", pendentes)
	}

	// Antes do prazo o worker não faz nada.
	services.ProcessDueEnrollments(db)
	assertActivityCount(t, db, contact.ID, "passo 2", 0)

	// Adianta o relógio da inscrição e roda de novo.
	if _, err := db.Exec(`UPDATE sequence_enrollments SET next_run_at = NOW() - INTERVAL '1 minute'`); err != nil {
		t.Fatal(err)
	}
	services.ProcessDueEnrollments(db)

	assertActivityCount(t, db, contact.ID, "passo 2", 1)
	// Passo 1 não repete na retomada.
	assertActivityCount(t, db, contact.ID, "passo 1", 1)

	pendentes, _ = models.CountActiveEnrollments(db, auto.ID)
	if pendentes != 0 {
		t.Fatalf("a sequência deveria ter terminado, restam %d", pendentes)
	}
}

// Duas esperas seguidas encadeiam corretamente.
func TestSequenceWithTwoWaits(t *testing.T) {
	db := testDB(t)

	newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionWait, Config: cfgJSON(t, map[string]any{"days": 1})},
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "meio"})},
		{Kind: models.ActionWait, Config: cfgJSON(t, map[string]any{"days": 2})},
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "fim"})},
	})
	contact := newContact(t, db, "duas@cliente.com")
	services.FireContactCreated(db, contact)

	// Primeira espera.
	db.Exec(`UPDATE sequence_enrollments SET next_run_at = NOW() - INTERVAL '1 minute'`)
	services.ProcessDueEnrollments(db)
	assertActivityCount(t, db, contact.ID, "meio", 1)
	assertActivityCount(t, db, contact.ID, "fim", 0)

	// Segunda espera.
	db.Exec(`UPDATE sequence_enrollments SET next_run_at = NOW() - INTERVAL '1 minute'`)
	services.ProcessDueEnrollments(db)
	assertActivityCount(t, db, contact.ID, "fim", 1)
}

// Automação apagada cancela a sequência em andamento.
func TestDeletedAutomationCancelsEnrollment(t *testing.T) {
	db := testDB(t)

	auto := newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionWait, Config: cfgJSON(t, map[string]any{"days": 1})},
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "nunca"})},
	})
	contact := newContact(t, db, "apagada@cliente.com")
	services.FireContactCreated(db, contact)

	if err := models.DeleteAutomation(db, auto.ID); err != nil {
		t.Fatal(err)
	}
	services.ProcessDueEnrollments(db)

	assertActivityCount(t, db, contact.ID, "nunca", 0)
}

// A ação de tarefa cria a tarefa com o prazo pedido.
func TestActionCreateTask(t *testing.T) {
	db := testDB(t)

	newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionCreateTask, Config: cfgJSON(t, map[string]any{
			"title": "Ligar para o lead", "days": 2,
		})},
	})
	contact := newContact(t, db, "tarefa@cliente.com")
	services.FireContactCreated(db, contact)

	var title string
	var due time.Time
	if err := db.QueryRow(`SELECT title, due_date FROM tasks WHERE contact_id = $1`,
		contact.ID).Scan(&title, &due); err != nil {
		t.Fatalf("a tarefa deveria ter sido criada: %v", err)
	}
	if title != "Ligar para o lead" {
		t.Fatalf("título inesperado: %s", title)
	}
	if due.Before(time.Now().AddDate(0, 0, 1)) {
		t.Fatalf("o prazo deveria ser daqui a 2 dias, veio %s", due)
	}
}

// Ação mal configurada interrompe a automação e fica registrada como erro.
func TestActionErrorIsRecorded(t *testing.T) {
	db := testDB(t)

	auto := newAutomation(t, db, models.TriggerContactCreated, []models.AutomationAction{
		{Kind: models.ActionSetOwner, Config: cfgJSON(t, map[string]any{})}, // sem user_id
		{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "não chega aqui"})},
	})
	contact := newContact(t, db, "erro@cliente.com")
	services.FireContactCreated(db, contact)

	runs, err := models.ListAutomationRuns(db, auto.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "erro" {
		t.Fatalf("esperava uma execução com erro, veio %+v", runs)
	}
	assertActivityCount(t, db, contact.ID, "não chega aqui", 0)
}

// O filtro do gatilho separa quem dispara de quem não dispara.
func TestTriggerConfigFiltersByLifecycle(t *testing.T) {
	db := testDB(t)

	auto := &models.Automation{
		Name:          "Só clientes",
		TriggerKind:   models.TriggerContactStage,
		TriggerConfig: cfgJSON(t, map[string]any{"lifecycle_stage": "cliente"}),
		Actions: []models.AutomationAction{
			{Kind: models.ActionAddNote, Config: cfgJSON(t, map[string]any{"content": "virou cliente"})},
		},
		Active: true,
	}
	if err := models.CreateAutomation(db, auto); err != nil {
		t.Fatal(err)
	}

	// Estágio diferente: não dispara.
	lead := newContact(t, db, "lead2@cliente.com")
	lead.LifecycleStage = "mql"
	services.FireContactStage(db, lead)
	assertActivityCount(t, db, lead.ID, "virou cliente", 0)

	// Estágio combinando: dispara.
	cliente := newContact(t, db, "cliente@cliente.com")
	cliente.LifecycleStage = "cliente"
	services.FireContactStage(db, cliente)
	assertActivityCount(t, db, cliente.ID, "virou cliente", 1)
}

func assertActivityCount(t *testing.T, db *sql.DB, contactID int64, content string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM activities WHERE contact_id = $1 AND content = $2`,
		contactID, content).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("atividades com %q = %d, esperado %d", content, got, want)
	}
}
