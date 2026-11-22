// Serviço de dados públicos do CRM IA: carrega a base aberta do CNPJ (Receita
// Federal, mensal) e a dívida ativa (PGFN, trimestral) e responde contagem,
// busca e ficha para o painel. Só rede interna.
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
	"strconv"
	"syscall"
	"time"

	"github.com/armando-couto/crm-ia/dados/internal/amostra"
	"github.com/armando-couto/crm-ia/dados/internal/api"
	"github.com/armando-couto/crm-ia/dados/internal/banco"
	"github.com/armando-couto/crm-ia/dados/internal/busca"
	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/config"
	"github.com/armando-couto/crm-ia/dados/internal/jobs"
)

func main() {
	soHealthcheck := flag.Bool("healthcheck", false, "consulta /saude e sai (usado pelo Docker)")
	soMigrar := flag.Bool("migrate", false, "aplica as migrações e sai")
	soCarga := flag.Bool("carga", false, "roda uma carga (ou monta a amostra) e sai")
	fonte := flag.String("fonte", carga.FonteTudo, "com -carga: cnpj, pgfn ou tudo")
	flag.Parse()
	if *soHealthcheck {
		os.Exit(healthcheck())
	}
	if err := executar(*soMigrar, *soCarga, *fonte); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func executar(soMigrar, soCarga bool, fonte string) error {
	cfg, err := config.Carregar()
	if err != nil {
		return fmt.Errorf("configuração inválida: %w", err)
	}
	nivel := slog.LevelInfo
	if cfg.EmDesenvolvimento() {
		nivel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})))

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	// sessão de busca: sem JIT (custa mais do que rende), work_mem de 64 MB
	// (com o padrão o mapa de bits fica com perda) e statement_timeout curto
	pool, err := banco.Conectar(ctx, cfg.DatabaseURL, 8, map[string]string{
		"jit":               "off",
		"work_mem":          "64MB",
		"statement_timeout": strconv.FormatInt(cfg.TempoBusca.Milliseconds(), 10),
		"application_name":  "crmia-dados",
	})
	if err != nil {
		return fmt.Errorf("banco de dados: %w", err)
	}
	defer pool.Close()
	if err := banco.Migrar(ctx, pool); err != nil {
		return fmt.Errorf("migrações: %w", err)
	}
	if soMigrar {
		slog.Info("migrações aplicadas")
		return nil
	}

	cargas := carga.Novo(cfg)
	if soCarga {
		return cargaUnica(ctx, cfg, cargas, fonte, func() error { return amostra.Montar(ctx, pool) })
	}

	esquema := carga.EsquemaServico
	if cfg.Amostra {
		esquema = carga.EsquemaAmostra
	}
	agendador := jobs.Novo(ctx, cfg, pool, cargas)
	fundo := make(chan struct{})
	go func() {
		defer close(fundo)
		agendador.Rodar()
	}()

	servidor := &http.Server{
		Addr:              ":" + cfg.Porta,
		Handler:           api.Nova(busca.Novo(pool, esquema, cfg.Amostra), agendador, cfg.Token).Rotas(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	erros := make(chan error, 1)
	go func() {
		slog.Info("serviço de dados no ar", "porta", cfg.Porta, "amostra", cfg.Amostra, "carga_automatica", cfg.CargaAutomatica, "ufs", cfg.UFs)
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
	err = servidor.Shutdown(ctxParada)
	cancelar()
	// a carga em curso encerra de forma limpa (a próxima subida retoma)
	select {
	case <-fundo:
	case <-ctxParada.Done():
		slog.Warn("a carga em curso não encerrou a tempo")
	}
	return err
}

// cargaUnica atende a flag -carga: roda uma carga e sai. Quando a referência
// nova ainda está sendo publicada, espera e confere de novo.
func cargaUnica(ctx context.Context, cfg config.Config, cargas *carga.Servico, fonte string, montarAmostra func() error) error {
	if cfg.Amostra {
		return montarAmostra()
	}
	for {
		err := cargas.Executar(ctx, fonte)
		if !errors.Is(err, carga.ErrAguardando) {
			return err
		}
		slog.Info("referência nova ainda em publicação; nova conferência depois da espera", "espera", cfg.EsperaPublicacao.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.EsperaPublicacao + time.Minute):
		}
	}
}

func healthcheck() int {
	porta := os.Getenv("PORT")
	if porta == "" {
		porta = "7896"
	}
	cli := &http.Client{Timeout: 4 * time.Second}
	resp, err := cli.Get("http://127.0.0.1:" + porta + "/saude")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
