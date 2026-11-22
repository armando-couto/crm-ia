package controllers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Prospecção: plataforma (resumo, catálogos, contagem, prévia), segmentos,
// listas, itens e jobs. Contrato: docs/PROSPECCAO.md seções 6.1 a 6.3, com as
// correções de docs/PROSPECCAO-ADENDO.md. Leitura exige prospecting.view;
// escrita, prospecting.manage (ver routes/routes.go).
//
// O controller só fala com a plataforma pelo cliente de services
// (plataforma.go) e nunca executa trabalho longo: gerar a lista é um job
// (services/prospeccao_lista.go) que o worker roda em segundo plano.

// -----------------------------------------------------------------------------
// Respostas comuns
// -----------------------------------------------------------------------------

// prospCtlErro responde um erro da plataforma com o status e o corpo já
// normalizados ({"error","codigo"} e, conforme o caso, cota ou
// tentar_em_segundos). Devolve false se err não é da plataforma.
func prospCtlErro(ctx iris.Context, err error) bool {
	e, ok := services.ComoErroPlataforma(err)
	if !ok {
		return false
	}
	if e.TentarEmSegundos > 0 {
		ctx.Header("Retry-After", strconv.Itoa(e.TentarEmSegundos))
	}
	ctx.StopWithJSON(e.Status, e.Corpo())
	return true
}

// prospCtlFalha responde err: erro da plataforma, 404 ou 500.
func prospCtlFalha(ctx iris.Context, err error) {
	if prospCtlErro(ctx, err) {
		return
	}
	handleDBError(ctx, err)
}

// prospCtlInvalido responde 422 com codigo filtro_invalido.
func prospCtlInvalido(ctx iris.Context, msg string) {
	ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": msg, "codigo": services.CodigoFiltroInvalido})
}

// prospCtlJobAtivo responde 409 {error, codigo: job_ativo, job} com o job em
// andamento na lista.
func prospCtlJobAtivo(ctx iris.Context, listaID int64) {
	corpo := iris.Map{"error": models.ErrJobAtivo.Error(), "codigo": services.CodigoJobAtivo}
	if job, err := models.ProspJobAtivoDaLista(utils.DB, listaID); err == nil && job != nil {
		corpo["job"] = job
	}
	ctx.StopWithJSON(iris.StatusConflict, corpo)
}

// prospCtlListaSemJob carrega a lista e confere que não há job ativo nela
// (descartar, restaurar e mudar o destino respondem 409 com job ativo).
func prospCtlListaSemJob(ctx iris.Context, id int64) (*models.ProspLista, bool) {
	lista, err := models.ProspListaByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return nil, false
	}
	job, err := models.ProspJobAtivoDaLista(utils.DB, id)
	if err != nil && err != sql.ErrNoRows {
		serverError(ctx, err)
		return nil, false
	}
	if job != nil {
		ctx.StopWithJSON(iris.StatusConflict, iris.Map{
			"error": models.ErrJobAtivo.Error(), "codigo": services.CodigoJobAtivo, "job": job,
		})
		return nil, false
	}
	return lista, true
}

// prospCtlUsuario devolve id e nome de quem chama (0 sem sessão).
func prospCtlUsuario(ctx iris.Context) (int64, string) {
	if c := middlewareClaims(ctx); c != nil {
		return c.UserID, c.Name
	}
	return 0, ""
}

// prospCtlAuditar grava na trilha com detalhes em JSON. Falha só vai para o log.
func prospCtlAuditar(ctx iris.Context, acao, entidade string, id int64, resumo string, detalhes any) {
	entrada := models.AuditEntry{Action: acao, Entity: entidade, Summary: resumo, IP: clientIP(ctx)}
	if id > 0 {
		entrada.EntityID = &id
	}
	if uid, nome := prospCtlUsuario(ctx); uid > 0 {
		entrada.UserID, entrada.UserName = &uid, nome
	}
	if detalhes != nil {
		if b, err := json.Marshal(detalhes); err == nil {
			entrada.Details = b
		}
	}
	if err := models.RecordAudit(utils.DB, &entrada); err != nil {
		ctx.Application().Logger().Errorf("falha ao registrar auditoria: %v", err)
	}
}

