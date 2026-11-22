package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Origens e status da lista de prospecção.
const (
	ListaOrigemBase  = "base"
	ListaOrigemCNPJs = "cnpjs"

	ListaGerando = "gerando"
	ListaPronta  = "pronta"
	ListaFalhou  = "falhou"

	// MaxQuantidadeLista é o teto de empresas por lista.
	MaxQuantidadeLista = 20000
)

// ProspContadores resume os itens de uma lista.
type ProspContadores struct {
	Total        int `json:"total"`
	Novo         int `json:"novo"`
	Enriquecido  int `json:"enriquecido"`
	SemResultado int `json:"sem_resultado"`
	Erro         int `json:"erro"`
	Enviado      int `json:"enviado"`
	Descartado   int `json:"descartado"`
	Duplicado    int `json:"duplicado"`
	Bloqueado    int `json:"bloqueado"`
	ComCelular   int `json:"com_celular"`
	ComEmail     int `json:"com_email"`
	ComDecisor   int `json:"com_decisor"`
	ComWhatsApp  int `json:"com_whatsapp"`
	PessoaFisica int `json:"pessoa_fisica"`
	// CustoCentavos soma o custo de enriquecimento dos itens que ainda estão
	// na lista (o uso mensal é contabilizado em prospeccao_jobs).
	CustoCentavos int `json:"custo_centavos"`
}

// ProspLista é a lista de prospecção: CNPJs que ainda não são registros do CRM.
type ProspLista struct {
	ID               int64           `json:"id"`
	Nome             string          `json:"nome"`
	Descricao        string          `json:"descricao"`
	Origem           string          `json:"origem"`
	Filtros          ProspFiltros    `json:"filtros"`
	Ordem            string          `json:"ordem"`
	QuantidadePedida int             `json:"quantidade_pedida"`
	ExcluirNoCRM     bool            `json:"excluir_no_crm"`
	ExcluirEmListas  bool            `json:"excluir_em_listas"`
	Status           string          `json:"status"`
	Mensagem         string          `json:"mensagem"`
	PedidoID         string          `json:"-"` // UUID da lista nas extrações da plataforma
	VersaoBase       string          `json:"versao_base"`
	IncluirPF        bool            `json:"incluir_pf"`
	SegmentoID       *int64          `json:"segmento_id"`
	PlaybookID       *int64          `json:"playbook_id"`
	PipelineID       *int64          `json:"pipeline_id"`
	StageID          *int64          `json:"stage_id"`
	CriadoPor        *int64          `json:"criado_por"`
	CriadoPorNome    string          `json:"criado_por_nome"`
	CriadoEm         time.Time       `json:"criado_em"`
	AtualizadoEm     time.Time       `json:"atualizado_em"`
	Contadores       ProspContadores `json:"contadores"`
}

// Validar confere o que o app decide sozinho ao criar a lista (a cota é
// conferida pelo job, na plataforma). Preenche os padrões.
func (l *ProspLista) Validar() error {
	l.Nome = strings.TrimSpace(l.Nome)
	l.Descricao = strings.TrimSpace(l.Descricao)
	if l.Nome == "" {
		return fmt.Errorf("informe o nome da lista")
	}
	if len([]rune(l.Nome)) > 150 {
		return fmt.Errorf("o nome da lista é longo demais")
	}
	if l.Origem == "" {
		l.Origem = ListaOrigemBase
	}
	if l.Origem != ListaOrigemBase && l.Origem != ListaOrigemCNPJs {
		return fmt.Errorf("origem inválida")
	}
	if l.Ordem == "" {
		l.Ordem = OrdemRelevancia
	}
	if !ProspOrdemValida(l.Ordem) {
		return fmt.Errorf("ordenação inválida")
	}
	l.Filtros.Normalizar()
	if l.Origem == ListaOrigemCNPJs {
		if len(l.Filtros.CNPJs) == 0 {
			return fmt.Errorf("informe os CNPJs da lista")
		}
		l.QuantidadePedida = len(l.Filtros.CNPJs)
	}
	if err := l.Filtros.Validar(); err != nil {
		return err
	}
	if l.QuantidadePedida < 1 || l.QuantidadePedida > MaxQuantidadeLista {
		return fmt.Errorf("a quantidade deve ficar entre 1 e %d", MaxQuantidadeLista)
	}
	l.IncluirPF = l.Filtros.IncluirPF
	return nil
}

