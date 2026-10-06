package repo

import (
	"context"
	"strconv"

	"github.com/armando-couto/crm-ia/painel/internal/modelo"
)

// ---------------------------------------------------------------------------
// Grupos
// ---------------------------------------------------------------------------

const colunasGrupo = `g.id, g.nome, g.cnpj_responsavel, g.email_financeiro, g.telefone, g.cobranca_unificada, g.observacoes, g.ativo,
	g.cep, g.logradouro, g.numero, g.bairro, g.municipio, g.uf, g.pagador_id, g.criado_em,
	(SELECT count(*) FROM clientes c WHERE c.grupo_id=g.id AND c.status<>'cancelado'),
	(SELECT count(*) FROM clientes c WHERE c.grupo_id=g.id AND c.status='ativo')`

func lerGrupo(s interface{ Scan(...any) error }) (modelo.Grupo, error) {
	var g modelo.Grupo
	err := s.Scan(&g.ID, &g.Nome, &g.CNPJResponsavel, &g.EmailFinanceiro, &g.Telefone, &g.CobrancaUnificada, &g.Observacoes, &g.Ativo,
		&g.CEP, &g.Logradouro, &g.Numero, &g.Bairro, &g.Municipio, &g.UF, &g.PagadorID, &g.CriadoEm, &g.TotalClientes, &g.ClientesAtivos)
	return g, err
}

