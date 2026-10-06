// Package email dispara e-mails transacionais do painel pela API do Mandrill.
// Sem a chave, o envio é simulado: registra no log e devolve sucesso.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const urlPadrao = "https://mandrillapp.com/api/1.0"

type Config struct {
	Chave          string
	RemetenteEmail string
	RemetenteNome  string
	URL            string
}

type Cliente struct {
	cfg  Config
	http *http.Client
}

func Novo(cfg Config) *Cliente {
	if cfg.URL == "" {
		cfg.URL = urlPadrao
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return &Cliente{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Cliente) Simulado() bool { return c.cfg.Chave == "" }

type Mensagem struct {
	Para     string
	ParaNome string
	Assunto  string
	HTML     string
}

func (c *Cliente) Enviar(ctx context.Context, m Mensagem) error {
	if m.Para == "" {
		return errors.New("destinatário vazio")
	}
	if c.Simulado() {
		slog.Info("e-mail simulado", "para", m.Para, "assunto", m.Assunto)
		return nil
	}
	corpo := map[string]any{
		"key": c.cfg.Chave,
		"message": map[string]any{
			"from_email": c.cfg.RemetenteEmail, "from_name": c.cfg.RemetenteNome,
			"to":      []map[string]string{{"email": m.Para, "name": m.ParaNome, "type": "to"}},
			"subject": m.Assunto, "html": m.HTML,
		},
	}
	b, _ := json.Marshal(corpo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL+"/messages/send.json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Mandrill inacessível: %w", err)
	}
	defer resp.Body.Close()
	dados, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Mandrill respondeu %d: %s", resp.StatusCode, strings.TrimSpace(string(dados)))
	}
	var respostas []struct {
		Email  string `json:"email"`
		Status string `json:"status"`
		Motivo string `json:"reject_reason"`
	}
	if json.Unmarshal(dados, &respostas) == nil {
		for _, r := range respostas {
			if r.Status == "rejected" || r.Status == "invalid" {
				return fmt.Errorf("e-mail para %s %s (%s)", r.Email, r.Status, r.Motivo)
			}
		}
	}
	return nil
}

// Layout envolve o corpo no visual do produto (nunca cita a infraestrutura).
func Layout(produto, cor, titulo, corpo string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html lang="pt-BR"><body style="margin:0;background:#f5f4fb;font-family:Arial,Helvetica,sans-serif">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 0">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:12px;overflow:hidden">
<tr><td style="background:%s;padding:22px 32px;color:#fff;font-size:20px;font-weight:bold">%s</td></tr>
<tr><td style="padding:30px 32px;color:#333;font-size:14px;line-height:1.6"><h2 style="margin:0 0 14px;font-size:18px;color:#111">%s</h2>%s</td></tr>
<tr><td style="padding:14px 32px;background:#efedfe;color:#777;font-size:12px">%s · e-mail automático</td></tr>
</table></td></tr></table></body></html>`, cor, produto, titulo, corpo, produto)
}
