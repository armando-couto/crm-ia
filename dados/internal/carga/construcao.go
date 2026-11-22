package carga

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Esquemas fixos. O de serviço é construído ao lado do que está em uso
// (sufixo _novo) e trocado numa transação; o anterior vira _velho e é
// apagado.
const (
	EsquemaServico = "prospec"
	EsquemaAmostra = "prospec_amostra"
	EsquemaCarga   = "carga"
	EsquemaPGFN    = "pgfn"
)

// CriarTabelasDeCarga cria (se faltarem) as tabelas de carga do esquema:
// UNLOGGED, tudo text, sem chave. A coluna arq diz de que arquivo veio cada
// linha, para conferir e refazer um arquivo na retomada.
func CriarTabelasDeCarga(ctx context.Context, db Executor, esquema string) error {
	comandos := []string{fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, esquema)}
	for _, l := range leiautes {
		comandos = append(comandos, fmt.Sprintf(`CREATE UNLOGGED TABLE IF NOT EXISTS %s.%s (arq text, %s text)`, esquema, l.Tabela, strings.Join(l.Colunas, " text, ")))
	}
	comandos = append(comandos, fmt.Sprintf(`CREATE UNLOGGED TABLE IF NOT EXISTS %s.pgfn (arq text, %s text)`, esquema, strings.Join(colunasPGFN, " text, ")))
	return executar(ctx, db, comandos...)
}

func executar(ctx context.Context, db Executor, comandos ...string) error {
	for _, c := range comandos {
		if _, err := db.Exec(ctx, c); err != nil {
			return fmt.Errorf("%w\n-- comando: %.300s", err, strings.TrimSpace(c))
		}
	}
	return nil
}

// AgregarDivida monta a tabela de dívida por CNPJ (14 caracteres) a partir
// da tabela de carga da PGFN. Só há linhas de devedor principal; a mesma
// inscrição repetida entre CSVs de regiões diferentes conta uma vez.
func AgregarDivida(ctx context.Context, db Executor, esquemaCarga, tabelaDestino string) error {
	return executar(ctx, db,
		fmt.Sprintf(`DROP TABLE IF EXISTS %s`, tabelaDestino),
		fmt.Sprintf(`CREATE TABLE %s AS
SELECT cnpj,
       sum(valor)::numeric(18,2)                                        AS total,
       count(*)::int                                                    AS inscricoes,
       coalesce(sum(valor) FILTER (WHERE irregular), 0)::numeric(18,2)     AS em_cobranca,
       coalesce(sum(valor) FILTER (WHERE NOT irregular), 0)::numeric(18,2) AS regular,
       bool_or(sistema = 'geral')          AS geral,
       bool_or(sistema = 'previdenciaria') AS previdenciaria,
       bool_or(sistema = 'fgts')           AS fgts,
       bool_or(ajuizada)                   AS ajuizada,
       min(data)                           AS mais_antiga
FROM (SELECT DISTINCT ON (cnpj, sistema, inscricao)
             cnpj, sistema, valor::numeric AS valor, irregular = 't' AS irregular, ajuizada = 't' AS ajuizada, data::date AS data
      FROM %s.pgfn
      ORDER BY cnpj, sistema, inscricao) i
GROUP BY cnpj`, tabelaDestino, esquemaCarga),
		fmt.Sprintf(`ALTER TABLE %s ADD PRIMARY KEY (cnpj)`, tabelaDestino),
		fmt.Sprintf(`ANALYZE %s`, tabelaDestino))
}

// Construcao diz de onde e para onde construir o esquema de serviço.
type Construcao struct {
	// Destino é o esquema de serviço (prospec ou prospec_amostra).
	Destino string
	// Carga é o esquema das tabelas de carga. Vazio = reaproveitar a
	// base_ativa, os sócios e os catálogos do Destino em uso (quando só a
	// dívida ativa mudou).
	Carga string
	// Divida é a tabela (com esquema) da dívida por CNPJ; vazio = sem dívida.
	Divida string

	Versao         string
	ReferenciaCNPJ string
	ReferenciaPGFN string
	Amostra        bool
	UFs            []string
	Estatisticas   any
	// Etapa recebe o nome de cada passo e o avanço (0 a 100).
	Etapa func(nome string, percentual int)
}

// Sinais que entram como chave dos índices de cobertura e dos cubos, nesta
// ordem. São as colunas que a contagem exata sabe responder sem ler a tabela.
const sinais = "pf, porte, mei, simples, tem_celular, tem_fone, tem_divida, tem_email, matriz"

