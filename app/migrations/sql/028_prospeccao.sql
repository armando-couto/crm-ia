-- Prospecção: listas geradas a partir da base pública da plataforma, itens,
-- jobs em segundo plano, leads (caixa separada do funil), bloqueios de contato
-- e os campos que o funil ganhou junto (motivo de perda, origem do negócio).
-- Contrato em docs/PROSPECCAO.md, com as correções de docs/PROSPECCAO-ADENDO.md.

-- Defeito antigo: a descrição do CNAE (até ~150 caracteres) não cabia em 100.
ALTER TABLE companies ALTER COLUMN industry TYPE VARCHAR(255);

-- CNPJ comparado sem pontuação e aceitando letras (CNPJ alfanumérico, 2026).
-- A expressão é a mesma usada por models.CompanyByCNPJ.
CREATE INDEX IF NOT EXISTS idx_companies_cnpj_norm
    ON companies ((upper(regexp_replace(COALESCE(cnpj, ''), '[^0-9A-Za-z]', '', 'g'))));

-- Segmento: filtro salvo (não o resultado), para reexecutar quando a base muda.
CREATE TABLE IF NOT EXISTS prospeccao_segmentos (
    id                 BIGSERIAL PRIMARY KEY,
    nome               VARCHAR(150) NOT NULL,
    filtros            JSONB        NOT NULL DEFAULT '{}'::jsonb,
    ordem              VARCHAR(20)  NOT NULL DEFAULT 'relevancia',
    compartilhado      BOOLEAN      NOT NULL DEFAULT TRUE,
    ultima_contagem    INT,
    ultima_contagem_em TIMESTAMPTZ,
    criado_por         BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    criado_em          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    atualizado_em      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Lista de prospecção: CNPJs que ainda não são registros do CRM.
CREATE TABLE IF NOT EXISTS prospeccao_listas (
    id                BIGSERIAL PRIMARY KEY,
    nome              VARCHAR(150) NOT NULL,
    descricao         TEXT,
    origem            VARCHAR(10)  NOT NULL DEFAULT 'base',       -- base | cnpjs
    filtros           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    ordem             VARCHAR(20)  NOT NULL DEFAULT 'relevancia',
    quantidade_pedida INT          NOT NULL DEFAULT 0,
    excluir_no_crm    BOOLEAN      NOT NULL DEFAULT TRUE,
    excluir_em_listas BOOLEAN      NOT NULL DEFAULT TRUE,
    status            VARCHAR(10)  NOT NULL DEFAULT 'gerando',    -- gerando | pronta | falhou
    mensagem          TEXT,
    pedido_id         VARCHAR(40)  NOT NULL,                      -- idempotência da extração na plataforma
    versao_base       VARCHAR(60),
    incluir_pf        BOOLEAN      NOT NULL DEFAULT FALSE,
    segmento_id       BIGINT       REFERENCES prospeccao_segmentos(id) ON DELETE SET NULL,
    playbook_id       BIGINT       REFERENCES playbooks(id) ON DELETE SET NULL,        -- roteiro de ligação
    pipeline_id       BIGINT       REFERENCES pipelines(id) ON DELETE SET NULL,        -- destino da conversão
    stage_id          BIGINT       REFERENCES pipeline_stages(id) ON DELETE SET NULL,
    criado_por        BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    criado_em         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    atualizado_em     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_prospeccao_listas_pedido ON prospeccao_listas (pedido_id);

-- Lead: empresa a trabalhar, fora do funil até virar negócio.
CREATE TABLE IF NOT EXISTS leads (
    id                 BIGSERIAL PRIMARY KEY,
    titulo             VARCHAR(200) NOT NULL,
    company_id         BIGINT       REFERENCES companies(id) ON DELETE SET NULL,
    contact_id         BIGINT       REFERENCES contacts(id) ON DELETE SET NULL,   -- contato principal (decisor)
    cnpj               VARCHAR(14)  NOT NULL DEFAULT '',
    cidade             VARCHAR(120) NOT NULL DEFAULT '',
    uf                 VARCHAR(2)   NOT NULL DEFAULT '',
    origem             VARCHAR(15)  NOT NULL DEFAULT 'manual',    -- prospeccao | manual | formulario | importacao
    lista_id           BIGINT       REFERENCES prospeccao_listas(id) ON DELETE SET NULL,
    item_id            BIGINT,                                    -- FK criada depois de prospeccao_itens
    owner_id           BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    estado             VARCHAR(12)  NOT NULL DEFAULT 'novo',      -- novo | tentando | retornar | convertido | descartado
    temperatura        VARCHAR(10)  NOT NULL DEFAULT '',          -- '' | fria | media | quente
    etiquetas          TEXT[]       NOT NULL DEFAULT '{}',
    nota               SMALLINT     NOT NULL DEFAULT 0,
    valor_estimado     NUMERIC(14,2),
    telefones          JSONB        NOT NULL DEFAULT '[]'::jsonb, -- Telefone[], do melhor para o pior
    emails             JSONB        NOT NULL DEFAULT '[]'::jsonb, -- string[]
    resumo             JSONB        NOT NULL DEFAULT '{}'::jsonb, -- ResumoLead
    tentativas         INT          NOT NULL DEFAULT 0,
    ultimo_contato_em  TIMESTAMPTZ,
    ultimo_resultado   VARCHAR(20)  NOT NULL DEFAULT '',
    proxima_acao_em    TIMESTAMPTZ,
    motivo_descarte    VARCHAR(80)  NOT NULL DEFAULT '',
    descartado_em      TIMESTAMPTZ,
    deal_id            BIGINT       REFERENCES deals(id) ON DELETE SET NULL,
    meeting_id         BIGINT       REFERENCES meetings(id) ON DELETE SET NULL,
    convertido_em      TIMESTAMPTZ,
    whatsapp_optin     BOOLEAN      NOT NULL DEFAULT FALSE,
    whatsapp_optin_em  TIMESTAMPTZ,
    pessoa_fisica      BOOLEAN      NOT NULL DEFAULT FALSE,
    base_legal         VARCHAR(30)  NOT NULL DEFAULT 'legitimo_interesse',
    em_atendimento_por BIGINT       REFERENCES users(id) ON DELETE SET NULL,     -- trava da fila
    em_atendimento_ate TIMESTAMPTZ,
    visto_em           TIMESTAMPTZ,
    criado_por         BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    criado_em          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    atualizado_em      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_leads_fila    ON leads (owner_id, estado, proxima_acao_em);
CREATE INDEX IF NOT EXISTS idx_leads_estado  ON leads (estado, proxima_acao_em);
CREATE INDEX IF NOT EXISTS idx_leads_company ON leads (company_id);
CREATE INDEX IF NOT EXISTS idx_leads_contact ON leads (contact_id);
CREATE INDEX IF NOT EXISTS idx_leads_cnpj    ON leads (cnpj);
CREATE INDEX IF NOT EXISTS idx_leads_lista   ON leads (lista_id);
CREATE INDEX IF NOT EXISTS idx_leads_trava   ON leads (em_atendimento_por) WHERE em_atendimento_por IS NOT NULL;

-- Item da lista: retrato da empresa na base pública + enriquecimento.
CREATE TABLE IF NOT EXISTS prospeccao_itens (
    id                      BIGSERIAL PRIMARY KEY,
    lista_id                BIGINT       NOT NULL REFERENCES prospeccao_listas(id) ON DELETE CASCADE,
    cnpj                    VARCHAR(14)  NOT NULL,
    razao_social            VARCHAR(255) NOT NULL DEFAULT '',
    nome_fantasia           VARCHAR(255) NOT NULL DEFAULT '',
    uf                      VARCHAR(2)   NOT NULL DEFAULT '',
    municipio               VARCHAR(120) NOT NULL DEFAULT '',
    cnae_codigo             INT          NOT NULL DEFAULT 0,
    cnae_descricao          VARCHAR(255) NOT NULL DEFAULT '',
    porte                   VARCHAR(15)  NOT NULL DEFAULT 'nao_informado',
    pessoa_fisica           BOOLEAN      NOT NULL DEFAULT FALSE,
    estado                  VARCHAR(15)  NOT NULL DEFAULT 'novo',  -- novo | enriquecido | sem_resultado | erro | enviado | descartado | duplicado | bloqueado
    estado_anterior         VARCHAR(15)  NOT NULL DEFAULT '',      -- para restaurar um descartado
    nota                    SMALLINT     NOT NULL DEFAULT 0,
    nota_motivos            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    tem_celular             BOOLEAN      NOT NULL DEFAULT FALSE,
    tem_email               BOOLEAN      NOT NULL DEFAULT FALSE,
    tem_decisor             BOOLEAN      NOT NULL DEFAULT FALSE,
    tem_whatsapp            BOOLEAN      NOT NULL DEFAULT FALSE,
    tem_divida              BOOLEAN      NOT NULL DEFAULT FALSE,
    contato_compartilhado   BOOLEAN      NOT NULL DEFAULT FALSE,     -- único contato é de contador ou compartilhado
    telefone_principal      VARCHAR(20)  NOT NULL DEFAULT '',
    dados                   JSONB        NOT NULL DEFAULT '{}'::jsonb,  -- Empresa
    enriquecimento          JSONB,                                      -- PerfilContato (já com CPF mascarado)
    enriquecimento_bruto    JSONB,                                      -- resposta do provedor (CPF já mascarado); nunca sai em rota nem em exportação
    enriquecido_em          TIMESTAMPTZ,
    enriquecimento_provedor VARCHAR(20)  NOT NULL DEFAULT '',
    custo_centavos          INT          NOT NULL DEFAULT 0,                 -- soma de todos os enriquecimentos do item
    erro                    TEXT,
    company_id              BIGINT       REFERENCES companies(id) ON DELETE SET NULL,
    lead_id                 BIGINT       REFERENCES leads(id) ON DELETE SET NULL,
    enviado_em              TIMESTAMPTZ,
    criado_em               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    atualizado_em           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (lista_id, cnpj)
);
CREATE INDEX IF NOT EXISTS idx_prospeccao_itens_estado ON prospeccao_itens (lista_id, estado);
CREATE INDEX IF NOT EXISTS idx_prospeccao_itens_nota   ON prospeccao_itens (lista_id, nota DESC, id);
CREATE INDEX IF NOT EXISTS idx_prospeccao_itens_cnpj   ON prospeccao_itens (cnpj);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'leads_item_fk') THEN
        ALTER TABLE leads ADD CONSTRAINT leads_item_fk
            FOREIGN KEY (item_id) REFERENCES prospeccao_itens(id) ON DELETE SET NULL;
    END IF;
END $$;
-- Um lead por item: o envio ao CRM insere com ON CONFLICT DO NOTHING e relê,
-- o que torna a retomada do job repetível.
CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_item ON leads (item_id) WHERE item_id IS NOT NULL;

-- Jobs em segundo plano: gerar lista, enriquecer, enviar ao CRM.
-- lista_id é SET NULL: o custo do enriquecimento (contabilizado aqui, por mês
-- de criado_em) sobrevive à exclusão da lista.
CREATE TABLE IF NOT EXISTS prospeccao_jobs (
    id                  BIGSERIAL PRIMARY KEY,
    tipo                VARCHAR(20)  NOT NULL,                     -- gerar_lista | enriquecer | enviar_crm
    lista_id            BIGINT       REFERENCES prospeccao_listas(id) ON DELETE SET NULL,
    status              VARCHAR(12)  NOT NULL DEFAULT 'pendente',  -- pendente | rodando | concluido | falhou | cancelado
    parametros          JSONB        NOT NULL DEFAULT '{}'::jsonb, -- entrada do job
    estado              JSONB        NOT NULL DEFAULT '{}'::jsonb, -- cursor de retomada
    resultado           JSONB,                                     -- resumo final
    total               INT          NOT NULL DEFAULT 0,
    feitos              INT          NOT NULL DEFAULT 0,
    erros               INT          NOT NULL DEFAULT 0,
    custo_centavos      INT          NOT NULL DEFAULT 0,
    mensagem            TEXT,
    tentativas          INT          NOT NULL DEFAULT 0,
    proxima_execucao_em TIMESTAMPTZ  NOT NULL DEFAULT NOW(),      -- também é o prazo da fatia em curso
    limpeza_pendente    BOOLEAN      NOT NULL DEFAULT FALSE,      -- cancelado: o worker ainda roda o encerramento do tipo
    criado_por          BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    criado_em           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    iniciado_em         TIMESTAMPTZ,
    finalizado_em       TIMESTAMPTZ,
    atualizado_em       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_prospeccao_jobs_fila  ON prospeccao_jobs (status, proxima_execucao_em);
CREATE INDEX IF NOT EXISTS idx_prospeccao_jobs_lista ON prospeccao_jobs (lista_id, criado_em DESC);
CREATE INDEX IF NOT EXISTS idx_prospeccao_jobs_custo ON prospeccao_jobs (criado_em) WHERE tipo = 'enriquecer';
-- No máximo um job ativo por lista (violação vira 409 "job_ativo").
CREATE UNIQUE INDEX IF NOT EXISTS idx_prospeccao_jobs_ativo
    ON prospeccao_jobs (lista_id) WHERE status IN ('pendente', 'rodando');

-- "Não contatar": sobrevive à exclusão do lead e a novas listas.
CREATE TABLE IF NOT EXISTS contato_bloqueios (
    id         BIGSERIAL PRIMARY KEY,
    tipo       VARCHAR(10)  NOT NULL,                              -- telefone | email | cnpj
    valor      VARCHAR(255) NOT NULL,                              -- normalizado
    motivo     VARCHAR(20)  NOT NULL DEFAULT 'manual',             -- opt_out | numero_errado | pedido_titular | cliente | manual | lista_externa
    canal      VARCHAR(20)  NOT NULL DEFAULT '',
    observacao TEXT,
    criado_por BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    criado_em  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tipo, valor)
);

-- Chamadas: de qual lead, qual número e o desfecho comercial.
ALTER TABLE calls
    ADD COLUMN IF NOT EXISTS lead_id  BIGINT REFERENCES leads(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS desfecho VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS numero   VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS chave    VARCHAR(40);                 -- idempotência do resultado (UUID da tela)
CREATE INDEX IF NOT EXISTS idx_calls_lead      ON calls (lead_id);
CREATE INDEX IF NOT EXISTS idx_calls_user_when ON calls (user_id, called_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_calls_chave ON calls (chave) WHERE chave IS NOT NULL;

-- Negócios: motivo de perda e de onde veio.
ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS lost_reason VARCHAR(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lost_note   TEXT,
    ADD COLUMN IF NOT EXISTS lead_id     BIGINT REFERENCES leads(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS origem      VARCHAR(30) NOT NULL DEFAULT '';  -- origem do lead (prospeccao | manual | formulario | importacao); vazio fora de lead

-- Próxima atividade do negócio (semáforo do quadro).
CREATE INDEX IF NOT EXISTS idx_tasks_deal_pendente ON tasks (deal_id, due_date) WHERE completed_at IS NULL;
