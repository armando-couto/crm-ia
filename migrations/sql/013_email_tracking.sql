-- Rastreio dos e-mails enviados pelo CRM: aberturas (pixel) e cliques (redirect).
CREATE TABLE IF NOT EXISTS email_messages (
    id           BIGSERIAL PRIMARY KEY,
    token        VARCHAR(64)  NOT NULL UNIQUE,
    subject      VARCHAR(300) NOT NULL DEFAULT '',
    to_email     VARCHAR(255) NOT NULL DEFAULT '',
    body         TEXT         NOT NULL DEFAULT '',
    contact_id   BIGINT REFERENCES contacts(id) ON DELETE CASCADE,
    deal_id      BIGINT REFERENCES deals(id)    ON DELETE SET NULL,
    user_id      BIGINT REFERENCES users(id)    ON DELETE SET NULL,
    -- origem: manual (enviado por uma pessoa) ou automacao
    source       VARCHAR(20)  NOT NULL DEFAULT 'manual',
    opens        INTEGER      NOT NULL DEFAULT 0,
    clicks       INTEGER      NOT NULL DEFAULT 0,
    first_open_at TIMESTAMPTZ,
    last_open_at  TIMESTAMPTZ,
    first_click_at TIMESTAMPTZ,
    sent_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_messages_contact ON email_messages (contact_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_messages_sent    ON email_messages (sent_at DESC);

CREATE TABLE IF NOT EXISTS email_events (
    id         BIGSERIAL PRIMARY KEY,
    message_id BIGINT      NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
    -- kind: abertura | clique
    kind       VARCHAR(20) NOT NULL,
    url        TEXT        NOT NULL DEFAULT '',
    ip         VARCHAR(60) NOT NULL DEFAULT '',
    user_agent VARCHAR(300) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_events_message ON email_events (message_id, created_at DESC);