// Construir monta o esquema Destino_novo ao lado do que está em uso e faz
// VACUUM (ANALYZE) em tudo. Não troca: isso é com Trocar.
//
// A linha é gravada em ordem (uf, municipio, cnae, cnpj) e o id é a posição
// nessa ordem: cada UF e cada município ocupam uma faixa contígua de id, o
// que deixa a paginação por chave barata.
func Construir(ctx context.Context, db Executor, c Construcao) error {
	n := c.Destino + "_novo"
	etapa := func(nome string, p int) {
		slog.Info("construção", "esquema", n, "etapa", nome)
		if c.Etapa != nil {
			c.Etapa(nome, p)
		}
	}
	if err := executar(ctx, db,
		`SET maintenance_work_mem = '512MB'`, `SET work_mem = '96MB'`, `SET statement_timeout = 0`,
		`SET synchronize_seqscans = off`, `SET jit = off`,
		fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, n),
		fmt.Sprintf(`CREATE SCHEMA %s`, n)); err != nil {
		return err
	}

	if c.Carga != "" {
		etapa("contatos compartilhados", 2)
		if err := executar(ctx, db,
			// telefone presente em 5 ou mais raízes de CNPJ ativas: provável contador
			fmt.Sprintf(`CREATE UNLOGGED TABLE %[1]s.t_fone AS
SELECT fone FROM (SELECT fone1 AS fone, cnpj_basico FROM %[2]s.estabelecimentos WHERE fone1 IS NOT NULL
                  UNION ALL SELECT fone2, cnpj_basico FROM %[2]s.estabelecimentos WHERE fone2 IS NOT NULL) f
GROUP BY fone HAVING count(DISTINCT cnpj_basico) >= 5`, n, c.Carga),
			fmt.Sprintf(`ALTER TABLE %s.t_fone ADD PRIMARY KEY (fone)`, n),
			fmt.Sprintf(`CREATE UNLOGGED TABLE %[1]s.t_email AS
SELECT email FROM %[2]s.estabelecimentos WHERE email IS NOT NULL GROUP BY email HAVING count(DISTINCT cnpj_basico) >= 5`, n, c.Carga),
			fmt.Sprintf(`ALTER TABLE %s.t_email ADD PRIMARY KEY (email)`, n),
			fmt.Sprintf(`CREATE UNLOGGED TABLE %[1]s.t_socios AS SELECT cnpj_basico, count(*)::int AS n FROM %[2]s.socios GROUP BY 1`, n, c.Carga),
			fmt.Sprintf(`ALTER TABLE %s.t_socios ADD PRIMARY KEY (cnpj_basico)`, n),
			fmt.Sprintf(`ANALYZE %s.t_fone`, n), fmt.Sprintf(`ANALYZE %s.t_email`, n), fmt.Sprintf(`ANALYZE %s.t_socios`, n),
		); err != nil {
			return err
		}

		etapa("base de estabelecimentos ativos", 10)
		if err := executar(ctx, db, fmt.Sprintf(sqlBaseAtiva, n, c.Carga)); err != nil {
			return err
		}

		etapa("sócios e catálogos", 40)
		if err := executar(ctx, db,
			fmt.Sprintf(`CREATE TABLE %[1]s.socio AS
SELECT s.cnpj_basico, coalesce(s.nome, '') AS nome, coalesce(q.descricao, '') AS qualificacao, s.desde::date AS desde,
       s.tipo, coalesce(s.documento, '') AS documento, coalesce(s.representante, '') AS representante
FROM %[2]s.socios s
LEFT JOIN %[2]s.qualificacoes q ON q.codigo = s.qualificacao
WHERE s.cnpj_basico IN (SELECT cnpj_basico FROM %[1]s.base_ativa)
ORDER BY s.cnpj_basico, s.nome`, n, c.Carga),
			fmt.Sprintf(`CREATE TABLE %[1]s.cnae AS SELECT DISTINCT ON (codigo) codigo::int AS codigo, descricao, descricao_busca FROM %[2]s.cnaes ORDER BY codigo`, n, c.Carga),
			fmt.Sprintf(`CREATE TABLE %[1]s.natureza AS SELECT DISTINCT ON (codigo) codigo::smallint AS codigo, descricao FROM %[2]s.naturezas ORDER BY codigo`, n, c.Carga),
			fmt.Sprintf(`CREATE TABLE %[1]s.municipio AS
SELECT m.codigo, m.nome, m.nome_busca, b.uf, b.empresas
FROM (SELECT DISTINCT ON (codigo) codigo::smallint AS codigo, nome, nome_busca FROM %[2]s.municipios ORDER BY codigo) m
JOIN (SELECT municipio, min(uf) AS uf, count(*)::int AS empresas FROM %[1]s.base_ativa WHERE municipio IS NOT NULL GROUP BY 1) b ON b.municipio = m.codigo`, n, c.Carga),
			fmt.Sprintf(`DROP TABLE %[1]s.t_fone, %[1]s.t_email, %[1]s.t_socios`, n),
		); err != nil {
			return err
		}
	} else {
		etapa("copiando a base de estabelecimentos ativos", 10)
		a := c.Destino
		if err := executar(ctx, db,
			fmt.Sprintf(`CREATE TABLE %s.base_ativa AS SELECT * FROM %s.base_ativa ORDER BY id`, n, a),
			fmt.Sprintf(`CREATE TABLE %s.socio AS SELECT * FROM %s.socio ORDER BY cnpj_basico, nome`, n, a),
			fmt.Sprintf(`CREATE TABLE %s.cnae AS SELECT * FROM %s.cnae`, n, a),
			fmt.Sprintf(`CREATE TABLE %s.natureza AS SELECT * FROM %s.natureza`, n, a),
			fmt.Sprintf(`CREATE TABLE %s.municipio AS SELECT * FROM %s.municipio`, n, a),
		); err != nil {
			return err
		}
	}

	etapa("dívida ativa", 50)
	divida := `SELECT NULL::text AS cnpj, NULL::numeric(18,2) AS total, NULL::int AS inscricoes, NULL::numeric(18,2) AS em_cobranca, NULL::numeric(18,2) AS regular,
       NULL::boolean AS geral, NULL::boolean AS previdenciaria, NULL::boolean AS fgts, NULL::boolean AS ajuizada, NULL::date AS mais_antiga WHERE false`
	if c.Divida != "" {
		divida = fmt.Sprintf(`SELECT d.cnpj, d.total, d.inscricoes, d.em_cobranca, d.regular, d.geral, d.previdenciaria, d.fgts, d.ajuizada, d.mais_antiga
FROM %s d JOIN %s.base_ativa b ON b.cnpj = d.cnpj ORDER BY b.id`, c.Divida, n)
	}
	if err := executar(ctx, db,
		fmt.Sprintf(`ALTER TABLE %s.base_ativa ADD PRIMARY KEY (id) WITH (fillfactor = 100)`, n),
		fmt.Sprintf(`CREATE UNIQUE INDEX base_ativa_cnpj ON %s.base_ativa (cnpj) WITH (fillfactor = 100)`, n),
		fmt.Sprintf(`CREATE TABLE %s.divida AS %s`, n, divida),
		fmt.Sprintf(`ALTER TABLE %s.divida ADD PRIMARY KEY (cnpj)`, n),
	); err != nil {
		return err
	}

	etapa("tabela de busca", 58)
	if err := executar(ctx, db,
		// só colunas de filtro; mesma ordem física da base (id)
		fmt.Sprintf(`CREATE TABLE %[1]s.busca AS
SELECT b.id, b.cnae, b.data_inicio, b.municipio, b.natureza, b.porte, b.matriz, b.simples, b.mei, b.pf,
       (b.tipo1 = 'celular' OR b.tipo2 = 'celular') IS TRUE AS tem_celular,
       (b.fone1 IS NOT NULL) AS tem_fone, (b.email IS NOT NULL) AS tem_email,
       (d.cnpj IS NOT NULL) AS tem_divida, b.contador, b.qualidade,
       (b.capital * 100)::bigint AS capital, (d.total * 100)::bigint AS divida, b.uf, b.cnaes
FROM %[1]s.base_ativa b LEFT JOIN %[1]s.divida d ON d.cnpj = b.cnpj
ORDER BY b.id`, n),
	); err != nil {
		return err
	}

	etapa("índices", 68)
	if err := executar(ctx, db,
		fmt.Sprintf(`ALTER TABLE %s.busca ADD PRIMARY KEY (id) WITH (fillfactor = 100)`, n),
		// os sinais são chave do índice (e não INCLUDE): com deduplicação o
		// índice fica 4 vezes menor e a contagem não visita a tabela
		fmt.Sprintf(`CREATE INDEX busca_c1 ON %s.busca (uf, municipio, cnae, %s) WITH (fillfactor = 100)`, n, sinais),
		fmt.Sprintf(`CREATE INDEX busca_c2 ON %s.busca (cnae, uf, municipio, %s) WITH (fillfactor = 100)`, n, sinais),
		fmt.Sprintf(`CREATE INDEX busca_cnaes_gin ON %s.busca USING gin (cnaes)`, n),
		fmt.Sprintf(`CREATE INDEX socio_empresa ON %s.socio (cnpj_basico) WITH (fillfactor = 100)`, n),
	); err != nil {
		return err
	}

	etapa("cubos de contagem e faixas de id", 82)
	if err := executar(ctx, db,
		fmt.Sprintf(`CREATE TABLE %[1]s.cubo_mun AS SELECT uf, municipio, %[2]s, count(*)::int AS n FROM %[1]s.busca GROUP BY uf, municipio, %[2]s`, n, sinais),
		fmt.Sprintf(`CREATE INDEX ON %s.cubo_mun (uf, municipio) WITH (fillfactor = 100)`, n),
		fmt.Sprintf(`CREATE TABLE %[1]s.cubo_uf_cnae AS SELECT uf, cnae, %[2]s, count(*)::int AS n FROM %[1]s.busca GROUP BY uf, cnae, %[2]s`, n, sinais),
		fmt.Sprintf(`CREATE INDEX ON %s.cubo_uf_cnae (cnae, uf) WITH (fillfactor = 100)`, n),
		// CNAE em qualquer posição: o mesmo estabelecimento conta uma vez por código
		fmt.Sprintf(`CREATE TABLE %[1]s.cubo_cnae_qq AS SELECT c AS cnae, uf, %[2]s, count(*)::int AS n FROM %[1]s.busca, unnest(cnaes) AS c GROUP BY c, uf, %[2]s`, n, sinais),
		fmt.Sprintf(`CREATE INDEX ON %s.cubo_cnae_qq (cnae, uf) WITH (fillfactor = 100)`, n),
		fmt.Sprintf(`CREATE TABLE %[1]s.faixa_mun AS SELECT uf, municipio, min(id) AS id_min, max(id) AS id_max, count(*)::int AS n FROM %[1]s.busca GROUP BY uf, municipio`, n),
		fmt.Sprintf(`CREATE TABLE %[1]s.faixa_uf AS SELECT uf, min(id_min) AS id_min, max(id_max) AS id_max, sum(n)::int AS n FROM %[1]s.faixa_mun GROUP BY uf`, n),
		fmt.Sprintf(`CREATE TABLE %s.meta (versao text NOT NULL, referencia_cnpj text NOT NULL, referencia_pgfn text NOT NULL,
  empresas bigint NOT NULL, socios bigint NOT NULL, devedores bigint NOT NULL, atualizada_em timestamptz NOT NULL,
  amostra boolean NOT NULL, ufs text[] NOT NULL, estatisticas jsonb NOT NULL)`, n),
	); err != nil {
		return err
	}
	ufs := c.UFs
	if ufs == nil {
		ufs = []string{}
	}
	est := c.Estatisticas
	if est == nil {
		est = map[string]any{}
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`INSERT INTO %[1]s.meta
SELECT $1, $2, $3, (SELECT count(*) FROM %[1]s.base_ativa), (SELECT count(*) FROM %[1]s.socio), (SELECT count(*) FROM %[1]s.divida), now(), $4, $5, $6`, n),
		c.Versao, c.ReferenciaCNPJ, c.ReferenciaPGFN, c.Amostra, ufs, est); err != nil {
		return err
	}

	// sem o VACUUM o Index Only Scan volta a visitar a tabela; por isso ele
	// vem antes da troca
	etapa("VACUUM (ANALYZE)", 90)
	tabelas, err := tabelasDe(ctx, db, n)
	if err != nil {
		return err
	}
	for _, t := range tabelas {
		if _, err := db.Exec(ctx, fmt.Sprintf(`VACUUM (ANALYZE) %s.%s`, n, t)); err != nil {
			return fmt.Errorf("VACUUM %s.%s: %w", n, t, err)
		}
	}
	return executar(ctx, db, `RESET maintenance_work_mem`, `RESET work_mem`, `RESET synchronize_seqscans`)
}

