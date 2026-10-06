package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// -----------------------------------------------------------------------------
// Disparo de e-mails: as regras que o cliente define no setup.
//
// Sequências e automações mandam e-mail sozinhas, de madrugada se ninguém
// configurar nada. Aqui ficam a janela de horário, os dias da semana e o teto
// diário — contado no cache do ambiente (Redis), então vale mesmo com a API
// reiniciando. Envio manual (botão da tela) não passa pela janela, só pelo teto.
// -----------------------------------------------------------------------------

// ErrForaDaJanela e ErrTetoDiario explicam por que um envio automático esperou.
var (
	ErrForaDaJanela = errors.New("fora da janela de disparo configurada")
	ErrTetoDiario   = errors.New("teto diário de e-mails atingido")
	ErrPausado      = errors.New("disparos automáticos pausados nas configurações")
)

const chaveContadorDia = "disparo:dia:"

// EnvioAutomaticoPermitido decide se uma sequência/automação pode mandar
// e-mail AGORA. Não consome o teto: quem envia chama RegistrarEnvio depois.
func EnvioAutomaticoPermitido(db *sql.DB, agora time.Time) error {
	cfg := models.LoadSendingSettings(db)
	if cfg.Paused {
		return ErrPausado
	}
	if !cfg.DentroDaJanela(agora) {
		return ErrForaDaJanela
	}
	if cfg.DailyLimit > 0 && EnviadosHoje(agora) >= int64(cfg.DailyLimit) {
		return ErrTetoDiario
	}
	return nil
}

// EnviadosHoje lê o contador do dia.
func EnviadosHoje(agora time.Time) int64 {
	v, ok := utils.Cache.Get(chaveContadorDia + agora.Format("2006-01-02"))
	if !ok {
		return 0
	}
	var n int64
	fmt.Sscan(v, &n)
	return n
}

// RegistrarEnvio soma um e-mail no contador do dia (expira em 48h).
func RegistrarEnvio(agora time.Time) int64 {
	return utils.Cache.Incr(chaveContadorDia+agora.Format("2006-01-02"), 48*time.Hour)
}

// ResumoDisparo é o que a tela de configurações mostra ao lado do formulário.
type ResumoDisparo struct {
	EnviadosHoje int64  `json:"enviados_hoje"`
	Teto         int    `json:"teto"`
	DentroJanela bool   `json:"dentro_da_janela"`
	Provedor     string `json:"provedor"`
}

func ResumoDoDisparo(db *sql.DB, provedor string) ResumoDisparo {
	cfg := models.LoadSendingSettings(db)
	agora := time.Now()
	return ResumoDisparo{EnviadosHoje: EnviadosHoje(agora), Teto: cfg.DailyLimit,
		DentroJanela: cfg.DentroDaJanela(agora), Provedor: provedor}
}
