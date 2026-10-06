-- Categoria de previsão por negócio, como no Sales Forecast do HubSpot:
-- o vendedor classifica a chance de cada negócio fechar no período.
-- excluido | pipeline | melhor_caso | comprometido | fechado
ALTER TABLE deals ADD COLUMN IF NOT EXISTS forecast_category VARCHAR(20) NOT NULL DEFAULT 'pipeline';

CREATE INDEX IF NOT EXISTS idx_deals_forecast ON deals (forecast_category) WHERE status = 'aberto';

-- Envio de previsão: o número que o próprio vendedor submete para o mês,
-- com a observação. Guarda o histórico (um envio por pessoa por período).
CREATE TABLE IF NOT EXISTS forecast_submissions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period     DATE           NOT NULL,
    amount     NUMERIC(14, 2) NOT NULL DEFAULT 0,
    note       VARCHAR(500)   NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, period)
);
