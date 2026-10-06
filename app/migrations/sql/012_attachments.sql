-- Arquivos anexados a contatos, empresas, negócios e tickets.
-- O conteúdo fica no próprio Postgres (bytea): produção roda um único container
-- sem volume persistente, então guardar no banco evita perder anexo a cada deploy
-- e já entra no backup existente. O limite de tamanho é aplicado na aplicação.
CREATE TABLE IF NOT EXISTS attachments (
    id           BIGSERIAL PRIMARY KEY,
    entity       VARCHAR(20)  NOT NULL,
    entity_id    BIGINT       NOT NULL,
    filename     VARCHAR(255) NOT NULL,
    content_type VARCHAR(120) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes   BIGINT       NOT NULL DEFAULT 0,
    data         BYTEA        NOT NULL,
    uploaded_by  BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachments_entity ON attachments (entity, entity_id, created_at DESC);
