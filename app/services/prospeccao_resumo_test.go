package services_test

import (
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

func TestMontarResumoLead(t *testing.T) {
	agora := time.Date(2026, 11, 24, 10, 0, 0, 0, time.Local)
	abertura := "2020-05-01"
	capital := 1500000.0
	wa := true
	dados := &models.ProspEmpresa{
		RazaoSocial: "PADARIA PAO BOM LTDA", NomeFantasia: "Pão Bom", Porte: models.PorteEPP,
		Abertura: &abertura, Capital: &capital, Simples: true,
		CNAE:     models.ProspCodigoDescricao{Codigo: 1091102, Descricao: "PADARIA E CONFEITARIA"},
		Endereco: models.ProspEndereco{TipoLogradouro: "RUA", Logradouro: "DAS FLORES", Numero: "10", Bairro: "CENTRO", CEP: "60000000", Municipio: "FORTALEZA", UF: "CE"},
		Socios: []models.ProspSocio{
			{Nome: "JOAO DA SILVA", Qualificacao: "Sócio-Administrador", Tipo: "pf"},
			{Nome: "PEDRO MENOR", Qualificacao: "Sócio", Tipo: "pf", Representante: "JOAO DA SILVA"},
			{Nome: "MARIA SOUZA", Qualificacao: "Sócio-Administrador", Tipo: "pf"},
		},
		Divida: &models.ProspDivida{Total: 12000, Ajuizada: true, Referencia: "2026-T3"},
	}
	perfil := &models.PerfilContato{Site: "paobom.com.br", FaturamentoFaixa: "até R$ 360 mil",
		Pessoas: []models.PessoaContato{{Nome: "Maria Souza", Cargo: "Diretora"}}}
	telefones := []models.ProspTelefone{
		{Numero: "85999990001", Tipo: models.TelefoneCelular, WhatsApp: &wa, Pessoa: "MARIA SOUZA", Principal: true},
		{Numero: "8533334444", Tipo: models.TelefoneFixo},
	}
	r := services.MontarResumoLead(dados, perfil, telefones, agora)
	if len(r.Frases) == 0 || r.Frases[0] != "Padaria e confeitaria em Fortaleza/CE, aberta há 6 anos, pequeno porte" {
		t.Fatalf("frases: %q", r.Frases)
	}
	if !strings.Contains(strings.Join(r.Frases, "|"), "Capital social de R$ 1.500.000") {
		t.Fatalf("capital: %q", r.Frases)
	}
	// Maria (com telefone, do enriquecimento) primeiro; o menor nunca aparece.
	if len(r.Decisores) != 2 || r.Decisores[0].Nome != "Maria Souza" || r.Decisores[0].Cargo != "Diretora" ||
		len(r.Decisores[0].Telefones) != 1 || r.Decisores[1].Nome != "Joao da Silva" {
		t.Fatalf("decisores: %+v", r.Decisores)
	}
	tipos := map[string]string{}
	for _, s := range r.Sinais {
		tipos[s.Tipo] = s.Nivel
	}
	if tipos["whatsapp"] != "ok" || tipos["decisor"] != "ok" || tipos["divida"] != "risco" {
		t.Fatalf("sinais: %+v", r.Sinais)
	}
	if r.Cadastro.Site != "paobom.com.br" || r.Cadastro.Endereco != "RUA DAS FLORES, 10 - CENTRO - CEP 60000-000" {
		t.Fatalf("cadastro: %+v", r.Cadastro)
	}

	vazio := services.MontarResumoLead(nil, nil, nil, agora)
	if vazio.Frases == nil || vazio.Decisores == nil || vazio.Sinais == nil || vazio.Cadastro.Socios == nil {
		t.Fatalf("listas nulas no resumo vazio: %+v", vazio)
	}
	if len(vazio.Sinais) != 1 || vazio.Sinais[0].Tipo != "sem_telefone" {
		t.Fatalf("sinal sem telefone: %+v", vazio.Sinais)
	}
}

func TestAuxiliaresDoResumo(t *testing.T) {
	if got := services.NomeProprio("JOSE DOS SANTOS E FILHOS"); got != "Jose dos Santos e Filhos" {
		t.Errorf("NomeProprio: %q", got)
	}
	if got := services.NomeProprio("Já Formatado"); got != "Já Formatado" {
		t.Errorf("NomeProprio manteve: %q", got)
	}
	if services.ChaveNome("  JOSÉ  Conceição ") != services.ChaveNome("jose conceicao") {
		t.Error("ChaveNome deveria ignorar acento, caixa e espaços")
	}
	if got := services.ReaisPorExtenso(1234.5); got != "R$ 1.234" {
		t.Errorf("ReaisPorExtenso acima de mil: %q", got)
	}
	if got := services.ReaisPorExtenso(99.5); got != "R$ 99,50" {
		t.Errorf("ReaisPorExtenso: %q", got)
	}
	if !services.SocioAdministrador("Diretor") || services.SocioAdministrador("Sócio") {
		t.Error("SocioAdministrador")
	}
	if !services.SocioMenor(models.ProspSocio{Qualificacao: "Sócio Menor (Assistido/Representado)"}) {
		t.Error("SocioMenor")
	}
	c := &models.Company{Name: "Loja X", LegalName: "LOJA X LTDA", City: "Recife", State: "PE", CompanySize: "ME"}
	r := services.ResumoDaEmpresa(c, time.Now())
	if r.Cadastro.RazaoSocial != "LOJA X LTDA" || r.Cadastro.Porte != models.PorteME || r.Cadastro.Municipio != "Recife" {
		t.Errorf("ResumoDaEmpresa: %+v", r.Cadastro)
	}
}

func TestMontarLeadCompletoMarcaBloqueiosERoteiro(t *testing.T) {
	db := prepararEnvio(t)
	if _, err := db.Exec(`TRUNCATE playbooks RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	pb := &models.Playbook{Name: "Roteiro padarias", Body: "Olá {{decisor}}, aqui é {{vendedor}}. Vi a {{empresa}} em {{cidade}}/{{uf}}.", Active: true}
	if err := models.CreatePlaybook(db, pb); err != nil {
		t.Fatal(err)
	}
	cfg := models.DefaultConfigProspeccao()
	cfg.PlaybookID = &pb.ID
	if err := models.SaveConfigProspeccao(db, &cfg, nil); err != nil {
		t.Fatal(err)
	}
	cnpj := envioCNPJ(90)
	c := &models.Company{Name: "Pão Bom", CNPJ: cnpj}
	if err := models.CreateCompany(db, c); err != nil {
		t.Fatal(err)
	}
	l := &models.Lead{Titulo: "Pão Bom", CompanyID: &c.ID, CNPJ: cnpj, Cidade: "FORTALEZA", UF: "CE",
		Telefones: []models.ProspTelefone{{Numero: "8533334444", Tipo: models.TelefoneFixo, Principal: true},
			{Numero: "85999990001", Tipo: models.TelefoneCelular}},
		Resumo: models.ResumoLead{Decisores: []models.ResumoDecisor{{Nome: "Maria Souza"}}}}
	if _, err := models.CreateLead(db, l); err != nil {
		t.Fatal(err)
	}
	for _, b := range []models.Bloqueio{{Tipo: models.BloqueioTelefone, Valor: "8533334444"}, {Tipo: models.BloqueioCNPJ, Valor: cnpj}} {
		b := b
		if _, err := models.CreateBloqueio(db, &b); err != nil {
			t.Fatal(err)
		}
	}
	lido, err := models.LeadByID(db, l.ID, models.VisaoTotal(1))
	if err != nil {
		t.Fatal(err)
	}
	lc, err := services.MontarLeadCompleto(db, lido, "Carlos Vendedor", time.Date(2026, 11, 24, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if !lc.Lead.Telefones[0].Invalido || lc.Lead.Telefones[1].Invalido {
		t.Fatalf("telefone bloqueado deveria vir inválido: %+v", lc.Lead.Telefones)
	}
	tipos := map[string]bool{}
	for _, a := range lc.Alertas {
		tipos[a.Tipo] = true
	}
	if !tipos["bloqueado"] || !tipos["whatsapp_sem_optin"] || tipos["fora_da_janela"] {
		t.Fatalf("alertas: %+v", lc.Alertas)
	}
	if lc.Roteiro == nil || lc.Roteiro.Corpo != "Olá Maria, aqui é Carlos. Vi a Pão Bom em Fortaleza/CE." {
		t.Fatalf("roteiro: %+v", lc.Roteiro)
	}
	if !lc.Janela.Aberta || lc.Tentativas == nil || lc.Contatos == nil {
		t.Fatalf("ficha: %+v", lc)
	}
}
