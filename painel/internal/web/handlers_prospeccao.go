package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/armando-couto/crm-ia/painel/internal/basepublica"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
)

// -----------------------------------------------------------------------------
// Prospecção: API para os ambientes dos clientes (TOKEN_INTERNO do tenant)
//
// O app nunca fala com o serviço `dados`: fala com o painel, que autentica o
// ambiente, aplica a cota do plano, mede o uso e repassa. Contrato em
// docs/PROSPECCAO.md (seção 4) e docs/PROSPECCAO-ADENDO.md.
//
// Todo erro sai com as duas chaves de mensagem ("erro" para o padrão do painel,
// "error" porque é a que o app lê) e com "codigo".
// -----------------------------------------------------------------------------

// Códigos de erro da API de prospecção.
const (
	codNaoAutorizado  = "nao_autorizado"
	codNaoEncontrado  = "nao_encontrado"
	codFiltroInvalido = "filtro_invalido"    // 422
	codCota           = "cota"               // 429, com "cota"
	codLimite         = "limite_requisicoes" // 429, com Retry-After
	codBaseEmCarga    = "base_em_carga"      // 503: serviço fora do ar ou base ainda não carregada
	codBaseAtualizada = "base_atualizada"    // 409: cursor de versão antiga
	codInterno        = "erro_interno"
)

// Limites por cliente e por minuto (janela deslizante, em memória).
const (
	limiteContagem   = 600
	limitePrevia     = 60
	limiteCatalogos  = 600
	limiteCandidatos = 30
	limiteCNPJ       = 60
	// limiteCNPJDia: fichas avulsas por dia e por cliente (contadas no banco).
	limiteCNPJDia = 300
	// Corpo máximo de um pedido (filtros com até 5.000 CNPJs cabem com folga).
	corpoMaximoProspeccao = 1 << 20
)

const (
	msgBaseAusente      = "a base de empresas não está disponível nesta instalação"
	msgBaseForaDoAr     = "a base de empresas não está respondendo; tente em alguns minutos"
	msgPlanoSemProspec  = "seu plano não inclui prospecção"
	msgMuitasConsultas  = "muitas consultas em pouco tempo, aguarde alguns segundos"
	msgCotaEsgotada     = "cota mensal de prospecção esgotada"
	msgLimiteDiarioCNPJ = "limite diário de consultas de CNPJ atingido"
)

const chaveCliente chaveCtx = "cliente"

func clienteDe(r *http.Request) modelo.Cliente {
	c, _ := r.Context().Value(chaveCliente).(modelo.Cliente)
	return c
}

// erroAPI responde um erro da API de prospecção.
func (s *Servidor) erroAPI(w http.ResponseWriter, status int, codigo, mensagem string, extras map[string]any) {
	corpo := map[string]any{"erro": mensagem, "error": mensagem, "codigo": codigo}
	for k, v := range extras {
		corpo[k] = v
	}
	s.json(w, status, corpo)
}

// exigirAmbiente autentica o ambiente pelo TOKEN_INTERNO do slug e deixa o
// cliente no contexto.
func (s *Servidor) exigirAmbiente(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if err := s.tokenDoTenant(r, slug); err != nil {
			s.erroAPI(w, http.StatusUnauthorized, codNaoAutorizado, "não autorizado", nil)
			return
		}
		c, err := s.repo.ClientePorSlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, repo.ErrNaoEncontrado) {
				s.erroAPI(w, http.StatusNotFound, codNaoEncontrado, "empresa não encontrada", nil)
				return
			}
			s.erroInterno(w, r, err)
			return
		}
		proximo.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), chaveCliente, c)))
	})
}

func (s *Servidor) erroInterno(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("prospecção: erro interno", "caminho", r.URL.Path, "erro", err)
	s.erroAPI(w, http.StatusInternalServerError, codInterno, "erro interno; tente de novo em instantes", nil)
}

// dentroDoLimite aplica o limite por minuto da rota ao cliente. Estourou:
// responde 429 com Retry-After e devolve falso.
func (s *Servidor) dentroDoLimite(w http.ResponseWriter, r *http.Request, rota string, porMinuto int) bool {
	ok, espera := s.limites.permitir(clienteDe(r).Slug+"|"+rota, porMinuto, time.Minute)
	if ok {
		return true
	}
	s.limiteEstourado(w, msgMuitasConsultas, espera)
	return false
}

