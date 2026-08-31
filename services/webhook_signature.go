package services

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"sort"
	"strings"

	"fixpay/fix-crm/utils"
)

// ErrWebhookKeyMissing indica que a chave do webhook não foi configurada:
// sem ela o endpoint é recusado (nunca aberto por omissão).
var ErrWebhookKeyMissing = errors.New("mandrill_webhook_key não configurada no .env: o webhook fica desativado")

// ErrWebhookSignature indica assinatura ausente ou inválida.
var ErrWebhookSignature = errors.New("assinatura do webhook inválida")

// MandrillSignature calcula a assinatura esperada pelo Mandrill: HMAC-SHA1 da
// URL do webhook concatenada com cada par chave+valor do POST, ordenado pela
// chave, usando a webhook key como segredo (resultado em base64).
func MandrillSignature(key, url string, params map[string]string) string {
	var sb strings.Builder
	sb.WriteString(url)

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(sb.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyMandrillSignature valida o cabeçalho X-Mandrill-Signature da requisição.
// A URL usada no cálculo precisa ser exatamente a cadastrada no painel do
// Mandrill (chave mandrill_webhook_url do .env, carregada em utils.LoadConfig).
func VerifyMandrillSignature(signature, url string, params map[string]string) error {
	if utils.MandrillWebhookKey == "" {
		return ErrWebhookKeyMissing
	}
	if signature == "" {
		return ErrWebhookSignature
	}
	if url == "" {
		url = utils.MandrillWebhookURL
	}

	expected := MandrillSignature(utils.MandrillWebhookKey, url, params)
	// Comparação em tempo constante evita vazar a assinatura por timing.
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrWebhookSignature
	}
	return nil
}
