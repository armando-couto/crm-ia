package enriquecimento

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Perfil é o retrato normalizado de uma empresa, igual para todas as fontes.
// As chaves do JSON são contrato com a tela (docs/FUNIL-E-INTELIGENCIA.md).
type Perfil struct {
	Provedor     string    `json:"provedor"`
	Demo         bool      `json:"demo"`
	ConsultadoEm time.Time `json:"consultado_em"`
	// Campos usa as mesmas chaves das colunas de companies (legal_name, city…).
	Campos           map[string]any   `json:"campos"`
	Socios           []Socio          `json:"socios"`
	CNAEsSecundarios []CNAE           `json:"cnaes_secundarios"`
	Indicadores      *Indicadores     `json:"indicadores,omitempty"`
	Contatos         *Contatos        `json:"contatos,omitempty"`
	PresencaDigital  *PresencaDigital `json:"presenca_digital,omitempty"`
	Risco            *Risco           `json:"risco,omitempty"`
	Processos        *Processos       `json:"processos,omitempty"`
	Conformidade     *Conformidade    `json:"conformidade,omitempty"`
	Midia            *Midia           `json:"midia,omitempty"`
	// Localizacao (lat/long do CEP) é extensão opcional do contrato.
	Localizacao   *Localizacao `json:"localizacao,omitempty"`
	Sinais        []Sinal      `json:"sinais"`
	CustoCentavos int          `json:"custo_centavos"`
	// Mensagem acompanha perfis de demonstração ("dados de demonstração").
	Mensagem string `json:"mensagem,omitempty"`
	// Origem (seção → provedor) e CamposOrigem (campo → provedor) são
	// preenchidos no perfil mesclado para a tela mostrar a fonte de cada parte.
	Origem       map[string]string `json:"origem,omitempty"`
	CamposOrigem map[string]string `json:"campos_origem,omitempty"`
}

type Socio struct {
	Nome         string `json:"nome"`
	Documento    string `json:"documento"`
	Qualificacao string `json:"qualificacao"`
	Desde        string `json:"desde"`
	Tipo         string `json:"tipo"` // PF|PJ|Estrangeiro
}

type CNAE struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

type Indicadores struct {
	NivelAtividade      float64 `json:"nivel_atividade"`
	TemDominioAtivo     bool    `json:"tem_dominio_ativo"`
	TemSSL              bool    `json:"tem_ssl"`
	TemEmailCorporativo bool    `json:"tem_email_corporativo"`
	FuncionariosFaixa   string  `json:"funcionarios_faixa"`
	FaturamentoFaixa    string  `json:"faturamento_faixa"`
}

type Telefone struct {
	Numero    string `json:"numero"`
	Tipo      string `json:"tipo"`
	Principal bool   `json:"principal"`
	Validado  bool   `json:"validado"`
}

type EmailContato struct {
	Email    string `json:"email"`
	Tipo     string `json:"tipo"`
	Validado bool   `json:"validado"`
}

type Contatos struct {
	Telefones []Telefone     `json:"telefones"`
	Emails    []EmailContato `json:"emails"`
}

type Dominio struct {
	Dominio string `json:"dominio"`
	Ativo   bool   `json:"ativo"`
	SSL     bool   `json:"ssl"`
}

type PresencaDigital struct {
	Dominios []Dominio         `json:"dominios"`
	Redes    map[string]string `json:"redes"`
}

type Risco struct {
	Score           int     `json:"score"`
	Faixa           string  `json:"faixa"`
	Nivel           string  `json:"nivel"` // baixo|medio|alto
	PendenciasQtd   int     `json:"pendencias_qtd"`
	PendenciasValor float64 `json:"pendencias_valor"`
	ProtestosQtd    int     `json:"protestos_qtd"`
	ChequesSemFundo int     `json:"cheques_sem_fundo"`
	AcoesQtd        int     `json:"acoes_qtd"`
	Consultas90d    int     `json:"consultas_90d"`
	LimiteSugerido  float64 `json:"limite_sugerido"`
	Observacao      string  `json:"observacao"`
}

type Processo struct {
	Numero   string  `json:"numero"`
	Tipo     string  `json:"tipo"`
	Tribunal string  `json:"tribunal"`
	Status   string  `json:"status"`
	Polo     string  `json:"polo"`
	Valor    float64 `json:"valor"`
	Data     string  `json:"data"`
}

