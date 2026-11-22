package controllers

import "github.com/kataras/iris/v12"

// Prospecção: plataforma (resumo, catálogos, contagem, prévia), segmentos,
// listas, itens e jobs. Contrato: docs/PROSPECCAO.md seções 6.1 a 6.3, com as
// correções de docs/PROSPECCAO-ADENDO.md. Leitura exige prospecting.view;
// escrita, prospecting.manage (ver routes/routes.go).
//
// Rotas já registradas em routes/routes.go; cada handler abaixo responde 501
// até ser implementado (naoImplementado, em prospeccao_stub.go).

// ProspeccaoResumo: GET /prospeccao → ResumoPlataforma (habilitada = false com motivo sem painel).
func ProspeccaoResumo(ctx iris.Context) { naoImplementado(ctx) }

// ProspeccaoCatalogo: GET /prospeccao/catalogos/{nome} → repasse (cnaes | municipios | naturezas | ufs; cache de 1 h).
func ProspeccaoCatalogo(ctx iris.Context) { naoImplementado(ctx) }

// ProspeccaoContagem: POST /prospeccao/contagem { filtros, segmento_id? } → Contagem.
func ProspeccaoContagem(ctx iris.Context) { naoImplementado(ctx) }

// ProspeccaoPrevia: POST /prospeccao/previa { filtros, ordem? } → { itens, ordem_aplicada }.
func ProspeccaoPrevia(ctx iris.Context) { naoImplementado(ctx) }

// ListProspSegmentos: GET /prospeccao/segmentos → Segmento[].
func ListProspSegmentos(ctx iris.Context) { naoImplementado(ctx) }

// CreateProspSegmento: POST /prospeccao/segmentos { nome, filtros, ordem, compartilhado }.
func CreateProspSegmento(ctx iris.Context) { naoImplementado(ctx) }

// UpdateProspSegmento: PUT /prospeccao/segmentos/{id}.
func UpdateProspSegmento(ctx iris.Context) { naoImplementado(ctx) }

// DeleteProspSegmento: DELETE /prospeccao/segmentos/{id}.
func DeleteProspSegmento(ctx iris.Context) { naoImplementado(ctx) }

// ListProspListas: GET /prospeccao/listas → { data, pagination }.
func ListProspListas(ctx iris.Context) { naoImplementado(ctx) }

// CreateProspLista: POST /prospeccao/listas (CriarLista) → 201 { lista, job }.
func CreateProspLista(ctx iris.Context) { naoImplementado(ctx) }

// GetProspLista: GET /prospeccao/listas/{id} → { lista, jobs }.
func GetProspLista(ctx iris.Context) { naoImplementado(ctx) }

// UpdateProspLista: PUT /prospeccao/listas/{id} { nome?, descricao?, playbook_id?, pipeline_id?, stage_id? }.
func UpdateProspLista(ctx iris.Context) { naoImplementado(ctx) }

// DeleteProspLista: DELETE /prospeccao/listas/{id} → 204 (409 job_ativo com job em andamento).
func DeleteProspLista(ctx iris.Context) { naoImplementado(ctx) }

// ListProspItens: GET /prospeccao/listas/{id}/itens → { data, pagination }.
func ListProspItens(ctx iris.Context) { naoImplementado(ctx) }

// GetProspItem: GET /prospeccao/itens/{id} → Item com dados e enriquecimento.
func GetProspItem(ctx iris.Context) { naoImplementado(ctx) }

// DescartarProspItens: POST /prospeccao/listas/{id}/itens/descartar { selecao } → { afetados }.
func DescartarProspItens(ctx iris.Context) { naoImplementado(ctx) }

// RestaurarProspItens: POST /prospeccao/listas/{id}/itens/restaurar { selecao } → { afetados }.
func RestaurarProspItens(ctx iris.Context) { naoImplementado(ctx) }

// ExportarProspLista: GET /prospeccao/listas/{id}/exportar → CSV (; e UTF-8 com BOM), auditado.
func ExportarProspLista(ctx iris.Context) { naoImplementado(ctx) }

// GetProspJob: GET /prospeccao/jobs/{id} → Job.
func GetProspJob(ctx iris.Context) { naoImplementado(ctx) }

// CancelarProspJob: POST /prospeccao/jobs/{id}/cancelar → Job.
func CancelarProspJob(ctx iris.Context) { naoImplementado(ctx) }
