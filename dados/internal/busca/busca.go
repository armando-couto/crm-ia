package busca

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

func (s *Servico) sql(c string) string { return strings.ReplaceAll(c, "{E}", s.esquema) }

// ---------------------------------------------------------------- contagem

type contagem struct {
	total    int64
	exato    bool
	limite   *int64 // limite superior dado pelos cubos, quando há
	parciais [3]int64
}

func (c contagem) pequeno() bool {
	return (c.exato && c.total <= ConjuntoPequeno) || (!c.exato && c.limite != nil && *c.limite <= ConjuntoPequeno)
}

func (c contagem) resposta() Contagem {
	if !c.exato {
		// "mais de 10.000": parciais nulas
		return Contagem{Total: TetoContagem, TotalModo: "teto", TotalLimiteSuperior: c.limite}
	}
	p := c.parciais
	return Contagem{Total: c.total, TotalModo: "exato", Parciais: Parciais{ComCelular: &p[0], ComEmail: &p[1], ComDivida: &p[2]}}
}

// somaCubo soma n num cubo, com os recortes de celular, e-mail e dívida.
func (s *Servico) somaCubo(ctx context.Context, tabela string, alvo int, p *plano) (contagem, error) {
	b := &sqlb{}
	c := contagem{exato: true}
	consulta := s.sql(`SELECT coalesce(sum(n), 0)::bigint, coalesce(sum(n) FILTER (WHERE tem_celular), 0)::bigint,
       coalesce(sum(n) FILTER (WHERE tem_email), 0)::bigint, coalesce(sum(n) FILTER (WHERE tem_divida), 0)::bigint
FROM {E}.` + tabela + ` b WHERE ` + p.onde(b, alvo))
	err := s.pool.QueryRow(ctx, consulta, b.args...).Scan(&c.total, &c.parciais[0], &c.parciais[1], &c.parciais[2])
	return c, err
}

// contar aplica as três formas de contagem:
//
//  1. só colunas dos cubos: exata pelo cubo (município e UF; UF e CNAE
//     principal; um código de CNAE em qualquer posição);
//  2. fora dos cubos, com limite superior de até 1.000.000 dado pelos cubos:
//     count(*) exato na tabela de busca (índice de cobertura ou GIN);
//  3. acima disso: contagem com teto de 10.001 linhas.
//
// Consulta que estoura o statement_timeout vira teto. A estimativa do
// planejador nunca é usada (erra de −94% a +120%).
func (s *Servico) contar(ctx context.Context, p *plano) (contagem, error) {
	if p.vazio {
		return contagem{exato: true}, nil
	}
	var limite *int64
	menor := func(c contagem) {
		if limite == nil || c.total < *limite {
			v := c.total
			limite = &v
		}
	}
	semExtras := len(p.extras) == 0
	switch {
	case len(p.cnpjs) > 0:
		v := int64(len(p.cnpjs))
		limite = &v
	case !p.filtraCnae:
		c, err := s.somaCubo(ctx, "cubo_mun", alvoCuboMun, p)
		if err != nil || semExtras {
			return c, err
		}
		menor(c)
	case !p.qualquerPosicao:
		c, err := s.somaCubo(ctx, "cubo_uf_cnae", alvoCuboUFCnae, p)
		if err != nil || (semExtras && len(p.municipios) == 0) {
			return c, err
		}
		menor(c)
	default:
		// a soma por código conta o mesmo estabelecimento uma vez por CNAE:
		// com mais de um código é só limite superior
		c, err := s.somaCubo(ctx, "cubo_cnae_qq", alvoCuboQQ, p)
		if err != nil || (semExtras && len(p.municipios) == 0 && len(p.cnaes) == 1) {
			return c, err
		}
		menor(c)
	}
	if p.filtraCnae && len(p.municipios) > 0 {
		c, err := s.somaCubo(ctx, "cubo_mun", alvoCuboMun, p)
		if err != nil {
			return c, err
		}
		menor(c)
	}
	if limite != nil && *limite == 0 {
		return contagem{exato: true}, nil
	}

	b := &sqlb{}
	onde := p.onde(b, alvoBusca)
	teto := ""
	if limite == nil || *limite > LimiteContagemExata {
		teto = fmt.Sprintf(" LIMIT %d", TetoContagem+1)
	}
	consulta := s.sql(`SELECT count(*), count(*) FILTER (WHERE tem_celular), count(*) FILTER (WHERE tem_email), count(*) FILTER (WHERE tem_divida)
FROM (SELECT b.tem_celular, b.tem_email, b.tem_divida FROM {E}.busca b WHERE ` + onde + teto + `) t`)
	c := contagem{exato: true, limite: limite}
	err := s.pool.QueryRow(ctx, consulta, b.args...).Scan(&c.total, &c.parciais[0], &c.parciais[1], &c.parciais[2])
	if estourouTempo(err) {
		return contagem{limite: limite}, nil
	}
	if err != nil {
		return c, err
	}
	if teto != "" && c.total > TetoContagem {
		return contagem{limite: limite}, nil
	}
	return c, nil
}

