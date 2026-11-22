package carga

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/encoding/charmap"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// Os três sistemas da dívida ativa e o tipo que cada um vira no contrato.
const (
	DividaGeral          = "geral"          // SIDA (não previdenciário)
	DividaPrevidenciaria = "previdenciaria" // PREV
	DividaFGTS           = "fgts"
)

type sistemaPGFN struct {
	tipo      string
	cabecalho string
	// posições das colunas usadas
	inscricao, situacao, data, ajuizado, valor int
	inscricaoNumerica                          bool
}

var sistemasPGFN = []sistemaPGFN{
	{tipo: DividaGeral, inscricao: 6, situacao: 7, data: 10, ajuizado: 11, valor: 12, inscricaoNumerica: true,
		cabecalho: "CPF_CNPJ;TIPO_PESSOA;TIPO_DEVEDOR;NOME_DEVEDOR;UF_DEVEDOR;UNIDADE_RESPONSAVEL;NUMERO_INSCRICAO;TIPO_SITUACAO_INSCRICAO;SITUACAO_INSCRICAO;RECEITA_PRINCIPAL;DATA_INSCRICAO;INDICADOR_AJUIZADO;VALOR_CONSOLIDADO"},
	{tipo: DividaPrevidenciaria, inscricao: 6, situacao: 7, data: 10, ajuizado: 11, valor: 12, inscricaoNumerica: true,
		cabecalho: "CPF_CNPJ;TIPO_PESSOA;TIPO_DEVEDOR;NOME_DEVEDOR;UF_DEVEDOR;UNIDADE_RESPONSAVEL;NUMERO_INSCRICAO;TIPO_SITUACAO_INSCRICAO;SITUACAO_INSCRICAO;TIPO_CREDITO;DATA_INSCRICAO;INDICADOR_AJUIZADO;VALOR_CONSOLIDADO"},
	{tipo: DividaFGTS, inscricao: 8, situacao: 9, data: 12, ajuizado: 13, valor: 14,
		cabecalho: "CPF_CNPJ;TIPO_PESSOA;TIPO_DEVEDOR;NOME_DEVEDOR;UF_DEVEDOR;UNIDADE_RESPONSAVEL;ENTIDADE_RESPONSAVEL;UNIDADE_INSCRICAO;NUMERO_INSCRICAO;TIPO_SITUACAO_INSCRICAO;SITUACAO_INSCRICAO;RECEITA_PRINCIPAL;DATA_INSCRICAO;INDICADOR_AJUIZADO;VALOR_CONSOLIDADO"},
}

// colunasPGFN são as colunas da tabela de carga da dívida (tudo text). O
// nome do devedor não é gravado: nas linhas de MEI ele traz nome e CPF.
var colunasPGFN = []string{"cnpj", "sistema", "inscricao", "irregular", "ajuizada", "data", "valor"}

// limiteRejeitosPGFN: acima de 1 linha fora do padrão em 100 mil (com
// tolerância de 20 linhas) o trimestre é abortado.
const limiteRejeitosPGFN = 100_000

var reValorPGFN = regexp.MustCompile(`^[0-9]*\.?[0-9]+$`)

// fontePGFN lê um CSV da PGFN linha a linha. O arquivo não tem aspas nem
// escape, então a validação é própria: cabeçalho igual ao esperado, número
// de ";" do cabeçalho em cada linha e nenhum byte de controle. Só entram
// pessoa jurídica e devedor principal (os corresponsáveis repetem o valor
// cheio da inscrição).
type fontePGFN struct {
	ctx      context.Context
	r        *bufio.Reader
	s        sistemaPGFN
	arq      string
	est      *Estatisticas
	campos   int
	valores  []any
	err      error
	rejeitos []string
}

func (f *fontePGFN) Next() bool {
	for {
		linha, err := f.r.ReadString('\n')
		if err != nil && err != io.EOF {
			f.err = err
			return false
		}
		if linha == "" && err == io.EOF {
			return false
		}
		fim := err == io.EOF
		linha = strings.TrimSuffix(linha, "\n")
		if linha == "" {
			if fim {
				return false
			}
			continue
		}
		f.est.Lidas++
		if f.est.Lidas&0x3fff == 0 {
			if err := f.ctx.Err(); err != nil {
				f.err = err
				return false
			}
		}
		if v := f.linha(linha); v != nil {
			f.valores = append(append(f.valores[:0], f.arq), v...)
			f.est.Gravadas++
			return true
		}
		if f.est.Rejeitadas > 20 && f.est.Rejeitadas*limiteRejeitosPGFN > f.est.Lidas {
			f.err = fmt.Errorf("%s: %d linhas fora do padrão em %d lidas (primeiras: %s)", f.arq, f.est.Rejeitadas, f.est.Lidas, strings.Join(f.rejeitos, " | "))
			return false
		}
		if fim {
			return false
		}
	}
}

func (f *fontePGFN) rejeitar(motivo string) []any {
	f.est.Rejeitadas++
	if len(f.rejeitos) < 5 {
		f.rejeitos = append(f.rejeitos, fmt.Sprintf("linha %d: %s", f.est.Lidas+1, motivo))
	}
	return nil
}

