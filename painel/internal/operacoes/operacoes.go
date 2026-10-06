// Package operacoes reúne os casos de uso que cruzam mais de um sistema:
// provisionar um cliente (painel + provisionador), contratar e cobrar uma
// licença (painel + recorrência), mudar versão, suspender e reativar. Os
// handlers HTTP só traduzem formulário ↔ chamada daqui.
package operacoes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/provisionador"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
	"github.com/armando-couto/crm-ia/painel/internal/sign"
)

type Servico struct {
	repo *repo.Repo
	prov *provisionador.Cliente
	sign *sign.Cliente
	// NomeProduto aparece na assinatura e nos textos ("CRM IA · Profissional").
	NomeProduto string
	// URLCheckout monta o endereço público da página de cartão (%s = código).
	URLCheckout func(codigo string) string
}

func Novo(r *repo.Repo, prov *provisionador.Cliente, sg *sign.Cliente) *Servico {
	return &Servico{repo: r, prov: prov, sign: sg, NomeProduto: "CRM IA",
		URLCheckout: func(c string) string { return "/painel/assinar/" + c }}
}

func (s *Servico) Sign() *sign.Cliente { return s.sign }

// prazoOperacao acompanha o do cliente do provisionador com folga.
const prazoOperacao = 9 * time.Minute

// destacar solta a operação do contexto da requisição: fechar a aba não pode
// matar um `compose up` no meio.
func destacar(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), prazoOperacao)
}

// -----------------------------------------------------------------------------
// Provisionamento e ambiente
// -----------------------------------------------------------------------------

type ResultadoProvisionamento struct {
	URL        string
	Versao     string
	SenhaAdmin string
}

// Provisionar sobe (ou recria) o ambiente do cliente.
func (s *Servico) Provisionar(ctx context.Context, clienteID int64, versao string) (ResultadoProvisionamento, error) {
	var res ResultadoProvisionamento
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return res, err
	}
	switch c.Status {
	case modelo.StatusCancelado:
		return res, errors.New("cliente cancelado")
	case modelo.StatusSuspenso:
		return res, errors.New("ambiente suspenso: reative o acesso antes de recriar os containers")
	}
	jaNoAr := c.Provisionado()
	if versao == "" {
		versao = c.Versao
	}
	if versao == "" {
		if v, err := s.repo.VersaoPadrao(ctx); err == nil {
			versao = v.Tag
		} else {
			versao = "latest"
		}
	}
	if !jaNoAr {
		_ = s.repo.AtualizarStatusCliente(ctx, c.ID, modelo.StatusProvisionando, "")
	}
	r, err := s.prov.Criar(ctx, provisionador.Pedido{
		Slug: c.Slug, Nome: c.NomeFantasia, CNPJ: c.CNPJ, AdminNome: c.AdminNome, AdminEmail: c.AdminEmail,
		Versao: versao, UsuariosMax: c.UsuariosContratados, CorPrimaria: c.CorPrimaria, LogoURL: c.LogoURL,
	})
	if err != nil {
		status := modelo.StatusErro
		if jaNoAr {
			status = c.Status
		}
		_ = s.repo.AtualizarStatusCliente(ctx, c.ID, status, err.Error())
		s.repo.RegistrarEvento(ctx, c.ID, "erro", "provisionamento falhou: "+err.Error())
		return res, err
	}
	if err := s.repo.MarcarProvisionado(ctx, c.ID, r.URL, r.Versao); err != nil {
		return res, err
	}
	s.repo.RegistrarEvento(ctx, c.ID, "provisionado", "ambiente no ar em "+r.URL+" (versão "+r.Versao+")")
	return ResultadoProvisionamento{URL: r.URL, Versao: r.Versao, SenhaAdmin: r.SenhaAdmin}, nil
}

func (s *Servico) AtualizarVersao(ctx context.Context, clienteID int64, versao string) error {
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return err
	}
	if !c.Provisionado() {
		return errors.New("cliente ainda não provisionado")
	}
	r, err := s.prov.Atualizar(ctx, c.Slug, provisionador.Atualizacao{Versao: versao})
	if err != nil {
		s.repo.RegistrarEvento(ctx, c.ID, "erro", "atualização para "+versao+" falhou: "+err.Error())
		return err
	}
	if err := s.repo.AtualizarVersaoCliente(ctx, c.ID, r.Versao); err != nil {
		return err
	}
	s.repo.RegistrarEvento(ctx, c.ID, "atualizado", "ambiente atualizado para a versão "+r.Versao)
	return nil
}

