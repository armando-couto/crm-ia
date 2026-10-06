-- Campos de negócio nas empresas (equivalentes às propriedades
-- personalizadas usadas no HubSpot): dados de credenciamento/adquirência.

ALTER TABLE companies ADD COLUMN IF NOT EXISTS ec_number         VARCHAR(30);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS economic_group    VARCHAR(200);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS cnpj              VARCHAR(20);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS accredited_at     DATE;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS representative    VARCHAR(150);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS instagram         VARCHAR(100);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS products          TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE companies ADD COLUMN IF NOT EXISTS machines_count    INT NOT NULL DEFAULT 0;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS is_client         BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS anticipation_mode VARCHAR(20); -- pontual | automatica | nenhuma
ALTER TABLE companies ADD COLUMN IF NOT EXISTS validator         BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS do_not_disturb    BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_companies_ec ON companies (ec_number);
CREATE INDEX IF NOT EXISTS idx_companies_cnpj ON companies (cnpj);
