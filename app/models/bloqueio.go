package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

// -----------------------------------------------------------------------------
// Bloqueios ("não contatar"): sobrevivem à exclusão do lead e a novas listas.
// Efeito por tipo: cnpj barra a empresa inteira (candidato pulado ao gerar a
// lista, item "bloqueado" no envio, lead fora da fila); telefone e e-mail só
// removem aquele contato.
// -----------------------------------------------------------------------------

// Tipos e motivos de bloqueio.
const (
	BloqueioTelefone = "telefone"
	BloqueioEmail    = "email"
	BloqueioCNPJ     = "cnpj"

	BloqueioOptOut        = "opt_out"
	BloqueioNumeroErrado  = "numero_errado"
	BloqueioPedidoTitular = "pedido_titular"
	BloqueioCliente       = "cliente"
	BloqueioManual        = "manual"
	BloqueioListaExterna  = "lista_externa"

	// MaxBloqueiosImportacao é o teto de valores por importação.
	MaxBloqueiosImportacao = 5000
)

var (
	BloqueioTipos   = []string{BloqueioTelefone, BloqueioEmail, BloqueioCNPJ}
	BloqueioMotivos = []string{BloqueioOptOut, BloqueioNumeroErrado, BloqueioPedidoTitular, BloqueioCliente, BloqueioManual, BloqueioListaExterna}
)

func BloqueioTipoValido(t string) bool   { return slices.Contains(BloqueioTipos, t) }
func BloqueioMotivoValido(m string) bool { return slices.Contains(BloqueioMotivos, m) }

// Bloqueio é uma linha de contato_bloqueios (interface Bloqueio).
type Bloqueio struct {
	ID            int64     `json:"id"`
	Tipo          string    `json:"tipo"`
	Valor         string    `json:"valor"`
	Motivo        string    `json:"motivo"`
	Canal         string    `json:"canal"`
	Observacao    string    `json:"observacao"`
	CriadoPor     *int64    `json:"-"`
	CriadoPorNome string    `json:"criado_por_nome"`
	CriadoEm      time.Time `json:"criado_em"`
}

// NormalizarTelefone deixa só dígitos com DDD (10 ou 11), tirando o DDI 55 e o
// zero de operadora. Vazio quando não sobra um telefone brasileiro plausível.
func NormalizarTelefone(s string) string {
	d := SomenteDigitos(s)
	if len(d) > 11 && strings.HasPrefix(d, "55") {
		d = d[2:]
	}
	d = strings.TrimLeft(d, "0")
	if len(d) != 10 && len(d) != 11 {
		return ""
	}
	return d
}

// NormalizarValorBloqueio devolve o valor na forma gravada: telefone só
// dígitos com DDD; e-mail minúsculo; CNPJ normalizado e válido.
func NormalizarValorBloqueio(tipo, valor string) (string, error) {
	switch tipo {
	case BloqueioTelefone:
		if t := NormalizarTelefone(valor); t != "" {
			return t, nil
		}
		return "", fmt.Errorf("telefone inválido: informe o número com DDD")
	case BloqueioEmail:
		e := strings.ToLower(strings.TrimSpace(valor))
		at := strings.LastIndex(e, "@")
		if at < 1 || at == len(e)-1 || strings.ContainsAny(e, " \t") || len(e) > 255 {
			return "", fmt.Errorf("e-mail inválido")
		}
		return e, nil
	case BloqueioCNPJ:
		if !CNPJValido(valor) {
			return "", fmt.Errorf("CNPJ inválido")
		}
		return NormalizarCNPJ(valor), nil
	}
	return "", fmt.Errorf("tipo de bloqueio inválido")
}