// sqlBaseAtiva: junção de estabelecimentos ativos, empresas e Simples, com
// contato compartilhado, tipo de e-mail, pessoa física e qualidade do
// contato. %[1]s = esquema novo, %[2]s = esquema de carga.
//
// qualidade (0 a 100): 60 com celular próprio; 35 com só fixo próprio (o
// indefinido conta como fixo); +20 e-mail que não é de contabilidade; +10
// segundo telefone; contato só compartilhado vale 15; sem contato, 0.
const sqlBaseAtiva = `CREATE TABLE %[1]s.base_ativa AS
WITH j AS (
  SELECT e.cnpj, e.cnpj_basico, (e.matriz = '1') AS matriz,
         coalesce(em.razao_social, '') AS razao_social, coalesce(e.nome_fantasia, '') AS nome_fantasia,
         em.natureza::smallint AS natureza, coalesce(em.porte::smallint, 0::smallint) AS porte, em.capital::numeric(18,2) AS capital,
         e.data_situacao::date AS data_situacao, e.data_inicio::date AS data_inicio,
         e.cnae::int AS cnae,
         CASE WHEN e.cnae IS NULL THEN '{}'::int[] ELSE ARRAY[e.cnae::int] END
           || coalesce(string_to_array(e.cnaes_sec, ',')::int[], '{}'::int[]) AS cnaes,
         coalesce(s.simples = 'S', false) AS simples, coalesce(s.mei = 'S', false) AS mei,
         e.tipo_logradouro, e.logradouro, e.numero, e.complemento, e.bairro, e.bairro_busca, e.cep,
         e.uf, e.municipio::smallint AS municipio,
         e.ddd1, e.tel1, e.ddd2, e.tel2,
         e.fone1, e.tipo1, (f1.fone IS NOT NULL) AS comp1,
         e.fone2, e.tipo2, (f2.fone IS NOT NULL) AS comp2,
         e.email,
         CASE WHEN e.email IS NULL THEN NULL WHEN me.email IS NOT NULL THEN 'contabilidade' ELSE e.email_tipo END AS email_tipo,
         coalesce(sq.n, 0)::smallint AS socios_qtd,
         nullif(btrim(coalesce(em.razao_busca, '') || ' ' || coalesce(e.fantasia_busca, '')), '') AS nome_busca
  FROM %[2]s.estabelecimentos e
  LEFT JOIN %[2]s.empresas em ON em.cnpj_basico = e.cnpj_basico
  LEFT JOIN %[2]s.simples s   ON s.cnpj_basico = e.cnpj_basico
  LEFT JOIN %[1]s.t_fone f1   ON f1.fone = e.fone1
  LEFT JOIN %[1]s.t_fone f2   ON f2.fone = e.fone2
  LEFT JOIN %[1]s.t_email me  ON me.email = e.email
  LEFT JOIN %[1]s.t_socios sq ON sq.cnpj_basico = e.cnpj_basico
), c AS (
  SELECT j.*,
         -- pessoa física: empresário individual, naturezas 4000 a 4999 ou MEI
         (coalesce(j.natureza = 2135 OR j.natureza BETWEEN 4000 AND 4999, false) OR j.mei) AS pf,
         ((j.fone1 IS NOT NULL AND NOT j.comp1 AND j.tipo1 = 'celular') OR (j.fone2 IS NOT NULL AND NOT j.comp2 AND j.tipo2 = 'celular')) AS celular_proprio,
         ((j.fone1 IS NOT NULL AND NOT j.comp1) OR (j.fone2 IS NOT NULL AND NOT j.comp2)) AS fone_proprio,
         (j.email IS NOT NULL AND j.email_tipo <> 'contabilidade') AS email_proprio,
         (j.fone1 IS NOT NULL OR j.email IS NOT NULL) AS tem_contato
  FROM j
)
SELECT (row_number() OVER (ORDER BY c.uf, c.municipio, c.cnae, c.cnpj))::int AS id,
       c.cnpj, c.cnpj_basico, c.matriz, c.razao_social, c.nome_fantasia, c.natureza, c.porte, c.capital,
       c.data_situacao, c.data_inicio, c.cnae, c.cnaes, c.simples, c.mei, c.pf,
       c.tipo_logradouro, c.logradouro, c.numero, c.complemento, c.bairro, c.bairro_busca, c.cep, c.uf, c.municipio,
       c.ddd1, c.tel1, c.ddd2, c.tel2, 1::smallint AS regra_telefone,
       c.fone1, c.tipo1, c.comp1, c.fone2, c.tipo2, c.comp2, c.email, c.email_tipo,
       -- contador: tem contato, mas nenhum é próprio (só compartilhado ou de contabilidade)
       (c.tem_contato AND NOT c.fone_proprio AND NOT c.email_proprio) AS contador,
       (CASE WHEN NOT c.tem_contato THEN 0
             WHEN NOT c.fone_proprio AND NOT c.email_proprio THEN 15
             ELSE least(100, CASE WHEN c.celular_proprio THEN 60 WHEN c.fone_proprio THEN 35 ELSE 0 END
                             + CASE WHEN c.email_proprio THEN 20 ELSE 0 END
                             + CASE WHEN c.fone_proprio AND c.fone2 IS NOT NULL THEN 10 ELSE 0 END) END)::smallint AS qualidade,
       c.socios_qtd, c.nome_busca
FROM c
ORDER BY c.uf, c.municipio, c.cnae, c.cnpj`