// Contar responde POST /v1/empresas/contagem.
func (s *Servico) Contar(ctx context.Context, f Filtros) (Contagem, error) {
	cat, err := s.catalogoAtual(ctx)
	if err != nil {
		return Contagem{}, err
	}
	p, err := cat.planejar(f)
	if err != nil {
		return Contagem{}, err
	}
	c, err := s.contar(ctx, p)
	if err != nil {
		if semBase(err) {
			return Contagem{}, ErrNaoPronta
		}
		return Contagem{}, err
	}
	return c.resposta(), nil
}

// ---------------------------------------------------------------- paginação

// cursor é o estado da paginação por chave. Vai para o cliente opaco
// (base64 de JSON) e carrega a versão da base: como o id é recalculado a
// cada reconstrução, cursor de versão antiga é recusado.
type cursor struct {
	Versao  string `json:"v"`
	Ordem   string `json:"o"`
	Chave   int64  `json:"k"`
	ID      int32  `json:"i"`
	Semente int64  `json:"s,omitempty"`
	Filtro  string `json:"h"`
}

func (c cursor) codificar() string {
	j, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(j)
}

func decodificarCursor(v string) (cursor, error) {
	var c cursor
	j, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(v))
	if err != nil || json.Unmarshal(j, &c) != nil || c.Versao == "" {
		return c, invalido("cursor inválido")
	}
	return c, nil
}

// impressao identifica a busca (filtros e ordem pedida) dentro do cursor.
func impressao(f Filtros, ordem string) string {
	j, _ := json.Marshal(f)
	h := sha256.Sum256(append(j, ordem...))
	return hex.EncodeToString(h[:6])
}

// chaveDeOrdem é a expressão (bigint) ordenada de forma decrescente, com o
// id como desempate. Nulos vão para o fim.
func chaveDeOrdem(ordem string, b *sqlb, semente int64) (string, bool) {
	switch ordem {
	case OrdemRelevancia:
		return "b.qualidade::bigint", true
	case OrdemRecentes:
		return "coalesce(b.data_inicio - DATE '1800-01-01', -1)::bigint", true
	case OrdemAntigas:
		return "coalesce(DATE '9999-12-31' - b.data_inicio, -1)::bigint", true
	case OrdemCapital:
		return "coalesce(b.capital, -1)", true
	case OrdemDivida:
		return "coalesce(b.divida, -1)", true
	case OrdemAleatoria:
		return "hashint8(b.id::bigint # " + b.p(semente) + "::bigint)::bigint", true
	}
	return "", false
}

func ordemValida(o string) (string, error) {
	switch o {
	case "":
		return OrdemRelevancia, nil
	case OrdemRelevancia, OrdemRecentes, OrdemAntigas, OrdemCapital, OrdemDivida, OrdemAleatoria:
		return o, nil
	}
	return "", invalido(fmt.Sprintf("ordem inválida: %q", o))
}

type linhaPagina struct {
	cnpj  string
	id    int32
	chave int64
}