func (s *Servidor) limiteEstourado(w http.ResponseWriter, mensagem string, espera time.Duration) {
	segundos := int(math.Ceil(espera.Seconds()))
	if segundos < 1 {
		segundos = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(segundos))
	s.erroAPI(w, http.StatusTooManyRequests, codLimite, mensagem, map[string]any{"tentar_em_segundos": segundos})
}

// falhaDaBase traduz um erro do cliente do serviço `dados` em resposta.
func (s *Servidor) falhaDaBase(w http.ResponseWriter, r *http.Request, err error) {
	var e *basepublica.Erro
	switch {
	case errors.Is(err, basepublica.ErrNaoConfigurado):
		s.erroAPI(w, http.StatusServiceUnavailable, codBaseEmCarga, msgBaseAusente, nil)
	case errors.As(err, &e):
		s.statusDaBase(w, r, e.Status, e.Mensagem)
	default:
		slog.Warn("prospecção: serviço dados inacessível", "caminho", r.URL.Path, "erro", err)
		s.erroAPI(w, http.StatusServiceUnavailable, codBaseEmCarga, msgBaseForaDoAr, nil)
	}
}

// statusDaBase traduz um status de erro devolvido pelo serviço `dados`.
func (s *Servidor) statusDaBase(w http.ResponseWriter, r *http.Request, status int, mensagem string) {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, mensagem, nil)
	case http.StatusConflict:
		s.erroAPI(w, http.StatusConflict, codBaseAtualizada, mensagem, nil)
	case http.StatusNotFound:
		s.erroAPI(w, http.StatusNotFound, codNaoEncontrado, mensagem, nil)
	case http.StatusServiceUnavailable:
		s.erroAPI(w, http.StatusServiceUnavailable, codBaseEmCarga, mensagem, nil)
	default:
		// 401 (token errado), 429, 5xx: problema da plataforma, não do pedido.
		slog.Error("prospecção: resposta inesperada do serviço dados", "caminho", r.URL.Path, "status", status, "mensagem", mensagem)
		s.erroAPI(w, http.StatusServiceUnavailable, codBaseEmCarga, msgBaseForaDoAr, nil)
	}
}

// repassar chama o serviço `dados` e devolve a resposta como veio (2xx) ou o
// erro traduzido.
func (s *Servidor) repassar(w http.ResponseWriter, r *http.Request, metodo, caminho string, consulta url.Values, corpo []byte) {
	status, resposta, err := s.publica.Repassar(r.Context(), metodo, caminho, consulta, corpo)
	if err != nil {
		s.falhaDaBase(w, r, err)
		return
	}
	if status >= 300 {
		s.statusDaBase(w, r, status, basepublica.MensagemDeErro(status, resposta))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(resposta)
}

// lerCorpo lê o JSON do pedido (objeto). Falha: responde 422 e devolve falso.
func (s *Servidor) lerCorpo(w http.ResponseWriter, r *http.Request) ([]byte, map[string]json.RawMessage, bool) {
	corpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, corpoMaximoProspeccao))
	if err != nil {
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, "corpo grande demais ou ilegível", nil)
		return nil, nil, false
	}
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(corpo, &campos); err != nil || campos == nil {
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, "corpo inválido: envie um objeto JSON", nil)
		return nil, nil, false
	}
	return corpo, campos, true
}

// ---- resumo -----------------------------------------------------------------

type resumoCarga struct {
	Estado     string  `json:"estado"`
	Percentual float64 `json:"percentual"`
	Etapa      string  `json:"etapa"`
	Erro       string  `json:"erro"`
}

type resumoBase struct {
	Pronta         bool        `json:"pronta"`
	Amostra        bool        `json:"amostra"`
	ReferenciaCNPJ string      `json:"referencia_cnpj"`
	ReferenciaPGFN string      `json:"referencia_pgfn"`
	Versao         string      `json:"versao"`
	Empresas       int64       `json:"empresas"`
	Carga          resumoCarga `json:"carga"`
}

