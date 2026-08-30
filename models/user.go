package models

import (
	"database/sql"
	"strings"
	"time"
)

// Os papéis e permissões ficam em permission.go (RoleSeller/RoleManager/RoleAdmin).

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	TeamID    *int64    `json:"team_id"`
	TeamName  string    `json:"team_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// PasswordHash nunca é serializado para o front.
	PasswordHash string `json:"-"`
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

const userSelect = `
	SELECT u.id, u.name, u.email, u.role, u.active, u.team_id, COALESCE(t.name,''),
	       u.created_at, u.updated_at, u.password_hash
	FROM users u
	LEFT JOIN teams t ON t.id = u.team_id`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active, &u.TeamID, &u.TeamName,
		&u.CreatedAt, &u.UpdatedAt, &u.PasswordHash)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func UserByEmail(db *sql.DB, email string) (*User, error) {
	row := db.QueryRow(userSelect+` WHERE u.email = $1`, NormalizeEmail(email))
	return scanUser(row)
}

func UserByID(db *sql.DB, id int64) (*User, error) {
	row := db.QueryRow(userSelect+` WHERE u.id = $1`, id)
	return scanUser(row)
}

func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(userSelect + ` ORDER BY u.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func CountUsers(db *sql.DB) (int, error) {
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total)
	return total, err
}

func CreateUser(db *sql.DB, u *User) error {
	return db.QueryRow(`
		INSERT INTO users (name, email, password_hash, role, active, team_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		u.Name, NormalizeEmail(u.Email), u.PasswordHash, u.Role, u.Active, u.TeamID,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func UpdateUser(db *sql.DB, u *User) error {
	_, err := db.Exec(`
		UPDATE users SET name = $1, email = $2, role = $3, active = $4, team_id = $5, updated_at = NOW()
		WHERE id = $6`,
		u.Name, NormalizeEmail(u.Email), u.Role, u.Active, u.TeamID, u.ID)
	return err
}

func UpdateUserPassword(db *sql.DB, userID int64, passwordHash string) error {
	_, err := db.Exec(`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, passwordHash, userID)
	return err
}

// PasswordReset representa um token de redefinição de senha.
type PasswordReset struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
	Used      bool
}

func CreatePasswordReset(db *sql.DB, userID int64, token string, expiresAt time.Time) error {
	_, err := db.Exec(`INSERT INTO password_resets (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, expiresAt)
	return err
}

func PasswordResetByToken(db *sql.DB, token string) (*PasswordReset, error) {
	var pr PasswordReset
	err := db.QueryRow(`
		SELECT id, user_id, token, expires_at, used
		FROM password_resets WHERE token = $1`, token,
	).Scan(&pr.ID, &pr.UserID, &pr.Token, &pr.ExpiresAt, &pr.Used)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}

func MarkPasswordResetUsed(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE password_resets SET used = TRUE WHERE id = $1`, id)
	return err
}
