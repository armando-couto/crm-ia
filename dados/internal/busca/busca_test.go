package busca

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/dados/internal/amostra"
	"github.com/armando-couto/crm-ia/dados/internal/banco"
	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/testebanco"
	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// Testes de integração: PostgreSQL de verdade com a base de amostra,
// montada pelo mesmo passo de construção da base real.

var poolTeste *pgxpool.Pool

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if url, ok := testebanco.Criar("crmia_test_dados_busca"); ok {
		ctx := context.Background()
		var err error
		poolTeste, err = banco.Conectar(ctx, url, 8, map[string]string{"jit": "off", "work_mem": "64MB", "statement_timeout": "4000"})
		if err == nil {
			err = banco.Migrar(ctx, poolTeste)
		}
		if err == nil {
			err = amostra.Montar(ctx, poolTeste)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "não consegui preparar o banco de teste:", err)
			os.Exit(1)
		}
	}
	codigo := m.Run()
	if poolTeste != nil {
		poolTeste.Close()
	}
	os.Exit(codigo)
}

func servico(t *testing.T) *Servico {
	t.Helper()
	if poolTeste == nil {
		t.Skip("sem PostgreSQL de teste (CRMIA_TEST_PG)")
	}
	return Novo(poolTeste, carga.EsquemaAmostra, true)
}

