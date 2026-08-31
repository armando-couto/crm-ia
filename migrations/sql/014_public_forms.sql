-- Formulários públicos de captura de leads (embutidos no site por iframe).
CREATE TABLE IF NOT EXISTS public_forms (
    id              BIGSERIAL PRIMARY KEY,
    slug            VARCHAR(80)  NOT NULL UNIQUE,
    name            VARCHAR(150) NOT NULL,
    headline        VARCHAR(200) NOT NULL DEFAULT '',
    description     VARCHAR(500) NOT NULL DEFAULT '',
    fields          JSONB        NOT NULL DEFAULT '[]'::jsonb,
    submit_label    VARCHAR(60)  NOT NULL DEFAULT 'Enviar',
    success_message VARCHAR(300) NOT NULL DEFAULT 'Recebemos seus dados. Em breve entraremos em contato.',
    redirect_url    VARCHAR(500) NOT NULL DEFAULT '',
    -- O que fazer com o contato criado:
    owner_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    list_id         BIGINT REFERENCES lists(id) ON DELETE SET NULL,
    lifecycle_stage VARCHAR(30)  NOT NULL DEFAULT 'lead',
    source          VARCHAR(60)  NOT NULL DEFAULT 'formulario',
    active          BOOLEAN      NOT NULL DEFAULT TRUE,
    submissions     INTEGER      NOT NULL DEFAULT 0,
    created_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS form_submissions (
    id         BIGSERIAL PRIMARY KEY,
    form_id    BIGINT      NOT NULL REFERENCES public_forms(id) ON DELETE CASCADE,
    contact_id BIGINT      REFERENCES contacts(id) ON DELETE SET NULL,
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ip         VARCHAR(60) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_form_submissions_form ON form_submissions (form_id, created_at DESC);
