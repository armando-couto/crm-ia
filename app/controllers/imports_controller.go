package controllers

import (
	"fmt"
	"io"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// maxImportFileSize limita o upload de planilhas (10 MB cobre as cargas usuais).
const maxImportFileSize = 10 << 20

// ListImports devolve o histórico de importações de arquivos.
func ListImports(ctx iris.Context) {
	list, err := models.ListImports(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": list})
}

// RunImportHandler recebe a planilha (CSV/XLSX), casa as colunas pelo
// cabeçalho e cria OU atualiza os registros — contato pelo e-mail, empresa
// pelo CNPJ ou nome — gravando a carga no histórico.
func RunImportHandler(ctx iris.Context) {
	entity := ctx.FormValueDefault("entity", "contatos")
	if entity != "contatos" && entity != "empresas" {
		badRequest(ctx, "entidade inválida: use contatos ou empresas")
		return
	}

	file, header, err := ctx.FormFile("file")
	if err != nil {
		badRequest(ctx, "envie o arquivo no campo 'file'")
		return
	}
	defer file.Close()
	if header.Size > maxImportFileSize {
		badRequest(ctx, "arquivo acima de 10 MB")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxImportFileSize+1))
	if err != nil {
		serverError(ctx, err)
		return
	}
	if len(data) > maxImportFileSize {
		badRequest(ctx, "arquivo acima de 10 MB")
		return
	}

	headers, rows, err := services.ParseImportFile(header.Filename, data)
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}

	var createdBy *int64
	if claims := middlewareClaims(ctx); claims != nil {
		createdBy = &claims.UserID
	}

	record, err := services.RunImport(utils.DB, entity, header.Filename, headers, rows, createdBy)
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}

	audit(ctx, models.AuditImport, entity, record.ID,
		fmt.Sprintf("importou %s (%d novos, %d atualizados, %d associações, %d erros)",
			record.FileName, record.NewRecords, record.UpdatedRecords,
			record.NewAssociations, record.ErrorCount))
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(record)
}