// pagina lê até n linhas depois do cursor. Em ordem "base" anda por id
// dentro da faixa da UF ou do município; nas demais, materializa o conjunto
// (pequeno por definição) e ordena pela chave.
func (s *Servico) pagina(ctx context.Context, p *plano, cur cursor, primeira bool, n int) ([]linhaPagina, error) {
	if p.vazio {
		return nil, nil
	}
	b := &sqlb{}
	onde := p.onde(b, alvoBusca)
	var consulta string
	if expr, ordenada := chaveDeOrdem(cur.Ordem, b, cur.Semente); ordenada {
		depois := ""
		if !primeira {
			k, i := b.p(cur.Chave), b.p(cur.ID)
			depois = " WHERE (t.k < " + k + " OR (t.k = " + k + " AND t.id > " + i + "))"
		}
		consulta = `SELECT f.cnpj, pg.id, pg.k FROM (
  SELECT t.id, t.k FROM (SELECT b.id, ` + expr + ` AS k FROM {E}.busca b WHERE ` + onde + `) t` + depois + `
  ORDER BY t.k DESC, t.id LIMIT ` + b.p(n) + `) pg
JOIN {E}.base_ativa f ON f.id = pg.id ORDER BY pg.k DESC, pg.id`
	} else {
		if !primeira {
			onde += " AND b.id > " + b.p(cur.ID)
		}
		if p.temFaixa {
			onde += " AND b.id BETWEEN " + b.p(p.faixa.min) + " AND " + b.p(p.faixa.max)
		}
		consulta = `SELECT f.cnpj, pg.id, 0::bigint FROM (
  SELECT b.id FROM {E}.busca b WHERE ` + onde + ` ORDER BY b.id LIMIT ` + b.p(n) + `) pg
JOIN {E}.base_ativa f ON f.id = pg.id ORDER BY pg.id`
	}
	linhas, err := s.pool.Query(ctx, s.sql(consulta), b.args...)
	if err != nil {
		return nil, traduzir(err)
	}
	var saida []linhaPagina
	var l linhaPagina
	_, err = pgx.ForEachRow(linhas, []any{&l.cnpj, &l.id, &l.chave}, func() error {
		saida = append(saida, l)
		return nil
	})
	return saida, traduzir(err)
}

func traduzir(err error) error {
	switch {
	case err == nil:
		return nil
	case estourouTempo(err):
		return ErrFiltroAmplo
	case semBase(err):
		return ErrNaoPronta
	}
	return err
}

// buscar resolve catálogo, plano, cursor e ordem aplicada, e lê uma página.
func (s *Servico) buscar(ctx context.Context, f Filtros, ordemPedida, cursorTexto string, n int) (cat *catalogo, linhas []linhaPagina, cur cursor, fim bool, err error) {
	if cat, err = s.catalogoAtual(ctx); err != nil {
		return
	}
	ordem, err := ordemValida(ordemPedida)
	if err != nil {
		return
	}
	p, err := cat.planejar(f)
	if err != nil {
		return
	}
	marca := impressao(f, ordem)
	primeira := strings.TrimSpace(cursorTexto) == ""
	if primeira {
		// só em conjunto pequeno (total exato ou limite superior de até
		// 200.000) a ordem pedida é aplicada; fora disso vale a ordem da base
		var c contagem
		if c, err = s.contar(ctx, p); err != nil {
			err = traduzir(err)
			return
		}
		cur = cursor{Versao: cat.base.Versao, Ordem: OrdemBase, Filtro: marca}
		if c.pequeno() {
			cur.Ordem = ordem
			if ordem == OrdemAleatoria {
				var semente [8]byte
				_, _ = rand.Read(semente[:])
				cur.Semente = int64(binary.LittleEndian.Uint64(semente[:]) >> 1)
			}
		}
	} else {
		if cur, err = decodificarCursor(cursorTexto); err != nil {
			return
		}
		if cur.Versao != cat.base.Versao {
			err = ErrCursorAntigo
			return
		}
		if cur.Filtro != marca {
			err = invalido("o cursor não pertence a esta busca (filtros ou ordem mudaram)")
			return
		}
	}
	if linhas, err = s.pagina(ctx, p, cur, primeira, n+1); err != nil {
		return
	}
	fim = len(linhas) <= n
	if !fim {
		linhas = linhas[:n]
	}
	if len(linhas) > 0 {
		ultimo := linhas[len(linhas)-1]
		cur.Chave, cur.ID = ultimo.chave, ultimo.id
	}
	// a base pode ter sido trocada entre as consultas desta chamada
	if v, errV := s.versao(ctx); errV != nil || v != cat.base.Versao {
		err = ErrCursorAntigo
	}
	return
}

