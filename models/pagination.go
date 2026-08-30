package models

// Pagination padroniza paginação nas listagens do CRM.
type Pagination struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

// Normalize garante valores seguros de página e limite (máx. 100 por página).
func (p *Pagination) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PerPage < 1 {
		p.PerPage = 25
	}
	if p.PerPage > 100 {
		p.PerPage = 100
	}
}

// Offset retorna o deslocamento SQL correspondente à página atual.
func (p *Pagination) Offset() int {
	return (p.Page - 1) * p.PerPage
}