func tabelasDe(ctx context.Context, db Executor, esquema string) ([]string, error) {
	linhas, err := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = $1 ORDER BY 1`, esquema)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, pgx.RowTo[string])
}

// EsquemaExiste diz se o esquema existe.
func EsquemaExiste(ctx context.Context, db Executor, esquema string) (bool, error) {
	var existe bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, esquema).Scan(&existe)
	return existe, err
}

// Trocar põe o esquema construído em uso numa transação: o atual vira
// _velho, o _novo assume o nome e o velho é apagado. Os cursores de busca
// carregam a versão e passam a responder 409.
func Trocar(ctx context.Context, db Executor, destino string) error {
	novo, velho := destino+"_novo", destino+"_velho"
	atual, err := EsquemaExiste(ctx, db, destino)
	if err != nil {
		return err
	}
	comandos := []string{`BEGIN`, `SET LOCAL lock_timeout = '30s'`, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, velho)}
	if atual {
		comandos = append(comandos, fmt.Sprintf(`ALTER SCHEMA %s RENAME TO %s`, destino, velho))
	}
	comandos = append(comandos, fmt.Sprintf(`ALTER SCHEMA %s RENAME TO %s`, novo, destino), `COMMIT`)
	if err := executar(ctx, db, comandos...); err != nil {
		_, _ = db.Exec(context.WithoutCancel(ctx), `ROLLBACK`)
		return fmt.Errorf("troca de esquema: %w", err)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, velho)); err != nil {
		slog.Warn("não foi possível apagar o esquema anterior", "esquema", velho, "erro", err)
	}
	return nil
}

// Aquecer carrega em memória a tabela de busca, os índices de cobertura, o
// GIN e os cubos. Usa pg_prewarm quando a extensão existe (ela é criada pelo
// script de preparação, como administrador); na falta, lê as tabelas.
func Aquecer(ctx context.Context, db Executor, esquema string) {
	inicio := time.Now()
	var temPrewarm bool
	_ = db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_prewarm')`).Scan(&temPrewarm)
	quentes := []string{"busca", "busca_pkey", "busca_c1", "busca_c2", "busca_cnaes_gin", "cubo_mun", "cubo_uf_cnae", "cubo_cnae_qq", "faixa_mun", "faixa_uf", "base_ativa_cnpj"}
	if temPrewarm {
		var falha error
		for _, r := range quentes {
			if _, err := db.Exec(ctx, `SELECT pg_prewarm(c.oid::regclass) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2`, esquema, r); err != nil {
				falha = err
				break
			}
		}
		if falha == nil {
			slog.Info("base aquecida com pg_prewarm", "esquema", esquema, "duracao", time.Since(inicio).Round(time.Millisecond).String())
			return
		}
		var pg *pgconn.PgError
		if !errors.As(falha, &pg) {
			slog.Warn("aquecimento interrompido", "erro", falha)
			return
		}
	}
	for _, c := range []string{
		fmt.Sprintf(`SELECT count(*) FROM %s.busca`, esquema),
		fmt.Sprintf(`SELECT count(*) FROM %s.busca WHERE uf > ''`, esquema),
		fmt.Sprintf(`SELECT count(*) FROM %s.busca WHERE cnae > 0`, esquema),
		fmt.Sprintf(`SELECT sum(n) FROM %s.cubo_mun`, esquema),
		fmt.Sprintf(`SELECT sum(n) FROM %s.cubo_uf_cnae`, esquema),
		fmt.Sprintf(`SELECT sum(n) FROM %s.cubo_cnae_qq`, esquema),
	} {
		if _, err := db.Exec(ctx, c); err != nil {
			slog.Warn("aquecimento interrompido", "erro", err)
			return
		}
	}
	slog.Info("base aquecida por leitura", "esquema", esquema, "duracao", time.Since(inicio).Round(time.Millisecond).String())
}