// AtualizarAmbiente empurra nome/cor/logo e o teto de usuários ao container.
func (s *Servico) AtualizarAmbiente(ctx context.Context, c modelo.Cliente) error {
	if !c.Provisionado() {
		return nil
	}
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	usuarios := c.UsuariosContratados
	_, err := s.prov.Atualizar(ctx, c.Slug, provisionador.Atualizacao{UsuariosMax: &usuarios, CorPrimaria: &c.CorPrimaria, LogoURL: &c.LogoURL, Nome: &c.NomeFantasia})
	return err
}

func (s *Servico) AtualizarLimiteUsuarios(ctx context.Context, c modelo.Cliente, usuarios int) error {
	if !c.Provisionado() || usuarios <= 0 {
		return nil
	}
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	_, err := s.prov.Atualizar(ctx, c.Slug, provisionador.Atualizacao{UsuariosMax: &usuarios})
	return err
}

func (s *Servico) Suspender(ctx context.Context, clienteID int64, motivo string) error {
	return s.suspender(ctx, clienteID, motivo, false)
}

func (s *Servico) SuspenderPorInadimplencia(ctx context.Context, clienteID int64, motivo string) error {
	return s.suspender(ctx, clienteID, motivo, true)
}

func (s *Servico) suspender(ctx context.Context, clienteID int64, motivo string, porInadimplencia bool) error {
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return err
	}
	if c.Status != modelo.StatusAtivo {
		return fmt.Errorf("cliente está %s; só ambientes ativos podem ser suspensos", c.Status.Rotulo())
	}
	if err := s.prov.Suspender(ctx, c.Slug); err != nil {
		return err
	}
	_ = s.repo.MarcarSuspenso(ctx, c.ID, porInadimplencia)
	s.repo.RegistrarEvento(ctx, c.ID, "suspenso", motivo)
	return nil
}

func (s *Servico) Reativar(ctx context.Context, clienteID int64) error {
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return err
	}
	if c.Status != modelo.StatusSuspenso {
		return errors.New("só ambientes suspensos podem ser reativados")
	}
	if err := s.prov.Reativar(ctx, c.Slug); err != nil {
		return err
	}
	_ = s.repo.AtualizarStatusCliente(ctx, c.ID, modelo.StatusAtivo, "")
	s.repo.RegistrarEvento(ctx, c.ID, "reativado", "acesso restabelecido")
	return nil
}

func (s *Servico) Backup(ctx context.Context, clienteID int64) (string, error) {
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return "", err
	}
	arquivo, err := s.prov.Backup(ctx, c.Slug)
	if err != nil {
		return "", err
	}
	s.repo.RegistrarEvento(ctx, c.ID, "backup", arquivo)
	return arquivo, nil
}

// Remover apaga o ambiente (com backup) e cancela o cliente no painel.
func (s *Servico) Remover(ctx context.Context, clienteID int64) error {
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	c, err := s.repo.ClientePorID(ctx, clienteID)
	if err != nil {
		return err
	}
	if c.Provisionado() || c.Status == modelo.StatusErro || c.Status == modelo.StatusProvisionando {
		if err := s.prov.Remover(ctx, c.Slug); err != nil {
			var pe *provisionador.Erro
			if !errors.As(err, &pe) || !strings.Contains(pe.Mensagem, "não está provisionado") {
				return err
			}
		}
	}
	_ = s.repo.AtualizarStatusCliente(ctx, c.ID, modelo.StatusCancelado, "")
	s.repo.RegistrarEvento(ctx, c.ID, "removido", "ambiente removido (backup guardado)")
	return nil
}

// -----------------------------------------------------------------------------
// Licenças e cobrança recorrente
// -----------------------------------------------------------------------------

type PedidoLicenca struct {
	GrupoID       int64
	ClienteID     *int64
	PlanoID       int64
	UsuariosMax   int
	Periodicidade string // mensal | anual
	Inicio        time.Time
	Unidades      int
	Observacoes   string
	// Cobranca: "recorrente" cria a assinatura no cartão agora e gera o link
	// de cadastro do cartão; "manual" é para quem paga por fora.
	Cobranca string
}

// CalcularValor: plano × unidades (o anual usa o preço anual quando existe).
func (s *Servico) CalcularValor(ctx context.Context, p PedidoLicenca) (int64, modelo.Plano, error) {
	plano, err := s.repo.PlanoPorID(ctx, p.PlanoID)
	if err != nil {
		return 0, plano, err
	}
	unidades := int64(p.Unidades)
	if unidades <= 0 {
		unidades = 1
	}
	base := plano.PrecoMensalCentavos
	meses := int64(1)
	if p.Periodicidade == "anual" {
		if plano.PrecoAnualCentavos > 0 {
			base = plano.PrecoAnualCentavos
		} else {
			meses = 12
		}
	}
	return base * meses * unidades, plano, nil
}

