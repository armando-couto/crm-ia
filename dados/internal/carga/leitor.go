// Package carga baixa, valida e carrega as bases públicas (CNPJ da Receita e
// dívida ativa da PGFN), constrói o esquema de serviço ao lado do que está em
// uso e faz a troca.
package carga

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/encoding/charmap"
)

// Executor é o que a carga precisa de uma conexão (pgx.Conn, pgxpool.Conn ou
// pgxpool.Pool).
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	CopyFrom(ctx context.Context, tabela pgx.Identifier, colunas []string, fonte pgx.CopyFromSource) (int64, error)
}

// semNUL remove bytes 0x00 do fluxo: um arquivo de 2026-11 tem NUL num campo
// de texto e isso aborta o COPY do PostgreSQL 16.
type semNUL struct {
	r io.Reader
	n *int64
}

func (s *semNUL) Read(p []byte) (int, error) {
	for {
		n, err := s.r.Read(p)
		if n > 0 && bytes.IndexByte(p[:n], 0) >= 0 {
			j := 0
			for i := 0; i < n; i++ {
				if p[i] != 0 {
					p[j] = p[i]
					j++
				}
			}
			*s.n += int64(n - j)
			n = j
		}
		if n > 0 || err != nil {
			return n, err
		}
	}
}

// fonteCSV entrega ao COPY os registros de um CSV da Receita já limpos. O
// CSV é lido em fluxo com encoding/csv estrito: separador ";", número de
// campos fixo, aspas duplicadas, quebra de linha dentro de aspas aceita (o
// arquivo nunca é dividido por linhas).
type fonteCSV struct {
	ctx       context.Context
	r         *csv.Reader
	l         Leiaute
	arq       string
	ufs       map[string]bool
	est       *Estatisticas
	valores   []any
	err       error
	progresso func(lidas int64)
}

func (f *fonteCSV) Next() bool {
	for {
		reg, err := f.r.Read()
		if err == io.EOF {
			return false
		}
		if err != nil {
			f.err = fmt.Errorf("registro %d: %w", f.est.Lidas+1, err)
			return false
		}
		f.est.Lidas++
		if f.est.Lidas&0x3fff == 0 {
			if err := f.ctx.Err(); err != nil {
				f.err = err
				return false
			}
			if f.progresso != nil {
				f.progresso(f.est.Lidas)
			}
		}
		v := f.l.Linha(reg, f.est, f.ufs)
		if v == nil {
			continue
		}
		f.valores = append(append(f.valores[:0], f.arq), v...)
		f.est.Gravadas++
		return true
	}
}

func (f *fonteCSV) Values() ([]any, error) { return f.valores, nil }
func (f *fonteCSV) Err() error             { return f.err }

// CarregarCSV lê um CSV da Receita (ISO-8859-1, sem cabeçalho) e o envia por
// COPY para a tabela de carga do leiaute, marcando as linhas com arq. Erro
// de estrutura (número de campos, aspas) aborta o COPY inteiro: nada entra.
func CarregarCSV(ctx context.Context, db Executor, esquema string, l Leiaute, arq string, r io.Reader, ufs []string, progresso func(lidas int64)) (Estatisticas, error) {
	var est Estatisticas
	leitor := csv.NewReader(charmap.ISO8859_1.NewDecoder().Reader(&semNUL{r: r, n: &est.NULs}))
	leitor.Comma = ';'
	leitor.FieldsPerRecord = l.Campos
	leitor.LazyQuotes = false
	leitor.ReuseRecord = true
	recorte := map[string]bool{}
	for _, uf := range ufs {
		recorte[uf] = true
	}
	// Retomada: se a carga anterior deste arquivo ficou pela metade no
	// controle, as linhas dele saem antes de entrar de novo.
	if _, err := db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.%s WHERE arq = $1`, esquema, l.Tabela), arq); err != nil {
		return est, err
	}
	fonte := &fonteCSV{ctx: ctx, r: leitor, l: l, arq: arq, ufs: recorte, est: &est, progresso: progresso}
	_, err := db.CopyFrom(ctx, pgx.Identifier{esquema, l.Tabela}, append([]string{"arq"}, l.Colunas...), fonte)
	if err != nil {
		est.Gravadas = 0
		if fonte.err != nil {
			err = fonte.err // o pgx devolve o erro da fonte embrulhado em texto pelo servidor
		}
		var perr *csv.ParseError
		if errors.As(err, &perr) {
			return est, fmt.Errorf("%s: CSV fora do leiaute (%d campos esperados): %w", arq, l.Campos, err)
		}
		return est, fmt.Errorf("%s: %w", arq, err)
	}
	return est, nil
}

// CarregarZip lê em fluxo o único membro do zip (o nome interno muda todo
// mês). O CRC do zip é conferido pelo archive/zip ao chegar ao fim.
func CarregarZip(ctx context.Context, db Executor, esquema string, l Leiaute, caminho string, ufs []string, progresso func(lidas int64)) (Estatisticas, error) {
	z, err := zip.OpenReader(caminho)
	if err != nil {
		return Estatisticas{}, fmt.Errorf("%s: zip ilegível: %w", caminho, err)
	}
	defer z.Close()
	var membros []*zip.File
	for _, f := range z.File {
		if !f.FileInfo().IsDir() {
			membros = append(membros, f)
		}
	}
	if len(membros) != 1 {
		return Estatisticas{}, fmt.Errorf("%s: esperado 1 arquivo dentro do zip, há %d", caminho, len(membros))
	}
	r, err := membros[0].Open()
	if err != nil {
		return Estatisticas{}, err
	}
	defer r.Close()
	arq := caminho
	if i := strings.LastIndexAny(caminho, `/\`); i >= 0 {
		arq = caminho[i+1:]
	}
	return CarregarCSV(ctx, db, esquema, l, strings.TrimSuffix(arq, ".zip"), r, ufs, progresso)
}

// CarregarZipBytes é o CarregarZip para um zip em memória (catálogos
// embutidos da amostra).
func CarregarZipBytes(ctx context.Context, db Executor, esquema string, l Leiaute, arq string, dados []byte) (Estatisticas, error) {
	z, err := zip.NewReader(bytes.NewReader(dados), int64(len(dados)))
	if err != nil {
		return Estatisticas{}, fmt.Errorf("%s: zip ilegível: %w", arq, err)
	}
	if len(z.File) != 1 {
		return Estatisticas{}, fmt.Errorf("%s: esperado 1 arquivo dentro do zip, há %d", arq, len(z.File))
	}
	r, err := z.File[0].Open()
	if err != nil {
		return Estatisticas{}, err
	}
	defer r.Close()
	return CarregarCSV(ctx, db, esquema, l, arq, r, nil, nil)
}

// InserirLinhas grava linhas já limpas numa tabela de carga (a amostra usa o
// mesmo formato das tabelas de carga e o mesmo passo de construção).
func InserirLinhas(ctx context.Context, db Executor, esquema, tabela, arq string, colunas []string, linhas [][]any) error {
	for i := range linhas {
		linhas[i] = append([]any{arq}, linhas[i]...)
	}
	_, err := db.CopyFrom(ctx, pgx.Identifier{esquema, tabela}, append([]string{"arq"}, colunas...), pgx.CopyFromRows(linhas))
	return err
}

// ColunasPGFN são as colunas da tabela de carga da dívida ativa.
func ColunasPGFN() []string { return append([]string{}, colunasPGFN...) }
