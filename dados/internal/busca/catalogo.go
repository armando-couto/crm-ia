package busca

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// A Receita só publica as subclasses (7 dígitos). Divisões, grupos e classes
// vêm da estrutura da CNAE 2.3 do IBGE (API de dados abertos, baixada em
// 22/11/2026): código, nível e descrição, separados por TAB.
//
//go:embed cnae_hierarquia.tsv
var hierarquiaTSV string

type nivelCnae struct {
	codigo, descricao, busca, nivel string
}

var hierarquia = func() []nivelCnae {
	var itens []nivelCnae
	for _, l := range strings.Split(hierarquiaTSV, "\n") {
		c := strings.SplitN(strings.TrimSpace(l), "\t", 3)
		if len(c) != 3 {
			continue
		}
		d := frase(c[2])
		itens = append(itens, nivelCnae{codigo: c[0], nivel: c[1], descricao: d, busca: texto.SemAcento(d)})
	}
	return itens
}()

// frase põe em caixa de frase a descrição que o IBGE publica em maiúsculas,
// para ficar igual às subclasses da Receita.
func frase(s string) string {
	r := []rune(strings.ToLower(s))
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

var nomesUF = map[string]string{
	"AC": "Acre", "AL": "Alagoas", "AM": "Amazonas", "AP": "Amapá", "BA": "Bahia", "CE": "Ceará", "DF": "Distrito Federal",
	"ES": "Espírito Santo", "GO": "Goiás", "MA": "Maranhão", "MG": "Minas Gerais", "MS": "Mato Grosso do Sul", "MT": "Mato Grosso",
	"PA": "Pará", "PB": "Paraíba", "PE": "Pernambuco", "PI": "Piauí", "PR": "Paraná", "RJ": "Rio de Janeiro", "RN": "Rio Grande do Norte",
	"RO": "Rondônia", "RR": "Roraima", "RS": "Rio Grande do Sul", "SC": "Santa Catarina", "SE": "Sergipe", "SP": "São Paulo",
	"TO": "Tocantins", "EX": "Exterior",
}

type faixa struct{ min, max int32 }

type municipio struct {
	ItemMunicipio
	busca string
}

// catalogo é a parte pequena da base mantida em memória por versão:
// catálogos e faixas de id.
type catalogo struct {
	base       Base
	cnaes      []nivelCnae // subclasses da Receita seguidas da hierarquia do IBGE
	cnaeDesc   map[int]string
	subclasses []int // códigos de 7 dígitos, para expandir prefixos
	municipios []municipio
	munPorCod  map[int]*municipio
	naturezas  []ItemNatureza
	natDesc    map[int]string
	ufs        []ItemUF
	faixaUF    map[string]faixa
	faixaMun   map[int]faixa
}

// Servico consulta um esquema de serviço (prospec ou prospec_amostra).
type Servico struct {
	pool    *pgxpool.Pool
	esquema string
	amostra bool

	mu  sync.Mutex
	cat *catalogo
}

// Novo cria o serviço de busca sobre o esquema.
func Novo(pool *pgxpool.Pool, esquema string, amostra bool) *Servico {
	return &Servico{pool: pool, esquema: esquema, amostra: amostra}
}

func semBase(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (pg.Code == "42P01" || pg.Code == "3F000")
}

func estourouTempo(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "57014"
}

// versao lê a versão do esquema em uso.
func (s *Servico) versao(ctx context.Context) (string, error) {
	var v string
	err := s.pool.QueryRow(ctx, fmt.Sprintf(`SELECT versao FROM %s.meta LIMIT 1`, s.esquema)).Scan(&v)
	if semBase(err) || errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNaoPronta
	}
	return v, err
}

