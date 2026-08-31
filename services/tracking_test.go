package services

import (
	"encoding/base64"
	"strings"
	"testing"

	"fixpay/fix-crm/utils"
)

func withAppURL(t *testing.T, url string) {
	t.Helper()
	original := utils.AppURL
	utils.AppURL = url
	t.Cleanup(func() { utils.AppURL = original })
}

func TestInstrumentEmailAddsPixel(t *testing.T) {
	withAppURL(t, "https://crm.fixpay.com.br")

	html := InstrumentEmailHTML("<p>Olá</p>", "tok123")
	if !strings.Contains(html, "https://crm.fixpay.com.br/api/track/o/tok123/pixel.gif") {
		t.Fatalf("pixel de abertura ausente: %s", html)
	}
	if !strings.Contains(html, "<p>Olá</p>") {
		t.Fatal("o conteúdo original deveria ser preservado")
	}
}

// Com layout completo, o pixel entra antes do </body>.
func TestInstrumentEmailPixelBeforeBodyClose(t *testing.T) {
	withAppURL(t, "https://crm.fixpay.com.br")

	html := InstrumentEmailHTML("<html><body><p>Oi</p></body></html>", "tok")
	pixel := strings.Index(html, "pixel.gif")
	closing := strings.Index(html, "</body>")
	if pixel < 0 || closing < 0 || pixel > closing {
		t.Fatalf("pixel deveria vir antes de </body>: %s", html)
	}
}

func TestInstrumentEmailRewritesLinks(t *testing.T) {
	withAppURL(t, "https://crm.fixpay.com.br")

	html := InstrumentEmailHTML(`<a href="https://fixpay.com.br/planos">planos</a>`, "tok")
	if !strings.Contains(html, "/api/track/c/tok?u=") {
		t.Fatalf("link não foi reescrito: %s", html)
	}
	if strings.Contains(html, `href="https://fixpay.com.br/planos"`) {
		t.Fatalf("o link original não deveria continuar direto: %s", html)
	}
}

// mailto:, tel: e âncoras continuam intactos.
func TestInstrumentEmailKeepsNonHTTPLinks(t *testing.T) {
	withAppURL(t, "https://crm.fixpay.com.br")

	html := InstrumentEmailHTML(
		`<a href="mailto:ana@fixpay.com.br">e-mail</a><a href="tel:1130000000">tel</a><a href="#topo">topo</a>`,
		"tok")
	for _, keep := range []string{`href="mailto:ana@fixpay.com.br"`, `href="tel:1130000000"`, `href="#topo"`} {
		if !strings.Contains(html, keep) {
			t.Fatalf("%s deveria ficar intacto: %s", keep, html)
		}
	}
}

func TestDecodeTrackedURL(t *testing.T) {
	encoded := base64.URLEncoding.EncodeToString([]byte("https://fixpay.com.br/planos"))
	got, ok := DecodeTrackedURL(encoded)
	if !ok || got != "https://fixpay.com.br/planos" {
		t.Fatalf("decodificação falhou: %q ok=%v", got, ok)
	}
}

// O redirecionador não pode virar ponte para javascript:/file:/data:.
func TestDecodeTrackedURLRejectsOtherSchemes(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "data:text/html,<b>x"} {
		encoded := base64.URLEncoding.EncodeToString([]byte(raw))
		if _, ok := DecodeTrackedURL(encoded); ok {
			t.Fatalf("%q deveria ser recusado", raw)
		}
	}
	if _, ok := DecodeTrackedURL("não-é-base64!!"); ok {
		t.Fatal("base64 inválido deveria ser recusado")
	}
}

func TestTrackingPixelIsAGIF(t *testing.T) {
	if len(TrackingPixel) < 6 || string(TrackingPixel[:6]) != "GIF89a" {
		t.Fatalf("pixel não parece um GIF: %v", TrackingPixel[:6])
	}
}
