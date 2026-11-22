package carga

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// bancoFalso recebe o COPY em memória: os testes de leitura não precisam de
// PostgreSQL. Como no banco, um COPY que falha não grava nada.
type bancoFalso struct {
	linhas map[string][]map[string]any
	execs  []string
}

func (b *bancoFalso) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	b.execs = append(b.execs, sql)
	return pgconn.CommandTag{}, nil
}
func (b *bancoFalso) Query(context.Context, string, ...any) (pgx.Rows, error) { panic("não usado") }
func (b *bancoFalso) QueryRow(context.Context, string, ...any) pgx.Row        { panic("não usado") }
func (b *bancoFalso) CopyFrom(_ context.Context, tabela pgx.Identifier, colunas []string, fonte pgx.CopyFromSource) (int64, error) {
	var novas []map[string]any
	for fonte.Next() {
		v, err := fonte.Values()
		if err != nil {
			return 0, err
		}
		l := map[string]any{}
		for i, c := range colunas {
			l[c] = v[i]
		}
		novas = append(novas, l)
	}
	if err := fonte.Err(); err != nil {
		return 0, err
	}
	if b.linhas == nil {
		b.linhas = map[string][]map[string]any{}
	}
	nome := tabela[len(tabela)-1]
	b.linhas[nome] = append(b.linhas[nome], novas...)
	return int64(len(novas)), nil
}

func carregarFixture(t *testing.T, b *bancoFalso, l Leiaute, arquivo string, ufs []string) (Estatisticas, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", arquivo))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return CarregarCSV(context.Background(), b, "carga", l, strings.TrimSuffix(arquivo, ".csv"), f, ufs, nil)
}

func porChave(linhas []map[string]any, coluna string) map[string]map[string]any {
	m := map[string]map[string]any{}
	for _, l := range linhas {
		m[l[coluna].(string)] = l
	}
	return m
}

// Os fixtures estão na codificação original da Receita (ISO-8859-1, tudo
// entre aspas, ";") e trazem as armadilhas confirmadas na pesquisa.
func TestEstabelecimentosComNULQuebraDeLinhaETelefones(t *testing.T) {
	b := &bancoFalso{}
	est, err := carregarFixture(t, b, LeiauteEstabelecimentos, "Estabelecimentos.csv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if est.Lidas != 5 || est.Gravadas != 4 || est.NULs != 1 {
		t.Fatalf("lidas=%d gravadas=%d nuls=%d; esperado 5, 4 (a baixada fica fora) e 1 NUL", est.Lidas, est.Gravadas, est.NULs)
	}
	e := porChave(b.linhas["estabelecimentos"], "cnpj")

	p := e["00000001000136"]
	if p == nil {
		t.Fatalf("estabelecimento ativo não entrou: %v", e)
	}
	// byte NUL removido do complemento (abortaria o COPY do PostgreSQL 16)
	if p["complemento"] != "SALA 2" {
		t.Errorf("complemento = %q", p["complemento"])
	}
	// ISO-8859-1 → UTF-8, com coluna de busca sem acento
	if p["nome_fantasia"] != "PADARIA SÃO JOSÉ" || p["fantasia_busca"] != "PADARIA SAO JOSE" {
		t.Errorf("fantasia = %q / %q", p["nome_fantasia"], p["fantasia_busca"])
	}
	if p["fone1"] != "85999887766" || p["tipo1"] != texto.Celular || p["fone2"] != "8532456789" || p["tipo2"] != texto.Fixo {
		t.Errorf("telefones = %v %v %v %v", p["fone1"], p["tipo1"], p["fone2"], p["tipo2"])
	}
	// o valor cru da Receita fica guardado para reclassificar depois
	if p["ddd1"] != "85" || p["tel1"] != "99887766" {
		t.Errorf("telefone cru = %v %v", p["ddd1"], p["tel1"])
	}
	if p["cnaes_sec"] != "5611203,1091102" { // sem o principal repetido
		t.Errorf("cnaes_sec = %v", p["cnaes_sec"])
	}
	if p["email"] != "contato@padaria.com.br" || p["email_tipo"] != texto.EmailCorporativo || p["data_inicio"] != "2010-03-15" || p["matriz"] != "1" {
		t.Errorf("email/data = %v %v %v", p["email"], p["email_tipo"], p["data_inicio"])
	}

	m := e["00000002000117"]
	// quebra de linha e aspas duplicadas dentro do campo: um registro só
	if m["nome_fantasia"] != `MERCADO "BOM PREÇO" DO CENTRO` {
		t.Errorf("fantasia com quebra de linha = %q", m["nome_fantasia"])
	}
	if m["fone1"] != "1145671234" || m["tipo1"] != texto.Indefinido || m["fone2"] != nil { // DDD 11 com 4; 99999999 descartado
		t.Errorf("telefones SP = %v %v %v", m["fone1"], m["tipo1"], m["fone2"])
	}
	if m["cep"] != "01001000" || m["email"] != nil || m["bairro"] != "SÉ" || m["bairro_busca"] != "SE" {
		t.Errorf("cep/email/bairro = %v %v %v %v", m["cep"], m["email"], m["bairro"], m["bairro_busca"])
	}

	o := e["00000004000189"]
	// CPF formatado no nome fantasia é cortado; controle (TAB, 0x1A) some
	if o["nome_fantasia"] != "JOAO" || o["complemento"] != "FUNDOS" {
		t.Errorf("fantasia/complemento = %q %q", o["nome_fantasia"], o["complemento"])
	}
	if o["data_situacao"] != nil || o["data_inicio"] != nil {
		t.Errorf("datas zeradas deveriam ser nulas: %v %v", o["data_situacao"], o["data_inicio"])
	}
	// primeiro telefone é lixo (12345678): o segundo assume o lugar
	if o["fone1"] != "21988776655" || o["tipo1"] != texto.Celular || o["fone2"] != nil {
		t.Errorf("telefones RJ = %v %v %v", o["fone1"], o["tipo1"], o["fone2"])
	}
	if est.CortesCPF.Formatado != 1 {
		t.Errorf("cortes de CPF = %+v", est.CortesCPF)
	}
	// CNPJ alfanumérico (em produção desde julho de 2026)
	if a := e["12ABC34501DE35"]; a == nil || a["cnpj_basico"] != "12ABC345" || a["matriz"] != "0" || a["uf"] != "MG" {
		t.Errorf("CNPJ alfanumérico = %v", a)
	}
	if _, baixada := e["00000003000106"]; baixada {
		t.Error("estabelecimento com situação 08 não pode entrar")
	}
}

