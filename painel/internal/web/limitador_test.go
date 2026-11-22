package web

import (
	"testing"
	"time"
)

func TestLimitadorJanelaDeslizante(t *testing.T) {
	var l limitador
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if ok, _ := l.permitirEm(t0.Add(time.Duration(i)*10*time.Second), "acme|previa", 3, time.Minute); !ok {
			t.Fatalf("requisição %d deveria passar", i)
		}
	}
	ok, espera := l.permitirEm(t0.Add(30*time.Second), "acme|previa", 3, time.Minute)
	if ok || espera != 30*time.Second {
		t.Fatalf("a quarta deveria esperar 30 s: ok=%v espera=%v", ok, espera)
	}
	// Outra chave (outro cliente, outra rota) tem o próprio balde.
	if ok, _ := l.permitirEm(t0.Add(30*time.Second), "beta|previa", 3, time.Minute); !ok {
		t.Fatal("o limite de um cliente não pode pegar outro")
	}
	if ok, _ := l.permitirEm(t0.Add(30*time.Second), "acme|contagem", 3, time.Minute); !ok {
		t.Fatal("o limite é por rota")
	}
	// A primeira marca sai da janela aos 60 s: cabe mais uma, e só uma.
	if ok, _ := l.permitirEm(t0.Add(60*time.Second), "acme|previa", 3, time.Minute); !ok {
		t.Fatal("depois que a marca mais antiga saiu, deveria passar")
	}
	if ok, espera := l.permitirEm(t0.Add(61*time.Second), "acme|previa", 3, time.Minute); ok || espera != 9*time.Second {
		t.Fatalf("janela cheia de novo: ok=%v espera=%v", ok, espera)
	}
	// A requisição barrada não ocupa lugar na janela.
	if n := len(l.marcas["acme|previa"]); n != 3 {
		t.Fatalf("marcas guardadas: %d", n)
	}
	// Espera mínima de 1 s, para o Retry-After nunca ser zero.
	if _, espera := l.permitirEm(t0.Add(69*time.Second+900*time.Millisecond), "acme|previa", 3, time.Minute); espera != time.Second {
		t.Fatalf("espera mínima: %v", espera)
	}
}

func TestLimitadorSoltaChavesParadas(t *testing.T) {
	var l limitador
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5000; i++ {
		l.permitirEm(t0.Add(time.Duration(i)*time.Second), "c"+string(rune('a'+i%26))+"|rota", 1000, time.Minute)
	}
	l.permitirEm(t0.Add(3*time.Hour), "novo|rota", 10, time.Minute)
	for i := 0; i < 4096; i++ {
		l.permitirEm(t0.Add(3*time.Hour), "novo|rota", 100000, time.Minute)
	}
	if len(l.marcas) != 1 {
		t.Fatalf("chaves paradas deveriam sair do mapa: %d", len(l.marcas))
	}
}
