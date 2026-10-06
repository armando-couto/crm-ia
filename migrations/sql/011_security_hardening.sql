-- Endurecimento de segurança: senha temporária com validade e troca
-- obrigatória, invalidação de sessões antigas e trilha de auditoria.

-- Troca obrigatória no primeiro acesso (convites) e validade do convite.
ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS invite_expires_at    TIMESTAMPTZ;

-- Tokens emitidos antes desta data são recusados (troca de senha derruba sessões).
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Trilha de auditoria: quem fez o quê, quando e de onde.
CREATE TABLE IF NOT EXISTS audit_log (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users (id) ON DELETE SET NULL,
    user_name  VARCHAR(150) NOT NULL DEFAULT '',
    action     VARCHAR(60)  NOT NULL, -- login | login_falha | criar | editar | excluir | exportar | permissoes | ...
    entity     VARCHAR(40)  NOT NULL DEFAULT '', -- contacts | companies | deals | users | ...
    entity_id  BIGINT,
    summary    VARCHAR(300) NOT NULL DEFAULT '',
    details    JSONB,
    ip         VARCHAR(60)  NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_log (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_log (entity, entity_id, created_at DESC);
