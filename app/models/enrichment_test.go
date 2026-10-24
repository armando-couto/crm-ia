package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Segredos cifrados nunca saem na API: só has_secret.
func TestEnrichmentSettingsPublicaNaoExpoeSegredos(t *testing.T) {
	s := DefaultEnrichmentSettings()
	s.BigDataCorp.Habilitado = true
	s.BigDataCorp.TokenID = "tid"
	s.BigDataCorp.AccessToken = "em-claro"
	s.BigDataCorp.AccessTokenCifrado = "cifrado-bdc"
	s.Serasa.ClientID = "cid"
	s.Serasa.ClientSecret = "em-claro"
	s.Serasa.ClientSecretCifrado = "cifrado-serasa"

	pub := s.Publica()
	b, _ := json.Marshal(pub)
	js := string(b)
	for _, proibido := range []string{"em-claro", "cifrado-bdc", "cifrado-serasa", "access_token_cifrado", "client_secret_cifrado", "\"access_token\"", "\"client_secret\""} {
		if strings.Contains(js, proibido) {
			t.Errorf("JSON público contém %q: %s", proibido, js)
		}
	}
	if !pub.BigDataCorp.HasSecret || !pub.Serasa.HasSecret {
		t.Error("has_secret deveria ser true")
	}
	for _, chave := range []string{"auto_receita", "sobrescrever", "limite_mensal_centavos", "prazo_cache_dias", "bigdatacorp", "serasa"} {
		if !strings.Contains(js, "\""+chave+"\"") {
			t.Errorf("JSON público sem %q", chave)
		}
	}
	// O original não é alterado e o que vai ao banco não leva o texto em claro.
	if s.BigDataCorp.AccessTokenCifrado != "cifrado-bdc" {
		t.Error("Publica alterou o original")
	}
	gravar := s.ParaGravar()
	if gravar.BigDataCorp.AccessToken != "" || gravar.Serasa.ClientSecret != "" || gravar.BigDataCorp.AccessTokenCifrado == "" {
		t.Errorf("ParaGravar = %+v", gravar)
	}
	if gravar.BigDataCorpPronto() == false {
		t.Error("habilitado com credenciais deveria estar pronto")
	}
	if gravar.SerasaPronto() {
		t.Error("serasa desabilitado não está pronto")
	}
}

func TestEnrichmentSettingsValidate(t *testing.T) {
	s := DefaultEnrichmentSettings()
	if err := s.Validate(); err != nil {
		t.Fatalf("padrão deveria validar: %v", err)
	}
	if s.CustoBigDataCorp(BigDataCorpDatasetsPadrao) != 27 {
		t.Errorf("custo padrão = %d", s.CustoBigDataCorp(BigDataCorpDatasetsPadrao))
	}
	s.Sobrescrever = "talvez"
	if err := s.Validate(); err == nil {
		t.Error("sobrescrever inválido deveria falhar")
	}
	s = DefaultEnrichmentSettings()
	s.BigDataCorp.DatasetsPadrao = []string{"basic_data", "xpto"}
	if err := s.Validate(); err == nil {
		t.Error("dataset desconhecido deveria falhar")
	}
	s = DefaultEnrichmentSettings()
	s.Serasa.Ambiente = "homolog"
	if err := s.Validate(); err == nil {
		t.Error("ambiente inválido deveria falhar")
	}
	s = DefaultEnrichmentSettings()
	s.PrazoCacheDias = 0
	if err := s.Validate(); err == nil {
		t.Error("prazo 0 deveria falhar")
	}
	// Preços parciais completam com o padrão.
	s = EnrichmentSettings{BigDataCorp: BigDataCorpSettings{PrecosCentavos: map[string]int{"basic_data": 5}}}
	s.completar()
	if s.BigDataCorp.PrecosCentavos["basic_data"] != 5 || s.BigDataCorp.PrecosCentavos["processes"] != 8 || s.PrazoCacheDias != 30 || s.Sobrescrever != "vazios" {
		t.Errorf("completar = %+v", s)
	}
}