func TestRecorteDeUFs(t *testing.T) {
	b := &bancoFalso{}
	est, err := carregarFixture(t, b, LeiauteEstabelecimentos, "Estabelecimentos.csv", []string{"CE", "MG"})
	if err != nil {
		t.Fatal(err)
	}
	if est.Gravadas != 2 {
		t.Fatalf("gravadas = %d, esperado 2 (CE e MG)", est.Gravadas)
	}
}

func TestNumeroDeCamposErradoAbortaSemGravarNada(t *testing.T) {
	b := &bancoFalso{}
	est, err := carregarFixture(t, b, LeiauteEstabelecimentos, "Estabelecimentos_29_campos.csv", nil)
	if err == nil || !strings.Contains(err.Error(), "fora do leiaute") {
		t.Fatalf("esperado erro de leiaute, veio %v", err)
	}
	if est.Gravadas != 0 || len(b.linhas["estabelecimentos"]) != 0 {
		t.Fatalf("nada deveria ter sido gravado: %d", len(b.linhas["estabelecimentos"]))
	}
}

func TestEmpresasCortaCPFEPulaFantasma(t *testing.T) {
	b := &bancoFalso{}
	est, err := carregarFixture(t, b, LeiauteEmpresas, "Empresas.csv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if est.Lidas != 5 || est.Gravadas != 4 || est.Fantasmas != 1 || est.CortesCPF.Fim != 1 {
		t.Fatalf("estatísticas = %+v", est)
	}
	e := porChave(b.linhas["empresas"], "cnpj_basico")
	if len(e) != 4 {
		t.Fatalf("a linha fantasma repetiu um cnpj_basico: %v", b.linhas["empresas"])
	}
	// CPF completo no fim da razão social de empresário individual
	if e["00000002"]["razao_social"] != "MARIA DA SILVA" || e["00000002"]["natureza"] != "2135" {
		t.Errorf("razão com CPF = %v", e["00000002"])
	}
	// o prefixo novo do MEI (raiz do CNPJ) fica
	if e["00000004"]["razao_social"] != "61.114.725 JOSE DE SOUZA" {
		t.Errorf("prefixo do MEI = %v", e["00000004"]["razao_social"])
	}
	if a := e["12ABC345"]; a["razao_social"] != "EMPRESA ALFA S.A." || a["capital"] != "1500000.50" || a["porte"] != "05" {
		t.Errorf("empresa alfa = %v", a)
	}
}