// -----------------------------------------------------------------------------
// 6.1 Plataforma
// -----------------------------------------------------------------------------

// ProspeccaoResumo: GET /prospeccao → ResumoPlataforma (habilitada = false com motivo sem painel).
//
// Falha da plataforma não é erro da tela: volta habilitada = false com o motivo
// (a tela mostra só o motivo e tenta de novo depois).
func ProspeccaoResumo(ctx iris.Context) {
	r, err := services.PlataformaResumoComVersao(ctx.Request().Context())
	if err != nil {
		motivo := err.Error()
		if e, ok := services.ComoErroPlataforma(err); ok {
			motivo = e.Mensagem
		}
		ctx.JSON(&models.ProspResumoPlataforma{Motivo: motivo})
		return
	}
	if !r.Habilitada && r.Motivo == "" {
		r.Motivo = "a prospecção não está disponível neste ambiente"
	}
	ctx.JSON(r)
}

// ProspeccaoCatalogo: GET /prospeccao/catalogos/{nome} → repasse (cnaes | municipios | naturezas | ufs; cache de 1 h).
func ProspeccaoCatalogo(ctx iris.Context) {
	nome := ctx.Params().Get("nome")
	valido := false
	for _, n := range services.CatalogosPlataforma {
		valido = valido || n == nome
	}
	if !valido {
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "catálogo desconhecido", "codigo": services.CodigoNaoEncontrado})
		return
	}
	out, err := services.PlataformaCatalogoComCache(ctx.Request().Context(), nome, ctx.Request().URL.Query())
	if err != nil {
		prospCtlFalha(ctx, err)
		return
	}
	ctx.ContentType("application/json")
	_, _ = ctx.Write(out)
}

// ProspeccaoContagem: POST /prospeccao/contagem { filtros, segmento_id? } → Contagem.
func ProspeccaoContagem(ctx iris.Context) {
	var req struct {
		Filtros    models.ProspFiltros `json:"filtros"`
		SegmentoID *int64              `json:"segmento_id"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	req.Filtros.Normalizar()
	if err := req.Filtros.Validar(); err != nil {
		prospCtlInvalido(ctx, err.Error())
		return
	}
	c, err := services.PlataformaContagemComCache(ctx.Request().Context(), req.Filtros)
	if err != nil {
		prospCtlFalha(ctx, err)
		return
	}
	if req.SegmentoID != nil && *req.SegmentoID > 0 {
		if err := models.RegistrarContagemProspSegmento(utils.DB, *req.SegmentoID, c.Total); err != nil {
			ctx.Application().Logger().Errorf("prospecção: falha ao gravar a contagem do segmento %d: %v", *req.SegmentoID, err)
		}
	}
	ctx.JSON(c)
}

// ProspeccaoPrevia: POST /prospeccao/previa { filtros, ordem? } → { itens, ordem_aplicada }.
func ProspeccaoPrevia(ctx iris.Context) {
	var req struct {
		Filtros models.ProspFiltros `json:"filtros"`
		Ordem   string              `json:"ordem"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	if !models.ProspOrdemValida(req.Ordem) {
		prospCtlInvalido(ctx, "ordenação inválida")
		return
	}
	if req.Ordem == "" {
		req.Ordem = models.OrdemRelevancia
	}
	req.Filtros.Normalizar()
	if err := req.Filtros.Validar(); err != nil {
		prospCtlInvalido(ctx, err.Error())
		return
	}
	p, err := services.PlataformaPrevia(ctx.Request().Context(), req.Filtros, req.Ordem, 0)
	if err != nil {
		prospCtlFalha(ctx, err)
		return
	}
	if p.Itens == nil {
		p.Itens = []models.ProspEmpresaPrevia{}
	}
	ctx.JSON(iris.Map{"itens": p.Itens, "ordem_aplicada": p.OrdemAplicada})
}

// -----------------------------------------------------------------------------
// 6.2 Segmentos
// -----------------------------------------------------------------------------

type prospSegmentoReq struct {
	Nome          string              `json:"nome"`
	Filtros       models.ProspFiltros `json:"filtros"`
	Ordem         string              `json:"ordem"`
	Compartilhado bool                `json:"compartilhado"`
}

func (r prospSegmentoReq) segmento() (*models.ProspSegmento, error) {
	s := &models.ProspSegmento{Nome: r.Nome, Filtros: r.Filtros, Ordem: r.Ordem, Compartilhado: r.Compartilhado}
	if err := s.Validar(); err != nil {
		return nil, err
	}
	if err := s.Filtros.Validar(); err != nil {
		return nil, err
	}
	return s, nil
}

// prospCtlSegmentoVisivel carrega o segmento se quem chama pode vê-lo
// (compartilhado ou dele); senão responde 404.
func prospCtlSegmentoVisivel(ctx iris.Context, id int64) (*models.ProspSegmento, bool) {
	s, err := models.ProspSegmentoByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return nil, false
	}
	uid, _ := prospCtlUsuario(ctx)
	if !s.Compartilhado && (s.CriadoPor == nil || *s.CriadoPor != uid) {
		notFound(ctx)
		return nil, false
	}
	return s, true
}

