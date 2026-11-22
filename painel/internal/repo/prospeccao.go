package repo

import (
	"context"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/modelo"
)

// ---------------------------------------------------------------------------
// Prospecção: uso mensal da base pública e extrações
// ---------------------------------------------------------------------------

const formatoData = "2006-01-02"

// UsoProspeccao devolve o que o cliente já consumiu na competência (dia 1 do
// mês): empresas extraídas e fichas avulsas consultadas. Sem linha = zero.
func (r *Repo) UsoProspeccao(ctx context.Context, clienteID int64, competencia time.Time) (leads, consultas int, err error) {
	err = r.pool.QueryRow(ctx, `SELECT coalesce(sum(leads),0), coalesce(sum(consultas),0) FROM prospeccao_uso WHERE cliente_id=$1 AND competencia=$2::date`,
		clienteID, competencia.Format(formatoData)).Scan(&leads, &consultas)
	return leads, consultas, err
}

// ExtracaoRegistrada diz se este lote deste pedido já foi entregue e cobrado
// (em qualquer mês).
func (r *Repo) ExtracaoRegistrada(ctx context.Context, clienteID int64, pedidoID, lote string) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM prospeccao_extracoes WHERE cliente_id=$1 AND pedido_id=$2 AND lote=$3)`,
		clienteID, pedidoID, lote).Scan(&existe)
	return existe, err
}

// ResultadoExtracao é o desfecho de RegistrarExtracao.
type ResultadoExtracao int

const (
	// ExtracaoDebitada: lote novo, registrado e descontado da cota.
	ExtracaoDebitada ResultadoExtracao = iota
	// ExtracaoRepetida: o lote já tinha sido registrado; nada foi descontado.
	ExtracaoRepetida
	// ExtracaoSemSaldo: o desconto estouraria o limite; nada foi gravado.
	ExtracaoSemSaldo
)

// RegistrarExtracao grava o lote e desconta `quantidade` da cota do mês, tudo
// numa transação. O registro do lote vem primeiro: duas chamadas iguais ao
// mesmo tempo disputam a chave única, a segunda espera a primeira terminar e
// sai como repetida, sem descontar. O desconto é condicional (não passa do
// limite); recusado, a transação inteira é desfeita.
func (r *Repo) RegistrarExtracao(ctx context.Context, clienteID int64, pedidoID, lote string, competencia time.Time, quantidade, limite int) (ResultadoExtracao, error) {
	comp := competencia.Format(formatoData)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ExtracaoSemSaldo, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	marca, err := tx.Exec(ctx, `INSERT INTO prospeccao_extracoes (cliente_id, pedido_id, lote, competencia, quantidade)
		VALUES ($1,$2,$3,$4::date,$5) ON CONFLICT (cliente_id, pedido_id, lote) DO NOTHING`, clienteID, pedidoID, lote, comp, quantidade)
	if err != nil {
		return ExtracaoSemSaldo, err
	}
	if marca.RowsAffected() == 0 {
		return ExtracaoRepetida, nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO prospeccao_uso (cliente_id, competencia) VALUES ($1,$2::date) ON CONFLICT DO NOTHING`, clienteID, comp); err != nil {
		return ExtracaoSemSaldo, err
	}
	marca, err = tx.Exec(ctx, `UPDATE prospeccao_uso SET leads = leads + $3, atualizado_em = now()
		WHERE cliente_id=$1 AND competencia=$2::date AND leads + $3 <= $4`, clienteID, comp, quantidade, limite)
	if err != nil {
		return ExtracaoSemSaldo, err
	}
	if marca.RowsAffected() == 0 {
		return ExtracaoSemSaldo, nil
	}
	return ExtracaoDebitada, tx.Commit(ctx)
}

// ContarConsultaCNPJ soma uma ficha avulsa ao mês e ao dia do cliente. Devolve
// falso, sem somar, quando o dia já chegou a limiteDia.
func (r *Repo) ContarConsultaCNPJ(ctx context.Context, clienteID int64, competencia, dia time.Time, limiteDia int) (bool, error) {
	marca, err := r.pool.Exec(ctx, `INSERT INTO prospeccao_uso AS u (cliente_id, competencia, consultas, consultas_dia, consultas_no_dia)
		VALUES ($1, $2::date, 1, $3::date, 1)
		ON CONFLICT (cliente_id, competencia) DO UPDATE SET
		  consultas = u.consultas + 1,
		  consultas_no_dia = CASE WHEN u.consultas_dia = $3::date THEN u.consultas_no_dia + 1 ELSE 1 END,
		  consultas_dia = $3::date,
		  atualizado_em = now()
		WHERE u.consultas_dia IS DISTINCT FROM $3::date OR u.consultas_no_dia < $4`,
		clienteID, competencia.Format(formatoData), dia.Format(formatoData), limiteDia)
	if err != nil {
		return false, err
	}
	return marca.RowsAffected() > 0, nil
}

// UsoProspeccaoPorCliente lista quem usou a base na competência, do maior
// consumo para o menor.
func (r *Repo) UsoProspeccaoPorCliente(ctx context.Context, competencia time.Time) ([]modelo.UsoProspeccao, error) {
	linhas, err := r.pool.Query(ctx, `SELECT u.cliente_id, c.nome_fantasia, c.slug, u.leads, u.consultas,
		  (SELECT max(e.criado_em) FROM prospeccao_extracoes e WHERE e.cliente_id = u.cliente_id AND e.competencia = u.competencia)
		FROM prospeccao_uso u JOIN clientes c ON c.id = u.cliente_id
		WHERE u.competencia = $1::date AND (u.leads > 0 OR u.consultas > 0)
		ORDER BY u.leads DESC, c.nome_fantasia`, competencia.Format(formatoData))
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.UsoProspeccao
	for linhas.Next() {
		var u modelo.UsoProspeccao
		if err := linhas.Scan(&u.ClienteID, &u.ClienteNome, &u.ClienteSlug, &u.Leads, &u.Consultas, &u.UltimaExtracao); err != nil {
			return nil, err
		}
		lista = append(lista, u)
	}
	return lista, linhas.Err()
}
