package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/modelo"
)

// ---------------------------------------------------------------------------
// Planos
// ---------------------------------------------------------------------------

const colunasPlano = `id, codigo, nome, descricao, usuarios_min, usuarios_max, preco_mensal_centavos, preco_anual_centavos, destaque, recursos::text, ordem, ativo`

func lerPlano(s interface{ Scan(...any) error }) (modelo.Plano, error) {
	var p modelo.Plano
	var recursos string
	err := s.Scan(&p.ID, &p.Codigo, &p.Nome, &p.Descricao, &p.UsuariosMin, &p.UsuariosMax, &p.PrecoMensalCentavos, &p.PrecoAnualCentavos, &p.Destaque, &recursos, &p.Ordem, &p.Ativo)
	if err == nil {
		_ = json.Unmarshal([]byte(recursos), &p.Recursos)
	}
	return p, err
}

func (r *Repo) ListarPlanos(ctx context.Context, somenteAtivos bool) ([]modelo.Plano, error) {
	q := `SELECT ` + colunasPlano + ` FROM planos`
	if somenteAtivos {
		q += ` WHERE ativo`
	}
	q += ` ORDER BY ordem, usuarios_min`
	linhas, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Plano
	for linhas.Next() {
		p, err := lerPlano(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, p)
	}
	return lista, linhas.Err()
}

func (r *Repo) PlanoPorID(ctx context.Context, id int64) (modelo.Plano, error) {
	p, err := lerPlano(r.pool.QueryRow(ctx, `SELECT `+colunasPlano+` FROM planos WHERE id=$1`, id))
	return p, traduzir(err)
}

func (r *Repo) PlanoPorCodigo(ctx context.Context, codigo string) (modelo.Plano, error) {
	p, err := lerPlano(r.pool.QueryRow(ctx, `SELECT `+colunasPlano+` FROM planos WHERE codigo=$1`, codigo))
	return p, traduzir(err)
}

