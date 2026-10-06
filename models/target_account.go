package models

import (
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/lib/pq"
)

// Papéis do contato na decisão de compra.
var BuyingRoles = []string{"decisor", "influenciador", "usuario", "financeiro", "bloqueador", "campeao"}

var BuyingRoleLabels = map[string]string{
	"decisor":       "Decisor",
	"influenciador": "Influenciador",
	"usuario":       "Usuário",
	"financeiro":    "Financeiro",
	"bloqueador":    "Bloqueador",
	"campeao":       "Campeão",
}

func ValidBuyingRole(role string) bool {
	return role == "" || slices.Contains(BuyingRoles, role)
}

// TargetAccount é a empresa perseguida pela equipe, com o resumo do que já
// aconteceu nela: quem são os contatos, negócios abertos e a última interação.
type TargetAccount struct {
	Company
	ContactsCount  int        `json:"contacts_count_total"`
	DecisionMakers int        `json:"decision_makers"`
	OpenDeals      int        `json:"open_deals"`
	OpenAmount     float64    `json:"open_amount"`
	WonAmount      float64    `json:"won_amount"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	OpenTasks      int        `json:"open_tasks"`
}

// SetTargetAccount marca ou desmarca a empresa como conta-alvo.
func SetTargetAccount(db *sql.DB, id int64, isTarget bool, tier int, notes string) error {
	if isTarget && (tier < 1 || tier > 3) {
		return fmt.Errorf("a prioridade da conta-alvo deve ser 1, 2 ou 3")
	}
	if !isTarget {
		tier = 0
		notes = ""
	}
	_, err := db.Exec(`
		UPDATE companies
		SET is_target = $1,
		    target_tier = $2,
		    target_notes = $3,
		    -- Mantém a data original quando a conta já era alvo.
		    target_since = CASE WHEN $1 THEN COALESCE(target_since, NOW()) ELSE NULL END,
		    updated_at = NOW()
		WHERE id = $4`, isTarget, tier, notes, id)
	return err
}

// ListTargetAccounts devolve as contas-alvo com o resumo de cada uma,
// ordenadas por prioridade e depois pelo maior valor em aberto.
func ListTargetAccounts(db *sql.DB, ownerID int64) ([]TargetAccount, error) {
	where := "c.is_target = TRUE"
	args := []any{}
	if ownerID > 0 {
		args = append(args, ownerID)
		where += " AND c.owner_id = $1"
	}

	query := fmt.Sprintf(`
		SELECT c.id, c.name, COALESCE(c.domain,''), COALESCE(c.phone,''), COALESCE(c.industry,''),
		       COALESCE(c.city,''), COALESCE(c.state,''), c.owner_id, COALESCE(u.name,''),
		       c.created_at, c.updated_at, %s,
		       (SELECT COUNT(*) FROM contacts ct WHERE ct.company_id = c.id),
		       (SELECT COUNT(*) FROM contacts ct WHERE ct.company_id = c.id AND ct.buying_role = 'decisor'),
		       (SELECT COUNT(*) FROM deals d WHERE d.company_id = c.id AND d.status = 'aberto'),
		       (SELECT COALESCE(SUM(d.amount),0) FROM deals d WHERE d.company_id = c.id AND d.status = 'aberto') AS open_amount,
		       (SELECT COALESCE(SUM(d.amount),0) FROM deals d WHERE d.company_id = c.id AND d.status = 'ganho'),
		       (SELECT MAX(a.created_at) FROM activities a WHERE a.company_id = c.id),
		       (SELECT COUNT(*) FROM tasks t WHERE t.company_id = c.id AND t.completed_at IS NULL)
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_id
		WHERE %s
		ORDER BY c.target_tier, open_amount DESC NULLS LAST, c.name`,
		companyBusinessColumns, where)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []TargetAccount{}
	for rows.Next() {
		var t TargetAccount
		var products pq.StringArray
		dest := []any{
			&t.ID, &t.Name, &t.Domain, &t.Phone, &t.Industry, &t.City, &t.State,
			&t.OwnerID, &t.OwnerName, &t.CreatedAt, &t.UpdatedAt,
		}
		dest = append(dest, t.businessScanTargets(&products)...)
		dest = append(dest, &t.ContactsCount, &t.DecisionMakers, &t.OpenDeals,
			&t.OpenAmount, &t.WonAmount, &t.LastActivityAt, &t.OpenTasks)

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		t.Products = products
		list = append(list, t)
	}
	return list, rows.Err()
}

// TargetAccountSummary alimenta os cartões do topo da tela de contas-alvo.
type TargetAccountSummary struct {
	Total      int     `json:"total"`
	Tier1      int     `json:"tier1"`
	WithDeals  int     `json:"with_deals"`
	OpenAmount float64 `json:"open_amount"`
	NoActivity int     `json:"no_activity_30d"`
	NoDecision int     `json:"without_decision_maker"`
}

func LoadTargetSummary(db *sql.DB) (*TargetAccountSummary, error) {
	var s TargetAccountSummary
	err := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE target_tier = 1),
		       COUNT(*) FILTER (WHERE EXISTS (
		           SELECT 1 FROM deals d WHERE d.company_id = c.id AND d.status = 'aberto')),
		       COALESCE(SUM((SELECT COALESCE(SUM(d.amount),0) FROM deals d
		           WHERE d.company_id = c.id AND d.status = 'aberto')), 0),
		       COUNT(*) FILTER (WHERE NOT EXISTS (
		           SELECT 1 FROM activities a
		           WHERE a.company_id = c.id AND a.created_at >= NOW() - INTERVAL '30 days')),
		       COUNT(*) FILTER (WHERE NOT EXISTS (
		           SELECT 1 FROM contacts ct
		           WHERE ct.company_id = c.id AND ct.buying_role = 'decisor'))
		FROM companies c
		WHERE c.is_target = TRUE`,
	).Scan(&s.Total, &s.Tier1, &s.WithDeals, &s.OpenAmount, &s.NoActivity, &s.NoDecision)
	return &s, err
}
