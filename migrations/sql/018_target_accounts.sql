-- Contas-alvo: as empresas que a equipe escolheu perseguir, com prioridade,
-- e o papel de cada contato na decisão de compra.
ALTER TABLE companies ADD COLUMN IF NOT EXISTS is_target     BOOLEAN      NOT NULL DEFAULT FALSE;
-- tier: 1 (prioridade máxima), 2 ou 3
ALTER TABLE companies ADD COLUMN IF NOT EXISTS target_tier   SMALLINT     NOT NULL DEFAULT 0;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS target_notes  VARCHAR(500) NOT NULL DEFAULT '';
ALTER TABLE companies ADD COLUMN IF NOT EXISTS target_since  TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_companies_target ON companies (is_target, target_tier);

-- buying_role: decisor | influenciador | usuario | financeiro | bloqueador | campeao
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS buying_role VARCHAR(20) NOT NULL DEFAULT '';