func (r *Repo) ListarGrupos(ctx context.Context) ([]modelo.Grupo, error) {
	linhas, err := r.pool.Query(ctx, `SELECT `+colunasGrupo+` FROM grupos g ORDER BY g.nome`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Grupo
	for linhas.Next() {
		g, err := lerGrupo(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, g)
	}
	return lista, linhas.Err()
}

func (r *Repo) GrupoPorID(ctx context.Context, id int64) (modelo.Grupo, error) {
	g, err := lerGrupo(r.pool.QueryRow(ctx, `SELECT `+colunasGrupo+` FROM grupos g WHERE g.id=$1`, id))
	return g, traduzir(err)
}

func (r *Repo) SalvarGrupo(ctx context.Context, g modelo.Grupo) (int64, error) {
	if g.ID == 0 {
		var id int64
		err := r.pool.QueryRow(ctx, `INSERT INTO grupos (nome, cnpj_responsavel, email_financeiro, telefone, cobranca_unificada, observacoes,
			cep, logradouro, numero, bairro, municipio, uf) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
			g.Nome, g.CNPJResponsavel, g.EmailFinanceiro, g.Telefone, g.CobrancaUnificada, g.Observacoes,
			g.CEP, g.Logradouro, g.Numero, g.Bairro, g.Municipio, g.UF).Scan(&id)
		return id, err
	}
	_, err := r.pool.Exec(ctx, `UPDATE grupos SET nome=$2, cnpj_responsavel=$3, email_financeiro=$4, telefone=$5, cobranca_unificada=$6, observacoes=$7,
		ativo=$8, cep=$9, logradouro=$10, numero=$11, bairro=$12, municipio=$13, uf=$14, atualizado_em=now() WHERE id=$1`,
		g.ID, g.Nome, g.CNPJResponsavel, g.EmailFinanceiro, g.Telefone, g.CobrancaUnificada, g.Observacoes, g.Ativo,
		g.CEP, g.Logradouro, g.Numero, g.Bairro, g.Municipio, g.UF)
	return g.ID, err
}

func (r *Repo) DefinirPagadorGrupo(ctx context.Context, id int64, pagadorID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE grupos SET pagador_id=$2 WHERE id=$1`, id, pagadorID)
	return err
}

// ---------------------------------------------------------------------------
// Clientes
// ---------------------------------------------------------------------------

const colunasCliente = `c.id, c.grupo_id, g.nome, c.razao_social, c.nome_fantasia, c.cnpj, c.slug, c.segmento, c.cidade, c.uf, c.telefone,
	c.cep, c.logradouro, c.numero, c.bairro, c.admin_nome, c.admin_email, c.plano_id, coalesce(p.nome,''), c.usuarios_contratados,
	c.cobranca_propria, c.pagador_id, c.modelo_crm, c.status, c.versao, c.auto_atualizar, c.cor_primaria, c.logo_url, c.url,
	c.provisionado_em, c.ultimo_erro, c.suspenso_por_inadimplencia, c.criado_em`
const deCliente = ` FROM clientes c JOIN grupos g ON g.id=c.grupo_id LEFT JOIN planos p ON p.id=c.plano_id `

func lerCliente(s interface{ Scan(...any) error }) (modelo.Cliente, error) {
	var c modelo.Cliente
	err := s.Scan(&c.ID, &c.GrupoID, &c.GrupoNome, &c.RazaoSocial, &c.NomeFantasia, &c.CNPJ, &c.Slug, &c.Segmento, &c.Cidade, &c.UF, &c.Telefone,
		&c.CEP, &c.Logradouro, &c.Numero, &c.Bairro, &c.AdminNome, &c.AdminEmail, &c.PlanoID, &c.PlanoNome, &c.UsuariosContratados,
		&c.CobrancaPropria, &c.PagadorID, &c.ModeloCRM, &c.Status, &c.Versao, &c.AutoAtualizar, &c.CorPrimaria, &c.LogoURL, &c.URL,
		&c.ProvisionadoEm, &c.UltimoErro, &c.SuspensoPorInadimplencia, &c.CriadoEm)
	return c, err
}

type FiltroClientes struct {
	GrupoID int64
	Status  string
	Busca   string
}

func (r *Repo) ListarClientes(ctx context.Context, f FiltroClientes) ([]modelo.Cliente, error) {
	q := `SELECT ` + colunasCliente + deCliente + ` WHERE 1=1`
	var args []any
	if f.GrupoID > 0 {
		args = append(args, f.GrupoID)
		q += ` AND c.grupo_id=$` + strconv.Itoa(len(args))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		q += ` AND c.status=$` + strconv.Itoa(len(args))
	}
	if f.Busca != "" {
		args = append(args, "%"+f.Busca+"%")
		n := strconv.Itoa(len(args))
		q += ` AND (c.nome_fantasia ILIKE $` + n + ` OR c.razao_social ILIKE $` + n + ` OR c.cnpj ILIKE $` + n + ` OR c.slug ILIKE $` + n + `)`
	}
	q += ` ORDER BY c.nome_fantasia`
	linhas, err := r.pool.Query(ctx, q, args...)
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

func (r *Repo) ClientePorID(ctx context.Context, id int64) (modelo.Cliente, error) {
	c, err := lerCliente(r.pool.QueryRow(ctx, `SELECT `+colunasCliente+deCliente+` WHERE c.id=$1`, id))
	return c, traduzir(err)
}

func (r *Repo) ClientePorSlug(ctx context.Context, slug string) (modelo.Cliente, error) {
	c, err := lerCliente(r.pool.QueryRow(ctx, `SELECT `+colunasCliente+deCliente+` WHERE c.slug=$1`, slug))
	return c, traduzir(err)
}

func (r *Repo) CriarCliente(ctx context.Context, c modelo.Cliente) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO clientes (grupo_id, razao_social, nome_fantasia, cnpj, slug, segmento, cidade, uf, telefone,
		cep, logradouro, numero, bairro, admin_nome, admin_email, plano_id, usuarios_contratados, cobranca_propria, modelo_crm,
		auto_atualizar, cor_primaria, logo_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22) RETURNING id`,
		c.GrupoID, c.RazaoSocial, c.NomeFantasia, c.CNPJ, c.Slug, c.Segmento, c.Cidade, c.UF, c.Telefone,
		c.CEP, c.Logradouro, c.Numero, c.Bairro, c.AdminNome, c.AdminEmail, c.PlanoID, c.UsuariosContratados, c.CobrancaPropria, c.ModeloCRM,
		c.AutoAtualizar, c.CorPrimaria, c.LogoURL).Scan(&id)
	return id, err
}

func (r *Repo) AtualizarCliente(ctx context.Context, c modelo.Cliente) error {
	_, err := r.pool.Exec(ctx, `UPDATE clientes SET grupo_id=$2, razao_social=$3, nome_fantasia=$4, segmento=$5, cidade=$6, uf=$7, telefone=$8,
		cep=$9, logradouro=$10, numero=$11, bairro=$12, admin_nome=$13, admin_email=$14, plano_id=$15, usuarios_contratados=$16,
		cobranca_propria=$17, modelo_crm=$18, auto_atualizar=$19, cor_primaria=$20, logo_url=$21, atualizado_em=now() WHERE id=$1`,
		c.ID, c.GrupoID, c.RazaoSocial, c.NomeFantasia, c.Segmento, c.Cidade, c.UF, c.Telefone,
		c.CEP, c.Logradouro, c.Numero, c.Bairro, c.AdminNome, c.AdminEmail, c.PlanoID, c.UsuariosContratados,
		c.CobrancaPropria, c.ModeloCRM, c.AutoAtualizar, c.CorPrimaria, c.LogoURL)
	return err
}

func (r *Repo) DefinirPagadorCliente(ctx context.Context, id int64, pagadorID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE clientes SET pagador_id=$2 WHERE id=$1`, id, pagadorID)
	return err
}

// AtualizarStatusCliente troca o status; sair de "suspenso" limpa a marca de inadimplência.
func (r *Repo) AtualizarStatusCliente(ctx context.Context, id int64, status modelo.StatusCliente, erro string) error {
	_, err := r.pool.Exec(ctx, `UPDATE clientes SET status=$2, ultimo_erro=$3, suspenso_por_inadimplencia=false, atualizado_em=now() WHERE id=$1`, id, status, erro)
	return err
}

func (r *Repo) MarcarSuspenso(ctx context.Context, id int64, porInadimplencia bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE clientes SET status='suspenso', ultimo_erro='', suspenso_por_inadimplencia=$2, atualizado_em=now() WHERE id=$1`, id, porInadimplencia)
	return err
}

func (r *Repo) MarcarProvisionado(ctx context.Context, id int64, url, versao string) error {
	_, err := r.pool.Exec(ctx, `UPDATE clientes SET status='ativo', url=$2, versao=$3, provisionado_em=coalesce(provisionado_em, now()), ultimo_erro='', atualizado_em=now() WHERE id=$1`, id, url, versao)
	return err
}

func (r *Repo) AtualizarVersaoCliente(ctx context.Context, id int64, versao string) error {
	_, err := r.pool.Exec(ctx, `UPDATE clientes SET versao=$2, atualizado_em=now() WHERE id=$1`, id, versao)
	return err
}

// ClientesParaAutoAtualizar: ativos que acompanham a versão padrão e não estão nela.
func (r *Repo) ClientesParaAutoAtualizar(ctx context.Context, tagPadrao string) ([]modelo.Cliente, error) {
	linhas, err := r.pool.Query(ctx, `SELECT `+colunasCliente+deCliente+` WHERE c.status='ativo' AND c.auto_atualizar AND c.versao <> $1 ORDER BY c.id`, tagPadrao)
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
// Solicitações de plano
// ---------------------------------------------------------------------------

const colunasSolicitacao = `s.id, s.cliente_id, s.tipo, s.plano_id, s.mensagem, s.status, s.resposta, s.criado_em, s.decidido_em, s.decidido_por,
	c.nome_fantasia, c.slug, coalesce(p.nome,'')`
const deSolicitacao = ` FROM plano_solicitacoes s JOIN clientes c ON c.id=s.cliente_id LEFT JOIN planos p ON p.id=s.plano_id `

func lerSolicitacao(s interface{ Scan(...any) error }) (modelo.PlanoSolicitacao, error) {
	var x modelo.PlanoSolicitacao
	err := s.Scan(&x.ID, &x.ClienteID, &x.Tipo, &x.PlanoID, &x.Mensagem, &x.Status, &x.Resposta, &x.CriadoEm, &x.DecididoEm, &x.DecididoPor,
		&x.ClienteNome, &x.ClienteSlug, &x.PlanoNome)
	return x, err
}

func (r *Repo) CriarSolicitacao(ctx context.Context, s modelo.PlanoSolicitacao) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO plano_solicitacoes (cliente_id, tipo, plano_id, mensagem) VALUES ($1,$2,$3,$4) RETURNING id`,
		s.ClienteID, s.Tipo, s.PlanoID, s.Mensagem).Scan(&id)
	return id, err
}