// catalogoAtual devolve os catálogos da versão em uso, recarregando quando a
// base foi trocada.
func (s *Servico) catalogoAtual(ctx context.Context) (*catalogo, error) {
	v, err := s.versao(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cat != nil && s.cat.base.Versao == v {
		return s.cat, nil
	}
	c, err := s.carregarCatalogo(ctx)
	if err != nil {
		if semBase(err) {
			return nil, ErrNaoPronta
		}
		return nil, err
	}
	s.cat = c
	return c, nil
}

func (s *Servico) carregarCatalogo(ctx context.Context) (*catalogo, error) {
	e := s.esquema
	c := &catalogo{cnaeDesc: map[int]string{}, munPorCod: map[int]*municipio{}, natDesc: map[int]string{}, faixaUF: map[string]faixa{}, faixaMun: map[int]faixa{}}
	var atualizada time.Time
	if err := s.pool.QueryRow(ctx, fmt.Sprintf(`SELECT versao, referencia_cnpj, referencia_pgfn, empresas, socios, devedores, atualizada_em, amostra, ufs FROM %s.meta LIMIT 1`, e)).
		Scan(&c.base.Versao, &c.base.ReferenciaCNPJ, &c.base.ReferenciaPGFN, &c.base.Empresas, &c.base.Socios, &c.base.Devedores, &atualizada, &c.base.Amostra, &c.base.UFs); err != nil {
		return nil, err
	}
	c.base.Pronta, c.base.AtualizadaEm = true, &atualizada
	if c.base.UFs == nil {
		c.base.UFs = []string{}
	}

	linhas, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT codigo, descricao, descricao_busca FROM %s.cnae ORDER BY codigo`, e))
	if err != nil {
		return nil, err
	}
	var cod int
	var desc, busca string
	if _, err := pgx.ForEachRow(linhas, []any{&cod, &desc, &busca}, func() error {
		c.cnaes = append(c.cnaes, nivelCnae{codigo: fmt.Sprintf("%07d", cod), descricao: desc, busca: busca, nivel: "subclasse"})
		c.cnaeDesc[cod] = desc
		c.subclasses = append(c.subclasses, cod)
		return nil
	}); err != nil {
		return nil, err
	}
	c.cnaes = append(c.cnaes, hierarquia...)

	linhas, err = s.pool.Query(ctx, fmt.Sprintf(`SELECT codigo, descricao FROM %s.natureza ORDER BY codigo`, e))
	if err != nil {
		return nil, err
	}
	if _, err := pgx.ForEachRow(linhas, []any{&cod, &desc}, func() error {
		c.naturezas = append(c.naturezas, ItemNatureza{Codigo: cod, Descricao: desc, PessoaFisica: cod == 2135 || (cod >= 4000 && cod <= 4999)})
		c.natDesc[cod] = desc
		return nil
	}); err != nil {
		return nil, err
	}

	linhas, err = s.pool.Query(ctx, fmt.Sprintf(`SELECT codigo, nome, nome_busca, uf, empresas FROM %s.municipio ORDER BY empresas DESC, nome`, e))
	if err != nil {
		return nil, err
	}
	var m municipio
	if _, err := pgx.ForEachRow(linhas, []any{&m.Codigo, &m.Nome, &m.busca, &m.UF, &m.Empresas}, func() error {
		c.municipios = append(c.municipios, m)
		return nil
	}); err != nil {
		return nil, err
	}
	for i := range c.municipios {
		c.munPorCod[c.municipios[i].Codigo] = &c.municipios[i]
	}

	linhas, err = s.pool.Query(ctx, fmt.Sprintf(`SELECT uf, id_min, id_max, n FROM %s.faixa_uf ORDER BY uf`, e))
	if err != nil {
		return nil, err
	}
	var uf string
	var f faixa
	var n int
	if _, err := pgx.ForEachRow(linhas, []any{&uf, &f.min, &f.max, &n}, func() error {
		c.faixaUF[uf] = f
		nome := nomesUF[uf]
		if nome == "" {
			nome = uf
		}
		c.ufs = append(c.ufs, ItemUF{UF: uf, Nome: nome, Empresas: n})
		return nil
	}); err != nil {
		return nil, err
	}

	linhas, err = s.pool.Query(ctx, fmt.Sprintf(`SELECT municipio, min(id_min), max(id_max) FROM %s.faixa_mun WHERE municipio IS NOT NULL GROUP BY municipio`, e))
	if err != nil {
		return nil, err
	}
	if _, err := pgx.ForEachRow(linhas, []any{&cod, &f.min, &f.max}, func() error {
		c.faixaMun[cod] = f
		return nil
	}); err != nil {
		return nil, err
	}
	return c, nil
}

// Situacao devolve o resumo da base em uso; base que ainda não existe volta
// com pronta = false e sem erro.
func (s *Servico) Situacao(ctx context.Context) (Base, error) {
	c, err := s.catalogoAtual(ctx)
	if errors.Is(err, ErrNaoPronta) {
		return Base{Amostra: s.amostra, UFs: []string{}}, nil
	}
	if err != nil {
		return Base{}, err
	}
	b := c.base
	b.Amostra = s.amostra || b.Amostra
	// espaço do esquema de serviço, das tabelas de carga e da dívida ativa
	_ = s.pool.QueryRow(ctx, `SELECT coalesce(sum(pg_total_relation_size(c.oid)), 0)::bigint FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1) AND c.relkind = 'r'`, []string{s.esquema, "carga", "pgfn"}).Scan(&b.EspacoBytes)
	return b, nil
}

func limite(pedido, padrao, maximo int) int {
	if pedido <= 0 {
		return padrao
	}
	return min(pedido, maximo)
}

func listaDeCodigos(v string) map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		if d := texto.SoDigitos(p); d != "" && len(m) < 200 {
			m[d] = true
		}
	}
	return m
}

// CatalogoCnaes busca por texto sem acento ou por prefixo de código, em
// qualquer nível (divisão, grupo, classe, subclasse). codigos casa código
// exato em qualquer nível (para rotular um segmento salvo).
func (s *Servico) CatalogoCnaes(ctx context.Context, q, nivel, codigos string, lim int) ([]ItemCnae, error) {
	c, err := s.catalogoAtual(ctx)
	if err != nil {
		return nil, err
	}
	itens := []ItemCnae{}
	if strings.TrimSpace(codigos) != "" {
		quais := listaDeCodigos(codigos)
		for _, n := range c.cnaes {
			if quais[n.codigo] && (nivel == "" || nivel == n.nivel) {
				itens = append(itens, ItemCnae{Codigo: n.codigo, Descricao: n.descricao, Nivel: n.nivel})
			}
		}
		return itens, nil
	}
	lim = limite(lim, 20, 50)
	termo := texto.SemAcento(q)
	prefixo := ""
	if d := texto.SoDigitos(q); d != "" && strings.Trim(q, "0123456789.-/ ") == "" {
		prefixo, termo = d, ""
	}
	ordem := map[string]int{"divisao": 0, "grupo": 1, "classe": 2, "subclasse": 3}
	var achados []nivelCnae
	for _, n := range c.cnaes {
		if nivel != "" && nivel != n.nivel {
			continue
		}
		if prefixo != "" && !strings.HasPrefix(n.codigo, prefixo) {
			continue
		}
		if termo != "" && !strings.Contains(n.busca, termo) {
			continue
		}
		achados = append(achados, n)
	}
	// do mais geral para o mais específico; dentro do nível, por código
	sort.SliceStable(achados, func(i, j int) bool {
		if ordem[achados[i].nivel] != ordem[achados[j].nivel] {
			return ordem[achados[i].nivel] < ordem[achados[j].nivel]
		}
		return achados[i].codigo < achados[j].codigo
	})
	for _, n := range achados[:min(len(achados), lim)] {
		itens = append(itens, ItemCnae{Codigo: n.codigo, Descricao: n.descricao, Nivel: n.nivel})
	}
	return itens, nil
}

// CatalogoMunicipios lista municípios com empresas ativas, do maior para o
// menor.
func (s *Servico) CatalogoMunicipios(ctx context.Context, uf, q, codigos string, lim int) ([]ItemMunicipio, error) {
	c, err := s.catalogoAtual(ctx)
	if err != nil {
		return nil, err
	}
	itens := []ItemMunicipio{}
	if strings.TrimSpace(codigos) != "" {
		for cod := range listaDeCodigos(codigos) {
			n, _ := strconv.Atoi(cod)
			if m := c.munPorCod[n]; m != nil {
				itens = append(itens, m.ItemMunicipio)
			}
		}
		sort.Slice(itens, func(i, j int) bool { return itens[i].Empresas > itens[j].Empresas })
		return itens, nil
	}
	lim = limite(lim, 20, 50)
	uf = strings.ToUpper(strings.TrimSpace(uf))
	termo := texto.SemAcento(q)
	for _, m := range c.municipios {
		if uf != "" && m.UF != uf {
			continue
		}
		if termo != "" && !strings.Contains(m.busca, termo) {
			continue
		}
		itens = append(itens, m.ItemMunicipio)
		if len(itens) == lim {
			break
		}
	}
	return itens, nil
}

// CatalogoNaturezas lista as naturezas jurídicas.
func (s *Servico) CatalogoNaturezas(ctx context.Context) ([]ItemNatureza, error) {
	c, err := s.catalogoAtual(ctx)
	if err != nil {
		return nil, err
	}
	return append([]ItemNatureza{}, c.naturezas...), nil
}

// CatalogoUFs lista as UFs com empresas ativas na base.
func (s *Servico) CatalogoUFs(ctx context.Context) ([]ItemUF, error) {
	c, err := s.catalogoAtual(ctx)
	if err != nil {
		return nil, err
	}
	return append([]ItemUF{}, c.ufs...), nil
}