type Processos struct {
	Total      int        `json:"total"`
	Ativos     int        `json:"ativos"`
	ComoReu    int        `json:"como_reu"`
	ValorTotal float64    `json:"valor_total"`
	Ultimos    []Processo `json:"ultimos"`
}

type Conformidade struct {
	PEP        bool     `json:"pep"`
	Sancionado bool     `json:"sancionado"`
	Listas     []string `json:"listas"`
}

type Noticia struct {
	Titulo     string `json:"titulo"`
	URL        string `json:"url"`
	Sentimento string `json:"sentimento"`
	Data       string `json:"data"`
}

type Midia struct {
	Exposicao string    `json:"exposicao"`
	Noticias  []Noticia `json:"noticias"`
}

type Localizacao struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Sinal é um resumo colorido no topo da aba Inteligência.
type Sinal struct {
	Tipo  string `json:"tipo"`
	Nivel string `json:"nivel"` // ok|atencao|risco
	Texto string `json:"texto"`
}

// NovoPerfil cria um perfil vazio da fonte.
func NovoPerfil(provedor string) *Perfil {
	return &Perfil{
		Provedor:         provedor,
		ConsultadoEm:     time.Now().UTC().Truncate(time.Second),
		Campos:           map[string]any{},
		Socios:           []Socio{},
		CNAEsSecundarios: []CNAE{},
		Sinais:           []Sinal{},
	}
}

// Set grava um campo quando o valor não é vazio.
func (p *Perfil) Set(campo string, valor any) {
	if p.Campos == nil {
		p.Campos = map[string]any{}
	}
	if Vazio(valor) {
		return
	}
	if s, ok := valor.(string); ok {
		valor = strings.TrimSpace(s)
	}
	p.Campos[campo] = valor
}

// Texto devolve o campo como string ("" quando ausente).
func (p *Perfil) Texto(campo string) string {
	if p == nil || p.Campos == nil {
		return ""
	}
	v, ok := p.Campos[campo]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "sim"
		}
		return "não"
	case float64:
		return fmt.Sprintf("%g", t)
	case int:
		return fmt.Sprint(t)
	}
	return fmt.Sprint(v)
}

// Vazio diz se o valor não traz informação.
func Vazio(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case *string:
		return t == nil || strings.TrimSpace(*t) == ""
	case float64:
		return t == 0
	case *float64:
		return t == nil || *t == 0
	case int:
		return t == 0
	case *bool:
		return t == nil
	case bool:
		return false
	case []string:
		return len(t) == 0
	}
	return false
}

// Clonar devolve uma cópia profunda (via JSON).
func (p *Perfil) Clonar() *Perfil {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	var c Perfil
	_ = json.Unmarshal(b, &c)
	if c.Campos == nil {
		c.Campos = map[string]any{}
	}
	return &c
}

// PerfilDeJSON lê um perfil gravado em company_enrichments.perfil.
func PerfilDeJSON(b []byte) *Perfil {
	if len(b) == 0 {
		return nil
	}
	var p Perfil
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	if p.Campos == nil {
		p.Campos = map[string]any{}
	}
	if p.Socios == nil {
		p.Socios = []Socio{}
	}
	if p.CNAEsSecundarios == nil {
		p.CNAEsSecundarios = []CNAE{}
	}
	if p.Sinais == nil {
		p.Sinais = []Sinal{}
	}
	return &p
}

// -----------------------------------------------------------------------------
// Campos: ordem e rótulos em português
// -----------------------------------------------------------------------------

type campoInfo struct {
	Chave, Label string
}

// CamposConhecidos é a ordem em que o diff é apresentado.
var CamposConhecidos = []campoInfo{
	{"legal_name", "Razão social"},
	{"trade_name", "Nome fantasia"},
	{"name", "Nome"},
	{"cnae_code", "CNAE principal"},
	{"cnae_description", "Descrição do CNAE"},
	{"industry", "Segmento"},
	{"legal_nature", "Natureza jurídica"},
	{"founded_at", "Data de abertura"},
	{"capital", "Capital social"},
	{"company_size", "Porte"},
	{"registration_status", "Situação cadastral"},
	{"registration_status_at", "Data da situação"},
	{"simples", "Optante pelo Simples"},
	{"mei", "MEI"},
	{"street", "Logradouro"},
	{"street_number", "Número"},
	{"complement", "Complemento"},
	{"neighborhood", "Bairro"},
	{"zip", "CEP"},
	{"city", "Cidade"},
	{"state", "UF"},
	{"phone", "Telefone"},
	{"email", "E-mail"},
	{"website", "Site"},
	{"domain", "Domínio"},
	{"employees_range", "Faixa de funcionários"},
	{"revenue_range", "Faixa de faturamento"},
}

