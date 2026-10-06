package services_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

// seqTestDB limpa também as tabelas de sequência antes de cada teste.
func seqTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE sequences, sequence_members RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return db
}

func newSequence(t *testing.T, db *sql.DB, steps []models.SequenceStep, mutate ...func(*models.Sequence)) *models.Sequence {
	t.Helper()
	seq := &models.Sequence{
		Name: "Prospecção", Steps: steps, Active: true,
		ExitOnReply: true, ExitOnMeeting: true,
	}
	for _, m := range mutate {
		m(seq)
	}
	if err := models.CreateSequence(db, seq); err != nil {
		t.Fatal(err)
	}
	return seq
}

func enroll(t *testing.T, db *sql.DB, seqID int64, email string) *models.Contact {
	t.Helper()
	contact := newContact(t, db, email)
	if err := models.EnrollContact(db, seqID, contact.ID, nil); err != nil {
		t.Fatal(err)
	}
	return contact
}

// A validação recusa cadência sem conteúdo e dinâmica sem etapa manual.
func TestValidateSequence(t *testing.T) {
	valida := &models.Sequence{Name: "OK", Steps: []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Oi", Body: "Tudo bem?"},
	}}
	if err := models.ValidateSequence(valida); err != nil {
		t.Fatalf("sequência válida recusada: %v", err)
	}

	casos := map[string]*models.Sequence{
		"sem nome":           {Steps: []models.SequenceStep{{Kind: models.StepEmailAuto, Subject: "a", Body: "b"}}},
		"sem etapas":         {Name: "X"},
		"email sem conteúdo": {Name: "X", Steps: []models.SequenceStep{{Kind: models.StepEmailAuto}}},
		"etapa desconhecida": {Name: "X", Steps: []models.SequenceStep{{Kind: "pombo_correio"}}},
		"dinâmica sem manual": {Name: "X", Dynamic: true, Steps: []models.SequenceStep{
			{Kind: models.StepEmailAuto, Subject: "a", Body: "b"},
		}},
	}
	for nome, seq := range casos {
		if err := models.ValidateSequence(seq); err == nil {
			t.Errorf("%s: deveria ser recusado", nome)
		}
	}
}

// O e-mail automático sai, fica ligado à sequência e a cadência agenda a próxima.
func TestSequenceSendsAutoEmail(t *testing.T) {
	db := seqTestDB(t)
	mailer := withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Olá {{nome}}", Body: "Primeiro toque"},
		{Kind: models.StepEmailAuto, DelayDays: 3, Subject: "Retomando", Body: "Segundo toque"},
	})
	contact := enroll(t, db, seq.ID, "seq1@cliente.com")

	services.ProcessDueSequences(db)

	if len(mailer.sent) != 1 || mailer.sent[0] != "seq1@cliente.com|Olá Ana" {
		t.Fatalf("primeiro e-mail inesperado: %v", mailer.sent)
	}

	// Ligado à sequência para as métricas.
	var seqID int64
	if err := db.QueryRow(`SELECT sequence_id FROM email_messages WHERE contact_id = $1`,
		contact.ID).Scan(&seqID); err != nil || seqID != seq.ID {
		t.Fatalf("o e-mail deveria apontar para a sequência: %v (%d)", err, seqID)
	}

	// A próxima etapa ficou agendada para daqui a 3 dias, não para agora.
	members, _ := models.ListMembers(db, seq.ID, "")
	if members[0].Step != 1 || members[0].Status != models.MemberActive {
		t.Fatalf("inscrição não avançou: %+v", members[0])
	}
	if time.Until(members[0].NextRunAt) < 48*time.Hour {
		t.Fatalf("a espera de 3 dias não foi aplicada: %v", members[0].NextRunAt)
	}

	// Rodar de novo agora não manda o segundo e-mail antes da hora.
	services.ProcessDueSequences(db)
	if len(mailer.sent) != 1 {
		t.Fatalf("o segundo e-mail saiu antes do prazo: %v", mailer.sent)
	}
}

