package controllers

import "github.com/kataras/iris/v12"

// Configurações de prospecção (janela de contato, cadência, meta, destino da
// conversão, motivos de descarte). Contrato: docs/PROSPECCAO.md seção 8.4.
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// GetConfigProspeccao: GET /settings/prospeccao → ConfigProspeccao (qualquer um com leads.view).
func GetConfigProspeccao(ctx iris.Context) { naoImplementado(ctx) }

// UpdateConfigProspeccao: PUT /settings/prospeccao (prospecting.manage).
func UpdateConfigProspeccao(ctx iris.Context) { naoImplementado(ctx) }
