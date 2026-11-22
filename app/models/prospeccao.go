package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// -----------------------------------------------------------------------------
// Prospecção: tipos do contrato (docs/PROSPECCAO.md e docs/PROSPECCAO-ADENDO.md).
// Os nomes JSON valem exatamente como escritos lá. Os tipos que vêm da base
// pública da plataforma levam o prefixo Prosp para não colidir com o resto do
// pacote (Company, Contact, ContactList…).
// -----------------------------------------------------------------------------

// Executor é o que *sql.DB e *sql.Tx têm em comum. As funções de escrita da
// prospecção o recebem para poderem participar da transação de quem chama.
type Executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Portes da Receita (01, 03, 05, 00).
const (
	PorteME           = "me"
	PorteEPP          = "epp"
	PorteDemais       = "demais"
	PorteNaoInformado = "nao_informado"
)

// Ordens da busca. "base" só aparece em respostas (ordem_aplicada), quando o
// conjunto é grande demais para ordenar.
const (
	OrdemRelevancia  = "relevancia"
	OrdemRecentes    = "recentes"
	OrdemAntigas     = "antigas"
	OrdemCapitalDesc = "capital_desc"
	OrdemDividaDesc  = "divida_desc"
	OrdemAleatoria   = "aleatoria"
	OrdemBase        = "base"
)

// ProspOrdens são as ordens que o usuário pode pedir.
var ProspOrdens = []string{OrdemRelevancia, OrdemRecentes, OrdemAntigas, OrdemCapitalDesc, OrdemDividaDesc, OrdemAleatoria}

// ProspOrdemValida aceita vazio (vale relevância).
func ProspOrdemValida(o string) bool { return o == "" || slices.Contains(ProspOrdens, o) }

// ProspFiltros é o filtro da busca na base pública (interface Filtros).
type ProspFiltros struct {
	UFs                 []string `json:"ufs,omitempty"`
	Municipios          []int    `json:"municipios,omitempty"`
	CNAEs               []int    `json:"cnaes,omitempty"`
	CNAEPrefixos        []string `json:"cnae_prefixos,omitempty"`
	CNAESecundario      bool     `json:"cnae_secundario,omitempty"`
	Portes              []string `json:"portes,omitempty"`
	Naturezas           []int    `json:"naturezas,omitempty"`
	Matriz              string   `json:"matriz,omitempty"`  // todas | matriz | filial
	Simples             string   `json:"simples,omitempty"` // todos | sim | nao
	MEI                 string   `json:"mei,omitempty"`     // todos | sim | nao
	IncluirPF           bool     `json:"incluir_pf,omitempty"`
	AberturaDe          string   `json:"abertura_de,omitempty"`
	AberturaAte         string   `json:"abertura_ate,omitempty"`
	AberturaUltimosDias int      `json:"abertura_ultimos_dias,omitempty"`
	CapitalMin          *float64 `json:"capital_min,omitempty"`
	CapitalMax          *float64 `json:"capital_max,omitempty"`
	ComTelefone         bool     `json:"com_telefone,omitempty"`
	ComCelular          bool     `json:"com_celular,omitempty"`
	ComEmail            bool     `json:"com_email,omitempty"`
	OcultarContador     bool     `json:"ocultar_contador,omitempty"`
	Divida              string   `json:"divida,omitempty"` // todos | com | sem
	DividaMin           *float64 `json:"divida_min,omitempty"`
	DividaMax           *float64 `json:"divida_max,omitempty"`
	Bairros             []string `json:"bairros,omitempty"`
	CEPs                []string `json:"ceps,omitempty"`
	Nome                string   `json:"nome,omitempty"`
	CNPJs               []string `json:"cnpjs,omitempty"`
}

// MaxCNPJsFiltro é o teto de filtros.cnpjs (lista colada ou vinda de arquivo).
const MaxCNPJsFiltro = 5000

// TemObrigatorio diz se há ao menos um entre ufs, municipios, cnaes,
// cnae_prefixos e cnpjs (sem isso a plataforma responde 422).
func (f ProspFiltros) TemObrigatorio() bool {
	return len(f.UFs) > 0 || len(f.Municipios) > 0 || len(f.CNAEs) > 0 || len(f.CNAEPrefixos) > 0 || len(f.CNPJs) > 0
}

