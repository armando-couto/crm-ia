// Package repo concentra o acesso ao banco do painel.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/painel/internal/modelo"
)

var ErrNaoEncontrado = errors.New("registro não encontrado")

type Repo struct{ pool *pgxpool.Pool }

func Novo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func traduzir(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNaoEncontrado
	}
	return err
}

// ---------------------------------------------------------------------------
// Usuários
// ---------------------------------------------------------------------------

const colunasUsuario = `id, nome, email, senha_hash, perfil, ativo, ultimo_login, criado_em`

func lerUsuario(s interface{ Scan(...any) error }) (modelo.Usuario, error) {
	var u modelo.Usuario
	err := s.Scan(&u.ID, &u.Nome, &u.Email, &u.SenhaHash, &u.Perfil, &u.Ativo, &u.UltimoLogin, &u.CriadoEm)
	return u, err
}

func (r *Repo) ContarUsuarios(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM usuarios`).Scan(&n)
	return n, err
}

func (r *Repo) UsuarioPorEmail(ctx context.Context, email string) (modelo.Usuario, error) {
	u, err := lerUsuario(r.pool.QueryRow(ctx, `SELECT `+colunasUsuario+` FROM usuarios WHERE lower(email)=lower($1)`, email))
	return u, traduzir(err)
}

func (r *Repo) UsuarioPorID(ctx context.Context, id int64) (modelo.Usuario, error) {
	u, err := lerUsuario(r.pool.QueryRow(ctx, `SELECT `+colunasUsuario+` FROM usuarios WHERE id=$1`, id))
	return u, traduzir(err)
}

func (r *Repo) ListarUsuarios(ctx context.Context) ([]modelo.Usuario, error) {
	linhas, err := r.pool.Query(ctx, `SELECT `+colunasUsuario+` FROM usuarios ORDER BY nome`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Usuario
	for linhas.Next() {
		u, err := lerUsuario(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, u)
	}
	return lista, linhas.Err()
}

func (r *Repo) CriarUsuario(ctx context.Context, u modelo.Usuario) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO usuarios (nome, email, senha_hash, perfil, ativo) VALUES ($1, lower($2), $3, $4, $5) RETURNING id`,
		u.Nome, u.Email, u.SenhaHash, u.Perfil, u.Ativo).Scan(&id)
	return id, err
}

func (r *Repo) AtualizarUsuario(ctx context.Context, u modelo.Usuario) error {
	_, err := r.pool.Exec(ctx, `UPDATE usuarios SET nome=$2, email=lower($3), perfil=$4, ativo=$5, atualizado_em=now() WHERE id=$1`,
		u.ID, u.Nome, u.Email, u.Perfil, u.Ativo)
	return err
}

func (r *Repo) AtualizarSenha(ctx context.Context, id int64, hash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE usuarios SET senha_hash=$2, atualizado_em=now() WHERE id=$1`, id, hash)
	return err
}

func (r *Repo) RegistrarLogin(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE usuarios SET ultimo_login=now() WHERE id=$1`, id)
	return err
}

// ---------------------------------------------------------------------------
// Auditoria, eventos e configurações
// ---------------------------------------------------------------------------

