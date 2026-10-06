// Painel administrativo do CRM IA.
//
// Gere grupos de CNPJ, clientes (cada CNPJ = um ambiente isolado), planos,
// licenças cobradas por assinatura recorrente no cartão, faturas, versões do
// aplicativo e leads — e aciona o provisionador para subir, atualizar e
// suspender os ambientes.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/banco"
	"github.com/armando-couto/crm-ia/painel/internal/config"
	"github.com/armando-couto/crm-ia/painel/internal/email"
	"github.com/armando-couto/crm-ia/painel/internal/jobs"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/provisionador"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
	"github.com/armando-couto/crm-ia/painel/internal/sign"
	"github.com/armando-couto/crm-ia/painel/internal/web"
)

func main() {
	soHealthcheck := flag.Bool("healthcheck", false, "consulta /saude e sai (usado pelo Docker)")
	soMigrar := flag.Bool("migrate", false, "aplica as migrações e sai")
	soSemear := flag.Bool("seed", false, "cria o administrador inicial e sai")
	flag.Parse()
	if *soHealthcheck {
		os.Exit(healthcheck())
	}
	if err := executar(*soMigrar, *soSemear); err != nil {
		slog.Error("falha ao iniciar", "erro", err)
		os.Exit(1)
	}
}

func executar(soMigrar, soSemear bool) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	nivel := slog.LevelInfo
	if cfg.EmDesenvolvimento() {
		nivel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})))

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	pool, err := banco.Conectar(ctx, cfg.URLBanco)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := banco.Migrar(ctx, pool); err != nil {
		return fmt.Errorf("migrando: %w", err)
	}
	if soMigrar {
		slog.Info("migrações aplicadas")
		return nil
	}
	r := repo.Novo(pool)
	if soSemear {
		return semear(ctx, r, cfg)
	}
	if n, err := r.ContarUsuarios(ctx); err == nil && n == 0 {
		if err := semear(ctx, r, cfg); err != nil {
			slog.Warn("não foi possível criar o administrador inicial", "erro", err)
		}
	}

	sessoes := auth.NovoGerenciador(cfg.ChaveSessao, cfg.ChaveCSRF, cfg.BasePath, !cfg.EmDesenvolvimento())
	prov := provisionador.Novo(cfg.ProvisionadorURL, cfg.ProvisionadorToken)
	sg := sign.Novo(sign.Config{TokenAPI: cfg.SignTokenAPI, ApisURL: cfg.SignApisURL, Gateway: cfg.SignGateway, GrupoID: cfg.SignGrupoID, Simulado: cfg.SignSimulado})
	ops := operacoes.Novo(r, prov, sg)
	ops.NomeProduto = cfg.NomeProduto
	ops.URLCheckout = func(codigo string) string { return cfg.URLPublica("/assinar/" + codigo) }
	correio := email.Novo(email.Config{Chave: cfg.MandrillChave, RemetenteEmail: cfg.FromEmail, RemetenteNome: cfg.FromNome})

	switch {
	case sg.Simulado():
		slog.Warn("cobrança recorrente em modo SIMULADO: nada é cobrado de verdade")
	case !sg.Configurado():
		slog.Warn("cobrança recorrente indisponível: " + sg.MotivoNaoConfigurado())
	}

	servidor, err := web.NovoServidor(cfg, r, sessoes, ops, prov, correio)
	if err != nil {
		return fmt.Errorf("montando servidor: %w", err)
	}
	jobs.NovoDiario(r, ops, cfg.DiasAtrasoSuspensao).Iniciar(ctx)
	jobs.NovoConferente(ops).Iniciar(ctx)

	servidorHTTP := &http.Server{
		Addr: ":" + cfg.Porta, Handler: servidor.Rotas(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 10 * time.Minute, IdleTimeout: 120 * time.Second,
	}
	erros := make(chan error, 1)
	go func() {
		slog.Info("painel no ar", "porta", cfg.Porta, "base_path", cfg.BasePath, "ambiente", cfg.Ambiente)
		erros <- servidorHTTP.ListenAndServe()
	}()
	select {
	case err := <-erros:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("encerrando…")
	}
	ctxParada, pararAgora := context.WithTimeout(context.Background(), 30*time.Second)
	defer pararAgora()
	return servidorHTTP.Shutdown(ctxParada)
}

func semear(ctx context.Context, r *repo.Repo, cfg config.Config) error {
	if cfg.AdminSenha == "" {
		return errors.New("ADMIN_SENHA não definida; não é possível criar o administrador inicial")
	}
	if _, err := r.UsuarioPorEmail(ctx, cfg.AdminEmail); err == nil {
		return nil
	}
	hash, err := auth.HashSenha(cfg.AdminSenha)
	if err != nil {
		return err
	}
	_, err = r.CriarUsuario(ctx, modelo.Usuario{Nome: "Administrador", Email: cfg.AdminEmail, SenhaHash: hash, Perfil: modelo.PerfilAdmin, Ativo: true})
	if err == nil {
		slog.Info("administrador inicial criado", "email", cfg.AdminEmail)
	}
	return err
}

func healthcheck() int {
	porta := os.Getenv("PORT")
	if porta == "" {
		porta = "7894"
	}
	base := os.Getenv("BASE_PATH")
	if base == "" {
		base = "/painel"
	}
	if base == "/" {
		base = ""
	}
	cli := &http.Client{Timeout: 4 * time.Second}
	resp, err := cli.Get("http://127.0.0.1:" + porta + base + "/saude")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	resp.Body.Close()
	return 0
}
