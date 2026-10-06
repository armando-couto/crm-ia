CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(150) NOT NULL,
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(20)  NOT NULL DEFAULT 'vendedor', -- admin | gestor | vendedor
    active        BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS password_resets (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token      VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS companies (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(200) NOT NULL,
    domain     VARCHAR(150),
    phone      VARCHAR(30),
    industry   VARCHAR(100),
    city       VARCHAR(100),
    state      VARCHAR(50),
    owner_id   BIGINT REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS contacts (
    id              BIGSERIAL PRIMARY KEY,
    first_name      VARCHAR(100) NOT NULL,
    last_name       VARCHAR(100),
    email           VARCHAR(255),
    phone           VARCHAR(30),
    job_title       VARCHAR(100),
    lifecycle_stage VARCHAR(30) NOT NULL DEFAULT 'lead', -- lead | mql | sql | oportunidade | cliente | perdido
    source          VARCHAR(100),
    company_id      BIGINT REFERENCES companies (id) ON DELETE SET NULL,
    owner_id        BIGINT REFERENCES users (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pipelines (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    position   INT         NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pipeline_stages (
    id          BIGSERIAL PRIMARY KEY,
    pipeline_id BIGINT      NOT NULL REFERENCES pipelines (id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    position    INT         NOT NULL DEFAULT 0,
    probability INT         NOT NULL DEFAULT 0, -- 0..100
    is_won      BOOLEAN     NOT NULL DEFAULT FALSE,
    is_lost     BOOLEAN     NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS deals (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(200) NOT NULL,
    amount      NUMERIC(14, 2) NOT NULL DEFAULT 0,
    currency    VARCHAR(3)  NOT NULL DEFAULT 'BRL',
    pipeline_id BIGINT      NOT NULL REFERENCES pipelines (id),
    stage_id    BIGINT      NOT NULL REFERENCES pipeline_stages (id),
    contact_id  BIGINT REFERENCES contacts (id) ON DELETE SET NULL,
    company_id  BIGINT REFERENCES companies (id) ON DELETE SET NULL,
    owner_id    BIGINT REFERENCES users (id),
    status      VARCHAR(10) NOT NULL DEFAULT 'aberto', -- aberto | ganho | perdido
    close_date  DATE,
    position    INT         NOT NULL DEFAULT 0,
    closed_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
    id           BIGSERIAL PRIMARY KEY,
    title        VARCHAR(200) NOT NULL,
    description  TEXT,
    type         VARCHAR(20) NOT NULL DEFAULT 'tarefa', -- ligacao | email | reuniao | tarefa
    priority     VARCHAR(10) NOT NULL DEFAULT 'media',  -- baixa | media | alta
    due_date     TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    owner_id     BIGINT REFERENCES users (id),
    contact_id   BIGINT REFERENCES contacts (id) ON DELETE CASCADE,
    company_id   BIGINT REFERENCES companies (id) ON DELETE CASCADE,
    deal_id      BIGINT REFERENCES deals (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS activities (
    id         BIGSERIAL PRIMARY KEY,
    kind       VARCHAR(20) NOT NULL, -- nota | email | ligacao | reuniao | sistema
    content    TEXT        NOT NULL,
    metadata   JSONB,
    user_id    BIGINT REFERENCES users (id),
    contact_id BIGINT REFERENCES contacts (id) ON DELETE CASCADE,
    company_id BIGINT REFERENCES companies (id) ON DELETE CASCADE,
    deal_id    BIGINT REFERENCES deals (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts (email);
CREATE INDEX IF NOT EXISTS idx_contacts_owner ON contacts (owner_id);
CREATE INDEX IF NOT EXISTS idx_contacts_company ON contacts (company_id);
CREATE INDEX IF NOT EXISTS idx_companies_owner ON companies (owner_id);
CREATE INDEX IF NOT EXISTS idx_deals_stage ON deals (stage_id);
CREATE INDEX IF NOT EXISTS idx_deals_pipeline_status ON deals (pipeline_id, status);
CREATE INDEX IF NOT EXISTS idx_deals_owner ON deals (owner_id);
CREATE INDEX IF NOT EXISTS idx_tasks_owner_due ON tasks (owner_id, due_date);
CREATE INDEX IF NOT EXISTS idx_activities_contact ON activities (contact_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activities_company ON activities (company_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activities_deal ON activities (deal_id, created_at DESC);
