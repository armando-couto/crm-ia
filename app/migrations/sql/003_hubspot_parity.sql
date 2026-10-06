-- Paridade com o HubSpot: tickets, listas, projetos, caixa de entrada,
-- chamadas, reuniões, manuais de atividades, modelos de mensagens e snippets.

CREATE TABLE IF NOT EXISTS tickets (
    id          BIGSERIAL PRIMARY KEY,
    subject     VARCHAR(200) NOT NULL,
    description TEXT,
    status      VARCHAR(20) NOT NULL DEFAULT 'aberto', -- aberto | pendente | resolvido | fechado
    priority    VARCHAR(10) NOT NULL DEFAULT 'media',  -- baixa | media | alta
    contact_id  BIGINT REFERENCES contacts (id) ON DELETE SET NULL,
    company_id  BIGINT REFERENCES companies (id) ON DELETE SET NULL,
    owner_id    BIGINT REFERENCES users (id),
    closed_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets (status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_owner ON tickets (owner_id);
CREATE INDEX IF NOT EXISTS idx_tickets_contact ON tickets (contact_id);

-- Timeline nos tickets.
ALTER TABLE activities ADD COLUMN IF NOT EXISTS ticket_id BIGINT REFERENCES tickets (id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_activities_ticket ON activities (ticket_id, created_at DESC);

-- Listas / segmentos de contatos.
CREATE TABLE IF NOT EXISTS lists (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(150) NOT NULL,
    kind       VARCHAR(10) NOT NULL DEFAULT 'estatica', -- estatica | dinamica
    rules      JSONB, -- para listas dinâmicas: lifecycle_stage, owner_id, source
    created_by BIGINT REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS list_members (
    list_id    BIGINT NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    contact_id BIGINT NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (list_id, contact_id)
);

-- Projetos (com tarefas vinculadas).
CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    status      VARCHAR(15) NOT NULL DEFAULT 'ativo', -- ativo | concluido | arquivado
    due_date    DATE,
    owner_id    BIGINT REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS project_id BIGINT REFERENCES projects (id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks (project_id);

-- Caixa de entrada: conversas de e-mail (entrada via webhook do Mandrill).
CREATE TABLE IF NOT EXISTS conversations (
    id           BIGSERIAL PRIMARY KEY,
    subject      VARCHAR(300) NOT NULL,
    contact_id   BIGINT REFERENCES contacts (id) ON DELETE SET NULL,
    peer_email   VARCHAR(255) NOT NULL,
    status       VARCHAR(10) NOT NULL DEFAULT 'aberta', -- aberta | fechada
    unread       BOOLEAN NOT NULL DEFAULT TRUE,
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversations_last ON conversations (status, last_message_at DESC);

CREATE TABLE IF NOT EXISTS conversation_messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    direction       VARCHAR(10) NOT NULL, -- recebida | enviada
    from_email      VARCHAR(255) NOT NULL,
    to_email        VARCHAR(255) NOT NULL,
    subject         VARCHAR(300),
    body            TEXT NOT NULL,
    user_id         BIGINT REFERENCES users (id), -- quem respondeu (direção enviada)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conv_messages ON conversation_messages (conversation_id, created_at);

-- Registro de chamadas.
CREATE TABLE IF NOT EXISTS calls (
    id               BIGSERIAL PRIMARY KEY,
    direction        VARCHAR(10) NOT NULL DEFAULT 'saida', -- entrada | saida
    outcome          VARCHAR(20) NOT NULL DEFAULT 'conectada', -- conectada | sem_resposta | caixa_postal | ocupado | numero_errado
    duration_seconds INT NOT NULL DEFAULT 0,
    notes            TEXT,
    called_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    contact_id       BIGINT REFERENCES contacts (id) ON DELETE CASCADE,
    company_id       BIGINT REFERENCES companies (id) ON DELETE SET NULL,
    deal_id          BIGINT REFERENCES deals (id) ON DELETE SET NULL,
    user_id          BIGINT REFERENCES users (id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_calls_when ON calls (called_at DESC);
CREATE INDEX IF NOT EXISTS idx_calls_contact ON calls (contact_id);

-- Reuniões.
CREATE TABLE IF NOT EXISTS meetings (
    id         BIGSERIAL PRIMARY KEY,
    title      VARCHAR(200) NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'agendada', -- agendada | realizada | cancelada | nao_compareceu
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ,
    location   VARCHAR(300), -- endereço ou link da videochamada
    notes      TEXT,
    contact_id BIGINT REFERENCES contacts (id) ON DELETE CASCADE,
    company_id BIGINT REFERENCES companies (id) ON DELETE SET NULL,
    deal_id    BIGINT REFERENCES deals (id) ON DELETE SET NULL,
    user_id    BIGINT REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_meetings_starts ON meetings (starts_at);
CREATE INDEX IF NOT EXISTS idx_meetings_contact ON meetings (contact_id);

-- Biblioteca: manuais de atividades (playbooks), modelos de mensagens e snippets.
CREATE TABLE IF NOT EXISTS playbooks (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(200) NOT NULL,
    description VARCHAR(500),
    body        TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  BIGINT REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS message_templates (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(150) NOT NULL,
    subject    VARCHAR(200) NOT NULL,
    body       TEXT NOT NULL, -- variáveis: {{nome}}, {{sobrenome}}, {{email}}, {{empresa}}, {{cargo}}
    created_by BIGINT REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS snippets (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(150) NOT NULL,
    shortcut   VARCHAR(50) NOT NULL, -- ex.: #saudacao
    body       TEXT NOT NULL,
    created_by BIGINT REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
