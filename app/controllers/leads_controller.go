package controllers

import "github.com/kataras/iris/v12"

// Leads: caixa de leads (listar, criar, editar, ações em massa, converter,
// descartar, reabrir). Contrato: docs/PROSPECCAO.md seção 8.2, com as correções
// de docs/PROSPECCAO-ADENDO.md (visibilidade por dono: sem leads.assign só se
// enxerga o próprio lead, o sem dono e o de dono inativo; fora disso, 404).
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// ListLeads: GET /leads → { data, pagination, contadores }.
func ListLeads(ctx iris.Context) { naoImplementado(ctx) }

// CreateLead: POST /leads → 201 Lead.
func CreateLead(ctx iris.Context) { naoImplementado(ctx) }

// ListLeadListas: GET /leads/listas → { id, nome, abertos }[] (listas com leads visíveis ao usuário).
func ListLeadListas(ctx iris.Context) { naoImplementado(ctx) }

// GetLead: GET /leads/{id} → LeadCompleto (marca como visto; renova a trava de quem a detém).
func GetLead(ctx iris.Context) { naoImplementado(ctx) }

// UpdateLead: PUT /leads/{id} (parcial) → Lead.
func UpdateLead(ctx iris.Context) { naoImplementado(ctx) }

// DeleteLead: DELETE /leads/{id} → 204 (exige leads.assign).
func DeleteLead(ctx iris.Context) { naoImplementado(ctx) }

// BulkLeads: POST /leads/bulk { ids, acao, ... } → { afetados } (acao "dono" exige leads.assign, conferido aqui).
func BulkLeads(ctx iris.Context) { naoImplementado(ctx) }

// ConverterLead: POST /leads/{id}/converter → { lead, deal }.
func ConverterLead(ctx iris.Context) { naoImplementado(ctx) }

// DescartarLead: POST /leads/{id}/descartar { motivo, observacao?, bloquear? } → Lead.
func DescartarLead(ctx iris.Context) { naoImplementado(ctx) }

// ReabrirLead: POST /leads/{id}/reabrir → Lead.
func ReabrirLead(ctx iris.Context) { naoImplementado(ctx) }