func contarSQL(t *testing.T, onde string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := poolTeste.QueryRow(context.Background(), `SELECT count(*) FROM prospec_amostra.busca b JOIN prospec_amostra.base_ativa f USING (id) WHERE `+onde, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func f64(v float64) *float64 { return &v }

func TestContagemConfereComOSQLDireto(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	casos := []struct {
		nome string
		f    Filtros
		onde string
		args []any
	}{
		{"UF pelo cubo", Filtros{UFs: []string{"CE"}}, `b.uf = 'CE' AND NOT b.pf`, nil},
		{"UF com pessoa física", Filtros{UFs: []string{"ce"}, IncluirPF: true}, `b.uf = 'CE'`, nil},
		{"município", Filtros{Municipios: []int{1389}, ComCelular: true}, `b.municipio = 1389 AND b.tem_celular AND NOT b.pf`, nil},
		{"município de outra UF", Filtros{UFs: []string{"SP"}, Municipios: []int{1389}}, `false`, nil},
		{"CNAE principal", Filtros{Cnaes: []int{4721102}}, `b.cnae = 4721102 AND NOT b.pf`, nil},
		{"CNAE principal e município", Filtros{Cnaes: []int{4721102}, Municipios: []int{1389, 7107}}, `b.cnae = 4721102 AND b.municipio IN (1389, 7107) AND NOT b.pf`, nil},
		{"prefixo de CNAE", Filtros{CnaePrefixos: []string{"47"}, UFs: []string{"SP", "RJ"}}, `b.cnae BETWEEN 4700000 AND 4799999 AND b.uf IN ('SP','RJ') AND NOT b.pf`, nil},
		{"prefixo sem subclasse", Filtros{CnaePrefixos: []string{"99999"}}, `false`, nil},
		{"um CNAE em qualquer posição", Filtros{Cnaes: []int{5611203}, CnaeSecundario: true}, `b.cnaes && ARRAY[5611203] AND NOT b.pf`, nil},
		{"vários CNAEs em qualquer posição", Filtros{Cnaes: []int{5611203, 4721102, 1091102}, CnaeSecundario: true, UFs: []string{"CE"}}, `b.cnaes && ARRAY[5611203,4721102,1091102] AND b.uf = 'CE' AND NOT b.pf`, nil},
		{"qualquer posição com município", Filtros{Cnaes: []int{5611203}, CnaeSecundario: true, Municipios: []int{1389}}, `b.cnaes && ARRAY[5611203] AND b.municipio = 1389 AND NOT b.pf`, nil},
		{"sinais", Filtros{UFs: []string{"CE", "SP"}, Portes: []string{"epp", "demais"}, Matriz: "filial", Simples: "nao", ComEmail: true, ComTelefone: true, Divida: "sem"},
			`b.uf IN ('CE','SP') AND b.porte IN (3,5) AND NOT b.matriz AND NOT b.simples AND b.tem_email AND b.tem_fone AND NOT b.tem_divida AND NOT b.pf`, nil},
		{"MEI", Filtros{UFs: []string{"CE", "SP", "RJ", "MG"}, MEI: "sim", IncluirPF: true}, `b.uf IN ('CE','SP','RJ','MG') AND b.mei`, nil},
		{"abertura e capital", Filtros{UFs: []string{"SP"}, AberturaDe: "2010-01-01", AberturaAte: "2020-12-31", CapitalMin: f64(50000)},
			`b.uf = 'SP' AND b.data_inicio BETWEEN '2010-01-01' AND '2020-12-31' AND f.capital >= 50000 AND NOT b.pf`, nil},
		{"dívida mínima", Filtros{UFs: []string{"CE"}, DividaMin: f64(100000)}, `b.uf = 'CE' AND b.divida >= 10000000 AND NOT b.pf`, nil},
		{"ocultar contador", Filtros{UFs: []string{"CE"}, OcultarContador: true}, `b.uf = 'CE' AND NOT b.contador AND NOT b.pf`, nil},
		{"naturezas", Filtros{UFs: []string{"CE"}, Naturezas: []int{2046, 2305}}, `b.uf = 'CE' AND b.natureza IN (2046, 2305) AND NOT b.pf`, nil},
		{"nome sem acento", Filtros{Municipios: []int{1389}, Nome: "farmácia"}, `b.municipio = 1389 AND f.nome_busca LIKE '%FARMACIA%' AND NOT b.pf`, nil},
		{"bairro e CEP", Filtros{Municipios: []int{1389}, Bairros: []string{"São José", "centro"}, CEPs: []string{"600", "601", "602", "603", "60-4"}},
			`b.municipio = 1389 AND f.bairro_busca IN ('SAO JOSE','CENTRO') AND f.cep ~ '^60[0-4]' AND NOT b.pf`, nil},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			esperado := contarSQL(t, c.onde, c.args...)
			r, err := s.Contar(ctx, c.f)
			if err != nil {
				t.Fatal(err)
			}
			if r.TotalModo != "exato" || r.Total != esperado {
				t.Fatalf("total = %d (%s), SQL direto = %d", r.Total, r.TotalModo, esperado)
			}
			if r.Parciais.ComCelular == nil || *r.Parciais.ComCelular != contarSQL(t, "("+c.onde+") AND b.tem_celular", c.args...) ||
				*r.Parciais.ComEmail != contarSQL(t, "("+c.onde+") AND b.tem_email", c.args...) ||
				*r.Parciais.ComDivida != contarSQL(t, "("+c.onde+") AND b.tem_divida", c.args...) {
				t.Fatalf("parciais não conferem: %+v", r.Parciais)
			}
		})
	}
	// a base tem de exercitar os dois lados dos casos acima
	if contarSQL(t, `b.contador`) == 0 || contarSQL(t, `b.pf`) == 0 || contarSQL(t, `b.tem_divida`) == 0 || contarSQL(t, `f.comp1`) == 0 || contarSQL(t, `f.email_tipo = 'contabilidade'`) == 0 {
		t.Fatal("a amostra deveria ter contador, pessoa física, dívida e contato compartilhado")
	}
}