const prospContadoresSQL = `
	SELECT COUNT(*),
	       COUNT(*) FILTER (WHERE i.estado = 'novo'),
	       COUNT(*) FILTER (WHERE i.estado = 'enriquecido'),
	       COUNT(*) FILTER (WHERE i.estado = 'sem_resultado'),
	       COUNT(*) FILTER (WHERE i.estado = 'erro'),
	       COUNT(*) FILTER (WHERE i.estado = 'enviado'),
	       COUNT(*) FILTER (WHERE i.estado = 'descartado'),
	       COUNT(*) FILTER (WHERE i.estado = 'duplicado'),
	       COUNT(*) FILTER (WHERE i.estado = 'bloqueado'),
	       COUNT(*) FILTER (WHERE i.tem_celular),
	       COUNT(*) FILTER (WHERE i.tem_email),
	       COUNT(*) FILTER (WHERE i.tem_decisor),
	       COUNT(*) FILTER (WHERE i.tem_whatsapp),
	       COUNT(*) FILTER (WHERE i.pessoa_fisica),
	       COALESCE(SUM(i.custo_centavos), 0)
	FROM prospeccao_itens i WHERE i.lista_id = `

const prospListaSelect = `
	SELECT l.id, l.nome, COALESCE(l.descricao,''), l.origem, l.filtros, l.ordem, l.quantidade_pedida,
	       l.excluir_no_crm, l.excluir_em_listas, l.status, COALESCE(l.mensagem,''), l.pedido_id,
	       COALESCE(l.versao_base,''), l.incluir_pf, l.segmento_id, l.playbook_id, l.pipeline_id, l.stage_id,
	       l.criado_por, COALESCE(u.name,''), l.criado_em, l.atualizado_em, ct.*
	FROM prospeccao_listas l
	LEFT JOIN users u ON u.id = l.criado_por
	LEFT JOIN LATERAL (` + prospContadoresSQL + `l.id) ct ON TRUE
	WHERE `

func scanProspLista(row interface{ Scan(...any) error }) (*ProspLista, error) {
	var l ProspLista
	var filtros []byte
	c := &l.Contadores
	if err := row.Scan(&l.ID, &l.Nome, &l.Descricao, &l.Origem, &filtros, &l.Ordem, &l.QuantidadePedida,
		&l.ExcluirNoCRM, &l.ExcluirEmListas, &l.Status, &l.Mensagem, &l.PedidoID,
		&l.VersaoBase, &l.IncluirPF, &l.SegmentoID, &l.PlaybookID, &l.PipelineID, &l.StageID,
		&l.CriadoPor, &l.CriadoPorNome, &l.CriadoEm, &l.AtualizadoEm,
		&c.Total, &c.Novo, &c.Enriquecido, &c.SemResultado, &c.Erro, &c.Enviado, &c.Descartado,
		&c.Duplicado, &c.Bloqueado, &c.ComCelular, &c.ComEmail, &c.ComDecisor, &c.ComWhatsApp,
		&c.PessoaFisica, &c.CustoCentavos); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(filtros, &l.Filtros)
	return &l, nil
}

// ProspListaFilter filtra a listagem de listas.
type ProspListaFilter struct {
	Q      string
	Status string
	Pagination
}

