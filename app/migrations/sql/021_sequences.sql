-- Sequências de prospecção: uma cadência de etapas por contato, misturando
-- e-mail automático com tarefas manuais que entram na fila do vendedor.
CREATE TABLE IF NOT EXISTS sequences (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(150) NOT NULL,
    description VARCHAR(400) NOT NULL DEFAULT '',
    -- steps: [{"kind":"email_auto","delay_days":0,"subject":"","body":"","template_id":0}]
    steps       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    active      BOOLEAN      NOT NULL DEFAULT TRUE,
    -- Dinâmica: roda as etapas automáticas até o contato engajar; a partir daí
    -- pula para as etapas manuais.
    dynamic         BOOLEAN NOT NULL DEFAULT FALSE,
    -- Regras de saída automática.
    exit_on_reply   BOOLEAN NOT NULL DEFAULT TRUE,
    exit_on_meeting BOOLEAN NOT NULL DEFAULT TRUE,
    owner_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sequences_active ON sequences (active);

-- Quem está inscrito e em que ponto da cadência parou.
CREATE TABLE IF NOT EXISTS sequence_members (
    id          BIGSERIAL PRIMARY KEY,
    sequence_id BIGINT NOT NULL REFERENCES sequences(id) ON DELETE CASCADE,
    contact_id  BIGINT NOT NULL REFERENCES contacts(id)  ON DELETE CASCADE,
    -- Índice da próxima etapa dentro do array steps.
    step        INTEGER     NOT NULL DEFAULT 0,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- status: ativa | aguardando_tarefa | concluida | cancelada
    status      VARCHAR(24) NOT NULL DEFAULT 'ativa',
    -- Por que saiu: respondeu | reuniao | manual | fim
    exit_reason VARCHAR(20) NOT NULL DEFAULT '',
    -- Tarefa manual aberta que segura a cadência.
    task_id     BIGINT REFERENCES tasks(id) ON DELETE SET NULL,
    -- Marcado quando o contato abre, clica ou responde.
    engaged     BOOLEAN     NOT NULL DEFAULT FALSE,
    enrolled_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    enrolled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    UNIQUE (sequence_id, contact_id)
);

CREATE INDEX IF NOT EXISTS idx_sequence_members_due
    ON sequence_members (status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_sequence_members_contact
    ON sequence_members (contact_id, status);

-- Liga o e-mail enviado à sequência, para medir abertura e resposta.
ALTER TABLE email_messages ADD COLUMN IF NOT EXISTS sequence_id BIGINT
    REFERENCES sequences(id) ON DELETE SET NULL;
ALTER TABLE email_messages ADD COLUMN IF NOT EXISTS replied_at TIMESTAMPTZ;

-- Liga a tarefa manual à inscrição, para concluir a tarefa mover a cadência.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS sequence_member_id BIGINT
    REFERENCES sequence_members(id) ON DELETE SET NULL;
