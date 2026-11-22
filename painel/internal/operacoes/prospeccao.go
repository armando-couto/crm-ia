package operacoes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // o fuso da cota não pode depender do sistema onde o painel roda

	"github.com/armando-couto/crm-ia/painel/internal/basepublica"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
)

// Prospecção: cota mensal de leads por plano e extração de empresas da base
// pública. Contrato em docs/PROSPECCAO.md (seção 4) e docs/PROSPECCAO-ADENDO.md.

// Limites de um pedido de extração.
const (
	MaxCNPJsPorExtracao = 500
	maxPedidoID         = 64
	// CotaSemLicencaPadrao vale para cliente sem licença alguma quando a
	// configuração prospeccao.cota_sem_licenca não existe ou é inválida.
	CotaSemLicencaPadrao = 300
	chaveCotaSemLicenca  = "prospeccao.cota_sem_licenca"
)

// fusoCota: o mês da cota vira à meia-noite de Brasília, em qualquer servidor.
var fusoCota = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("-03", -3*60*60)
	}
	return l
}()

// Competencia devolve o primeiro dia do mês de `agora` em America/Sao_Paulo.
func Competencia(agora time.Time) time.Time {
	l := agora.In(fusoCota)
	return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, fusoCota)
}

// DiaDaCota devolve a data de `agora` em America/Sao_Paulo (limites diários).
func DiaDaCota(agora time.Time) time.Time {
	l := agora.In(fusoCota)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, fusoCota)
}

// Cota é o saldo mensal de leads de prospecção de um cliente.
type Cota struct {
	Mes      string `json:"mes"` // "2026-11"
	Limite   int    `json:"limite"`
	Usado    int    `json:"usado"`
	Restante int    `json:"restante"`
}

// ErroCota: não há saldo para entregar o lote inteiro. Nada foi entregue.
type ErroCota struct{ Cota Cota }

func (e *ErroCota) Error() string { return "cota mensal de prospecção esgotada" }

// ErrPedidoInvalido: o pedido de extração não passa na validação.
var ErrPedidoInvalido = errors.New("pedido inválido")

