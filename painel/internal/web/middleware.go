package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
)

type chaveCtx string

const chaveSessao chaveCtx = "sessao"

func sessaoDe(r *http.Request) auth.Sessao {
	s, _ := r.Context().Value(chaveSessao).(auth.Sessao)
	return s
}

func (s *Servidor) logar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		proximo.ServeHTTP(w, r)
		if r.URL.Path == s.url("/saude") {
			return
		}
		slog.Info("requisição", "metodo", r.Method, "caminho", r.URL.Path, "duracao_ms", time.Since(inicio).Milliseconds())
	})
}

func (s *Servidor) recuperar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				slog.Error("panic no painel", "erro", e, "caminho", r.URL.Path)
				s.erroHTTP(w, r, http.StatusInternalServerError, "Erro interno. Tente de novo em instantes.")
			}
		}()
		proximo.ServeHTTP(w, r)
	})
}

// exigirSessao redireciona para o login quem não tem sessão válida. O perfil
// vale o que está no banco agora, não o que o cookie gravou.
func (s *Servidor) exigirSessao(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessao, err := s.sessoes.Ler(r)
		if err != nil {
			if !errors.Is(err, auth.ErrSemSessao) {
				s.sessoes.Encerrar(w)
			}
			http.Redirect(w, r, s.url("/login"), http.StatusSeeOther)
			return
		}
		u, err := s.repo.UsuarioPorID(r.Context(), sessao.UsuarioID)
		if err != nil || !u.Ativo || auth.Impressao(u.SenhaHash) != sessao.Impressao {
			s.sessoes.Encerrar(w)
			http.Redirect(w, r, s.url("/login"), http.StatusSeeOther)
			return
		}
		sessao.Nome, sessao.Email, sessao.Perfil = u.Nome, u.Email, u.Perfil
		proximo.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), chaveSessao, sessao)))
	})
}

func (s *Servidor) exigirVisitante(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.sessoes.Ler(r); err == nil {
			http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

func (s *Servidor) protegerCSRF(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			if err := s.sessoes.ValidarCSRF(r); err != nil {
				s.erroHTTP(w, r, http.StatusForbidden, "Sessão expirada ou formulário inválido. Recarregue a página.")
				return
			}
		}
		proximo.ServeHTTP(w, r)
	})
}

type permissao func(auth.Sessao) bool

func podeEditarClientes(s auth.Sessao) bool        { return s.PodeEditarClientes() }
func podeEditarLicencas(s auth.Sessao) bool        { return s.PodeEditarLicencas() }
func podeEditarFaturas(s auth.Sessao) bool         { return s.PodeEditarFaturas() }
func podeProvisionar(s auth.Sessao) bool           { return s.PodeProvisionar() }
func podeSuspender(s auth.Sessao) bool             { return s.PodeSuspender() }
func podeEditarPlanos(s auth.Sessao) bool          { return s.PodeEditarPlanos() }
func podeGerenciarUsuarios(s auth.Sessao) bool     { return s.PodeGerenciarUsuarios() }
func podeRedefinirSenhaCliente(s auth.Sessao) bool { return s.PodeRedefinirSenhaCliente() }

func (s *Servidor) exigir(p permissao) func(http.Handler) http.Handler {
	return func(proximo http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !p(sessaoDe(r)) {
				s.erroHTTP(w, r, http.StatusForbidden, "Seu perfil não tem permissão para esta ação.")
				return
			}
			proximo.ServeHTTP(w, r)
		})
	}
}
