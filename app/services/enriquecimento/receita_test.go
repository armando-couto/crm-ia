package enriquecimento

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

const cnpjTeste = "11222333000181"

const fixtureBrasilAPI = `{
  "cnpj": "11222333000181", "razao_social": "ACME VENDAS LTDA", "nome_fantasia": "ACME",
  "descricao_situacao_cadastral": "ATIVA", "data_situacao_cadastral": "2015-03-10",
  "data_inicio_atividade": "2015-03-10", "cnae_fiscal": 6201501,
  "cnae_fiscal_descricao": "Desenvolvimento de programas de computador sob encomenda",
  "cnaes_secundarios": [{"codigo": 6202300, "descricao": "Desenvolvimento e licenciamento de programas customizáveis"}],
  "natureza_juridica": "Sociedade Empresária Limitada", "porte": "MICRO EMPRESA", "capital_social": 100000,
  "descricao_tipo_de_logradouro": "RUA", "logradouro": "DAS FLORES", "numero": "100", "complemento": "SALA 2",
  "bairro": "CENTRO", "cep": "60000000", "uf": "CE", "municipio": "FORTALEZA",
  "ddd_telefone_1": "8533334444", "email": "CONTATO@ACME.COM.BR",
  "opcao_pelo_simples": true, "opcao_pelo_mei": false,
  "qsa": [{"nome_socio": "FULANO DE TAL", "cnpj_cpf_do_socio": "***123456**", "qualificacao_socio": "Sócio-Administrador",
           "data_entrada_sociedade": "2015-03-10", "identificador_de_socio": 2}]
}`

const fixtureOpenCNPJ = `{
  "cnpj": "11222333000181", "razao_social": "ACME VENDAS LTDA", "nome_fantasia": "ACME",
  "situacao_cadastral": "Ativa", "data_situacao_cadastral": "2015-03-10", "data_inicio_atividade": "2015-03-10",
  "cnae_principal": "6201501", "cnaes_secundarios": ["6202300"],
  "natureza_juridica": "Sociedade Empresária Limitada", "porte": "Microempresa (ME)", "capital_social": "100000.00",
  "logradouro": "DAS FLORES", "tipo_logradouro": "RUA", "numero": "100", "bairro": "CENTRO", "cep": "60000000",
  "uf": "CE", "municipio": "FORTALEZA", "email": "contato@acme.com.br",
  "telefones": [{"ddd": "85", "numero": "33334444", "is_fax": false}],
  "opcao_simples": "Sim", "opcao_mei": "Não",
  "QSA": [{"nome_socio": "FULANO DE TAL", "cnpj_cpf_socio": "***123456**", "qualificacao_socio": "Sócio-Administrador",
           "data_entrada_sociedade": "2015-03-10", "identificador_socio": "Pessoa Física"}]
}`

const fixtureCNPJWS = `{
  "razao_social": "ACME VENDAS LTDA", "capital_social": "100000.00",
  "natureza_juridica": {"id": "2062", "descricao": "Sociedade Empresária Limitada"},
  "porte": {"id": "01", "descricao": "Micro Empresa"},
  "simples": {"simples": "Sim", "mei": "Não"},
  "socios": [{"nome": "FULANO DE TAL", "cpf_cnpj_socio": "***123456**", "tipo": "Pessoa Física",
              "data_entrada": "2015-03-10", "qualificacao_socio": {"id": 49, "descricao": "Sócio-Administrador"}}],
  "estabelecimento": {
    "cnpj": "11222333000181", "nome_fantasia": "ACME", "situacao_cadastral": "Ativa",
    "data_situacao_cadastral": "2015-03-10", "data_inicio_atividade": "2015-03-10",
    "tipo_logradouro": "RUA", "logradouro": "DAS FLORES", "numero": "100", "bairro": "CENTRO", "cep": "60000000",
    "ddd1": "85", "telefone1": "33334444", "email": "contato@acme.com.br",
    "atividade_principal": {"id": "6201501", "subclasse": "6201501", "descricao": "Desenvolvimento de programas de computador sob encomenda"},
    "atividades_secundarias": [{"id": "6202300", "descricao": "Licenciamento"}],
    "estado": {"sigla": "CE"}, "cidade": {"nome": "Fortaleza"}
  }
}`

