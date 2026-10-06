// Package jobs roda as rotinas periódicas do painel.
package jobs

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
)

// Diario: marca licenças/faturas vencidas, gera as faturas do mês, suspende
// quem está sem licença vigente há mais de N dias e aplica a versão padrão a
// quem optou por acompanhar.
type Diario struct {
	repo       *repo.Repo
	ops        *operacoes.Servico
	diasAtraso int
}

func NovoDiario(r *repo.Repo, ops *operacoes.Servico, diasAtraso int) *Diario {
	return &Diario{repo: r, ops: ops, diasAtraso: diasAtraso}
}

func (d *Diario) Iniciar(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}
		d.Rodar(ctx)
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				d.Rodar(ctx)
			}
		}
	}()
}

func (d *Diario) Rodar(ctx context.Context) {
	hoje := time.Now()
	slog.Info("rotina diária iniciada")
	if n, err := d.repo.MarcarLicencasVencidas(ctx, hoje); err != nil {
		slog.Error("marcando licenças vencidas", "erro", err)
	} else if n > 0 {
		slog.Info("licenças vencidas", "quantidade", n)
	}
	if n, err := d.repo.MarcarFaturasVencidas(ctx, hoje); err != nil {
		slog.Error("marcando faturas vencidas", "erro", err)
	} else if n > 0 {
		slog.Info("faturas vencidas", "quantidade", n)
	}
	if n, err := d.ops.GerarFaturasDoMes(ctx, hoje); err != nil {
		slog.Error("gerando faturas do mês", "erro", err)
	} else if n > 0 {
		slog.Info("faturas do mês geradas", "quantidade", n)
	}
	// Suspensão por inadimplência: só suspende, nunca remove.
	clientes, err := d.repo.ClientesSemLicencaVigente(ctx, hoje, d.diasAtraso)
	if err != nil {
		slog.Error("listando clientes sem licença", "erro", err)
	}
	for _, c := range clientes {
		if err := d.ops.SuspenderPorInadimplencia(ctx, c.ID, "licença vencida há mais de "+strconv.Itoa(d.diasAtraso)+" dias"); err != nil {
			slog.Error("suspendendo cliente", "slug", c.Slug, "erro", err)
		} else {
			slog.Warn("cliente suspenso por licença vencida", "slug", c.Slug)
		}
	}
	if padrao, err := d.repo.VersaoPadrao(ctx); err == nil {
		lista, err := d.repo.ClientesParaAutoAtualizar(ctx, padrao.Tag)
		if err != nil {
			slog.Error("listando clientes para auto-atualizar", "erro", err)
		}
		for _, c := range lista {
			if err := d.ops.AtualizarVersao(ctx, c.ID, padrao.Tag); err != nil {
				slog.Error("auto-atualização falhou", "slug", c.Slug, "versao", padrao.Tag, "erro", err)
			} else {
				slog.Info("cliente auto-atualizado", "slug", c.Slug, "versao", padrao.Tag)
			}
		}
	}
	slog.Info("rotina diária concluída")
}

// Conferente consulta a recorrência de tempos em tempos e baixa as faturas
// pagas — é o que faz o pagamento virar acesso sem ninguém clicar em nada.
type Conferente struct {
	ops       *operacoes.Servico
	intervalo time.Duration
}

func NovoConferente(ops *operacoes.Servico) *Conferente {
	return &Conferente{ops: ops, intervalo: 30 * time.Minute}
}

func (c *Conferente) Iniciar(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Minute):
		}
		c.Rodar(ctx)
		t := time.NewTicker(c.intervalo)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.Rodar(ctx)
			}
		}
	}()
}

func (c *Conferente) Rodar(ctx context.Context) {
	n, err := c.ops.ConferirPagamentos(ctx)
	if err != nil {
		slog.Error("conferindo pagamentos da recorrência", "erro", err)
		return
	}
	if n > 0 {
		slog.Info("faturas baixadas pela conferência", "quantidade", n)
	}
}
