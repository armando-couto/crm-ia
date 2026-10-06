package services_test

import (
	"bytes"
	"testing"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"

	"github.com/xuri/excelize/v2"
)

func TestParseImportFileCSVComPontoEVirgulaEBOM(t *testing.T) {
	csv := "\xEF\xBB\xBFNome;E-mail;Empresa\nAna;ana@ex.com.br;Padaria Doce\n"
	headers, rows, err := services.ParseImportFile("carga.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if headers[0] != "Nome" || headers[1] != "E-mail" {
		t.Fatalf("cabeçalho errado: %v", headers)
	}
	if len(rows) != 1 || rows[0][2] != "Padaria Doce" {
		t.Fatalf("linhas erradas: %v", rows)
	}
}

func TestParseImportFileXLSX(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	f.SetSheetRow(sheet, "A1", &[]any{"Nome", "Sobrenome", "E-mail"})
	f.SetSheetRow(sheet, "A2", &[]any{"Carlos", "Lima", "carlos@ex.com.br"})
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	headers, rows, err := services.ParseImportFile("Carga 458 Sim.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 3 || headers[2] != "E-mail" {
		t.Fatalf("cabeçalho errado: %v", headers)
	}
	if len(rows) != 1 || rows[0][0] != "Carlos" {
		t.Fatalf("linhas erradas: %v", rows)
	}
}

func TestParseImportFileFormatoInvalido(t *testing.T) {
	if _, _, err := services.ParseImportFile("carga.pdf", []byte("x")); err == nil {
		t.Fatal("esperava erro de formato")
	}
}

// TestRunImportContatos prova o comportamento central do centro de importações:
// quem já existe é ATUALIZADO pelo e-mail (nunca duplicado), quem não existe é
// criado, a coluna "Empresa" gera a associação e linha sem e-mail vira erro.
func TestRunImportContatos(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE imports, contacts, companies RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	existing := &models.Contact{FirstName: "Ana", Email: "ana@ex.com.br", Phone: "11 91111-1111"}
	if err := models.CreateContact(db, existing); err != nil {
		t.Fatal(err)
	}

	headers := []string{"Nome", "Sobrenome", "E-mail", "Telefone", "Empresa"}
	rows := [][]string{
		{"Ana Paula", "", "ana@ex.com.br", "", "Padaria Doce"},   // atualiza + associa
		{"Bruno", "Souza", "bruno@ex.com.br", "11 92222-2222", ""}, // cria
		{"Sem", "Email", "", "11 93333-3333", ""},                  // erro
		{"", "", "", "", ""},                                       // linha vazia: ignorada
	}

	record, err := services.RunImport(db, "contatos", "Carga 458 Sim.xlsx", headers, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if record.TotalRows != 3 || record.NewRecords != 1 || record.UpdatedRecords != 1 ||
		record.NewAssociations != 1 || record.ErrorCount != 1 {
		t.Fatalf("contadores errados: %+v", record)
	}
	if record.Status != "concluida" {
		t.Fatalf("status errado: %s", record.Status)
	}

	// A Ana foi atualizada (nome novo, telefone preservado) e associada.
	updated, err := models.ContactByID(db, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.FirstName != "Ana Paula" || updated.Phone != "11 91111-1111" {
		t.Fatalf("atualização errada: %+v", updated)
	}
	if updated.CompanyID == nil {
		t.Fatal("associação com a empresa não criada")
	}

	// Não duplicou: só Ana e Bruno.
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("esperava 2 contatos, veio %d", total)
	}

	// Histórico gravado.
	list, err := models.ListImports(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].FileName != "Carga 458 Sim.xlsx" || list[0].UpdatedRecords != 1 {
		t.Fatalf("histórico errado: %+v", list)
	}
}

// TestRunImportEmpresas casa a empresa pelo CNPJ mesmo com máscara diferente.
func TestRunImportEmpresas(t *testing.T) {
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE imports, contacts, companies RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	existing := &models.Company{Name: "Padaria Doce", CNPJ: "12.345.678/0001-90"}
	if err := models.CreateCompany(db, existing); err != nil {
		t.Fatal(err)
	}

	headers := []string{"Razão Social", "CNPJ", "Cidade", "UF"}
	rows := [][]string{
		{"Padaria Doce Ltda", "12345678000190", "Osasco", "SP"}, // atualiza pelo CNPJ
		{"Mercado Novo", "98.765.432/0001-10", "Barueri", "SP"}, // cria
	}

	record, err := services.RunImport(db, "empresas", "empresas.csv", headers, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if record.NewRecords != 1 || record.UpdatedRecords != 1 || record.ErrorCount != 0 {
		t.Fatalf("contadores errados: %+v", record)
	}

	updated, err := models.CompanyByID(db, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Padaria Doce Ltda" || updated.City != "Osasco" || updated.State != "SP" {
		t.Fatalf("atualização errada: %+v", updated)
	}
}

func TestRunImportContatosSemColunaDeEmail(t *testing.T) {
	db := testDB(t)
	_, err := services.RunImport(db, "contatos", "x.csv",
		[]string{"Nome", "Telefone"}, [][]string{{"Ana", "11 9"}}, nil)
	if err == nil {
		t.Fatal("esperava erro pela falta da coluna de e-mail")
	}
}
