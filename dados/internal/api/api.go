// Package api expõe o serviço de dados ao painel. Não tem rota no Traefik:
// só a rede interna o alcança, e ainda assim com o token compartilhado.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/dados/internal/busca"
	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// Buscador é o que a API usa da busca.
type Buscador interface {
	Situacao(ctx context.Context) (busca.Base, error)
	CatalogoCnaes(ctx context.Context, q, nivel, codigos string, limite int) ([]busca.ItemCnae, error)
	CatalogoMunicipios(ctx context.Context, uf, q, codigos string, limite int) ([]busca.ItemMunicipio, error)
	CatalogoNaturezas(ctx context.Context) ([]busca.ItemNatureza, error)
	CatalogoUFs(ctx context.Context) ([]busca.ItemUF, error)
	Contar(ctx context.Context, f busca.Filtros) (busca.Contagem, error)
	Previa(ctx context.Context, f busca.Filtros, ordem string, limite int) (busca.Previa, error)
	Candidatos(ctx context.Context, f busca.Filtros, ordem, cursor string, limite int) (busca.Candidatos, error)
	Detalhar(ctx context.Context, cnpjs []string, incluirPF bool) (busca.Detalhes, error)
	Empresa(ctx context.Context, cnpj string, incluirPF bool) (busca.Empresa, error)
}

// Cargas é o que a API usa da carga.
type Cargas interface {
	// Situacao devolve o bloco "carga" da situação.
	Situacao(ctx context.Context) (carga.EstadoCarga, error)
	// Iniciar dispara uma carga em segundo plano; carga.ErrEmCurso quando já
	// há uma e carga.ErrAmostra em modo amostra.
	Iniciar(fonte string) error
}

type API struct {
	busca  Buscador
	cargas Cargas
	token  string
}

func Nova(b Buscador, c Cargas, token string) *API { return &API{busca: b, cargas: c, token: token} }

func (a *API) Rotas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /saude", a.saude)
	mux.HandleFunc("GET /v1/situacao", a.autenticar(a.situacao))
	mux.HandleFunc("GET /v1/catalogos/cnaes", a.autenticar(a.catalogoCnaes))
	mux.HandleFunc("GET /v1/catalogos/municipios", a.autenticar(a.catalogoMunicipios))
	mux.HandleFunc("GET /v1/catalogos/naturezas", a.autenticar(a.catalogoNaturezas))
	mux.HandleFunc("GET /v1/catalogos/ufs", a.autenticar(a.catalogoUFs))
	mux.HandleFunc("POST /v1/empresas/contagem", a.autenticar(a.contagem))
	mux.HandleFunc("POST /v1/empresas/previa", a.autenticar(a.previa))
	mux.HandleFunc("POST /v1/empresas/candidatos", a.autenticar(a.candidatos))
	mux.HandleFunc("POST /v1/empresas/detalhes", a.autenticar(a.detalhes))
	mux.HandleFunc("GET /v1/empresas/{cnpj}", a.autenticar(a.empresa))
	mux.HandleFunc("POST /v1/cargas", a.autenticar(a.iniciarCarga))
	return a.registrar(mux)
}

func (a *API) autenticar(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enviado, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(enviado), []byte(a.token)) != 1 {
			slog.Warn("chamada sem token válido", "caminho", r.URL.Path, "origem", r.RemoteAddr)
			erro(w, http.StatusUnauthorized, "não autorizado")
			return
		}
		h(w, r)
	}
}

func (a *API) registrar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		defer func() {
			if e := recover(); e != nil {
				slog.Error("panic no serviço de dados", "erro", e, "caminho", r.URL.Path)
				erro(w, http.StatusInternalServerError, "erro interno")
			}
		}()
		proximo.ServeHTTP(w, r)
		if r.URL.Path != "/saude" {
			slog.Debug("requisição", "metodo", r.Method, "caminho", r.URL.Path, "duracao_ms", time.Since(inicio).Milliseconds())
		}
	})
}

func responder(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func erro(w http.ResponseWriter, status int, mensagem string) {
	responder(w, status, map[string]string{"erro": mensagem})
}

// falha traduz os erros da busca: 422 filtro inválido ou amplo demais, 409
// cursor de versão antiga, 503 base ainda não carregada, 404 fora da base.
func falha(w http.ResponseWriter, r *http.Request, err error) {
	var filtro *busca.ErroFiltro
	switch {
	case errors.As(err, &filtro):
		erro(w, http.StatusUnprocessableEntity, filtro.Mensagem)
	case errors.Is(err, busca.ErrCursorAntigo):
		erro(w, http.StatusConflict, busca.ErrCursorAntigo.Error())
	case errors.Is(err, busca.ErrNaoPronta):
		erro(w, http.StatusServiceUnavailable, busca.ErrNaoPronta.Error())
	case errors.Is(err, busca.ErrNaoEncontrado):
		erro(w, http.StatusNotFound, busca.ErrNaoEncontrado.Error())
	case errors.Is(err, context.Canceled):
		erro(w, 499, "requisição cancelada")
	default:
		slog.Error("falha na consulta", "caminho", r.URL.Path, "erro", err)
		erro(w, http.StatusInternalServerError, "erro interno")
	}
}

// corpo decodifica o JSON do pedido (até 1 MiB: 5.000 CNPJs cabem com folga).
func corpo(w http.ResponseWriter, r *http.Request, destino any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := d.Decode(destino); err != nil {
		erro(w, http.StatusBadRequest, "corpo inválido")
		return false
	}
	return true
}

func inteiro(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

func (a *API) saude(w http.ResponseWriter, _ *http.Request) {
	responder(w, http.StatusOK, map[string]string{"status": "ok"})
}

// situacaoResposta é o tipo Situacao do contrato.
type situacaoResposta struct {
	busca.Base
	Carga carga.EstadoCarga `json:"carga"`
}

func (a *API) situacao(w http.ResponseWriter, r *http.Request) {
	base, err := a.busca.Situacao(r.Context())
	if err != nil {
		falha(w, r, err)
		return
	}
	estado, err := a.cargas.Situacao(r.Context())
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, situacaoResposta{Base: base, Carga: estado})
}

func (a *API) catalogoCnaes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	nivel := strings.TrimSpace(q.Get("nivel"))
	switch nivel {
	case "", "divisao", "grupo", "classe", "subclasse":
	default:
		erro(w, http.StatusUnprocessableEntity, "nivel deve ser divisao, grupo, classe ou subclasse")
		return
	}
	itens, err := a.busca.CatalogoCnaes(r.Context(), q.Get("q"), nivel, q.Get("codigos"), inteiro(q.Get("limite")))
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens})
}