// Candidatos responde POST /v1/empresas/candidatos: só CNPJs, paginados por
// chave (nunca OFFSET).
func (s *Servico) Candidatos(ctx context.Context, f Filtros, ordem, cursorTexto string, lim int) (Candidatos, error) {
	lim = limite(lim, 1000, MaximoCandidatos)
	cat, linhas, cur, fim, err := s.buscar(ctx, f, ordem, cursorTexto, lim)
	if err != nil {
		return Candidatos{}, err
	}
	r := Candidatos{CNPJs: make([]string, 0, len(linhas)), Fim: fim, Versao: cat.base.Versao, OrdemAplicada: cur.Ordem}
	for _, l := range linhas {
		r.CNPJs = append(r.CNPJs, l.cnpj)
	}
	if !fim {
		r.Cursor = cur.codificar()
	}
	return r, nil
}

// Previa responde POST /v1/empresas/previa: as primeiras empresas do filtro,
// sem contato e sem nomes de sócios.
func (s *Servico) Previa(ctx context.Context, f Filtros, ordem string, lim int) (Previa, error) {
	lim = limite(lim, MaximoPrevia, MaximoPrevia)
	cat, linhas, cur, _, err := s.buscar(ctx, f, ordem, "", lim)
	if err != nil {
		return Previa{}, err
	}
	r := Previa{Itens: make([]EmpresaPrevia, 0, len(linhas)), Versao: cat.base.Versao, OrdemAplicada: cur.Ordem}
	if len(linhas) == 0 {
		return r, nil
	}
	ids := make([]int32, len(linhas))
	posicao := map[string]int{}
	for i, l := range linhas {
		ids[i] = l.id
		posicao[l.cnpj] = i
	}
	res, err := s.pool.Query(ctx, s.sql(`SELECT f.cnpj, f.razao_social, f.nome_fantasia, coalesce(f.cnae, 0), f.porte, to_char(f.data_inicio, 'YYYY-MM-DD'),
       f.capital::float8, coalesce(f.municipio, 0), f.uf, f.pf, f.mei, (f.tipo1 = 'celular' OR f.tipo2 = 'celular') IS TRUE,
       f.fone1 IS NOT NULL, f.email IS NOT NULL, EXISTS (SELECT 1 FROM {E}.divida d WHERE d.cnpj = f.cnpj), f.socios_qtd, f.qualidade
FROM {E}.base_ativa f WHERE f.id = ANY($1)`), ids)
	if err != nil {
		return Previa{}, traduzir(err)
	}
	r.Itens = make([]EmpresaPrevia, len(linhas))
	var e EmpresaPrevia
	var cnae, porte, mun int
	_, err = pgx.ForEachRow(res, []any{&e.CNPJ, &e.RazaoSocial, &e.NomeFantasia, &cnae, &porte, &e.Abertura, &e.Capital, &mun, &e.UF,
		&e.PessoaFisica, &e.MEI, &e.TemCelular, &e.TemTelefone, &e.TemEmail, &e.TemDivida, &e.SociosQtd, &e.Qualidade}, func() error {
		e.Cnae = Codigo{Codigo: cnae, Descricao: cat.cnaeDesc[cnae]}
		e.Porte = NomePorte(porte)
		e.Municipio = ""
		if m := cat.munPorCod[mun]; m != nil {
			e.Municipio = m.Nome
		}
		r.Itens[posicao[e.CNPJ]] = e
		return nil
	})
	return r, traduzir(err)
}

// ---------------------------------------------------------------- ficha

