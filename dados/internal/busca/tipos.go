// Package busca responde contagem, prévia, candidatos, ficha e catálogos
// sobre o esquema de serviço construído pela carga.
package busca

import (
	"errors"
	"time"
)

// Ordens aceitas. "base" só aparece em ordem_aplicada: é a ordem física da
// base (id dentro das faixas de UF e município), usada quando o conjunto é
// grande demais para ordenar.
const (
	OrdemRelevancia = "relevancia"
	OrdemRecentes   = "recentes"
	OrdemAntigas    = "antigas"
	OrdemCapital    = "capital_desc"
	OrdemDivida     = "divida_desc"
	OrdemAleatoria  = "aleatoria"
	OrdemBase       = "base"
)

// Limites da contagem e da ordenação (variáveis para os testes poderem
// exercitar o teto com a base de amostra).
var (
	// TetoContagem: a contagem com teto para em TetoContagem+1 linhas.
	TetoContagem int64 = 10_000
	// LimiteContagemExata: até este limite superior (dado pelos cubos) a
	// contagem fora dos cubos é feita de forma exata.
	LimiteContagemExata int64 = 1_000_000
	// ConjuntoPequeno: até aqui o serviço aplica a ordem pedida.
	ConjuntoPequeno int64 = 200_000
)

const (
	MaximoPrevia     = 25
	MaximoCandidatos = 5000
	MaximoDetalhes   = 500
	MaximoCNPJs      = 5000
)

var (
	// ErrNaoPronta: ainda não existe esquema de serviço utilizável.
	ErrNaoPronta = errors.New("a base ainda não foi carregada")
	// ErrCursorAntigo: a base foi trocada depois que o cursor foi emitido.
	ErrCursorAntigo = errors.New("a base foi atualizada; refaça a busca")
	// ErrNaoEncontrado: CNPJ fora da base de empresas ativas.
	ErrNaoEncontrado = errors.New("empresa não encontrada na base de empresas ativas")
)

// ErroFiltro é um pedido inválido (HTTP 422).
type ErroFiltro struct{ Mensagem string }

func (e *ErroFiltro) Error() string { return e.Mensagem }

func invalido(m string) error { return &ErroFiltro{Mensagem: m} }

// ErrFiltroAmplo: a consulta estourou o statement_timeout.
var ErrFiltroAmplo = &ErroFiltro{Mensagem: "filtro amplo demais: acrescente cidade ou CNAE"}

// Filtros é o filtro de empresas do contrato.
type Filtros struct {
	UFs                 []string `json:"ufs,omitempty"`
	Municipios          []int    `json:"municipios,omitempty"`
	Cnaes               []int    `json:"cnaes,omitempty"`
	CnaePrefixos        []string `json:"cnae_prefixos,omitempty"`
	CnaeSecundario      bool     `json:"cnae_secundario,omitempty"`
	Portes              []string `json:"portes,omitempty"`
	Naturezas           []int    `json:"naturezas,omitempty"`
	Matriz              string   `json:"matriz,omitempty"`
	Simples             string   `json:"simples,omitempty"`
	MEI                 string   `json:"mei,omitempty"`
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
	Divida              string   `json:"divida,omitempty"`
	DividaMin           *float64 `json:"divida_min,omitempty"`
	DividaMax           *float64 `json:"divida_max,omitempty"`
	Bairros             []string `json:"bairros,omitempty"`
	CEPs                []string `json:"ceps,omitempty"`
	Nome                string   `json:"nome,omitempty"`
	CNPJs               []string `json:"cnpjs,omitempty"`
}

// Parciais são os recortes da contagem; nulos quando o total é "teto".
type Parciais struct {
	ComCelular *int64 `json:"com_celular"`
	ComEmail   *int64 `json:"com_email"`
	ComDivida  *int64 `json:"com_divida"`
}

// Contagem é a resposta de POST /v1/empresas/contagem. total_modo "exato":
// total é o número; "teto": a contagem parou em 10.001 e total vale 10.000
// ("mais de 10.000"), com total_limite_superior quando um cubo dá o limite.
type Contagem struct {
	Total               int64    `json:"total"`
	TotalModo           string   `json:"total_modo"`
	TotalLimiteSuperior *int64   `json:"total_limite_superior,omitempty"`
	Parciais            Parciais `json:"parciais"`
}

type Codigo struct {
	Codigo    int    `json:"codigo"`
	Descricao string `json:"descricao"`
}

type Telefone struct {
	Numero        string `json:"numero"`
	Tipo          string `json:"tipo"`
	Compartilhado bool   `json:"compartilhado"`
	Origem        string `json:"origem"`
	Principal     bool   `json:"principal"`
}

type Socio struct {
	Nome          string  `json:"nome"`
	Qualificacao  string  `json:"qualificacao"`
	Desde         *string `json:"desde"`
	Tipo          string  `json:"tipo"`
	Documento     string  `json:"documento"`
	Representante string  `json:"representante,omitempty"`
}

