// Package auth cuida de sessão (cookie assinado), CSRF e senhas do painel.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/armando-couto/crm-ia/painel/internal/modelo"
)

const (
	CookieSessao   = "crmia_painel_sessao"
	CookieCSRF     = "crmia_painel_csrf"
	CampoCSRF      = "_csrf"
	ValidadeSessao = 8 * time.Hour
	custoBcrypt    = 12
)

var (
	ErrSemSessao      = errors.New("sem sessão")
	ErrSessaoInvalida = errors.New("sessão inválida")
	ErrSessaoExpirada = errors.New("sessão expirada")
	ErrCSRFInvalido   = errors.New("token CSRF inválido ou ausente")
)

type Sessao struct {
	UsuarioID int64         `json:"uid"`
	Nome      string        `json:"n"`
	Email     string        `json:"e"`
	Perfil    modelo.Perfil `json:"p"`
	Expira    int64         `json:"x"`
	Impressao string        `json:"i"`
}

func (s Sessao) Admin() bool { return s.Perfil == modelo.PerfilAdmin }

func (s Sessao) PodeEditarClientes() bool {
	return s.Perfil == modelo.PerfilAdmin || s.Perfil == modelo.PerfilComercial
}
func (s Sessao) PodeEditarLicencas() bool {
	return s.Perfil == modelo.PerfilAdmin || s.Perfil == modelo.PerfilComercial
}
func (s Sessao) PodeEditarFaturas() bool {
	return s.Perfil == modelo.PerfilAdmin || s.Perfil == modelo.PerfilFinanceiro
}
func (s Sessao) PodeProvisionar() bool {
	return s.Perfil == modelo.PerfilAdmin || s.Perfil == modelo.PerfilSuporte
}
func (s Sessao) PodeSuspender() bool {
	return s.Perfil == modelo.PerfilAdmin || s.Perfil == modelo.PerfilFinanceiro
}
func (s Sessao) PodeEditarPlanos() bool          { return s.Perfil == modelo.PerfilAdmin }
func (s Sessao) PodeGerenciarUsuarios() bool     { return s.Perfil == modelo.PerfilAdmin }
func (s Sessao) PodeRedefinirSenhaCliente() bool { return s.Perfil == modelo.PerfilAdmin }

type Gerenciador struct {
	chaveSessao []byte
	chaveCSRF   []byte
	caminho     string
	seguro      bool
}

func NovoGerenciador(chaveSessao, chaveCSRF, caminho string, seguro bool) *Gerenciador {
	if caminho == "" {
		caminho = "/"
	}
	return &Gerenciador{chaveSessao: []byte(chaveSessao), chaveCSRF: []byte(chaveCSRF), caminho: caminho, seguro: seguro}
}

// Impressao resume o hash da senha: trocar a senha derruba as sessões antigas.
func Impressao(senhaHash string) string {
	h := sha256.Sum256([]byte(senhaHash))
	return base64.RawURLEncoding.EncodeToString(h[:8])
}

func (g *Gerenciador) Iniciar(w http.ResponseWriter, u modelo.Usuario) error {
	s := Sessao{UsuarioID: u.ID, Nome: u.Nome, Email: u.Email, Perfil: u.Perfil,
		Expira: time.Now().Add(ValidadeSessao).Unix(), Impressao: Impressao(u.SenhaHash)}
	valor, err := g.assinar(s)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: CookieSessao, Value: valor, Path: g.caminho, HttpOnly: true,
		Secure: g.seguro, SameSite: http.SameSiteLaxMode, Expires: time.Unix(s.Expira, 0)})
	csrf, err := g.novoCSRF()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: CookieCSRF, Value: csrf, Path: g.caminho, HttpOnly: true,
		Secure: g.seguro, SameSite: http.SameSiteLaxMode, Expires: time.Unix(s.Expira, 0)})
	return nil
}

func (g *Gerenciador) Encerrar(w http.ResponseWriter) {
	for _, nome := range []string{CookieSessao, CookieCSRF} {
		http.SetCookie(w, &http.Cookie{Name: nome, Value: "", Path: g.caminho, MaxAge: -1, HttpOnly: true, Secure: g.seguro, SameSite: http.SameSiteLaxMode})
	}
}

func (g *Gerenciador) Ler(r *http.Request) (Sessao, error) {
	c, err := r.Cookie(CookieSessao)
	if err != nil {
		return Sessao{}, ErrSemSessao
	}
	s, err := g.verificar(c.Value)
	if err != nil {
		return Sessao{}, err
	}
	if time.Now().Unix() > s.Expira {
		return Sessao{}, ErrSessaoExpirada
	}
	return s, nil
}

func (g *Gerenciador) TokenCSRF(r *http.Request) string {
	c, err := r.Cookie(CookieCSRF)
	if err != nil {
		return ""
	}
	return c.Value
}

func (g *Gerenciador) ValidarCSRF(r *http.Request) error {
	c, err := r.Cookie(CookieCSRF)
	if err != nil || c.Value == "" {
		return ErrCSRFInvalido
	}
	enviado := r.Header.Get("X-CSRF-Token")
	if enviado == "" {
		enviado = r.PostFormValue(CampoCSRF)
	}
	if enviado == "" || !g.csrfValido(c.Value) || subtle.ConstantTimeCompare([]byte(enviado), []byte(c.Value)) != 1 {
		return ErrCSRFInvalido
	}
	return nil
}

func (g *Gerenciador) assinar(s Sessao) (string, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	corpo := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, g.chaveSessao)
	mac.Write([]byte(corpo))
	return corpo + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (g *Gerenciador) verificar(valor string) (Sessao, error) {
	corpo, assinatura, ok := strings.Cut(valor, ".")
	if !ok {
		return Sessao{}, ErrSessaoInvalida
	}
	mac := hmac.New(sha256.New, g.chaveSessao)
	mac.Write([]byte(corpo))
	if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), []byte(assinatura)) != 1 {
		return Sessao{}, ErrSessaoInvalida
	}
	payload, err := base64.RawURLEncoding.DecodeString(corpo)
	if err != nil {
		return Sessao{}, ErrSessaoInvalida
	}
	var s Sessao
	if err := json.Unmarshal(payload, &s); err != nil {
		return Sessao{}, ErrSessaoInvalida
	}
	return s, nil
}

func (g *Gerenciador) novoCSRF() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	corpo := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, g.chaveCSRF)
	mac.Write([]byte(corpo))
	return corpo + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:12]), nil
}

func (g *Gerenciador) csrfValido(token string) bool {
	corpo, assinatura, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	mac := hmac.New(sha256.New, g.chaveCSRF)
	mac.Write([]byte(corpo))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:12])), []byte(assinatura)) == 1
}

func HashSenha(senha string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(senha), custoBcrypt)
	return string(h), err
}

func SenhaConfere(hash, senha string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(senha)) == nil
}

// ForcaSenhaOK exige 10+ caracteres com letra e número.
func ForcaSenhaOK(senha string) error {
	if len(senha) < 10 {
		return fmt.Errorf("a senha precisa ter ao menos 10 caracteres")
	}
	var letra, digito bool
	for _, r := range senha {
		switch {
		case unicode.IsLetter(r):
			letra = true
		case unicode.IsDigit(r):
			digito = true
		}
	}
	if !letra || !digito {
		return fmt.Errorf("a senha precisa misturar letras e números")
	}
	return nil
}

// CodigoAleatorio gera um identificador URL-safe (códigos de checkout).
func CodigoAleatorio(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
