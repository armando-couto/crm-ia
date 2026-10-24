package enriquecimento

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// Opcoes ajusta uma consulta.
type Opcoes struct {
	// Datasets (BigDataCorp) sobrepõe o conjunto padrão.
	Datasets []string
	// Forcar ignora o cache.
	Forcar bool
	// Timeout limita a consulta inteira (0 = padrão do provedor).
	Timeout time.Duration
}

// Bruto guarda a resposta original da fonte (sem segredos) para auditoria.
type Bruto struct {
	Fonte string          `json:"fonte"`
	Cache bool            `json:"cache,omitempty"`
	Corpo json.RawMessage `json:"corpo,omitempty"`
}

// Provedor é uma fonte de dados de empresa.
type Provedor interface {
	Codigo() string
	Consultar(ctx context.Context, cnpj string, opts Opcoes) (*Perfil, *Bruto, error)
}

// Tipos de falha, que o controller traduz em status HTTP.
const (
	FalhaConfig       = "config"        // provedor não configurado → 422
	FalhaCredencial   = "credencial"    // credenciais recusadas → erro na consulta
	FalhaLimite       = "limite"        // limite da fonte ou mensal → 429
	FalhaSemResultado = "sem_resultado" // CNPJ não encontrado
	FalhaExterna      = "externo"       // fonte indisponível → 502
	FalhaEntrada      = "entrada"       // CNPJ inválido etc. → 422
)

// Falha é um erro com mensagem amigável em português.
type Falha struct {
	Tipo     string
	Mensagem string
}

func (f *Falha) Error() string { return f.Mensagem }

func falha(tipo, formato string, args ...any) *Falha {
	return &Falha{Tipo: tipo, Mensagem: fmt.Sprintf(formato, args...)}
}

// TipoDaFalha devolve o tipo da falha ou "externo" para erros genéricos.
func TipoDaFalha(err error) string {
	if f, ok := err.(*Falha); ok {
		return f.Tipo
	}
	return FalhaExterna
}

// MensagemDaFalha devolve a mensagem amigável (nunca o erro técnico cru).
func MensagemDaFalha(err error) string {
	if err == nil {
		return ""
	}
	if f, ok := err.(*Falha); ok {
		return f.Mensagem
	}
	if err == context.DeadlineExceeded || strings.Contains(err.Error(), "deadline exceeded") {
		return "a fonte demorou demais para responder"
	}
	return "não foi possível consultar a fonte externa agora"
}

// -----------------------------------------------------------------------------
// HTTP
// -----------------------------------------------------------------------------

// Cliente é o http.Client das consultas externas (substituível em testes).
var Cliente = &http.Client{Timeout: 15 * time.Second}

const limiteCorpo = 4 << 20

// requisitar faz a chamada e devolve status + corpo (limitado a 4 MB).
func requisitar(ctx context.Context, metodo, url string, corpo io.Reader, cabecalhos map[string]string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, metodo, url, corpo)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CRM-IA/1.0 (+https://crm-ia.com.br)")
	for k, v := range cabecalhos {
		req.Header.Set(k, v)
	}
	resp, err := Cliente.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limiteCorpo))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, b, nil
}

// -----------------------------------------------------------------------------
// Cache (utils.Cache) de perfis públicos por CNPJ
// -----------------------------------------------------------------------------

type itemCache struct {
	Perfil *Perfil `json:"perfil"`
	Bruto  *Bruto  `json:"bruto"`
}

func chaveCache(provedor, cnpj string) string {
	return "enriq:" + provedor + ":" + SomenteDigitos(cnpj)
}

func lerCache(provedor, cnpj string) (*Perfil, *Bruto, bool) {
	raw, ok := utils.Cache.Get(chaveCache(provedor, cnpj))
	if !ok {
		return nil, nil, false
	}
	var it itemCache
	if err := json.Unmarshal([]byte(raw), &it); err != nil || it.Perfil == nil {
		return nil, nil, false
	}
	if it.Perfil.Campos == nil {
		it.Perfil.Campos = map[string]any{}
	}
	if it.Bruto == nil {
		it.Bruto = &Bruto{Fonte: provedor}
	}
	it.Bruto.Cache = true
	return it.Perfil, it.Bruto, true
}

func gravarCache(provedor, cnpj string, p *Perfil, b *Bruto, ttl time.Duration) {
	if p == nil || ttl <= 0 {
		return
	}
	raw, err := json.Marshal(itemCache{Perfil: p, Bruto: b})
	if err != nil {
		return
	}
	utils.Cache.Set(chaveCache(provedor, cnpj), string(raw), ttl)
}

// LimparCache apaga o perfil público em cache do CNPJ (forçar nova consulta).
func LimparCache(cnpj string) {
	utils.Cache.Del(chaveCache(models.ProvedorReceita, cnpj))
}

// -----------------------------------------------------------------------------
// Registro: monta os provedores a partir das configurações
// -----------------------------------------------------------------------------

// Registro conhece as fontes disponíveis para esta instalação.
type Registro struct {
	Cfg models.EnrichmentSettings
}

// NovoRegistro cria o registro a partir das configurações carregadas.
func NovoRegistro(cfg models.EnrichmentSettings) *Registro {
	return &Registro{Cfg: cfg}
}

// Pago diz se a fonte tem custo.
func Pago(codigo string) bool {
	return codigo == models.ProvedorBigDataCorp || codigo == models.ProvedorSerasa
}