// CreateBloqueio normaliza e grava. Valor que já estava bloqueado não é erro:
// devolve criado = false e preenche b com o registro existente.
func CreateBloqueio(db Executor, b *Bloqueio) (criado bool, err error) {
	if !BloqueioTipoValido(b.Tipo) {
		return false, fmt.Errorf("tipo de bloqueio inválido")
	}
	if b.Motivo == "" {
		b.Motivo = BloqueioManual
	}
	if !BloqueioMotivoValido(b.Motivo) {
		return false, fmt.Errorf("motivo de bloqueio inválido")
	}
	if b.Valor, err = NormalizarValorBloqueio(b.Tipo, b.Valor); err != nil {
		return false, err
	}
	b.Canal = cortarTexto(b.Canal, 20)
	b.Observacao = strings.TrimSpace(b.Observacao)
	err = db.QueryRow(`
		INSERT INTO contato_bloqueios (tipo, valor, motivo, canal, observacao, criado_por)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		ON CONFLICT (tipo, valor) DO NOTHING
		RETURNING id, criado_em`,
		b.Tipo, b.Valor, b.Motivo, b.Canal, b.Observacao, b.CriadoPor).Scan(&b.ID, &b.CriadoEm)
	if err == sql.ErrNoRows {
		existente, err := scanBloqueio(db.QueryRow(bloqueioSelect+`b.tipo = $1 AND b.valor = $2`, b.Tipo, b.Valor))
		if err != nil {
			return false, err
		}
		*b = *existente
		return false, nil
	}
	return err == nil, err
}

// ImportarBloqueios grava vários valores do mesmo tipo e motivo de uma vez.
// Devolve quantos foram criados, quantos já existiam (ou vieram repetidos na
// própria carga) e quantos eram inválidos.
func ImportarBloqueios(db Executor, tipo, motivo string, valores []string, criadoPor *int64) (criados, repetidos, invalidos int, err error) {
	if !BloqueioTipoValido(tipo) {
		return 0, 0, 0, fmt.Errorf("tipo de bloqueio inválido")
	}
	if motivo == "" {
		motivo = BloqueioListaExterna
	}
	if !BloqueioMotivoValido(motivo) {
		return 0, 0, 0, fmt.Errorf("motivo de bloqueio inválido")
	}
	if len(valores) > MaxBloqueiosImportacao {
		return 0, 0, 0, fmt.Errorf("a importação aceita até %d valores por vez", MaxBloqueiosImportacao)
	}
	visto := map[string]bool{}
	limpos := []string{}
	for _, v := range valores {
		if strings.TrimSpace(v) == "" {
			continue
		}
		n, errV := NormalizarValorBloqueio(tipo, v)
		switch {
		case errV != nil:
			invalidos++
		case visto[n]:
			repetidos++
		default:
			visto[n] = true
			limpos = append(limpos, n)
		}
	}
	if len(limpos) == 0 {
		return 0, repetidos, invalidos, nil
	}
	res, err := db.Exec(`
		INSERT INTO contato_bloqueios (tipo, valor, motivo, canal, criado_por)
		SELECT $1, v, $2, 'importacao', $3 FROM unnest($4::text[]) AS v
		ON CONFLICT (tipo, valor) DO NOTHING`, tipo, motivo, criadoPor, pq.Array(limpos))
	if err != nil {
		return 0, 0, 0, err
	}
	n, _ := res.RowsAffected()
	criados = int(n)
	repetidos += len(limpos) - criados
	return criados, repetidos, invalidos, nil
}

const bloqueioSelect = `
	SELECT b.id, b.tipo, b.valor, b.motivo, b.canal, COALESCE(b.observacao,''), b.criado_por,
	       COALESCE(u.name,''), b.criado_em
	FROM contato_bloqueios b
	LEFT JOIN users u ON u.id = b.criado_por
	WHERE `

func scanBloqueio(row interface{ Scan(...any) error }) (*Bloqueio, error) {
	var b Bloqueio
	if err := row.Scan(&b.ID, &b.Tipo, &b.Valor, &b.Motivo, &b.Canal, &b.Observacao, &b.CriadoPor,
		&b.CriadoPorNome, &b.CriadoEm); err != nil {
		return nil, err
	}
	return &b, nil
}

// BloqueioFilter filtra a listagem de bloqueios.
type BloqueioFilter struct {
	Q    string
	Tipo string
	Pagination
}