func TestSociosDescartaMenoresEMantemMascara(t *testing.T) {
	b := &bancoFalso{}
	est, err := carregarFixture(t, b, LeiauteSocios, "Socios.csv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if est.Gravadas != 4 || est.Menores != 2 {
		t.Fatalf("estatísticas = %+v", est)
	}
	s := porChave(b.linhas["socios"], "nome")
	if s["JOSÉ EXEMPLO DA SILVA"]["documento"] != "***444777**" || s["JOSÉ EXEMPLO DA SILVA"]["desde"] != "2010-03-15" || s["JOSÉ EXEMPLO DA SILVA"]["tipo"] != "pf" {
		t.Errorf("sócio PF = %v", s["JOSÉ EXEMPLO DA SILVA"])
	}
	if s["HOLDING EXEMPLO LTDA"]["tipo"] != "pj" || s["HOLDING EXEMPLO LTDA"]["documento"] != "00000009" { // 8 dígitos desde 2026-08
		t.Errorf("sócio PJ = %v", s["HOLDING EXEMPLO LTDA"])
	}
	if j := s["JOHN EXAMPLE"]; j["tipo"] != "estrangeiro" || j["representante"] != "PROCURADOR EXEMPLO" || j["desde"] != nil {
		t.Errorf("sócio estrangeiro = %v", j)
	}
	// CPF que viesse completo nunca passaria adiante
	if s["CPF COMPLETO EXEMPLO"]["documento"] != "***444777**" {
		t.Errorf("CPF completo não foi mascarado: %v", s["CPF COMPLETO EXEMPLO"]["documento"])
	}
}

func TestSimplesSoOptantes(t *testing.T) {
	b := &bancoFalso{}
	if _, err := carregarFixture(t, b, LeiauteSimples, "Simples.csv", nil); err != nil {
		t.Fatal(err)
	}
	s := porChave(b.linhas["simples"], "cnpj_basico")
	if len(s) != 2 || s["00000001"]["simples"] != "S" || s["00000001"]["mei"] != "N" || s["00000002"]["mei"] != "S" {
		t.Fatalf("simples = %v", s)
	}
}

