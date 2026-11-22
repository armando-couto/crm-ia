package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Estados do item da lista.
const (
	ItemNovo         = "novo"
	ItemEnriquecido  = "enriquecido"
	ItemSemResultado = "sem_resultado"
	ItemErro         = "erro"
	ItemEnviado      = "enviado"
	ItemDescartado   = "descartado"
	ItemDuplicado    = "duplicado"
	ItemBloqueado    = "bloqueado"
)

// ItemEstados é a lista fechada de estados.
var ItemEstados = []string{ItemNovo, ItemEnriquecido, ItemSemResultado, ItemErro, ItemEnviado, ItemDescartado, ItemDuplicado, ItemBloqueado}

// ItemEstadosAptosEnvio são os estados de onde um item pode ir para o CRM.
var ItemEstadosAptosEnvio = []string{ItemNovo, ItemEnriquecido, ItemSemResultado}

// ItemEstadosAEnriquecer são os estados que o job de enriquecimento processa
// (ItemEnriquecido só entra com reprocessar).
var ItemEstadosAEnriquecer = []string{ItemNovo, ItemSemResultado, ItemErro}

func ItemEstadoValido(s string) bool { return slices.Contains(ItemEstados, s) }

// MaxSelecaoIDs é o teto de ids (e de exceto) numa Selecao.
const MaxSelecaoIDs = 5000

// ProspItem é uma empresa da lista: retrato da base pública + enriquecimento.
type ProspItem struct {
	ID                     int64        `json:"id"`
	ListaID                int64        `json:"lista_id"`
	CNPJ                   string       `json:"cnpj"`
	RazaoSocial            string       `json:"razao_social"`
	NomeFantasia           string       `json:"nome_fantasia"`
	UF                     string       `json:"uf"`
	Municipio              string       `json:"municipio"`
	CnaeCodigo             int          `json:"cnae_codigo"`
	CnaeDescricao          string       `json:"cnae_descricao"`
	Porte                  string       `json:"porte"`
	PessoaFisica           bool         `json:"pessoa_fisica"`
	Estado                 string       `json:"estado"`
	Nota                   int          `json:"nota"`
	NotaMotivos            []NotaMotivo `json:"nota_motivos"`
	TemCelular             bool         `json:"tem_celular"`
	TemEmail               bool         `json:"tem_email"`
	TemDecisor             bool         `json:"tem_decisor"`
	TemWhatsApp            bool         `json:"tem_whatsapp"`
	TemDivida              bool         `json:"tem_divida"`
	ContatoCompartilhado   bool         `json:"contato_compartilhado"`
	TelefonePrincipal      string       `json:"telefone_principal"`
	EnriquecidoEm          *time.Time   `json:"enriquecido_em"`
	EnriquecimentoProvedor string       `json:"enriquecimento_provedor"`
	CustoCentavos          int          `json:"custo_centavos"`
	Erro                   string       `json:"erro"`
	CompanyID              *int64       `json:"company_id"`
	LeadID                 *int64       `json:"lead_id"`
	EnviadoEm              *time.Time   `json:"enviado_em"`
	CriadoEm               time.Time    `json:"criado_em"`
	// Só no detalhe (ProspItemByID e ProspItensCompletos). O bruto do provedor
	// nunca é lido para esta estrutura: não sai em rota nem em exportação.
	Dados          *ProspEmpresa  `json:"dados,omitempty"`
	Enriquecimento *PerfilContato `json:"enriquecimento,omitempty"`
}

const prospItemCols = `i.id, i.lista_id, i.cnpj, i.razao_social, i.nome_fantasia, i.uf, i.municipio,
	i.cnae_codigo, i.cnae_descricao, i.porte, i.pessoa_fisica, i.estado, i.nota, i.nota_motivos,
	i.tem_celular, i.tem_email, i.tem_decisor, i.tem_whatsapp, i.tem_divida, i.contato_compartilhado,
	i.telefone_principal, i.enriquecido_em, i.enriquecimento_provedor, i.custo_centavos, COALESCE(i.erro,''),
	i.company_id, i.lead_id, i.enviado_em, i.criado_em`

