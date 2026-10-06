-- Propriedades personalizadas (campos criados pela equipe), como no HubSpot:
-- cada objeto (contato, empresa, negócio, ticket) pode ter campos próprios.

CREATE TABLE IF NOT EXISTS custom_properties (
    id          BIGSERIAL PRIMARY KEY,
    entity      VARCHAR(20)  NOT NULL, -- contacts | companies | deals | tickets
    key         VARCHAR(60)  NOT NULL, -- nome interno (snake_case)
    label       VARCHAR(120) NOT NULL,
    description VARCHAR(300),
    field_type  VARCHAR(20)  NOT NULL, -- texto | texto_longo | numero | data | selecao | multipla | booleano
    options     JSONB        NOT NULL DEFAULT '[]', -- [{value,label}] para selecao/multipla
    group_name  VARCHAR(80)  NOT NULL DEFAULT 'Informações personalizadas',
    position    INT          NOT NULL DEFAULT 0,
    created_by  BIGINT REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (entity, key)
);

CREATE INDEX IF NOT EXISTS idx_custom_properties_entity ON custom_properties (entity, position, id);

CREATE TABLE IF NOT EXISTS custom_property_values (
    property_id BIGINT      NOT NULL REFERENCES custom_properties (id) ON DELETE CASCADE,
    record_id   BIGINT      NOT NULL,
    value       JSONB       NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (property_id, record_id)
);

CREATE INDEX IF NOT EXISTS idx_custom_values_record ON custom_property_values (record_id);