// ListBloqueios devolve a página e o total, do mais recente para o mais antigo.
// A busca casa o valor como digitado e também sem pontuação.
func ListBloqueios(db *sql.DB, f BloqueioFilter) ([]Bloqueio, int, error) {
	f.Normalize()
	where := []string{"TRUE"}
	args := []any{}
	if f.Tipo != "" {
		args = append(args, f.Tipo)
		where = append(where, fmt.Sprintf("b.tipo = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, likeContem(strings.ToLower(q)))
		cond := fmt.Sprintf("(LOWER(b.valor) LIKE $%d OR LOWER(COALESCE(b.observacao,'')) LIKE $%d", len(args), len(args))
		if limpo := NormalizarCNPJ(q); limpo != "" && limpo != strings.ToUpper(q) {
			args = append(args, likeContem(limpo))
			cond += fmt.Sprintf(" OR UPPER(b.valor) LIKE $%d", len(args))
		}
		where = append(where, cond+")")
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM contato_bloqueios b WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(bloqueioSelect+`%s ORDER BY b.criado_em DESC, b.id DESC LIMIT $%d OFFSET $%d`,
		cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	lista := []Bloqueio{}
	for rows.Next() {
		b, err := scanBloqueio(rows)
		if err != nil {
			return nil, 0, err
		}
		lista = append(lista, *b)
	}
	return lista, total, rows.Err()
}

// BloqueioByID devolve sql.ErrNoRows quando não existe.
func BloqueioByID(db Executor, id int64) (*Bloqueio, error) {
	return scanBloqueio(db.QueryRow(bloqueioSelect+`b.id = $1`, id))
}

// DeleteBloqueio remove o bloqueio e devolve o que foi removido (para a
// auditoria). sql.ErrNoRows quando não existe.
func DeleteBloqueio(db *sql.DB, id int64) (*Bloqueio, error) {
	b, err := BloqueioByID(db, id)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`DELETE FROM contato_bloqueios WHERE id = $1`, id); err != nil {
		return nil, err
	}
	return b, nil
}

// ValoresBloqueados devolve, entre os valores informados (em qualquer forma),
// os que estão bloqueados, já normalizados. Valores inválidos são ignorados.
func ValoresBloqueados(db Executor, tipo string, valores []string) (map[string]bool, error) {
	out := map[string]bool{}
	limpos := make([]string, 0, len(valores))
	for _, v := range valores {
		if n, err := NormalizarValorBloqueio(tipo, v); err == nil {
			limpos = append(limpos, n)
		} else if tipo == BloqueioCNPJ {
			// CNPJ de base antiga com DV errado ainda pode estar na lista de bloqueio.
			if n := NormalizarCNPJ(v); n != "" {
				limpos = append(limpos, n)
			}
		}
	}
	if len(limpos) == 0 {
		return out, nil
	}
	rows, err := db.Query(`SELECT valor FROM contato_bloqueios WHERE tipo = $1 AND valor = ANY($2)`, tipo, pq.Array(limpos))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// CNPJsBloqueados é o atalho de ValoresBloqueados para a geração de listas.
func CNPJsBloqueados(db Executor, cnpjs []string) (map[string]bool, error) {
	return ValoresBloqueados(db, BloqueioCNPJ, cnpjs)
}

// Bloqueado responde por um valor só.
func Bloqueado(db Executor, tipo, valor string) (bool, error) {
	m, err := ValoresBloqueados(db, tipo, []string{valor})
	return len(m) > 0, err
}

// BloqueiosDoContato reúne o que está bloqueado para uma empresa.
type BloqueiosDoContato struct {
	CNPJ      bool
	Telefones map[string]bool // número normalizado → bloqueado
	Emails    map[string]bool
}

// TodosTelefonesBloqueados diz se havia telefones e todos estão bloqueados.
func (b BloqueiosDoContato) TodosTelefonesBloqueados(telefones []string) bool {
	if len(telefones) == 0 {
		return false
	}
	for _, t := range telefones {
		if !b.Telefones[NormalizarTelefone(t)] {
			return false
		}
	}
	return true
}

// ConferirBloqueios consulta de uma vez o CNPJ, os telefones e os e-mails de
// uma empresa (envio ao CRM, ficha do lead, fila).
func ConferirBloqueios(db Executor, cnpj string, telefones, emails []string) (BloqueiosDoContato, error) {
	r := BloqueiosDoContato{Telefones: map[string]bool{}, Emails: map[string]bool{}}
	var err error
	if NormalizarCNPJ(cnpj) != "" {
		if r.CNPJ, err = Bloqueado(db, BloqueioCNPJ, cnpj); err != nil {
			return r, err
		}
	}
	if len(telefones) > 0 {
		if r.Telefones, err = ValoresBloqueados(db, BloqueioTelefone, telefones); err != nil {
			return r, err
		}
	}
	if len(emails) > 0 {
		if r.Emails, err = ValoresBloqueados(db, BloqueioEmail, emails); err != nil {
			return r, err
		}
	}
	return r, nil
}
