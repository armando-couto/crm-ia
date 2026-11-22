package enriquecimento

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func lerFixture(t *testing.T, nome string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + nome)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func achar(tels []Telefone, numero string) *Telefone {
	for i := range tels {
		if tels[i].Numero == numero {
			return &tels[i]
		}
	}
	return nil
}

// Contrato do formato "consulta" (GET /companies/ da Data Stone).
func TestCnpjDataDataStoneConsulta(t *testing.T) {
	p, formato, err := ParseCnpjData(lerFixture(t, "datastone_companies_formatoA.json"), "wholead", cnpjTeste)
	if err != nil || p == nil {
		t.Fatalf("perfil=%v err=%v", p, err)
	}
	if formato != FormatoDataStoneConsulta {
		t.Fatalf("formato = %s", formato)
	}
	campos := map[string]any{
		"legal_name": "ACME VENDAS LTDA", "trade_name": "ACME", "name": "ACME",
		"registration_status": "ATIVA", "founded_at": "2015-03-10", "cnae_code": "9511-8/00",
		"legal_nature": "Sociedade Empresária Limitada", "company_size": "ME", "capital": 100000.0,
		"industry":        "Atividades Profissionais, Científicas e Técnicas",
		"employees_range": "DE 50 A 99 FUNCIONARIOS", "revenue_range": "DE R$ 100 MM ATÉ R$ 250 MM",
		"street": "Rua Das Flores", "street_number": "100", "neighborhood": "Centro", "zip": "60000000",
		"city": "Fortaleza", "state": "CE",
		"phone": "(85) 3333-4444", "email": "contato@acme.com.br", "domain": "acme.com.br", "website": "https://acme.com.br",
	}
	for campo, quer := range campos {
		if !Iguais(p.Campos[campo], quer) {
			t.Errorf("%s = %v, quer %v", campo, p.Campos[campo], quer)
		}
	}
	if p.Contatos == nil || len(p.Contatos.Telefones) != 3 || len(p.Contatos.Emails) != 2 {
		t.Fatalf("contatos = %+v", p.Contatos)
	}
	cel := achar(p.Contatos.Telefones, "+5585999990000")
	if cel == nil || cel.Tipo != "celular" || !cel.Principal || !cel.Validado || cel.WhatsApp == nil || !*cel.WhatsApp || cel.NaoPerturbe == nil || *cel.NaoPerturbe {
		t.Errorf("celular principal = %+v", cel)
	}
	bloq := achar(p.Contatos.Telefones, "+5585988880000")
	if bloq == nil || bloq.Principal || bloq.Validado || bloq.NaoPerturbe == nil || !*bloq.NaoPerturbe {
		t.Errorf("celular bloqueado = %+v", bloq)
	}
	fixo := achar(p.Contatos.Telefones, "+558533334444")
	if fixo == nil || fixo.Tipo != "fixo" || !fixo.Principal || !fixo.Validado || fixo.WhatsApp != nil || fixo.NaoPerturbe != nil {
		t.Errorf("fixo = %+v", fixo)
	}
	if e := p.Contatos.Emails; e[0].Email != "contato@acme.com.br" || e[0].Tipo != "corporativo" || e[1].Tipo != "pessoal" || e[0].Validado {
		t.Errorf("emails = %+v", e)
	}
	if len(p.Socios) != 2 || p.Socios[0].Documento != "***456789**" || p.Socios[0].Tipo != "PF" ||
		p.Socios[0].Qualificacao != "Sócio-Administrador" || p.Socios[1].Tipo != "PJ" || p.Socios[1].Documento != "99.888.777/0001-66" {
		t.Errorf("socios = %+v", p.Socios)
	}
	// Sem Indicadores: não pode nascer o sinal "Sem presença digital identificada".
	if p.Indicadores != nil {
		t.Errorf("indicadores = %+v", p.Indicadores)
	}
	for _, s := range p.Sinais {
		if s.Tipo == "digital" {
			t.Errorf("sinal indevido: %+v", s)
		}
	}
	// O JSON continua compatível com a tela: chaves novas só quando informadas.
	b, _ := json.Marshal(fixo)
	if string(b) != `{"numero":"+558533334444","tipo":"fixo","principal":true,"validado":true}` {
		t.Errorf("json do fixo = %s", b)
	}
}

