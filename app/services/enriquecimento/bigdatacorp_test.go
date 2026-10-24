package enriquecimento

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/armando-couto/crm-ia/app/models"
)

const fixtureBDC = `{
 "Result": [{
   "MatchKeys": "doc{11222333000181}",
   "BasicData": {
     "TaxIdNumber": "11222333000181", "OfficialName": "ACME VENDAS LTDA", "TradeName": "ACME",
     "TaxIdStatus": "ATIVA", "TaxIdStatusDate": "2015-03-10T00:00:00Z", "FoundedDate": "2015-03-10T00:00:00Z",
     "LegalNature": {"Code": "2062", "Activity": "Sociedade Empresária Limitada"},
     "Activities": [{"Code": "6201501", "Activity": "Desenvolvimento de programas", "IsMain": true},
                    {"Code": "6202300", "Activity": "Licenciamento", "IsMain": false}]
   },
   "RegistrationData": {
     "Emails": {"Primary": {"EmailAddress": "contato@acme.com.br"}},
     "Phones": {"Primary": {"AreaCode": "85", "Number": "33334444"}},
     "Addresses": {"Primary": {"Typology": "RUA", "AddressMain": "DAS FLORES", "Number": "100", "Neighborhood": "CENTRO",
                               "ZipCode": "60000000", "City": "FORTALEZA", "State": "CE", "Latitude": -3.73, "Longitude": -38.52}}
   },
   "ActivityIndicators": {"EmployeesRange": "10 a 49", "IncomeRange": "R$ 1M a 5M", "ActivityLevel": "ALTA",
                          "HasActiveDomain": true, "HasActiveSSL": true, "HasCorporateEmail": true},
   "Relationships": {"CurrentRelationships": [{"RelatedEntityName": "FULANO DE TAL", "RelatedEntityTaxIdNumber": "***123456**",
                     "RelatedEntityTaxIdType": "CPF", "RelationshipType": "QSA", "RelationshipLevel": "SOCIO-ADMINISTRADOR",
                     "RelationshipStartDate": "2015-03-10T00:00:00Z"}]},
   "ExtendedPhones": {"Phones": [{"CountryCode": "55", "AreaCode": "85", "Number": "999990000", "Type": "MOBILE", "IsMain": true, "IsActive": true}]},
   "ExtendedEmails": {"Emails": [{"EmailAddress": "vendas@acme.com.br", "Type": "CORPORATE", "IsActive": true}]},
   "ExtendedDomains": {"Domains": [{"DomainName": "acme.com.br", "IsActive": true, "HasSSL": true}]},
   "Processes": {"TotalLawsuits": 2, "TotalLawsuitsAsDefendant": 1,
                 "Lawsuits": [{"Number": "0001", "Type": "Cível", "CourtName": "TJCE", "Status": "ATIVO", "Value": 20000,
                               "PublicationDate": "2026-01-10T00:00:00Z",
                               "Parties": [{"Doc": "11222333000181", "Polarity": "PASSIVE"}]},
                              {"Number": "0002", "Type": "Trabalhista", "CourtName": "TRT7", "Status": "ARQUIVADO", "Value": 15000,
                               "PublicationDate": "2024-05-01T00:00:00Z",
                               "Parties": [{"Doc": "11222333000181", "Polarity": "ACTIVE"}]}]},
   "KycData": {"IsCurrentlyPEP": false, "IsCurrentlySanctioned": false, "SanctionsHistory": []},
   "MediaProfileAndExposure": {"MediaExposureLevel": "LOW", "NewsItems": [{"Title": "ACME cresce", "Url": "https://x/1", "Sentiment": "POSITIVE", "PublicationDate": "2026-02-01"}]}
 }],
 "QueryId": "abc", "ElapsedMilliseconds": 10,
 "Status": {"basic_data": [{"Code": 0, "Message": "OK"}], "registration_data": [{"Code": 0, "Message": "OK"}]}
}`