// ListProspSegmentos: GET /prospeccao/segmentos → Segmento[].
func ListProspSegmentos(ctx iris.Context) {
	uid, _ := prospCtlUsuario(ctx)
	lista, err := models.ListProspSegmentos(utils.DB, uid)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(lista)
}

// CreateProspSegmento: POST /prospeccao/segmentos { nome, filtros, ordem, compartilhado }.
func CreateProspSegmento(ctx iris.Context) {
	var req prospSegmentoReq
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	s, err := req.segmento()
	if err != nil {
		prospCtlInvalido(ctx, err.Error())
		return
	}
	uid, nome := prospCtlUsuario(ctx)
	if uid > 0 {
		s.CriadoPor, s.CriadoPorNome = &uid, nome
	}
	if err := models.CreateProspSegmento(utils.DB, s); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, "segmento_prospeccao", s.ID, fmt.Sprintf("criou o segmento de prospecção %q", s.Nome))
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(s)
}

// UpdateProspSegmento: PUT /prospeccao/segmentos/{id}.
func UpdateProspSegmento(ctx iris.Context) {
	atual, ok := prospCtlSegmentoVisivel(ctx, paramID(ctx))
	if !ok {
		return
	}
	var req prospSegmentoReq
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	s, err := req.segmento()
	if err != nil {
		prospCtlInvalido(ctx, err.Error())
		return
	}
	s.ID, s.CriadoPor, s.CriadoPorNome, s.CriadoEm = atual.ID, atual.CriadoPor, atual.CriadoPorNome, atual.CriadoEm
	if err := models.UpdateProspSegmento(utils.DB, s); err != nil {
		handleDBError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "segmento_prospeccao", s.ID, fmt.Sprintf("editou o segmento de prospecção %q", s.Nome))
	ctx.JSON(s)
}

// DeleteProspSegmento: DELETE /prospeccao/segmentos/{id}.
func DeleteProspSegmento(ctx iris.Context) {
	s, ok := prospCtlSegmentoVisivel(ctx, paramID(ctx))
	if !ok {
		return
	}
	if err := models.DeleteProspSegmento(utils.DB, s.ID); err != nil {
		handleDBError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "segmento_prospeccao", s.ID, fmt.Sprintf("excluiu o segmento de prospecção %q", s.Nome))
	ctx.StatusCode(iris.StatusNoContent)
}

// -----------------------------------------------------------------------------
// 6.3 Listas
// -----------------------------------------------------------------------------

