package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/basepublica"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
)

// -----------------------------------------------------------------------------
// Página "Base de dados": situação do serviço `dados` e uso da prospecção
// -----------------------------------------------------------------------------

func (s *Servidor) paginaBaseDeDados(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.base(r, "Base de dados", "dados")
	d.V["Configurada"] = s.publica.Configurado()
	if s.publica.Configurado() {
		if sit, err := s.publica.Situacao(ctx); err == nil {
			d.V["Situacao"] = sit
		} else {
			d.V["SituacaoErro"] = err.Error()
		}
	}
	competencia := operacoes.Competencia(time.Now())
	d.V["Mes"] = competencia.Format("01/2006")
	uso, err := s.repo.UsoProspeccaoPorCliente(ctx, competencia)
	if err != nil {
		d.Erro = err.Error()
	}
	for i := range uso {
		if c, err := s.repo.ClientePorID(ctx, uso[i].ClienteID); err == nil {
			uso[i].Limite, uso[i].Plano, _ = s.ops.LimiteProspeccao(ctx, c)
		}
	}
	d.V["Uso"] = uso
	s.render(w, r, "dados", d)
}

func (s *Servidor) carregarBaseDeDados(w http.ResponseWriter, r *http.Request) {
	const pagina = "/base-de-dados"
	fonte := campo(r, "fonte")
	if fonte != "cnpj" && fonte != "pgfn" {
		fonte = "tudo"
	}
	err := s.publica.IniciarCarga(r.Context(), fonte)
	var e *basepublica.Erro
	switch {
	case err == nil:
		s.auditar(r, "carregar", "base_publica", nil, map[string]any{"fonte": fonte})
		s.redirecionar(w, r, pagina, "ok", "Verificação iniciada: se houver referência nova, a carga começa em seguida.")
	case errors.Is(err, basepublica.ErrNaoConfigurado):
		s.redirecionar(w, r, pagina, "erro", "O serviço de dados não está configurado (DADOS_URL e DADOS_TOKEN).")
	case errors.As(err, &e) && e.Status == http.StatusConflict:
		s.redirecionar(w, r, pagina, "aviso", "Não iniciei outra carga: "+e.Mensagem+".")
	default:
		s.redirecionar(w, r, pagina, "erro", "Não consegui falar com o serviço de dados: "+err.Error())
	}
}

// milhar formata um inteiro com ponto de milhar: 28700000 → "28.700.000".
func milhar(v any) string {
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int32:
		n = int64(x)
	case int64:
		n = x
	case float64:
		n = int64(x)
	default:
		return fmt.Sprint(v)
	}
	texto := strconv.FormatInt(n, 10)
	sinal := ""
	if n < 0 {
		sinal, texto = "-", texto[1:]
	}
	var b strings.Builder
	for i, r := range texto {
		if i > 0 && (len(texto)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return sinal + b.String()
}

// tamanhoLegivel: bytes em KB, MB ou GB, com vírgula decimal.
func tamanhoLegivel(n int64) string {
	const kb = 1024.0
	v := float64(n)
	var texto string
	switch {
	case v >= kb*kb*kb:
		texto = fmt.Sprintf("%.1f GB", v/(kb*kb*kb))
	case v >= kb*kb:
		texto = fmt.Sprintf("%.1f MB", v/(kb*kb))
	case v >= kb:
		texto = fmt.Sprintf("%.0f KB", v/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
	return strings.Replace(texto, ".", ",", 1)
}