func (r *Repo) Auditar(ctx context.Context, usuarioID *int64, usuario, acao, entidade string, entidadeID *int64, detalhes map[string]any, ip string) {
	b, _ := json.Marshal(detalhes)
	if detalhes == nil {
		b = []byte("{}")
	}
	_, _ = r.pool.Exec(ctx, `INSERT INTO auditoria (usuario_id, usuario, acao, entidade, entidade_id, detalhes, ip) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		usuarioID, usuario, acao, entidade, entidadeID, b, ip)
}

func (r *Repo) ListarAuditoria(ctx context.Context, limite int) ([]modelo.Auditoria, error) {
	linhas, err := r.pool.Query(ctx, `SELECT id, usuario, acao, entidade, entidade_id, detalhes::text, ip, criado_em FROM auditoria ORDER BY criado_em DESC LIMIT $1`, limite)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Auditoria
	for linhas.Next() {
		var a modelo.Auditoria
		if err := linhas.Scan(&a.ID, &a.Usuario, &a.Acao, &a.Entidade, &a.EntidadeID, &a.Detalhes, &a.IP, &a.CriadoEm); err != nil {
			return nil, err
		}
		lista = append(lista, a)
	}
	return lista, linhas.Err()
}

func (r *Repo) RegistrarEvento(ctx context.Context, clienteID int64, tipo, detalhe string) {
	_, _ = r.pool.Exec(ctx, `INSERT INTO eventos_ambiente (cliente_id, tipo, detalhe) VALUES ($1,$2,$3)`, clienteID, tipo, detalhe)
}

func (r *Repo) EventosDoCliente(ctx context.Context, clienteID int64, limite int) ([]modelo.EventoAmbiente, error) {
	linhas, err := r.pool.Query(ctx, `SELECT id, cliente_id, tipo, detalhe, criado_em FROM eventos_ambiente WHERE cliente_id=$1 ORDER BY criado_em DESC LIMIT $2`, clienteID, limite)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.EventoAmbiente
	for linhas.Next() {
		var e modelo.EventoAmbiente
		if err := linhas.Scan(&e.ID, &e.ClienteID, &e.Tipo, &e.Detalhe, &e.CriadoEm); err != nil {
			return nil, err
		}
		lista = append(lista, e)
	}
	return lista, linhas.Err()
}

func (r *Repo) Configuracoes(ctx context.Context) ([]modelo.Configuracao, error) {
	linhas, err := r.pool.Query(ctx, `SELECT chave, valor, descricao FROM configuracoes ORDER BY chave`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []modelo.Configuracao
	for linhas.Next() {
		var c modelo.Configuracao
		if err := linhas.Scan(&c.Chave, &c.Valor, &c.Descricao); err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, linhas.Err()
}

func (r *Repo) Configuracao(ctx context.Context, chave, padrao string) string {
	var v string
	if err := r.pool.QueryRow(ctx, `SELECT valor FROM configuracoes WHERE chave=$1`, chave).Scan(&v); err != nil {
		return padrao
	}
	return v
}

func (r *Repo) SalvarConfiguracao(ctx context.Context, chave, valor string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO configuracoes (chave, valor) VALUES ($1,$2) ON CONFLICT (chave) DO UPDATE SET valor=EXCLUDED.valor, atualizado_em=now()`, chave, valor)
	return err
}

// GuardarEventoCobranca guarda o corpo cru de um webhook.
func (r *Repo) GuardarEventoCobranca(ctx context.Context, origem string, corpo []byte, tratado bool) {
	if !json.Valid(corpo) {
		corpo, _ = json.Marshal(map[string]string{"bruto": string(corpo)})
	}
	_, _ = r.pool.Exec(ctx, `INSERT INTO cobranca_eventos (origem, corpo, tratado) VALUES ($1,$2,$3)`, origem, corpo, tratado)
}

// ---------------------------------------------------------------------------
// Resumo do dashboard
// ---------------------------------------------------------------------------

func (r *Repo) Resumo(ctx context.Context, hoje time.Time, diasAviso int) (modelo.Resumo, error) {
	var res modelo.Resumo
	err := r.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM grupos WHERE ativo),
		  (SELECT count(*) FROM clientes WHERE status='ativo'),
		  (SELECT count(*) FROM clientes WHERE status IN ('pronto','provisionando','erro')),
		  (SELECT count(*) FROM clientes WHERE status='suspenso'),
		  (SELECT count(*) FROM licencas WHERE status='ativa' AND fim BETWEEN $1::date AND ($1::date + $2::int)),
		  (SELECT count(*) FROM licencas WHERE status='ativa' AND cobranca='recorrente' AND pagamento_status='aguardando_cartao'),
		  (SELECT count(*) FROM faturas WHERE status='pendente'),
		  (SELECT count(*) FROM faturas WHERE status='vencida' OR (status='pendente' AND vencimento < $1::date)),
		  (SELECT count(*) FROM leads WHERE status='novo'),
		  (SELECT count(*) FROM plano_solicitacoes WHERE status='pendente'),
		  (SELECT coalesce(sum(CASE WHEN periodicidade='anual' THEN valor_centavos/12 ELSE valor_centavos END),0)
		     FROM licencas WHERE status='ativa' AND $1::date BETWEEN inicio AND fim),
		  (SELECT count(*) FROM clientes c WHERE c.status='ativo' AND c.versao <> ''
		     AND EXISTS (SELECT 1 FROM versoes v WHERE v.padrao AND v.tag <> c.versao))`,
		hoje, diasAviso).Scan(&res.Grupos, &res.ClientesAtivos, &res.ClientesAguardando, &res.ClientesSuspensos,
		&res.LicencasVencendo, &res.LicencasSemCartao, &res.FaturasPendentes, &res.FaturasVencidas, &res.LeadsNovos,
		&res.SolicitacoesPendentes, &res.MRRCentavos, &res.ClientesDesatualizados)
	return res, err
}

func nulo(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
