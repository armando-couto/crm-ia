-- Metas de vendas por mês. user_id nulo = meta da equipe inteira.
CREATE TABLE IF NOT EXISTS sales_goals (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    -- Sempre o primeiro dia do mês, para comparar período com período.
    period     DATE           NOT NULL,
    amount     NUMERIC(14, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- Uma meta por pessoa por mês (e uma da equipe, com user_id nulo).
CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_goals_user
    ON sales_goals (user_id, period) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_goals_team
    ON sales_goals (period) WHERE user_id IS NULL;
