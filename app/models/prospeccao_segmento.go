package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ProspSegmento é um filtro salvo (não o resultado), para reexecutar quando a
// base muda.
type ProspSegmento struct {
	ID               int64        `json:"id"`
	Nome             string       `json:"nome"`
	Filtros          ProspFiltros `json:"filtros"`
	Ordem            string       `json:"ordem"`
	Compartilhado    bool         `json:"compartilhado"`
	UltimaContagem   *int         `json:"ultima_contagem"`
	UltimaContagemEm *time.Time   `json:"ultima_contagem_em"`
	CriadoPor        *int64       `json:"criado_por"`
	CriadoPorNome    string       `json:"criado_por_nome"`
	CriadoEm         time.Time    `json:"criado_em"`
	AtualizadoEm     time.Time    `json:"atualizado_em"`
}

// Validar confere nome e ordem (os filtros são conferidos por ProspFiltros.Validar).
func (s *ProspSegmento) Validar() error {
	s.Nome = strings.TrimSpace(s.Nome)
	if s.Nome == "" {
		return fmt.Errorf("informe o nome do segmento")
	}
	if len([]rune(s.Nome)) > 150 {
		return fmt.Errorf("o nome do segmento é longo demais")
	}
	if s.Ordem == "" {
		s.Ordem = OrdemRelevancia
	}
	if !ProspOrdemValida(s.Ordem) {
		return fmt.Errorf("ordenação inválida")
	}
	s.Filtros.Normalizar()
	return nil
}

const prospSegmentoSelect = `
	SELECT s.id, s.nome, s.filtros, s.ordem, s.compartilhado, s.ultima_contagem, s.ultima_contagem_em,
	       s.criado_por, COALESCE(u.name,''), s.criado_em, s.atualizado_em
	FROM prospeccao_segmentos s
	LEFT JOIN users u ON u.id = s.criado_por
	WHERE `

func scanProspSegmento(row interface{ Scan(...any) error }) (*ProspSegmento, error) {
	var s ProspSegmento
	var filtros []byte
	if err := row.Scan(&s.ID, &s.Nome, &filtros, &s.Ordem, &s.Compartilhado, &s.UltimaContagem,
		&s.UltimaContagemEm, &s.CriadoPor, &s.CriadoPorNome, &s.CriadoEm, &s.AtualizadoEm); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(filtros, &s.Filtros)
	return &s, nil
}

// ListProspSegmentos devolve os segmentos compartilhados e os do próprio
// usuário, do mais recente para o mais antigo.
func ListProspSegmentos(db *sql.DB, userID int64) ([]ProspSegmento, error) {
	rows, err := db.Query(prospSegmentoSelect+`(s.compartilhado OR s.criado_por = $1)
		ORDER BY s.atualizado_em DESC, s.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lista := []ProspSegmento{}
	for rows.Next() {
		s, err := scanProspSegmento(rows)
		if err != nil {
			return nil, err
		}
		lista = append(lista, *s)
	}
	return lista, rows.Err()
}

// ProspSegmentoByID devolve sql.ErrNoRows quando não existe.
func ProspSegmentoByID(db *sql.DB, id int64) (*ProspSegmento, error) {
	return scanProspSegmento(db.QueryRow(prospSegmentoSelect+`s.id = $1`, id))
}

// CreateProspSegmento grava o segmento e preenche id e datas.
func CreateProspSegmento(db *sql.DB, s *ProspSegmento) error {
	return db.QueryRow(`
		INSERT INTO prospeccao_segmentos (nome, filtros, ordem, compartilhado, criado_por)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, criado_em, atualizado_em`,
		s.Nome, jsonOu(s.Filtros, "{}"), s.Ordem, s.Compartilhado, s.CriadoPor,
	).Scan(&s.ID, &s.CriadoEm, &s.AtualizadoEm)
}

// UpdateProspSegmento regrava nome, filtros, ordem e compartilhamento. Mudar
// os filtros invalida a última contagem. sql.ErrNoRows quando não existe.
func UpdateProspSegmento(db *sql.DB, s *ProspSegmento) error {
	return db.QueryRow(`
		UPDATE prospeccao_segmentos
		SET nome = $2, ordem = $4, compartilhado = $5, atualizado_em = NOW(),
		    ultima_contagem    = CASE WHEN filtros = $3::jsonb THEN ultima_contagem END,
		    ultima_contagem_em = CASE WHEN filtros = $3::jsonb THEN ultima_contagem_em END,
		    filtros = $3::jsonb
		WHERE id = $1
		RETURNING ultima_contagem, ultima_contagem_em, atualizado_em`,
		s.ID, s.Nome, jsonOu(s.Filtros, "{}"), s.Ordem, s.Compartilhado,
	).Scan(&s.UltimaContagem, &s.UltimaContagemEm, &s.AtualizadoEm)
}

// DeleteProspSegmento apaga o segmento (as listas geradas dele ficam).
func DeleteProspSegmento(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM prospeccao_segmentos WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RegistrarContagemProspSegmento guarda o total da contagem feita a partir do
// segmento (POST /prospeccao/contagem com segmento_id).
func RegistrarContagemProspSegmento(db *sql.DB, id int64, total int) error {
	_, err := db.Exec(`
		UPDATE prospeccao_segmentos SET ultima_contagem = $2, ultima_contagem_em = NOW() WHERE id = $1`,
		id, total)
	return err
}
