package services

import (
	"fmt"
	"strconv"
	"time"

	"github.com/armando-couto/crm-ia/app/utils"
)

// Limites de tentativas de autenticação. O estado vive no cache do ambiente
// (Redis em produção, memória nos testes): sobrevive ao reinício do container
// e vale para todas as réplicas da API.
const (
	LoginMaxAttempts = 5
	LoginWindow      = 15 * time.Minute
	LoginBlockFor    = 15 * time.Minute
)

const (
	prefixoFalhas   = "login:falhas:"
	prefixoBloqueio = "login:bloqueio:"
	prefixoPublico  = "publico:"
)

// LoginBlocked informa se a chave (IP+e-mail) está bloqueada e por quanto
// tempo ainda, sem contabilizar nova tentativa.
func LoginBlocked(key string) (bool, time.Duration) {
	ate, ok := utils.Cache.Get(prefixoBloqueio + key)
	if !ok {
		return false, 0
	}
	fim, err := strconv.ParseInt(ate, 10, 64)
	if err != nil {
		utils.Cache.Del(prefixoBloqueio + key)
		return false, 0
	}
	resta := time.Until(time.Unix(fim, 0))
	if resta <= 0 {
		utils.Cache.Del(prefixoBloqueio+key, prefixoFalhas+key)
		return false, 0
	}
	return true, resta
}

// RegisterLoginFailure contabiliza uma falha e devolve true quando o limite
// foi atingido (a partir daí a chave fica bloqueada por LoginBlockFor).
func RegisterLoginFailure(key string) bool {
	n := utils.Cache.Incr(prefixoFalhas+key, LoginWindow)
	if n >= LoginMaxAttempts {
		fim := time.Now().Add(LoginBlockFor)
		utils.Cache.Set(prefixoBloqueio+key, fmt.Sprint(fim.Unix()), LoginBlockFor)
		utils.Cache.Del(prefixoFalhas + key)
		return true
	}
	return false
}

// ClearLoginFailures zera o histórico após um login bem-sucedido.
func ClearLoginFailures(key string) {
	utils.Cache.Del(prefixoFalhas+key, prefixoBloqueio+key)
}

// Limites das rotas públicas (formulários e agendamento): mais folgados que o
// login, mas suficientes para conter robô de spam vindo de um mesmo IP.
const (
	PublicMaxAttempts = 20
	PublicWindow      = 10 * time.Minute
)

// PublicRateLimited conta um envio público e devolve true quando o IP passou do
// limite da janela. Assim que a janela vence, o IP volta a enviar.
func PublicRateLimited(key string) bool {
	return utils.Cache.Incr(prefixoPublico+key, PublicWindow) > PublicMaxAttempts
}

// ResetRateLimiter limpa todo o estado (usado nos testes).
func ResetRateLimiter() {
	utils.Cache.DelPrefixo(prefixoFalhas)
	utils.Cache.DelPrefixo(prefixoBloqueio)
	utils.Cache.DelPrefixo(prefixoPublico)
}