type Divida struct {
	Total      float64  `json:"total"`
	Inscricoes int      `json:"inscricoes"`
	EmCobranca float64  `json:"em_cobranca"`
	Regular    float64  `json:"regular"`
	Tipos      []string `json:"tipos"`
	Ajuizada   bool     `json:"ajuizada"`
	MaisAntiga *string  `json:"mais_antiga"`
	Referencia string   `json:"referencia"`
}

type Endereco struct {
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

// Empresa é a ficha inteira (detalhes e GET /v1/empresas/{cnpj}).
type Empresa struct {
	CNPJ             string     `json:"cnpj"`
	Matriz           bool       `json:"matriz"`
	RazaoSocial      string     `json:"razao_social"`
	NomeFantasia     string     `json:"nome_fantasia"`
	Natureza         Codigo     `json:"natureza"`
	PessoaFisica     bool       `json:"pessoa_fisica"`
	Porte            string     `json:"porte"`
	Capital          *float64   `json:"capital"`
	Abertura         *string    `json:"abertura"`
	Situacao         string     `json:"situacao"`
	SituacaoDesde    *string    `json:"situacao_desde"`
	Simples          bool       `json:"simples"`
	MEI              bool       `json:"mei"`
	Cnae             Codigo     `json:"cnae"`
	CnaesSecundarios []Codigo   `json:"cnaes_secundarios"`
	Endereco         Endereco   `json:"endereco"`
	Telefones        []Telefone `json:"telefones"`
	Email            string     `json:"email"`
	EmailTipo        string     `json:"email_tipo"`
	Socios           []Socio    `json:"socios"`
	Divida           *Divida    `json:"divida"`
	Qualidade        int        `json:"qualidade"`
	Referencia       string     `json:"referencia"`
}

// EmpresaPrevia é a linha da prévia gratuita: sem contato e sem nomes de
// sócios.
type EmpresaPrevia struct {
	CNPJ         string   `json:"cnpj"`
	RazaoSocial  string   `json:"razao_social"`
	NomeFantasia string   `json:"nome_fantasia"`
	Cnae         Codigo   `json:"cnae"`
	Porte        string   `json:"porte"`
	Abertura     *string  `json:"abertura"`
	Capital      *float64 `json:"capital"`
	Municipio    string   `json:"municipio"`
	UF           string   `json:"uf"`
	PessoaFisica bool     `json:"pessoa_fisica"`
	MEI          bool     `json:"mei"`
	TemCelular   bool     `json:"tem_celular"`
	TemTelefone  bool     `json:"tem_telefone"`
	TemEmail     bool     `json:"tem_email"`
	TemDivida    bool     `json:"tem_divida"`
	SociosQtd    int      `json:"socios_qtd"`
	Qualidade    int      `json:"qualidade"`
}

// Previa é a resposta de POST /v1/empresas/previa.
type Previa struct {
	Itens         []EmpresaPrevia `json:"itens"`
	Versao        string          `json:"versao"`
	OrdemAplicada string          `json:"ordem_aplicada"`
}

// Candidatos é a resposta de POST /v1/empresas/candidatos.
type Candidatos struct {
	CNPJs         []string `json:"cnpjs"`
	Cursor        string   `json:"cursor"`
	Fim           bool     `json:"fim"`
	Versao        string   `json:"versao"`
	OrdemAplicada string   `json:"ordem_aplicada"`
}

// Detalhes é a resposta de POST /v1/empresas/detalhes.
type Detalhes struct {
	Itens          []Empresa `json:"itens"`
	NaoEncontrados []string  `json:"nao_encontrados"`
}

// Base resume o esquema de serviço em uso (parte de GET /v1/situacao).
type Base struct {
	Pronta         bool       `json:"pronta"`
	Amostra        bool       `json:"amostra"`
	ReferenciaCNPJ string     `json:"referencia_cnpj"`
	ReferenciaPGFN string     `json:"referencia_pgfn"`
	Versao         string     `json:"versao"`
	Empresas       int64      `json:"empresas"`
	Socios         int64      `json:"socios"`
	Devedores      int64      `json:"devedores"`
	AtualizadaEm   *time.Time `json:"atualizada_em"`
	EspacoBytes    int64      `json:"espaco_bytes"`
	UFs            []string   `json:"ufs"`
}

type ItemCnae struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	Nivel     string `json:"nivel"`
}

type ItemMunicipio struct {
	Codigo   int    `json:"codigo"`
	Nome     string `json:"nome"`
	UF       string `json:"uf"`
	Empresas int    `json:"empresas"`
}

type ItemNatureza struct {
	Codigo       int    `json:"codigo"`
	Descricao    string `json:"descricao"`
	PessoaFisica bool   `json:"pessoa_fisica"`
}

type ItemUF struct {
	UF       string `json:"uf"`
	Nome     string `json:"nome"`
	Empresas int    `json:"empresas"`
}
