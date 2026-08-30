-- Preferências por usuário (ex.: notificações por e-mail).

CREATE TABLE IF NOT EXISTS user_settings (
    user_id    BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key        VARCHAR(100) NOT NULL,
    value      JSONB        NOT NULL,
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, key)
);