// Normalizar apara textos, põe UFs em maiúsculas e deixa os CNPJs na forma
// normalizada, sem repetidos (a ordem de entrada é mantida).
func (f *ProspFiltros) Normalizar() {
	for i, uf := range f.UFs {
		f.UFs[i] = strings.ToUpper(strings.TrimSpace(uf))
	}
	for i, p := range f.CNAEPrefixos {
		f.CNAEPrefixos[i] = strings.TrimSpace(p)
	}
	f.Nome = strings.TrimSpace(f.Nome)
	if len(f.CNPJs) > 0 {
		visto := make(map[string]bool, len(f.CNPJs))
		limpos := make([]string, 0, len(f.CNPJs))
		for _, c := range f.CNPJs {
			c = NormalizarCNPJ(c)
			if c == "" || visto[c] {
				continue
			}
			visto[c] = true
			limpos = append(limpos, c)
		}
		f.CNPJs = limpos
	}
}

// Validar aplica as regras que o app pode conferir sem a plataforma. A
// mensagem é a que vai para a tela (422, codigo "filtro_invalido").
func (f ProspFiltros) Validar() error {
	if !f.TemObrigatorio() {
		return fmt.Errorf("escolha um segmento (CNAE) ou um lugar (UF ou cidade)")
	}
	if len(f.CNPJs) > MaxCNPJsFiltro {
		return fmt.Errorf("a lista de CNPJs aceita até %d por vez", MaxCNPJsFiltro)
	}
	for _, c := range f.CNPJs {
		if !CNPJValido(c) {
			return fmt.Errorf("CNPJ inválido na lista: %s", c)
		}
	}
	if len(f.CNPJs) > 0 {
		return nil // com cnpjs os demais filtros são ignorados (exceto incluir_pf)
	}
	if f.MEI == "sim" && !f.IncluirPF {
		return fmt.Errorf("para filtrar MEI, ligue 'incluir MEI e empresário individual'")
	}
	if f.Nome != "" && utf8.RuneCountInString(f.Nome) < 3 {
		return fmt.Errorf("a busca por nome precisa de pelo menos 3 letras")
	}
	if (f.Nome != "" || len(f.Bairros) > 0 || len(f.CEPs) > 0) &&
		len(f.Municipios) == 0 && len(f.CNAEs) == 0 && len(f.CNAEPrefixos) == 0 {
		return fmt.Errorf("para filtrar por nome, bairro ou CEP, escolha também uma cidade ou um CNAE")
	}
	for _, p := range f.CNAEPrefixos {
		if len(p) < 2 || len(p) > 5 || strings.Trim(p, "0123456789") != "" {
			return fmt.Errorf("prefixo de CNAE inválido: %s", p)
		}
	}
	for _, uf := range f.UFs {
		if len(uf) != 2 {
			return fmt.Errorf("UF inválida: %s", uf)
		}
	}
	return nil
}

// ProspTelefone é um telefone da empresa, de um sócio ou de um decisor.
// numero: só dígitos com DDD (celular = 11; fixo e indefinido = 10).
type ProspTelefone struct {
	Numero        string `json:"numero"`
	Tipo          string `json:"tipo"` // celular | fixo | indefinido
	Compartilhado bool   `json:"compartilhado,omitempty"`
	WhatsApp      *bool  `json:"whatsapp,omitempty"`
	NaoPerturbe   *bool  `json:"nao_perturbe,omitempty"`
	Origem        string `json:"origem,omitempty"` // receita | wholead | manual
	Pessoa        string `json:"pessoa,omitempty"`
	Principal     bool   `json:"principal,omitempty"`
	Invalido      bool   `json:"invalido,omitempty"`
}

// Tipos e origens de telefone.
const (
	TelefoneCelular    = "celular"
	TelefoneFixo       = "fixo"
	TelefoneIndefinido = "indefinido"

	OrigemContatoReceita = "receita"
	OrigemContatoWhoLead = "wholead"
	OrigemContatoManual  = "manual"
)

