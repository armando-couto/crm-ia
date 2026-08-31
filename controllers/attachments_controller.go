package controllers

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ListAttachments devolve os arquivos de um registro (?entity=contato&entity_id=1).
func ListAttachments(ctx iris.Context) {
	entity := ctx.URLParam("entity")
	entityID := ctx.URLParamInt64Default("entity_id", 0)
	if !models.ValidAttachmentEntity(entity) || entityID == 0 {
		badRequest(ctx, "informe o registro (entity e entity_id)")
		return
	}

	list, err := models.ListAttachments(utils.DB, entity, entityID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

// UploadAttachment recebe o arquivo em multipart (campo "file") e o guarda no banco.
func UploadAttachment(ctx iris.Context) {
	entity := ctx.FormValue("entity")
	entityID, _ := ctx.PostValueInt64("entity_id")
	if !models.ValidAttachmentEntity(entity) || entityID == 0 {
		badRequest(ctx, "informe o registro (entity e entity_id)")
		return
	}

	// SetMaxRequestBodySize já corta uploads gigantes antes de chegar aqui.
	file, header, err := ctx.FormFile("file")
	if err != nil {
		badRequest(ctx, "envie o arquivo no campo 'file'")
		return
	}
	defer file.Close()

	if header.Size > models.MaxAttachmentBytes {
		badRequest(ctx, fmt.Sprintf("arquivo muito grande: o limite é %d MB",
			models.MaxAttachmentBytes>>20))
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, models.MaxAttachmentBytes+1))
	if err != nil {
		serverError(ctx, err)
		return
	}
	if len(data) > models.MaxAttachmentBytes {
		badRequest(ctx, fmt.Sprintf("arquivo muito grande: o limite é %d MB",
			models.MaxAttachmentBytes>>20))
		return
	}
	if len(data) == 0 {
		badRequest(ctx, "arquivo vazio")
		return
	}

	att := &models.Attachment{
		Entity:      entity,
		EntityID:    entityID,
		Filename:    safeFilename(header.Filename),
		ContentType: contentTypeOf(header.Header.Get("Content-Type"), header.Filename),
	}
	if claims := middlewareClaims(ctx); claims != nil {
		att.UploadedBy = &claims.UserID
	}

	if err := models.CreateAttachment(utils.DB, att, data); err != nil {
		serverError(ctx, err)
		return
	}

	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   "Arquivo anexado: " + att.Filename,
		ContactID: entityRef(entity, models.AttachContact, entityID),
		CompanyID: entityRef(entity, models.AttachCompany, entityID),
		DealID:    entityRef(entity, models.AttachDeal, entityID),
		TicketID:  entityRef(entity, models.AttachTicket, entityID),
	})
	audit(ctx, models.AuditCreate, "anexo", att.ID,
		fmt.Sprintf("anexou %s ao %s #%d", att.Filename, entity, entityID))

	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(att)
}

// DownloadAttachment devolve o arquivo com o nome original.
func DownloadAttachment(ctx iris.Context) {
	att, data, err := models.AttachmentContent(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	ctx.Header("Content-Type", att.ContentType)
	// Sempre como anexo: evita renderizar HTML/SVG enviado por terceiros no nosso domínio.
	ctx.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(att.Filename, `"`, "")))
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Write(data)
}

func DeleteAttachment(ctx iris.Context) {
	id := paramID(ctx)
	att, err := models.AttachmentByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteAttachment(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "anexo", id,
		fmt.Sprintf("removeu o arquivo %s do %s #%d", att.Filename, att.Entity, att.EntityID))
	ctx.JSON(iris.Map{"message": "arquivo removido"})
}

// entityRef devolve o ID quando a entidade do anexo é a esperada (para a timeline).
func entityRef(entity, want string, id int64) *int64 {
	if entity != want {
		return nil
	}
	return &id
}

// safeFilename remove diretórios do nome enviado pelo navegador.
func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "arquivo"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

// contentTypeOf confia na extensão para os tipos comuns e cai no genérico no resto.
func contentTypeOf(declared, filename string) string {
	declared = strings.TrimSpace(strings.ToLower(declared))
	if declared != "" && declared != "application/octet-stream" {
		// Nunca devolvemos HTML/SVG como tal: seriam executados no navegador.
		if strings.Contains(declared, "html") || strings.Contains(declared, "svg") ||
			strings.Contains(declared, "javascript") {
			return "application/octet-stream"
		}
		return declared
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".csv":
		return "text/csv"
	case ".txt":
		return "text/plain"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	}
	return "application/octet-stream"
}
