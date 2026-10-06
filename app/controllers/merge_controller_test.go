package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func TestListDuplicateContacts(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	rows := func(key string, ids ...int64) *sqlmock.Rows {
		r := sqlmock.NewRows([]string{
			"chave", "id", "first_name", "last_name", "email", "phone",
			"owner_name", "created_at", "deals",
		})
		for _, id := range ids {
			r.AddRow(key, id, "Ana", "Silva", key, "", "Dono", time.Now(), 2)
		}
		return r
	}

	// Um SELECT por critério: e-mail, telefone e nome.
	mock.ExpectQuery("FROM contacts").WillReturnRows(rows("ana@exemplo.com.br", 1, 2))
	mock.ExpectQuery("FROM contacts").WillReturnRows(rows("", 0))
	mock.ExpectQuery("FROM contacts").WillReturnRows(rows("", 0))

	resp := e.GET("/api/v1/duplicates").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("entity").IsEqual("contacts")
	groups := resp.Value("groups").Array()
	groups.Length().IsEqual(1)
	groups.Value(0).Object().Value("reason").IsEqual("mesmo e-mail")
	groups.Value(0).Object().Value("records").Array().Length().IsEqual(2)
}

func TestMergeRejectsSameRecord(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/duplicates/merge").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"entity": "contacts", "primary_id": 5, "duplicate_id": 5}).
		Expect().Status(iris.StatusBadRequest)
}

func TestMergeRejectsUnknownEntity(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/duplicates/merge").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"entity": "planetas", "primary_id": 1, "duplicate_id": 2}).
		Expect().Status(iris.StatusBadRequest)
}

// Mesclar é destrutivo: fica com o Manager e o Admin, não com o Seller.
func TestMergeForbiddenForSeller(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 40, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.POST("/api/v1/duplicates/merge").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"entity": "contacts", "primary_id": 1, "duplicate_id": 2}).
		Expect().Status(iris.StatusForbidden)

	if models.RoleCan(models.RoleSeller, models.PermRecordsMerge) {
		t.Fatal("Seller não deveria mesclar registros por padrão")
	}
	if !models.RoleCan(models.RoleManager, models.PermRecordsMerge) {
		t.Fatal("Manager deveria poder mesclar registros")
	}
}