func (r *Repo) SalvarPlano(ctx context.Context, p modelo.Plano) (int64, error) {
	if p.Recursos == nil {
		p.Recursos = []string{}
	}
	recursos, _ := json.Marshal(p.Recursos)
	if p.ID == 0 {
		var id int64
		err := r.pool.QueryRow(ctx, `INSERT INTO planos (codigo, nome, descricao, usuarios_min, usuarios_max, preco_mensal_centavos, preco_anual_centavos, destaque, recursos, ordem, ativo)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
			p.Codigo, p.Nome, p.Descricao, p.UsuariosMin, p.UsuariosMax, p.PrecoMensalCentavos, p.PrecoAnualCentavos, p.Destaque, recursos, p.Ordem, p.Ativo).Scan(&id)
		return id, err
	}
	_, err := r.pool.Exec(ctx, `UPDATE planos SET codigo=$2, nome=$3, descricao=$4, usuarios_min=$5, usuarios_max=$6, preco_mensal_centavos=$7,
		preco_anual_centavos=$8, destaque=$9, recursos=$10, ordem=$11, ativo=$12, atualizado_em=now() WHERE id=$1`,
		p.ID, p.Codigo, p.Nome, p.Descricao, p.UsuariosMin, p.UsuariosMax, p.PrecoMensalCentavos, p.PrecoAnualCentavos, p.Destaque, recursos, p.Ordem, p.Ativo)
	return p.ID, err
}

// ---------------------------------------------------------------------------
// Licenças
// ---------------------------------------------------------------------------

const colunasLicenca = `l.id, l.grupo_id, g.nome, l.cliente_id, coalesce(c.nome_fantasia,''), l.plano_id, p.nome, l.usuarios_max, l.periodicidade,
	l.valor_centavos, l.inicio, l.fim, l.status, l.unidades, l.observacoes, l.cobranca, l.assinatura_id, l.assinatura_token,
	coalesce(l.codigo_checkout,''), l.pagamento_status, l.cartao_final, l.cartao_bandeira, l.cartao_em, l.criado_em`
const deLicenca = ` FROM licencas l JOIN grupos g ON g.id=l.grupo_id LEFT JOIN clientes c ON c.id=l.cliente_id JOIN planos p ON p.id=l.plano_id `

func lerLicenca(s interface{ Scan(...any) error }) (modelo.Licenca, error) {
	var l modelo.Licenca
	err := s.Scan(&l.ID, &l.GrupoID, &l.GrupoNome, &l.ClienteID, &l.ClienteNome, &l.PlanoID, &l.PlanoNome, &l.UsuariosMax, &l.Periodicidade,
		&l.ValorCentavos, &l.Inicio, &l.Fim, &l.Status, &l.Unidades, &l.Observacoes, &l.Cobranca, &l.AssinaturaID, &l.AssinaturaToken,
		&l.CodigoCheckout, &l.PagamentoStatus, &l.CartaoFinal, &l.CartaoBandeira, &l.CartaoEm, &l.CriadoEm)
	return l, err
}

func (r *Repo) listarLicencas(ctx context.Context, q string, args ...any) ([]modelo.Licenca, error) {
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Licenca
	for linhas.Next() {
		l, err := lerLicenca(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, l)
	}
	return lista, linhas.Err()
}

func (r *Repo) ListarLicencas(ctx context.Context, status string) ([]modelo.Licenca, error) {
	q := `SELECT ` + colunasLicenca + deLicenca
	var args []any
	if status != "" {
		args = append(args, status)
		q += ` WHERE l.status=$1`
	}
	return r.listarLicencas(ctx, q+` ORDER BY l.fim, g.nome`, args...)
}

func (r *Repo) LicencaPorID(ctx context.Context, id int64) (modelo.Licenca, error) {
	l, err := lerLicenca(r.pool.QueryRow(ctx, `SELECT `+colunasLicenca+deLicenca+` WHERE l.id=$1`, id))
	return l, traduzir(err)
}

func (r *Repo) LicencaPorCheckout(ctx context.Context, codigo string) (modelo.Licenca, error) {
	l, err := lerLicenca(r.pool.QueryRow(ctx, `SELECT `+colunasLicenca+deLicenca+` WHERE l.codigo_checkout=$1`, codigo))
	return l, traduzir(err)
}

func (r *Repo) LicencaPorAssinatura(ctx context.Context, assinaturaID string) (modelo.Licenca, error) {
	l, err := lerLicenca(r.pool.QueryRow(ctx, `SELECT `+colunasLicenca+deLicenca+` WHERE l.assinatura_id=$1 AND l.assinatura_id <> '' ORDER BY l.id DESC LIMIT 1`, assinaturaID))
	return l, traduzir(err)
}

func (r *Repo) LicencasDoGrupo(ctx context.Context, grupoID int64) ([]modelo.Licenca, error) {
	return r.listarLicencas(ctx, `SELECT `+colunasLicenca+deLicenca+` WHERE l.grupo_id=$1 ORDER BY l.fim DESC`, grupoID)
}

// LicencaVigenteDoCliente: a do CNPJ (cobrança própria) ou, na falta, a do grupo.
func (r *Repo) LicencaVigenteDoCliente(ctx context.Context, c modelo.Cliente, hoje time.Time) (modelo.Licenca, error) {
	l, err := lerLicenca(r.pool.QueryRow(ctx, `SELECT `+colunasLicenca+deLicenca+`
		WHERE l.grupo_id=$1 AND l.status='ativa' AND $3::date BETWEEN l.inicio AND l.fim AND (l.cliente_id=$2 OR l.cliente_id IS NULL)
		ORDER BY (l.cliente_id IS NOT NULL) DESC, l.fim DESC LIMIT 1`, c.GrupoID, c.ID, hoje))
	return l, traduzir(err)
}

// LicencaDoCliente: a mais recente que cobre o CNPJ, vigente ou vencida.
func (r *Repo) LicencaDoCliente(ctx context.Context, c modelo.Cliente) (modelo.Licenca, error) {
	l, err := lerLicenca(r.pool.QueryRow(ctx, `SELECT `+colunasLicenca+deLicenca+`
		WHERE l.grupo_id=$1 AND l.status IN ('ativa','vencida') AND (l.cliente_id=$2 OR l.cliente_id IS NULL)
		ORDER BY (l.cliente_id IS NOT NULL) DESC, l.fim DESC LIMIT 1`, c.GrupoID, c.ID))
	return l, traduzir(err)
}

func (r *Repo) CriarLicenca(ctx context.Context, l modelo.Licenca) (int64, error) {
	var id int64
	if l.Unidades < 1 {
		l.Unidades = 1
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO licencas (grupo_id, cliente_id, plano_id, usuarios_max, periodicidade, valor_centavos, inicio, fim, unidades, observacoes,
		cobranca, assinatura_id, assinatura_token, codigo_checkout, pagamento_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		l.GrupoID, l.ClienteID, l.PlanoID, l.UsuariosMax, l.Periodicidade, l.ValorCentavos, l.Inicio, l.Fim, l.Unidades, l.Observacoes,
		l.Cobranca, l.AssinaturaID, l.AssinaturaToken, nulo(l.CodigoCheckout), l.PagamentoStatus).Scan(&id)
	return id, err
}

func (r *Repo) RenovarLicenca(ctx context.Context, id int64, novoFim time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE licencas SET fim=$2, status='ativa', atualizado_em=now() WHERE id=$1`, id, novoFim)
	return err
}

func (r *Repo) CancelarLicenca(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE licencas SET status='cancelada', pagamento_status=CASE WHEN cobranca='recorrente' THEN 'cancelado' ELSE pagamento_status END, atualizado_em=now() WHERE id=$1`, id)
	return err
}

// RegistrarCartao marca a recorrência como ativa (ou recusada) depois do checkout.
func (r *Repo) RegistrarCartao(ctx context.Context, id int64, status, final, bandeira string) error {
	_, err := r.pool.Exec(ctx, `UPDATE licencas SET pagamento_status=$2, cartao_final=$3, cartao_bandeira=$4, cartao_em=CASE WHEN $2='ativo' THEN now() ELSE cartao_em END, atualizado_em=now() WHERE id=$1`,
		id, status, final, bandeira)
	return err
}

// AtualizarPlanoLicenca troca plano/teto/valor (upgrade).
func (r *Repo) AtualizarPlanoLicenca(ctx context.Context, id, planoID int64, usuariosMax int, valorCentavos int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE licencas SET plano_id=$2, usuarios_max=$3, valor_centavos=$4, atualizado_em=now() WHERE id=$1`, id, planoID, usuariosMax, valorCentavos)
	return err
}