// ListProspListas: GET /prospeccao/listas → { data, pagination }.
func ListProspListas(ctx iris.Context) {
	f := models.ProspListaFilter{
		Q:      ctx.URLParam("q"),
		Status: ctx.URLParam("status"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	switch f.Status {
	case "", models.ListaGerando, models.ListaPronta, models.ListaFalhou:
	default:
		badRequest(ctx, "status inválido")
		return
	}
	listas, total, err := models.ListProspListas(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Normalize()
	f.Total = total
	ctx.JSON(iris.Map{"data": listas, "pagination": f.Pagination})
}

// prospCriarListaReq é o corpo CriarLista.
type prospCriarListaReq struct {
	Nome            string              `json:"nome"`
	Descricao       string              `json:"descricao"`
	Origem          string              `json:"origem"`
	Filtros         models.ProspFiltros `json:"filtros"`
	Ordem           string              `json:"ordem"`
	Quantidade      int                 `json:"quantidade"`
	ExcluirNoCRM    *bool               `json:"excluir_no_crm"`
	ExcluirEmListas *bool               `json:"excluir_em_listas"`
	SegmentoID      *int64              `json:"segmento_id"`
}

// CreateProspLista: POST /prospeccao/listas (CriarLista) → 201 { lista, job }.
//
// Só grava a lista ("gerando") e o job gerar_lista; quem busca, desconta a
// cota e grava os itens é o worker. A cota é conferida aqui só para recusar
// cedo (429 com a cota) quando já não resta nada.
func CreateProspLista(ctx iris.Context) {
	var req prospCriarListaReq
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	lista := &models.ProspLista{
		Nome: req.Nome, Descricao: req.Descricao, Origem: req.Origem, Filtros: req.Filtros, Ordem: req.Ordem,
		QuantidadePedida: req.Quantidade, ExcluirNoCRM: true, ExcluirEmListas: true,
	}
	if req.ExcluirNoCRM != nil {
		lista.ExcluirNoCRM = *req.ExcluirNoCRM
	}
	if req.ExcluirEmListas != nil {
		lista.ExcluirEmListas = *req.ExcluirEmListas
	}
	if err := lista.Validar(); err != nil {
		prospCtlInvalido(ctx, err.Error())
		return
	}
	if req.SegmentoID != nil && *req.SegmentoID > 0 {
		if _, err := models.ProspSegmentoByID(utils.DB, *req.SegmentoID); err == nil {
			lista.SegmentoID = req.SegmentoID
		} else if err != sql.ErrNoRows {
			serverError(ctx, err)
			return
		}
	}

	resumo, err := services.PlataformaResumoComVersao(ctx.Request().Context())
	if err != nil {
		prospCtlFalha(ctx, err)
		return
	}
	if !resumo.Habilitada {
		motivo := resumo.Motivo
		if motivo == "" {
			motivo = "a prospecção não está disponível neste ambiente"
		}
		ctx.StopWithJSON(iris.StatusServiceUnavailable, iris.Map{"error": motivo, "codigo": services.CodigoPlataforma})
		return
	}
	if resumo.Cota.Restante <= 0 {
		ctx.StopWithJSON(iris.StatusTooManyRequests, iris.Map{
			"error": "cota mensal de prospecção esgotada", "codigo": services.CodigoCotaEsgotada, "cota": resumo.Cota,
		})
		return
	}
	lista.VersaoBase = resumo.Base.Versao

	uid, _ := prospCtlUsuario(ctx)
	if uid > 0 {
		lista.CriadoPor = &uid
	}
	job := &models.ProspJob{Tipo: models.JobGerarLista, Total: lista.QuantidadePedida, Mensagem: "na fila para gerar"}
	if err := models.CreateProspListaComJob(utils.DB, lista, job); err != nil {
		serverError(ctx, err)
		return
	}
	detalhes := map[string]any{
		"origem": lista.Origem, "quantidade": lista.QuantidadePedida, "incluir_pf": lista.IncluirPF,
		"excluir_no_crm": lista.ExcluirNoCRM, "excluir_em_listas": lista.ExcluirEmListas,
	}
	resumoAud := fmt.Sprintf("pediu a lista de prospecção %q com %d empresas", lista.Nome, lista.QuantidadePedida)
	if lista.IncluirPF {
		resumoAud += " (incluindo pessoa física)"
	}
	prospCtlAuditar(ctx, models.AuditCreate, services.EntidadeListaProspeccao, lista.ID, resumoAud, detalhes)

	if l, err := models.ProspListaByID(utils.DB, lista.ID); err == nil {
		lista = l
	}
	if j, err := models.ProspJobByID(utils.DB, job.ID); err == nil {
		job = j
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(iris.Map{"lista": lista, "job": job})
}

// GetProspLista: GET /prospeccao/listas/{id} → { lista, jobs }.
func GetProspLista(ctx iris.Context) {
	lista, err := models.ProspListaByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	jobs, err := models.ProspJobsDaLista(utils.DB, lista.ID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if jobs == nil {
		jobs = []models.ProspJob{}
	}
	ctx.JSON(iris.Map{"lista": lista, "jobs": jobs})
}

// prospCtlLerID lê um id opcional do corpo parcial: ausente = não mexe;
// null ou 0 = grava nulo.
func prospCtlLerID(bruto map[string]json.RawMessage, chave string) (models.OpcionalID, error) {
	v, ok := bruto[chave]
	if !ok {
		return models.OpcionalID{}, nil
	}
	if string(v) == "null" {
		return models.DefinirIDPtr(nil), nil
	}
	var id int64
	if err := json.Unmarshal(v, &id); err != nil {
		return models.OpcionalID{}, fmt.Errorf("%s inválido", chave)
	}
	return models.DefinirID(id), nil
}

// UpdateProspLista: PUT /prospeccao/listas/{id} { nome?, descricao?, playbook_id?, pipeline_id?, stage_id? }.
func UpdateProspLista(ctx iris.Context) {
	id := paramID(ctx)
	var bruto map[string]json.RawMessage
	if err := ctx.ReadJSON(&bruto); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	var p models.ProspListaPatch
	for chave, alvo := range map[string]**string{"nome": &p.Nome, "descricao": &p.Descricao} {
		if v, ok := bruto[chave]; ok && string(v) != "null" {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				prospCtlInvalido(ctx, chave+" inválido")
				return
			}
			*alvo = &s
		}
	}
	if p.Nome != nil && strings.TrimSpace(*p.Nome) == "" {
		prospCtlInvalido(ctx, "informe o nome da lista")
		return
	}
	var err error
	if p.PlaybookID, err = prospCtlLerID(bruto, "playbook_id"); err == nil {
		if p.PipelineID, err = prospCtlLerID(bruto, "pipeline_id"); err == nil {
			p.StageID, err = prospCtlLerID(bruto, "stage_id")
		}
	}
	if err != nil {
		prospCtlInvalido(ctx, err.Error())
		return
	}

	var lista *models.ProspLista
	if p.MudaDestino() {
		var ok bool
		if lista, ok = prospCtlListaSemJob(ctx, id); !ok {
			return
		}
	} else if lista, err = models.ProspListaByID(utils.DB, id); err != nil {
		handleDBError(ctx, err)
		return
	}

	// Destino do negócio: etapa precisa existir e pertencer ao funil (o
	// informado agora ou o que a lista já tem).
	if p.StageID.Definir && p.StageID.ID != nil {
		pipeline := lista.PipelineID
		if p.PipelineID.Definir {
			pipeline = p.PipelineID.ID
		}
		var dono int64
		if err := utils.DB.QueryRow(`SELECT pipeline_id FROM pipeline_stages WHERE id = $1`, *p.StageID.ID).Scan(&dono); err != nil {
			if err == sql.ErrNoRows {
				prospCtlInvalido(ctx, "etapa não encontrada")
				return
			}
			serverError(ctx, err)
			return
		}
		if pipeline != nil && *pipeline != dono {
			prospCtlInvalido(ctx, "a etapa não pertence ao funil escolhido")
			return
		}
	}
	if p.PipelineID.Definir && p.PipelineID.ID != nil {
		var existe bool
		if err := utils.DB.QueryRow(`SELECT EXISTS (SELECT 1 FROM pipelines WHERE id = $1)`, *p.PipelineID.ID).Scan(&existe); err != nil {
			serverError(ctx, err)
			return
		}
		if !existe {
			prospCtlInvalido(ctx, "funil não encontrado")
			return
		}
	}

	if err := models.UpdateProspLista(utils.DB, id, p); err != nil {
		handleDBError(ctx, err)
		return
	}
	atual, err := models.ProspListaByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, services.EntidadeListaProspeccao, id, fmt.Sprintf("editou a lista de prospecção %q", atual.Nome))
	ctx.JSON(atual)
}

// DeleteProspLista: DELETE /prospeccao/listas/{id} → 204 (409 job_ativo com job em andamento).
func DeleteProspLista(ctx iris.Context) {
	id := paramID(ctx)
	lista, err := models.ProspListaByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteProspLista(utils.DB, id); err != nil {
		if errors.Is(err, models.ErrJobAtivo) {
			prospCtlJobAtivo(ctx, id)
			return
		}
		handleDBError(ctx, err)
		return
	}
	prospCtlAuditar(ctx, models.AuditDelete, services.EntidadeListaProspeccao, id,
		fmt.Sprintf("excluiu a lista de prospecção %q (%d empresas)", lista.Nome, lista.Contadores.Total),
		map[string]any{"total": lista.Contadores.Total, "enviados": lista.Contadores.Enviado})
	ctx.StatusCode(iris.StatusNoContent)
}

// -----------------------------------------------------------------------------
// Itens
// -----------------------------------------------------------------------------

// prospCtlBoolParam lê um booleano opcional da query ("" = não filtra).
func prospCtlBoolParam(ctx iris.Context, nome string) (*bool, bool) {
	v := strings.TrimSpace(ctx.URLParam(nome))
	if v == "" {
		return nil, true
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		badRequest(ctx, nome+" inválido")
		return nil, false
	}
	return &b, true
}

// ListProspItens: GET /prospeccao/listas/{id}/itens → { data, pagination }.
func ListProspItens(ctx iris.Context) {
	id := paramID(ctx)
	if _, err := models.ProspListaByID(utils.DB, id); err != nil {
		handleDBError(ctx, err)
		return
	}
	f := models.ProspItemLista{
		ProspItemFiltro: models.ProspItemFiltro{
			Estado:  ctx.URLParam("estado"),
			Q:       ctx.URLParam("q"),
			NotaMin: ctx.URLParamIntDefault("nota_min", 0),
		},
		Sort: ctx.URLParam("sort"),
		Dir:  ctx.URLParam("dir"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	if f.Estado != "" && !models.ItemEstadoValido(f.Estado) {
		badRequest(ctx, "estado inválido")
		return
	}
	switch f.Sort {
	case "", "nota", "razao_social", "municipio", "estado":
	default:
		badRequest(ctx, "ordenação inválida")
		return
	}
	var ok bool
	if f.ComCelular, ok = prospCtlBoolParam(ctx, "com_celular"); !ok {
		return
	}
	if f.ComDecisor, ok = prospCtlBoolParam(ctx, "com_decisor"); !ok {
		return
	}
	itens, total, err := models.ListProspItens(utils.DB, id, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Normalize()
	f.Total = total
	ctx.JSON(iris.Map{"data": itens, "pagination": f.Pagination})
}

// GetProspItem: GET /prospeccao/itens/{id} → Item com dados e enriquecimento.
func GetProspItem(ctx iris.Context) {
	it, err := models.ProspItemByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(it)
}

// prospCtlSelecao lê { selecao } e valida (422).
func prospCtlSelecao(ctx iris.Context) (models.ProspSelecao, bool) {
	var req struct {
		Selecao models.ProspSelecao `json:"selecao"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return req.Selecao, false
	}
	if err := req.Selecao.Validar(); err != nil {
		prospCtlInvalido(ctx, err.Error())
		return req.Selecao, false
	}
	return req.Selecao, true
}

// DescartarProspItens: POST /prospeccao/listas/{id}/itens/descartar { selecao } → { afetados }.
func DescartarProspItens(ctx iris.Context) {
	prospCtlMudarItens(ctx, true)
}

// RestaurarProspItens: POST /prospeccao/listas/{id}/itens/restaurar { selecao } → { afetados }.
func RestaurarProspItens(ctx iris.Context) {
	prospCtlMudarItens(ctx, false)
}

func prospCtlMudarItens(ctx iris.Context, descartar bool) {
	sel, ok := prospCtlSelecao(ctx)
	if !ok {
		return
	}
	lista, ok := prospCtlListaSemJob(ctx, paramID(ctx))
	if !ok {
		return
	}
	var n int
	var err error
	verbo := "restaurou"
	if descartar {
		verbo = "descartou"
		n, err = models.DescartarProspItens(utils.DB, lista.ID, sel)
	} else {
		n, err = models.RestaurarProspItens(utils.DB, lista.ID, sel)
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	if n > 0 {
		audit(ctx, models.AuditBulk, services.EntidadeListaProspeccao, lista.ID,
			fmt.Sprintf("%s %d empresas da lista de prospecção %q", verbo, n, lista.Nome))
	}
	ctx.JSON(iris.Map{"afetados": n})
}

// ExportarProspLista: GET /prospeccao/listas/{id}/exportar → CSV (; e UTF-8 com BOM), auditado.
//
// Uma linha por empresa, com o retrato da base e os contatos do enriquecimento.
// O bruto do provedor nunca é lido; documento de sócio e de pessoa só sai
// mascarado (e aqui nem sai).
func ExportarProspLista(ctx iris.Context) {
	lista, err := models.ProspListaByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.Header("Content-Type", "text/csv; charset=utf-8")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="lista-prospeccao-%d-%s.csv"`,
		lista.ID, time.Now().Format("2006-01-02")))
	_, _ = ctx.ResponseWriter().Write([]byte("\xEF\xBB\xBF"))
	w := csv.NewWriter(ctx.ResponseWriter())
	w.Comma = ';'
	_ = w.Write(services.ColunasExportacaoLista)

	exportados := 0
	defer func() {
		w.Flush()
		prospCtlAuditar(ctx, models.AuditExport, services.EntidadeListaProspeccao, lista.ID,
			fmt.Sprintf("exportou %d empresas da lista de prospecção %q em CSV", exportados, lista.Nome),
			map[string]any{"total": exportados, "incluir_pf": lista.IncluirPF, "pessoa_fisica": lista.Contadores.PessoaFisica})
	}()

	todos := models.ProspSelecao{Todos: true}
	var apos int64
	for {
		ids, err := models.ResolveProspSelecao(utils.DB, lista.ID, todos, nil, apos, 500)
		if err != nil || len(ids) == 0 {
			if err != nil {
				ctx.Application().Logger().Errorf("prospecção: exportação da lista %d interrompida: %v", lista.ID, err)
			}
			return
		}
		itens, err := models.ProspItensCompletos(utils.DB, ids)
		if err != nil {
			ctx.Application().Logger().Errorf("prospecção: exportação da lista %d interrompida: %v", lista.ID, err)
			return
		}
		for i := range itens {
			_ = w.Write(services.LinhaExportacaoItem(&itens[i]))
		}
		exportados += len(itens)
		apos = ids[len(ids)-1]
		w.Flush()
	}
}

// -----------------------------------------------------------------------------
// Jobs
// -----------------------------------------------------------------------------

// GetProspJob: GET /prospeccao/jobs/{id} → Job.
func GetProspJob(ctx iris.Context) {
	j, err := models.ProspJobByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(j)
}

// CancelarProspJob: POST /prospeccao/jobs/{id}/cancelar → Job.
//
// Job já encerrado volta como está (cancelar duas vezes não é erro).
func CancelarProspJob(ctx iris.Context) {
	id := paramID(ctx)
	j, err := models.ProspJobByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if !j.Ativo() {
		ctx.JSON(j)
		return
	}
	j, err = models.CancelarProspJob(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	var listaID int64
	if j.ListaID != nil {
		listaID = *j.ListaID
	}
	audit(ctx, models.AuditUpdate, services.EntidadeListaProspeccao, listaID,
		fmt.Sprintf("cancelou o processamento %d (%s) da lista de prospecção", j.ID, j.Tipo))
	ctx.JSON(j)
}
