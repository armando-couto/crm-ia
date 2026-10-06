// Package api expõe o provisionador ao painel. Não tem rota no Traefik: só a
// rede interna o alcança, e ainda assim com o token compartilhado.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/provisioner/internal/tenant"
)

type API struct {
	servico *tenant.Servico
	token   string
}

func Nova(s *tenant.Servico, token string) *API { return &API{servico: s, token: token} }

func (a *API) Rotas() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.saude)
	mux.HandleFunc("POST /tenants", a.autenticar(a.criar))
	mux.HandleFunc("GET /tenants", a.autenticar(a.listar))
	mux.HandleFunc("GET /versoes", a.autenticar(a.versoes))
	mux.HandleFunc("GET /tenants/{slug}", a.autenticar(a.status))
	mux.HandleFunc("GET /tenants/{slug}/token-interno", a.autenticar(a.tokenInterno))
	mux.HandleFunc("DELETE /tenants/{slug}", a.autenticar(a.remover))
	mux.HandleFunc("POST /tenants/{slug}/atualizar", a.autenticar(a.atualizar))
	mux.HandleFunc("POST /tenants/{slug}/suspender", a.autenticar(a.suspender))
	mux.HandleFunc("POST /tenants/{slug}/reativar", a.autenticar(a.reativar))
	mux.HandleFunc("POST /tenants/{slug}/backup", a.autenticar(a.backup))
	return a.registrar(mux)
}

func (a *API) autenticar(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enviado, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(enviado), []byte(a.token)) != 1 {
			slog.Warn("chamada sem token válido", "caminho", r.URL.Path, "origem", r.RemoteAddr)
			responder(w, http.StatusUnauthorized, map[string]string{"erro": "não autorizado"})
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
				slog.Error("panic no provisionador", "erro", e, "caminho", r.URL.Path)
				responder(w, http.StatusInternalServerError, map[string]string{"erro": "erro interno"})
			}
		}()
		proximo.ServeHTTP(w, r)
		if r.URL.Path != "/health" {
			slog.Info("requisição", "metodo", r.Method, "caminho", r.URL.Path, "duracao_ms", time.Since(inicio).Milliseconds())
		}
	})
}

// prazoOperacao cobre o pior caso: pull das imagens, up e espera do healthcheck.
const prazoOperacao = 10 * time.Minute

// destacada solta a operação do contexto da requisição: se o painel desistir
// de esperar, um `compose up` morto no meio deixaria o ambiente pela metade.
func destacada(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), prazoOperacao)
}

func (a *API) saude(w http.ResponseWriter, _ *http.Request) {
	responder(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) criar(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := destacada(r)
	defer cancelar()
	var p tenant.Pedido
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&p); err != nil {
		responder(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	res, err := a.servico.Criar(ctx, p)
	if err != nil {
		slog.Error("falha ao provisionar", "slug", p.Slug, "erro", err)
		res.Erro = err.Error()
		responder(w, http.StatusUnprocessableEntity, res)
		return
	}
	responder(w, http.StatusCreated, res)
}

func (a *API) atualizar(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := destacada(r)
	defer cancelar()
	var at tenant.Atualizacao
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&at); err != nil {
		responder(w, http.StatusBadRequest, map[string]string{"erro": "corpo inválido"})
		return
	}
	res, err := a.servico.Atualizar(ctx, r.PathValue("slug"), at)
	if err != nil {
		responder(w, http.StatusUnprocessableEntity, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, res)
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	res, err := a.servico.Status(r.Context(), r.PathValue("slug"))
	if err != nil {
		responder(w, http.StatusBadRequest, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, res)
}

func (a *API) listar(w http.ResponseWriter, r *http.Request) {
	lista, err := a.servico.Listar(r.Context())
	if err != nil {
		responder(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if lista == nil {
		lista = []tenant.Resultado{}
	}
	responder(w, http.StatusOK, lista)
}

func (a *API) versoes(w http.ResponseWriter, r *http.Request) {
	tags, err := a.servico.VersoesDisponiveis(r.Context())
	if err != nil {
		responder(w, http.StatusInternalServerError, map[string]string{"erro": err.Error()})
		return
	}
	if tags == nil {
		tags = []string{}
	}
	responder(w, http.StatusOK, map[string]any{"tags": tags})
}

func (a *API) tokenInterno(w http.ResponseWriter, r *http.Request) {
	tok, err := a.servico.TokenInterno(r.PathValue("slug"))
	if err != nil {
		responder(w, http.StatusNotFound, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, map[string]string{"token": tok})
}

func (a *API) suspender(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := destacada(r)
	defer cancelar()
	if err := a.servico.Suspender(ctx, r.PathValue("slug")); err != nil {
		responder(w, http.StatusUnprocessableEntity, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, map[string]string{"status": "suspenso"})
}

func (a *API) reativar(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := destacada(r)
	defer cancelar()
	if err := a.servico.Reativar(ctx, r.PathValue("slug")); err != nil {
		responder(w, http.StatusUnprocessableEntity, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, map[string]string{"status": "ativo"})
}

func (a *API) backup(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := destacada(r)
	defer cancelar()
	arquivo, err := a.servico.Backup(ctx, r.PathValue("slug"))
	if err != nil {
		responder(w, http.StatusUnprocessableEntity, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, map[string]string{"arquivo": arquivo})
}

// remover exige a confirmação do slug no corpo: apagar banco de cliente não
// pode acontecer por um clique acidental.
func (a *API) remover(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := destacada(r)
	defer cancelar()
	var in struct {
		Confirmar string `json:"confirmar"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in)
	slug := r.PathValue("slug")
	if in.Confirmar != slug {
		responder(w, http.StatusUnprocessableEntity, map[string]string{"erro": "confirme a remoção repetindo o slug no campo 'confirmar'"})
		return
	}
	if err := a.servico.Remover(ctx, slug); err != nil {
		responder(w, http.StatusUnprocessableEntity, map[string]string{"erro": err.Error()})
		return
	}
	responder(w, http.StatusOK, map[string]string{"status": "removido"})
}

func responder(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}
