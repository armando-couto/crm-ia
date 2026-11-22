package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/config"
)

func TestModoAmostraNuncaIniciaCarga(t *testing.T) {
	cfg := config.Config{Amostra: true}
	a := Novo(context.Background(), cfg, nil, carga.Novo(cfg))
	if err := a.Iniciar(carga.FonteTudo); !errors.Is(err, carga.ErrAmostra) {
		t.Fatalf("esperado ErrAmostra, veio %v", err)
	}
}

func TestSegundaCargaERecusadaEnquantoAPrimeiraRoda(t *testing.T) {
	cfg := config.Config{}
	cargas := carga.Novo(cfg)
	if !cargas.Reservar() { // uma carga em curso
		t.Fatal("a primeira reserva deveria passar")
	}
	a := Novo(context.Background(), cfg, nil, cargas)
	if err := a.Iniciar(carga.FonteCNPJ); !errors.Is(err, carga.ErrEmCurso) {
		t.Fatalf("esperado ErrEmCurso, veio %v", err)
	}
}

func TestEsperarRespeitaOCancelamento(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	if esperar(ctx, time.Hour) {
		t.Fatal("contexto cancelado deveria interromper a espera")
	}
	if !esperar(context.Background(), time.Millisecond) {
		t.Fatal("a espera deveria terminar")
	}
}