func TestDateJSON(t *testing.T) {
	d, err := ParseDate("2015-03-10")
	if err != nil || d == nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(struct {
		A *Date `json:"a"`
		B *Date `json:"b"`
	}{A: d})
	if string(b) != `{"a":"2015-03-10","b":null}` {
		t.Errorf("json = %s", b)
	}
	var volta struct {
		A *Date `json:"a"`
	}
	if err := json.Unmarshal([]byte(`{"a":"2015-03-10T00:00:00Z"}`), &volta); err != nil || volta.A.String() != "2015-03-10" {
		t.Errorf("unmarshal = %+v %v", volta.A, err)
	}
	if _, err := ParseDate("ontem"); err == nil {
		t.Error("data inválida deveria falhar")
	}
	var vazio Date
	if err := vazio.Scan(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)); err != nil || vazio.String() != "2020-01-02" {
		t.Errorf("scan = %v %v", vazio, err)
	}
	var j JSONB
	if err := j.Scan([]byte(`{"x":1}`)); err != nil || string(j) != `{"x":1}` {
		t.Errorf("jsonb scan = %s %v", j, err)
	}
	if b, _ := json.Marshal(JSONB(nil)); string(b) != "{}" {
		t.Errorf("jsonb vazio = %s", b)
	}
}

func TestApplyEnrichedFieldsSoListaBranca(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE companies SET legal_name = \\$1, founded_at = \\$2, capital = \\$3, simples = \\$4, updated_at = NOW\\(\\) WHERE id = \\$5").
		WithArgs("ACME LTDA", time.Date(2015, 3, 10, 0, 0, 0, 0, time.UTC), 100000.0, true, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = ApplyEnrichedFields(db, 7, map[string]any{
		"legal_name": " ACME LTDA ", "founded_at": "2015-03-10", "capital": 100000.0, "simples": true,
		"owner_id": 99, "intel": "{}", "id": 1, // ignorados
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	// Nada na lista branca: nenhuma consulta.
	if err := ApplyEnrichedFields(db, 7, map[string]any{"owner_id": 1}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyEnrichedFields(db, 7, map[string]any{"founded_at": "xx"}); err == nil {
		t.Error("data inválida deveria falhar")
	}
}

func TestCompanyByCNPJUsaSoDigitos(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	cols := []string{"id", "name", "domain", "phone", "industry", "city", "state", "owner_id", "owner_name", "created_at", "updated_at",
		"ec_number", "economic_group", "cnpj", "accredited_at", "representative", "instagram", "products", "machines_count", "is_client",
		"anticipation_mode", "validator", "do_not_disturb", "is_target", "target_tier", "target_notes", "target_since",
		"legal_name", "trade_name", "cnae_code", "cnae_description", "legal_nature", "founded_at", "capital", "company_size",
		"registration_status", "registration_status_at", "simples", "mei", "street", "street_number", "complement", "neighborhood",
		"zip", "email", "website", "employees_range", "revenue_range", "intel", "enriched_at"}
	mock.ExpectQuery("regexp_replace").WithArgs("11222333000181").WillReturnRows(sqlmock.NewRows(cols).AddRow(
		3, "ACME", "", "", "", "", "", nil, "", now, now,
		"", "", "11.222.333/0001-81", nil, "", "", []byte("{}"), 0, false, "", false, false, false, 0, "", nil,
		"ACME LTDA", "", "", "", "", time.Date(2015, 3, 10, 0, 0, 0, 0, time.UTC), "100000.00", "ME", "ATIVA", nil, true, nil,
		"", "", "", "", "", "", "", "", "", []byte(`{"provedor":"receita"}`), nil))

	c, err := CompanyByCNPJ(db, "11.222.333/0001-81")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != 3 || c.LegalName != "ACME LTDA" || c.FoundedAt == nil || c.FoundedAt.String() != "2015-03-10" ||
		c.Capital == nil || *c.Capital != 100000 || c.Simples == nil || !*c.Simples || c.MEI != nil || string(c.Intel) != `{"provedor":"receita"}` {
		t.Errorf("empresa = %+v", c)
	}
	b, _ := json.Marshal(c)
	for _, trecho := range []string{`"founded_at":"2015-03-10"`, `"intel":{"provedor":"receita"}`, `"mei":null`, `"capital":100000`} {
		if !strings.Contains(string(b), trecho) {
			t.Errorf("JSON sem %s: %s", trecho, b)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrichmentUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM company_enrichments").WillReturnRows(sqlmock.NewRows([]string{"provedor", "count", "sum"}).
		AddRow("receita", 12, 0).AddRow("bigdatacorp", 3, 81))
	uso, err := EnrichmentUsage(db, time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if uso.Mes != "2026-10" || uso.Consultas != 15 || uso.CustoCentavos != 81 || uso.PorProvedor["bigdatacorp"].CustoCentavos != 81 {
		t.Errorf("uso = %+v", uso)
	}
}
