// Provisionador do CRM IA.
//
// Sobe e administra a stack Docker de cada cliente — PostgreSQL, Redis e a
// aplicação — roteada pelo Traefik em https://$DOMAIN/<slug>. É o único
// componente da plataforma com acesso ao Docker; por isso não tem rota
// pública e só é alcançado pelo painel, na rede interna, com token.
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

	"github.com/armando-couto/crm-ia/provisioner/internal/api"
	"github.com/armando-couto/crm-ia/provisioner/internal/config"
	"github.com/armando-couto/crm-ia/provisioner/internal/tenant"
)

func main() {
	soHealthcheck := flag.Bool("healthcheck", false, "consulta /health e sai (usado pelo Docker)")
	flag.Parse()
	if *soHealthcheck {
		os.Exit(healthcheck())
	}
	if err := executar(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func executar() error {
	cfg, err := config.Carregar()
	if err != nil {
		return fmt.Errorf("configuração inválida: %w", err)
	}
	nivel := slog.LevelInfo
	if cfg.EmDesenvolvimento() {
		nivel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})))

	servico, err := tenant.NovoServico(tenant.Config{Config: cfg})
	if err != nil {
		return fmt.Errorf("falha ao iniciar o serviço: %w", err)
	}
	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	servidor := &http.Server{
		Addr:              ":" + cfg.Porta,
		Handler:           api.Nova(servico, cfg.Token).Rotas(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Minute, // provisionar leva minutos
		IdleTimeout:       120 * time.Second,
	}
	erros := make(chan error, 1)
	go func() {
		slog.Info("provisionador no ar", "porta", cfg.Porta, "dominio", cfg.Dominio)
		erros <- servidor.ListenAndServe()
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
	return servidor.Shutdown(ctxParada)
}

func healthcheck() int {
	porta := os.Getenv("PORT")
	if porta == "" {
		porta = "7898"
	}
	cli := &http.Client{Timeout: 4 * time.Second}
	resp, err := cli.Get("http://127.0.0.1:" + porta + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	resp.Body.Close()
	return 0
}
