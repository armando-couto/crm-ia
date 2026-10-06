package models

import "testing"

func TestPaginationNormalize(t *testing.T) {
	cases := []struct {
		in      Pagination
		page    int
		perPage int
		offset  int
	}{
		{Pagination{Page: 0, PerPage: 0}, 1, 25, 0},
		{Pagination{Page: -3, PerPage: -1}, 1, 25, 0},
		{Pagination{Page: 2, PerPage: 10}, 2, 10, 10},
		{Pagination{Page: 3, PerPage: 500}, 3, 100, 200},
	}
	for _, c := range cases {
		c.in.Normalize()
		if c.in.Page != c.page || c.in.PerPage != c.perPage || c.in.Offset() != c.offset {
			t.Errorf("Normalize(%+v): page=%d per_page=%d offset=%d, esperado %d/%d/%d",
				c.in, c.in.Page, c.in.PerPage, c.in.Offset(), c.page, c.perPage, c.offset)
		}
	}
}

func TestValidLifecycleStage(t *testing.T) {
	for _, s := range LifecycleStages {
		if !ValidLifecycleStage(s) {
			t.Errorf("%q deveria ser válido", s)
		}
	}
	if ValidLifecycleStage("outro") || ValidLifecycleStage("") {
		t.Error("estágios desconhecidos não podem ser válidos")
	}
}
