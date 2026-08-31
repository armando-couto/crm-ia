package services

import (
	"testing"
)

func TestRateLimiterBlocksAfterMaxAttempts(t *testing.T) {
	ResetRateLimiter()
	key := "10.0.0.1|ana@fixpay.com.br"

	for i := 1; i < LoginMaxAttempts; i++ {
		if RegisterLoginFailure(key) {
			t.Fatalf("bloqueou cedo demais, na tentativa %d", i)
		}
		if blocked, _ := LoginBlocked(key); blocked {
			t.Fatalf("chave bloqueada antes do limite, tentativa %d", i)
		}
	}

	if !RegisterLoginFailure(key) {
		t.Fatalf("deveria bloquear na tentativa %d", LoginMaxAttempts)
	}
	blocked, wait := LoginBlocked(key)
	if !blocked {
		t.Fatal("chave deveria estar bloqueada")
	}
	if wait <= 0 || wait > LoginBlockFor {
		t.Fatalf("tempo de espera fora do intervalo: %v", wait)
	}
}

func TestRateLimiterIsolatesKeys(t *testing.T) {
	ResetRateLimiter()
	alvo := "10.0.0.1|alvo@fixpay.com.br"
	outro := "10.0.0.2|outro@fixpay.com.br"

	for i := 0; i < LoginMaxAttempts; i++ {
		RegisterLoginFailure(alvo)
	}

	if blocked, _ := LoginBlocked(alvo); !blocked {
		t.Fatal("a chave atacada deveria estar bloqueada")
	}
	if blocked, _ := LoginBlocked(outro); blocked {
		t.Fatal("o bloqueio vazou para outra chave")
	}
}

func TestClearLoginFailuresResetsCounter(t *testing.T) {
	ResetRateLimiter()
	key := "10.0.0.3|ana@fixpay.com.br"

	for i := 0; i < LoginMaxAttempts-1; i++ {
		RegisterLoginFailure(key)
	}
	ClearLoginFailures(key)

	// Depois de um acesso bem-sucedido a contagem recomeça do zero.
	for i := 1; i < LoginMaxAttempts; i++ {
		if RegisterLoginFailure(key) {
			t.Fatalf("contador não foi zerado: bloqueou na tentativa %d", i)
		}
	}
}
