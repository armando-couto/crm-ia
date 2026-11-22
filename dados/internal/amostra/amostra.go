// Package amostra monta a base de demonstração (DADOS_AMOSTRA=true): alguns
// milhares de estabelecimentos inventados e determinísticos, em várias UFs e
// CNAEs, com sócios e dívidas, passando pelo mesmo passo de construção da
// base real. Nunca acessa a rede.
//
// Tudo é fictício por construção: CNPJs com raiz na faixa 00.000.0NN e
// dígitos verificadores válidos; telefones com DDD real e assinante
// 90000-0NNN (celular) ou 3000-0NNN (fixo); e-mails em @exemplo.invalid;
// sócios com nomes genéricos numerados. Só os catálogos (CNAEs, municípios,
// naturezas e qualificações) são os oficiais da Receita.
package amostra

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

var (
	//go:embed catalogos/Cnaes.zip
	zipCnaes []byte
	//go:embed catalogos/Municipios.zip
	zipMunicipios []byte
	//go:embed catalogos/Naturezas.zip
	zipNaturezas []byte
	//go:embed catalogos/Qualificacoes.zip
	zipQualificacoes []byte
)

const (
	// Geracao muda quando o gerador muda: base de geração antiga é refeita
	// na subida.
	Geracao        = "1"
	PrefixoVersao  = "amostra-"
	ReferenciaCNPJ = "2026-11"
	ReferenciaPGFN = "2026-T3"
	esquemaCarga   = "carga_amostra"
	chaveTrava     = 0x6372_6d69_6162 // trava consultiva própria da amostra
)

// Raizes é o número de raízes de CNPJ (00.000.001 a 00.000.099).
const Raizes = 99

type cidade struct {
	codigo, uf, ddd string
	peso            int
}

// Municípios (código da Receita), com o DDD real de cada um.
var cidades = []cidade{
	{"1389", "CE", "85", 14}, {"1373", "CE", "85", 3}, {"1559", "CE", "88", 3}, {"1447", "CE", "88", 3},
	{"7107", "SP", "11", 12}, {"6477", "SP", "11", 3}, {"6291", "SP", "19", 4}, {"6969", "SP", "16", 3}, {"7071", "SP", "13", 2},
	{"6001", "RJ", "21", 7}, {"5865", "RJ", "21", 2}, {"4123", "MG", "31", 6}, {"4371", "MG", "31", 2}, {"5403", "MG", "34", 2},
	{"7535", "PR", "41", 4}, {"7667", "PR", "43", 2}, {"7691", "PR", "44", 2}, {"8801", "RS", "51", 4}, {"8599", "RS", "54", 2},
	{"3849", "BA", "71", 4}, {"3515", "BA", "75", 2}, {"2531", "PE", "81", 4}, {"2457", "PE", "81", 2},
	{"9373", "GO", "62", 3}, {"9221", "GO", "62", 1}, {"9701", "DF", "61", 4}, {"8105", "SC", "48", 2}, {"8179", "SC", "47", 2}, {"8047", "SC", "47", 2},
	{"0255", "AM", "92", 2}, {"0427", "PA", "91", 2}, {"1761", "RN", "84", 2}, {"1759", "RN", "84", 1}, {"2051", "PB", "83", 2},
	{"1219", "PI", "86", 2}, {"0921", "MA", "98", 2}, {"5705", "ES", "27", 2}, {"9067", "MT", "65", 2}, {"9051", "MS", "67", 2},
	{"2785", "AL", "82", 1}, {"3105", "SE", "79", 1},
}

// Dois primeiros dígitos do CEP de cada UF usada na amostra.
var cepPorUF = map[string]string{"CE": "60", "SP": "01", "RJ": "20", "MG": "30", "PR": "80", "RS": "90", "BA": "40", "PE": "50", "GO": "74",
	"DF": "70", "SC": "88", "AM": "69", "PA": "66", "RN": "59", "PB": "58", "PI": "64", "MA": "65", "ES": "29", "MT": "78", "MS": "79", "AL": "57", "SE": "49"}

type ramo struct {
	cnae, nome string
	outros     []string // CNAEs secundários típicos
}