func TestZipComMaisDeUmMembroERecusado(t *testing.T) {
	dir := t.TempDir()
	caminho := filepath.Join(dir, "Cnaes.zip")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, n := range []string{"a.csv", "b.csv"} {
		w, _ := z.Create(n)
		_, _ = w.Write([]byte("\"0111301\";\"Cultivo de arroz\"\n"))
	}
	_ = z.Close()
	if err := os.WriteFile(caminho, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CarregarZip(context.Background(), &bancoFalso{}, "carga", LeiauteCnaes, caminho, nil, nil); err == nil {
		t.Fatal("zip com dois membros deveria ser recusado")
	}
}

func TestLeiautePeloNomeDoArquivo(t *testing.T) {
	if len(ArquivosCNPJ()) != 37 {
		t.Fatalf("esperados 37 arquivos, há %d", len(ArquivosCNPJ()))
	}
	casos := map[string]string{"Empresas0.zip": "empresas", "Estabelecimentos9.zip": "estabelecimentos", "Socios3.zip": "socios", "Simples.zip": "simples",
		"Cnaes.zip": "cnaes", "Municipios.zip": "municipios", "Naturezas.zip": "naturezas", "Qualificacoes.zip": "qualificacoes"}
	for nome, tabela := range casos {
		if l, ok := LeiauteDoArquivo(nome); !ok || l.Tabela != tabela {
			t.Errorf("%s → %q %v", nome, l.Tabela, ok)
		}
	}
	for _, nome := range []string{"Paises.zip", "Motivos.zip"} {
		if _, ok := LeiauteDoArquivo(nome); ok {
			t.Errorf("%s não é carregado", nome)
		}
	}
}

// ---------------------------------------------------------------- PGFN

func carregarPGFN(t *testing.T, b *bancoFalso, arquivo string) (Estatisticas, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", arquivo))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return CarregarPGFN(context.Background(), b, "carga", arquivo, f)
}

func TestPGFNSidaComTodasAsManias(t *testing.T) {
	b := &bancoFalso{}
	est, err := carregarPGFN(t, b, "pgfn_sida.csv")
	if err != nil {
		t.Fatal(err)
	}
	// 7 linhas: 4 gravadas; corresponsável e pessoa física ficam fora sem
	// contar como defeito; a linha com ";" a mais no nome é rejeitada
	if est.Lidas != 7 || est.Gravadas != 4 || est.Rejeitadas != 1 {
		t.Fatalf("estatísticas = %+v", est)
	}
	l := porChave(b.linhas["pgfn"], "inscricao")
	// inscrição gaúcha: perde os zeros e ganha ".0" no arquivo
	g := l["0069200098400"]
	if g == nil || g["cnpj"] != "00000001000136" || g["sistema"] != DividaGeral || g["irregular"] != "t" || g["ajuizada"] != "t" || g["data"] != "1992-12-15" || g["valor"] != "14439.73" {
		t.Errorf("inscrição com .0 = %v (todas: %v)", g, l)
	}
	// sentinela 01/01/1000 vira nula; ".00" é zero; benefício fiscal é regular
	s := l["3062004015281"]
	if s["data"] != nil || s["valor"] != "0.00" || s["irregular"] != "f" || s["ajuizada"] != "f" || s["cnpj"] != "00000001000136" {
		t.Errorf("sentinela = %v", s)
	}
	if a := l["6062100000011"]; a["cnpj"] != "12ABC34501DE35" || a["irregular"] != "f" {
		t.Errorf("CNPJ alfanumérico na PGFN = %v", a)
	}
	// data impossível (31/02) vira nula, a linha continua valendo
	if d := l["7062100000033"]; d == nil || d["data"] != nil || d["valor"] != "1000.5" {
		t.Errorf("data impossível = %v", d)
	}
	for _, linha := range b.linhas["pgfn"] {
		for c, v := range linha {
			if s, ok := v.(string); ok && strings.Contains(s, "EXEMPLO") {
				t.Errorf("o nome do devedor não pode ser gravado (coluna %s)", c)
			}
		}
	}
}

func TestPGFNPrevEFgts(t *testing.T) {
	b := &bancoFalso{}
	if est, err := carregarPGFN(t, b, "pgfn_prev.csv"); err != nil || est.Gravadas != 1 {
		t.Fatalf("PREV: %+v %v", est, err)
	}
	// "Principal" em minúsculas nos outros dois sistemas; arquivo sem LF no fim
	if est, err := carregarPGFN(t, b, "pgfn_fgts.csv"); err != nil || est.Gravadas != 1 || est.Lidas != 2 {
		t.Fatalf("FGTS: %+v %v", est, err)
	}
	l := porChave(b.linhas["pgfn"], "sistema")
	if p := l[DividaPrevidenciaria]; p["inscricao"] != "0000123456789" || p["irregular"] != "f" || p["valor"] != "2500.00" {
		t.Errorf("PREV = %v", p)
	}
	if f := l[DividaFGTS]; f["inscricao"] != "FGSP202200129" || f["cnpj"] != "00000002000117" || f["irregular"] != "t" || f["data"] != "2022-12-21" {
		t.Errorf("FGTS = %v", f)
	}
}

func TestPGFNCabecalhoDesconhecidoPara(t *testing.T) {
	b := &bancoFalso{}
	if _, err := carregarPGFN(t, b, "pgfn_cabecalho_novo.csv"); err == nil || !strings.Contains(err.Error(), "cabeçalho desconhecido") {
		t.Fatalf("esperado erro de cabeçalho, veio %v", err)
	}
}

func TestPGFNMuitosRejeitosAbortam(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(sistemasPGFN[0].cabecalho + "\n")
	for i := 0; i < 40; i++ {
		sb.WriteString("linha;quebrada\n")
	}
	b := &bancoFalso{}
	_, err := CarregarPGFN(context.Background(), b, "carga", "ruim.csv", strings.NewReader(sb.String()))
	if err == nil || !strings.Contains(err.Error(), "fora do padrão") || len(b.linhas["pgfn"]) != 0 {
		t.Fatalf("esperado aborto por rejeitos, veio %v", err)
	}
}

func TestReferenciaPGFN(t *testing.T) {
	if r, ok := ReferenciaPGFN("2026_trimestre_03/"); !ok || r != "2026-T3" {
		t.Fatalf("referência = %q %v", r, ok)
	}
	if PastaPGFN("2026-T3") != "2026_trimestre_03" {
		t.Fatal("pasta")
	}
	if _, ok := ReferenciaPGFN("Portal_da_Cidadania_Tributaria/"); ok {
		t.Fatal("pasta que não é trimestre")
	}
	casos := map[string]string{"Dados_abertos_FGTS.zip": DividaFGTS, "Dados_abertos_Previdenciario.zip": DividaPrevidenciaria,
		"Dados_abertos_Sistema_SIDA.zip": DividaGeral, "Dados_abertos_Nao_Previdenciario.zip": DividaGeral}
	for nome, tipo := range casos {
		if TipoZipPGFN(nome) != tipo {
			t.Errorf("%s → %s", nome, TipoZipPGFN(nome))
		}
	}
}