// Detalhar responde POST /v1/empresas/detalhes: a ficha inteira, com sócios
// e dívida. Sem incluirPF, a pessoa física (MEI, empresário individual) volta
// sem telefones, sem e-mail e sem sócios.
func (s *Servico) Detalhar(ctx context.Context, cnpjs []string, incluirPF bool) (Detalhes, error) {
	cat, err := s.catalogoAtual(ctx)
	if err != nil {
		return Detalhes{}, err
	}
	if len(cnpjs) > MaximoDetalhes {
		return Detalhes{}, invalido(fmt.Sprintf("cnpjs aceita até %d itens", MaximoDetalhes))
	}
	r := Detalhes{Itens: []Empresa{}, NaoEncontrados: []string{}}
	var pedidos []string
	normalizado := map[string]string{} // normalizado → como veio
	for _, v := range cnpjs {
		n := texto.NormalizarCNPJ(v)
		if !texto.CNPJFormaValida(n) {
			r.NaoEncontrados = append(r.NaoEncontrados, v)
			continue
		}
		if _, repetido := normalizado[n]; !repetido {
			normalizado[n] = v
			pedidos = append(pedidos, n)
		}
	}
	if len(pedidos) == 0 {
		return r, nil
	}
	linhas, err := s.pool.Query(ctx, s.sql(`SELECT f.cnpj, f.cnpj_basico, f.matriz, f.razao_social, f.nome_fantasia, coalesce(f.natureza, 0), f.pf, f.porte,
       f.capital::float8, to_char(f.data_inicio, 'YYYY-MM-DD'), to_char(f.data_situacao, 'YYYY-MM-DD'), f.simples, f.mei,
       coalesce(f.cnae, 0), f.cnaes, coalesce(f.tipo_logradouro, ''), coalesce(f.logradouro, ''), coalesce(f.numero, ''),
       coalesce(f.complemento, ''), coalesce(f.bairro, ''), coalesce(f.cep, ''), coalesce(f.municipio, 0), f.uf,
       coalesce(f.fone1, ''), coalesce(f.tipo1, ''), f.comp1, coalesce(f.fone2, ''), coalesce(f.tipo2, ''), f.comp2,
       coalesce(f.email, ''), coalesce(f.email_tipo, ''), f.qualidade,
       d.total::float8, coalesce(d.inscricoes, 0), coalesce(d.em_cobranca, 0)::float8, coalesce(d.regular, 0)::float8,
       coalesce(d.geral, false), coalesce(d.previdenciaria, false), coalesce(d.fgts, false), coalesce(d.ajuizada, false),
       to_char(d.mais_antiga, 'YYYY-MM-DD')
FROM {E}.base_ativa f LEFT JOIN {E}.divida d ON d.cnpj = f.cnpj
WHERE f.cnpj = ANY($1)`), pedidos)
	if err != nil {
		return Detalhes{}, traduzir(err)
	}
	type achado struct {
		e      Empresa
		basico string
	}
	achados := map[string]*achado{}
	var basicos []string
	var (
		e                                   Empresa
		basico, f1, t1, f2, t2              string
		c1, c2, geral, prev, fgts, ajuizada bool
		natureza, porte, cnae, mun, inscr   int
		cnaes                               []int32
		total                               *float64
		emCobranca, regular                 float64
		maisAntiga                          *string
	)
	_, err = pgx.ForEachRow(linhas, []any{&e.CNPJ, &basico, &e.Matriz, &e.RazaoSocial, &e.NomeFantasia, &natureza, &e.PessoaFisica, &porte,
		&e.Capital, &e.Abertura, &e.SituacaoDesde, &e.Simples, &e.MEI, &cnae, &cnaes,
		&e.Endereco.TipoLogradouro, &e.Endereco.Logradouro, &e.Endereco.Numero, &e.Endereco.Complemento, &e.Endereco.Bairro, &e.Endereco.CEP, &mun, &e.Endereco.UF,
		&f1, &t1, &c1, &f2, &t2, &c2, &e.Email, &e.EmailTipo, &e.Qualidade,
		&total, &inscr, &emCobranca, &regular, &geral, &prev, &fgts, &ajuizada, &maisAntiga}, func() error {
		item := e
		item.Natureza = Codigo{Codigo: natureza, Descricao: cat.natDesc[natureza]}
		item.Porte = NomePorte(porte)
		item.Situacao = "ATIVA"
		item.Cnae = Codigo{Codigo: cnae, Descricao: cat.cnaeDesc[cnae]}
		item.CnaesSecundarios = []Codigo{}
		for _, c := range cnaes {
			if int(c) != cnae {
				item.CnaesSecundarios = append(item.CnaesSecundarios, Codigo{Codigo: int(c), Descricao: cat.cnaeDesc[int(c)]})
			}
		}
		item.Endereco.MunicipioCodigo = mun
		item.Endereco.Municipio = ""
		if m := cat.munPorCod[mun]; m != nil {
			item.Endereco.Municipio = m.Nome
		}
		item.Telefones = []Telefone{}
		item.Socios = []Socio{}
		item.Referencia = cat.base.ReferenciaCNPJ
		item.Divida = nil
		if total != nil {
			d := &Divida{Total: *total, Inscricoes: inscr, EmCobranca: emCobranca, Regular: regular, Tipos: []string{}, Ajuizada: ajuizada, MaisAntiga: maisAntiga, Referencia: cat.base.ReferenciaPGFN}
			for _, t := range []struct {
				tem  bool
				nome string
			}{{geral, "geral"}, {prev, "previdenciaria"}, {fgts, "fgts"}} {
				if t.tem {
					d.Tipos = append(d.Tipos, t.nome)
				}
			}
			item.Divida = d
		}
		if item.PessoaFisica && !incluirPF {
			// contato de pessoa física só sai para lista criada com incluir_pf
			item.Email, item.EmailTipo = "", ""
		} else {
			if f1 != "" {
				item.Telefones = append(item.Telefones, Telefone{Numero: f1, Tipo: t1, Compartilhado: c1, Origem: "receita", Principal: true})
			}
			if f2 != "" {
				item.Telefones = append(item.Telefones, Telefone{Numero: f2, Tipo: t2, Compartilhado: c2, Origem: "receita"})
			}
			basicos = append(basicos, basico)
		}
		achados[item.CNPJ] = &achado{e: item, basico: basico}
		return nil
	})
	if err != nil {
		return Detalhes{}, traduzir(err)
	}

	if len(basicos) > 0 {
		socios := map[string][]Socio{}
		res, err := s.pool.Query(ctx, s.sql(`SELECT cnpj_basico, nome, qualificacao, to_char(desde, 'YYYY-MM-DD'), tipo, documento, representante
FROM {E}.socio WHERE cnpj_basico = ANY($1) ORDER BY cnpj_basico, nome`), basicos)
		if err != nil {
			return Detalhes{}, traduzir(err)
		}
		var so Socio
		var b string
		if _, err := pgx.ForEachRow(res, []any{&b, &so.Nome, &so.Qualificacao, &so.Desde, &so.Tipo, &so.Documento, &so.Representante}, func() error {
			so.Documento = texto.MascaraCPF(so.Documento) // CPF completo nunca sai em rota nenhuma
			socios[b] = append(socios[b], so)
			return nil
		}); err != nil {
			return Detalhes{}, traduzir(err)
		}
		for _, a := range achados {
			if lista := socios[a.basico]; lista != nil && !(a.e.PessoaFisica && !incluirPF) {
				a.e.Socios = lista
			}
		}
	}
	for _, n := range pedidos {
		if a := achados[n]; a != nil {
			r.Itens = append(r.Itens, a.e)
		} else {
			r.NaoEncontrados = append(r.NaoEncontrados, normalizado[n])
		}
	}
	return r, nil
}

// Empresa responde GET /v1/empresas/{cnpj}.
func (s *Servico) Empresa(ctx context.Context, cnpj string, incluirPF bool) (Empresa, error) {
	d, err := s.Detalhar(ctx, []string{cnpj}, incluirPF)
	if err != nil {
		return Empresa{}, err
	}
	if len(d.Itens) == 0 {
		return Empresa{}, ErrNaoEncontrado
	}
	return d.Itens[0], nil
}
