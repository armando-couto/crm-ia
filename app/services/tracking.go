package services

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// hrefPattern captura o destino de cada link do corpo do e-mail.
var hrefPattern = regexp.MustCompile(`(?i)href\s*=\s*"([^"]+)"`)

// TrackOutgoingEmail registra o envio e devolve o HTML já instrumentado:
// links passam pelo redirecionador e o rodapé ganha o pixel de abertura.
// Se o registro falhar, devolve o HTML original — rastreio nunca impede o envio.
func TrackOutgoingEmail(db *sql.DB, msg *models.EmailMessage) (string, error) {
	token, err := RandomToken()
	if err != nil {
		return msg.Body, err
	}
	msg.Token = token

	if err := models.CreateEmailMessage(db, msg); err != nil {
		return msg.Body, err
	}
	return InstrumentEmailHTML(msg.Body, token), nil
}

// InstrumentEmailHTML reescreve os links e acrescenta o pixel de abertura.
func InstrumentEmailHTML(html, token string) string {
	base := strings.TrimSuffix(utils.AppURL, "/")

	instrumented := hrefPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := hrefPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		target := parts[1]
		// mailto:, tel: e âncoras não são clique rastreável.
		lower := strings.ToLower(strings.TrimSpace(target))
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return match
		}
		tracked := fmt.Sprintf("%s/api/track/c/%s?u=%s",
			base, token, url.QueryEscape(base64.URLEncoding.EncodeToString([]byte(target))))
		return fmt.Sprintf(`href="%s"`, tracked)
	})

	pixel := fmt.Sprintf(
		`<img src="%s/api/track/o/%s/pixel.gif" width="1" height="1" alt="" style="display:none">`,
		base, token)

	// O pixel entra antes de </body> quando existe layout; senão, no fim.
	if idx := strings.LastIndex(strings.ToLower(instrumented), "</body>"); idx >= 0 {
		return instrumented[:idx] + pixel + instrumented[idx:]
	}
	return instrumented + pixel
}

// DecodeTrackedURL desfaz o base64 do link rastreado e só aceita http/https,
// para o redirecionador não virar um "open redirect" para qualquer esquema.
func DecodeTrackedURL(encoded string) (string, bool) {
	raw, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	target := string(raw)
	parsed, err := url.Parse(target)
	if err != nil {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	return target, true
}

// TrackingPixel é um GIF 1x1 transparente devolvido na abertura.
var TrackingPixel = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
	0x44, 0x01, 0x00, 0x3b,
}
