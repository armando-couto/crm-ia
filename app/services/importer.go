package services

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/xuri/excelize/v2"
)

// Centro de importações: lê CSV ou XLSX, casa as colunas pelo cabeçalho e
// ATUALIZA registros existentes em vez de duplicar — contato casa pelo e-mail,
// empresa pelo CNPJ (ou nome). Associações contato→empresa são criadas quando a
// planilha traz a coluna de empresa.

const maxImportErrors = 50

// ParseImportFile devolve cabeçalho e linhas do arquivo, decidindo o formato
// pela extensão (.xlsx ou .csv).
func ParseImportFile(fileName string, data []byte) (headers []string, rows [][]string, err error) {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".xlsx":
		return parseXLSX(data)
	case ".csv", ".txt":
		return parseCSV(data)
	default:
		return nil, nil, fmt.Errorf("formato não suportado: envie .csv ou .xlsx")
	}
}

func parseXLSX(data []byte) ([]string, [][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("não consegui abrir o XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, fmt.Errorf("planilha vazia")
	}
	all, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("planilha vazia")
	}
	return all[0], all[1:], nil
}

func parseCSV(data []byte) ([]string, [][]string, error) {
	// Excel brasileiro costuma exportar com BOM e separador ';'.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	firstLine := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		firstLine = data[:i]
	}
	delimiter := ','
	if bytes.Count(firstLine, []byte{';'}) > bytes.Count(firstLine, []byte{','}) {
		delimiter = ';'
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	all, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("não consegui ler o CSV: %w", err)
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("arquivo vazio")
	}
	return all[0], all[1:], nil
}

// normalizeHeader baixa a caixa e remove acentos para casar os cabeçalhos que
// vêm da planilha ("E-mail", "Razão Social"…) com o catálogo de campos.
func normalizeHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a",
		"é", "e", "ê", "e", "í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u", "ü", "u", "ç", "c",
	)
	h = replacer.Replace(h)
	return strings.Join(strings.Fields(h), " ")
}

// contactHeaderField mapeia o cabeçalho normalizado para o campo do contato.
// "empresa" é especial: vira a associação contato→empresa.
var contactHeaderField = map[string]string{
	"nome": "first_name", "primeiro nome": "first_name", "first name": "first_name",
	"sobrenome": "last_name", "last name": "last_name",
	"email": "email", "e-mail": "email",
	"telefone": "phone", "celular": "phone", "whatsapp": "phone", "phone": "phone",
	"cargo": "job_title", "job title": "job_title",
	"fase do ciclo de vida": "lifecycle_stage", "ciclo de vida": "lifecycle_stage",
	"estagio": "lifecycle_stage", "etapa": "lifecycle_stage", "lifecycle stage": "lifecycle_stage",
	"origem": "source", "fonte": "source", "fonte do registro": "source", "source": "source",
	"papel de compra": "buying_role", "buying role": "buying_role",
	"empresa": "company", "nome da empresa": "company", "company": "company",
}

var companyHeaderField = map[string]string{
	"nome": "name", "empresa": "name", "nome da empresa": "name",
	"razao social": "name", "company": "name",
	"cnpj":     "cnpj",
	"dominio":  "domain", "site": "domain", "website": "domain", "domain": "domain",
	"telefone": "phone", "phone": "phone",
	"setor": "industry", "industria": "industry", "segmento": "industry", "industry": "industry",
	"cidade": "city", "city": "city",
	"estado": "state", "uf": "state", "state": "state",
	"numero ec": "ec_number", "numero do ec": "ec_number", "ec": "ec_number",
	"grupo economico": "economic_group",
	"representante":   "representative",
	"instagram":       "instagram",
}

// mapHeaders devolve, para cada campo reconhecido, o índice da coluna.
func mapHeaders(headers []string, catalog map[string]string) map[string]int {
	cols := map[string]int{}
	for i, h := range headers {
		if field, ok := catalog[normalizeHeader(h)]; ok {
			if _, taken := cols[field]; !taken {
				cols[field] = i
			}
		}
	}
	return cols
}

