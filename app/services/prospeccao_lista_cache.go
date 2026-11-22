package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sync"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Caches em memória das consultas gratuitas à plataforma (seção 6.1):
// catálogos por 1 hora e contagem por 60 segundos (hash dos filtros + versão
// da base). Poupam o limite de requisições do painel, que é por cliente: a
// contagem ao vivo de vários usuários repete muito os mesmos filtros.
// -----------------------------------------------------------------------------

const (
	cacheCatalogoValidade = time.Hour
	cacheContagemValidade = 60 * time.Second
	cacheMaxEntradas      = 2000
)

type entradaCache struct {
	valor  []byte
	expira time.Time
}

// cacheTTL é um mapa com validade por entrada. Ao encher, descarta as vencidas
// e, se ainda assim não couber, esvazia (os dados são baratos de refazer).
type cacheTTL struct {
	mu       sync.Mutex
	entradas map[string]entradaCache
}

func (c *cacheTTL) ler(chave string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entradas[chave]
	if !ok || time.Now().After(e.expira) {
		delete(c.entradas, chave)
		return nil, false
	}
	return e.valor, true
}

func (c *cacheTTL) gravar(chave string, valor []byte, validade time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entradas == nil {
		c.entradas = map[string]entradaCache{}
	}
	if len(c.entradas) >= cacheMaxEntradas {
		agora := time.Now()
		for k, e := range c.entradas {
			if agora.After(e.expira) {
				delete(c.entradas, k)
			}
		}
		if len(c.entradas) >= cacheMaxEntradas {
			c.entradas = map[string]entradaCache{}
		}
	}
	c.entradas[chave] = entradaCache{valor: valor, expira: time.Now().Add(validade)}
}

func (c *cacheTTL) limpar() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entradas = nil
}

var (
	cacheCatalogos cacheTTL
	cacheContagens cacheTTL

	versaoBaseMu        sync.RWMutex
	versaoBaseConhecida string
)

// LimparCachesProspeccao esvazia os caches (testes).
func LimparCachesProspeccao() {
	cacheCatalogos.limpar()
	cacheContagens.limpar()
	versaoBaseMu.Lock()
	versaoBaseConhecida = ""
	versaoBaseMu.Unlock()
}

// PlataformaResumoComVersao é PlataformaResumo guardando a versão da base
// para a chave do cache de contagem: base nova, contagens novas.
func PlataformaResumoComVersao(ctx context.Context) (*models.ProspResumoPlataforma, error) {
	r, err := PlataformaResumo(ctx)
	if err == nil && r != nil && r.Base.Versao != "" {
		versaoBaseMu.Lock()
		versaoBaseConhecida = r.Base.Versao
		versaoBaseMu.Unlock()
	}
	return r, err
}

// PlataformaCatalogoComCache repassa o catálogo guardando a resposta por 1 h.
// A chave é o nome mais a query string em ordem canônica.
func PlataformaCatalogoComCache(ctx context.Context, nome string, consulta url.Values) (json.RawMessage, error) {
	chave := nome + "?" + consulta.Encode()
	if len(chave) <= 2048 {
		if v, ok := cacheCatalogos.ler(chave); ok {
			return v, nil
		}
	}
	out, err := PlataformaCatalogo(ctx, nome, consulta)
	if err != nil {
		return nil, err
	}
	if len(chave) <= 2048 {
		cacheCatalogos.gravar(chave, out, cacheCatalogoValidade)
	}
	return out, nil
}

// PlataformaContagemComCache conta as empresas do filtro, reaproveitando por
// 60 s a resposta de um filtro idêntico na mesma versão da base.
func PlataformaContagemComCache(ctx context.Context, filtros models.ProspFiltros) (*models.ProspContagem, error) {
	bruto, _ := json.Marshal(filtros)
	versaoBaseMu.RLock()
	versao := versaoBaseConhecida
	versaoBaseMu.RUnlock()
	soma := sha256.Sum256(append(bruto, versao...))
	chave := hex.EncodeToString(soma[:])
	if v, ok := cacheContagens.ler(chave); ok {
		var c models.ProspContagem
		if json.Unmarshal(v, &c) == nil {
			return &c, nil
		}
	}
	c, err := PlataformaContagem(ctx, filtros)
	if err != nil {
		return nil, err
	}
	if b, err := json.Marshal(c); err == nil {
		cacheContagens.gravar(chave, b, cacheContagemValidade)
	}
	return c, nil
}