// LabelCampo devolve o rótulo humano do campo.
func LabelCampo(campo string) string {
	for _, c := range CamposConhecidos {
		if c.Chave == campo {
			return c.Label
		}
	}
	return campo
}

// CampoConhecido diz se a chave faz parte do contrato.
func CampoConhecido(campo string) bool {
	for _, c := range CamposConhecidos {
		if c.Chave == campo {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Mesclagem: receita como base; pagos sobrepõem as seções analíticas e
// preenchem campos vazios.
// -----------------------------------------------------------------------------

// Mesclar consolida vários perfis num só. nil quando não há perfis.
func Mesclar(perfis ...*Perfil) *Perfil {
	validos := []*Perfil{}
	for _, p := range perfis {
		if p != nil {
			validos = append(validos, p)
		}
	}
	if len(validos) == 0 {
		return nil
	}
	// Receita primeiro; depois a ordem recebida.
	sort.SliceStable(validos, func(i, j int) bool {
		return validos[i].Provedor == models.ProvedorReceita && validos[j].Provedor != models.ProvedorReceita
	})
	base := validos[0].Clonar()
	base.Origem = map[string]string{}
	base.CamposOrigem = map[string]string{}
	for k, v := range base.Campos {
		if !Vazio(v) {
			base.CamposOrigem[k] = base.Provedor
		}
	}
	base.Origem["campos"] = base.Provedor
	for _, sec := range secoesPresentes(base) {
		base.Origem[sec] = base.Provedor
	}
	provedores := []string{base.Provedor}
	demo := base.Demo
	custo := base.CustoCentavos
	ultimo := base.ConsultadoEm

	for _, outro := range validos[1:] {
		o := outro.Clonar()
		provedores = append(provedores, o.Provedor)
		demo = demo || o.Demo
		custo += o.CustoCentavos
		if o.ConsultadoEm.After(ultimo) {
			ultimo = o.ConsultadoEm
		}
		for k, v := range o.Campos {
			if Vazio(v) {
				continue
			}
			if Vazio(base.Campos[k]) {
				base.Campos[k] = v
				base.CamposOrigem[k] = o.Provedor
			}
		}
		if len(base.Socios) == 0 && len(o.Socios) > 0 {
			base.Socios = o.Socios
			base.Origem["socios"] = o.Provedor
		}
		if len(base.CNAEsSecundarios) == 0 && len(o.CNAEsSecundarios) > 0 {
			base.CNAEsSecundarios = o.CNAEsSecundarios
		}
		if base.Localizacao == nil && o.Localizacao != nil {
			base.Localizacao = o.Localizacao
		}
		if o.Indicadores != nil {
			base.Indicadores = o.Indicadores
			base.Origem["indicadores"] = o.Provedor
		}
		if o.Contatos != nil {
			base.Contatos = o.Contatos
			base.Origem["contatos"] = o.Provedor
		}
		if o.PresencaDigital != nil {
			base.PresencaDigital = o.PresencaDigital
			base.Origem["presenca_digital"] = o.Provedor
		}
		if o.Risco != nil {
			base.Risco = o.Risco
			base.Origem["risco"] = o.Provedor
		}
		if o.Processos != nil {
			base.Processos = o.Processos
			base.Origem["processos"] = o.Provedor
		}
		if o.Conformidade != nil {
			base.Conformidade = o.Conformidade
			base.Origem["conformidade"] = o.Provedor
		}
		if o.Midia != nil {
			base.Midia = o.Midia
			base.Origem["midia"] = o.Provedor
		}
	}
	base.Provedor = strings.Join(provedores, ",")
	base.Demo = demo
	base.CustoCentavos = custo
	base.ConsultadoEm = ultimo
	base.Mensagem = ""
	if demo {
		base.Mensagem = "dados de demonstração"
	}
	base.Sinais = CalcularSinais(base)
	return base
}

func secoesPresentes(p *Perfil) []string {
	s := []string{}
	if len(p.Socios) > 0 {
		s = append(s, "socios")
	}
	if p.Indicadores != nil {
		s = append(s, "indicadores")
	}
	if p.Contatos != nil {
		s = append(s, "contatos")
	}
	if p.PresencaDigital != nil {
		s = append(s, "presenca_digital")
	}
	if p.Risco != nil {
		s = append(s, "risco")
	}
	if p.Processos != nil {
		s = append(s, "processos")
	}
	if p.Conformidade != nil {
		s = append(s, "conformidade")
	}
	if p.Midia != nil {
		s = append(s, "midia")
	}
	return s
}

// ProvedorDoCampo diz de onde veio o campo no perfil (mesclado ou simples).
func (p *Perfil) ProvedorDoCampo(campo string) string {
	if p.CamposOrigem != nil {
		if prov, ok := p.CamposOrigem[campo]; ok {
			return prov
		}
	}
	if i := strings.IndexByte(p.Provedor, ','); i > 0 {
		return p.Provedor[:i]
	}
	return p.Provedor
}

// -----------------------------------------------------------------------------
// Diff entre a ficha e o perfil
// -----------------------------------------------------------------------------

// CampoDiff é uma linha da tabela "atual × proposto".
type CampoDiff struct {
	Campo    string `json:"campo"`
	Label    string `json:"label"`
	Atual    any    `json:"atual"`
	Proposto any    `json:"proposto"`
	Provedor string `json:"provedor"`
	Sugerido bool   `json:"sugerido"`
}

// ValorAtual lê o campo da empresa no mesmo formato do perfil.
func ValorAtual(c *models.Company, campo string) any {
	if c == nil {
		return nil
	}
	switch campo {
	case "name":
		return c.Name
	case "domain":
		return c.Domain
	case "phone":
		return c.Phone
	case "industry":
		return c.Industry
	case "city":
		return c.City
	case "state":
		return c.State
	case "legal_name":
		return c.LegalName
	case "trade_name":
		return c.TradeName
	case "cnae_code":
		return c.CNAECode
	case "cnae_description":
		return c.CNAEDescription
	case "legal_nature":
		return c.LegalNature
	case "founded_at":
		if c.FoundedAt == nil {
			return nil
		}
		return c.FoundedAt.String()
	case "capital":
		if c.Capital == nil {
			return nil
		}
		return *c.Capital
	case "company_size":
		return c.CompanySize
	case "registration_status":
		return c.RegistrationStatus
	case "registration_status_at":
		if c.RegistrationStatusAt == nil {
			return nil
		}
		return c.RegistrationStatusAt.String()
	case "simples":
		if c.Simples == nil {
			return nil
		}
		return *c.Simples
	case "mei":
		if c.MEI == nil {
			return nil
		}
		return *c.MEI
	case "street":
		return c.Street
	case "street_number":
		return c.StreetNumber
	case "complement":
		return c.Complement
	case "neighborhood":
		return c.Neighborhood
	case "zip":
		return c.Zip
	case "email":
		return c.Email
	case "website":
		return c.Website
	case "employees_range":
		return c.EmployeesRange
	case "revenue_range":
		return c.RevenueRange
	}
	return nil
}

// Iguais compara dois valores ignorando caixa, espaços e tipo numérico.
func Iguais(a, b any) bool {
	if Vazio(a) && Vazio(b) {
		return true
	}
	if Vazio(a) != Vazio(b) {
		return false
	}
	fa, aok := numero(a)
	fb, bok := numero(b)
	if aok && bok {
		return math.Abs(fa-fb) < 0.005
	}
	na := strings.ToLower(strings.TrimSpace(fmt.Sprint(a)))
	nb := strings.ToLower(strings.TrimSpace(fmt.Sprint(b)))
	return na == nb
}

func numero(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case *float64:
		if t == nil {
			return 0, false
		}
		return *t, true
	}
	return 0, false
}

// Diff lista os campos em que o perfil traz algo diferente da ficha.
// sugerido = atual vazio (ou sobrescrever = "sempre") e proposto diferente.
func Diff(company *models.Company, perfil *Perfil, sobrescrever string) []CampoDiff {
	out := []CampoDiff{}
	if perfil == nil {
		return out
	}
	sempre := sobrescrever == "sempre"
	for _, info := range CamposConhecidos {
		proposto, ok := perfil.Campos[info.Chave]
		if !ok || Vazio(proposto) {
			continue
		}
		atual := ValorAtual(company, info.Chave)
		if Iguais(atual, proposto) {
			continue
		}
		out = append(out, CampoDiff{
			Campo:    info.Chave,
			Label:    info.Label,
			Atual:    atual,
			Proposto: proposto,
			Provedor: perfil.ProvedorDoCampo(info.Chave),
			Sugerido: Vazio(atual) || sempre,
		})
	}
	return out
}

// -----------------------------------------------------------------------------
// Sinais
// -----------------------------------------------------------------------------

// CalcularSinais resume o perfil em chips ok|atencao|risco.
func CalcularSinais(p *Perfil) []Sinal {
	sinais := []Sinal{}
	if p == nil {
		return sinais
	}
	add := func(tipo, nivel, texto string) {
		sinais = append(sinais, Sinal{Tipo: tipo, Nivel: nivel, Texto: texto})
	}
	hoje := time.Now()

	// Situação cadastral.
	if sit := strings.ToUpper(p.Texto("registration_status")); sit != "" {
		ano := ""
		if d := p.Texto("registration_status_at"); len(d) >= 4 {
			ano = d[:4]
		}
		switch {
		case strings.HasPrefix(sit, "ATIVA"):
			if ano != "" {
				add("situacao", "ok", "Situação cadastral ativa desde "+ano)
			} else {
				add("situacao", "ok", "Situação cadastral ativa")
			}
		case strings.Contains(sit, "BAIXADA"), strings.Contains(sit, "NULA"), strings.Contains(sit, "INAPTA"):
			add("situacao", "risco", "Situação cadastral: "+strings.ToLower(sit))
		default:
			add("situacao", "atencao", "Situação cadastral: "+strings.ToLower(sit))
		}
	}

	// Idade da empresa.
	if d := p.Texto("founded_at"); len(d) >= 10 {
		if t, err := time.Parse("2006-01-02", d[:10]); err == nil {
			anos := int(hoje.Sub(t).Hours() / 24 / 365.25)
			switch {
			case anos < 1:
				add("idade", "atencao", "Empresa aberta há menos de 1 ano")
			case anos < 3:
				add("idade", "atencao", fmt.Sprintf("Empresa com %d anos de mercado", anos))
			default:
				add("idade", "ok", fmt.Sprintf("Empresa com %d anos de mercado", anos))
			}
		}
	}

	// Porte.
	porte := p.Texto("company_size")
	if mei, ok := p.Campos["mei"].(bool); ok && mei {
		add("porte", "atencao", "Microempreendedor individual (MEI)")
	} else if porte != "" {
		texto := "Porte: " + descreverPorte(porte)
		if f := p.Texto("employees_range"); f != "" {
			texto += " · " + f + " funcionários"
		}
		add("porte", "ok", texto)
	} else if f := p.Texto("employees_range"); f != "" {
		add("porte", "ok", f+" funcionários")
	}

	// Atividade digital.
	if ind := p.Indicadores; ind != nil {
		switch {
		case ind.TemDominioAtivo && ind.TemSSL && ind.TemEmailCorporativo:
			add("digital", "ok", "Presença digital consistente: domínio ativo, SSL e e-mail corporativo")
		case ind.TemDominioAtivo:
			add("digital", "ok", "Domínio ativo")
		case ind.NivelAtividade > 0 && ind.NivelAtividade < 0.3:
			add("digital", "atencao", "Baixo nível de atividade")
		default:
			add("digital", "atencao", "Sem presença digital identificada")
		}
		if ind.NivelAtividade >= 0.7 {
			add("atividade", "ok", fmt.Sprintf("Nível de atividade alto (%d%%)", int(ind.NivelAtividade*100)))
		} else if ind.NivelAtividade > 0 && ind.NivelAtividade < 0.3 {
			add("atividade", "atencao", fmt.Sprintf("Nível de atividade baixo (%d%%)", int(ind.NivelAtividade*100)))
		}
	}

	// Risco de crédito.
	if r := p.Risco; r != nil {
		if r.Score > 0 {
			nivel := r.Nivel
			if nivel == "" {
				nivel = NivelDoScore(r.Score)
			}
			chip := "ok"
			if nivel == "medio" {
				chip = "atencao"
			} else if nivel == "alto" {
				chip = "risco"
			}
			add("risco", chip, fmt.Sprintf("Score %d (risco %s)", r.Score, nivel))
		}
		neg := r.PendenciasQtd + r.ProtestosQtd + r.ChequesSemFundo
		if neg > 0 {
			partes := []string{}
			if r.PendenciasQtd > 0 {
				partes = append(partes, plural(r.PendenciasQtd, "pendência", "pendências"))
			}
			if r.ProtestosQtd > 0 {
				partes = append(partes, plural(r.ProtestosQtd, "protesto", "protestos"))
			}
			if r.ChequesSemFundo > 0 {
				partes = append(partes, plural(r.ChequesSemFundo, "cheque sem fundo", "cheques sem fundo"))
			}
			add("negativacao", "risco", "Anotações negativas: "+strings.Join(partes, ", "))
		} else if r.Score > 0 {
			add("negativacao", "ok", "Sem anotações negativas")
		}
	}

	// Processos.
	if pr := p.Processos; pr != nil && pr.Total > 0 {
		nivel := "atencao"
		if pr.Total >= 5 || pr.ComoReu >= 3 {
			nivel = "risco"
		}
		texto := plural(pr.Total, "processo judicial", "processos judiciais")
		if pr.ComoReu > 0 {
			texto += fmt.Sprintf(" (%d como réu)", pr.ComoReu)
		}
		add("processos", nivel, texto)
	} else if pr != nil {
		add("processos", "ok", "Nenhum processo judicial encontrado")
	}

	// Conformidade.
	if c := p.Conformidade; c != nil {
		if c.Sancionado {
			add("conformidade", "risco", "Presente em lista de sanções")
		}
		if c.PEP {
			add("conformidade", "atencao", "Pessoa politicamente exposta no quadro")
		}
		if !c.Sancionado && !c.PEP {
			add("conformidade", "ok", "Sem sanções ou PEP")
		}
	}

	// Mídia.
	if m := p.Midia; m != nil && m.Exposicao != "" {
		switch strings.ToLower(m.Exposicao) {
		case "alta", "muito alta":
			add("midia", "atencao", "Alta exposição na mídia")
		}
	}
	return sinais
}

// NivelDoScore converte o score (0–1000) em baixo|medio|alto.
func NivelDoScore(score int) string {
	switch {
	case score >= 700:
		return "baixo"
	case score >= 500:
		return "medio"
	}
	return "alto"
}

// FaixaDoScore converte o score em A–E.
func FaixaDoScore(score int) string {
	switch {
	case score >= 800:
		return "A"
	case score >= 700:
		return "B"
	case score >= 500:
		return "C"
	case score >= 300:
		return "D"
	}
	return "E"
}

func plural(n int, um, varios string) string {
	if n == 1 {
		return "1 " + um
	}
	return fmt.Sprintf("%d %s", n, varios)
}

func descreverPorte(p string) string {
	switch strings.ToUpper(p) {
	case "ME":
		return "Microempresa (ME)"
	case "EPP":
		return "Empresa de Pequeno Porte (EPP)"
	case "DEMAIS":
		return "Demais (médio/grande)"
	}
	return p
}

// NormalizarPorte reduz as grafias das fontes a ME|EPP|DEMAIS.
func NormalizarPorte(p string) string {
	u := strings.ToUpper(strings.TrimSpace(p))
	switch {
	case u == "":
		return ""
	case strings.Contains(u, "(ME)"), strings.HasPrefix(u, "MICRO"), u == "ME", u == "01":
		return "ME"
	case strings.Contains(u, "EPP"), strings.Contains(u, "PEQUENO PORTE"), u == "03":
		return "EPP"
	case strings.Contains(u, "DEMAIS"), u == "05":
		return "DEMAIS"
	}
	return strings.TrimSpace(p)
}

// NormalizarData aceita AAAA-MM-DD, DD/MM/AAAA e ISO com hora; devolve AAAA-MM-DD.
func NormalizarData(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
	}
	if t, err := time.Parse("02/01/2006", s); err == nil {
		return t.Format("2006-01-02")
	}
	if len(s) == 8 && SomenteDigitos(s) == s {
		if t, err := time.Parse("20060102", s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}