var ramos = []ramo{
	{"4721102", "Padaria", []string{"1091102", "5611203"}}, {"5611201", "Restaurante", []string{"5611203", "5620104"}},
	{"5611203", "Lanchonete", []string{"5611201"}}, {"4711302", "Supermercado", []string{"4712100", "4721102"}},
	{"4712100", "Mercearia", []string{"4723700", "4729699"}}, {"4781400", "Loja de Roupas", []string{"4789099"}},
	{"4771701", "Farmácia", []string{"4772500"}}, {"9602501", "Salão de Beleza", []string{"4772500"}},
	{"4520001", "Oficina Mecânica", []string{"4530703"}}, {"6201501", "Desenvolvimento de Software", []string{"6204000"}},
	{"6204000", "Consultoria em TI", []string{"6201501", "8599604"}}, {"6920601", "Contabilidade", []string{"8211300"}},
	{"8630503", "Clínica Médica", []string{"8650004"}}, {"8630504", "Clínica Odontológica", nil},
	{"4120400", "Construtora", []string{"4399103", "4330404", "7112000"}}, {"4744099", "Materiais de Construção", []string{"4789099"}},
	{"4751201", "Loja de Informática", []string{"6204000"}}, {"9313100", "Academia", nil},
	{"4930202", "Transportadora", nil}, {"6911701", "Advocacia", nil}, {"8121400", "Limpeza e Conservação", []string{"8011101"}},
	{"4754701", "Loja de Móveis", nil}, {"9609208", "Pet Shop", []string{"4789099"}}, {"7319002", "Promoção de Vendas", []string{"8211300"}},
}

// gerador é um splitmix64: a mesma sequência em qualquer versão do Go.
type gerador uint64

