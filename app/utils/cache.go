package utils

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store é o cache de curta duração do ambiente: sessão de usuário, limites de
// tentativa de login, contadores de disparo de e-mail e respostas caras do
// dashboard. Em produção é o Redis que sobe na stack do cliente; sem
// REDIS_URL (testes, desenvolvimento avulso) um mapa em memória com TTL faz o
// mesmo papel — o comportamento é idêntico, só não sobrevive ao reinício.
type Store interface {
	Get(chave string) (string, bool)
	Set(chave, valor string, ttl time.Duration)
	Del(chaves ...string)
	// Incr soma 1 e devolve o total; cria a chave com o TTL informado.
	Incr(chave string, ttl time.Duration) int64
	// DelPrefixo remove todas as chaves que começam com o prefixo (testes e
	// invalidação em massa).
	DelPrefixo(prefixo string)
	Nome() string
}

// Cache é o store ativo. Começa em memória; ConectarRedis o troca pelo Redis.
var Cache Store = NovoMemoria()

// ConectarRedis liga o cache ao Redis de REDIS_URL. Falha de conexão não
// derruba o ambiente: o CRM continua com o cache em memória e registra o aviso.
func ConectarRedis(url string) error {
	if url == "" {
		return errors.New("REDIS_URL não definido")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return err
	}
	cliente := redis.NewClient(opts)
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	if err := cliente.Ping(ctx).Err(); err != nil {
		return err
	}
	Cache = &redisStore{cli: cliente, prefixo: "crmia:" + Cfg.TenantSlug + ":"}
	return nil
}

// ---------------------------------------------------------------------------
// Redis
// ---------------------------------------------------------------------------

type redisStore struct {
	cli     *redis.Client
	prefixo string
}

func (r *redisStore) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}

func (r *redisStore) Nome() string { return "redis" }

func (r *redisStore) Get(chave string) (string, bool) {
	ctx, cancelar := r.ctx()
	defer cancelar()
	v, err := r.cli.Get(ctx, r.prefixo+chave).Result()
	if err != nil {
		return "", false
	}
	return v, true
}

func (r *redisStore) Set(chave, valor string, ttl time.Duration) {
	ctx, cancelar := r.ctx()
	defer cancelar()
	if err := r.cli.Set(ctx, r.prefixo+chave, valor, ttl).Err(); err != nil {
		log.Printf("redis set %s: %v", chave, err)
	}
}

func (r *redisStore) Del(chaves ...string) {
	if len(chaves) == 0 {
		return
	}
	ctx, cancelar := r.ctx()
	defer cancelar()
	completas := make([]string, len(chaves))
	for i, c := range chaves {
		completas[i] = r.prefixo + c
	}
	_ = r.cli.Del(ctx, completas...).Err()
}

func (r *redisStore) Incr(chave string, ttl time.Duration) int64 {
	ctx, cancelar := r.ctx()
	defer cancelar()
	pipe := r.cli.TxPipeline()
	incr := pipe.Incr(ctx, r.prefixo+chave)
	// NX: o TTL é fixado na criação e não anda a cada incremento.
	pipe.ExpireNX(ctx, r.prefixo+chave, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("redis incr %s: %v", chave, err)
		return 0
	}
	return incr.Val()
}

func (r *redisStore) DelPrefixo(prefixo string) {
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	iter := r.cli.Scan(ctx, 0, r.prefixo+prefixo+"*", 200).Iterator()
	var lote []string
	for iter.Next(ctx) {
		lote = append(lote, iter.Val())
		if len(lote) >= 200 {
			_ = r.cli.Del(ctx, lote...).Err()
			lote = lote[:0]
		}
	}
	if len(lote) > 0 {
		_ = r.cli.Del(ctx, lote...).Err()
	}
}

// ---------------------------------------------------------------------------
// Memória (fallback e testes)
// ---------------------------------------------------------------------------

type itemMemoria struct {
	valor  string
	expira time.Time
}

type memoriaStore struct {
	mu    sync.Mutex
	itens map[string]itemMemoria
}

// NovoMemoria cria um store em memória com expiração por chave.
func NovoMemoria() Store { return &memoriaStore{itens: map[string]itemMemoria{}} }

func (m *memoriaStore) Nome() string { return "memoria" }

func (m *memoriaStore) vivo(chave string, agora time.Time) (itemMemoria, bool) {
	it, ok := m.itens[chave]
	if !ok {
		return it, false
	}
	if !it.expira.IsZero() && agora.After(it.expira) {
		delete(m.itens, chave)
		return it, false
	}
	return it, true
}

func (m *memoriaStore) Get(chave string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.vivo(chave, time.Now())
	if !ok {
		return "", false
	}
	return it.valor, true
}

func (m *memoriaStore) Set(chave, valor string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.faxina()
	it := itemMemoria{valor: valor}
	if ttl > 0 {
		it.expira = time.Now().Add(ttl)
	}
	m.itens[chave] = it
}

func (m *memoriaStore) Del(chaves ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range chaves {
		delete(m.itens, c)
	}
}

func (m *memoriaStore) Incr(chave string, ttl time.Duration) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	agora := time.Now()
	it, ok := m.vivo(chave, agora)
	var n int64
	if ok {
		for _, r := range it.valor {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int64(r-'0')
		}
	} else {
		it = itemMemoria{}
		if ttl > 0 {
			it.expira = agora.Add(ttl)
		}
	}
	n++
	it.valor = itoa(n)
	m.itens[chave] = it
	return n
}

func (m *memoriaStore) DelPrefixo(prefixo string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for c := range m.itens {
		if strings.HasPrefix(c, prefixo) {
			delete(m.itens, c)
		}
	}
}

// faxina remove itens vencidos quando o mapa cresce; chamada com o mutex travado.
func (m *memoriaStore) faxina() {
	if len(m.itens) < 5000 {
		return
	}
	agora := time.Now()
	for c, it := range m.itens {
		if !it.expira.IsZero() && agora.After(it.expira) {
			delete(m.itens, c)
		}
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
