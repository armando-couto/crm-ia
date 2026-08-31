package models

import (
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// Entidades que aceitam anexo.
const (
	AttachContact = "contato"
	AttachCompany = "empresa"
	AttachDeal    = "negocio"
	AttachTicket  = "ticket"
)

var attachEntities = []string{AttachContact, AttachCompany, AttachDeal, AttachTicket}

// MaxAttachmentBytes limita cada arquivo a 10 MB.
const MaxAttachmentBytes = 10 << 20

func ValidAttachmentEntity(entity string) bool {
	return slices.Contains(attachEntities, entity)
}

// Attachment é o metadado do arquivo; o conteúdo só é lido no download.
type Attachment struct {
	ID           int64     `json:"id"`
	Entity       string    `json:"entity"`
	EntityID     int64     `json:"entity_id"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	UploadedBy   *int64    `json:"uploaded_by"`
	UploaderName string    `json:"uploader_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func CreateAttachment(db *sql.DB, a *Attachment, data []byte) error {
	if !ValidAttachmentEntity(a.Entity) {
		return fmt.Errorf("tipo de registro inválido para anexo")
	}
	a.SizeBytes = int64(len(data))
	return db.QueryRow(`
		INSERT INTO attachments (entity, entity_id, filename, content_type, size_bytes, data, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		a.Entity, a.EntityID, a.Filename, a.ContentType, a.SizeBytes, data, a.UploadedBy,
	).Scan(&a.ID, &a.CreatedAt)
}

func ListAttachments(db *sql.DB, entity string, entityID int64) ([]Attachment, error) {
	rows, err := db.Query(`
		SELECT a.id, a.entity, a.entity_id, a.filename, a.content_type, a.size_bytes,
		       a.uploaded_by, COALESCE(u.name, ''), a.created_at
		FROM attachments a
		LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.entity = $1 AND a.entity_id = $2
		ORDER BY a.created_at DESC`, entity, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.Entity, &a.EntityID, &a.Filename, &a.ContentType,
			&a.SizeBytes, &a.UploadedBy, &a.UploaderName, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// AttachmentByID devolve o metadado sem o conteúdo (para checagens e exclusão).
func AttachmentByID(db *sql.DB, id int64) (*Attachment, error) {
	var a Attachment
	err := db.QueryRow(`
		SELECT id, entity, entity_id, filename, content_type, size_bytes, uploaded_by, created_at
		FROM attachments WHERE id = $1`, id,
	).Scan(&a.ID, &a.Entity, &a.EntityID, &a.Filename, &a.ContentType, &a.SizeBytes,
		&a.UploadedBy, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AttachmentContent devolve o arquivo completo para download.
func AttachmentContent(db *sql.DB, id int64) (*Attachment, []byte, error) {
	var a Attachment
	var data []byte
	err := db.QueryRow(`
		SELECT id, entity, entity_id, filename, content_type, size_bytes, created_at, data
		FROM attachments WHERE id = $1`, id,
	).Scan(&a.ID, &a.Entity, &a.EntityID, &a.Filename, &a.ContentType, &a.SizeBytes,
		&a.CreatedAt, &data)
	if err != nil {
		return nil, nil, err
	}
	return &a, data, nil
}

func DeleteAttachment(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM attachments WHERE id = $1`, id)
	return err
}

// CountAttachments informa quantos arquivos o registro tem (usado nas abas).
func CountAttachments(db *sql.DB, entity string, entityID int64) (int, error) {
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE entity = $1 AND entity_id = $2`,
		entity, entityID).Scan(&total)
	return total, err
}
