package models

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// A ordenação vem de query string: só colunas da lista fechada podem entrar no SQL.
func TestContactOrderBy(t *testing.T) {
	cases := []struct {
		sortBy, sortDir, want string
	}{
		{"created_at", "asc", "c.created_at ASC NULLS LAST"},
		{"created_at", "desc", "c.created_at DESC NULLS LAST"},
		{"last_activity", "", "last_activity_at DESC NULLS LAST"},
		{"name", "ASC", "LOWER(c.first_name || ' ' || COALESCE(c.last_name,'')) ASC NULLS LAST"},
		// Valores desconhecidos (potencial SQL injection) caem no padrão.
		{"created_at; DROP TABLE contacts", "asc", "c.updated_at ASC NULLS LAST"},
		{"", "", "c.updated_at DESC NULLS LAST"},
	}
	for _, c := range cases {
		if got := contactOrderBy(c.sortBy, c.sortDir); got != c.want {
			t.Errorf("contactOrderBy(%q, %q) = %q, esperado %q", c.sortBy, c.sortDir, got, c.want)
		}
	}
}

func TestLoadContactStats(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"total", "sem_dono", "sem_email", "leads", "inativos"}).
			AddRow(3180, 68, 12, 4580, 1740))

	s, err := LoadContactStats(db)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if s.Total != 3180 || s.SemDono != 68 || s.SemAtividade30 != 1740 {
		t.Fatalf("stats inesperadas: %+v", s)
	}
}

func TestBulkAssignOwner(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	owner := int64(7)
	mock.ExpectExec("UPDATE contacts SET owner_id").
		WillReturnResult(sqlmock.NewResult(0, 3))

	affected, err := BulkAssignOwner(db, []int64{1, 2, 3}, &owner)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if affected != 3 {
		t.Fatalf("esperados 3 afetados, obtidos %d", affected)
	}
}

func TestBulkDeleteContacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec("DELETE FROM contacts WHERE id = ANY").
		WillReturnResult(sqlmock.NewResult(0, 2))

	affected, err := BulkDeleteContacts(db, []int64{5, 6})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if affected != 2 {
		t.Fatalf("esperados 2 afetados, obtidos %d", affected)
	}
}