// ListProspListas devolve a página pedida e o total, da mais recente para a mais antiga.
func ListProspListas(db *sql.DB, f ProspListaFilter) ([]ProspLista, int, error) {
	f.Normalize()
	where := []string{"TRUE"}
	args := []any{}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, likeContem(q))
		where = append(where, fmt.Sprintf(`(l.nome ILIKE $%d OR COALESCE(l.descricao,'') ILIKE $%d)`, len(args), len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf(`l.status = $%d`, len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prospeccao_listas l WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(prospListaSelect+`%s ORDER BY l.criado_em DESC, l.id DESC LIMIT $%d OFFSET $%d`,
		cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	listas := []ProspLista{}
	for rows.Next() {
		l, err := scanProspLista(rows)
		if err != nil {
			return nil, 0, err
		}
		listas = append(listas, *l)
	}
	return listas, total, rows.Err()
}

// ProspListaByID devolve a lista com os contadores; sql.ErrNoRows quando não existe.
func ProspListaByID(db Executor, id int64) (*ProspLista, error) {
	return scanProspLista(db.QueryRow(prospListaSelect+`l.id = $1`, id))
}

// ProspContadoresDaLista recalcula só os contadores (para o progresso).
func ProspContadoresDaLista(db Executor, listaID int64) (ProspContadores, error) {
	var c ProspContadores
	err := db.QueryRow(prospContadoresSQL+`$1`, listaID).Scan(
		&c.Total, &c.Novo, &c.Enriquecido, &c.SemResultado, &c.Erro, &c.Enviado, &c.Descartado,
		&c.Duplicado, &c.Bloqueado, &c.ComCelular, &c.ComEmail, &c.ComDecisor, &c.ComWhatsApp,
		&c.PessoaFisica, &c.CustoCentavos)
	return c, err
}

// CreateProspLista grava a lista (status "gerando", pedido_id novo se vazio).
func CreateProspLista(db Executor, l *ProspLista) error {
	if l.PedidoID == "" {
		l.PedidoID = NovoPedidoID()
	}
	if l.Status == "" {
		l.Status = ListaGerando
	}
	if l.Origem == "" {
		l.Origem = ListaOrigemBase
	}
	if l.Ordem == "" {
		l.Ordem = OrdemRelevancia
	}
	return db.QueryRow(`
		INSERT INTO prospeccao_listas (nome, descricao, origem, filtros, ordem, quantidade_pedida,
		    excluir_no_crm, excluir_em_listas, status, mensagem, pedido_id, versao_base, incluir_pf,
		    segmento_id, playbook_id, pipeline_id, stage_id, criado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, criado_em, atualizado_em`,
		l.Nome, l.Descricao, l.Origem, jsonOu(l.Filtros, "{}"), l.Ordem, l.QuantidadePedida,
		l.ExcluirNoCRM, l.ExcluirEmListas, l.Status, l.Mensagem, l.PedidoID, l.VersaoBase, l.IncluirPF,
		l.SegmentoID, l.PlaybookID, l.PipelineID, l.StageID, l.CriadoPor,
	).Scan(&l.ID, &l.CriadoEm, &l.AtualizadoEm)
}

// CreateProspListaComJob grava a lista e o job que vai gerá-la na mesma
// transação: ou nascem os dois, ou nenhum. job.ListaID é preenchido aqui.
func CreateProspListaComJob(db *sql.DB, l *ProspLista, job *ProspJob) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := CreateProspLista(tx, l); err != nil {
		return err
	}
	job.ListaID = &l.ID
	if job.Tipo == "" {
		job.Tipo = JobGerarLista
	}
	if job.CriadoPor == nil {
		job.CriadoPor = l.CriadoPor
	}
	if err := CreateProspJob(tx, job); err != nil {
		return err
	}
	return tx.Commit()
}

// OpcionalID distingue "não mexer" de "gravar nulo" nas atualizações parciais.
type OpcionalID struct {
	Definir bool
	ID      *int64
}

// DefinirID monta um OpcionalID que grava o valor (0 ou negativo vira nulo).
func DefinirID(id int64) OpcionalID {
	if id <= 0 {
		return OpcionalID{Definir: true}
	}
	return OpcionalID{Definir: true, ID: &id}
}

// DefinirIDPtr monta um OpcionalID a partir de um ponteiro (nil grava nulo).
func DefinirIDPtr(id *int64) OpcionalID { return OpcionalID{Definir: true, ID: id} }

// ProspListaPatch é a atualização parcial de PUT /prospeccao/listas/{id}.
type ProspListaPatch struct {
	Nome       *string
	Descricao  *string
	PlaybookID OpcionalID
	PipelineID OpcionalID
	StageID    OpcionalID
}

// MudaDestino diz se o patch mexe em pipeline_id ou stage_id (proibido com job ativo).
func (p ProspListaPatch) MudaDestino() bool { return p.PipelineID.Definir || p.StageID.Definir }

// UpdateProspLista aplica só os campos informados. sql.ErrNoRows quando não existe.
func UpdateProspLista(db Executor, id int64, p ProspListaPatch) error {
	sets := []string{"atualizado_em = NOW()"}
	args := []any{id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Nome != nil {
		add("nome", cortarTexto(*p.Nome, 150))
	}
	if p.Descricao != nil {
		add("descricao", strings.TrimSpace(*p.Descricao))
	}
	if p.PlaybookID.Definir {
		add("playbook_id", p.PlaybookID.ID)
	}
	if p.PipelineID.Definir {
		add("pipeline_id", p.PipelineID.ID)
	}
	if p.StageID.Definir {
		add("stage_id", p.StageID.ID)
	}
	res, err := db.Exec(`UPDATE prospeccao_listas SET `+strings.Join(sets, ", ")+` WHERE id = $1`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetProspListaStatus muda status e mensagem; versaoBase vazia mantém a gravada.
func SetProspListaStatus(db Executor, id int64, status, mensagem, versaoBase string) error {
	_, err := db.Exec(`
		UPDATE prospeccao_listas
		SET status = $2, mensagem = $3, versao_base = COALESCE(NULLIF($4, ''), versao_base), atualizado_em = NOW()
		WHERE id = $1`, id, status, mensagem, versaoBase)
	return err
}

// DeleteProspLista apaga a lista e os itens (cascata). Leads já criados ficam
// (lista_id e item_id viram nulos) e os jobs também (lista_id nulo), para o
// custo do enriquecimento continuar contando no mês. Com job ativo devolve
// ErrJobAtivo e não apaga nada.
func DeleteProspLista(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existe bool
	if err := tx.QueryRow(`SELECT TRUE FROM prospeccao_listas WHERE id = $1 FOR UPDATE`, id).Scan(&existe); err != nil {
		return err
	}
	var ativo bool
	if err := tx.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM prospeccao_jobs WHERE lista_id = $1 AND status IN ('pendente','rodando'))`,
		id).Scan(&ativo); err != nil {
		return err
	}
	if ativo {
		return ErrJobAtivo
	}
	if _, err := tx.Exec(`DELETE FROM prospeccao_listas WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