func (g *gerador) prox() uint64 {
	*g += 0x9e3779b97f4a7c15
	z := uint64(*g)
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (g *gerador) n(limite int) int { return int(g.prox() % uint64(limite)) }

func (g *gerador) chance(porCento int) bool { return g.n(100) < porCento }

func s(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// Linhas é a base gerada, no formato das tabelas de carga.
type Linhas struct {
	Empresas, Estabelecimentos, Simples, Socios, Dividas [][]any
}

// Gerar produz sempre as mesmas linhas.
func Gerar() Linhas {
	var l Linhas
	g := gerador(20261122)
	var sorteio []int
	for i, c := range cidades {
		for j := 0; j < c.peso; j++ {
			sorteio = append(sorteio, i)
		}
	}
	celulares, fixos := map[string]int{}, map[string]int{}
	contador := 0 // escritórios de contabilidade fictícios: telefone e e-mail repetidos em várias raízes

	for raiz := 1; raiz <= Raizes; raiz++ {
		basico := fmt.Sprintf("%08d", raiz)
		rm := ramos[g.n(len(ramos))]
		natureza, sufixo := "2062", "Ltda"
		pf, mei := false, false
		switch x := g.n(100); {
		case x < 12:
			natureza, sufixo, pf = "2135", "", true
			mei = g.chance(70)
		case x < 20:
			natureza, sufixo = "2305", "Eireli"
		case x < 25:
			natureza, sufixo = "2046", "S.A."
		case x < 30:
			natureza, sufixo = "2240", "Sociedade Simples Ltda"
		}
		porte, capital := "01", 1000*(1+g.n(80))
		switch x := g.n(100); {
		case pf:
		case x < 25:
			porte, capital = "03", 100000*(1+g.n(20))
		case x < 40:
			porte, capital = "05", 1000000*(1+g.n(50))
		}
		razao := strings.TrimSpace(fmt.Sprintf("%s Exemplo %02d %s", rm.nome, raiz, sufixo))
		if pf {
			razao = fmt.Sprintf("Pessoa Exemplo %02d", raiz)
		}
		l.Empresas = append(l.Empresas, []any{basico, razao, texto.SemAcento(razao), natureza, fmt.Sprintf("%d.00", capital), porte})
		simples := mei || (porte != "05" && g.chance(75))
		if simples || mei {
			l.Simples = append(l.Simples, []any{basico, sn(simples), sn(mei)})
		}
		if !pf {
			for i, n := 0, 1+g.n(3); i < n; i++ {
				qualificacao := "49" // sócio-administrador
				if i > 0 && g.chance(60) {
					qualificacao = "22" // sócio
				}
				if natureza == "2046" {
					qualificacao = []string{"16", "10", "05"}[i%3] // presidente, diretor, administrador
				}
				numero := raiz*10 + i
				l.Socios = append(l.Socios, []any{basico, "pf", fmt.Sprintf("Sócio Exemplo %03d", numero), fmt.Sprintf("***%06d**", numero),
					qualificacao, fmt.Sprintf("%04d-%02d-%02d", 2000+g.n(26), 1+g.n(12), 1+g.n(28)), nil})
			}
		}

		estabelecimentos := 38
		if pf {
			estabelecimentos = 1 + g.n(2)
		}
		for ordem := 1; ordem <= estabelecimentos; ordem++ {
			base12 := fmt.Sprintf("%s%04d", basico, ordem)
			cnpj := base12 + texto.DVCNPJ(base12)
			cid := cidades[sorteio[g.n(len(sorteio))]]
			cnae, sec := rm.cnae, ""
			if ordem > 1 && g.chance(30) {
				outro := ramos[g.n(len(ramos))]
				cnae = outro.cnae
			}
			var secs []string
			for _, o := range rm.outros {
				if o != cnae && g.chance(60) {
					secs = append(secs, o)
				}
			}
			sec = strings.Join(secs, ",")
			matriz := "0"
			fantasia := fmt.Sprintf("%s Exemplo %02d - Unidade %02d", rm.nome, raiz, ordem)
			if ordem == 1 {
				matriz = "1"
				fantasia = fmt.Sprintf("%s Exemplo %02d", rm.nome, raiz)
			}
			if pf {
				fantasia = ""
				if g.chance(50) {
					fantasia = fmt.Sprintf("%s Exemplo %02d", rm.nome, raiz)
				}
			}
			inicio := fmt.Sprintf("%04d-%02d-%02d", 1996+g.n(30), 1+g.n(12), 1+g.n(28))
			if g.chance(8) {
				inicio = fmt.Sprintf("2026-%02d-%02d", 7+g.n(4), 1+g.n(28)) // abertas há pouco
			}

			var ddd1, tel1, ddd2, tel2, f1, t1, f2, t2, email string
			celular := func() (string, string) {
				n := celulares[cid.ddd] % 990
				celulares[cid.ddd]++
				return fmt.Sprintf("00000%03d", n), fmt.Sprintf("%s90000%04d", cid.ddd, n)
			}
			fixo := func() (string, string) {
				n := fixos[cid.ddd] % 990
				fixos[cid.ddd]++
				return fmt.Sprintf("3000%04d", n), fmt.Sprintf("%s3000%04d", cid.ddd, n)
			}
			doContador := false
			switch x := g.n(100); {
			case x < 50:
				tel1, f1 = celular()
				ddd1, t1 = cid.ddd, texto.Celular
			case x < 67:
				tel1, f1 = celular()
				tel2, f2 = fixo()
				ddd1, t1, ddd2, t2 = cid.ddd, texto.Celular, cid.ddd, texto.Fixo
			case x < 78:
				tel1, f1 = fixo()
				ddd1, t1 = cid.ddd, texto.Fixo
			case x < 90:
				// telefone do contador: o mesmo número em muitas raízes
				k := contador % 6
				contador++
				ddd1, tel1, t1 = "85", fmt.Sprintf("3000%04d", 990+k), texto.Fixo
				f1 = "85" + tel1
				doContador = true
			}
			switch x := g.n(100); {
			case doContador && x < 70:
				email = fmt.Sprintf("contabilidade%d@exemplo.invalid", contador%6)
			case x < 60:
				email = fmt.Sprintf("contato.%s.%04d@exemplo.invalid", basico, ordem)
			}
			bairro := []string{"Centro", "Aldeota", "Jardim América", "Boa Vista", "São José", "Vila Nova"}[g.n(6)]
			l.Estabelecimentos = append(l.Estabelecimentos, []any{cnpj, basico, matriz, s(fantasia), s(texto.SemAcento(fantasia)),
				inicio, inicio, cnae, s(sec), "RUA", fmt.Sprintf("EXEMPLO %d", 1+g.n(90)), fmt.Sprintf("%d", 10+g.n(2000)), nil,
				bairro, texto.SemAcento(bairro), fmt.Sprintf("%s%03d000", cepPorUF[cid.uf], g.n(1000)), cid.uf, cid.codigo,
				s(ddd1), s(tel1), s(ddd2), s(tel2), s(f1), s(t1), s(f2), s(t2), s(email), s(texto.TipoEmail(email))})

			if g.chance(13) {
				for i, n := 0, 1+g.n(3); i < n; i++ {
					sistema := []string{carga.DividaGeral, carga.DividaGeral, carga.DividaPrevidenciaria, carga.DividaFGTS}[g.n(4)]
					l.Dividas = append(l.Dividas, []any{cnpj, sistema, fmt.Sprintf("%013d", raiz*100000+ordem*10+i), tf(g.chance(65)), tf(g.chance(40)),
						fmt.Sprintf("%04d-%02d-%02d", 2012+g.n(14), 1+g.n(12), 1+g.n(28)), fmt.Sprintf("%d.%02d", 500+g.n(400000), g.n(100))})
				}
			}
		}
	}
	return l
}

func sn(v bool) string {
	if v {
		return "S"
	}
	return "N"
}

func tf(v bool) string {
	if v {
		return "t"
	}
	return "f"
}

// Pronta diz se a base de amostra desta geração já existe.
func Pronta(ctx context.Context, pool *pgxpool.Pool) bool {
	var versao string
	err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT versao FROM %s.meta LIMIT 1`, carga.EsquemaAmostra)).Scan(&versao)
	return err == nil && strings.HasPrefix(versao, PrefixoVersao+Geracao+"-")
}

// Garantir monta a base de amostra se ela ainda não existe nesta geração.
func Garantir(ctx context.Context, pool *pgxpool.Pool) error {
	if Pronta(ctx, pool) {
		return nil
	}
	return Montar(ctx, pool)
}

// Montar (re)constrói o esquema prospec_amostra, separado do real, e o põe
// em uso com uma versão nova.
func Montar(ctx context.Context, pool *pgxpool.Pool) error {
	inicio := time.Now()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	// uma montagem por vez no banco (duas réplicas, ou testes em paralelo)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, int64(chaveTrava)); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, int64(chaveTrava))
		_, _ = conn.Exec(context.WithoutCancel(ctx), `RESET statement_timeout`)
	}()
	if _, err := conn.Exec(ctx, `SET statement_timeout = 0`); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, esquemaCarga)); err != nil {
		return err
	}
	if err := carga.CriarTabelasDeCarga(ctx, conn, esquemaCarga); err != nil {
		return err
	}
	for _, c := range []struct {
		l     carga.Leiaute
		nome  string
		dados []byte
	}{{carga.LeiauteCnaes, "Cnaes", zipCnaes}, {carga.LeiauteMunicipios, "Municipios", zipMunicipios},
		{carga.LeiauteNaturezas, "Naturezas", zipNaturezas}, {carga.LeiauteQualificacoes, "Qualificacoes", zipQualificacoes}} {
		if _, err := carga.CarregarZipBytes(ctx, conn, esquemaCarga, c.l, c.nome, c.dados); err != nil {
			return fmt.Errorf("catálogo da amostra: %w", err)
		}
	}
	l := Gerar()
	for _, t := range []struct {
		tabela  string
		colunas []string
		linhas  [][]any
	}{{"empresas", carga.LeiauteEmpresas.Colunas, l.Empresas}, {"estabelecimentos", carga.LeiauteEstabelecimentos.Colunas, l.Estabelecimentos},
		{"simples", carga.LeiauteSimples.Colunas, l.Simples}, {"socios", carga.LeiauteSocios.Colunas, l.Socios},
		{"pgfn", carga.ColunasPGFN(), l.Dividas}} {
		if err := carga.InserirLinhas(ctx, conn, esquemaCarga, t.tabela, "amostra", t.colunas, t.linhas); err != nil {
			return fmt.Errorf("amostra, tabela %s: %w", t.tabela, err)
		}
	}
	if err := carga.AgregarDivida(ctx, conn, esquemaCarga, esquemaCarga+".divida"); err != nil {
		return err
	}
	if err := carga.Construir(ctx, conn, carga.Construcao{
		Destino: carga.EsquemaAmostra, Carga: esquemaCarga, Divida: esquemaCarga + ".divida",
		Versao:         fmt.Sprintf("%s%s-%x", PrefixoVersao, Geracao, time.Now().UnixNano()),
		ReferenciaCNPJ: ReferenciaCNPJ, ReferenciaPGFN: ReferenciaPGFN, Amostra: true,
	}); err != nil {
		return err
	}
	if err := carga.Trocar(ctx, conn, carga.EsquemaAmostra); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, esquemaCarga)); err != nil {
		return err
	}
	slog.Info("base de amostra montada", "estabelecimentos", len(l.Estabelecimentos), "duracao", time.Since(inicio).Round(time.Millisecond).String())
	return nil
}
