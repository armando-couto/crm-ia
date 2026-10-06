// Package provisionador é o cliente HTTP do painel para o provisionador.
package provisionador

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Cliente struct {
	base  string
	token string
	http  *http.Client
}

func Novo(base, token string) *Cliente {
	return &Cliente{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 9 * time.Minute}}
}

type Pedido struct {
	Slug        string `json:"slug"`
	Nome        string `json:"nome"`
	CNPJ        string `json:"cnpj"`
	AdminNome   string `json:"admin_nome"`
	AdminEmail  string `json:"admin_email"`
	Versao      string `json:"versao"`
	UsuariosMax int    `json:"usuarios_max"`
	CorPrimaria string `json:"cor_primaria"`
	LogoURL     string `json:"logo_url"`
}

type Atualizacao struct {
	Versao      string  `json:"versao,omitempty"`
	UsuariosMax *int    `json:"usuarios_max,omitempty"`
	CorPrimaria *string `json:"cor_primaria,omitempty"`
	LogoURL     *string `json:"logo_url,omitempty"`
	Nome        *string `json:"nome,omitempty"`
}

type ContainerInfo struct {
	Nome   string `json:"nome"`
	Estado string `json:"estado"`
	Status string `json:"status"`
	Imagem string `json:"imagem"`
}

type Resultado struct {
	Slug       string          `json:"slug"`
	Status     string          `json:"status"`
	URL        string          `json:"url"`
	Versao     string          `json:"versao"`
	SenhaAdmin string          `json:"senha_admin"`
	Containers []ContainerInfo `json:"containers"`
	Versoes    struct {
		Configurada string `json:"configurada"`
		EmExecucao  string `json:"em_execucao"`
	} `json:"versoes"`
	Erro string `json:"erro"`
}

type Erro struct {
	Status   int
	Mensagem string
}

func (e *Erro) Error() string { return e.Mensagem }

func (c *Cliente) chamar(ctx context.Context, metodo, caminho string, corpo, saida any) error {
	var leitor io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return err
		}
		leitor = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.base+caminho, leitor)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("provisionador inacessível: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Erro string `json:"erro"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Erro == "" {
			e.Erro = strings.TrimSpace(string(b))
		}
		if e.Erro == "" {
			e.Erro = resp.Status
		}
		return &Erro{Status: resp.StatusCode, Mensagem: e.Erro}
	}
	if saida != nil && len(b) > 0 {
		return json.Unmarshal(b, saida)
	}
	return nil
}

func (c *Cliente) Criar(ctx context.Context, p Pedido) (Resultado, error) {
	var r Resultado
	err := c.chamar(ctx, http.MethodPost, "/tenants", p, &r)
	return r, err
}

func (c *Cliente) Atualizar(ctx context.Context, slug string, a Atualizacao) (Resultado, error) {
	var r Resultado
	err := c.chamar(ctx, http.MethodPost, "/tenants/"+slug+"/atualizar", a, &r)
	return r, err
}

func (c *Cliente) Status(ctx context.Context, slug string) (Resultado, error) {
	var r Resultado
	err := c.chamar(ctx, http.MethodGet, "/tenants/"+slug, nil, &r)
	return r, err
}

func (c *Cliente) Listar(ctx context.Context) ([]Resultado, error) {
	var lista []Resultado
	err := c.chamar(ctx, http.MethodGet, "/tenants", nil, &lista)
	return lista, err
}

func (c *Cliente) Suspender(ctx context.Context, slug string) error {
	return c.chamar(ctx, http.MethodPost, "/tenants/"+slug+"/suspender", map[string]any{}, nil)
}

func (c *Cliente) Reativar(ctx context.Context, slug string) error {
	return c.chamar(ctx, http.MethodPost, "/tenants/"+slug+"/reativar", map[string]any{}, nil)
}

func (c *Cliente) Backup(ctx context.Context, slug string) (string, error) {
	var r struct {
		Arquivo string `json:"arquivo"`
	}
	err := c.chamar(ctx, http.MethodPost, "/tenants/"+slug+"/backup", map[string]any{}, &r)
	return r.Arquivo, err
}

func (c *Cliente) Remover(ctx context.Context, slug string) error {
	return c.chamar(ctx, http.MethodDelete, "/tenants/"+slug, map[string]string{"confirmar": slug}, nil)
}

func (c *Cliente) TokenInterno(ctx context.Context, slug string) (string, error) {
	var r struct {
		Token string `json:"token"`
	}
	err := c.chamar(ctx, http.MethodGet, "/tenants/"+slug+"/token-interno", nil, &r)
	return r.Token, err
}

func (c *Cliente) VersoesDisponiveis(ctx context.Context) ([]string, error) {
	var r struct {
		Tags []string `json:"tags"`
	}
	err := c.chamar(ctx, http.MethodGet, "/versoes", nil, &r)
	return r.Tags, err
}
