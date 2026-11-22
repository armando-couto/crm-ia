// Package jobs roda o trabalho de fundo do serviço: a montagem da amostra, a
// retomada de carga interrompida e a conferência periódica das fontes. Erro
// de carga nunca derruba o processo: fica registrado e a API continua
// respondendo com a base anterior.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/dados/internal/amostra"
	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/config"
)

// Agendador é o dono das cargas em segundo plano. Implementa api.Cargas.
type Agendador struct {
	cfg    config.Config
	pool   *pgxpool.Pool
	cargas *carga.Servico

	ctx context.Context
	wg  sync.WaitGroup
}

// Novo cria o agendador; ctx é o contexto de vida do serviço (o SIGTERM o
// cancela e a carga em curso encerra de forma limpa).
func Novo(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, cargas *carga.Servico) *Agendador {
	return &Agendador{cfg: cfg, pool: pool, cargas: cargas, ctx: ctx}
}

// Situacao devolve a carga mais recente registrada no banco.
func (a *Agendador) Situacao(ctx context.Context) (carga.EstadoCarga, error) {
	return carga.Situacao(ctx, a.pool)
}

// Iniciar dispara uma carga em segundo plano e volta na hora.
func (a *Agendador) Iniciar(fonte string) error {
	if a.cfg.Amostra {
		return carga.ErrAmostra
	}
	if !a.cargas.Reservar() {
		return carga.ErrEmCurso
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.registrar(a.cargas.ExecutarReservada(a.ctx, fonte), fonte)
	}()
	return nil
}

func (a *Agendador) registrar(err error, fonte string) {
	switch {
	case err == nil, errors.Is(err, carga.ErrAguardando), errors.Is(err, context.Canceled):
	case errors.Is(err, carga.ErrEmCurso):
		slog.Info("carga não iniciada: já há outra em curso", "fonte", fonte)
	default:
		slog.Error("carga terminou com erro", "fonte", fonte, "erro", err)
	}
}

// Rodar bloqueia até o contexto do serviço acabar e então espera a carga em curso
// encerrar de forma limpa.
func (a *Agendador) Rodar() {
	ctx := a.ctx
	defer a.wg.Wait()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic no agendador", "erro", r)
		}
	}()

	if a.cfg.Amostra {
		// modo amostra: ignora DADOS_CARGA_AUTOMATICA e nunca acessa a rede
		for ctx.Err() == nil {
			err := amostra.Garantir(ctx, a.pool)
			if err == nil {
				break
			}
			slog.Error("não foi possível montar a base de amostra; nova tentativa em 30 s", "erro", err)
			if !esperar(ctx, 30*time.Second) {
				return
			}
		}
		<-ctx.Done()
		return
	}

	// carga interrompida por um encerramento retoma mesmo com a carga
	// automática desligada
	if fonte, ok := carga.Pendente(ctx, a.pool); ok {
		slog.Info("retomando a carga interrompida", "fonte", fonte)
		espera := a.executar(ctx, fonte)
		if !a.cfg.CargaAutomatica {
			<-ctx.Done()
			return
		}
		if !esperar(ctx, espera) {
			return
		}
	}
	if !a.cfg.CargaAutomatica {
		<-ctx.Done()
		return
	}
	for {
		if !esperar(ctx, a.executar(ctx, carga.FonteTudo)) {
			return
		}
	}
}

// executar roda uma carga e devolve quanto esperar até a próxima
// conferência: o intervalo normal, ou a espera da publicação quando a
// referência nova ainda está subindo.
func (a *Agendador) executar(ctx context.Context, fonte string) time.Duration {
	err := a.cargas.Executar(ctx, fonte)
	a.registrar(err, fonte)
	if errors.Is(err, carga.ErrAguardando) {
		return a.cfg.EsperaPublicacao + time.Minute
	}
	return a.cfg.IntervaloCarga
}

func esperar(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