type resumoPlataforma struct {
	Habilitada bool           `json:"habilitada"`
	Motivo     string         `json:"motivo"`
	Cota       operacoes.Cota `json:"cota"`
	Base       resumoBase     `json:"base"`
}

func (s *Servidor) apiProspeccaoResumo(w http.ResponseWriter, r *http.Request) {
	cota, err := s.ops.CotaProspeccao(r.Context(), clienteDe(r))
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	out := resumoPlataforma{Cota: cota, Base: resumoBase{Carga: resumoCarga{Estado: "ociosa"}}}
	switch {
	case !s.publica.Configurado():
		out.Motivo = msgBaseAusente
	case cota.Limite == 0:
		out.Motivo = msgPlanoSemProspec
	default:
		sit, err := s.publica.Situacao(r.Context())
		if err != nil {
			slog.Warn("prospecção: situação da base indisponível", "erro", err)
			out.Motivo = msgBaseForaDoAr
			break
		}
		out.Habilitada = true
		out.Base = resumoBase{Pronta: sit.Pronta, Amostra: sit.Amostra, ReferenciaCNPJ: sit.ReferenciaCNPJ, ReferenciaPGFN: sit.ReferenciaPGFN,
			Versao: sit.Versao, Empresas: sit.Empresas,
			Carga: resumoCarga{Estado: sit.Carga.Estado, Percentual: sit.Carga.Percentual, Etapa: sit.Carga.Etapa, Erro: sit.Carga.Erro}}
	}
	s.json(w, http.StatusOK, out)
}

// ---- repasses gratuitos -----------------------------------------------------

var catalogosValidos = map[string]bool{"cnaes": true, "municipios": true, "naturezas": true, "ufs": true}

func (s *Servidor) apiProspeccaoCatalogo(w http.ResponseWriter, r *http.Request) {
	nome := chi.URLParam(r, "nome")
	if !catalogosValidos[nome] {
		s.erroAPI(w, http.StatusNotFound, codNaoEncontrado, "catálogo desconhecido", nil)
		return
	}
	if !s.dentroDoLimite(w, r, "catalogos", limiteCatalogos) {
		return
	}
	s.repassar(w, r, http.MethodGet, "/v1/catalogos/"+nome, r.URL.Query(), nil)
}

func (s *Servidor) apiProspeccaoContagem(w http.ResponseWriter, r *http.Request) {
	if !s.dentroDoLimite(w, r, "contagem", limiteContagem) {
		return
	}
	corpo, _, ok := s.lerCorpo(w, r)
	if !ok {
		return
	}
	s.repassar(w, r, http.MethodPost, "/v1/empresas/contagem", nil, corpo)
}

func (s *Servidor) apiProspeccaoPrevia(w http.ResponseWriter, r *http.Request) {
	if !s.dentroDoLimite(w, r, "previa", limitePrevia) {
		return
	}
	corpo, _, ok := s.lerCorpo(w, r)
	if !ok {
		return
	}
	s.repassar(w, r, http.MethodPost, "/v1/empresas/previa", nil, corpo)
}

// apiProspeccaoCandidatos devolve só CNPJs, mas em volume (até 5.000 por
// chamada): exige o pedido_id da lista que está sendo gerada, registra em log
// e tem limite apertado, para a rota não virar um jeito de varrer a base.
func (s *Servidor) apiProspeccaoCandidatos(w http.ResponseWriter, r *http.Request) {
	if !s.dentroDoLimite(w, r, "candidatos", limiteCandidatos) {
		return
	}
	_, campos, ok := s.lerCorpo(w, r)
	if !ok {
		return
	}
	var pedidoID string
	_ = json.Unmarshal(campos["pedido_id"], &pedidoID)
	if pedidoID == "" || len(pedidoID) > 64 {
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, "informe pedido_id (o identificador da lista)", nil)
		return
	}
	// pedido_id é assunto do painel; o serviço recebe só o que é dele.
	delete(campos, "pedido_id")
	corpo, err := json.Marshal(campos)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	slog.Info("prospecção: candidatos", "slug", clienteDe(r).Slug, "pedido_id", pedidoID)
	s.repassar(w, r, http.MethodPost, "/v1/empresas/candidatos", nil, corpo)
}

