package services

import (
	"sync"
	"time"
)

// Limites de tentativas de autenticação (janela deslizante em memória).
const (
	LoginMaxAttempts = 5
	LoginWindow      = 15 * time.Minute
	LoginBlockFor    = 15 * time.Minute
)

type attemptRecord struct {
	failures  []time.Time
	blockedAt time.Time
}

var (
	attemptsMu sync.Mutex
	attempts   = map[string]*attemptRecord{}
	lastPurge  time.Time
)

// purgeLocked remove registros antigos; chamado com o mutex já travado.
func purgeLocked(now time.Time) {
	if now.Sub(lastPurge) < LoginWindow {
		return
	}
	lastPurge = now
	for key, rec := range attempts {
		if len(rec.failures) == 0 && now.Sub(rec.blockedAt) > LoginBlockFor {
			delete(attempts, key)
			continue
		}
		if len(rec.failures) > 0 && now.Sub(rec.failures[len(rec.failures)-1]) > LoginWindow &&
			now.Sub(rec.blockedAt) > LoginBlockFor {
			delete(attempts, key)
		}
	}
}

// LoginBlocked informa se a chave (IP ou IP+e-mail) está bloqueada e por quanto
// tempo ainda, sem contabilizar nova tentativa.
func LoginBlocked(key string) (bool, time.Duration) {
	now := time.Now()
	attemptsMu.Lock()
	defer attemptsMu.Unlock()
	purgeLocked(now)

	rec, ok := attempts[key]
	if !ok || rec.blockedAt.IsZero() {
		return false, 0
	}
	if elapsed := now.Sub(rec.blockedAt); elapsed < LoginBlockFor {
		return true, LoginBlockFor - elapsed
	}
	// Bloqueio expirou: zera o histórico.
	rec.blockedAt = time.Time{}
	rec.failures = nil
	return false, 0
}

// RegisterLoginFailure contabiliza uma falha e devolve true quando o limite foi
// atingido (a partir daí a chave fica bloqueada por LoginBlockFor).
func RegisterLoginFailure(key string) bool {
	now := time.Now()
	attemptsMu.Lock()
	defer attemptsMu.Unlock()

	rec, ok := attempts[key]
	if !ok {
		rec = &attemptRecord{}
		attempts[key] = rec
	}

	// Mantém apenas as falhas dentro da janela.
	kept := rec.failures[:0]
	for _, t := range rec.failures {
		if now.Sub(t) <= LoginWindow {
			kept = append(kept, t)
		}
	}
	rec.failures = append(kept, now)

	if len(rec.failures) >= LoginMaxAttempts {
		rec.blockedAt = now
		return true
	}
	return false
}

// Limites das rotas públicas (formulários e agendamento): mais folgados que o
// login, mas suficientes para conter robô de spam vindo de um mesmo IP.
const (
	PublicMaxAttempts = 20
	PublicWindow      = 10 * time.Minute
)

// PublicRateLimited conta um envio público e devolve true quando o IP passou do
// limite da janela. Diferente do login, aqui não há bloqueio prolongado: assim
// que a janela anda, o IP volta a enviar.
func PublicRateLimited(key string) bool {
	now := time.Now()
	attemptsMu.Lock()
	defer attemptsMu.Unlock()

	rec, ok := attempts["public|"+key]
	if !ok {
		rec = &attemptRecord{}
		attempts["public|"+key] = rec
	}

	kept := rec.failures[:0]
	for _, t := range rec.failures {
		if now.Sub(t) <= PublicWindow {
			kept = append(kept, t)
		}
	}
	rec.failures = append(kept, now)
	return len(rec.failures) > PublicMaxAttempts
}

// ClearLoginFailures zera o histórico após um login bem-sucedido.
func ClearLoginFailures(key string) {
	attemptsMu.Lock()
	delete(attempts, key)
	attemptsMu.Unlock()
}

// ResetRateLimiter limpa todo o estado (usado nos testes).
func ResetRateLimiter() {
	attemptsMu.Lock()
	attempts = map[string]*attemptRecord{}
	lastPurge = time.Time{}
	attemptsMu.Unlock()
}
