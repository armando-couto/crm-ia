package models_test

import (
	"database/sql"
	"os"
	"testing"

	"fixpay/fix-crm/migrations"
	"fixpay/fix-crm/models"

	_ "github.com/lib/pq"
)

// A mesclagem é SQL demais para sqlmock provar alguma coisa: este teste roda
// contra um Postgres de verdade e só liga quando FIXCRM_TEST_DSN aponta para um
// banco descartável. Sem a variável (CI, máquina de quem só roda unit) ele pula.
//
//	docker run -d --name fixcrm-merge-test -e POSTGRES_PASSWORD=postgres \
//	  -e POSTGRES_DB=fixcrm_test -p 55433:5432 postgres:16-alpine
//	FIXCRM_TEST_DSN="postgres://postgres:postgres@localhost:55433/fixcrm_test?sslmode=disable" go test ./models/

// integrationLockID identifica o lock consultivo compartilhado pelos testes de
// integração dos dois pacotes, que dividem o mesmo banco.
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
	return db
}

// cleanTables zera as tabelas usadas pelos testes de mesclagem.
func cleanTables(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`TRUNCATE attachments, activities, tasks, deals, tickets, calls,
		meetings, conversations, list_members, lists, custom_property_values,
		custom_properties, contacts, companies RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergeContactsMovesEverything(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	principal := &models.Contact{FirstName: "Ana", LastName: "Silva", Email: "ana@fixpay.com.br"}
	if err := models.CreateContact(db, principal); err != nil {
		t.Fatal(err)
	}
	// O duplicado tem telefone e cargo que faltam no principal.
	duplicado := &models.Contact{
		FirstName: "Ana", LastName: "Silva", Email: "ana@fixpay.com.br",
		Phone: "11999998888", JobTitle: "Diretora",
	}
	if err := models.CreateContact(db, duplicado); err != nil {
		t.Fatal(err)
	}

	// Histórico e negócio pendurados no duplicado.
	if err := models.CreateActivity(db, &models.Activity{
		Kind: models.ActivityNota, Content: "ligou pedindo proposta", ContactID: &duplicado.ID,
	}); err != nil {
		t.Fatal(err)
	}
	var stageID, pipelineID int64
	if err := db.QueryRow(
		`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).
		Scan(&stageID, &pipelineID); err != nil {
		t.Fatal(err)
	}
	deal := &models.Deal{Name: "Maquininhas", Amount: 5000, PipelineID: pipelineID,
		StageID: stageID, ContactID: &duplicado.ID}
	if err := models.CreateDeal(db, deal); err != nil {
		t.Fatal(err)
	}
	att := &models.Attachment{Entity: models.AttachContact, EntityID: duplicado.ID, Filename: "proposta.pdf"}
	if err := models.CreateAttachment(db, att, []byte("conteudo")); err != nil {
		t.Fatal(err)
	}

	merged, err := models.MergeContacts(db, principal.ID, duplicado.ID)
	if err != nil {
		t.Fatalf("mesclagem falhou: %v", err)
	}

	// Campos vazios do principal completados pelo duplicado.
	if merged.Phone != "11999998888" || merged.JobTitle != "Diretora" {
		t.Fatalf("campos vazios não foram preenchidos: %+v", merged)
	}

	// Duplicado sumiu.
	if _, err := models.ContactByID(db, duplicado.ID); err != sql.ErrNoRows {
		t.Fatalf("duplicado deveria ter sido apagado, veio: %v", err)
	}

	// Tudo apontando para o principal.
	assertCount(t, db, `SELECT COUNT(*) FROM deals WHERE contact_id = $1`, principal.ID, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM activities WHERE contact_id = $1`, principal.ID, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM attachments WHERE entity = 'contato' AND entity_id = $1`,
		principal.ID, 1)
}