// ProspSocio é o sócio como vem da Receita (documento já mascarado).
type ProspSocio struct {
	Nome          string  `json:"nome"`
	Qualificacao  string  `json:"qualificacao"`
	Desde         *string `json:"desde"`
	Tipo          string  `json:"tipo"` // pf | pj | estrangeiro
	Documento     string  `json:"documento"`
	Representante string  `json:"representante,omitempty"`
}

// ProspDivida é o resumo da dívida ativa (PGFN) do CNPJ.
type ProspDivida struct {
	Total      float64  `json:"total"`
	Inscricoes int      `json:"inscricoes"`
	EmCobranca float64  `json:"em_cobranca"`
	Regular    float64  `json:"regular"`
	Tipos      []string `json:"tipos"`
	Ajuizada   bool     `json:"ajuizada"`
	MaisAntiga *string  `json:"mais_antiga"`
	Referencia string   `json:"referencia"`
}

// ProspCodigoDescricao serve a natureza jurídica e aos CNAEs.
type ProspCodigoDescricao struct {
	Codigo    int    `json:"codigo"`
	Descricao string `json:"descricao"`
}

// ProspEndereco é o endereço do estabelecimento.
type ProspEndereco struct {
	TipoLogradouro  string `json:"tipo_logradouro"`
	Logradouro      string `json:"logradouro"`
	Numero          string `json:"numero"`
	Complemento     string `json:"complemento"`
	Bairro          string `json:"bairro"`
	CEP             string `json:"cep"`
	Municipio       string `json:"municipio"`
	MunicipioCodigo int    `json:"municipio_codigo"`
	UF              string `json:"uf"`
}

// ProspEmpresa é a ficha inteira de um CNPJ na base pública (interface Empresa).
type ProspEmpresa struct {
	CNPJ             string                 `json:"cnpj"`
	Matriz           bool                   `json:"matriz"`
	RazaoSocial      string                 `json:"razao_social"`
	NomeFantasia     string                 `json:"nome_fantasia"`
	Natureza         ProspCodigoDescricao   `json:"natureza"`
	PessoaFisica     bool                   `json:"pessoa_fisica"`
	Porte            string                 `json:"porte"`
	Capital          *float64               `json:"capital"`
	Abertura         *string                `json:"abertura"`
	Situacao         string                 `json:"situacao"`
	SituacaoDesde    *string                `json:"situacao_desde"`
	Simples          bool                   `json:"simples"`
	MEI              bool                   `json:"mei"`
	CNAE             ProspCodigoDescricao   `json:"cnae"`
	CNAEsSecundarios []ProspCodigoDescricao `json:"cnaes_secundarios"`
	Endereco         ProspEndereco          `json:"endereco"`
	Telefones        []ProspTelefone        `json:"telefones"`
	Email            string                 `json:"email"`
	EmailTipo        string                 `json:"email_tipo"` // corporativo | gratuito | contabilidade | ""
	Socios           []ProspSocio           `json:"socios"`
	Divida           *ProspDivida           `json:"divida"`
	Qualidade        int                    `json:"qualidade"`
	Referencia       string                 `json:"referencia"`
}

// ProspEmpresaPrevia é a linha da prévia gratuita: sem contato e sem sócios.
type ProspEmpresaPrevia struct {
	CNPJ         string               `json:"cnpj"`
	RazaoSocial  string               `json:"razao_social"`
	NomeFantasia string               `json:"nome_fantasia"`
	CNAE         ProspCodigoDescricao `json:"cnae"`
	Porte        string               `json:"porte"`
	Abertura     *string              `json:"abertura"`
	Capital      *float64             `json:"capital"`
	Municipio    string               `json:"municipio"`
	UF           string               `json:"uf"`
	PessoaFisica bool                 `json:"pessoa_fisica"`
	MEI          bool                 `json:"mei"`
	TemCelular   bool                 `json:"tem_celular"`
	TemTelefone  bool                 `json:"tem_telefone"`
	TemEmail     bool                 `json:"tem_email"`
	TemDivida    bool                 `json:"tem_divida"`
	SociosQtd    int                  `json:"socios_qtd"`
	Qualidade    int                  `json:"qualidade"`
}

