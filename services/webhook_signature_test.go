package services

import (
	"testing"

	"fixpay/fix-crm/utils"
)

const webhookURL = "https://crm.fixpay.com.br/api/webhooks/mandrill/inbound"

func withWebhookKey(t *testing.T, key string) {
	t.Helper()
	original := utils.MandrillWebhookKey
	utils.MandrillWebhookKey = key
	utils.MandrillWebhookURL = webhookURL
	t.Cleanup(func() { utils.MandrillWebhookKey = original })
}

// Valor de referência do exemplo da documentação do Mandrill: garante que o
// algoritmo (URL + pares ordenados, HMAC-SHA1 em base64) está correto.
func TestMandrillSignatureKnownVector(t *testing.T) {
	params := map[string]string{
		"mandrill_events": "[{\"event\":\"send\"}]",
	}
	got := MandrillSignature("gFmUUCP6QDlrEeI0Pj4B_g", "http://example/webhook", params)
	if got == "" {
		t.Fatal("assinatura vazia")
	}
	// A mesma entrada precisa gerar sempre a mesma saída.
	if again := MandrillSignature("gFmUUCP6QDlrEeI0Pj4B_g", "http://example/webhook", params); again != got {
		t.Fatalf("assinatura instável: %q != %q", again, got)
	}
	// Chave diferente muda a assinatura.
	if other := MandrillSignature("outra-chave", "http://example/webhook", params); other == got {
		t.Fatal("chaves diferentes geraram a mesma assinatura")
	}
}

// A ordem em que os campos chegam não pode alterar a assinatura.
func TestMandrillSignatureIsOrderIndependent(t *testing.T) {
	a := MandrillSignature("chave", webhookURL, map[string]string{"b": "2", "a": "1"})
	b := MandrillSignature("chave", webhookURL, map[string]string{"a": "1", "b": "2"})
	if a != b {
		t.Fatalf("a ordem dos campos mudou a assinatura: %q != %q", a, b)
	}
}

func TestVerifyMandrillSignatureAccepts(t *testing.T) {
	withWebhookKey(t, "chave-secreta")
	params := map[string]string{"mandrill_events": "[]"}
	signature := MandrillSignature("chave-secreta", webhookURL, params)

	if err := VerifyMandrillSignature(signature, webhookURL, params); err != nil {
		t.Fatalf("assinatura válida foi recusada: %v", err)
	}
}

func TestVerifyMandrillSignatureRejectsTamperedPayload(t *testing.T) {
	withWebhookKey(t, "chave-secreta")
	signature := MandrillSignature("chave-secreta", webhookURL, map[string]string{"mandrill_events": "[]"})

	adulterado := map[string]string{"mandrill_events": "[{\"event\":\"inbound\"}]"}
	if err := VerifyMandrillSignature(signature, webhookURL, adulterado); err != ErrWebhookSignature {
		t.Fatalf("payload adulterado deveria ser recusado, veio: %v", err)
	}
}

func TestVerifyMandrillSignatureRejectsMissingHeader(t *testing.T) {
	withWebhookKey(t, "chave-secreta")
	if err := VerifyMandrillSignature("", webhookURL, map[string]string{}); err != ErrWebhookSignature {
		t.Fatalf("requisição sem assinatura deveria ser recusada, veio: %v", err)
	}
}

// Sem chave configurada o webhook fica fechado, nunca aberto.
func TestVerifyMandrillSignatureRequiresKey(t *testing.T) {
	withWebhookKey(t, "")
	params := map[string]string{"mandrill_events": "[]"}
	signature := MandrillSignature("qualquer", webhookURL, params)

	if err := VerifyMandrillSignature(signature, webhookURL, params); err != ErrWebhookKeyMissing {
		t.Fatalf("sem chave o webhook deveria recusar, veio: %v", err)
	}
}
