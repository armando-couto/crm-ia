package controllers_test

import (
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func TestUploadAttachment(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("INSERT INTO attachments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(4, time.Now()))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(11, time.Now()))
	mock.ExpectExec("INSERT INTO audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resp := e.POST("/api/v1/attachments").
		WithHeader("Authorization", "Bearer "+token).
		WithMultipart().
		WithFormField("entity", "contato").
		WithFormField("entity_id", "7").
		WithFileBytes("file", "proposta.pdf", []byte("%PDF-1.4 conteudo")).
		Expect().Status(iris.StatusCreated).JSON().Object()

	resp.Value("filename").IsEqual("proposta.pdf")
	resp.Value("content_type").IsEqual("application/pdf")
	resp.Value("size_bytes").IsEqual(17)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUploadAttachmentRejectsUnknownEntity(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/attachments").
		WithHeader("Authorization", "Bearer "+token).
		WithMultipart().
		WithFormField("entity", "planeta").
		WithFormField("entity_id", "1").
		WithFileBytes("file", "x.txt", []byte("oi")).
		Expect().Status(iris.StatusBadRequest)
}

func TestUploadAttachmentRejectsEmptyFile(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/attachments").
		WithHeader("Authorization", "Bearer "+token).
		WithMultipart().
		WithFormField("entity", "negocio").
		WithFormField("entity_id", "1").
		WithFileBytes("file", "vazio.txt", []byte{}).
		Expect().Status(iris.StatusBadRequest)
}

func TestListAttachments(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("empresa", int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "entity", "entity_id", "filename", "content_type", "size_bytes",
			"uploaded_by", "uploader_name", "created_at",
		}).AddRow(1, "empresa", 3, "contrato.pdf", "application/pdf", 2048,
			int64(1), "Ana", time.Now()))

	e.GET("/api/v1/attachments").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("entity", "empresa").WithQuery("entity_id", 3).
		Expect().Status(iris.StatusOK).
		JSON().Array().Value(0).Object().Value("filename").IsEqual("contrato.pdf")
}

// O download vai sempre como anexo, nunca renderizado no navegador.
func TestDownloadAttachment(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT (.+) FROM attachments WHERE id").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "entity", "entity_id", "filename", "content_type", "size_bytes", "created_at", "data",
		}).AddRow(9, "contato", 1, "notas.txt", "text/plain", 5, time.Now(), []byte("olá!")))

	resp := e.GET("/api/v1/attachments/9/download").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK)

	resp.Header("Content-Disposition").Contains("attachment")
	resp.Header("Content-Disposition").Contains("notas.txt")
	resp.Header("X-Content-Type-Options").IsEqual("nosniff")
	resp.Body().IsEqual("olá!")
}

func TestDeleteAttachment(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT (.+) FROM attachments WHERE id").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "entity", "entity_id", "filename", "content_type", "size_bytes", "uploaded_by", "created_at",
		}).AddRow(5, "negocio", 2, "old.pdf", "application/pdf", 10, nil, time.Now()))
	mock.ExpectExec("DELETE FROM attachments").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	e.DELETE("/api/v1/attachments/5").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Seller enxerga anexo, mas o perfil sem files.manage não pode remover.
func TestAttachmentPermissions(t *testing.T) {
	e, _, _ := newTestApp(t)

	// Perfil sem nenhuma permissão de arquivo: usamos um papel válido e a matriz
	// padrão do Seller (que tem files.view e files.manage) como contraste.
	if !models.RoleCan(models.RoleSeller, models.PermFilesManage) {
		t.Fatal("por padrão o Seller deveria poder anexar arquivos")
	}
	if models.RoleCan("desconhecido", models.PermFilesManage) {
		t.Fatal("papel desconhecido não pode anexar")
	}

	token := tokenFor(t, &models.User{ID: 30, Email: "x@exemplo.com.br", Role: "desconhecido"})
	e.DELETE("/api/v1/attachments/1").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}

// Nomes com caminho e tipos executáveis são normalizados no upload.
func TestUploadAttachmentSanitizesName(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("INSERT INTO attachments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(6, time.Now()))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(12, time.Now()))
	mock.ExpectExec("INSERT INTO audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))

	resp := e.POST("/api/v1/attachments").
		WithHeader("Authorization", "Bearer "+token).
		WithMultipart().
		WithFormField("entity", "ticket").
		WithFormField("entity_id", "2").
		WithFileBytes("file", "pagina.html", []byte("<script>alert(1)</script>")).
		Expect().Status(iris.StatusCreated).JSON().Object()

	name := resp.Value("filename").String().Raw()
	if strings.Contains(name, "/") {
		t.Fatalf("nome deveria vir sem caminho: %q", name)
	}
	// HTML nunca volta como text/html: seria executado no nosso domínio.
	resp.Value("content_type").IsEqual("application/octet-stream")
}