// ProspContagem é o total da busca. total_modo: "exato" ou "teto" (a contagem
// parou em 10.001: total = 10000 e a tela mostra "mais de 10.000"); as
// parciais vêm nulas em "teto".
type ProspContagem struct {
	Total               int                   `json:"total"`
	TotalModo           string                `json:"total_modo"`
	TotalLimiteSuperior *int                  `json:"total_limite_superior,omitempty"`
	Parciais            ProspContagemParciais `json:"parciais"`
}

type ProspContagemParciais struct {
	ComCelular *int `json:"com_celular"`
	ComEmail   *int `json:"com_email"`
	ComDivida  *int `json:"com_divida"`
}

// ProspCota é a cota mensal de leads do plano.
type ProspCota struct {
	Mes      string `json:"mes"`
	Limite   int    `json:"limite"`
	Usado    int    `json:"usado"`
	Restante int    `json:"restante"`
}

// ProspResumoPlataforma é o que GET /prospeccao devolve.
type ProspResumoPlataforma struct {
	Habilitada bool            `json:"habilitada"`
	Motivo     string          `json:"motivo"`
	Cota       ProspCota       `json:"cota"`
	Base       ProspBaseResumo `json:"base"`
}

type ProspBaseResumo struct {
	Pronta         bool           `json:"pronta"`
	Amostra        bool           `json:"amostra"`
	ReferenciaCNPJ string         `json:"referencia_cnpj"`
	ReferenciaPGFN string         `json:"referencia_pgfn"`
	Versao         string         `json:"versao"`
	Empresas       int            `json:"empresas"`
	Carga          ProspBaseCarga `json:"carga"`
}

type ProspBaseCarga struct {
	Estado     string  `json:"estado"`
	Percentual float64 `json:"percentual"`
	Etapa      string  `json:"etapa"`
	Erro       string  `json:"erro"`
}

// PerfilContato é o resultado do enriquecimento de um item (Who Lead), já com
// o CPF das pessoas mascarado.
type PerfilContato struct {
	Provedor          string          `json:"provedor"`
	Demo              bool            `json:"demo"`
	ConsultadoEm      string          `json:"consultado_em"`
	Telefones         []ProspTelefone `json:"telefones"`
	Emails            []EmailContato  `json:"emails"`
	Pessoas           []PessoaContato `json:"pessoas"`
	FuncionariosFaixa string          `json:"funcionarios_faixa"`
	FaturamentoFaixa  string          `json:"faturamento_faixa"`
	Site              string          `json:"site"`
	Observacao        string          `json:"observacao"`
}

type EmailContato struct {
	Email    string `json:"email"`
	Tipo     string `json:"tipo,omitempty"`
	Validado *bool  `json:"validado,omitempty"`
	Pessoa   string `json:"pessoa,omitempty"`
}

// PessoaContato é um sócio, representante ou funcionário com seus contatos.
type PessoaContato struct {
	Nome      string          `json:"nome"`
	Documento string          `json:"documento"` // mascarado: ***123456**
	Papel     string          `json:"papel"`     // socio | representante | funcionario
	Cargo     string          `json:"cargo"`
	Telefones []ProspTelefone `json:"telefones"`
	Emails    []string        `json:"emails"`
}

// MascararCPF devolve ***123456** para um CPF de 11 dígitos; qualquer outra
// entrada volta só com asteriscos no lugar dos dígitos das pontas.
func MascararCPF(s string) string {
	d := SomenteDigitos(s)
	if len(d) != 11 {
		if d == "" {
			return ""
		}
		return strings.Repeat("*", len(d))
	}
	return "***" + d[3:9] + "**"
}

// NotaMotivo explica uma parcela da nota do item.
type NotaMotivo struct {
	Pontos int    `json:"pontos"`
	Texto  string `json:"texto"`
}

// NovoPedidoID gera o UUID (v4) que identifica a lista nas extrações da
// plataforma e a chave de idempotência dos resultados.
func NovoPedidoID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand não falha em sistemas suportados
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// cortarTexto limita s a n caracteres (não bytes), para caber na coluna.
func cortarTexto(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n]))
}

// jsonOu serializa v; em erro (ou nil) devolve o padrão informado.
func jsonOu(v any, padrao string) string {
	if v == nil {
		return padrao
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return padrao
	}
	return string(b)
}

// likeContem monta o padrão de ILIKE "contém", escapando % e _.
func likeContem(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(s))
	return "%" + s + "%"
}