// ---- extração (desconta da cota) ---------------------------------------------

func (s *Servidor) apiProspeccaoExtracoes(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PedidoID  string   `json:"pedido_id"`
		CNPJs     []string `json:"cnpjs"`
		IncluirPF bool     `json:"incluir_pf"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, corpoMaximoProspeccao)).Decode(&in); err != nil {
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, "corpo inválido", nil)
		return
	}
	c := clienteDe(r)
	out, err := s.ops.ExtrairEmpresas(r.Context(), c, in.PedidoID, in.CNPJs, in.IncluirPF)
	var semCota *operacoes.ErroCota
	switch {
	case err == nil:
		slog.Info("prospecção: extração", "slug", c.Slug, "pedido_id", in.PedidoID, "pedidos", len(in.CNPJs), "entregues", len(out.Itens),
			"incluir_pf", in.IncluirPF, "usado", out.Cota.Usado, "limite", out.Cota.Limite)
		s.json(w, http.StatusOK, out)
	case errors.As(err, &semCota):
		s.erroAPI(w, http.StatusTooManyRequests, codCota, msgCotaEsgotada, map[string]any{"cota": semCota.Cota})
	case errors.Is(err, operacoes.ErrPedidoInvalido):
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, err.Error(), nil)
	case errors.Is(err, basepublica.ErrNaoConfigurado), errors.Is(err, basepublica.ErrIndisponivel), errors.As(err, new(*basepublica.Erro)):
		s.falhaDaBase(w, r, err)
	default:
		s.erroInterno(w, r, err)
	}
}

// ---- ficha avulsa -------------------------------------------------------------

// apiProspeccaoCNPJ devolve a ficha de um CNPJ sem tocar na cota. Por isso tem
// limite por minuto e por dia, e pessoa física (MEI, empresário individual)
// sai sem telefones, e-mail e sócios: esses contatos só saem por /extracoes de
// lista criada com incluir_pf.
func (s *Servidor) apiProspeccaoCNPJ(w http.ResponseWriter, r *http.Request) {
	cnpj, ok := operacoes.NormalizarCNPJ(chi.URLParam(r, "cnpj"))
	if !ok {
		s.erroAPI(w, http.StatusUnprocessableEntity, codFiltroInvalido, "CNPJ inválido", nil)
		return
	}
	if !s.publica.Configurado() {
		s.falhaDaBase(w, r, basepublica.ErrNaoConfigurado)
		return
	}
	if !s.dentroDoLimite(w, r, "cnpj", limiteCNPJ) {
		return
	}
	agora := time.Now()
	dia := operacoes.DiaDaCota(agora)
	dentro, err := s.repo.ContarConsultaCNPJ(r.Context(), clienteDe(r).ID, operacoes.Competencia(agora), dia, limiteCNPJDia)
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	if !dentro {
		s.limiteEstourado(w, msgLimiteDiarioCNPJ, dia.AddDate(0, 0, 1).Sub(agora))
		return
	}
	status, resposta, err := s.publica.Repassar(r.Context(), http.MethodGet, "/v1/empresas/"+cnpj, nil, nil)
	if err != nil {
		s.falhaDaBase(w, r, err)
		return
	}
	if status >= 300 {
		s.statusDaBase(w, r, status, basepublica.MensagemDeErro(status, resposta))
		return
	}
	var ficha map[string]json.RawMessage
	if err := json.Unmarshal(resposta, &ficha); err != nil || ficha == nil {
		s.falhaDaBase(w, r, basepublica.ErrIndisponivel)
		return
	}
	// O serviço já devolve pessoa física sem contatos quando não se pede
	// incluir_pf; o painel repete o corte para não depender disso.
	var pf bool
	_ = json.Unmarshal(ficha["pessoa_fisica"], &pf)
	if pf {
		ficha["telefones"] = json.RawMessage(`[]`)
		ficha["email"] = json.RawMessage(`""`)
		ficha["email_tipo"] = json.RawMessage(`""`)
		ficha["socios"] = json.RawMessage(`[]`)
	}
	s.json(w, http.StatusOK, ficha)
}
