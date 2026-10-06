-- Automações: um gatilho dispara uma sequência de ações. Com a ação "aguardar"
-- a automação vira uma sequência de e-mails ao longo de dias.
CREATE TABLE IF NOT EXISTS automations (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(150) NOT NULL,
    description    VARCHAR(400) NOT NULL DEFAULT '',
    trigger_kind   VARCHAR(40)  NOT NULL,
    trigger_config JSONB        NOT NULL DEFAULT '{}'::jsonb,
    actions        JSONB        NOT NULL DEFAULT '[]'::jsonb,
    active         BOOLEAN      NOT NULL DEFAULT TRUE,
    runs           INTEGER      NOT NULL DEFAULT 0,
    last_run_at    TIMESTAMPTZ,
    created_by     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_automations_trigger ON automations (trigger_kind, active);

-- Histórico de execuções, para a equipe entender o que a automação fez.
CREATE TABLE IF NOT EXISTS automation_runs (
    id            BIGSERIAL PRIMARY KEY,
    automation_id BIGINT      NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    entity        VARCHAR(20) NOT NULL DEFAULT '',
    entity_id     BIGINT,
    -- status: sucesso | erro | aguardando
    status        VARCHAR(20) NOT NULL DEFAULT 'sucesso',
    detail        VARCHAR(500) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_automation_runs ON automation_runs (automation_id, created_at DESC);

-- Sequências em andamento: onde cada contato parou e quando retomar.
CREATE TABLE IF NOT EXISTS sequence_enrollments (
    id            BIGSERIAL PRIMARY KEY,
    automation_id BIGINT      NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    contact_id    BIGINT      NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    deal_id       BIGINT      REFERENCES deals(id) ON DELETE SET NULL,
    -- Índice da próxima ação a executar dentro do array actions.
    step          INTEGER     NOT NULL DEFAULT 0,
    next_run_at   TIMESTAMPTZ NOT NULL,
    -- status: ativa | concluida | cancelada
    status        VARCHAR(20) NOT NULL DEFAULT 'ativa',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (automation_id, contact_id)
);

CREATE INDEX IF NOT EXISTS idx_sequence_due
    ON sequence_enrollments (status, next_run_at);
