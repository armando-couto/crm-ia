-- Funil visual: prazo saudável por etapa e rastro de mudanças dos negócios
-- (valor, data de fechamento, etapa, dono, status), que permite reconstruir
-- a movimentação do pipeline em qualquer período sem snapshot diário.
ALTER TABLE pipeline_stages ADD COLUMN IF NOT EXISTS prazo_dias INT;

CREATE TABLE IF NOT EXISTS deal_changes (
    id         BIGSERIAL PRIMARY KEY,
    deal_id    BIGINT      NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    campo      VARCHAR(20) NOT NULL, -- amount|close_date|stage_id|owner_id|status
    de         TEXT,
    para       TEXT,
    user_id    BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_deal_changes_deal  ON deal_changes (deal_id, created_at);
CREATE INDEX IF NOT EXISTS idx_deal_changes_campo ON deal_changes (campo, created_at);

-- Inteligência de empresas: dados cadastrais vindos da Receita Federal e de
-- provedores pagos, com a fonte de cada campo e o histórico das consultas.
ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS legal_name             VARCHAR(255),
    ADD COLUMN IF NOT EXISTS trade_name             VARCHAR(255),
    ADD COLUMN IF NOT EXISTS cnae_code              VARCHAR(10),
    ADD COLUMN IF NOT EXISTS cnae_description       VARCHAR(255),
    ADD COLUMN IF NOT EXISTS legal_nature           VARCHAR(120),
    ADD COLUMN IF NOT EXISTS founded_at             DATE,
    ADD COLUMN IF NOT EXISTS capital                NUMERIC(16,2),
    ADD COLUMN IF NOT EXISTS company_size           VARCHAR(30),
    ADD COLUMN IF NOT EXISTS registration_status    VARCHAR(30),
    ADD COLUMN IF NOT EXISTS registration_status_at DATE,
    ADD COLUMN IF NOT EXISTS simples                BOOLEAN,
    ADD COLUMN IF NOT EXISTS mei                    BOOLEAN,
    ADD COLUMN IF NOT EXISTS street                 VARCHAR(255),
    ADD COLUMN IF NOT EXISTS street_number          VARCHAR(20),
    ADD COLUMN IF NOT EXISTS complement             VARCHAR(120),
    ADD COLUMN IF NOT EXISTS neighborhood           VARCHAR(120),
    ADD COLUMN IF NOT EXISTS zip                    VARCHAR(10),
    ADD COLUMN IF NOT EXISTS email                  VARCHAR(255),
    ADD COLUMN IF NOT EXISTS website                VARCHAR(255),
    ADD COLUMN IF NOT EXISTS employees_range        VARCHAR(40),
    ADD COLUMN IF NOT EXISTS revenue_range          VARCHAR(60),
    ADD COLUMN IF NOT EXISTS intel                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS enriched_at            TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS company_enrichments (
    id               BIGSERIAL PRIMARY KEY,
    company_id       BIGINT      NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    provedor         VARCHAR(20) NOT NULL,
    status           VARCHAR(20) NOT NULL,
    mensagem         TEXT,
    datasets         TEXT,
    demo             BOOLEAN     NOT NULL DEFAULT FALSE,
    custo_centavos   INT         NOT NULL DEFAULT 0,
    perfil           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    bruto            JSONB,
    aplicado_em      TIMESTAMPTZ,
    campos_aplicados TEXT[],
    user_id          BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_company_enrichments_company ON company_enrichments (company_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_company_enrichments_mes     ON company_enrichments (created_at);

CREATE TABLE IF NOT EXISTS company_field_sources (
    company_id    BIGINT      NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    campo         VARCHAR(60) NOT NULL,
    provedor      VARCHAR(20) NOT NULL,
    enrichment_id BIGINT      REFERENCES company_enrichments(id) ON DELETE SET NULL,
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (company_id, campo)
);
