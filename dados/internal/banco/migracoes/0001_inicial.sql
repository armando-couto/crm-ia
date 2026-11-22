-- Tabelas de controle da carga. Os esquemas de serviço (prospec,
-- prospec_amostra), o de carga (carga) e o da dívida ativa (pgfn) não são
-- criados aqui: a carga os constrói e troca. Nenhuma extensão é exigida.

-- Uma linha por execução de carga; a mais recente alimenta GET /v1/situacao.
CREATE TABLE cargas (
    id            BIGSERIAL PRIMARY KEY,
    fonte         TEXT        NOT NULL,              -- cnpj | pgfn | tudo
    referencia    TEXT        NOT NULL DEFAULT '',
    estado        TEXT        NOT NULL DEFAULT 'baixando'
                  CHECK (estado IN ('ociosa', 'baixando', 'carregando', 'construindo', 'erro')),
    etapa         TEXT        NOT NULL DEFAULT '',
    percentual    INT         NOT NULL DEFAULT 0,
    erro          TEXT        NOT NULL DEFAULT '',
    estatisticas  JSONB       NOT NULL DEFAULT '{}',
    iniciada_em   TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizada_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    finalizada_em TIMESTAMPTZ
);
CREATE INDEX cargas_abertas ON cargas (id) WHERE finalizada_em IS NULL;

-- Estado por arquivo: é o que torna o download e a carga retomáveis.
CREATE TABLE carga_arquivos (
    fonte         TEXT        NOT NULL,              -- cnpj | pgfn
    referencia    TEXT        NOT NULL,
    nome          TEXT        NOT NULL,
    url           TEXT        NOT NULL DEFAULT '',
    estado        TEXT        NOT NULL DEFAULT 'pendente'
                  CHECK (estado IN ('pendente', 'baixando', 'baixado', 'carregado', 'erro')),
    bytes         BIGINT      NOT NULL DEFAULT 0,
    total_bytes   BIGINT      NOT NULL DEFAULT 0,
    linhas        BIGINT      NOT NULL DEFAULT 0,    -- linhas gravadas na tabela de carga
    linhas_lidas  BIGINT      NOT NULL DEFAULT 0,    -- registros lidos do arquivo
    etag          TEXT        NOT NULL DEFAULT '',
    modificado    TEXT        NOT NULL DEFAULT '',
    erro          TEXT        NOT NULL DEFAULT '',
    atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (fonte, referencia, nome)
);

-- Listagens vistas de cada referência: uma referência do CNPJ só é elegível
-- quando duas listagens separadas por 30 minutos têm a mesma assinatura.
CREATE TABLE publicacoes (
    fonte       TEXT        NOT NULL,
    referencia  TEXT        NOT NULL,
    assinatura  TEXT        NOT NULL,
    visto_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (fonte, referencia)
);

-- Arquivos tocados pela execução corrente (para a tela de situação).
ALTER TABLE carga_arquivos ADD COLUMN carga_id BIGINT;
CREATE INDEX carga_arquivos_carga ON carga_arquivos (carga_id);