func scanProspItem(row interface{ Scan(...any) error }, completo bool) (*ProspItem, error) {
	var it ProspItem
	var motivos, dados, enriq []byte
	dest := []any{&it.ID, &it.ListaID, &it.CNPJ, &it.RazaoSocial, &it.NomeFantasia, &it.UF, &it.Municipio,
		&it.CnaeCodigo, &it.CnaeDescricao, &it.Porte, &it.PessoaFisica, &it.Estado, &it.Nota, &motivos,
		&it.TemCelular, &it.TemEmail, &it.TemDecisor, &it.TemWhatsApp, &it.TemDivida, &it.ContatoCompartilhado,
		&it.TelefonePrincipal, &it.EnriquecidoEm, &it.EnriquecimentoProvedor, &it.CustoCentavos, &it.Erro,
		&it.CompanyID, &it.LeadID, &it.EnviadoEm, &it.CriadoEm}
	if completo {
		dest = append(dest, &dados, &enriq)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	it.NotaMotivos = []NotaMotivo{}
	_ = json.Unmarshal(motivos, &it.NotaMotivos)
	if completo {
		if len(dados) > 2 {
			var e ProspEmpresa
			if json.Unmarshal(dados, &e) == nil {
				it.Dados = &e
			}
		}
		if len(enriq) > 0 && string(enriq) != "null" {
			var p PerfilContato
			if json.Unmarshal(enriq, &p) == nil {
				it.Enriquecimento = &p
			}
		}
	}
	return &it, nil
}

// InsertProspItens grava os itens em lote (blocos de 500), pulando CNPJ que já
// está na lista. Usa de cada item: cnpj, retrato (razão, fantasia, UF,
// município, CNAE, porte, PF), nota, motivos, marcas, telefone principal e
// Dados. Devolve quantos foram de fato inseridos.
func InsertProspItens(db Executor, listaID int64, itens []ProspItem) (int, error) {
	const porBloco = 500
	const colunas = 20
	inseridos := 0
	for ini := 0; ini < len(itens); ini += porBloco {
		bloco := itens[ini:min(ini+porBloco, len(itens))]
		var sb strings.Builder
		sb.WriteString(`INSERT INTO prospeccao_itens (lista_id, cnpj, razao_social, nome_fantasia, uf, municipio,
			cnae_codigo, cnae_descricao, porte, pessoa_fisica, nota, nota_motivos, tem_celular, tem_email,
			tem_decisor, tem_whatsapp, tem_divida, contato_compartilhado, telefone_principal, dados) VALUES `)
		args := make([]any, 0, len(bloco)*colunas)
		for n, it := range bloco {
			if n > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('(')
			for c := 0; c < colunas; c++ {
				if c > 0 {
					sb.WriteByte(',')
				}
				fmt.Fprintf(&sb, "$%d", n*colunas+c+1)
			}
			sb.WriteByte(')')
			porte := it.Porte
			if porte == "" {
				porte = PorteNaoInformado
			}
			motivos := it.NotaMotivos
			if motivos == nil {
				motivos = []NotaMotivo{}
			}
			args = append(args, listaID, NormalizarCNPJ(it.CNPJ), cortarTexto(it.RazaoSocial, 255),
				cortarTexto(it.NomeFantasia, 255), cortarTexto(strings.ToUpper(it.UF), 2), cortarTexto(it.Municipio, 120),
				it.CnaeCodigo, cortarTexto(it.CnaeDescricao, 255), porte, it.PessoaFisica, limitarNota(it.Nota),
				jsonOu(motivos, "[]"), it.TemCelular, it.TemEmail, it.TemDecisor, it.TemWhatsApp, it.TemDivida,
				it.ContatoCompartilhado, cortarTexto(it.TelefonePrincipal, 20), jsonOu(it.Dados, "{}"))
		}
		sb.WriteString(` ON CONFLICT (lista_id, cnpj) DO NOTHING`)
		res, err := db.Exec(sb.String(), args...)
		if err != nil {
			return inseridos, err
		}
		n, _ := res.RowsAffected()
		inseridos += int(n)
	}
	return inseridos, nil
}

func limitarNota(n int) int { return max(0, min(100, n)) }

// ProspItemFiltro é o filtro da tabela de itens e também o filtro de uma
// Selecao com todos = true (mesmas chaves JSON do contrato).
type ProspItemFiltro struct {
	Estado     string `json:"estado,omitempty"`
	Q          string `json:"q,omitempty"`
	ComCelular *bool  `json:"com_celular,omitempty"`
	ComDecisor *bool  `json:"com_decisor,omitempty"`
	NotaMin    int    `json:"nota_min,omitempty"`
}

// condicoes acrescenta as condições do filtro (alias "i") e os argumentos.
func (f ProspItemFiltro) condicoes(where *[]string, args *[]any) {
	add := func(cond string, v any) {
		*args = append(*args, v)
		*where = append(*where, strings.ReplaceAll(cond, "$N", fmt.Sprintf("$%d", len(*args))))
	}
	if f.Estado != "" {
		add("i.estado = $N", f.Estado)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		if c := NormalizarCNPJ(q); len(c) >= 8 && strings.Trim(c[:8], "0123456789") == "" {
			add("(i.cnpj LIKE $N || '%')", c)
		} else {
			add("(i.razao_social ILIKE $N OR i.nome_fantasia ILIKE $N OR i.municipio ILIKE $N)", likeContem(q))
		}
	}
	if f.ComCelular != nil {
		add("i.tem_celular = $N", *f.ComCelular)
	}
	if f.ComDecisor != nil {
		add("i.tem_decisor = $N", *f.ComDecisor)
	}
	if f.NotaMin > 0 {
		add("i.nota >= $N", f.NotaMin)
	}
}

// ProspItemLista é a consulta paginada da tabela de itens.
type ProspItemLista struct {
	ProspItemFiltro
	Sort string // nota (padrão) | razao_social | municipio | estado
	Dir  string // asc | desc (padrão: desc para nota, asc para os demais)
	Pagination
}

// ListProspItens devolve a página de itens (sem dados nem enriquecimento) e o total.
func ListProspItens(db *sql.DB, listaID int64, f ProspItemLista) ([]ProspItem, int, error) {
	f.Normalize()
	where := []string{"i.lista_id = $1"}
	args := []any{listaID}
	f.ProspItemFiltro.condicoes(&where, &args)
	cond := strings.Join(where, " AND ")

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prospeccao_itens i WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	desc := strings.EqualFold(f.Dir, "desc")
	var ordem string
	switch f.Sort {
	case "razao_social":
		ordem = "i.razao_social"
	case "municipio":
		ordem = "i.municipio"
	case "estado":
		ordem = "i.estado"
	default:
		ordem = "i.nota"
		desc = !strings.EqualFold(f.Dir, "asc")
	}
	if desc {
		ordem += " DESC"
	}
	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`SELECT %s FROM prospeccao_itens i WHERE %s ORDER BY %s, i.id LIMIT $%d OFFSET $%d`,
		prospItemCols, cond, ordem, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	itens := []ProspItem{}
	for rows.Next() {
		it, err := scanProspItem(rows, false)
		if err != nil {
			return nil, 0, err
		}
		itens = append(itens, *it)
	}
	return itens, total, rows.Err()
}

// ProspItemByID devolve o item com dados e enriquecimento (sem o bruto).
func ProspItemByID(db Executor, id int64) (*ProspItem, error) {
	return scanProspItem(db.QueryRow(`SELECT `+prospItemCols+`, i.dados, i.enriquecimento
		FROM prospeccao_itens i WHERE i.id = $1`, id), true)
}

// ProspItensCompletos devolve os itens pedidos (com dados e enriquecimento),
// em ordem de id. É o que os jobs usam para processar uma página da seleção.
func ProspItensCompletos(db Executor, ids []int64) ([]ProspItem, error) {
	if len(ids) == 0 {
		return []ProspItem{}, nil
	}
	rows, err := db.Query(`SELECT `+prospItemCols+`, i.dados, i.enriquecimento
		FROM prospeccao_itens i WHERE i.id = ANY($1) ORDER BY i.id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	itens := make([]ProspItem, 0, len(ids))
	for rows.Next() {
		it, err := scanProspItem(rows, true)
		if err != nil {
			return nil, err
		}
		itens = append(itens, *it)
	}
	return itens, rows.Err()
}

// ProspSelecao diz quais itens de uma lista uma ação alcança: ids (até 5.000)
// OU todos = true, com filtro opcional e a lista de exceções (linhas
// desmarcadas depois de "selecionar todos os N").
type ProspSelecao struct {
	IDs    []int64          `json:"ids,omitempty"`
	Todos  bool             `json:"todos,omitempty"`
	Filtro *ProspItemFiltro `json:"filtro,omitempty"`
	Exceto []int64          `json:"exceto,omitempty"`
}

// Validar confere a forma da seleção (mensagem pronta para 422).
func (s ProspSelecao) Validar() error {
	if s.Todos {
		if len(s.IDs) > 0 {
			return fmt.Errorf("informe ids ou todos, não os dois")
		}
		if len(s.Exceto) > MaxSelecaoIDs {
			return fmt.Errorf("a seleção aceita até %d exceções", MaxSelecaoIDs)
		}
		if s.Filtro != nil && s.Filtro.Estado != "" && !ItemEstadoValido(s.Filtro.Estado) {
			return fmt.Errorf("estado inválido no filtro da seleção")
		}
		return nil
	}
	if len(s.Exceto) > 0 {
		return fmt.Errorf("exceto só vale com todos = true")
	}
	if len(s.IDs) == 0 {
		return fmt.Errorf("selecione ao menos um item")
	}
	if len(s.IDs) > MaxSelecaoIDs {
		return fmt.Errorf("a seleção aceita até %d itens por vez", MaxSelecaoIDs)
	}
	return nil
}

// condicoes monta o WHERE da seleção dentro de uma lista (alias "i").
// estados, quando não vazio, restringe aos estados informados.
func (s ProspSelecao) condicoes(listaID int64, estados []string) (string, []any) {
	where := []string{"i.lista_id = $1"}
	args := []any{listaID}
	if s.Todos {
		if s.Filtro != nil {
			s.Filtro.condicoes(&where, &args)
		}
		if len(s.Exceto) > 0 {
			args = append(args, pq.Array(s.Exceto))
			where = append(where, fmt.Sprintf("i.id <> ALL($%d)", len(args)))
		}
	} else {
		args = append(args, pq.Array(s.IDs))
		where = append(where, fmt.Sprintf("i.id = ANY($%d)", len(args)))
	}
	if len(estados) > 0 {
		args = append(args, pq.Array(estados))
		where = append(where, fmt.Sprintf("i.estado = ANY($%d)", len(args)))
	}
	return strings.Join(where, " AND "), args
}

// CountProspSelecao conta os itens da seleção (restritos a estados, se informado).
func CountProspSelecao(db Executor, listaID int64, s ProspSelecao, estados []string) (int, error) {
	cond, args := s.condicoes(listaID, estados)
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM prospeccao_itens i WHERE `+cond, args...).Scan(&n)
	return n, err
}

// ProspSelecaoResumo conta a seleção por estado e pelas marcas que as prévias
// (enriquecer e enviar) mostram.
type ProspSelecaoResumo struct {
	Total        int
	PorEstado    map[string]int
	PessoaFisica int // entre os aptos a envio
	SemTelefone  int // entre os aptos a envio
}

// ResumoProspSelecao agrega a seleção numa consulta só.
func ResumoProspSelecao(db Executor, listaID int64, s ProspSelecao) (ProspSelecaoResumo, error) {
	r := ProspSelecaoResumo{PorEstado: map[string]int{}}
	cond, args := s.condicoes(listaID, nil)
	rows, err := db.Query(`
		SELECT i.estado, COUNT(*),
		       COUNT(*) FILTER (WHERE i.pessoa_fisica),
		       COUNT(*) FILTER (WHERE i.telefone_principal = '')
		FROM prospeccao_itens i WHERE `+cond+` GROUP BY i.estado`, args...)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var estado string
		var n, pf, semTel int
		if err := rows.Scan(&estado, &n, &pf, &semTel); err != nil {
			return r, err
		}
		r.PorEstado[estado] = n
		r.Total += n
		if slices.Contains(ItemEstadosAptosEnvio, estado) {
			r.PessoaFisica += pf
			r.SemTelefone += semTel
		}
	}
	return r, rows.Err()
}