// ResultadoLicenca é o que a tela mostra depois de contratar.
type ResultadoLicenca struct {
	ID          int64
	URLCheckout string
}

// ContratarLicenca cria a licença e, na cobrança recorrente, a assinatura: o
// pagador (grupo ou CNPJ) é garantido na recorrência, a assinatura mensal é
// criada e nasce o código da página de cartão com a marca do CRM IA.
func (s *Servico) ContratarLicenca(ctx context.Context, p PedidoLicenca) (ResultadoLicenca, error) {
	var res ResultadoLicenca
	grupo, err := s.repo.GrupoPorID(ctx, p.GrupoID)
	if err != nil {
		return res, err
	}
	if p.Periodicidade != "mensal" && p.Periodicidade != "anual" {
		return res, errors.New("periodicidade inválida")
	}
	if p.Cobranca == "" {
		p.Cobranca = "recorrente"
	}
	if p.Inicio.IsZero() {
		p.Inicio = time.Now()
	}
	fim := p.Inicio.AddDate(0, 1, 0)
	if p.Periodicidade == "anual" {
		fim = p.Inicio.AddDate(1, 0, 0)
	}
	valor, plano, err := s.CalcularValor(ctx, p)
	if err != nil {
		return res, err
	}
	if p.UsuariosMax <= 0 {
		p.UsuariosMax = plano.UsuariosMax
	}
	lic := modelo.Licenca{GrupoID: p.GrupoID, ClienteID: p.ClienteID, PlanoID: p.PlanoID, UsuariosMax: p.UsuariosMax,
		Periodicidade: p.Periodicidade, ValorCentavos: valor, Inicio: p.Inicio, Fim: fim, Unidades: max(p.Unidades, 1),
		Observacoes: p.Observacoes, Cobranca: p.Cobranca, PagamentoStatus: modelo.PagamentoManual}

	if p.Cobranca == "recorrente" {
		if !s.sign.Configurado() {
			return res, errors.New("cobrança recorrente indisponível: " + s.sign.MotivoNaoConfigurado())
		}
		if p.Periodicidade != "mensal" {
			return res, errors.New("a cobrança recorrente é mensal; para o período anual use a cobrança manual")
		}
		documento, err := s.garantirPagador(ctx, grupo, p.ClienteID)
		if err != nil {
			return res, err
		}
		nome := s.NomeProduto + " · " + plano.Nome
		if p.ClienteID != nil {
			if c, err := s.repo.ClientePorID(ctx, *p.ClienteID); err == nil {
				nome += " · " + c.NomeFantasia
			}
		} else {
			nome += " · " + grupo.Nome
		}
		ass, err := s.sign.CriarAssinatura(ctx, sign.NovaAssinatura{Documento: documento, ValorCentavos: valor, Produto: nome, PrimeiraCobranca: p.Inicio})
		if err != nil {
			return res, fmt.Errorf("recorrência: %w", err)
		}
		codigo, err := auth.CodigoAleatorio(24)
		if err != nil {
			return res, err
		}
		lic.AssinaturaID, lic.AssinaturaToken, lic.CodigoCheckout = ass.ID, ass.Token, codigo
		lic.PagamentoStatus = modelo.PagamentoAguardandoCartao
	}

	id, err := s.repo.CriarLicenca(ctx, lic)
	if err != nil {
		return res, err
	}
	_, _, _ = s.repo.CriarFatura(ctx, modelo.Fatura{LicencaID: id, Competencia: primeiroDia(p.Inicio), Vencimento: p.Inicio, ValorCentavos: valor})
	if p.ClienteID != nil {
		if c, err := s.repo.ClientePorID(ctx, *p.ClienteID); err == nil {
			_ = s.AtualizarLimiteUsuarios(ctx, c, p.UsuariosMax)
		}
	}
	res.ID = id
	if lic.CodigoCheckout != "" {
		res.URLCheckout = s.URLCheckout(lic.CodigoCheckout)
	}
	return res, nil
}