const fixtureCEP = `{"cep":"60000000","state":"CE","city":"Fortaleza","neighborhood":"Centro","street":"Rua das Flores",
  "service":"viacep","location":{"type":"Point","coordinates":{"longitude":"-38.5267","latitude":"-3.7319"}}}`

// servidorReceita simula as três fontes públicas e o CEP num só httptest.
type servidorReceita struct {
	brasilapi, opencnpj, cnpjws int
	chamadas                    atomic.Int32
}

func (s *servidorReceita) handler(w http.ResponseWriter, r *http.Request) {
	s.chamadas.Add(1)
	switch {
	case r.URL.Path == "/api/cnpj/v1/"+cnpjTeste:
		w.WriteHeader(s.brasilapi)
		if s.brasilapi == 200 {
			_, _ = w.Write([]byte(fixtureBrasilAPI))
		}
	case r.URL.Path == "/"+cnpjTeste:
		w.WriteHeader(s.opencnpj)
		if s.opencnpj == 200 {
			_, _ = w.Write([]byte(fixtureOpenCNPJ))
		}
	case r.URL.Path == "/cnpj/"+cnpjTeste:
		w.WriteHeader(s.cnpjws)
		if s.cnpjws == 200 {
			_, _ = w.Write([]byte(fixtureCNPJWS))
		}
	case r.URL.Path == "/api/cep/v2/60000000":
		_, _ = w.Write([]byte(fixtureCEP))
	default:
		w.WriteHeader(404)
	}
}

func apontarFontes(t *testing.T, url string) {
	t.Helper()
	antigos := []string{BaseBrasilAPI, BaseOpenCNPJ, BaseCNPJWS, BaseViaCEP}
	BaseBrasilAPI, BaseOpenCNPJ, BaseCNPJWS, BaseViaCEP = url, url, url, url
	utils.Cache = utils.NovoMemoria()
	t.Cleanup(func() {
		BaseBrasilAPI, BaseOpenCNPJ, BaseCNPJWS, BaseViaCEP = antigos[0], antigos[1], antigos[2], antigos[3]
		utils.Cache = utils.NovoMemoria()
	})
}

func conferirPerfilACME(t *testing.T, p *Perfil, comDescricaoCNAE bool) {
	t.Helper()
	esperado := map[string]any{
		"legal_name": "ACME VENDAS LTDA", "trade_name": "ACME", "name": "ACME",
		"cnae_code": "6201-5/01", "legal_nature": "Sociedade Empresária Limitada",
		"founded_at": "2015-03-10", "capital": 100000.0, "company_size": "ME",
		"registration_status": "ATIVA", "registration_status_at": "2015-03-10",
		"simples": true, "mei": false, "street": "Rua das Flores", "street_number": "100",
		"neighborhood": "Centro", "zip": "60000000", "city": "Fortaleza", "state": "CE",
		"phone": "(85) 3333-4444", "email": "contato@acme.com.br",
		"website": "https://acme.com.br", "domain": "acme.com.br", "cnpj": "11.222.333/0001-81",
	}
	for k, v := range esperado {
		if got := p.Campos[k]; got != v {
			t.Errorf("campo %s = %#v, esperado %#v", k, got, v)
		}
	}
	// OpenCNPJ devolve só o código do CNAE, sem descrição.
	if comDescricaoCNAE && (p.Texto("industry") == "" || p.Texto("cnae_description") == "") {
		t.Errorf("industry/cnae_description vazios: %q / %q", p.Texto("industry"), p.Texto("cnae_description"))
	}
	if len(p.Socios) != 1 || p.Socios[0].Nome != "FULANO DE TAL" || p.Socios[0].Documento != "***123456**" ||
		p.Socios[0].Tipo != "PF" || p.Socios[0].Qualificacao != "Sócio-Administrador" || p.Socios[0].Desde != "2015-03-10" {
		t.Errorf("sócios = %+v", p.Socios)
	}
	if len(p.CNAEsSecundarios) != 1 || p.CNAEsSecundarios[0].Codigo != "6202-3/00" {
		t.Errorf("cnaes secundários = %+v", p.CNAEsSecundarios)
	}
}

