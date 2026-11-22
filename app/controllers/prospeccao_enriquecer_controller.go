package controllers

import "github.com/kataras/iris/v12"

// Prospecção: enriquecimento em lote pelo Who Lead. Contrato:
// docs/PROSPECCAO.md seção 7, com as correções de docs/PROSPECCAO-ADENDO.md.
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// EnriquecerProspEstimativa: POST /prospeccao/listas/{id}/enriquecer/estimativa { selecao, reprocessar? } → EnriquecerEstimativa.
func EnriquecerProspEstimativa(ctx iris.Context) { naoImplementado(ctx) }

// EnriquecerProspLista: POST /prospeccao/listas/{id}/enriquecer { selecao, reprocessar?, custo_confirmado_centavos } → 202 { job }.
func EnriquecerProspLista(ctx iris.Context) { naoImplementado(ctx) }