func (r *Repo) SolicitacaoPorID(ctx context.Context, id int64) (modelo.PlanoSolicitacao, error) {
	s, err := lerSolicitacao(r.pool.QueryRow(ctx, `SELECT `+colunasSolicitacao+deSolicitacao+` WHERE s.id=$1`, id))
	return s, traduzir(err)
}

func (r *Repo) ListarSolicitacoes(ctx context.Context, status string) ([]modelo.PlanoSolicitacao, error) {
	q := `SELECT ` + colunasSolicitacao + deSolicitacao
	var args []any
	if status != "" {
		args = append(args, status)
		q += ` WHERE s.status=$1`
	}
	q += ` ORDER BY (s.status='pendente') DESC, s.criado_em DESC LIMIT 500`
	linhas, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.PlanoSolicitacao
	for linhas.Next() {
		s, err := lerSolicitacao(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, s)
	}
	return lista, linhas.Err()
}

func (r *Repo) UltimaSolicitacaoDoCliente(ctx context.Context, clienteID int64) (modelo.PlanoSolicitacao, error) {
	s, err := lerSolicitacao(r.pool.QueryRow(ctx, `SELECT `+colunasSolicitacao+deSolicitacao+` WHERE s.cliente_id=$1 ORDER BY s.criado_em DESC LIMIT 1`, clienteID))
	return s, traduzir(err)
}

func (r *Repo) DecidirSolicitacao(ctx context.Context, id int64, status, resposta, decididoPor string) error {
	_, err := r.pool.Exec(ctx, `UPDATE plano_solicitacoes SET status=$2, resposta=$3, decidido_em=now(), decidido_por=$4 WHERE id=$1 AND status='pendente'`,
		id, status, resposta, decididoPor)
	return err
}
