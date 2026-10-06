package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

var panelWidths = []string{"meio", "inteiro"}

// DashboardItem é um relatório pendurado no painel.
type DashboardItem struct {
	ID       int64  `json:"id"`
	ReportID int64  `json:"report_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Width    string `json:"width"`
	// Preenchido só quando o painel é aberto com os resultados.
	Result *ReportResult `json:"result,omitempty"`
}

// Panel é um painel: um nome e a lista de relatórios que ele mostra.
type Panel struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Shared    bool            `json:"shared"`
	CreatedBy *int64          `json:"created_by"`
	Items     []DashboardItem `json:"items"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func ValidPanelWidth(w string) bool {
	return slices.Contains(panelWidths, w)
}

// ListPanels devolve os painéis compartilhados mais os do próprio usuário.
func ListPanels(db *sql.DB, userID int64) ([]Panel, error) {
	rows, err := db.Query(`
		SELECT id, name, shared, created_by, created_at, updated_at
		FROM dashboards
		WHERE shared = TRUE OR created_by = $1
		ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Panel{}
	for rows.Next() {
		var p Panel
		if err := rows.Scan(&p.ID, &p.Name, &p.Shared, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Items = []DashboardItem{}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Os itens de todos os painéis em uma consulta só.
	items, err := db.Query(`
		SELECT i.dashboard_id, i.id, i.report_id, r.name, i.position, i.width
		FROM dashboard_items i
		JOIN reports r ON r.id = i.report_id
		ORDER BY i.position`)
	if err != nil {
		return nil, err
	}
	defer items.Close()

	byPanel := map[int64][]DashboardItem{}
	for items.Next() {
		var panelID int64
		var item DashboardItem
		if err := items.Scan(&panelID, &item.ID, &item.ReportID, &item.Name,
			&item.Position, &item.Width); err != nil {
			return nil, err
		}
		byPanel[panelID] = append(byPanel[panelID], item)
	}
	if err := items.Err(); err != nil {
		return nil, err
	}
	for i := range list {
		if found, ok := byPanel[list[i].ID]; ok {
			list[i].Items = found
		}
	}
	return list, nil
}

func PanelByID(db *sql.DB, id int64) (*Panel, error) {
	var p Panel
	err := db.QueryRow(`
		SELECT id, name, shared, created_by, created_at, updated_at
		FROM dashboards WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Shared, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT i.id, i.report_id, r.name, i.position, i.width
		FROM dashboard_items i
		JOIN reports r ON r.id = i.report_id
		WHERE i.dashboard_id = $1
		ORDER BY i.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	p.Items = []DashboardItem{}
	for rows.Next() {
		var item DashboardItem
		if err := rows.Scan(&item.ID, &item.ReportID, &item.Name,
			&item.Position, &item.Width); err != nil {
			return nil, err
		}
		p.Items = append(p.Items, item)
	}
	return &p, rows.Err()
}

// RunPanel abre o painel já com o resultado de cada relatório.
func RunPanel(db *sql.DB, id int64) (*Panel, error) {
	panel, err := PanelByID(db, id)
	if err != nil {
		return nil, err
	}
	for i := range panel.Items {
		report, err := ReportByID(db, panel.Items[i].ReportID)
		if err != nil {
			continue
		}
		result, err := RunReport(db, report)
		if err != nil {
			// Um relatório quebrado não derruba o painel inteiro.
			continue
		}
		panel.Items[i].Result = result
	}
	return panel, nil
}

func CreatePanel(db *sql.DB, p *Panel) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("informe o nome do painel")
	}
	return db.QueryRow(`
		INSERT INTO dashboards (name, shared, created_by) VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		p.Name, p.Shared, p.CreatedBy,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func UpdatePanel(db *sql.DB, p *Panel) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("informe o nome do painel")
	}
	_, err := db.Exec(`
		UPDATE dashboards SET name = $1, shared = $2, updated_at = NOW() WHERE id = $3`,
		p.Name, p.Shared, p.ID)
	return err
}

func DeletePanel(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM dashboards WHERE id = $1`, id)
	return err
}

// AddPanelItem pendura um relatório no painel (no fim da lista).
func AddPanelItem(db *sql.DB, panelID, reportID int64, width string) error {
	if !ValidPanelWidth(width) {
		width = "meio"
	}
	_, err := db.Exec(`
		INSERT INTO dashboard_items (dashboard_id, report_id, position, width)
		VALUES ($1, $2, COALESCE(
		    (SELECT MAX(position) + 1 FROM dashboard_items WHERE dashboard_id = $1), 0), $3)
		ON CONFLICT (dashboard_id, report_id) DO UPDATE SET width = EXCLUDED.width`,
		panelID, reportID, width)
	return err
}

func RemovePanelItem(db *sql.DB, panelID, itemID int64) error {
	_, err := db.Exec(`DELETE FROM dashboard_items WHERE dashboard_id = $1 AND id = $2`,
		panelID, itemID)
	return err
}

// SetWorkspaceDashboard guarda qual painel a pessoa vê na aba Painel.
func SetWorkspaceDashboard(db *sql.DB, userID int64, dashboardID *int64) error {
	_, err := db.Exec(`UPDATE users SET workspace_dashboard_id = $1 WHERE id = $2`,
		dashboardID, userID)
	return err
}

func WorkspaceDashboardID(db *sql.DB, userID int64) (*int64, error) {
	var id *int64
	err := db.QueryRow(`SELECT workspace_dashboard_id FROM users WHERE id = $1`, userID).Scan(&id)
	return id, err
}
