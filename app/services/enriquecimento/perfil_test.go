package enriquecimento

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

func perfilReceitaTeste() *Perfil {
	p, _ := ParseBrasilAPI([]byte(fixtureBrasilAPI))
	p.ConsultadoEm = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	return p
}

func TestDiffSugerido(t *testing.T) {
	p := perfilReceitaTeste()
	c := &models.Company{Name: "Acme (digitado)", City: "Fortaleza", Phone: "(85) 9999-0000"}

	diff := Diff(c, p, "vazios")
	por := map[string]CampoDiff{}
	for _, d := range diff {
		por[d.Campo] = d
		if d.Label == "" || d.Label == d.Campo {
			t.Errorf("campo %s sem rótulo humano", d.Campo)
		}
		if d.Provedor != models.ProvedorReceita {
			t.Errorf("campo %s com provedor %q", d.Campo, d.Provedor)
		}
	}
	if _, ok := por["city"]; ok {
		t.Error("city igual não deveria aparecer no diff")
	}
	if d := por["legal_name"]; !d.Sugerido || d.Atual != "" || d.Proposto != "ACME VENDAS LTDA" {
		t.Errorf("legal_name = %+v", d)
	}
	if d := por["name"]; d.Sugerido || d.Atual != "Acme (digitado)" {
		t.Errorf("name preenchido não pode ser sugerido em 'vazios': %+v", d)
	}
	if d := por["phone"]; d.Sugerido {
		t.Errorf("phone preenchido não pode ser sugerido em 'vazios': %+v", d)
	}
	if d := por["simples"]; !d.Sugerido || d.Proposto != true {
		t.Errorf("simples = %+v", d)
	}

	for _, d := range Diff(c, p, "sempre") {
		if !d.Sugerido {
			t.Errorf("em 'sempre' tudo que difere é sugerido: %+v", d)
		}
	}
	if len(Diff(nil, nil, "vazios")) != 0 {
		t.Error("diff de nil deve ser vazio")
	}
}

func TestMesclarPrecedencia(t *testing.T) {
	receita := perfilReceitaTeste()
	receita.Indicadores = &Indicadores{FuncionariosFaixa: "antiga"}
	demo := &Demo{Fonte: models.ProvedorBigDataCorp}
	bdc, _, _ := demo.Consultar(context.Background(), cnpjTeste, Opcoes{})
	bdc.Set("legal_name", "OUTRA RAZAO") // não deve vencer a Receita
	bdc.Set("revenue_range", "R$ 1M a 5M")
	bdc.CustoCentavos = 27

	m := Mesclar(bdc, receita) // ordem invertida de propósito: receita continua a base
	if m.Provedor != "receita,bigdatacorp" {
		t.Errorf("provedor = %q", m.Provedor)
	}
	if m.Texto("legal_name") != "ACME VENDAS LTDA" {
		t.Errorf("receita deveria ser base: %q", m.Texto("legal_name"))
	}
	if m.Texto("revenue_range") != "R$ 1M a 5M" || m.CamposOrigem["revenue_range"] != models.ProvedorBigDataCorp {
		t.Errorf("campo vazio deveria vir do pago: %q (%s)", m.Texto("revenue_range"), m.CamposOrigem["revenue_range"])
	}
	if m.CamposOrigem["legal_name"] != models.ProvedorReceita {
		t.Errorf("origem de legal_name = %s", m.CamposOrigem["legal_name"])
	}
	if m.Indicadores == nil || m.Indicadores.FuncionariosFaixa == "antiga" || m.Origem["indicadores"] != models.ProvedorBigDataCorp {
		t.Errorf("indicadores deveriam ser sobrepostos pelo pago: %+v %v", m.Indicadores, m.Origem)
	}
	if !m.Demo || m.Mensagem != "dados de demonstração" || m.CustoCentavos != 27 {
		t.Errorf("demo/custo = %v %q %d", m.Demo, m.Mensagem, m.CustoCentavos)
	}
	if len(m.Socios) != 1 || m.Socios[0].Nome != "FULANO DE TAL" {
		t.Errorf("sócios da receita deveriam ficar: %+v", m.Socios)
	}
	if m.Processos == nil || m.Conformidade == nil {
		t.Error("seções do pago deveriam entrar")
	}
	if len(m.Sinais) == 0 {
		t.Error("sinais recalculados")
	}
	if Mesclar(nil, nil) != nil {
		t.Error("mesclar sem perfis é nil")
	}
	// Diff sobre o mesclado atribui o provedor certo a cada campo.
	for _, d := range Diff(&models.Company{}, m, "vazios") {
		if d.Campo == "revenue_range" && d.Provedor != models.ProvedorBigDataCorp {
			t.Errorf("provedor do diff para revenue_range = %s", d.Provedor)
		}
	}
}

func TestSinais(t *testing.T) {
	p := NovoPerfil("receita")
	p.Set("registration_status", "BAIXADA")
	p.Set("founded_at", time.Now().AddDate(0, -6, 0).Format("2006-01-02"))
	p.Campos["mei"] = true
	p.Risco = &Risco{Score: 420, PendenciasQtd: 2, ProtestosQtd: 1}
	p.Processos = &Processos{Total: 6, ComoReu: 4}
	p.Conformidade = &Conformidade{Sancionado: true}
	p.Indicadores = &Indicadores{NivelAtividade: 0.1}
	sinais := CalcularSinais(p)
	nivel := map[string]string{}
	for _, s := range sinais {
		nivel[s.Tipo] = s.Nivel
	}
	esperado := map[string]string{
		"situacao": "risco", "idade": "atencao", "porte": "atencao", "risco": "risco",
		"negativacao": "risco", "processos": "risco", "conformidade": "risco", "digital": "atencao",
	}
	for tipo, n := range esperado {
		if nivel[tipo] != n {
			t.Errorf("sinal %s = %q, esperado %q (todos: %+v)", tipo, nivel[tipo], n, sinais)
		}
	}
	if len(CalcularSinais(nil)) != 0 {
		t.Error("nil → sem sinais")
	}
}