// CodigoValido diz se a fonte existe.
func CodigoValido(codigo string) bool {
	switch codigo {
	case models.ProvedorReceita, models.ProvedorBigDataCorp, models.ProvedorSerasa:
		return true
	}
	return false
}

// Provedor devolve a fonte pronta para uso, ou Falha de configuração.
func (r *Registro) Provedor(codigo string) (Provedor, error) {
	switch codigo {
	case models.ProvedorReceita:
		return &Receita{PrazoCache: time.Duration(r.Cfg.PrazoCacheDias) * 24 * time.Hour}, nil
	case models.ProvedorBigDataCorp:
		b := r.Cfg.BigDataCorp
		if !b.Habilitado || (!b.Demo && (b.TokenID == "" || b.AccessTokenCifrado == "")) {
			return nil, falha(FalhaConfig, "BigDataCorp não configurado. Peça ao administrador para ativar em Configurações → Integrações.")
		}
		if b.Demo {
			return &Demo{Fonte: models.ProvedorBigDataCorp, Precos: b.PrecosCentavos, DatasetsPadrao: b.DatasetsPadrao}, nil
		}
		token, err := utils.Decifrar(b.AccessTokenCifrado)
		if err != nil {
			return nil, falha(FalhaConfig, "não foi possível ler a credencial do BigDataCorp: %s", err.Error())
		}
		return &BigDataCorp{TokenID: b.TokenID, AccessToken: token, BaseURL: b.BaseURL,
			DatasetsPadrao: b.DatasetsPadrao, Precos: b.PrecosCentavos}, nil
	case models.ProvedorSerasa:
		s := r.Cfg.Serasa
		if !s.Habilitado || (!s.Demo && (s.ClientID == "" || s.ClientSecretCifrado == "")) {
			return nil, falha(FalhaConfig, "Serasa Experian não configurado. Peça ao administrador para ativar em Configurações → Integrações.")
		}
		if s.Demo {
			return &Demo{Fonte: models.ProvedorSerasa, PrecoFixo: 0}, nil
		}
		secret, err := utils.Decifrar(s.ClientSecretCifrado)
		if err != nil {
			return nil, falha(FalhaConfig, "não foi possível ler a credencial do Serasa: %s", err.Error())
		}
		return &Serasa{ClientID: s.ClientID, ClientSecret: secret, Ambiente: s.Ambiente, BaseURL: s.BaseURL,
			ReportName: s.ReportName, OptionalFeatures: s.OptionalFeatures, ReportPath: s.ReportPath,
			PrecoCentavos: s.PrecoCentavos}, nil
	}
	return nil, falha(FalhaEntrada, "fonte desconhecida: %s", codigo)
}

// CustoEstimado devolve o custo previsto (centavos) da fonte; 0 em demo/grátis.
func (r *Registro) CustoEstimado(codigo string, datasets []string) int {
	switch codigo {
	case models.ProvedorBigDataCorp:
		if r.Cfg.BigDataCorp.Demo {
			return 0
		}
		if len(datasets) == 0 {
			datasets = r.Cfg.BigDataCorp.DatasetsPadrao
		}
		return r.Cfg.CustoBigDataCorp(datasets)
	case models.ProvedorSerasa:
		if r.Cfg.Serasa.Demo {
			return 0
		}
		return r.Cfg.Serasa.PrecoCentavos
	}
	return 0
}

// Catalogo descreve as fontes para a tela de configurações.
func (r *Registro) Catalogo() []map[string]any {
	datasets := []map[string]any{}
	for _, d := range models.BigDataCorpDatasets {
		datasets = append(datasets, map[string]any{
			"codigo": d.Codigo, "nome": d.Nome, "descricao": d.Descricao,
			"preco_centavos": r.Cfg.BigDataCorp.PrecosCentavos[d.Codigo],
		})
	}
	return []map[string]any{
		{
			"codigo": models.ProvedorReceita, "nome": models.NomeProvedor(models.ProvedorReceita),
			"descricao": "Dados públicos do CNPJ: razão social, situação, CNAE, endereço, sócios, Simples/MEI (BrasilAPI, OpenCNPJ, CNPJ.ws) e CEP normalizado.",
			"custo":     "grátis", "custo_centavos": 0, "habilitado": true, "demo": false, "pronto": true,
		},
		{
			"codigo": models.ProvedorBigDataCorp, "nome": models.NomeProvedor(models.ProvedorBigDataCorp),
			"descricao": "Indicadores de atividade, contatos validados, sócios, presença digital, processos, KYC e mídia — cobrado por conjunto de dados.",
			"custo":     "por conjunto de dados", "custo_centavos": r.CustoEstimado(models.ProvedorBigDataCorp, nil),
			"habilitado": r.Cfg.BigDataCorp.Habilitado, "demo": r.Cfg.BigDataCorp.Demo, "pronto": r.Cfg.BigDataCorpPronto(),
			"datasets": datasets, "datasets_padrao": r.Cfg.BigDataCorp.DatasetsPadrao,
		},
		{
			"codigo": models.ProvedorSerasa, "nome": models.NomeProvedor(models.ProvedorSerasa),
			"descricao": "Score positivo, anotações negativas (pendências, protestos, cheques), consultas recentes, limite de crédito e participações — por contrato.",
			"custo":     "por contrato", "custo_centavos": r.CustoEstimado(models.ProvedorSerasa, nil),
			"habilitado": r.Cfg.Serasa.Habilitado, "demo": r.Cfg.Serasa.Demo, "pronto": r.Cfg.SerasaPronto(),
		},
	}
}