// EstenderLicenca empurra o fim (nunca para trás) e devolve a "ativa" se estava vencida.
func (r *Repo) EstenderLicenca(ctx context.Context, id int64, novoFim time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE licencas SET fim=GREATEST(fim, $2::date),
		status=CASE WHEN status='vencida' AND GREATEST(fim, $2::date) >= current_date THEN 'ativa' ELSE status END, atualizado_em=now()
		WHERE id=$1 AND status<>'cancelada'`, id, novoFim)
	return err
}

func (r *Repo) MarcarLicencasVencidas(ctx context.Context, hoje time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE licencas SET status='vencida', atualizado_em=now() WHERE status='ativa' AND fim < $1::date`, hoje)
	return tag.RowsAffected(), err
}

// ClientesSemLicencaVigente: ativos cuja licença venceu há mais de `dias`.
func (r *Repo) ClientesSemLicencaVigente(ctx context.Context, hoje time.Time, dias int) ([]modelo.Cliente, error) {
	linhas, err := r.pool.Query(ctx, `SELECT `+colunasCliente+deCliente+`
		WHERE c.status='ativo' AND NOT EXISTS (
		  SELECT 1 FROM licencas l WHERE l.grupo_id=c.grupo_id AND l.status IN ('ativa','vencida')
		    AND (l.cliente_id=c.id OR l.cliente_id IS NULL) AND l.fim >= ($1::date - $2::int)
		) ORDER BY c.id`, hoje, dias)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Cliente
	for linhas.Next() {
		c, err := lerCliente(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, linhas.Err()
}

// SuspensosPorInadimplenciaDaLicenca: quem volta ao ar quando a licença é paga.
func (r *Repo) SuspensosPorInadimplenciaDaLicenca(ctx context.Context, l modelo.Licenca) ([]modelo.Cliente, error) {
	linhas, err := r.pool.Query(ctx, `SELECT `+colunasCliente+deCliente+`
		WHERE c.status='suspenso' AND c.suspenso_por_inadimplencia AND c.grupo_id=$1 AND ($2::bigint IS NULL OR c.id=$2) ORDER BY c.id`, l.GrupoID, l.ClienteID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Cliente
	for linhas.Next() {
		c, err := lerCliente(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, linhas.Err()
}

// ---------------------------------------------------------------------------
// Faturas
// ---------------------------------------------------------------------------

const colunasFatura = `f.id, f.licenca_id, f.competencia, f.vencimento, f.valor_centavos, f.status, f.forma, f.referencia, f.pago_em, f.criado_em,
	l.grupo_id, l.cliente_id, g.nome, coalesce(c.nome_fantasia,''), p.nome, l.assinatura_id, l.pagamento_status`
const deFatura = ` FROM faturas f JOIN licencas l ON l.id=f.licenca_id JOIN grupos g ON g.id=l.grupo_id LEFT JOIN clientes c ON c.id=l.cliente_id JOIN planos p ON p.id=l.plano_id `

func lerFatura(s interface{ Scan(...any) error }) (modelo.Fatura, error) {
	var f modelo.Fatura
	err := s.Scan(&f.ID, &f.LicencaID, &f.Competencia, &f.Vencimento, &f.ValorCentavos, &f.Status, &f.Forma, &f.Referencia, &f.PagoEm, &f.CriadoEm,
		&f.GrupoID, &f.ClienteID, &f.GrupoNome, &f.ClienteNome, &f.PlanoNome, &f.AssinaturaID, &f.PagamentoStatus)
	return f, err
}

func (r *Repo) listarFaturas(ctx context.Context, q string, args ...any) ([]modelo.Fatura, error) {
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Fatura
	for linhas.Next() {
		f, err := lerFatura(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, f)
	}
	return lista, linhas.Err()
}

func (r *Repo) ListarFaturas(ctx context.Context, status string) ([]modelo.Fatura, error) {
	q := `SELECT ` + colunasFatura + deFatura
	var args []any
	if status != "" {
		args = append(args, status)
		q += ` WHERE f.status=$1`
	}
	return r.listarFaturas(ctx, q+` ORDER BY f.vencimento DESC, f.id DESC LIMIT 1000`, args...)
}

func (r *Repo) FaturaPorID(ctx context.Context, id int64) (modelo.Fatura, error) {
	f, err := lerFatura(r.pool.QueryRow(ctx, `SELECT `+colunasFatura+deFatura+` WHERE f.id=$1`, id))
	return f, traduzir(err)
}

// FaturasAbertasDaLicenca: as em aberto, mais antiga primeiro.
func (r *Repo) FaturasAbertasDaLicenca(ctx context.Context, licencaID int64) ([]modelo.Fatura, error) {
	return r.listarFaturas(ctx, `SELECT `+colunasFatura+deFatura+` WHERE f.licenca_id=$1 AND f.status IN ('pendente','vencida') ORDER BY f.competencia`, licencaID)
}

// FaturasAbertasRecorrentes: o que a conferência varre.
func (r *Repo) FaturasAbertasRecorrentes(ctx context.Context) ([]modelo.Fatura, error) {
	return r.listarFaturas(ctx, `SELECT `+colunasFatura+deFatura+`
		WHERE f.status IN ('pendente','vencida') AND l.cobranca='recorrente' AND l.assinatura_id <> '' AND l.pagamento_status='ativo'
		ORDER BY f.competencia LIMIT 500`)
}

const filtroFaturaDoCliente = ` l.grupo_id=$1 AND (l.cliente_id=$2 OR l.cliente_id IS NULL) `

// FaturasDoCliente: o que o app mostra em "Meu plano".
func (r *Repo) FaturasDoCliente(ctx context.Context, c modelo.Cliente, limite int) ([]modelo.Fatura, error) {
	return r.listarFaturas(ctx, `SELECT `+colunasFatura+deFatura+` WHERE `+filtroFaturaDoCliente+`
		ORDER BY (f.status IN ('pendente','vencida')) DESC, f.competencia DESC, f.id DESC LIMIT $3`, c.GrupoID, c.ID, limite)
}

// CriarFatura é idempotente por (licença, competência).
func (r *Repo) CriarFatura(ctx context.Context, f modelo.Fatura) (int64, bool, error) {
	var id int64
	var inserida bool
	err := r.pool.QueryRow(ctx, `INSERT INTO faturas (licenca_id, competencia, vencimento, valor_centavos) VALUES ($1,$2,$3,$4)
		ON CONFLICT (licenca_id, competencia) DO UPDATE SET vencimento=faturas.vencimento RETURNING id, (xmax = 0)`,
		f.LicencaID, f.Competencia, f.Vencimento, f.ValorCentavos).Scan(&id, &inserida)
	return id, inserida, err
}

// BaixarFatura marca como paga só se ainda estava em aberto; devolve se ESTA
// chamada baixou (duas conferências simultâneas não renovam duas vezes).
func (r *Repo) BaixarFatura(ctx context.Context, id int64, forma, referencia string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE faturas SET status='paga', forma=$2, referencia=$3, pago_em=now() WHERE id=$1 AND status IN ('pendente','vencida')`, id, forma, referencia)
	return tag.RowsAffected() > 0, err
}

func (r *Repo) ReferenciaJaUsada(ctx context.Context, licencaID int64, referencia string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM faturas WHERE licenca_id=$1 AND referencia=$2 AND referencia <> ''`, licencaID, referencia).Scan(&n)
	return n > 0, err
}

func (r *Repo) CancelarFatura(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE faturas SET status='cancelada' WHERE id=$1 AND status<>'paga'`, id)
	return err
}

func (r *Repo) MarcarFaturasVencidas(ctx context.Context, hoje time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE faturas SET status='vencida' WHERE status='pendente' AND vencimento < $1::date`, hoje)
	return tag.RowsAffected(), err
}

// ---------------------------------------------------------------------------
// Versões
// ---------------------------------------------------------------------------

func (r *Repo) ListarVersoes(ctx context.Context) ([]modelo.Versao, error) {
	linhas, err := r.pool.Query(ctx, `SELECT v.id, v.tag, v.descricao, v.changelog, v.estavel, v.padrao, v.publicada_em,
		(SELECT count(*) FROM clientes c WHERE c.versao=v.tag AND c.status IN ('ativo','suspenso')) FROM versoes v ORDER BY v.publicada_em DESC`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Versao
	for linhas.Next() {
		var v modelo.Versao
		if err := linhas.Scan(&v.ID, &v.Tag, &v.Descricao, &v.Changelog, &v.Estavel, &v.Padrao, &v.PublicadaEm, &v.Clientes); err != nil {
			return nil, err
		}
		lista = append(lista, v)
	}
	return lista, linhas.Err()
}

func (r *Repo) VersaoPadrao(ctx context.Context) (modelo.Versao, error) {
	var v modelo.Versao
	err := r.pool.QueryRow(ctx, `SELECT id, tag, descricao, changelog, estavel, padrao, publicada_em FROM versoes WHERE padrao`).
		Scan(&v.ID, &v.Tag, &v.Descricao, &v.Changelog, &v.Estavel, &v.Padrao, &v.PublicadaEm)
	return v, traduzir(err)
}

func (r *Repo) VersaoPorTag(ctx context.Context, tag string) (modelo.Versao, error) {
	var v modelo.Versao
	err := r.pool.QueryRow(ctx, `SELECT id, tag, descricao, changelog, estavel, padrao, publicada_em FROM versoes WHERE tag=$1`, tag).
		Scan(&v.ID, &v.Tag, &v.Descricao, &v.Changelog, &v.Estavel, &v.Padrao, &v.PublicadaEm)
	return v, traduzir(err)
}

func (r *Repo) CriarVersao(ctx context.Context, v modelo.Versao) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO versoes (tag, descricao, changelog, estavel) VALUES ($1,$2,$3,$4) RETURNING id`, v.Tag, v.Descricao, v.Changelog, v.Estavel).Scan(&id)
	return id, err
}

func (r *Repo) DefinirVersaoPadrao(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE versoes SET padrao=false WHERE padrao`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE versoes SET padrao=true WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repo) RemoverVersao(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM versoes WHERE id=$1 AND NOT padrao`, id)
	return err
}

// ---------------------------------------------------------------------------
// Leads
// ---------------------------------------------------------------------------

func (r *Repo) CriarLead(ctx context.Context, l modelo.Lead) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO leads (nome, email, telefone, empresa, segmento, cidade, usuarios, plano_interesse, mensagem, origem)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		l.Nome, l.Email, l.Telefone, l.Empresa, l.Segmento, l.Cidade, l.Usuarios, l.PlanoInteresse, l.Mensagem, l.Origem).Scan(&id)
	return id, err
}

func (r *Repo) ListarLeads(ctx context.Context) ([]modelo.Lead, error) {
	linhas, err := r.pool.Query(ctx, `SELECT id, nome, email, telefone, empresa, segmento, cidade, usuarios, plano_interesse, mensagem, origem, status, criado_em
		FROM leads ORDER BY (status='novo') DESC, criado_em DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Lead
	for linhas.Next() {
		var l modelo.Lead
		if err := linhas.Scan(&l.ID, &l.Nome, &l.Email, &l.Telefone, &l.Empresa, &l.Segmento, &l.Cidade, &l.Usuarios, &l.PlanoInteresse, &l.Mensagem, &l.Origem, &l.Status, &l.CriadoEm); err != nil {
			return nil, err
		}
		lista = append(lista, l)
	}
	return lista, linhas.Err()
}

func (r *Repo) AtualizarStatusLead(ctx context.Context, id int64, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE leads SET status=$2 WHERE id=$1`, id, status)
	return err
}
