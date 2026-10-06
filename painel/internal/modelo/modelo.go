// Package modelo define os tipos de domínio do painel.
package modelo

import (
	"fmt"
	"time"
)

type Perfil string

const (
	PerfilAdmin      Perfil = "admin"
	PerfilComercial  Perfil = "comercial"
	PerfilFinanceiro Perfil = "financeiro"
	PerfilSuporte    Perfil = "suporte"
)

func (p Perfil) Rotulo() string {
	switch p {
	case PerfilAdmin:
		return "Administrador"
	case PerfilComercial:
		return "Comercial"
	case PerfilFinanceiro:
		return "Financeiro"
	case PerfilSuporte:
		return "Suporte"
	}
	return string(p)
}

func (p Perfil) Valido() bool {
	switch p {
	case PerfilAdmin, PerfilComercial, PerfilFinanceiro, PerfilSuporte:
		return true
	}
	return false
}

type Usuario struct {
	ID          int64
	Nome        string
	Email       string
	SenhaHash   string
	Perfil      Perfil
	Ativo       bool
	UltimoLogin *time.Time
	CriadoEm    time.Time
}

type Plano struct {
	ID                  int64
	Codigo              string
	Nome                string
	Descricao           string
	UsuariosMin         int
	UsuariosMax         int
	PrecoMensalCentavos int64
	PrecoAnualCentavos  int64
	Destaque            bool
	Recursos            []string
	Ordem               int
	Ativo               bool
}

// SobConsulta é o plano sem preço: a landing mostra "fale conosco".
func (p Plano) SobConsulta() bool { return p.PrecoMensalCentavos == 0 }

func (p Plano) Faixa() string {
	if p.UsuariosMax >= 999 {
		return fmt.Sprintf("a partir de %d usuários", p.UsuariosMin)
	}
	if p.UsuariosMin == p.UsuariosMax {
		return fmt.Sprintf("%d usuário(s)", p.UsuariosMin)
	}
	return fmt.Sprintf("%d a %d usuários", p.UsuariosMin, p.UsuariosMax)
}

type Configuracao struct {
	Chave, Valor, Descricao string
}

type Grupo struct {
	ID                int64
	Nome              string
	CNPJResponsavel   string
	EmailFinanceiro   string
	Telefone          string
	CobrancaUnificada bool
	Observacoes       string
	Ativo             bool
	CEP, Logradouro   string
	Numero, Bairro    string
	Municipio, UF     string
	PagadorID         string
	CriadoEm          time.Time
	TotalClientes     int
	ClientesAtivos    int
}

type StatusCliente string

const (
	StatusPronto        StatusCliente = "pronto"
	StatusProvisionando StatusCliente = "provisionando"
	StatusAtivo         StatusCliente = "ativo"
	StatusSuspenso      StatusCliente = "suspenso"
	StatusErro          StatusCliente = "erro"
	StatusCancelado     StatusCliente = "cancelado"
)

func (s StatusCliente) Rotulo() string {
	switch s {
	case StatusPronto:
		return "Pronto para provisionar"
	case StatusProvisionando:
		return "Provisionando…"
	case StatusAtivo:
		return "Ativo"
	case StatusSuspenso:
		return "Suspenso"
	case StatusErro:
		return "Erro"
	case StatusCancelado:
		return "Cancelado"
	}
	return string(s)
}

func (s StatusCliente) Cor() string {
	switch s {
	case StatusAtivo:
		return "ok"
	case StatusPronto, StatusProvisionando:
		return "atencao"
	case StatusSuspenso, StatusErro:
		return "erro"
	}
	return "neutro"
}

type Cliente struct {
	ID                  int64
	GrupoID             int64
	GrupoNome           string
	RazaoSocial         string
	NomeFantasia        string
	CNPJ                string
	Slug                string
	Segmento            string
	Cidade, UF          string
	Telefone            string
	CEP, Logradouro     string
	Numero, Bairro      string
	AdminNome           string
	AdminEmail          string
	PlanoID             *int64
	PlanoNome           string
	UsuariosContratados int
	CobrancaPropria     bool
	PagadorID           string
	ModeloCRM           string

	Status                   StatusCliente
	Versao                   string
	AutoAtualizar            bool
	CorPrimaria              string
	LogoURL                  string
	URL                      string
	ProvisionadoEm           *time.Time
	UltimoErro               string
	SuspensoPorInadimplencia bool
	CriadoEm                 time.Time
}

func (c Cliente) Provisionado() bool {
	return c.Status == StatusAtivo || c.Status == StatusSuspenso
}