// garantirPagador cadastra (uma vez) o pagador na recorrência e devolve o documento.
func (s *Servico) garantirPagador(ctx context.Context, grupo modelo.Grupo, clienteID *int64) (string, error) {
	if clienteID != nil {
		c, err := s.repo.ClientePorID(ctx, *clienteID)
		if err != nil {
			return "", err
		}
		if c.PagadorID != "" {
			return c.CNPJ, nil
		}
		id, err := s.sign.GarantirPagador(ctx, sign.Pagador{Nome: c.RazaoSocial, Documento: c.CNPJ, Email: c.AdminEmail, Telefone: c.Telefone,
			CEP: c.CEP, Logradouro: c.Logradouro, Numero: c.Numero, Bairro: c.Bairro, Municipio: c.Cidade, UF: c.UF})
		if err != nil {
			return "", fmt.Errorf("pagador: %w", err)
		}
		return c.CNPJ, s.repo.DefinirPagadorCliente(ctx, c.ID, id)
	}
	if grupo.CNPJResponsavel == "" {
		return "", errors.New("informe o CNPJ responsável do grupo para a cobrança recorrente")
	}
	if grupo.PagadorID != "" {
		return grupo.CNPJResponsavel, nil
	}
	id, err := s.sign.GarantirPagador(ctx, sign.Pagador{Nome: grupo.Nome, Documento: grupo.CNPJResponsavel, Email: grupo.EmailFinanceiro, Telefone: grupo.Telefone,
		CEP: grupo.CEP, Logradouro: grupo.Logradouro, Numero: grupo.Numero, Bairro: grupo.Bairro, Municipio: grupo.Municipio, UF: grupo.UF})
	if err != nil {
		return "", fmt.Errorf("pagador: %w", err)
	}
	return grupo.CNPJResponsavel, s.repo.DefinirPagadorGrupo(ctx, grupo.ID, id)
}

// CancelarLicenca encerra a licença e a recorrência.
func (s *Servico) CancelarLicenca(ctx context.Context, id int64) error {
	l, err := s.repo.LicencaPorID(ctx, id)
	if err != nil {
		return err
	}
	if l.Recorrente() && l.AssinaturaID != "" && l.PagamentoStatus == modelo.PagamentoAtivo {
		if err := s.sign.AlternarAssinatura(ctx, l.AssinaturaID); err != nil {
			return fmt.Errorf("a licença não foi cancelada porque a recorrência recusou o cancelamento: %w", err)
		}
	}
	return s.repo.CancelarLicenca(ctx, id)
}

// -----------------------------------------------------------------------------
// Checkout do cartão (página com a marca do CRM IA)
// -----------------------------------------------------------------------------

var (
	ErrCheckoutInvalido  = errors.New("este link de pagamento não existe ou expirou")
	ErrCheckoutConcluido = errors.New("o cartão desta assinatura já está cadastrado")
)

// Checkout é o que a página pública mostra.
type Checkout struct {
	Licenca   modelo.Licenca
	Pagador   string
	Documento string
	Produto   string
}

func (s *Servico) Checkout(ctx context.Context, codigo string) (Checkout, error) {
	l, err := s.repo.LicencaPorCheckout(ctx, codigo)
	if err != nil || l.Status == "cancelada" {
		return Checkout{}, ErrCheckoutInvalido
	}
	c := Checkout{Licenca: l, Produto: s.NomeProduto + " · " + l.PlanoNome}
	if l.ClienteID != nil {
		if cli, err := s.repo.ClientePorID(ctx, *l.ClienteID); err == nil {
			c.Pagador, c.Documento = cli.RazaoSocial, cli.CNPJ
		}
	} else if g, err := s.repo.GrupoPorID(ctx, l.GrupoID); err == nil {
		c.Pagador, c.Documento = g.Nome, g.CNPJResponsavel
	}
	return c, nil
}

// ResultadoCheckout: aprovado, recusado ou redirecionamento para autenticação (3DS).
type ResultadoCheckout struct {
	Aprovado bool
	URL3DS   string
	Mensagem string
	Final    string
}

// CadastrarCartao recebe o cartão da página e o entrega à recorrência. Os
// dados do cartão passam e não ficam; o que se grava é bandeira e final.
func (s *Servico) CadastrarCartao(ctx context.Context, codigo string, cartao sign.Cartao) (ResultadoCheckout, error) {
	l, err := s.repo.LicencaPorCheckout(ctx, codigo)
	if err != nil || l.Status == "cancelada" {
		return ResultadoCheckout{}, ErrCheckoutInvalido
	}
	if !l.Recorrente() || l.AssinaturaToken == "" {
		return ResultadoCheckout{}, ErrCheckoutInvalido
	}
	r, err := s.sign.CadastrarCartao(ctx, l.AssinaturaToken, cartao)
	if err != nil {
		return ResultadoCheckout{}, err
	}
	if r.URL3DS != "" {
		return ResultadoCheckout{URL3DS: r.URL3DS, Final: r.Final}, nil
	}
	if !r.Aprovado {
		_ = s.repo.RegistrarCartao(ctx, l.ID, modelo.PagamentoRecusado, r.Final, r.Bandeira)
		return ResultadoCheckout{Mensagem: r.Mensagem, Final: r.Final}, nil
	}
	if err := s.repo.RegistrarCartao(ctx, l.ID, modelo.PagamentoAtivo, r.Final, r.Bandeira); err != nil {
		return ResultadoCheckout{}, err
	}
	// A primeira cobrança acontece na adesão: a fatura de abertura é baixada.
	if abertas, err := s.repo.FaturasAbertasDaLicenca(ctx, l.ID); err == nil && len(abertas) > 0 {
		_, _ = s.RegistrarPagamento(ctx, abertas[0].ID, "cartao", r.Referencia)
	}
	return ResultadoCheckout{Aprovado: true, Final: r.Final}, nil
}

