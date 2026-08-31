-- Histórico de passagem por etapa: é o que permite calcular conversão de funil
-- e tempo por etapa de verdade, em vez de olhar só a foto atual.
CREATE TABLE IF NOT EXISTS deal_stage_history (
    id         BIGSERIAL PRIMARY KEY,
    deal_id    BIGINT      NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
    stage_id   BIGINT      NOT NULL REFERENCES pipeline_stages(id) ON DELETE CASCADE,
    entered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stage_history_deal  ON deal_stage_history (deal_id, entered_at);
CREATE INDEX IF NOT EXISTS idx_stage_history_stage ON deal_stage_history (stage_id, entered_at);

-- Backfill: cada negócio existente entra no histórico na etapa atual, datado
-- da criação. A conversão fica precisa daqui para frente.
INSERT INTO deal_stage_history (deal_id, stage_id, entered_at)
SELECT d.id, d.stage_id, d.created_at
FROM deals d
WHERE NOT EXISTS (SELECT 1 FROM deal_stage_history h WHERE h.deal_id = d.id);

-- Tipo do relatório: agregado (métrica × agrupamento, o que já existia),
-- conversao (funil), duracao (tempo por etapa) e progresso (série mensal).
ALTER TABLE reports ADD COLUMN IF NOT EXISTS kind VARCHAR(20) NOT NULL DEFAULT 'agregado';