func (f *fontePGFN) linha(l string) []any {
	for i := 0; i < len(l); i++ {
		if l[i] < 0x20 || l[i] == 0x7f {
			return f.rejeitar("byte de controle")
		}
	}
	c := strings.Split(l, ";")
	if len(c) != f.campos {
		return f.rejeitar(fmt.Sprintf("%d campos, esperados %d", len(c), f.campos))
	}
	if c[1] != "Pessoa jurídica" || !strings.EqualFold(c[2], "PRINCIPAL") {
		return nil // pessoa física e corresponsável nunca são gravados
	}
	cnpj := texto.NormalizarCNPJ(c[0])
	if !texto.CNPJFormaValida(cnpj) {
		return f.rejeitar("CNPJ fora do formato")
	}
	inscricao := strings.TrimSpace(c[f.s.inscricao])
	if f.s.inscricaoNumerica {
		// o exportador trata o número como numérico: inscrição que começa
		// por 00 (Rio Grande do Sul) perde os zeros e ganha ".0"
		inscricao = strings.TrimSuffix(inscricao, ".0")
		if len(inscricao) < 13 {
			inscricao = strings.Repeat("0", 13-len(inscricao)) + inscricao
		}
	}
	valor := strings.TrimSpace(c[f.s.valor])
	if inscricao == "" || !reValorPGFN.MatchString(valor) {
		return f.rejeitar("inscrição ou valor fora do formato")
	}
	if valor[0] == '.' {
		valor = "0" + valor // ".00" é zero
	}
	return []any{cnpj, f.s.tipo, inscricao, tf(c[f.s.situacao] == "Em cobrança"), tf(strings.EqualFold(c[f.s.ajuizado], "SIM")),
		nulo(texto.DataBR(c[f.s.data])), valor}
}

func tf(v bool) string {
	if v {
		return "t"
	}
	return "f"
}

func (f *fontePGFN) Values() ([]any, error) { return f.valores, nil }
func (f *fontePGFN) Err() error             { return f.err }

// CarregarPGFN lê um CSV da PGFN (ISO-8859-1, com cabeçalho, sem aspas) e
// envia as inscrições de pessoa jurídica como devedora principal para a
// tabela de carga pgfn. O sistema (SIDA, PREV ou FGTS) sai do cabeçalho: se
// ele mudar, a leitura para.
func CarregarPGFN(ctx context.Context, db Executor, esquema, arq string, r io.Reader) (Estatisticas, error) {
	var est Estatisticas
	leitor := bufio.NewReaderSize(charmap.ISO8859_1.NewDecoder().Reader(r), 1<<20)
	cab, err := leitor.ReadString('\n')
	if err != nil && err != io.EOF {
		return est, fmt.Errorf("%s: %w", arq, err)
	}
	cab = strings.TrimRight(cab, "\r\n")
	fonte := &fontePGFN{ctx: ctx, r: leitor, arq: arq, est: &est}
	for _, s := range sistemasPGFN {
		if s.cabecalho == cab {
			fonte.s, fonte.campos = s, strings.Count(s.cabecalho, ";")+1
		}
	}
	if fonte.campos == 0 {
		return est, fmt.Errorf("%s: cabeçalho desconhecido (o leiaute da PGFN mudou?): %.200q", arq, cab)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.pgfn WHERE arq = $1`, esquema), arq); err != nil {
		return est, err
	}
	if _, err := db.CopyFrom(ctx, pgx.Identifier{esquema, "pgfn"}, append([]string{"arq"}, colunasPGFN...), fonte); err != nil {
		est.Gravadas = 0
		if fonte.err != nil {
			err = fonte.err
		}
		return est, fmt.Errorf("%s: %w", arq, err)
	}
	return est, nil
}

// CarregarZipPGFN carrega todos os CSVs de um zip da PGFN (um por região).
// As linhas ficam marcadas com o nome do zip, para a retomada.
func CarregarZipPGFN(ctx context.Context, db Executor, esquema, caminho string) (Estatisticas, error) {
	var total Estatisticas
	z, err := zip.OpenReader(caminho)
	if err != nil {
		return total, fmt.Errorf("%s: zip ilegível: %w", caminho, err)
	}
	defer z.Close()
	base := caminho
	if i := strings.LastIndexAny(caminho, `/\`); i >= 0 {
		base = caminho[i+1:]
	}
	base = strings.TrimSuffix(base, ".zip")
	if _, err := db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.pgfn WHERE arq LIKE $1`, esquema), base+"/%"); err != nil {
		return total, err
	}
	n := 0
	for _, m := range z.File {
		if m.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(m.Name), ".csv") {
			continue
		}
		r, err := m.Open()
		if err != nil {
			return total, err
		}
		est, err := CarregarPGFN(ctx, db, esquema, base+"/"+m.Name, r)
		r.Close()
		if err != nil {
			return total, err
		}
		total.Somar(est)
		n++
	}
	if n == 0 {
		return total, fmt.Errorf("%s: nenhum CSV dentro do zip", caminho)
	}
	return total, nil
}
