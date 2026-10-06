-- Relatórios montados pela equipe: entidade + métrica + agrupamento + filtros.
-- A consulta é gerada a partir de listas fechadas no código (nada de SQL livre).
CREATE TABLE IF NOT EXISTS reports (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(150) NOT NULL,
    description VARCHAR(400) NOT NULL DEFAULT '',
    entity      VARCHAR(30)  NOT NULL,
    metric      VARCHAR(30)  NOT NULL,
    dimension   VARCHAR(30)  NOT NULL,
    filters     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    -- chart: barras | linha | pizza | tabela
    chart       VARCHAR(20)  NOT NULL DEFAULT 'barras',
    -- Compartilhado: aparece para toda a equipe, não só para quem criou.
    shared      BOOLEAN      NOT NULL DEFAULT TRUE,
    position    INTEGER      NOT NULL DEFAULT 0,
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reports_owner ON reports (created_by, position);