// O principal não perde o que já tinha preenchido.
func TestMergeContactsKeepsPrimaryValues(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	principal := &models.Contact{FirstName: "Bruno", LastName: "Lima",
		Email: "bruno@fixpay.com.br", Phone: "11911112222", JobTitle: "Gerente"}
	if err := models.CreateContact(db, principal); err != nil {
		t.Fatal(err)
	}
	duplicado := &models.Contact{FirstName: "Bruno", LastName: "Lima",
		Email: "bruno@fixpay.com.br", Phone: "11933334444", JobTitle: "Analista"}
	if err := models.CreateContact(db, duplicado); err != nil {
		t.Fatal(err)
	}

	merged, err := models.MergeContacts(db, principal.ID, duplicado.ID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Phone != "11911112222" || merged.JobTitle != "Gerente" {
		t.Fatalf("o principal perdeu os próprios dados: %+v", merged)
	}
}

// Contato em duas listas não pode gerar chave duplicada em list_members.
func TestMergeContactsHandlesSharedList(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	principal := &models.Contact{FirstName: "Carla", LastName: "Souza", Email: "carla@fixpay.com.br"}
	duplicado := &models.Contact{FirstName: "Carla", LastName: "Souza", Email: "carla@fixpay.com.br"}
	for _, c := range []*models.Contact{principal, duplicado} {
		if err := models.CreateContact(db, c); err != nil {
			t.Fatal(err)
		}
	}

	var listID int64
	if err := db.QueryRow(
		`INSERT INTO lists (name, kind) VALUES ('Clientes', 'estatica') RETURNING id`).Scan(&listID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{principal.ID, duplicado.ID} {
		if _, err := db.Exec(`INSERT INTO list_members (list_id, contact_id) VALUES ($1, $2)`,
			listID, id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := models.MergeContacts(db, principal.ID, duplicado.ID); err != nil {
		t.Fatalf("mesclagem com lista compartilhada falhou: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM list_members WHERE contact_id = $1`, principal.ID, 1)
}

func TestMergeCompaniesMovesContactsAndDeals(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	principal := &models.Company{Name: "Fix Pay", CNPJ: "12345678000199"}
	duplicada := &models.Company{Name: "Fix Pay LTDA", CNPJ: "12345678000199", Domain: "fixpay.com.br"}
	for _, c := range []*models.Company{principal, duplicada} {
		if err := models.CreateCompany(db, c); err != nil {
			t.Fatal(err)
		}
	}

	contato := &models.Contact{FirstName: "Diego", LastName: "Alves", CompanyID: &duplicada.ID}
	if err := models.CreateContact(db, contato); err != nil {
		t.Fatal(err)
	}

	merged, err := models.MergeCompanies(db, principal.ID, duplicada.ID)
	if err != nil {
		t.Fatalf("mesclagem falhou: %v", err)
	}
	if merged.Domain != "fixpay.com.br" {
		t.Fatalf("domínio do duplicado deveria preencher o vazio: %+v", merged)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM contacts WHERE company_id = $1`, principal.ID, 1)
	if _, err := models.CompanyByID(db, duplicada.ID); err != sql.ErrNoRows {
		t.Fatalf("empresa duplicada deveria ter sido apagada, veio: %v", err)
	}
}

// A busca por duplicados encontra o par de e-mail igual.
func TestFindDuplicateContacts(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	for i := 0; i < 2; i++ {
		if err := models.CreateContact(db, &models.Contact{
			FirstName: "Elisa", LastName: "Rocha", Email: "elisa@fixpay.com.br",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Um contato sozinho não pode virar grupo.
	if err := models.CreateContact(db, &models.Contact{
		FirstName: "Único", LastName: "Registro", Email: "unico@fixpay.com.br",
	}); err != nil {
		t.Fatal(err)
	}

	groups, err := models.FindDuplicateContacts(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("esperado 1 grupo de duplicados, veio %d: %+v", len(groups), groups)
	}
	if groups[0].Reason != "mesmo e-mail" || len(groups[0].Records) != 2 {
		t.Fatalf("grupo inesperado: %+v", groups[0])
	}
}

func TestFindDuplicateCompanies(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	for _, name := range []string{"Mercado Central", "Mercado Central ME"} {
		if err := models.CreateCompany(db, &models.Company{Name: name, CNPJ: "98765432000155"}); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := models.FindDuplicateCompanies(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Reason != "mesmo CNPJ" {
		t.Fatalf("esperado um grupo por CNPJ, veio: %+v", groups)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, arg any, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, arg).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s => %d, esperado %d", query, got, want)
	}
}