// LimiteProspeccao devolve a cota mensal do cliente e o nome do plano de onde
// ela veio. Vale o plano da licença vigente; na falta, o da licença mais
// recente ativa ou vencida (a carência entre o vencimento e a suspensão não
// derruba a cota); só sem licença alguma vale a configuração
// prospeccao.cota_sem_licenca. Licença de grupo: cada CNPJ tem a cota inteira.
func (s *Servico) LimiteProspeccao(ctx context.Context, c modelo.Cliente) (limite int, plano string, err error) {
	lic, err := s.repo.LicencaVigenteDoCliente(ctx, c, DiaDaCota(time.Now()))
	if errors.Is(err, repo.ErrNaoEncontrado) {
		lic, err = s.repo.LicencaDoCliente(ctx, c)
	}
	if errors.Is(err, repo.ErrNaoEncontrado) {
		n, errConv := strconv.Atoi(strings.TrimSpace(s.repo.Configuracao(ctx, chaveCotaSemLicenca, strconv.Itoa(CotaSemLicencaPadrao))))
		if errConv != nil || n < 0 {
			n = CotaSemLicencaPadrao
		}
		return n, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	p, err := s.repo.PlanoPorID(ctx, lic.PlanoID)
	if err != nil {
		return 0, "", err
	}
	return p.ProspeccaoLeadsMes, p.Nome, nil
}

// CotaProspeccao devolve o saldo do mês corrente.
func (s *Servico) CotaProspeccao(ctx context.Context, c modelo.Cliente) (Cota, error) {
	limite, _, err := s.LimiteProspeccao(ctx, c)
	if err != nil {
		return Cota{}, err
	}
	return s.cotaCom(ctx, c.ID, limite, Competencia(time.Now()))
}

func (s *Servico) cotaCom(ctx context.Context, clienteID int64, limite int, competencia time.Time) (Cota, error) {
	usado, _, err := s.repo.UsoProspeccao(ctx, clienteID, competencia)
	if err != nil {
		return Cota{}, err
	}
	return Cota{Mes: competencia.Format("2006-01"), Limite: limite, Usado: usado, Restante: max(0, limite-usado)}, nil
}

// NormalizarCNPJ deixa só [0-9A-Z] em maiúsculas. ok = tem a forma de um CNPJ
// (12 alfanuméricos e 2 dígitos); os dígitos verificadores não são conferidos
// aqui, quem responde se a empresa existe é a base.
func NormalizarCNPJ(v string) (cnpj string, ok bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(v) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	cnpj = b.String()
	if len(cnpj) != 14 || cnpj[12] < '0' || cnpj[12] > '9' || cnpj[13] < '0' || cnpj[13] > '9' {
		return cnpj, false
	}
	return cnpj, true
}

// HashDoLote: sha256 (hex) dos CNPJs normalizados, sem repetição, ordenados e
// unidos por vírgula. Devolve também a lista nessa forma.
func HashDoLote(cnpjs []string) (lote string, normalizados []string, err error) {
	vistos := map[string]bool{}
	for _, bruto := range cnpjs {
		c, ok := NormalizarCNPJ(bruto)
		if !ok {
			return "", nil, fmt.Errorf("%w: CNPJ inválido: %q", ErrPedidoInvalido, bruto)
		}
		if !vistos[c] {
			vistos[c] = true
			normalizados = append(normalizados, c)
		}
	}
	sort.Strings(normalizados)
	soma := sha256.Sum256([]byte(strings.Join(normalizados, ",")))
	return hex.EncodeToString(soma[:]), normalizados, nil
}

// Extracao é o que /extracoes entrega ao ambiente do cliente.
type Extracao struct {
	Itens          []json.RawMessage `json:"itens"`
	NaoEncontrados []string          `json:"nao_encontrados"`
	Cota           Cota              `json:"cota"`
}

// ExtrairEmpresas entrega as fichas completas de um lote e desconta da cota a
// quantidade devolvida pela base.
//
//  1. lote = hash dos CNPJs. Se (cliente, pedido, lote) já foi cobrado, as
//     fichas são pedidas de novo e devolvidas sem descontar, em qualquer mês
//     e em qualquer versão da base (o app pode ter perdido a resposta).
//  2. Senão: pede à base; q = fichas devolvidas; grava o lote e desconta q
//     numa transação só. Sem saldo para q, nada é gravado nem entregue
//     (*ErroCota). Conflito na gravação (chamada igual em paralelo) cai no
//     caso 1.
//  3. q = 0 não grava nada.
func (s *Servico) ExtrairEmpresas(ctx context.Context, c modelo.Cliente, pedidoID string, cnpjs []string, incluirPF bool) (Extracao, error) {
	var out Extracao
	pedidoID = strings.TrimSpace(pedidoID)
	switch {
	case pedidoID == "" || len(pedidoID) > maxPedidoID:
		return out, fmt.Errorf("%w: informe pedido_id (até %d caracteres)", ErrPedidoInvalido, maxPedidoID)
	case len(cnpjs) == 0:
		return out, fmt.Errorf("%w: informe ao menos um CNPJ", ErrPedidoInvalido)
	case len(cnpjs) > MaxCNPJsPorExtracao:
		return out, fmt.Errorf("%w: no máximo %d CNPJs por chamada", ErrPedidoInvalido, MaxCNPJsPorExtracao)
	}
	lote, normalizados, err := HashDoLote(cnpjs)
	if err != nil {
		return out, err
	}
	if !s.BasePublica.Configurado() {
		return out, basepublica.ErrNaoConfigurado
	}
	limite, _, err := s.LimiteProspeccao(ctx, c)
	if err != nil {
		return out, err
	}
	competencia := Competencia(time.Now())

	jaCobrado, err := s.repo.ExtracaoRegistrada(ctx, c.ID, pedidoID, lote)
	if err != nil {
		return out, err
	}
	if !jaCobrado {
		// Sem saldo nenhum, nem vale incomodar a base.
		cota, err := s.cotaCom(ctx, c.ID, limite, competencia)
		if err != nil {
			return out, err
		}
		if cota.Restante == 0 {
			return out, &ErroCota{Cota: cota}
		}
	}

	det, err := s.BasePublica.Detalhes(ctx, normalizados, incluirPF)
	if err != nil {
		return out, err
	}
	q := len(det.Itens)
	if q > len(normalizados) {
		return out, fmt.Errorf("%w: a base devolveu mais fichas do que o pedido", basepublica.ErrIndisponivel)
	}
	if !jaCobrado && q > 0 {
		res, err := s.repo.RegistrarExtracao(ctx, c.ID, pedidoID, lote, competencia, q, limite)
		if err != nil {
			return out, err
		}
		if res == repo.ExtracaoSemSaldo {
			cota, err := s.cotaCom(ctx, c.ID, limite, competencia)
			if err != nil {
				return out, err
			}
			return out, &ErroCota{Cota: cota}
		}
	}
	out.Itens, out.NaoEncontrados = det.Itens, det.NaoEncontrados
	if out.Cota, err = s.cotaCom(ctx, c.ID, limite, competencia); err != nil {
		return out, err
	}
	return out, nil
}