func cell(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func emptyRow(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RunImport processa o arquivo já parseado e grava o resultado no histórico.
// A linha da planilha nunca apaga dado existente: só campos preenchidos entram.
func RunImport(db *sql.DB, entity, fileName string, headers []string, rows [][]string, createdBy *int64) (*models.ImportRecord, error) {
	record := &models.ImportRecord{
		FileName:  fileName,
		Entity:    entity,
		Status:    "concluida",
		CreatedBy: createdBy,
		Errors:    []string{},
	}

	var err error
	switch entity {
	case "contatos":
		err = importContacts(db, headers, rows, createdBy, record)
	case "empresas":
		err = importCompanies(db, headers, rows, record)
	default:
		return nil, fmt.Errorf("entidade inválida: use contatos ou empresas")
	}
	if err != nil {
		return nil, err
	}

	if record.ErrorCount > 0 && record.NewRecords+record.UpdatedRecords == 0 {
		record.Status = "falhou"
	}
	if len(record.Errors) > maxImportErrors {
		extra := len(record.Errors) - maxImportErrors
		record.Errors = append(record.Errors[:maxImportErrors],
			fmt.Sprintf("… e mais %d erros", extra))
	}
	if err := models.CreateImport(db, record); err != nil {
		return nil, err
	}
	return record, nil
}

func addRowError(record *models.ImportRecord, line int, msg string) {
	record.ErrorCount++
	record.Errors = append(record.Errors, fmt.Sprintf("linha %d: %s", line, msg))
}

func importContacts(db *sql.DB, headers []string, rows [][]string, createdBy *int64, record *models.ImportRecord) error {
	cols := mapHeaders(headers, contactHeaderField)
	emailCol, ok := cols["email"]
	if !ok {
		return fmt.Errorf("o arquivo precisa de uma coluna de e-mail — é por ela que os contatos existentes são atualizados")
	}

	col := func(field string) int {
		if i, ok := cols[field]; ok {
			return i
		}
		return -1
	}

	for n, row := range rows {
		line := n + 2 // 1 é o cabeçalho
		if emptyRow(row) {
			continue
		}
		record.TotalRows++

		email := models.NormalizeEmail(cell(row, emailCol))
		if email == "" {
			addRowError(record, line, "sem e-mail")
			continue
		}

		// Associação contato→empresa pela coluna "empresa".
		var companyID *int64
		if name := cell(row, col("company")); name != "" {
			id, err := findOrCreateCompanyByName(db, name)
			if err != nil {
				addRowError(record, line, "empresa: "+err.Error())
			} else {
				companyID = &id
			}
		}

		var existingID int64
		err := db.QueryRow(`SELECT id FROM contacts WHERE email = $1 LIMIT 1`, email).Scan(&existingID)
		switch {
		case err == sql.ErrNoRows:
			contact := &models.Contact{
				FirstName:      cell(row, col("first_name")),
				LastName:       cell(row, col("last_name")),
				Email:          email,
				Phone:          cell(row, col("phone")),
				JobTitle:       cell(row, col("job_title")),
				LifecycleStage: cell(row, col("lifecycle_stage")),
				Source:         cell(row, col("source")),
				BuyingRole:     cell(row, col("buying_role")),
				CompanyID:      companyID,
				OwnerID:        createdBy,
			}
			if contact.LifecycleStage != "" && !models.ValidLifecycleStage(contact.LifecycleStage) {
				contact.LifecycleStage = "lead"
			}
			if contact.Source == "" {
				contact.Source = "importacao"
			}
			if err := models.CreateContact(db, contact); err != nil {
				addRowError(record, line, err.Error())
				continue
			}
			record.NewRecords++
			if companyID != nil {
				record.NewAssociations++
			}
		case err != nil:
			addRowError(record, line, err.Error())
		default:
			contact, err := models.ContactByID(db, existingID)
			if err != nil {
				addRowError(record, line, err.Error())
				continue
			}
			setIf := func(dst *string, v string) {
				if v != "" {
					*dst = v
				}
			}
			setIf(&contact.FirstName, cell(row, col("first_name")))
			setIf(&contact.LastName, cell(row, col("last_name")))
			setIf(&contact.Phone, cell(row, col("phone")))
			setIf(&contact.JobTitle, cell(row, col("job_title")))
			setIf(&contact.BuyingRole, cell(row, col("buying_role")))
			setIf(&contact.Source, cell(row, col("source")))
			if stage := cell(row, col("lifecycle_stage")); stage != "" && models.ValidLifecycleStage(stage) {
				contact.LifecycleStage = stage
			}
			if companyID != nil && contact.CompanyID == nil {
				contact.CompanyID = companyID
				record.NewAssociations++
			}
			if err := models.UpdateContact(db, contact); err != nil {
				addRowError(record, line, err.Error())
				continue
			}
			record.UpdatedRecords++
		}
	}
	return nil
}

// findOrCreateCompanyByName acha a empresa pelo nome (sem diferenciar caixa)
// ou cria uma nova só com o nome.
func findOrCreateCompanyByName(db *sql.DB, name string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM companies WHERE LOWER(name) = LOWER($1) LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	company := &models.Company{Name: name}
	if err := models.CreateCompany(db, company); err != nil {
		return 0, err
	}
	return company.ID, nil
}

func importCompanies(db *sql.DB, headers []string, rows [][]string, record *models.ImportRecord) error {
	cols := mapHeaders(headers, companyHeaderField)
	if _, ok := cols["name"]; !ok {
		return fmt.Errorf("o arquivo precisa de uma coluna com o nome da empresa")
	}

	col := func(field string) int {
		if i, ok := cols[field]; ok {
			return i
		}
		return -1
	}

	for n, row := range rows {
		line := n + 2
		if emptyRow(row) {
			continue
		}
		record.TotalRows++

		name := cell(row, col("name"))
		cnpj := onlyDigits(cell(row, col("cnpj")))
		if name == "" && cnpj == "" {
			addRowError(record, line, "sem nome nem CNPJ")
			continue
		}

		// O CNPJ é a chave preferida; sem ele, casa pelo nome.
		var existingID int64
		var err error
		if cnpj != "" {
			err = db.QueryRow(`
				SELECT id FROM companies
				WHERE regexp_replace(cnpj, '\D', '', 'g') = $1 AND cnpj <> ''
				LIMIT 1`, cnpj).Scan(&existingID)
		} else {
			err = sql.ErrNoRows
		}
		if err == sql.ErrNoRows && name != "" {
			err = db.QueryRow(`SELECT id FROM companies WHERE LOWER(name) = LOWER($1) LIMIT 1`, name).Scan(&existingID)
		}

		switch {
		case err == sql.ErrNoRows:
			company := &models.Company{
				Name:           name,
				CNPJ:           cell(row, col("cnpj")),
				Domain:         cell(row, col("domain")),
				Phone:          cell(row, col("phone")),
				Industry:       cell(row, col("industry")),
				City:           cell(row, col("city")),
				State:          cell(row, col("state")),
				ECNumber:       cell(row, col("ec_number")),
				EconomicGroup:  cell(row, col("economic_group")),
				Representative: cell(row, col("representative")),
				Instagram:      cell(row, col("instagram")),
			}
			if company.Name == "" {
				company.Name = cell(row, col("cnpj"))
			}
			if err := models.CreateCompany(db, company); err != nil {
				addRowError(record, line, err.Error())
				continue
			}
			record.NewRecords++
		case err != nil:
			addRowError(record, line, err.Error())
		default:
			company, err := models.CompanyByID(db, existingID)
			if err != nil {
				addRowError(record, line, err.Error())
				continue
			}
			setIf := func(dst *string, v string) {
				if v != "" {
					*dst = v
				}
			}
			setIf(&company.Name, name)
			setIf(&company.CNPJ, cell(row, col("cnpj")))
			setIf(&company.Domain, cell(row, col("domain")))
			setIf(&company.Phone, cell(row, col("phone")))
			setIf(&company.Industry, cell(row, col("industry")))
			setIf(&company.City, cell(row, col("city")))
			setIf(&company.State, cell(row, col("state")))
			setIf(&company.ECNumber, cell(row, col("ec_number")))
			setIf(&company.EconomicGroup, cell(row, col("economic_group")))
			setIf(&company.Representative, cell(row, col("representative")))
			setIf(&company.Instagram, cell(row, col("instagram")))
			if err := models.UpdateCompany(db, company); err != nil {
				addRowError(record, line, err.Error())
				continue
			}
			record.UpdatedRecords++
		}
	}
	return nil
}