func (a *API) catalogoMunicipios(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	itens, err := a.busca.CatalogoMunicipios(r.Context(), q.Get("uf"), q.Get("q"), q.Get("codigos"), inteiro(q.Get("limite")))
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens})
}

func (a *API) catalogoNaturezas(w http.ResponseWriter, r *http.Request) {
	itens, err := a.busca.CatalogoNaturezas(r.Context())
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens})
}

func (a *API) catalogoUFs(w http.ResponseWriter, r *http.Request) {
	itens, err := a.busca.CatalogoUFs(r.Context())
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens})
}

type pedidoBusca struct {
	Filtros busca.Filtros `json:"filtros"`
	Ordem   string        `json:"ordem"`
	Limite  int           `json:"limite"`
	Cursor  string        `json:"cursor"`
}

func (a *API) contagem(w http.ResponseWriter, r *http.Request) {
	var p pedidoBusca
	if !corpo(w, r, &p) {
		return
	}
	c, err := a.busca.Contar(r.Context(), p.Filtros)
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, c)
}

func (a *API) previa(w http.ResponseWriter, r *http.Request) {
	var p pedidoBusca
	if !corpo(w, r, &p) {
		return
	}
	if p.Limite > busca.MaximoPrevia {
		erro(w, http.StatusUnprocessableEntity, "limite da prévia: até 25")
		return
	}
	res, err := a.busca.Previa(r.Context(), p.Filtros, p.Ordem, p.Limite)
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, res)
}

func (a *API) candidatos(w http.ResponseWriter, r *http.Request) {
	var p pedidoBusca
	if !corpo(w, r, &p) {
		return
	}
	if p.Limite > busca.MaximoCandidatos {
		erro(w, http.StatusUnprocessableEntity, "limite de candidatos: até 5000 por chamada")
		return
	}
	res, err := a.busca.Candidatos(r.Context(), p.Filtros, p.Ordem, p.Cursor, p.Limite)
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, res)
}

func (a *API) detalhes(w http.ResponseWriter, r *http.Request) {
	var p struct {
		CNPJs     []string `json:"cnpjs"`
		IncluirPF bool     `json:"incluir_pf"`
	}
	if !corpo(w, r, &p) {
		return
	}
	if len(p.CNPJs) == 0 {
		erro(w, http.StatusUnprocessableEntity, "informe cnpjs")
		return
	}
	res, err := a.busca.Detalhar(r.Context(), p.CNPJs, p.IncluirPF)
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, res)
}

func (a *API) empresa(w http.ResponseWriter, r *http.Request) {
	cnpj := texto.NormalizarCNPJ(r.PathValue("cnpj"))
	if !texto.CNPJFormaValida(cnpj) {
		erro(w, http.StatusUnprocessableEntity, "CNPJ inválido")
		return
	}
	incluirPF, _ := strconv.ParseBool(r.URL.Query().Get("incluir_pf"))
	e, err := a.busca.Empresa(r.Context(), cnpj, incluirPF)
	if err != nil {
		falha(w, r, err)
		return
	}
	responder(w, http.StatusOK, e)
}

func (a *API) iniciarCarga(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Fonte string `json:"fonte"`
	}
	if !corpo(w, r, &p) {
		return
	}
	switch p.Fonte {
	case carga.FonteCNPJ, carga.FontePGFN, carga.FonteTudo:
	default:
		erro(w, http.StatusUnprocessableEntity, "fonte deve ser cnpj, pgfn ou tudo")
		return
	}
	switch err := a.cargas.Iniciar(p.Fonte); {
	case err == nil:
		responder(w, http.StatusAccepted, map[string]bool{"ok": true})
	case errors.Is(err, carga.ErrAmostra):
		erro(w, http.StatusConflict, carga.ErrAmostra.Error())
	case errors.Is(err, carga.ErrEmCurso):
		erro(w, http.StatusConflict, carga.ErrEmCurso.Error())
	default:
		slog.Error("não foi possível iniciar a carga", "erro", err)
		erro(w, http.StatusInternalServerError, "erro interno")
	}
}
