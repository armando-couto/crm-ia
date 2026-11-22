package controllers

import "github.com/kataras/iris/v12"

// Prospecção: envio dos itens ao CRM (empresa, contatos e lead, com
// distribuição entre vendedores). Contrato: docs/PROSPECCAO.md seção 8.1, com
// as correções de docs/PROSPECCAO-ADENDO.md.
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// EnviarProspPrevia: POST /prospeccao/listas/{id}/enviar/previa (EnviarRequest) → EnviarPrevia.
func EnviarProspPrevia(ctx iris.Context) { naoImplementado(ctx) }

// EnviarProspLista: POST /prospeccao/listas/{id}/enviar (EnviarRequest) → 202 { job }.
func EnviarProspLista(ctx iris.Context) { naoImplementado(ctx) }