// O mesmo payload dentro dos envelopes da Who Lead (hipótese).
func TestCnpjDataEnvelopesWhoLead(t *testing.T) {
	var pagina struct {
		Items []struct {
			CNPJ     string          `json:"cnpj"`
			Found    bool            `json:"found"`
			Result   string          `json:"result"`
			CnpjData json.RawMessage `json:"cnpj_data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(lerFixture(t, "wholead_enrichment_results_hipotese.json"), &pagina); err != nil {
		t.Fatal(err)
	}
	p, formato, err := ParseCnpjData(pagina.Items[0].CnpjData, "wholead", pagina.Items[0].CNPJ)
	if err != nil || formato != FormatoDataStoneConsulta || p.Texto("legal_name") != "ACME VENDAS LTDA" {
		t.Fatalf("lote: perfil=%v formato=%s err=%v", p, formato, err)
	}
	// Item não encontrado: cnpj_data nulo não é erro.
	if p, formato, err := ParseCnpjData(pagina.Items[1].CnpjData, "wholead", pagina.Items[1].CNPJ); p != nil || err != nil || formato != FormatoDesconhecido {
		t.Errorf("nulo: perfil=%v formato=%s err=%v", p, formato, err)
	}
	var sinc struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(lerFixture(t, "wholead_cnpj_lookup_hipotese.json"), &sinc); err != nil {
		t.Fatal(err)
	}
	if p, _, err := ParseCnpjData(sinc.Data, "wholead", cnpjTeste); err != nil || p == nil || p.Texto("phone") != "(85) 3333-4444" {
		t.Errorf("síncrono: perfil=%v err=%v", p, err)
	}
}

// Contrato do formato B2B (`detalhes_contato`).
func TestCnpjDataDataStoneB2B(t *testing.T) {
	var status struct {
		Contatos []struct {
			Detalhes json.RawMessage `json:"detalhes_contato"`
		} `json:"contatos"`
	}
	if err := json.Unmarshal(lerFixture(t, "datastone_b2b_status_empresa_formatoB.json"), &status); err != nil {
		t.Fatal(err)
	}
	p, formato, err := ParseCnpjData(status.Contatos[0].Detalhes, "wholead", cnpjTeste)
	if err != nil || p == nil || formato != FormatoDataStoneB2B {
		t.Fatalf("perfil=%v formato=%s err=%v", p, formato, err)
	}
	campos := map[string]any{
		"legal_name": "ACME VENDAS LTDA", "trade_name": "ACME", "registration_status": "ATIVA",
		"founded_at": "2015-03-10", "cnae_code": "9511-8/00", "company_size": "DEMAIS", "capital": 100000.0,
		"industry": "Tecnologia", "employees_range": "51-200", "revenue_range": "DE R$ 50 MM ATÉ R$ 100 MM",
		"street": "Rua Das Flores", "street_number": "100", "city": "Fortaleza", "state": "CE", "zip": "60000000",
		"website": "https://acme.com.br", "phone": "(85) 3333-4444", "email": "contato@acme.com.br",
	}
	for campo, quer := range campos {
		if !Iguais(p.Campos[campo], quer) {
			t.Errorf("%s = %v, quer %v", campo, p.Campos[campo], quer)
		}
	}
	if len(p.CNAEsSecundarios) != 1 || p.CNAEsSecundarios[0].Codigo != "6201-5/01" {
		t.Errorf("cnaes = %+v", p.CNAEsSecundarios)
	}
	if p.Localizacao == nil || p.Localizacao.Lat != -3.73 || p.Localizacao.Lng != -38.52 {
		t.Errorf("localizacao = %+v", p.Localizacao)
	}
	cel := achar(p.Contatos.Telefones, "+5585999990000")
	if cel == nil || cel.WhatsApp == nil || !*cel.WhatsApp || cel.NaoPerturbe != nil {
		t.Errorf("celular = %+v (o formato B2B não informa bloqueio)", cel)
	}
	if e := p.Contatos.Emails; len(e) != 2 || !e[0].Validado || e[1].Validado {
		t.Errorf("emails = %+v", e)
	}
	if len(p.Socios) != 1 || p.Socios[0].Qualificacao != "Administrador" || p.Socios[0].Documento != "***456789**" {
		t.Errorf("socios = %+v", p.Socios)
	}
}

// Tolerâncias: documento numérico, ddd em texto, lista vazia e formato desconhecido.
func TestCnpjDataTolerancias(t *testing.T) {
	corpo := `{"cnpj": 1222333000181, "company_name": "X LTDA", "cnae_code": 111301,
	  "mobile_phones": [{"ddd": "55", "number": "999991111", "priority": 1, "whatsapp": false, "active_nao_me_ligue": null}],
	  "partners": [{"name": "A", "cpf": 1234567890, "cnpj": null}]}`
	p, formato, err := ParseCnpjData([]byte(corpo), "wholead", "01222333000181")
	if err != nil || formato != FormatoDataStoneConsulta {
		t.Fatalf("formato=%s err=%v", formato, err)
	}
	if p.Texto("cnae_code") != "0111-3/01" {
		t.Errorf("cnae = %q", p.Texto("cnae_code"))
	}
	tel := p.Contatos.Telefones[0]
	if tel.Numero != "+5555999991111" || tel.Validado || tel.NaoPerturbe != nil || p.Texto("phone") != "(55) 99999-1111" {
		t.Errorf("telefone = %+v phone=%q", tel, p.Texto("phone"))
	}
	// 1234567890 perdeu o zero à esquerda: o CPF é 01234567890.
	if p.Socios[0].Documento != "***345678**" {
		t.Errorf("cpf numérico = %q", p.Socios[0].Documento)
	}
	for _, c := range []string{`{"qualquer": 1}`, `[]`, `null`, `"texto"`, ``, ` `} {
		if p, formato, err := ParseCnpjData([]byte(c), "wholead", cnpjTeste); p != nil || err != nil || formato != FormatoDesconhecido {
			t.Errorf("%s: perfil=%v formato=%s err=%v", c, p, formato, err)
		}
	}
	if _, _, err := ParseCnpjData([]byte(`{`), "wholead", cnpjTeste); err == nil {
		t.Error("JSON inválido deveria falhar")
	}
}

// Só número bloqueado disponível: ainda assim preenche phone (a tela mostra o aviso).
func TestCnpjDataSoBloqueado(t *testing.T) {
	corpo := `{"company_name": "X", "mobile_phones": [
	  {"ddd": 11, "number": "988887777", "priority": 1, "whatsapp": true, "active_nao_me_ligue": true},
	  {"ddd": 11, "number": "977776666", "priority": 2, "whatsapp": true, "active_nao_me_ligue": false}]}`
	p, _, _ := ParseCnpjData([]byte(corpo), "wholead", cnpjTeste)
	if p.Texto("phone") != "(11) 97777-6666" {
		t.Errorf("phone = %q (deveria pular o bloqueado)", p.Texto("phone"))
	}
	corpo = `{"company_name": "X", "mobile_phones": [{"ddd": 11, "number": "988887777", "priority": 1, "active_nao_me_ligue": true}]}`
	p, _, _ = ParseCnpjData([]byte(corpo), "wholead", cnpjTeste)
	if p.Texto("phone") != "(11) 98888-7777" {
		t.Errorf("phone = %q", p.Texto("phone"))
	}
}

// Pessoas do formato consulta: sócio PF, responsável legal (mesma pessoa,
// fica como sócio) e só o funcionário atual; CPF sempre mascarado.
func TestCnpjDataPessoas(t *testing.T) {
	p, _, err := ParseCnpjData(lerFixture(t, "datastone_companies_formatoA.json"), "wholead", cnpjTeste)
	if err != nil || p == nil {
		t.Fatalf("perfil=%v err=%v", p, err)
	}
	papeis := map[string]string{}
	for _, pe := range p.Pessoas {
		papeis[pe.Nome] = pe.Papel
		if len(pe.Documento) != 11 || pe.Documento[:3] != "***" {
			t.Errorf("documento sem máscara: %+v", pe)
		}
	}
	if papeis["FULANO DE TAL"] != PapelSocio || papeis["BELTRANA EXEMPLO"] != PapelFuncionario {
		t.Errorf("pessoas = %+v", p.Pessoas)
	}
	if _, ok := papeis["CICLANO EXEMPLO"]; ok {
		t.Errorf("vínculo histórico não deveria entrar: %+v", p.Pessoas)
	}
	if _, ok := papeis["EXEMPLO PARTICIPACOES LTDA"]; ok {
		t.Errorf("sócio PJ não é pessoa: %+v", p.Pessoas)
	}
	for _, pe := range p.Pessoas {
		if pe.Nome == "BELTRANA EXEMPLO" && pe.Cargo != "Assistente de Vendas" {
			t.Errorf("cargo = %q", pe.Cargo)
		}
	}
	b, _ := json.Marshal(p)
	for _, cpf := range []string{"12345678900", "98765432100", "11111111111"} {
		if strings.Contains(string(b), cpf) {
			t.Errorf("perfil contém CPF completo %s", cpf)
		}
	}
}

// Formato reconhecido dentro de um envelope desconhecido: procura em qualquer nível.
func TestCnpjDataEnvelopeDesconhecido(t *testing.T) {
	corpo := `{"resultado": {"empresa": {"company_name": "X LTDA", "mobile_phones": [{"ddd": 11, "number": "988887777", "priority": 1}]}}}`
	p, formato, err := ParseCnpjData([]byte(corpo), "wholead", cnpjTeste)
	if err != nil || p == nil || formato != FormatoDataStoneConsulta || p.Texto("legal_name") != "X LTDA" {
		t.Fatalf("perfil=%v formato=%s err=%v", p, formato, err)
	}
	if p.Texto("cnpj") != "11.222.333/0001-81" {
		t.Errorf("cnpj = %q", p.Texto("cnpj"))
	}
}

func TestMascararCPF(t *testing.T) {
	casos := map[string]string{
		"12345678900": "***456789**", "123.456.789-00": "***456789**", "1234567890": "***345678**",
		"***456789**": "***456789**", "": "",
	}
	for in, quer := range casos {
		if got := MascararCPF(in); got != quer {
			t.Errorf("MascararCPF(%q) = %q, quer %q", in, got, quer)
		}
	}
}
