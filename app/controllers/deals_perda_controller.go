package controllers

import "github.com/kataras/iris/v12"

// Motivos de perda de negócio. Contrato: docs/PROSPECCAO.md seção 9, com as
// correções de docs/PROSPECCAO-ADENDO.md. A gravação do motivo no fechamento e
// na mudança de etapa fica em deals_controller.go (CloseDealHandler e MoveDeal).
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// GetMotivosPerda: GET /settings/motivos-perda → { motivos, exigir } (só autenticação).
func GetMotivosPerda(ctx iris.Context) { naoImplementado(ctx) }

// UpdateMotivosPerda: PUT /settings/motivos-perda (settings.pipelines).
func UpdateMotivosPerda(ctx iris.Context) { naoImplementado(ctx) }
