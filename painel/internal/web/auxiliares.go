package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
)

// dados é o que todo template recebe.
type dados struct {
	Titulo  string
	Secao   string
	Sessao  auth.Sessao
	CSRF    string
	Sucesso string
	Erro    string
	Aviso   string
	Dominio string
	Produto string
	V       map[string]any
}

func (s *Servidor) base(r *http.Request, titulo, secao string) dados {
	d := dados{Titulo: titulo, Secao: secao, Sessao: sessaoDe(r), CSRF: s.sessoes.TokenCSRF(r),
		Dominio: s.cfg.Dominio, Produto: s.cfg.NomeProduto, V: map[string]any{}}
	q := r.URL.Query()
	d.Sucesso, d.Erro, d.Aviso = q.Get("ok"), q.Get("erro"), q.Get("aviso")
	return d
}

func (s *Servidor) render(w http.ResponseWriter, r *http.Request, pagina string, d dados) {
	t, ok := s.templates["pagina_"+pagina]
	if !ok {
		slog.Error("template não encontrado", "pagina", pagina)
		s.erroHTTP(w, r, http.StatusInternalServerError, "Página não encontrada no servidor.")
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", d); err != nil {
		slog.Error("renderizando template", "pagina", pagina, "erro", err)
		s.erroHTTP(w, r, http.StatusInternalServerError, "Erro ao montar a página.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = buf.WriteTo(w)
}

func (s *Servidor) erroHTTP(w http.ResponseWriter, r *http.Request, status int, mensagem string) {
	d := s.base(r, "Erro", "erro")
	d.Erro = mensagem
	d.V["Status"] = status
	t, ok := s.templates["pagina_erro"]
	w.WriteHeader(status)
	if !ok {
		_, _ = w.Write([]byte(mensagem))
		return
	}
	_ = t.ExecuteTemplate(w, "layout.html", d)
}

func (s *Servidor) redirecionar(w http.ResponseWriter, r *http.Request, caminho, tipo, msg string) {
	destino := s.url(caminho)
	if msg != "" {
		sep := "?"
		if strings.Contains(destino, "?") {
			sep = "&"
		}
		destino += sep + tipo + "=" + url.QueryEscape(msg)
	}
	http.Redirect(w, r, destino, http.StatusSeeOther)
}

func (s *Servidor) json(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

func (s *Servidor) auditar(r *http.Request, acao, entidade string, entidadeID *int64, detalhes map[string]any) {
	sessao := sessaoDe(r)
	var uid *int64
	if sessao.UsuarioID != 0 {
		uid = &sessao.UsuarioID
	}
	s.repo.Auditar(r.Context(), uid, sessao.Email, acao, entidade, entidadeID, detalhes, ipDe(r))
}

// senhaUnica guarda a senha de administrador gerada para exibir UMA vez na
// ficha do cliente (nunca na URL).
type senhaUnica struct {
	Titulo, Email, Senha string
	Expira               time.Time
}

func (s *Servidor) guardarSenhaUnica(r *http.Request, clienteID int64, titulo, email, senha string) {
	s.senhasMu.Lock()
	defer s.senhasMu.Unlock()
	s.senhas[chaveSenha(r, clienteID)] = senhaUnica{Titulo: titulo, Email: email, Senha: senha, Expira: time.Now().Add(10 * time.Minute)}
}

func (s *Servidor) tomarSenhaUnica(r *http.Request, clienteID int64) (senhaUnica, bool) {
	s.senhasMu.Lock()
	defer s.senhasMu.Unlock()
	k := chaveSenha(r, clienteID)
	su, ok := s.senhas[k]
	if !ok {
		return su, false
	}
	delete(s.senhas, k)
	return su, time.Now().Before(su.Expira)
}

func chaveSenha(r *http.Request, clienteID int64) string {
	return strconv.FormatInt(sessaoDe(r).UsuarioID, 10) + ":" + strconv.FormatInt(clienteID, 10)
}

func idDaRota(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}

func campo(r *http.Request, nome string) string { return strings.TrimSpace(r.PostFormValue(nome)) }

func campoInt(r *http.Request, nome string, padrao int) int {
	n, err := strconv.Atoi(campo(r, nome))
	if err != nil {
		return padrao
	}
	return n
}

func campoInt64Ptr(r *http.Request, nome string) *int64 {
	n, err := strconv.ParseInt(campo(r, nome), 10, 64)
	if err != nil || n == 0 {
		return nil
	}
	return &n
}

func campoBool(r *http.Request, nome string) bool {
	v := campo(r, nome)
	return v == "on" || v == "true" || v == "1"
}

// campoCentavos aceita "199,90", "199.90" ou "199".
func campoCentavos(r *http.Request, nome string) int64 {
	v := strings.ReplaceAll(campo(r, nome), ".", "")
	v = strings.ReplaceAll(v, ",", ".")
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return int64(f*100 + 0.5)
}

var naoDigito = regexp.MustCompile(`\D`)

func somenteDigitos(v string) string { return naoDigito.ReplaceAllString(v, "") }

func cnpjValido(cnpj string) bool {
	cnpj = somenteDigitos(cnpj)
	if len(cnpj) != 14 {
		return false
	}
	iguais := true
	for i := 1; i < 14; i++ {
		if cnpj[i] != cnpj[0] {
			iguais = false
			break
		}
	}
	if iguais {
		return false
	}
	calc := func(base string, pesos []int) int {
		soma := 0
		for i, p := range pesos {
			soma += int(base[i]-'0') * p
		}
		resto := soma % 11
		if resto < 2 {
			return 0
		}
		return 11 - resto
	}
	d1 := calc(cnpj[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	d2 := calc(cnpj[:13], []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return int(cnpj[12]-'0') == d1 && int(cnpj[13]-'0') == d2
}

var slugLimpar = regexp.MustCompile(`[^a-z0-9]+`)

// gerarSlug: "Minha Empresa" → "minhaempresa".
func gerarSlug(nome string) string {
	mapa := map[rune]string{'á': "a", 'à': "a", 'ã': "a", 'â': "a", 'ä': "a", 'é': "e", 'è': "e", 'ê': "e", 'ë': "e",
		'í': "i", 'ì': "i", 'î': "i", 'ï': "i", 'ó': "o", 'ò': "o", 'õ': "o", 'ô': "o", 'ö': "o",
		'ú': "u", 'ù': "u", 'û': "u", 'ü': "u", 'ç': "c", 'ñ': "n"}
	var b strings.Builder
	for _, r := range strings.ToLower(nome) {
		if t, ok := mapa[r]; ok {
			b.WriteString(t)
		} else {
			b.WriteRune(r)
		}
	}
	s := slugLimpar.ReplaceAllString(b.String(), "")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func ipDe(r *http.Request) string {
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	return strings.Trim(ip, "[]")
}

func erroAmigavel(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
