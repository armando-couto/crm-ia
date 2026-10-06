-- Visualizações salvas (abas de filtros em contatos/empresas) e
-- configurações do app (ex.: campos do formulário de criação de contato).

CREATE TABLE IF NOT EXISTS app_settings (
    key        VARCHAR(100) PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_by BIGINT REFERENCES users (id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS saved_views (
    id         BIGSERIAL PRIMARY KEY,
    entity     VARCHAR(20)  NOT NULL, -- contacts | companies
    name       VARCHAR(100) NOT NULL,
    filters    JSONB        NOT NULL,
    position   INT          NOT NULL DEFAULT 0,
    created_by BIGINT REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_saved_views_entity ON saved_views (entity, position, id);