// -----------------------------------------------------------------------------
// Faturas: geração, conferência e baixa
// -----------------------------------------------------------------------------

type Pagamento struct {
	Baixada    bool
	Reativados []string
}

// RegistrarPagamento dá baixa na fatura e aplica as consequências: estende a
// licença até o fim do ciclo e reativa os ambientes suspensos por
// inadimplência que ela cobre. Idempotente.
func (s *Servico) RegistrarPagamento(ctx context.Context, faturaID int64, forma, referencia string) (Pagamento, error) {
	var res Pagamento
	ctx, cancelar := destacar(ctx)
	defer cancelar()
	f, err := s.repo.FaturaPorID(ctx, faturaID)
	if err != nil {
		return res, err
	}
	baixou, err := s.repo.BaixarFatura(ctx, f.ID, forma, referencia)
	if err != nil || !baixou {
		return res, err
	}
	res.Baixada = true
	l, err := s.repo.LicencaPorID(ctx, f.LicencaID)
	if err != nil {
		return res, err
	}
	fim := fimDoCiclo(l, f.Competencia)
	if err := s.repo.EstenderLicenca(ctx, l.ID, fim); err != nil {
		return res, err
	}
	l, _ = s.repo.LicencaPorID(ctx, l.ID)
	// Quitou o atraso com a licença já vencida no calendário? Ela recomeça a
	// contar de hoje: quem paga volta a ter um período inteiro pela frente.
	if !l.Vigente(time.Now()) && l.Status != "cancelada" {
		if abertas, err := s.repo.FaturasAbertasDaLicenca(ctx, l.ID); err == nil && len(abertas) == 0 {
			novoFim := time.Now().AddDate(0, 1, 0)
			if l.Periodicidade == "anual" {
				novoFim = time.Now().AddDate(1, 0, 0)
			}
			if err := s.repo.EstenderLicenca(ctx, l.ID, novoFim); err != nil {
				return res, err
			}
			l, _ = s.repo.LicencaPorID(ctx, l.ID)
		}
	}
	if !l.Vigente(time.Now()) {
		return res, nil
	}
	suspensos, err := s.repo.SuspensosPorInadimplenciaDaLicenca(ctx, l)
	if err != nil {
		return res, nil
	}
	for _, c := range suspensos {
		if err := s.Reativar(ctx, c.ID); err != nil {
			slog.Error("reativando após pagamento", "slug", c.Slug, "erro", err)
			continue
		}
		res.Reativados = append(res.Reativados, c.Slug)
	}
	return res, nil
}

// fimDoCiclo: competência + período, nunca antes do fim atual.
func fimDoCiclo(l modelo.Licenca, competencia time.Time) time.Time {
	fim := competencia.AddDate(0, 1, 0)
	if l.Periodicidade == "anual" {
		fim = competencia.AddDate(1, 0, 0)
	}
	if fim.Before(l.Fim) {
		return l.Fim
	}
	return fim
}

// ConferirPagamentos consulta a recorrência para cada licença com fatura em
// aberto e baixa as faturas pagas. Uma falha numa licença não impede as
// outras. Devolve quantas faturas foram baixadas.
func (s *Servico) ConferirPagamentos(ctx context.Context) (int, error) {
	if s.sign.Simulado() {
		return 0, nil
	}
	faturas, err := s.repo.FaturasAbertasRecorrentes(ctx)
	if err != nil {
		return 0, err
	}
	porLicenca := map[int64][]modelo.Fatura{}
	for _, f := range faturas {
		porLicenca[f.LicencaID] = append(porLicenca[f.LicencaID], f)
	}
	baixadas := 0
	for licID, abertas := range porLicenca {
		cobrancas, err := s.sign.Cobrancas(ctx, abertas[0].AssinaturaID)
		if err != nil {
			slog.Warn("conferindo recorrência", "licenca", licID, "erro", err)
			continue
		}
		for _, cob := range cobrancas {
			if !cob.Pago || cob.Referencia == "" {
				continue
			}
			usada, _ := s.repo.ReferenciaJaUsada(ctx, licID, cob.Referencia)
			if usada {
				continue
			}
			// Casa pela competência do pagamento; sem data, a fatura aberta mais antiga.
			var alvo *modelo.Fatura
			for i := range abertas {
				if abertas[i].Status == "paga" {
					continue
				}
				if cob.Data.IsZero() || (abertas[i].Competencia.Year() == cob.Data.Year() && abertas[i].Competencia.Month() == cob.Data.Month()) {
					alvo = &abertas[i]
					break
				}
			}
			if alvo == nil {
				for i := range abertas {
					if abertas[i].Status != "paga" {
						alvo = &abertas[i]
						break
					}
				}
			}
			if alvo == nil {
				break
			}
			if p, err := s.RegistrarPagamento(ctx, alvo.ID, "cartao", cob.Referencia); err == nil && p.Baixada {
				alvo.Status = "paga"
				baixadas++
			}
		}
	}
	return baixadas, nil
}