func TestBigDataCorpConsulta(t *testing.T) {
	var recebido map[string]any
	var cabecalhos http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/empresas" || r.Method != http.MethodPost {
			w.WriteHeader(404)
			return
		}
		cabecalhos = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &recebido)
		_, _ = w.Write([]byte(fixtureBDC))
	}))
	defer ts.Close()

	bdc := &BigDataCorp{TokenID: "tid", AccessToken: "tok", BaseURL: ts.URL,
		DatasetsPadrao: []string{"basic_data", "registration_data"}, Precos: models.PrecosBigDataCorpPadrao()}
	p, b, err := bdc.Consultar(context.Background(), cnpjTeste, Opcoes{Datasets: []string{"basic_data", "registration_data", "activity_indicators", "relationships"}})
	if err != nil {
		t.Fatal(err)
	}
	if cabecalhos.Get("AccessToken") != "tok" || cabecalhos.Get("TokenId") != "tid" {
		t.Errorf("cabeçalhos = %v", cabecalhos)
	}
	if recebido["q"] != "doc{11222333000181}" || recebido["Datasets"] != "basic_data,registration_data,activity_indicators,relationships" || recebido["Limit"] != float64(1) {
		t.Errorf("corpo enviado = %v", recebido)
	}
	if p.CustoCentavos != 3+14+6+4 {
		t.Errorf("custo = %d", p.CustoCentavos)
	}
	if b.Fonte != models.ProvedorBigDataCorp || len(b.Corpo) == 0 {
		t.Errorf("bruto = %+v", b)
	}
	for k, v := range map[string]any{
		"legal_name": "ACME VENDAS LTDA", "trade_name": "ACME", "registration_status": "ATIVA",
		"founded_at": "2015-03-10", "cnae_code": "6201-5/01", "legal_nature": "Sociedade Empresária Limitada",
		"email": "contato@acme.com.br", "phone": "(85) 3333-4444", "street": "Rua das Flores", "city": "Fortaleza",
		"state": "CE", "zip": "60000000", "employees_range": "10 a 49", "revenue_range": "R$ 1M a 5M",
		"domain": "acme.com.br", "website": "https://acme.com.br",
	} {
		if p.Campos[k] != v {
			t.Errorf("campo %s = %#v, esperado %#v", k, p.Campos[k], v)
		}
	}
	if p.Indicadores == nil || p.Indicadores.NivelAtividade != 0.85 || !p.Indicadores.TemSSL || p.Indicadores.FuncionariosFaixa != "10 a 49" {
		t.Errorf("indicadores = %+v", p.Indicadores)
	}
	if len(p.Socios) != 1 || p.Socios[0].Qualificacao != "Socio Administrador" || p.Socios[0].Tipo != "PF" {
		t.Errorf("sócios = %+v", p.Socios)
	}
	if p.Contatos == nil || len(p.Contatos.Telefones) != 1 || p.Contatos.Telefones[0].Numero != "+5585999990000" || !p.Contatos.Telefones[0].Validado ||
		len(p.Contatos.Emails) != 1 || p.Contatos.Emails[0].Tipo != "corporativo" {
		t.Errorf("contatos = %+v", p.Contatos)
	}
	if p.PresencaDigital == nil || len(p.PresencaDigital.Dominios) != 1 || !p.PresencaDigital.Dominios[0].SSL {
		t.Errorf("presença digital = %+v", p.PresencaDigital)
	}
	if p.Processos == nil || p.Processos.Total != 2 || p.Processos.ComoReu != 1 || p.Processos.Ativos != 1 ||
		p.Processos.ValorTotal != 35000 || p.Processos.Ultimos[0].Polo != "Réu" || p.Processos.Ultimos[1].Polo != "Autor" {
		t.Errorf("processos = %+v", p.Processos)
	}
	if p.Conformidade == nil || p.Conformidade.PEP || p.Conformidade.Sancionado {
		t.Errorf("conformidade = %+v", p.Conformidade)
	}
	if p.Midia == nil || p.Midia.Exposicao != "baixa" || len(p.Midia.Noticias) != 1 || p.Midia.Noticias[0].Sentimento != "positivo" {
		t.Errorf("mídia = %+v", p.Midia)
	}
	if p.Localizacao == nil || p.Localizacao.Lat != -3.73 {
		t.Errorf("localização = %+v", p.Localizacao)
	}
	if len(p.CNAEsSecundarios) != 1 {
		t.Errorf("cnaes secundários = %+v", p.CNAEsSecundarios)
	}
}

func TestBigDataCorpStatus(t *testing.T) {
	casos := []struct {
		nome, corpo, tipo string
	}{
		{"sem resultado", `{"Result":[],"Status":{"basic_data":[{"Code":-204,"Message":"NO RESULTS"}]}}`, FalhaSemResultado},
		{"credencial", `{"Result":[],"Status":{"basic_data":[{"Code":-111,"Message":"INVALID TOKEN"}]}}`, FalhaCredencial},
		{"credencial 1003", `{"Result":[],"Status":{"login":[{"Code":-1003,"Message":"ACCESS DENIED"}]}}`, FalhaCredencial},
		{"limite", `{"Result":[],"Status":{"basic_data":[{"Code":-1214,"Message":"RATE LIMIT"}]}}`, FalhaLimite},
		{"dataset", `{"Result":[],"Status":{"xpto":[{"Code":-109,"Message":"DATASET NOT FOUND"}]}}`, FalhaExterna},
		{"resultado vazio sem status", `{"Result":[]}`, FalhaSemResultado},
		{"json inválido", `nao é json`, FalhaExterna},
	}
	for _, c := range casos {
		_, err := ParseBigDataCorp([]byte(c.corpo), cnpjTeste, []string{"basic_data"})
		if err == nil || TipoDaFalha(err) != c.tipo {
			t.Errorf("%s: esperado %s, veio %v", c.nome, c.tipo, err)
		}
	}
	// Dataset com erro mas Result presente: vira aviso, não falha.
	p, err := ParseBigDataCorp([]byte(`{"Result":[{"BasicData":{"OfficialName":"X LTDA"}}],
		"Status":{"basic_data":[{"Code":0,"Message":"OK"}],"processes":[{"Code":-110,"Message":"NOT CONTRACTED"}]}}`), cnpjTeste, nil)
	if err != nil || p.Texto("legal_name") != "X LTDA" || p.Mensagem == "" {
		t.Errorf("aviso de dataset: p=%+v err=%v", p, err)
	}
}

func TestBigDataCorpHTTP(t *testing.T) {
	status := 401
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	defer ts.Close()
	bdc := &BigDataCorp{TokenID: "t", AccessToken: "a", BaseURL: ts.URL}
	if _, _, err := bdc.Consultar(context.Background(), cnpjTeste, Opcoes{}); TipoDaFalha(err) != FalhaCredencial {
		t.Errorf("401 → %v", err)
	}
	status = 429
	if _, _, err := bdc.Consultar(context.Background(), cnpjTeste, Opcoes{}); TipoDaFalha(err) != FalhaLimite {
		t.Errorf("429 → %v", err)
	}
	status = 500
	if _, _, err := bdc.Consultar(context.Background(), cnpjTeste, Opcoes{}); TipoDaFalha(err) != FalhaExterna {
		t.Errorf("500 → %v", err)
	}
}
