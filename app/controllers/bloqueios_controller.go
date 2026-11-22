package controllers

import "github.com/kataras/iris/v12"

// Bloqueios ("não contatar") por telefone, e-mail e CNPJ. Contrato:
// docs/PROSPECCAO.md seção 8.3, com as correções de docs/PROSPECCAO-ADENDO.md.
// Leitura: prospecting.view ou leads.view; escrita: prospecting.manage.
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// ListBloqueios: GET /bloqueios → { data, pagination }.
func ListBloqueios(ctx iris.Context) { naoImplementado(ctx) }

// CreateBloqueio: POST /bloqueios { tipo, valor, motivo, observacao? }.
func CreateBloqueio(ctx iris.Context) { naoImplementado(ctx) }

// ImportarBloqueios: POST /bloqueios/importar { tipo, motivo, valores } → { criados, repetidos, invalidos }.
func ImportarBloqueios(ctx iris.Context) { naoImplementado(ctx) }

// DeleteBloqueio: DELETE /bloqueios/{id}.
func DeleteBloqueio(ctx iris.Context) { naoImplementado(ctx) }