// PagamentoConfirmado é o que um webhook da recorrência dispara: a fatura
// aberta mais antiga da assinatura é baixada.
func (s *Servico) PagamentoConfirmado(ctx context.Context, assinaturaID, referencia string) (bool, error) {
	l, err := s.repo.LicencaPorAssinatura(ctx, assinaturaID)
	if err != nil {
		return false, err
	}
	if referencia != "" {
		if usada, _ := s.repo.ReferenciaJaUsada(ctx, l.ID, referencia); usada {
			return false, nil
		}
	}
	if l.PagamentoStatus != modelo.PagamentoAtivo {
		_ = s.repo.RegistrarCartao(ctx, l.ID, modelo.PagamentoAtivo, l.CartaoFinal, l.CartaoBandeira)
	}
	abertas, err := s.repo.FaturasAbertasDaLicenca(ctx, l.ID)
	if err != nil || len(abertas) == 0 {
		return false, err
	}
	p, err := s.RegistrarPagamento(ctx, abertas[0].ID, "webhook", referencia)
	return p.Baixada, err
}

// GerarFaturasDoMes cria a fatura da competência atual para toda licença
// ativa que ainda não a tem (idempotente).
func (s *Servico) GerarFaturasDoMes(ctx context.Context, hoje time.Time) (int, error) {
	lics, err := s.repo.ListarLicencas(ctx, "ativa")
	if err != nil {
		return 0, err
	}
	competencia := primeiroDia(hoje)
	dia := 10
	if v, err := strconv.Atoi(s.repo.Configuracao(ctx, "fatura.dia_vencimento", "10")); err == nil && v >= 1 && v <= 28 {
		dia = v
	}
	criadas := 0
	for _, l := range lics {
		if l.Periodicidade == "anual" {
			continue
		}
		if competencia.Before(primeiroDia(l.Inicio)) || competencia.After(l.Fim) {
			continue
		}
		venc := time.Date(competencia.Year(), competencia.Month(), dia, 0, 0, 0, 0, time.Local)
		if _, inserida, err := s.repo.CriarFatura(ctx, modelo.Fatura{LicencaID: l.ID, Competencia: competencia, Vencimento: venc, ValorCentavos: l.ValorCentavos}); err == nil && inserida {
			criadas++
		}
	}
	return criadas, nil
}

func primeiroDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
}

// -----------------------------------------------------------------------------
// Upgrade, solicitações e "Meu plano"
// -----------------------------------------------------------------------------

// TrocarPlanoDaLicenca aplica um plano novo à licença ativa. O valor novo vale
// nas próximas faturas; a recorrência precisa ser recriada quando o valor
// muda — por isso o upgrade com recorrência ativa gera um novo link de cartão.
func (s *Servico) TrocarPlanoDaLicenca(ctx context.Context, licencaID, novoPlanoID int64) (modelo.Licenca, error) {
	l, err := s.repo.LicencaPorID(ctx, licencaID)
	if err != nil {
		return l, err
	}
	if l.Status != "ativa" {
		return l, errors.New("a licença não está ativa — contrate uma nova pela tela de licenças")
	}
	if l.PlanoID == novoPlanoID {
		return l, errors.New("a licença já está neste plano")
	}
	novo, err := s.repo.PlanoPorID(ctx, novoPlanoID)
	if err != nil {
		return l, err
	}
	if !novo.Ativo || novo.SobConsulta() {
		return l, errors.New("este plano não está disponível para contratação direta")
	}
	valorNovo := novo.PrecoMensalCentavos * int64(max(l.Unidades, 1))
	if err := s.repo.AtualizarPlanoLicenca(ctx, l.ID, novo.ID, novo.UsuariosMax, valorNovo); err != nil {
		return l, err
	}
	if l.ClienteID != nil {
		if c, err := s.repo.ClientePorID(ctx, *l.ClienteID); err == nil {
			_ = s.AtualizarLimiteUsuarios(ctx, c, novo.UsuariosMax)
		}
	}
	return s.repo.LicencaPorID(ctx, l.ID)
}

