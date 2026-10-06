// Package web monta o servidor HTTP do painel: rotas, templates e handlers.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/config"
	"github.com/armando-couto/crm-ia/painel/internal/email"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/provisionador"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
)

//go:embed templates/*.html estatico/*
var arquivos embed.FS

type Servidor struct {
	cfg       config.Config
	repo      *repo.Repo
	sessoes   *auth.Gerenciador
	ops       *operacoes.Servico
	prov      *provisionador.Cliente
	correio   *email.Cliente
	templates map[string]*template.Template
	digitais  map[string]string

	// Cache dos tokens internos dos ambientes (API do "Meu plano").
	tokensMu sync.Mutex
	tokens   map[string]tokenGuardado

	// Senhas de administrador à espera da única exibição.
	senhasMu sync.Mutex
	senhas   map[string]senhaUnica
}

func NovoServidor(cfg config.Config, r *repo.Repo, sessoes *auth.Gerenciador, ops *operacoes.Servico, prov *provisionador.Cliente, correio *email.Cliente) (*Servidor, error) {
	s := &Servidor{cfg: cfg, repo: r, sessoes: sessoes, ops: ops, prov: prov, correio: correio,
		tokens: map[string]tokenGuardado{}, senhas: map[string]senhaUnica{}}
	if err := s.calcularDigitais(); err != nil {
		return nil, err
	}
	if err := s.carregarTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

// carregarTemplates compila cada página junto com o layout e os componentes.
func (s *Servidor) carregarTemplates() error {
	entradas, err := fs.ReadDir(arquivos, "templates")
	if err != nil {
		return err
	}
	s.templates = map[string]*template.Template{}
	for _, e := range entradas {
		nome := e.Name()
		if !strings.HasPrefix(nome, "pagina_") {
			continue
		}
		t, err := template.New("layout.html").Funcs(s.funcoes()).ParseFS(arquivos, "templates/layout.html", "templates/componentes.html", "templates/"+nome)
		if err != nil {
			return fmt.Errorf("compilando %s: %w", nome, err)
		}
		s.templates[strings.TrimSuffix(nome, ".html")] = t
	}
	return nil
}

func (s *Servidor) calcularDigitais() error {
	s.digitais = map[string]string{}
	entradas, err := fs.ReadDir(arquivos, "estatico")
	if err != nil {
		return err
	}
	for _, e := range entradas {
		conteudo, err := fs.ReadFile(arquivos, "estatico/"+e.Name())
		if err != nil {
			return err
		}
		soma := sha256.Sum256(conteudo)
		s.digitais[e.Name()] = hex.EncodeToString(soma[:])[:10]
	}
	return nil
}

func (s *Servidor) funcoes() template.FuncMap {
	return template.FuncMap{
		"brl":      modelo.FormatarBRL,
		"url":      s.url,
		"estatico": s.estatico,
		"site": func() string {
			if s.cfg.SiteURL == "" {
				return "/"
			}
			return s.cfg.SiteURL
		},
		"produto":  func() string { return s.cfg.NomeProduto },
		"data":     func(t time.Time) string { return t.Format("02/01/2006") },
		"dataHora": func(t time.Time) string { return t.Format("02/01/2006 15:04") },
		"dataPtr": func(t *time.Time) string {
			if t == nil {
				return "—"
			}
			return t.Format("02/01/2006 15:04")
		},
		"cnpj": func(v string) string {
			if len(v) != 14 {
				return v
			}
			return fmt.Sprintf("%s.%s.%s/%s-%s", v[0:2], v[2:5], v[5:8], v[8:12], v[12:14])
		},
		"i64":     func(n int64) string { return strconv.FormatInt(n, 10) },
		"idIgual": func(p *int64, v int64) bool { return p != nil && *p == v },
		"ou": func(v, padrao string) string {
			if strings.TrimSpace(v) == "" {
				return padrao
			}
			return v
		},
		"reais":  func(centavos int64) string { return fmt.Sprintf("%.2f", float64(centavos)/100) },
		"juntar": strings.Join,
		"hoje":   func() time.Time { return time.Now() },
		"curto": func(v string, n int) string {
			if len(v) <= n {
				return v
			}
			return v[:n] + "…"
		},
	}
}

func (s *Servidor) estatico(caminho string) string {
	return s.url(caminho) + "?v=" + s.digitais[strings.TrimPrefix(caminho, "/estatico/")]
}

// url prefixa o caminho com o BASE_PATH (/painel).
func (s *Servidor) url(caminho string) string {
	if caminho == "" || caminho == "/" {
		return s.cfg.BasePath + "/"
	}
	if !strings.HasPrefix(caminho, "/") {
		caminho = "/" + caminho
	}
	return s.cfg.BasePath + caminho
}

// Rotas monta o roteador completo.
func (s *Servidor) Rotas() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, s.logar, s.recuperar)
	r.Use(middleware.Timeout(9 * time.Minute))

	base := s.cfg.BasePath
	if base == "" {
		base = "/"
	}
	r.Route(base, func(r chi.Router) {
		estatico, _ := fs.Sub(arquivos, "estatico")
		r.Handle("/estatico/*", http.StripPrefix(s.cfg.BasePath+"/estatico/", cacheLongo(http.FileServer(http.FS(estatico)))))
		r.Get("/saude", s.saude)

		// Público: landing e cliente final.
		r.Get("/api/planos", s.apiPlanos)
		r.Post("/api/leads", s.apiCriarLead)
		r.Get("/assinar/{codigo}", s.telaCheckout)
		r.Post("/assinar/{codigo}", s.processarCheckout)
		r.Post("/api/webhooks/recorrencia", s.webhookRecorrencia)

		// Pipeline de release e ambientes dos clientes (tokens de servidor).
		r.Post("/api/interno/versoes", s.apiPublicarVersao)
		r.Get("/api/empresas/{slug}/plano", s.apiPlanoDaEmpresa)
		r.Post("/api/empresas/{slug}/plano/solicitacoes", s.apiSolicitarPlano)

		r.Group(func(r chi.Router) {
			r.Use(s.exigirVisitante)
			r.Get("/login", s.telaLogin)
			r.Post("/login", s.processarLogin)
		})
		r.Post("/logout", s.logout)

		r.Group(func(r chi.Router) {
			r.Use(s.exigirSessao, s.protegerCSRF)
			r.Get("/", s.painel)

			r.Route("/grupos", func(r chi.Router) {
				r.Get("/", s.listarGrupos)
				r.Get("/{id}", s.verGrupo)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeEditarClientes))
					r.Get("/novo", s.formGrupo)
					r.Post("/novo", s.salvarGrupo)
					r.Get("/{id}/editar", s.formGrupo)
					r.Post("/{id}/editar", s.salvarGrupo)
				})
			})
			r.Route("/clientes", func(r chi.Router) {
				r.Get("/", s.listarClientes)
				r.Get("/{id}", s.verCliente)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeEditarClientes))
					r.Get("/novo", s.formCliente)
					r.Post("/novo", s.salvarCliente)
					r.Get("/{id}/editar", s.formCliente)
					r.Post("/{id}/editar", s.salvarCliente)
				})
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeProvisionar))
					r.Post("/{id}/provisionar", s.provisionarCliente)
					r.Post("/{id}/atualizar", s.atualizarVersaoCliente)
					r.Post("/{id}/backup", s.backupCliente)
					r.Post("/{id}/remover", s.removerCliente)
				})
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeSuspender))
					r.Post("/{id}/suspender", s.suspenderCliente)
					r.Post("/{id}/reativar", s.reativarCliente)
				})
				r.With(s.exigir(podeRedefinirSenhaCliente)).Post("/{id}/senha-admin", s.redefinirSenhaAdmin)
			})
			r.Route("/licencas", func(r chi.Router) {
				r.Get("/", s.listarLicencas)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeEditarLicencas))
					r.Get("/nova", s.formLicenca)
					r.Post("/nova", s.criarLicenca)
					r.Post("/{id}/renovar", s.renovarLicenca)
					r.Post("/{id}/cancelar", s.cancelarLicenca)
					r.Post("/{id}/enviar-link", s.enviarLinkCartao)
				})
			})
			r.Route("/faturas", func(r chi.Router) {
				r.Get("/", s.listarFaturas)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeEditarFaturas))
					r.Post("/conferir", s.conferirPagamentos)
					r.Post("/{id}/baixar", s.baixarFatura)
					r.Post("/{id}/cancelar", s.cancelarFatura)
				})
			})
			r.Route("/planos", func(r chi.Router) {
				r.Get("/", s.listarPlanos)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeEditarPlanos))
					r.Get("/novo", s.formPlano)
					r.Post("/novo", s.salvarPlano)
					r.Get("/{id}/editar", s.formPlano)
					r.Post("/{id}/editar", s.salvarPlano)
					r.Post("/configuracoes", s.salvarConfiguracoes)
				})
			})
			r.Route("/solicitacoes", func(r chi.Router) {
				r.Get("/", s.listarSolicitacoes)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeEditarLicencas))
					r.Post("/{id}/aprovar", s.aprovarSolicitacao)
					r.Post("/{id}/recusar", s.recusarSolicitacao)
				})
			})
			r.Route("/versoes", func(r chi.Router) {
				r.Get("/", s.listarVersoes)
				r.Group(func(r chi.Router) {
					r.Use(s.exigir(podeProvisionar))
					r.Post("/nova", s.criarVersao)
					r.Post("/{id}/padrao", s.definirVersaoPadrao)
					r.Post("/{id}/remover", s.removerVersao)
					r.Post("/aplicar", s.aplicarVersaoEmMassa)
				})
			})
			r.Get("/ambientes", s.listarAmbientes)
			r.Get("/leads", s.listarLeads)
			r.Post("/leads/{id}/status", s.atualizarLead)
			r.Group(func(r chi.Router) {
				r.Use(s.exigir(podeGerenciarUsuarios))
				r.Get("/usuarios", s.listarUsuarios)
				r.Post("/usuarios", s.salvarUsuario)
				r.Post("/usuarios/{id}/senha", s.trocarSenhaUsuario)
				r.Get("/auditoria", s.listarAuditoria)
			})
		})
	})
	return r
}

func cacheLongo(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		h.ServeHTTP(w, r)
	})
}

func (s *Servidor) saude(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