func TestReceitaBrasilAPI(t *testing.T) {
	srv := &servidorReceita{brasilapi: 200, opencnpj: 200, cnpjws: 200}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()
	apontarFontes(t, ts.URL)

	r := &Receita{PrazoCache: time.Hour}
	p, b, err := r.Consultar(context.Background(), "11.222.333/0001-81", Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	conferirPerfilACME(t, p, true)
	if b.Fonte != "brasilapi" || b.Cache {
		t.Errorf("bruto = %+v", b)
	}
	if p.Localizacao == nil || p.Localizacao.Lat != -3.7319 || p.Localizacao.Lng != -38.5267 {
		t.Errorf("localização do CEP = %+v", p.Localizacao)
	}
	if len(p.Sinais) == 0 || p.Sinais[0].Tipo != "situacao" || p.Sinais[0].Nivel != "ok" {
		t.Errorf("sinais = %+v", p.Sinais)
	}

	// Segunda consulta: vem do cache, sem bater na rede.
	antes := srv.chamadas.Load()
	p2, b2, err := r.Consultar(context.Background(), cnpjTeste, Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	if srv.chamadas.Load() != antes || !b2.Cache || p2.Texto("legal_name") != "ACME VENDAS LTDA" {
		t.Errorf("cache não foi usado (chamadas %d→%d, cache=%v)", antes, srv.chamadas.Load(), b2.Cache)
	}
	// forcar ignora o cache.
	if _, b3, err := r.Consultar(context.Background(), cnpjTeste, Opcoes{Forcar: true}); err != nil || b3.Cache {
		t.Errorf("forcar deveria consultar de novo: err=%v cache=%v", err, b3 != nil && b3.Cache)
	}
}

func TestReceitaFallbackOpenCNPJ(t *testing.T) {
	srv := &servidorReceita{brasilapi: 500, opencnpj: 200, cnpjws: 200}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()
	apontarFontes(t, ts.URL)

	p, b, err := (&Receita{}).Consultar(context.Background(), cnpjTeste, Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Fonte != "opencnpj" {
		t.Errorf("fonte = %s", b.Fonte)
	}
	conferirPerfilACME(t, p, false)
}

func TestReceitaFallbackCNPJWS(t *testing.T) {
	srv := &servidorReceita{brasilapi: 429, opencnpj: 503, cnpjws: 200}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()
	apontarFontes(t, ts.URL)

	p, b, err := (&Receita{}).Consultar(context.Background(), cnpjTeste, Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Fonte != "cnpjws" {
		t.Errorf("fonte = %s", b.Fonte)
	}
	conferirPerfilACME(t, p, true)
}

func TestReceitaSemResultadoEIndisponivel(t *testing.T) {
	srv := &servidorReceita{brasilapi: 404, opencnpj: 200, cnpjws: 200}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()
	apontarFontes(t, ts.URL)

	_, _, err := (&Receita{}).Consultar(context.Background(), cnpjTeste, Opcoes{})
	if TipoDaFalha(err) != FalhaSemResultado {
		t.Errorf("404 deveria ser sem_resultado, veio %v", err)
	}

	srv.brasilapi, srv.opencnpj, srv.cnpjws = 500, 500, 500
	_, _, err = (&Receita{}).Consultar(context.Background(), cnpjTeste, Opcoes{Forcar: true})
	if err == nil || TipoDaFalha(err) != FalhaExterna {
		t.Errorf("todas fora deveria ser falha externa, veio %v", err)
	}

	if _, _, err := (&Receita{}).Consultar(context.Background(), "123", Opcoes{}); TipoDaFalha(err) != FalhaEntrada {
		t.Errorf("CNPJ inválido deveria ser falha de entrada, veio %v", err)
	}
}

func TestParsersDiretos(t *testing.T) {
	for nome, fn := range map[string]func([]byte) (*Perfil, error){
		"brasilapi": ParseBrasilAPI, "opencnpj": ParseOpenCNPJ, "cnpjws": ParseCNPJWS,
	} {
		fixture := map[string]string{"brasilapi": fixtureBrasilAPI, "opencnpj": fixtureOpenCNPJ, "cnpjws": fixtureCNPJWS}[nome]
		p, err := fn([]byte(fixture))
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if p.Provedor != models.ProvedorReceita || p.Texto("legal_name") != "ACME VENDAS LTDA" || p.Texto("city") != "Fortaleza" {
			t.Errorf("%s: perfil = %+v", nome, p.Campos)
		}
		if _, err := fn([]byte(`{"message":"erro"}`)); err == nil {
			t.Errorf("%s: resposta sem dados deveria falhar", nome)
		}
	}
}