func (s *Servico) AprovarSolicitacao(ctx context.Context, id int64, decididoPor string) (string, error) {
	sol, err := s.repo.SolicitacaoPorID(ctx, id)
	if err != nil {
		return "", err
	}
	if sol.Status != "pendente" {
		return "", errors.New("esta solicitação já foi decidida")
	}
	cliente, err := s.repo.ClientePorID(ctx, sol.ClienteID)
	if err != nil {
		return "", err
	}
	lic, errLic := s.repo.LicencaVigenteDoCliente(ctx, cliente, time.Now())
	var resposta string
	switch sol.Tipo {
	case "upgrade":
		if errLic != nil {
			return "", errors.New("a empresa não tem licença vigente — contrate pela tela de licenças e recuse esta solicitação")
		}
		if sol.PlanoID == nil {
			return "", errors.New("solicitação sem plano de destino")
		}
		nova, err := s.TrocarPlanoDaLicenca(ctx, lic.ID, *sol.PlanoID)
		if err != nil {
			return "", err
		}
		resposta = "Upgrade aplicado: plano " + nova.PlanoNome + " por " + modelo.FormatarBRL(nova.ValorCentavos) + "/mês a partir da próxima cobrança."
	case "cancelamento":
		if errLic != nil {
			resposta = "Cancelamento registrado — não havia licença vigente."
		} else {
			if err := s.CancelarLicenca(ctx, lic.ID); err != nil {
				return "", err
			}
			resposta = "Plano cancelado. O acesso segue até " + lic.Fim.Format("02/01/2006") + ", fim do período já pago."
		}
	default:
		return "", errors.New("tipo de solicitação desconhecido")
	}
	if err := s.repo.DecidirSolicitacao(ctx, id, "aprovada", resposta, decididoPor); err != nil {
		return "", err
	}
	return resposta, nil
}

func (s *Servico) RecusarSolicitacao(ctx context.Context, id int64, resposta, decididoPor string) error {
	if strings.TrimSpace(resposta) == "" {
		resposta = "Solicitação recusada. Fale com o nosso suporte para entender as opções."
	}
	return s.repo.DecidirSolicitacao(ctx, id, "recusada", resposta, decididoPor)
}

// PlanoDaEmpresa é o que o app do cliente mostra em "Meu plano".
type PlanoDaEmpresa struct {
	Plano       *PlanoAtual       `json:"plano"`
	Planos      []PlanoDisponivel `json:"planos"`
	Solicitacao *SolicitacaoAtual `json:"solicitacao"`
	Faturas     []FaturaDaEmpresa `json:"faturas"`
	Pagamento   *PagamentoAtual   `json:"pagamento"`
}

type PlanoAtual struct {
	Nome          string `json:"nome"`
	ValorCentavos int64  `json:"valor_centavos"`
	UsuariosMax   int    `json:"usuarios_max"`
	Periodicidade string `json:"periodicidade"`
	Fim           string `json:"fim"`
}

type PagamentoAtual struct {
	Status      string `json:"status"`
	Link        string `json:"link,omitempty"`
	CartaoFinal string `json:"cartao_final,omitempty"`
	Bandeira    string `json:"bandeira,omitempty"`
}

type SolicitacaoAtual struct {
	Tipo      string `json:"tipo"`
	PlanoNome string `json:"plano_nome,omitempty"`
	Status    string `json:"status"`
	Resposta  string `json:"resposta,omitempty"`
	CriadoEm  string `json:"criado_em"`
}

type PlanoDisponivel struct {
	ID            int64    `json:"id"`
	Nome          string   `json:"nome"`
	Descricao     string   `json:"descricao"`
	Faixa         string   `json:"faixa"`
	ValorCentavos int64    `json:"valor_centavos"`
	Recursos      []string `json:"recursos"`
	Atual         bool     `json:"atual"`
}

type FaturaDaEmpresa struct {
	ID            int64   `json:"id"`
	Competencia   string  `json:"competencia"`
	Vencimento    string  `json:"vencimento"`
	ValorCentavos int64   `json:"valor_centavos"`
	Status        string  `json:"status"`
	PagoEm        *string `json:"pago_em"`
}

