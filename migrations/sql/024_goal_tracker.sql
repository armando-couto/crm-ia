-- Metas acompanhadas (Insights): além da meta mensal simples da Previsão
-- (sales_goals), estas têm tipo, métrica, funil e duração com início e fim.
CREATE TABLE IF NOT EXISTS tracked_goals (
    id          BIGSERIAL PRIMARY KEY,
    -- kind: ganho | adicionado | progresso | atividade
    kind        VARCHAR(20)    NOT NULL,
    -- metric: valor | numero (atividade é sempre numero)
    metric      VARCHAR(10)    NOT NULL DEFAULT 'valor',
    -- Responsável: nulo = a empresa inteira.
    user_id     BIGINT REFERENCES users(id) ON DELETE CASCADE,
    pipeline_id BIGINT REFERENCES pipelines(id) ON DELETE SET NULL,
    -- Etapa observada (só no tipo progresso).
    stage_id    BIGINT REFERENCES pipeline_stages(id) ON DELETE SET NULL,
    -- Tipo de atividade observado (só no tipo atividade: ligacao, reuniao...).
    activity_kind VARCHAR(20)  NOT NULL DEFAULT '',
    -- Alvo por mês.
    amount      NUMERIC(14, 2) NOT NULL,
    -- Duração: primeiro dia do mês inicial e (opcional) do final.
    start_period DATE          NOT NULL,
    end_period   DATE,
    created_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tracked_goals_period ON tracked_goals (start_period, end_period);
