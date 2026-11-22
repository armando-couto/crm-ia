package enriquecimento

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// servidorWhoLead simula a API do integrador.
type servidorWhoLead struct {
	t          *testing.T
	tokens     atomic.Int32
	recusar401 atomic.Int32 // quantas chamadas autenticadas devolvem 401
	statusCNPJ int
	corpoCNPJ  string
}

func (s *servidorWhoLead) handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/oauth/token" {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "segredo" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		n := s.tokens.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"tok` + string(rune('0'+n)) + `","token_type":"Bearer","expires_in":3600}`))
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if s.recusar401.Load() > 0 {
		s.recusar401.Add(-1)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/account":
		_, _ = w.Write([]byte(`{"plan_name":"integrator_basic","quota_total":1000,"quota_used":200,"quota_remaining":800,"cycle_start":"2026-11-01","cycle_end":"2026-11-30"}`))
	case r.URL.Path == "/enrichment/cnpj/"+cnpjTeste:
		if s.statusCNPJ != 0 && s.statusCNPJ != 200 {
			w.WriteHeader(s.statusCNPJ)
			_, _ = w.Write([]byte(`{"detail":{"code":"x"}}`))
			return
		}
		_, _ = w.Write([]byte(s.corpoCNPJ))
	case r.URL.Path == "/enrichment" && r.Method == http.MethodPost:
		var corpo struct {
			CNPJs  []string `json:"cnpjs"`
			Socios bool     `json:"include_socios"`
		}
		_ = json.NewDecoder(r.Body).Decode(&corpo)
		if len(corpo.CNPJs) != 2 || !corpo.Socios {
			s.t.Errorf("lote recebido = %+v", corpo)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"consultation_id":77,"status":"pending","total_cost":"1.60","units_debited":2,"item_count":2}`))
	case r.URL.Path == "/enrichment/77":
		_, _ = w.Write([]byte(`{"id":77,"status":"ready","ready":true,"items_total":2,"items_processed":2}`))
	case r.URL.Path == "/enrichment/77/results":
		if r.URL.Query().Get("limit") != "500" {
			s.t.Errorf("limit = %q", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write(lerFixture(s.t, "wholead_enrichment_results_hipotese.json"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func novoWhoLeadTeste(t *testing.T) (*WhoLead, *servidorWhoLead) {
	t.Helper()
	srv := &servidorWhoLead{t: t, corpoCNPJ: string(lerFixture(t, "wholead_cnpj_lookup_hipotese.json"))}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	t.Cleanup(ts.Close)
	utils.Cache = utils.NovoMemoria()
	t.Cleanup(func() { utils.Cache = utils.NovoMemoria() })
	return &WhoLead{ClientID: "cid", ClientSecret: "segredo", BaseURL: ts.URL, IncluirSocios: true, PrecoCentavos: 80}, srv
}

func TestWhoLeadTokenEmCacheERenovacaoEm401(t *testing.T) {
	w, srv := novoWhoLeadTeste(t)
	ctx := context.Background()
	conta, err := w.Conta(ctx)
	if err != nil || conta.Plano != "integrator_basic" || conta.FranquiaRestante == nil || *conta.FranquiaRestante != 800 {
		t.Fatalf("conta = %+v, err = %v", conta, err)
	}
	if _, err := w.Conta(ctx); err != nil || srv.tokens.Load() != 1 {
		t.Fatalf("token deveria vir do cache: emitidos=%d err=%v", srv.tokens.Load(), err)
	}
	// Token recusado: emite outro uma vez e segue.
	srv.recusar401.Store(1)
	if _, err := w.Conta(ctx); err != nil || srv.tokens.Load() != 2 {
		t.Fatalf("401 deveria renovar o token uma vez: emitidos=%d err=%v", srv.tokens.Load(), err)
	}
	// Recusado duas vezes seguidas: falha de credencial.
	srv.recusar401.Store(2)
	if _, err := w.Conta(ctx); TipoDaFalha(err) != FalhaCredencial {
		t.Fatalf("err = %v (%s)", err, TipoDaFalha(err))
	}
	// Credencial errada na emissão.
	w2 := *w
	w2.ClientSecret = "errado"
	if _, err := w2.Token(ctx); TipoDaFalha(err) != FalhaCredencial {
		t.Fatalf("segredo errado: %v", err)
	}
}

func TestWhoLeadConsultarSincrono(t *testing.T) {
	w, srv := novoWhoLeadTeste(t)
	p, b, err := w.Consultar(context.Background(), "11.222.333/0001-81", Opcoes{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Provedor != models.ProvedorWhoLead || p.Texto("legal_name") != "ACME VENDAS LTDA" || p.CustoCentavos != 80 {
		t.Errorf("perfil = %+v", p)
	}
	if len(p.Pessoas) == 0 {
		t.Error("perfil sem pessoas")
	}
	if strings.Contains(string(b.Corpo), "12345678900") || !strings.Contains(string(b.Corpo), "***456789**") {
		t.Errorf("bruto com CPF completo: %s", b.Corpo)
	}

	for status, tipo := range map[int]string{402: FalhaSaldo, 409: FalhaOcupado, 429: FalhaLimite, 404: FalhaSemResultado, 503: FalhaExterna} {
		srv.statusCNPJ = status
		if _, _, err := w.Consultar(context.Background(), cnpjTeste, Opcoes{}); TipoDaFalha(err) != tipo {
			t.Errorf("HTTP %d: tipo = %s (%v)", status, TipoDaFalha(err), err)
		}
	}
	srv.statusCNPJ = 200
	srv.corpoCNPJ = `{"cnpj":"11222333000181","found":false,"data":null,"cost":"0.00"}`
	if _, _, err := w.Consultar(context.Background(), cnpjTeste, Opcoes{}); TipoDaFalha(err) != FalhaSemResultado {
		t.Errorf("found=false: %v", err)
	}
	// Formato desconhecido não é erro: perfil vazio rotulado.
	srv.corpoCNPJ = `{"cnpj":"11222333000181","found":true,"data":{"qualquer":1},"cost":"0.80"}`
	p, _, err = w.Consultar(context.Background(), cnpjTeste, Opcoes{})
	if err != nil || p.Mensagem == "" || p.CustoCentavos != 80 {
		t.Errorf("desconhecido: perfil=%+v err=%v", p, err)
	}
	if _, _, err := w.Consultar(context.Background(), "123", Opcoes{}); TipoDaFalha(err) != FalhaEntrada {
		t.Errorf("CNPJ inválido: %v", err)
	}
}

func TestWhoLeadLote(t *testing.T) {
	w, _ := novoWhoLeadTeste(t)
	ctx := context.Background()
	lote, err := w.CriarLote(ctx, []string{cnpjTeste, "99888777000166"})
	if err != nil || lote.ConsultationID != 77 || lote.CustoCentavos != 160 || !lote.CustoInformado || lote.Itens != 2 {
		t.Fatalf("lote = %+v err = %v", lote, err)
	}
	and, err := w.Andamento(ctx, 77)
	if err != nil || !and.Pronto || and.Processados != 2 {
		t.Fatalf("andamento = %+v err = %v", and, err)
	}
	pag, err := w.Resultados(ctx, 77, 0, 9999)
	if err != nil || len(pag.Itens) != 2 {
		t.Fatalf("resultados = %+v err = %v", pag, err)
	}
	r := pag.Itens[0]
	if r.CNPJ != cnpjTeste || !r.Encontrado || r.Resultado != ResultadoEncontrado || len(r.Pessoas) != 1 || len(r.Pessoas[0].Telefones) != 2 {
		t.Errorf("item = %+v", r)
	}
	if strings.Contains(string(r.Bruto), "12345678900") || strings.Contains(string(r.Bruto), "98765432100") {
		t.Errorf("bruto com CPF completo: %s", r.Bruto)
	}
	if pag.Itens[1].Resultado != ResultadoNaoEncontrado || !pag.Itens[1].Estornado {
		t.Errorf("não encontrado = %+v", pag.Itens[1])
	}

	pc, perfil, formato := PerfilContatoDeResultado(r, time.Now())
	if perfil == nil || formato != FormatoDataStoneConsulta {
		t.Fatalf("formato = %s", formato)
	}
	if len(pc.Telefones) != 3 || pc.Telefones[0].Numero != "85999990000" || pc.Telefones[0].Tipo != models.TelefoneCelular ||
		pc.Telefones[0].WhatsApp == nil || !*pc.Telefones[0].WhatsApp || pc.Telefones[0].Origem != models.OrigemContatoWhoLead {
		t.Errorf("telefones = %+v", pc.Telefones)
	}
	if len(pc.Emails) != 2 || pc.Emails[0].Tipo != "corporativo" || pc.Emails[1].Tipo != "gratuito" {
		t.Errorf("emails = %+v", pc.Emails)
	}
	if pc.FuncionariosFaixa == "" || pc.FaturamentoFaixa == "" || pc.Site != "https://acme.com.br" {
		t.Errorf("faixas/site = %q %q %q", pc.FuncionariosFaixa, pc.FaturamentoFaixa, pc.Site)
	}
	var fulano *models.PessoaContato
	for i := range pc.Pessoas {
		if pc.Pessoas[i].Nome == "FULANO DE TAL" {
			fulano = &pc.Pessoas[i]
		}
	}
	if fulano == nil || fulano.Papel != PapelSocio || fulano.Documento != "***456789**" || len(fulano.Telefones) != 2 ||
		fulano.Telefones[0].Numero != "85999991111" || fulano.Telefones[1].Tipo != models.TelefoneFixo {
		t.Errorf("sócio = %+v", fulano)
	}
	if !PerfilContatoTemContato(pc) {
		t.Error("deveria ter contato")
	}
	// Formato desconhecido: só as pessoas e a observação.
	r.CnpjData = json.RawMessage(`{"x":1}`)
	pc, perfil, formato = PerfilContatoDeResultado(r, time.Now())
	if perfil != nil || formato != FormatoDesconhecido || len(pc.Pessoas) != 1 || pc.Observacao == "" || len(pc.Telefones) != 0 {
		t.Errorf("desconhecido = %+v", pc)
	}
}

func TestCentavosDeDecimal(t *testing.T) {
	casos := map[string]int{"0.80": 80, "1.6": 160, "3": 300, "12,345": 1235, "0.004": 0, "1234.50": 123450, "-0.10": -10}
	for in, quer := range casos {
		if got, ok := CentavosDeDecimal(in); !ok || got != quer {
			t.Errorf("CentavosDeDecimal(%q) = %d,%v; quer %d", in, got, ok, quer)
		}
	}
	for _, in := range []string{"", "abc", "1.2.3", "R$ 1"} {
		if _, ok := CentavosDeDecimal(in); ok {
			t.Errorf("%q deveria ser inválido", in)
		}
	}
}

func TestMascararCPFsJSON(t *testing.T) {
	in := `{"cpf":"12345678900","persons":[{"cpf":1234567890,"nome":"A","texto":"doc 123.456.789-00"}],
	  "cpfs":["98765432100"],"socios":[{"documento":"***111222**"}],"cnpj":"11222333000181","outro":"123.456.789-00"}`
	out := string(MascararCPFsJSON([]byte(in)))
	for _, proibido := range []string{"12345678900", "1234567890", "98765432100", "\"123.456.789-00\""} {
		if strings.Contains(out, proibido) {
			t.Errorf("ainda contém %s: %s", proibido, out)
		}
	}
	for _, quer := range []string{"***456789**", "***345678**", "***654321**", "***111222**", "11222333000181"} {
		if !strings.Contains(out, quer) {
			t.Errorf("deveria conter %s: %s", quer, out)
		}
	}
	if string(MascararCPFsJSON([]byte("{"))) != "null" || MascararCPFsJSON(nil) != nil {
		t.Error("entradas inválidas")
	}
}

func TestTruncarCampo(t *testing.T) {
	longo := strings.Repeat("á", 300)
	if got := TruncarCampo("legal_name", longo).(string); len([]rune(got)) != 255 {
		t.Errorf("legal_name com %d caracteres", len([]rune(got)))
	}
	if got := TruncarCampo("employees_range", "DE 50 A 99 FUNCIONARIOS E MAIS UM TEXTO MUITO LONGO AQUI").(string); len([]rune(got)) > 40 {
		t.Errorf("employees_range = %q", got)
	}
	if TruncarCampo("capital", 10.0) != 10.0 || TruncarCampo("desconhecido", " x ") != " x " {
		t.Error("não texto ou sem tamanho deveria voltar igual")
	}
	c := &models.Company{}
	AtribuirCampo(c, "revenue_range", strings.Repeat("R", 100))
	if len(c.RevenueRange) != 60 {
		t.Errorf("revenue_range com %d", len(c.RevenueRange))
	}
}

func TestRegistroWhoLead(t *testing.T) {
	cfg := models.DefaultEnrichmentSettings()
	reg := NovoRegistro(cfg)
	if len(reg.Catalogo()) != 4 {
		t.Fatalf("catálogo com %d fontes", len(reg.Catalogo()))
	}
	if !CodigoValido(models.ProvedorWhoLead) || !Pago(models.ProvedorWhoLead) {
		t.Error("wholead deveria ser fonte paga válida")
	}
	if _, err := reg.Provedor(models.ProvedorWhoLead); TipoDaFalha(err) != FalhaConfig {
		t.Errorf("desabilitado: %v", err)
	}
	cfg.WhoLead.Habilitado, cfg.WhoLead.Demo = true, true
	reg = NovoRegistro(cfg)
	prov, err := reg.Provedor(models.ProvedorWhoLead)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := prov.Consultar(context.Background(), cnpjTeste, Opcoes{})
	if err != nil || !p.Demo || p.CustoCentavos != 0 || len(p.Pessoas) == 0 || p.Contatos == nil {
		t.Fatalf("demo = %+v err = %v", p, err)
	}
	if reg.CustoEstimado(models.ProvedorWhoLead, nil) != 0 {
		t.Error("demo custa zero")
	}
	// Demo determinística pelo CNPJ.
	a, _ := PerfilContatoDemo(cnpjTeste)
	b, _ := PerfilContatoDemo(cnpjTeste)
	ja, _ := json.Marshal(a.Telefones)
	jb, _ := json.Marshal(b.Telefones)
	if string(ja) != string(jb) || !a.Demo || a.Observacao == "" || !PerfilContatoTemContato(a) {
		t.Errorf("demo não determinística: %s / %s", ja, jb)
	}
	// Sem credenciais (e sem demo) o cliente real não sai.
	cfg.WhoLead.Demo = false
	if _, err := NovoRegistro(cfg).WhoLead(); TipoDaFalha(err) != FalhaConfig {
		t.Errorf("sem credencial: %v", err)
	}
}

// O provedor receita consulta a base própria primeiro; falha dela cai nas
// APIs públicas; CNPJ com letras não vai às APIs públicas.
func TestReceitaPlataformaPrimeiro(t *testing.T) {
	srv := &servidorReceita{brasilapi: 200, opencnpj: 200, cnpjws: 200}
	ts := httptest.NewServer(http.HandlerFunc(srv.handler))
	defer ts.Close()
	apontarFontes(t, ts.URL)
	antigo := FichaDaPlataforma
	t.Cleanup(func() { FichaDaPlataforma = antigo })

	abertura := "2015-03-10"
	FichaDaPlataforma = func(ctx context.Context, cnpj string) (*models.ProspEmpresa, error) {
		return &models.ProspEmpresa{
			CNPJ: cnpj, RazaoSocial: "ACME VENDAS LTDA", NomeFantasia: "ACME", Situacao: "ATIVA", Abertura: &abertura,
			Porte: models.PorteME, CNAE: models.ProspCodigoDescricao{Codigo: 6201501, Descricao: "Desenvolvimento"},
			Endereco:  models.ProspEndereco{TipoLogradouro: "RUA", Logradouro: "DAS FLORES", Numero: "100", CEP: "60000000", Municipio: "FORTALEZA", UF: "CE"},
			Telefones: []models.ProspTelefone{{Numero: "85999990000", Tipo: models.TelefoneCelular}},
			Email:     "contato@acme.com.br",
			Socios:    []models.ProspSocio{{Nome: "FULANO", Qualificacao: "Sócio-Administrador", Tipo: "pf", Documento: "***123456**"}},
		}, nil
	}
	r := &Receita{SemCEP: true}
	p, b, err := r.Consultar(context.Background(), cnpjTeste, Opcoes{Forcar: true})
	if err != nil || b.Fonte != FontePlataforma || srv.chamadas.Load() != 0 {
		t.Fatalf("plataforma: bruto=%+v chamadas=%d err=%v", b, srv.chamadas.Load(), err)
	}
	if p.Texto("phone") != "(85) 99999-0000" || p.Texto("cnae_code") != "6201-5/01" || p.Texto("domain") != "acme.com.br" ||
		len(p.Socios) != 1 || p.Socios[0].Documento != "***123456**" {
		t.Errorf("perfil = %+v", p.Campos)
	}

	FichaDaPlataforma = func(ctx context.Context, cnpj string) (*models.ProspEmpresa, error) {
		return nil, errors.New("fora do ar")
	}
	if _, b, err := r.Consultar(context.Background(), cnpjTeste, Opcoes{Forcar: true}); err != nil || b.Fonte != "brasilapi" {
		t.Fatalf("queda para as públicas: %+v %v", b, err)
	}
	// CNPJ alfanumérico válido: sem base própria, não vai às APIs públicas.
	alfa := "12ABC34501DE" + models.CNPJDigitosVerificadores("12ABC34501DE")
	antes := srv.chamadas.Load()
	if _, _, err := r.Consultar(context.Background(), alfa, Opcoes{Forcar: true}); TipoDaFalha(err) != FalhaSemResultado || srv.chamadas.Load() != antes {
		t.Fatalf("alfanumérico: %v (chamadas %d → %d)", err, antes, srv.chamadas.Load())
	}
	if chaveCache(models.ProvedorReceita, strings.ToLower(alfa)) != "enriq:receita:"+alfa {
		t.Errorf("chave de cache = %q", chaveCache(models.ProvedorReceita, strings.ToLower(alfa)))
	}
}