func (s *Servico) PlanoDaEmpresa(ctx context.Context, slug string) (PlanoDaEmpresa, error) {
	var out PlanoDaEmpresa
	cliente, err := s.repo.ClientePorSlug(ctx, slug)
	if err != nil {
		return out, err
	}
	planoAtualID := int64(0)
	lic, errLic := s.repo.LicencaDoCliente(ctx, cliente)
	if errLic == nil && lic.Status == "ativa" {
		out.Plano = &PlanoAtual{Nome: lic.PlanoNome, ValorCentavos: lic.ValorCentavos, UsuariosMax: lic.UsuariosMax, Periodicidade: lic.Periodicidade, Fim: lic.Fim.Format("2006-01-02")}
		planoAtualID = lic.PlanoID
	}
	if errLic == nil && lic.Recorrente() {
		pg := &PagamentoAtual{Status: lic.PagamentoStatus, CartaoFinal: lic.CartaoFinal, Bandeira: lic.CartaoBandeira}
		if lic.CodigoCheckout != "" && lic.Status != "cancelada" {
			pg.Link = s.URLCheckout(lic.CodigoCheckout)
		}
		out.Pagamento = pg
	}
	planos, err := s.repo.ListarPlanos(ctx, true)
	if err != nil {
		return out, err
	}
	out.Planos = []PlanoDisponivel{}
	for _, p := range planos {
		if p.SobConsulta() {
			continue
		}
		out.Planos = append(out.Planos, PlanoDisponivel{ID: p.ID, Nome: p.Nome, Descricao: p.Descricao, Faixa: p.Faixa(),
			ValorCentavos: p.PrecoMensalCentavos, Recursos: p.Recursos, Atual: p.ID == planoAtualID})
	}
	if sol, err := s.repo.UltimaSolicitacaoDoCliente(ctx, cliente.ID); err == nil {
		out.Solicitacao = &SolicitacaoAtual{Tipo: sol.Tipo, PlanoNome: sol.PlanoNome, Status: sol.Status, Resposta: sol.Resposta, CriadoEm: sol.CriadoEm.Format(time.RFC3339)}
	}
	faturas, err := s.repo.FaturasDoCliente(ctx, cliente, 12)
	if err != nil {
		return out, err
	}
	hoje := time.Now()
	out.Faturas = []FaturaDaEmpresa{}
	for _, f := range faturas {
		fe := FaturaDaEmpresa{ID: f.ID, Competencia: f.Competencia.Format("2006-01"), Vencimento: f.Vencimento.Format("2006-01-02"), ValorCentavos: f.ValorCentavos, Status: statusDaFatura(f, hoje)}
		if f.PagoEm != nil {
			v := f.PagoEm.Format(time.RFC3339)
			fe.PagoEm = &v
		}
		out.Faturas = append(out.Faturas, fe)
	}
	return out, nil
}

func statusDaFatura(f modelo.Fatura, hoje time.Time) string {
	if f.Status == "pendente" && f.Vencimento.Before(time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 0, 0, 0, 0, f.Vencimento.Location())) {
		return "vencida"
	}
	return f.Status
}

// SolicitarPlano registra o pedido vindo do app (upgrade/cancelamento).
func (s *Servico) SolicitarPlano(ctx context.Context, slug, tipo string, planoID *int64, mensagem string) error {
	cliente, err := s.repo.ClientePorSlug(ctx, slug)
	if err != nil {
		return err
	}
	switch tipo {
	case "upgrade":
		if planoID == nil {
			return errors.New("escolha o plano desejado")
		}
		p, err := s.repo.PlanoPorID(ctx, *planoID)
		if err != nil || !p.Ativo || p.SobConsulta() {
			return errors.New("este plano não está disponível — fale com o nosso suporte")
		}
		if lic, err := s.repo.LicencaVigenteDoCliente(ctx, cliente, time.Now()); err == nil && lic.PlanoID == *planoID {
			return errors.New("este já é o seu plano atual")
		}
	case "cancelamento":
		planoID = nil
	default:
		return errors.New("tipo de solicitação inválido")
	}
	if len(mensagem) > 500 {
		mensagem = mensagem[:500]
	}
	_, err = s.repo.CriarSolicitacao(ctx, modelo.PlanoSolicitacao{ClienteID: cliente.ID, Tipo: tipo, PlanoID: planoID, Mensagem: mensagem})
	if err != nil && strings.Contains(err.Error(), "plano_solicitacoes_pendente_uk") {
		return ErrSolicitacaoPendente
	}
	return err
}

// ErrSolicitacaoPendente: já existe um pedido aguardando decisão.
var ErrSolicitacaoPendente = errors.New("já existe uma solicitação pendente para esta empresa")
