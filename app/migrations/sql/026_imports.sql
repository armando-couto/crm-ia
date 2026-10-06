-- Centro de importações: histórico de cargas de arquivo (CSV/XLSX), no molde
-- do HubSpot — cada carga guarda quantos registros criou, quantos ATUALIZOU
-- (casamento por e-mail no contato e por CNPJ/nome na empresa), quantas
-- associações contato→empresa criou e os erros linha a linha.
CREATE TABLE IF NOT EXISTS imports (
    id BIGSERIAL PRIMARY KEY,
    file_name TEXT NOT NULL,
    entity TEXT NOT NULL DEFAULT 'contatos', -- contatos | empresas
    status TEXT NOT NULL DEFAULT 'concluida', -- concluida | falhou
    total_rows INT NOT NULL DEFAULT 0,
    new_records INT NOT NULL DEFAULT 0,
    updated_records INT NOT NULL DEFAULT 0,
    new_associations INT NOT NULL DEFAULT 0,
    error_count INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_imports_created ON imports (created_at DESC);