// Etapa manual cria a tarefa e segura a cadência até a conclusão.
func TestManualStepHoldsUntilTaskDone(t *testing.T) {
	db := seqTestDB(t)
	mailer := withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepTaskCall, Title: "Ligar para o prospect"},
		{Kind: models.StepEmailAuto, DelayDays: 0, Subject: "Depois da ligação", Body: "Seguindo"},
	})
	contact := enroll(t, db, seq.ID, "seq2@cliente.com")

	services.ProcessDueSequences(db)

	// A tarefa nasceu na fila e a inscrição está travada nela.
	var taskID int64
	var title string
	if err := db.QueryRow(`SELECT id, title FROM tasks WHERE contact_id = $1`, contact.ID).
		Scan(&taskID, &title); err != nil {
		t.Fatalf("a tarefa da etapa manual não foi criada: %v", err)
	}
	if title != "Ligar para o prospect: Ana Silva" {
		t.Fatalf("título inesperado: %s", title)
	}
	members, _ := models.ListMembers(db, seq.ID, "")
	if members[0].Status != models.MemberWaiting {
		t.Fatalf("a inscrição deveria estar aguardando a tarefa: %+v", members[0])
	}

	// Rodar o worker de novo não duplica a tarefa nem avança.
	services.ProcessDueSequences(db)
	var tarefas int
	db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE contact_id = $1`, contact.ID).Scan(&tarefas)
	if tarefas != 1 {
		t.Fatalf("a tarefa foi duplicada: %d", tarefas)
	}
	if len(mailer.sent) != 0 {
		t.Fatalf("o e-mail seguinte saiu antes da tarefa ser concluída: %v", mailer.sent)
	}

	// Concluir a tarefa solta a cadência; o e-mail seguinte sai no próximo tick.
	if err := models.ToggleTask(db, taskID, true); err != nil {
		t.Fatal(err)
	}
	services.CompleteSequenceTask(db, taskID)
	services.ProcessDueSequences(db)

	if len(mailer.sent) != 1 {
		t.Fatalf("o e-mail pós-tarefa deveria ter saído: %v", mailer.sent)
	}
	members, _ = models.ListMembers(db, seq.ID, "")
	if members[0].Status != models.MemberDone {
		t.Fatalf("a cadência deveria ter terminado: %+v", members[0])
	}
}

// Responder tira o contato da sequência e alimenta a taxa de resposta.
func TestReplyExitsSequence(t *testing.T) {
	db := seqTestDB(t)
	withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Oi", Body: "Primeiro"},
		{Kind: models.StepEmailAuto, DelayDays: 3, Subject: "Oi de novo", Body: "Segundo"},
	})
	contact := enroll(t, db, seq.ID, "seq3@cliente.com")
	services.ProcessDueSequences(db)

	services.SequenceExitOnReply(db, contact.ID)

	members, _ := models.ListMembers(db, seq.ID, "")
	if members[0].Status != models.MemberExited || members[0].ExitReason != "respondeu" {
		t.Fatalf("a resposta deveria tirar o contato: %+v", members[0])
	}

	var respondido int
	db.QueryRow(`SELECT COUNT(*) FROM email_messages WHERE contact_id = $1 AND replied_at IS NOT NULL`,
		contact.ID).Scan(&respondido)
	if respondido != 1 {
		t.Fatalf("a resposta deveria marcar o e-mail: %d", respondido)
	}
}

// Com a regra desligada, a resposta não tira o contato.
func TestReplyKeepsMemberWhenRuleOff(t *testing.T) {
	db := seqTestDB(t)
	withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Oi", Body: "Primeiro"},
		{Kind: models.StepEmailAuto, DelayDays: 3, Subject: "Dois", Body: "Segundo"},
	}, func(s *models.Sequence) { s.ExitOnReply = false })
	contact := enroll(t, db, seq.ID, "seq4@cliente.com")
	services.ProcessDueSequences(db)

	services.SequenceExitOnReply(db, contact.ID)

	members, _ := models.ListMembers(db, seq.ID, "")
	if members[0].Status != models.MemberActive {
		t.Fatalf("com a regra desligada o contato deveria continuar: %+v", members[0])
	}
	// Mas o engajamento fica registrado mesmo assim.
	if !members[0].Engaged {
		t.Fatal("a resposta deveria marcar o engajamento")
	}
}

// Reunião agendada também encerra a cadência.
func TestMeetingExitsSequence(t *testing.T) {
	db := seqTestDB(t)
	withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Oi", Body: "Primeiro"},
		{Kind: models.StepEmailAuto, DelayDays: 5, Subject: "Dois", Body: "Segundo"},
	})
	contact := enroll(t, db, seq.ID, "seq5@cliente.com")
	services.ProcessDueSequences(db)

	services.SequenceExitOnMeeting(db, contact.ID)

	members, _ := models.ListMembers(db, seq.ID, "")
	if members[0].Status != models.MemberExited || members[0].ExitReason != "reuniao" {
		t.Fatalf("a reunião deveria tirar o contato: %+v", members[0])
	}
}

// Sequência dinâmica: contato engajado pula os e-mails restantes e cai na manual.
func TestDynamicSequenceJumpsToManualOnEngagement(t *testing.T) {
	db := seqTestDB(t)
	mailer := withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Um", Body: "Primeiro"},
		{Kind: models.StepEmailAuto, DelayDays: 0, Subject: "Dois", Body: "Segundo"},
		{Kind: models.StepTaskCall, Title: "Ligar agora"},
	}, func(s *models.Sequence) { s.Dynamic = true })
	contact := enroll(t, db, seq.ID, "seq6@cliente.com")

	// Primeiro e-mail sai.
	services.ProcessDueSequences(db)
	if len(mailer.sent) != 1 {
		t.Fatalf("esperava 1 e-mail, veio %v", mailer.sent)
	}

	// O contato abre o e-mail: engajou.
	services.SequenceMarkEngaged(db, contact.ID)

	// No próximo tick, em vez do segundo e-mail, nasce a tarefa de ligação.
	services.ProcessDueSequences(db)
	if len(mailer.sent) != 1 {
		t.Fatalf("o segundo e-mail não deveria sair para contato engajado: %v", mailer.sent)
	}
	var tarefas int
	db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE contact_id = $1`, contact.ID).Scan(&tarefas)
	if tarefas != 1 {
		t.Fatalf("a tarefa de ligação deveria ter sido criada: %d", tarefas)
	}
}

