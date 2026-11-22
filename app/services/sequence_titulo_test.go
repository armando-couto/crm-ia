package services_test

import (
	"testing"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

// O título da etapa manual troca as variáveis do contato: a tarefa não sai
// com "{{nome}}" cru.
func TestTituloDaTarefaTrocaVariaveis(t *testing.T) {
	db := seqTestDB(t)
	withMailer(t)
	seq := newSequence(t, db, []models.SequenceStep{{Kind: models.StepTaskCall, Title: "Ligar para {{nome}}"}})
	contact := enroll(t, db, seq.ID, "seq-titulo@cliente.com")

	services.ProcessDueSequences(db)

	var title string
	if err := db.QueryRow(`SELECT title FROM tasks WHERE contact_id = $1`, contact.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Ligar para Ana: Ana Silva" {
		t.Fatalf("título = %q", title)
	}
}
