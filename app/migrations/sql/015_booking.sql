-- Página pública de agendamento: cada pessoa da equipe publica um link com a
-- própria disponibilidade e o cliente escolhe o horário.
CREATE TABLE IF NOT EXISTS booking_pages (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slug         VARCHAR(80)  NOT NULL UNIQUE,
    title        VARCHAR(150) NOT NULL DEFAULT 'Agende uma conversa',
    description  VARCHAR(500) NOT NULL DEFAULT '',
    location     VARCHAR(200) NOT NULL DEFAULT '',
    duration_min INTEGER      NOT NULL DEFAULT 30,
    buffer_min   INTEGER      NOT NULL DEFAULT 0,
    -- Quantos dias à frente a agenda fica aberta.
    days_ahead   INTEGER      NOT NULL DEFAULT 14,
    -- Aviso mínimo em horas antes do primeiro horário disponível.
    notice_hours INTEGER      NOT NULL DEFAULT 4,
    -- Janelas por dia da semana: {"1":[["09:00","12:00"],["13:00","18:00"]]}
    -- com 0 = domingo, no fuso do servidor.
    weekly_hours JSONB        NOT NULL DEFAULT '{}'::jsonb,
    active       BOOLEAN      NOT NULL DEFAULT TRUE,
    bookings     INTEGER      NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_booking_pages_user ON booking_pages (user_id);

-- Marca as reuniões que nasceram de um agendamento público.
ALTER TABLE meetings ADD COLUMN IF NOT EXISTS booking_page_id BIGINT
    REFERENCES booking_pages(id) ON DELETE SET NULL;