func TestDemoDeterministico(t *testing.T) {
	for _, fonte := range []string{models.ProvedorBigDataCorp, models.ProvedorSerasa} {
		d := &Demo{Fonte: fonte}
		a, ba, err := d.Consultar(context.Background(), cnpjTeste, Opcoes{})
		if err != nil {
			t.Fatal(err)
		}
		b, _, _ := d.Consultar(context.Background(), cnpjTeste, Opcoes{})
		a.ConsultadoEm, b.ConsultadoEm = time.Time{}, time.Time{}
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			t.Errorf("%s: demo não é determinístico", fonte)
		}
		if !a.Demo || a.Mensagem != "dados de demonstração" || a.CustoCentavos != 0 || ba.Fonte != fonte+"-demo" {
			t.Errorf("%s: rótulo de demonstração ausente: %+v", fonte, a)
		}
		outro, _, _ := d.Consultar(context.Background(), "00000000000191", Opcoes{})
		if fonte == models.ProvedorSerasa {
			if a.Risco == nil || a.Risco.Score < 300 || a.Risco.Score > 950 {
				t.Errorf("score demo fora da faixa: %+v", a.Risco)
			}
			if outro.Risco.Score == a.Risco.Score && outro.Risco.Consultas90d == a.Risco.Consultas90d {
				t.Error("CNPJs diferentes deveriam dar perfis diferentes")
			}
		} else {
			if a.Indicadores == nil || a.Processos == nil || a.Contatos == nil || a.Texto("employees_range") == "" {
				t.Errorf("demo bigdatacorp incompleto: %+v", a)
			}
			if a.Processos.Total < 0 || a.Processos.Total > 2 || len(a.Processos.Ultimos) != a.Processos.Total {
				t.Errorf("processos demo = %+v", a.Processos)
			}
		}
	}
	if _, _, err := (&Demo{Fonte: "xpto"}).Consultar(context.Background(), cnpjTeste, Opcoes{}); err == nil {
		t.Error("fonte desconhecida deveria falhar")
	}
}

func TestPerfilJSONContrato(t *testing.T) {
	p := perfilReceitaTeste()
	p.Indicadores = &Indicadores{}
	p.Contatos = &Contatos{Telefones: []Telefone{}, Emails: []EmailContato{}}
	p.PresencaDigital = &PresencaDigital{Dominios: []Dominio{}, Redes: map[string]string{}}
	p.Risco = &Risco{}
	p.Processos = &Processos{Ultimos: []Processo{}}
	p.Conformidade = &Conformidade{Listas: []string{}}
	p.Midia = &Midia{Noticias: []Noticia{}}
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	chaves := []string{"provedor", "demo", "consultado_em", "campos", "socios", "cnaes_secundarios", "indicadores",
		"contatos", "presenca_digital", "risco", "processos", "conformidade", "midia", "sinais", "custo_centavos"}
	for _, k := range chaves {
		if _, ok := m[k]; !ok {
			t.Errorf("JSON sem a chave %q", k)
		}
	}
	risco := m["risco"].(map[string]any)
	for _, k := range []string{"score", "faixa", "nivel", "pendencias_qtd", "pendencias_valor", "protestos_qtd", "cheques_sem_fundo", "acoes_qtd", "consultas_90d", "limite_sugerido", "observacao"} {
		if _, ok := risco[k]; !ok {
			t.Errorf("risco sem %q", k)
		}
	}
	ind := m["indicadores"].(map[string]any)
	for _, k := range []string{"nivel_atividade", "tem_dominio_ativo", "tem_ssl", "tem_email_corporativo", "funcionarios_faixa", "faturamento_faixa"} {
		if _, ok := ind[k]; !ok {
			t.Errorf("indicadores sem %q", k)
		}
	}
	// Volta do JSON sem perder nada.
	volta := PerfilDeJSON(b)
	if volta == nil || !reflect.DeepEqual(volta.Campos["legal_name"], p.Campos["legal_name"]) || volta.ConsultadoEm != p.ConsultadoEm {
		t.Errorf("round-trip falhou: %+v", volta)
	}
}

func TestAtribuirCampo(t *testing.T) {
	c := &models.Company{}
	for campo, v := range map[string]any{
		"legal_name": "X LTDA", "founded_at": "2015-03-10", "capital": 100.0, "simples": true, "zip": "60000-000", "email": "A@B.COM.BR",
	} {
		if !AtribuirCampo(c, campo, v) {
			t.Errorf("AtribuirCampo(%s) falhou", campo)
		}
	}
	if c.LegalName != "X LTDA" || c.FoundedAt == nil || c.FoundedAt.String() != "2015-03-10" || c.Capital == nil || *c.Capital != 100 ||
		c.Simples == nil || !*c.Simples || c.Zip != "60000000" || c.Email != "a@b.com.br" {
		t.Errorf("empresa = %+v", c)
	}
	if AtribuirCampo(c, "xpto", "1") || AtribuirCampo(c, "founded_at", "ontem") || AtribuirCampo(c, "simples", "sim") {
		t.Error("campo desconhecido/valor incompatível deveria devolver false")
	}
}