// Sem engajamento, a dinâmica segue os e-mails normalmente.
func TestDynamicSequenceKeepsEmailsWithoutEngagement(t *testing.T) {
	db := seqTestDB(t)
	mailer := withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Um", Body: "Primeiro"},
		{Kind: models.StepEmailAuto, DelayDays: 0, Subject: "Dois", Body: "Segundo"},
		{Kind: models.StepTaskCall, Title: "Ligar"},
	}, func(s *models.Sequence) { s.Dynamic = true })
	enroll(t, db, seq.ID, "seq7@cliente.com")

	services.ProcessDueSequences(db)
	services.ProcessDueSequences(db)

	if len(mailer.sent) != 2 {
		t.Fatalf("sem engajamento os dois e-mails deveriam sair: %v", mailer.sent)
	}
}

// Sequência pausada não anda; contato sem e-mail não entra.
func TestSequenceGuards(t *testing.T) {
	db := seqTestDB(t)
	mailer := withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Oi", Body: "Olá"},
	}, func(s *models.Sequence) { s.Active = true })
	enroll(t, db, seq.ID, "seq8@cliente.com")

	// Pausa antes do tick.
	seq.Active = false
	if err := models.UpdateSequence(db, seq); err != nil {
		t.Fatal(err)
	}
	services.ProcessDueSequences(db)
	if len(mailer.sent) != 0 {
		t.Fatalf("sequência pausada não deveria enviar: %v", mailer.sent)
	}

	// Contato sem e-mail é recusado na inscrição.
	semEmail := &models.Contact{FirstName: "Sem", LastName: "Email"}
	if err := models.CreateContact(db, semEmail); err != nil {
		t.Fatal(err)
	}
	if err := models.EnrollContact(db, seq.ID, semEmail.ID, nil); err == nil {
		t.Fatal("contato sem e-mail deveria ser recusado")
	}
}

// Reinscrever alguém que saiu recomeça a cadência do zero.
func TestReenrollRestartsSequence(t *testing.T) {
	db := seqTestDB(t)
	withMailer(t)

	seq := newSequence(t, db, []models.SequenceStep{
		{Kind: models.StepEmailAuto, Subject: "Oi", Body: "Olá"},
		{Kind: models.StepEmailAuto, DelayDays: 3, Subject: "Dois", Body: "Segundo"},
	})
	contact := enroll(t, db, seq.ID, "seq9@cliente.com")
	services.ProcessDueSequences(db)
	services.SequenceExitOnReply(db, contact.ID)

	if err := models.EnrollContact(db, seq.ID, contact.ID, nil); err != nil {
		t.Fatal(err)
	}
	members, _ := models.ListMembers(db, seq.ID, "")
	if members[0].Status != models.MemberActive || members[0].Step != 0 || members[0].Engaged {
		t.Fatalf("a reinscrição deveria zerar a cadência: %+v", members[0])
	}
}
