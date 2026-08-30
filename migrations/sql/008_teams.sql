-- Equipes (como no HubSpot): cada usuário pode ter uma equipe principal.

CREATE TABLE IF NOT EXISTS teams (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS team_id BIGINT REFERENCES teams (id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_users_team ON users (team_id);
