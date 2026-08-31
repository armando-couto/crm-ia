-- Painéis: conjuntos de relatórios montados pela equipe, que podem ser
-- pendurados no espaço de trabalho de vendas.
CREATE TABLE IF NOT EXISTS dashboards (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(150) NOT NULL,
    shared     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dashboard_items (
    id           BIGSERIAL PRIMARY KEY,
    dashboard_id BIGINT      NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    report_id    BIGINT      NOT NULL REFERENCES reports(id)    ON DELETE CASCADE,
    position     INTEGER     NOT NULL DEFAULT 0,
    -- largura: meio (metade da linha) ou inteiro
    width        VARCHAR(10) NOT NULL DEFAULT 'meio',
    UNIQUE (dashboard_id, report_id)
);

CREATE INDEX IF NOT EXISTS idx_dashboard_items ON dashboard_items (dashboard_id, position);

-- Painel escolhido por cada pessoa na aba Painel do espaço de trabalho.
ALTER TABLE users ADD COLUMN IF NOT EXISTS workspace_dashboard_id BIGINT
    REFERENCES dashboards(id) ON DELETE SET NULL;