func TestContagemComTeto(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	teto, exata := TetoContagem, LimiteContagemExata
	defer func() { TetoContagem, LimiteContagemExata = teto, exata }()
	TetoContagem, LimiteContagemExata = 100, 200

	// filtro fora dos cubos com limite superior alto: conta com teto
	r, err := s.Contar(ctx, Filtros{UFs: []string{"CE"}, AberturaDe: "1990-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	limite := contarSQL(t, `b.uf = 'CE' AND NOT b.pf`)
	if r.TotalModo != "teto" || r.Total != 100 || r.TotalLimiteSuperior == nil || *r.TotalLimiteSuperior != limite {
		t.Fatalf("esperado teto de 100 com limite superior %d, veio %+v", limite, r)
	}
	if r.Parciais.ComCelular != nil || r.Parciais.ComEmail != nil || r.Parciais.ComDivida != nil {
		t.Fatalf("parciais deveriam ser nulas no teto: %+v", r.Parciais)
	}
	j, _ := json.Marshal(r)
	if !strings.Contains(string(j), `"parciais":{"com_celular":null,"com_email":null,"com_divida":null}`) || !strings.Contains(string(j), `"total_limite_superior"`) {
		t.Fatalf("JSON do teto = %s", j)
	}
	// o mesmo tipo de filtro abaixo do teto continua exato
	r, err = s.Contar(ctx, Filtros{UFs: []string{"CE"}, AberturaDe: "2026-07-01"})
	if err != nil || r.TotalModo != "exato" || r.Total != contarSQL(t, `b.uf = 'CE' AND b.data_inicio >= '2026-07-01' AND NOT b.pf`) || r.TotalLimiteSuperior != nil && false {
		t.Fatalf("abaixo do teto: %+v %v", r, err)
	}
	// só colunas dos cubos: exato mesmo com volume alto
	if r, _ := s.Contar(ctx, Filtros{UFs: []string{"CE"}}); r.TotalModo != "exato" || r.Total != limite {
		t.Fatalf("cubo: %+v", r)
	}
}

func TestFiltrosInvalidos(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	casos := map[string]Filtros{
		"informe ao menos um filtro": {ComCelular: true},
		"para filtrar MEI":           {UFs: []string{"CE"}, MEI: "sim"},
		"exigem municipios":          {UFs: []string{"CE"}, Nome: "padaria"},
		"ao menos 3 letras":          {Municipios: []int{1389}, Nome: "ab"},
		"UF inválida":                {UFs: []string{"Ceará"}},
		"porte inválido":             {UFs: []string{"CE"}, Portes: []string{"grande"}},
		"valor inválido em simples":  {UFs: []string{"CE"}, Simples: "talvez"},
		"abertura_de deve ser":       {UFs: []string{"CE"}, AberturaDe: "01/01/2020"},
		"CEP inválido":               {Municipios: []int{1389}, CEPs: []string{"60"}},
		"prefixo de CNAE inválido":   {CnaePrefixos: []string{"4"}},
	}
	for trecho, f := range casos {
		_, err := s.Contar(ctx, f)
		var ef *ErroFiltro
		if !errors.As(err, &ef) || !strings.Contains(ef.Mensagem, trecho) {
			t.Errorf("filtro %+v: esperado erro com %q, veio %v", f, trecho, err)
		}
	}
	if _, err := s.Candidatos(ctx, Filtros{UFs: []string{"CE"}}, "alfabetica", "", 10); err == nil {
		t.Error("ordem desconhecida deveria ser recusada")
	}
}

func TestPreviaSemContato(t *testing.T) {
	s := servico(t)
	p, err := s.Previa(context.Background(), Filtros{Municipios: []int{1389}}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Itens) != MaximoPrevia || p.OrdemAplicada != OrdemRelevancia || !strings.HasPrefix(p.Versao, "amostra-") {
		t.Fatalf("prévia: %d itens, ordem %s, versão %s", len(p.Itens), p.OrdemAplicada, p.Versao)
	}
	for i, e := range p.Itens {
		if e.CNPJ == "" || e.RazaoSocial == "" || e.Municipio != "FORTALEZA" || e.UF != "CE" || e.Cnae.Descricao == "" || e.PessoaFisica {
			t.Fatalf("item %d incompleto: %+v", i, e)
		}
		if i > 0 && e.Qualidade > p.Itens[i-1].Qualidade {
			t.Fatalf("relevância = qualidade decrescente; item %d tem %d depois de %d", i, e.Qualidade, p.Itens[i-1].Qualidade)
		}
	}
	// prévia gratuita: sem telefone, e-mail nem nome de sócio
	j, _ := json.Marshal(p)
	for _, proibido := range []string{"telefones", `"email"`, `"socios"`, "exemplo.invalid", `"numero"`} {
		if strings.Contains(string(j), proibido) {
			t.Fatalf("a prévia não pode trazer %q: %s", proibido, j[:300])
		}
	}
}

func chaveDe(t *testing.T, ordem, cnpj string) int64 {
	t.Helper()
	expr := map[string]string{OrdemRelevancia: "b.qualidade::bigint", OrdemRecentes: "(b.data_inicio - DATE '1800-01-01')::bigint",
		OrdemAntigas: "(DATE '9999-12-31' - b.data_inicio)::bigint", OrdemCapital: "coalesce(b.capital, -1)", OrdemDivida: "coalesce(b.divida, -1)"}[ordem]
	var k int64
	if err := poolTeste.QueryRow(context.Background(), `SELECT `+expr+` FROM prospec_amostra.busca b JOIN prospec_amostra.base_ativa f USING (id) WHERE f.cnpj = $1`, cnpj).Scan(&k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestCandidatosPaginamPorChaveSemRepetir(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	filtros := Filtros{UFs: []string{"CE"}, ComTelefone: true}
	total := contarSQL(t, `b.uf = 'CE' AND b.tem_fone AND NOT b.pf`)
	for _, ordem := range []string{OrdemRelevancia, OrdemRecentes, OrdemAntigas, OrdemCapital, OrdemDivida, OrdemAleatoria} {
		t.Run(ordem, func(t *testing.T) {
			var todos []string
			vistos := map[string]bool{}
			cursor, paginas := "", 0
			for {
				r, err := s.Candidatos(ctx, filtros, ordem, cursor, 70)
				if err != nil {
					t.Fatal(err)
				}
				paginas++
				if r.OrdemAplicada != ordem || !strings.HasPrefix(r.Versao, "amostra-") {
					t.Fatalf("ordem aplicada %s, versão %s", r.OrdemAplicada, r.Versao)
				}
				for _, c := range r.CNPJs {
					if vistos[c] {
						t.Fatalf("CNPJ repetido entre páginas: %s", c)
					}
					vistos[c] = true
				}
				todos = append(todos, r.CNPJs...)
				if r.Fim {
					if r.Cursor != "" {
						t.Fatal("última página não devolve cursor")
					}
					break
				}
				if len(r.CNPJs) != 70 || r.Cursor == "" {
					t.Fatalf("página incompleta sem fim: %d itens", len(r.CNPJs))
				}
				cursor = r.Cursor
			}
			if int64(len(todos)) != total || paginas < 3 {
				t.Fatalf("%d CNPJs em %d páginas; a contagem diz %d", len(todos), paginas, total)
			}
			if ordem == OrdemAleatoria {
				ordenados := append([]string{}, todos...)
				sort.Strings(ordenados)
				if strings.Join(ordenados[:50], ",") == strings.Join(todos[:50], ",") {
					t.Fatal("a ordem aleatória saiu igual à ordem por CNPJ")
				}
				return
			}
			anterior := chaveDe(t, ordem, todos[0])
			for _, i := range []int{1, 50, 69, 70, 71, 140, len(todos) - 1} {
				k := chaveDe(t, ordem, todos[i])
				if k > anterior {
					t.Fatalf("ordem %s quebrada na posição %d: %d depois de %d", ordem, i, k, anterior)
				}
				anterior = k
			}
		})
	}
}

func TestConjuntoGrandeUsaAOrdemDaBase(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	pequeno := ConjuntoPequeno
	defer func() { ConjuntoPequeno = pequeno }()
	ConjuntoPequeno = 50

	filtros := Filtros{UFs: []string{"SP", "CE"}}
	var ids []int32
	cursor := ""
	for {
		r, err := s.Candidatos(ctx, filtros, OrdemCapital, cursor, 200)
		if err != nil {
			t.Fatal(err)
		}
		if r.OrdemAplicada != OrdemBase {
			t.Fatalf("conjunto grande deveria sair em ordem da base, saiu %s", r.OrdemAplicada)
		}
		for _, c := range r.CNPJs {
			var id int32
			if err := poolTeste.QueryRow(ctx, `SELECT id FROM prospec_amostra.base_ativa WHERE cnpj = $1`, c).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if len(ids) > 0 && id <= ids[len(ids)-1] {
				t.Fatalf("ordem da base = id crescente; %d depois de %d", id, ids[len(ids)-1])
			}
			ids = append(ids, id)
		}
		if r.Fim {
			break
		}
		cursor = r.Cursor
	}
	if int64(len(ids)) != contarSQL(t, `b.uf IN ('SP','CE') AND NOT b.pf`) {
		t.Fatalf("paginação pela base devolveu %d", len(ids))
	}
	p, err := s.Previa(ctx, filtros, OrdemRecentes, 5)
	if err != nil || p.OrdemAplicada != OrdemBase || len(p.Itens) != 5 {
		t.Fatalf("prévia em conjunto grande: %+v %v", p.OrdemAplicada, err)
	}
}

func TestCandidatosPorListaDeCNPJs(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	var pj, pf string
	if err := poolTeste.QueryRow(ctx, `SELECT (SELECT cnpj FROM prospec_amostra.base_ativa WHERE NOT pf ORDER BY id LIMIT 1), (SELECT cnpj FROM prospec_amostra.base_ativa WHERE pf ORDER BY id LIMIT 1)`).Scan(&pj, &pf); err != nil {
		t.Fatal(err)
	}
	// com cnpjs os demais filtros são ignorados, exceto incluir_pf
	f := Filtros{CNPJs: []string{pj[:2] + "." + pj[2:], pf, "11222333000181", "lixo"}, UFs: []string{"ZZ"}, Nome: "x"}
	r, err := s.Candidatos(ctx, f, "", "", 100)
	if err != nil || len(r.CNPJs) != 1 || r.CNPJs[0] != pj || !r.Fim {
		t.Fatalf("sem incluir_pf: %+v %v", r, err)
	}
	f.IncluirPF = true
	r, err = s.Candidatos(ctx, f, "", "", 100)
	if err != nil || len(r.CNPJs) != 2 {
		t.Fatalf("com incluir_pf: %+v %v", r, err)
	}
	if c, err := s.Contar(ctx, f); err != nil || c.Total != 2 || c.TotalModo != "exato" {
		t.Fatalf("contagem por lista: %+v %v", c, err)
	}
}

var reOnzeDigitos = regexp.MustCompile(`[0-9]{11}`)

func TestDetalhesEFichaUnica(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	var pj, pf string
	if err := poolTeste.QueryRow(ctx, `SELECT (SELECT f.cnpj FROM prospec_amostra.base_ativa f JOIN prospec_amostra.divida d USING (cnpj) WHERE NOT f.pf AND f.fone2 IS NOT NULL AND f.email IS NOT NULL AND f.socios_qtd > 0 ORDER BY f.id LIMIT 1),
       (SELECT cnpj FROM prospec_amostra.base_ativa WHERE pf AND fone1 IS NOT NULL ORDER BY id LIMIT 1)`).Scan(&pj, &pf); err != nil {
		t.Fatal(err)
	}
	d, err := s.Detalhar(ctx, []string{pj, pf, pj, "11222333000181", "não é cnpj"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Itens) != 2 || strings.Join(d.NaoEncontrados, "|") != "não é cnpj|11222333000181" {
		t.Fatalf("itens=%d não encontrados=%v", len(d.Itens), d.NaoEncontrados)
	}
	e := d.Itens[0]
	if e.CNPJ != pj || e.Situacao != "ATIVA" || e.Referencia != amostra.ReferenciaCNPJ || e.Natureza.Descricao == "" || e.Cnae.Descricao == "" ||
		e.Endereco.Municipio == "" || e.Endereco.MunicipioCodigo == 0 || e.Abertura == nil || e.Capital == nil || e.Qualidade == 0 {
		t.Fatalf("ficha incompleta: %+v", e)
	}
	if len(e.Telefones) != 2 || !e.Telefones[0].Principal || e.Telefones[1].Principal || e.Telefones[0].Origem != "receita" || e.Email == "" || e.EmailTipo == "" {
		t.Fatalf("contatos: %+v %q", e.Telefones, e.Email)
	}
	if len(e.Socios) == 0 || !strings.HasPrefix(e.Socios[0].Documento, "***") || e.Socios[0].Qualificacao == "" || e.Socios[0].Tipo != "pf" {
		t.Fatalf("sócios: %+v", e.Socios)
	}
	if e.Divida == nil || e.Divida.Total <= 0 || e.Divida.Inscricoes == 0 || len(e.Divida.Tipos) == 0 || e.Divida.Referencia != amostra.ReferenciaPGFN ||
		fmt.Sprintf("%.2f", e.Divida.EmCobranca+e.Divida.Regular) != fmt.Sprintf("%.2f", e.Divida.Total) {
		t.Fatalf("dívida: %+v", e.Divida)
	}
	// pessoa física sem incluir_pf: sem telefones, sem e-mail e sem sócios
	p := d.Itens[1]
	if !p.PessoaFisica || len(p.Telefones) != 0 || p.Email != "" || p.EmailTipo != "" || len(p.Socios) != 0 {
		t.Fatalf("pessoa física deveria vir sem contato: %+v", p)
	}
	d2, err := s.Detalhar(ctx, []string{pf}, true)
	if err != nil || len(d2.Itens) != 1 || len(d2.Itens[0].Telefones) == 0 {
		t.Fatalf("pessoa física com incluir_pf: %+v %v", d2, err)
	}
	// nenhum CPF completo em nenhuma resposta
	j, _ := json.Marshal([]any{d, d2})
	for _, m := range reOnzeDigitos.FindAllString(string(j), -1) {
		if texto.CPFValido(m) && !strings.Contains(string(j), `"numero":"`+m) {
			t.Fatalf("sequência com cara de CPF na resposta: %s", m)
		}
	}
	// listas nunca saem nulas
	if strings.Contains(string(j), `"telefones":null`) || strings.Contains(string(j), `"socios":null`) || strings.Contains(string(j), `"cnaes_secundarios":null`) {
		t.Fatalf("lista nula no JSON")
	}

	u, err := s.Empresa(ctx, pj, false)
	if err != nil || u.CNPJ != pj || u.RazaoSocial != e.RazaoSocial {
		t.Fatalf("ficha única: %v", err)
	}
	if _, err := s.Empresa(ctx, "11222333000181", false); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("CNPJ fora da base: %v", err)
	}
	muitos := make([]string, MaximoDetalhes+1)
	if _, err := s.Detalhar(ctx, muitos, false); err == nil {
		t.Fatal("mais de 500 CNPJs deveria ser recusado")
	}
}

func TestCatalogos(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	cnaes, err := s.CatalogoCnaes(ctx, "PADARIA", "", "", 50)
	if err != nil || len(cnaes) < 3 {
		t.Fatalf("cnaes por texto: %v %v", cnaes, err)
	}
	if cnaes[0].Nivel != "classe" || cnaes[0].Codigo != "47211" {
		t.Fatalf("o nível mais geral vem primeiro: %+v", cnaes[0])
	}
	if r, _ := s.CatalogoCnaes(ctx, "47.1", "", "", 3); len(r) != 3 || r[0].Codigo != "471" || r[0].Nivel != "grupo" {
		t.Fatalf("cnaes por prefixo de código: %+v", r)
	}
	if r, _ := s.CatalogoCnaes(ctx, "construcao", "divisao", "", 0); len(r) == 0 || r[0].Nivel != "divisao" || len(r[0].Codigo) != 2 {
		t.Fatalf("cnaes por nível, sem acento: %+v", r)
	}
	if r, _ := s.CatalogoCnaes(ctx, "", "", "47, 4721102,56112,999", 0); len(r) != 3 {
		t.Fatalf("cnaes por código exato em qualquer nível: %+v", r)
	}
	if r, _ := s.CatalogoCnaes(ctx, "", "", "", 500); len(r) != 50 {
		t.Fatalf("limite máximo de 50: %d", len(r))
	}
	mun, err := s.CatalogoMunicipios(ctx, "ce", "", "", 0)
	if err != nil || len(mun) != 4 || mun[0].Nome != "FORTALEZA" || mun[0].Codigo != 1389 || mun[0].Empresas < mun[1].Empresas {
		t.Fatalf("municípios do CE: %+v %v", mun, err)
	}
	if r, _ := s.CatalogoMunicipios(ctx, "", "sao p", "", 0); len(r) != 1 || r[0].UF != "SP" {
		t.Fatalf("municípios por nome: %+v", r)
	}
	if r, _ := s.CatalogoMunicipios(ctx, "", "", "1389,7107,9999", 0); len(r) != 2 {
		t.Fatalf("municípios por código: %+v", r)
	}
	nat, _ := s.CatalogoNaturezas(ctx)
	pf := map[int]bool{}
	for _, n := range nat {
		pf[n.Codigo] = n.PessoaFisica
	}
	if len(nat) < 50 || !pf[2135] || !pf[4120] || pf[2062] {
		t.Fatalf("naturezas: %d, pf=%v", len(nat), pf)
	}
	ufs, _ := s.CatalogoUFs(ctx)
	total := 0
	for _, u := range ufs {
		total += u.Empresas
	}
	if len(ufs) < 10 || int64(total) != contarSQL(t, "true") {
		t.Fatalf("UFs: %d, soma %d", len(ufs), total)
	}
	b, err := s.Situacao(ctx)
	if err != nil || !b.Pronta || !b.Amostra || b.Empresas != int64(total) || b.EspacoBytes == 0 || b.AtualizadaEm == nil || b.UFs == nil {
		t.Fatalf("situação: %+v %v", b, err)
	}
}

func TestBaseQueAindaNaoExiste(t *testing.T) {
	servico(t)
	s := Novo(poolTeste, "prospec", false)
	ctx := context.Background()
	if b, err := s.Situacao(ctx); err != nil || b.Pronta || b.Amostra {
		t.Fatalf("situação sem base: %+v %v", b, err)
	}
	if _, err := s.Contar(ctx, Filtros{UFs: []string{"CE"}}); !errors.Is(err, ErrNaoPronta) {
		t.Fatalf("contagem sem base: %v", err)
	}
	if _, err := s.Detalhar(ctx, []string{"00000001000136"}, false); !errors.Is(err, ErrNaoPronta) {
		t.Fatalf("detalhes sem base: %v", err)
	}
}

// Fica por último (a ordem do arquivo é a ordem de execução): troca a base.
func TestCursorDeVersaoAntigaERecusado(t *testing.T) {
	s := servico(t)
	ctx := context.Background()
	filtros := Filtros{UFs: []string{"CE"}}
	r, err := s.Candidatos(ctx, filtros, OrdemRecentes, "", 10)
	if err != nil || r.Cursor == "" {
		t.Fatal(err)
	}
	// o cursor é da busca em que nasceu
	var ef *ErroFiltro
	if _, err := s.Candidatos(ctx, Filtros{UFs: []string{"SP"}}, OrdemRecentes, r.Cursor, 10); !errors.As(err, &ef) {
		t.Fatalf("cursor em outro filtro: %v", err)
	}
	if _, err := s.Candidatos(ctx, filtros, OrdemRecentes, "isto-nao-e-cursor", 10); !errors.As(err, &ef) {
		t.Fatalf("cursor inválido: %v", err)
	}
	antes, _ := s.Contar(ctx, filtros)

	// troca de base (mesmo conteúdo, versão nova): o id é recalculado
	if err := amostra.Montar(ctx, poolTeste); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Candidatos(ctx, filtros, OrdemRecentes, r.Cursor, 10); !errors.Is(err, ErrCursorAntigo) {
		t.Fatalf("esperado ErrCursorAntigo, veio %v", err)
	}
	// busca nova funciona na base nova, com versão diferente e o mesmo resultado
	r2, err := s.Candidatos(ctx, filtros, OrdemRecentes, "", 10)
	if err != nil || r2.Versao == r.Versao || strings.Join(r2.CNPJs, ",") != strings.Join(r.CNPJs, ",") {
		t.Fatalf("busca depois da troca: %+v %v", r2, err)
	}
	if depois, err := s.Contar(ctx, filtros); err != nil || depois.Total != antes.Total {
		t.Fatalf("contagem depois da troca: %+v %v", depois, err)
	}
	if !amostra.Pronta(ctx, poolTeste) {
		t.Fatal("a amostra deveria estar pronta")
	}
}
