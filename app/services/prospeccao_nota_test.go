package services_test

import (
	"strings"
	"testing"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

func verdade() *bool { b := true; return &b }

func pontosDe(n services.NotaItem) map[string]int {
	m := map[string]int{}
	for _, mo := range n.Motivos {
		m[mo.Texto] = mo.Pontos
	}
	return m
}

func somaMotivos(n services.NotaItem) int {
	s := 0
	for _, mo := range n.Motivos {
		s += mo.Pontos
	}
	return s
}

func TestNotaSoComORetratoDaBase(t *testing.T) {
	ab := "2010-05-01"
	emp := &models.ProspEmpresa{
		Porte: models.PorteDemais, Abertura: &ab,
		Telefones: []models.ProspTelefone{
			{Numero: "85999990001", Tipo: models.TelefoneCelular},
			{Numero: "8532220001", Tipo: models.TelefoneFixo},
		},
		Email: "comercial@empresa.com.br", EmailTipo: "corporativo",
		Socios: []models.ProspSocio{{Nome: "MARIA", Qualificacao: "Sócio-Administrador", Tipo: "pf"}},
	}
	n := services.CalcularNotaItem(emp, nil)
	p := pontosDe(n)
	// 35 celular + 8 sócio-adm + 10 e-mail corporativo + 10 fixo + 5 idade + 5 porte
	if n.Nota != 73 || p["Celular próprio"] != 35 || p["Sócio-administrador identificado"] != 8 || p["Telefone fixo próprio"] != 10 {
		t.Fatalf("nota %d motivos %+v", n.Nota, n.Motivos)
	}
	if !n.TemCelular || !n.TemEmail || n.TemDecisor || n.ContatoCompartilhado || n.TelefonePrincipal != "85999990001" {
		t.Fatalf("marcas: %+v", n)
	}
	if somaMotivos(n) != n.Nota {
		t.Fatal("a nota é a soma dos motivos quando não passa dos limites")
	}
}

func TestNotaDepoisDoEnriquecimento(t *testing.T) {
	emp := &models.ProspEmpresa{Porte: models.PorteME,
		Telefones: []models.ProspTelefone{{Numero: "8532220001", Tipo: models.TelefoneFixo, Compartilhado: true}},
		Email:     "fiscal@contabilidadeexemplo.com.br", EmailTipo: "contabilidade"}

	// Antes: o único contato é de contador.
	antes := services.CalcularNotaItem(emp, nil)
	if antes.Nota != 0 || !antes.ContatoCompartilhado || pontosDe(antes)["O único contato é compartilhado ou de contabilidade"] != -25 {
		t.Fatalf("antes: %d %+v", antes.Nota, antes.Motivos)
	}

	perfil := &models.PerfilContato{
		Site: "https://empresa.com.br",
		Telefones: []models.ProspTelefone{
			{Numero: "85988880002", Tipo: models.TelefoneCelular, NaoPerturbe: verdade()},
		},
		Emails: []models.EmailContato{{Email: "dono@gmail.com"}},
		Pessoas: []models.PessoaContato{{Nome: "JOÃO", Papel: "socio",
			Telefones: []models.ProspTelefone{{Numero: "85977770003", Tipo: models.TelefoneCelular, WhatsApp: verdade()}}}},
	}
	n := services.CalcularNotaItem(emp, perfil)
	p := pontosDe(n)
	// 35 celular + 10 WhatsApp + 15 decisor + 4 gratuito + 5 site − 10 não perturbe
	if n.Nota != 59 || p["Decisor com telefone"] != 15 || p["WhatsApp confirmado"] != 10 || p["E-mail de provedor gratuito"] != 4 {
		t.Fatalf("depois: %d %+v", n.Nota, n.Motivos)
	}
	if n.ContatoCompartilhado || !n.TemDecisor || !n.TemWhatsApp {
		t.Fatalf("marcas: %+v", n)
	}
	// O melhor telefone primeiro: o do decisor com WhatsApp; "não perturbe" e o compartilhado no fim.
	if n.TelefonePrincipal != "85977770003" || n.Telefones[0].Pessoa != "JOÃO" || !n.Telefones[0].Principal {
		t.Fatalf("telefones: %+v", n.Telefones)
	}
	if ultimo := n.Telefones[len(n.Telefones)-1]; ultimo.NaoPerturbe == nil || !*ultimo.NaoPerturbe {
		t.Fatalf("o telefone com não perturbe vai para o fim: %+v", n.Telefones)
	}
}

func TestNotaSemTelefoneEIndefinido(t *testing.T) {
	n := services.CalcularNotaItem(&models.ProspEmpresa{}, nil)
	if n.Nota != 0 || pontosDe(n)["Sem telefone"] != -40 || n.TelefonePrincipal != "" {
		t.Fatalf("sem contato: %d %+v", n.Nota, n.Motivos)
	}
	// "indefinido" conta como fixo e não como celular.
	n = services.CalcularNotaItem(&models.ProspEmpresa{Telefones: []models.ProspTelefone{{Numero: "1145550000", Tipo: models.TelefoneIndefinido}}}, nil)
	if n.TemCelular || pontosDe(n)["Telefone fixo próprio"] != 10 || n.Nota != 10 {
		t.Fatalf("indefinido: %d %+v", n.Nota, n.Motivos)
	}
	if n.Nota < 0 || n.Nota > 100 {
		t.Fatal("nota fora de 0 a 100")
	}
}

func TestLinhaDaExportacao(t *testing.T) {
	ab := "2010-05-01"
	capital := 150000.5
	it := &models.ProspItem{
		CNPJ: "12345678000195", RazaoSocial: "=HYPERLINK(\"x\")", NomeFantasia: "Loja", Estado: models.ItemEnriquecido, Nota: 50,
		NotaMotivos: []models.NotaMotivo{{Pontos: 35, Texto: "Celular próprio"}}, UF: "CE", Municipio: "FORTALEZA",
		TemCelular: true, TelefonePrincipal: "85999990001",
		Dados: &models.ProspEmpresa{Abertura: &ab, Capital: &capital, Email: "a@b.com.br",
			Telefones: []models.ProspTelefone{{Numero: "85999990001", Tipo: models.TelefoneCelular}},
			Socios:    []models.ProspSocio{{Nome: "MARIA", Qualificacao: "Sócio-Administrador", Tipo: "pf", Documento: "***123456**"}}},
		Enriquecimento: &models.PerfilContato{Emails: []models.EmailContato{{Email: "A@B.com.br"}, {Email: "c@d.com"}},
			Pessoas: []models.PessoaContato{{Nome: "JOÃO", Documento: "***999999**", Papel: "socio", Cargo: "Diretor"}}},
	}
	linha := services.LinhaExportacaoItem(it)
	if len(linha) != len(services.ColunasExportacaoLista) {
		t.Fatalf("colunas: %d de %d", len(linha), len(services.ColunasExportacaoLista))
	}
	col := map[string]string{}
	for i, c := range services.ColunasExportacaoLista {
		col[c] = linha[i]
	}
	if col["cnpj"] != "12.345.678/0001-95" || col["razao_social"] != `'=HYPERLINK("x")` || col["capital"] != "150000.50" ||
		col["emails"] != "a@b.com.br | c@d.com" || col["decisores"] != "JOÃO (Diretor)" || col["motivos_nota"] != "Celular próprio (+35)" {
		t.Fatalf("linha: %+v", col)
	}
	if strings.Contains(strings.Join(linha, ";"), "***") {
		t.Fatal("nenhum documento de pessoa sai na exportação")
	}
}
