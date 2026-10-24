package enriquecimento

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/armando-couto/crm-ia/app/utils"
)

const fixtureSerasa = `{
  "reports": [{
    "reportName": "RELATORIO_AVANCADO_PJ_PME",
    "registration": {"companyName": "ACME VENDAS LTDA", "razaoSocial": "ACME VENDAS LTDA", "situacao": {"descricao": "Ativa", "data": "10/03/2015"},
                     "dataFundacao": "10/03/2015", "capitalSocial": "100.000,00"},
    "score": {"scorePositivo": {"pontuacao": 720, "faixa": "B", "mensagem": "Baixo risco de inadimplência"}},
    "negativeData": {
      "pendenciasFinanceiras": {"quantidade": 2, "valorTotal": 3500.50, "ocorrencias": [{"valor": 2000}, {"valor": 1500.5}]},
      "protestos": {"quantidade": 1, "valorTotal": 800},
      "chequesSemFundo": {"quantidade": 0},
      "acoesJudiciais": [{"valor": 12000, "tipo": "Execução"}]
    },
    "consultas": {"quantidade": 3, "ultimos90Dias": true},
    "limiteCredito": {"valor": 50000},
    "participacoes": [{"nome": "FULANO DE TAL", "cpf": "***123456**", "cargo": "Sócio-Administrador", "percentual": 60, "dataEntrada": "10/03/2015"}]
  }]
}`

func TestSerasaLoginEParse(t *testing.T) {
	var logins atomic.Int32
	var authRecebido, bearerRecebido, caminhoRelatorio string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/security/iam/v1/client-identities/login" && r.Method == http.MethodPost:
			logins.Add(1)
			authRecebido = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"accessToken":"tok-123","tokenType":"Bearer","expiresIn":"3600"}`))
		case strings.HasPrefix(r.URL.Path, "/credit-services/business-information-report/v1/reports"):
			bearerRecebido = r.Header.Get("Authorization")
			caminhoRelatorio = r.URL.RequestURI()
			_, _ = w.Write([]byte(fixtureSerasa))
		default:
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()
	utils.Cache = utils.NovoMemoria()
	t.Cleanup(func() { utils.Cache = utils.NovoMemoria() })

	s := &Serasa{ClientID: "cid", ClientSecret: "segredo", BaseURL: ts.URL, ReportName: "RELATORIO_AVANCADO_PJ_PME",
		OptionalFeatures: "PROTESTOS", PrecoCentavos: 1500}
	p, b, err := s.Consultar(context.Background(), cnpjTeste, Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	esperado := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:segredo"))
	if authRecebido != esperado {
		t.Errorf("Authorization do login = %q", authRecebido)
	}
	if bearerRecebido != "Bearer tok-123" {
		t.Errorf("Bearer do relatório = %q", bearerRecebido)
	}
	if !strings.Contains(caminhoRelatorio, "reportName=RELATORIO_AVANCADO_PJ_PME") || !strings.Contains(caminhoRelatorio, "optionalFeatures=PROTESTOS") ||
		!strings.Contains(caminhoRelatorio, "documento="+cnpjTeste) {
		t.Errorf("caminho do relatório = %q", caminhoRelatorio)
	}
	if p.CustoCentavos != 1500 || b.Fonte != "serasa" || len(b.Corpo) == 0 {
		t.Errorf("custo/bruto = %d / %+v", p.CustoCentavos, b)
	}
	r := p.Risco
	if r == nil {
		t.Fatal("risco não preenchido")
	}
	if r.Score != 720 || r.Faixa != "B" || r.Nivel != "baixo" || r.Observacao == "" {
		t.Errorf("score = %+v", r)
	}
	if r.PendenciasQtd != 2 || r.PendenciasValor != 3500.50 || r.ProtestosQtd != 1 || r.ChequesSemFundo != 0 ||
		r.AcoesQtd != 1 || r.Consultas90d != 3 || r.LimiteSugerido != 50000 {
		t.Errorf("anotações = %+v", r)
	}
	if p.Texto("legal_name") != "ACME VENDAS LTDA" || p.Texto("registration_status") != "ATIVA" ||
		p.Texto("registration_status_at") != "2015-03-10" || p.Texto("founded_at") != "2015-03-10" || p.Campos["capital"] != 100000.0 {
		t.Errorf("campos = %+v", p.Campos)
	}
	if len(p.Socios) != 1 || p.Socios[0].Nome != "FULANO DE TAL" || !strings.Contains(p.Socios[0].Qualificacao, "60%") || p.Socios[0].Desde != "2015-03-10" {
		t.Errorf("sócios = %+v", p.Socios)
	}
	temRisco := false
	for _, s := range p.Sinais {
		if s.Tipo == "risco" && s.Nivel == "ok" {
			temRisco = true
		}
	}
	if !temRisco {
		t.Errorf("sinais = %+v", p.Sinais)
	}

	// Token em cache: segunda consulta não faz login de novo.
	if _, _, err := s.Consultar(context.Background(), cnpjTeste, Opcoes{}); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 1 {
		t.Errorf("logins = %d, esperado 1 (token em cache)", logins.Load())
	}
}

func TestSerasaCredencialRecusada(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer ts.Close()
	utils.Cache = utils.NovoMemoria()
	s := &Serasa{ClientID: "cid", ClientSecret: "x", BaseURL: ts.URL}
	_, _, err := s.Consultar(context.Background(), cnpjTeste, Opcoes{})
	if TipoDaFalha(err) != FalhaCredencial {
		t.Errorf("401 no login → %v", err)
	}
}

func TestSerasaParseSemChavesConhecidas(t *testing.T) {
	p, err := ParseSerasa([]byte(`{"foo":{"bar":1}}`), cnpjTeste)
	if err != nil {
		t.Fatal(err)
	}
	if p.Risco != nil || p.Mensagem == "" {
		t.Errorf("sem chaves: risco=%+v mensagem=%q", p.Risco, p.Mensagem)
	}
	if _, err := ParseSerasa([]byte(`xx`), cnpjTeste); err == nil {
		t.Error("json inválido deveria falhar")
	}
}