// ResolveProspSelecao transforma a seleção em ids, uma página por vez e em
// ordem de id: passe aposID = 0 na primeira chamada e o último id recebido nas
// seguintes, até vir uma página vazia. estados restringe (vazio = qualquer).
// Como o job muda o estado dos itens que processa, a paginação por id continua
// correta mesmo com o conjunto encolhendo.
func ResolveProspSelecao(db Executor, listaID int64, s ProspSelecao, estados []string, aposID int64, limite int) ([]int64, error) {
	if limite < 1 {
		limite = 500
	}
	cond, args := s.condicoes(listaID, estados)
	args = append(args, aposID, limite)
	rows, err := db.Query(fmt.Sprintf(`SELECT i.id FROM prospeccao_itens i WHERE %s AND i.id > $%d ORDER BY i.id LIMIT $%d`,
		cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DescartarProspItens marca a seleção como descartada, guardando o estado
// anterior para restaurar. Itens já enviados ou já descartados não mudam.
func DescartarProspItens(db Executor, listaID int64, s ProspSelecao) (int, error) {
	cond, args := s.condicoes(listaID, nil)
	res, err := db.Exec(`
		UPDATE prospeccao_itens i
		SET estado_anterior = i.estado, estado = 'descartado', atualizado_em = NOW()
		WHERE `+cond+` AND i.estado NOT IN ('descartado', 'enviado')`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RestaurarProspItens devolve os descartados da seleção ao estado anterior.
func RestaurarProspItens(db Executor, listaID int64, s ProspSelecao) (int, error) {
	cond, args := s.condicoes(listaID, nil)
	res, err := db.Exec(`
		UPDATE prospeccao_itens i
		SET estado = COALESCE(NULLIF(i.estado_anterior, ''), 'novo'), estado_anterior = '', atualizado_em = NOW()
		WHERE `+cond+` AND i.estado = 'descartado'`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ProspItemEstado é a mudança de estado de um item feita pelos jobs.
// Campos nulos não mexem na coluna; Erro é sempre gravado (vazio limpa).
type ProspItemEstado struct {
	Estado    string
	CompanyID *int64
	LeadID    *int64
	Erro      string
}

// SetProspItemEstado muda o estado do item. "enviado" carimba enviado_em.
func SetProspItemEstado(db Executor, id int64, e ProspItemEstado) error {
	if !ItemEstadoValido(e.Estado) {
		return fmt.Errorf("estado de item inválido: %q", e.Estado)
	}
	_, err := db.Exec(`
		UPDATE prospeccao_itens
		SET estado = $2::varchar,
		    company_id = COALESCE($3::bigint, company_id),
		    lead_id = COALESCE($4::bigint, lead_id),
		    erro = NULLIF($5::text, ''),
		    enviado_em = CASE WHEN $2::varchar = 'enviado' THEN NOW() ELSE enviado_em END,
		    atualizado_em = NOW()
		WHERE id = $1`, id, e.Estado, e.CompanyID, e.LeadID, e.Erro)
	return err
}

// SetProspItemEmpresa grava a empresa criada para o item mantendo o estado: é
// o que deixa o envio ao CRM retomar do passo 3 depois de um reinício (item
// apto com company_id preenchido não é duplicado).
func SetProspItemEmpresa(db Executor, id, companyID int64) error {
	_, err := db.Exec(`UPDATE prospeccao_itens SET company_id = $2, atualizado_em = NOW() WHERE id = $1`, id, companyID)
	return err
}

// ProspItemEnriquecimento é o que o job de enriquecimento grava num item.
type ProspItemEnriquecimento struct {
	Estado   string          // enriquecido | sem_resultado | erro
	Perfil   *PerfilContato  // nil mantém o perfil que já havia
	Bruto    json.RawMessage // resposta do provedor com CPF já mascarado; nil mantém
	Provedor string
	// CustoCentavos é o custo DESTA consulta: é somado ao que o item já tinha
	// (reprocessar não apaga o custo anterior).
	CustoCentavos        int
	Nota                 int
	NotaMotivos          []NotaMotivo
	TemCelular           bool
	TemEmail             bool
	TemDecisor           bool
	TemWhatsApp          bool
	ContatoCompartilhado bool
	TelefonePrincipal    string
	Erro                 string
}

// UpdateProspItemEnriquecimento grava o resultado do enriquecimento. Em "erro"
// só o estado e a mensagem mudam: perfil, marcas e nota ficam como estavam.
func UpdateProspItemEnriquecimento(db Executor, id int64, e ProspItemEnriquecimento) error {
	switch e.Estado {
	case ItemErro:
		_, err := db.Exec(`
			UPDATE prospeccao_itens
			SET estado = 'erro', erro = NULLIF($2, ''), custo_centavos = custo_centavos + $3, atualizado_em = NOW()
			WHERE id = $1`, id, e.Erro, e.CustoCentavos)
		return err
	case ItemEnriquecido, ItemSemResultado:
	default:
		return fmt.Errorf("estado de enriquecimento inválido: %q", e.Estado)
	}
	var perfil, bruto any
	if e.Perfil != nil {
		perfil = jsonOu(e.Perfil, "null")
	}
	if len(e.Bruto) > 0 {
		bruto = string(e.Bruto)
	}
	motivos := e.NotaMotivos
	if motivos == nil {
		motivos = []NotaMotivo{}
	}
	_, err := db.Exec(`
		UPDATE prospeccao_itens
		SET estado = $2,
		    enriquecimento = COALESCE($3::jsonb, enriquecimento),
		    enriquecimento_bruto = COALESCE($4::jsonb, enriquecimento_bruto),
		    enriquecimento_provedor = $5,
		    enriquecido_em = NOW(),
		    custo_centavos = custo_centavos + $6,
		    nota = $7, nota_motivos = $8::jsonb,
		    tem_celular = $9, tem_email = $10, tem_decisor = $11, tem_whatsapp = $12,
		    contato_compartilhado = $13, telefone_principal = $14,
		    erro = NULL, atualizado_em = NOW()
		WHERE id = $1`,
		id, e.Estado, perfil, bruto, cortarTexto(e.Provedor, 20), e.CustoCentavos,
		limitarNota(e.Nota), jsonOu(motivos, "[]"),
		e.TemCelular, e.TemEmail, e.TemDecisor, e.TemWhatsApp,
		e.ContatoCompartilhado, cortarTexto(e.TelefonePrincipal, 20))
	return err
}

// ProspItensPorCNPJ mapeia CNPJ → id do item dentro de uma lista (para casar
// os resultados de um lote do provedor com os itens).
func ProspItensPorCNPJ(db Executor, listaID int64, cnpjs []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	rows, err := db.Query(`SELECT cnpj, id FROM prospeccao_itens WHERE lista_id = $1 AND cnpj = ANY($2)`,
		listaID, pq.Array(normalizarCNPJs(cnpjs)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cnpj string
		var id int64
		if err := rows.Scan(&cnpj, &id); err != nil {
			return nil, err
		}
		out[cnpj] = id
	}
	return out, rows.Err()
}

// CNPJsEmListas devolve, entre os CNPJs informados, os que já estão em alguma
// lista de prospecção (fora a lista exceto, se > 0). Usado por excluir_em_listas.
func CNPJsEmListas(db Executor, cnpjs []string, exceto int64) (map[string]bool, error) {
	out := map[string]bool{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	rows, err := db.Query(`SELECT DISTINCT cnpj FROM prospeccao_itens WHERE cnpj = ANY($1) AND lista_id <> $2`,
		pq.Array(normalizarCNPJs(cnpjs)), exceto)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cnpj string
		if err := rows.Scan(&cnpj); err != nil {
			return nil, err
		}
		out[cnpj] = true
	}
	return out, rows.Err()
}

// CNPJsNoCRM devolve CNPJ normalizado → id da empresa para os CNPJs que já são
// empresas do CRM (a mais antiga, se houver repetidas). Usa o índice
// idx_companies_cnpj_norm; serve a excluir_no_crm e à prévia do envio.
func CNPJsNoCRM(db Executor, cnpjs []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	rows, err := db.Query(`
		SELECT `+CNPJNormSQL+`, MIN(c.id) FROM companies c
		WHERE `+CNPJNormSQL+` = ANY($1) GROUP BY 1`, pq.Array(normalizarCNPJs(cnpjs)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cnpj string
		var id int64
		if err := rows.Scan(&cnpj, &id); err != nil {
			return nil, err
		}
		out[cnpj] = id
	}
	return out, rows.Err()
}

func normalizarCNPJs(cnpjs []string) []string {
	out := make([]string, 0, len(cnpjs))
	for _, c := range cnpjs {
		if c = NormalizarCNPJ(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}