const (
	PagamentoAguardandoCartao = "aguardando_cartao"
	PagamentoAtivo            = "ativo"
	PagamentoRecusado         = "recusado"
	PagamentoCancelado        = "cancelado"
	PagamentoManual           = "manual"
)

type Licenca struct {
	ID            int64
	GrupoID       int64
	GrupoNome     string
	ClienteID     *int64
	ClienteNome   string
	PlanoID       int64
	PlanoNome     string
	UsuariosMax   int
	Periodicidade string
	ValorCentavos int64
	Inicio        time.Time
	Fim           time.Time
	Status        string
	Unidades      int
	Observacoes   string

	Cobranca        string
	AssinaturaID    string
	AssinaturaToken string
	CodigoCheckout  string
	PagamentoStatus string
	CartaoFinal     string
	CartaoBandeira  string
	CartaoEm        *time.Time
	CriadoEm        time.Time
}

func (l Licenca) DoGrupo() bool { return l.ClienteID == nil }

func (l Licenca) Vigente(hoje time.Time) bool {
	d := hoje.Truncate(24 * time.Hour)
	return l.Status == "ativa" && !d.Before(l.Inicio) && !d.After(l.Fim)
}

func (l Licenca) DiasParaVencer(hoje time.Time) int {
	return int(l.Fim.Sub(hoje.Truncate(24*time.Hour)).Hours() / 24)
}

func (l Licenca) Recorrente() bool { return l.Cobranca == "recorrente" }

func (l Licenca) PagamentoRotulo() string {
	switch l.PagamentoStatus {
	case PagamentoAguardandoCartao:
		return "aguardando cartão"
	case PagamentoAtivo:
		if l.CartaoFinal != "" {
			return "cartão •••• " + l.CartaoFinal
		}
		return "recorrência ativa"
	case PagamentoRecusado:
		return "cartão recusado"
	case PagamentoCancelado:
		return "recorrência cancelada"
	case PagamentoManual:
		return "cobrança manual"
	}
	return l.PagamentoStatus
}

type Fatura struct {
	ID            int64
	LicencaID     int64
	Competencia   time.Time
	Vencimento    time.Time
	ValorCentavos int64
	Status        string
	Forma         string
	Referencia    string
	PagoEm        *time.Time
	CriadoEm      time.Time

	GrupoID         int64
	ClienteID       *int64
	GrupoNome       string
	ClienteNome     string
	PlanoNome       string
	AssinaturaID    string
	PagamentoStatus string
}

func (f Fatura) EmAberto() bool { return f.Status == "pendente" || f.Status == "vencida" }

type PlanoSolicitacao struct {
	ID          int64
	ClienteID   int64
	Tipo        string
	PlanoID     *int64
	Mensagem    string
	Status      string
	Resposta    string
	CriadoEm    time.Time
	DecididoEm  *time.Time
	DecididoPor string
	ClienteNome string
	ClienteSlug string
	PlanoNome   string
}

type Versao struct {
	ID          int64
	Tag         string
	Descricao   string
	Changelog   string
	Estavel     bool
	Padrao      bool
	PublicadaEm time.Time
	Clientes    int
}

type Lead struct {
	ID             int64
	Nome, Email    string
	Telefone       string
	Empresa        string
	Segmento       string
	Cidade         string
	Usuarios       int
	PlanoInteresse string
	Mensagem       string
	Origem, Status string
	CriadoEm       time.Time
}

type Auditoria struct {
	ID         int64
	Usuario    string
	Acao       string
	Entidade   string
	EntidadeID *int64
	Detalhes   string
	IP         string
	CriadoEm   time.Time
}

type EventoAmbiente struct {
	ID        int64
	ClienteID int64
	Tipo      string
	Detalhe   string
	CriadoEm  time.Time
}

type Resumo struct {
	Grupos                 int
	ClientesAtivos         int
	ClientesAguardando     int
	ClientesSuspensos      int
	LicencasVencendo       int
	LicencasSemCartao      int
	FaturasPendentes       int
	FaturasVencidas        int
	LeadsNovos             int
	SolicitacoesPendentes  int
	MRRCentavos            int64
	ClientesDesatualizados int
}

// FormatarBRL formata centavos como R$ 1.234,56.
func FormatarBRL(centavos int64) string {
	neg := centavos < 0
	if neg {
		centavos = -centavos
	}
	reais := centavos / 100
	cents := centavos % 100
	s := fmt.Sprintf("%d", reais)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	r := fmt.Sprintf("R$ %s,%02d", string(out), cents)
	if neg {
		return "-" + r
	}
	return r
}
