-- =============================================================================
-- CRM IA — painel administrativo — esquema inicial
--
-- grupo    : conjunto de CNPJs de um mesmo cliente comercial (rede/franquia).
-- cliente  : um CNPJ. Vira um ambiente isolado (PostgreSQL + Redis + app) em
--            https://$DOMAIN/<slug>.
-- licença  : direito de uso de um plano por período, do grupo (cobrança
--            unificada) ou de um CNPJ. É cobrada por ASSINATURA RECORRENTE no
--            cartão de crédito; o cliente cadastra o cartão numa página com a
--            marca do CRM IA e nunca vê quem processa.
-- fatura   : a cobrança de uma competência. É baixada quando a recorrência
--            confirma o pagamento (conferência periódica/webhook) ou à mão.
-- plano    : faixa de usuários + preço; parametrizável e lido pela landing.
-- versão   : tag das imagens do app; cada cliente escolhe se acompanha.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE usuarios (
  id            BIGSERIAL PRIMARY KEY,
  nome          TEXT NOT NULL,
  email         TEXT NOT NULL UNIQUE,
  senha_hash    TEXT NOT NULL,
  perfil        TEXT NOT NULL CHECK (perfil IN ('admin','comercial','financeiro','suporte')),
  ativo         BOOLEAN NOT NULL DEFAULT true,
  ultimo_login  TIMESTAMPTZ,
  criado_em     TIMESTAMPTZ NOT NULL DEFAULT now(),
  atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE planos (
  id                    BIGSERIAL PRIMARY KEY,
  codigo                TEXT NOT NULL UNIQUE,
  nome                  TEXT NOT NULL,
  descricao             TEXT NOT NULL DEFAULT '',
  usuarios_min          INT  NOT NULL DEFAULT 1,
  usuarios_max          INT  NOT NULL,
  preco_mensal_centavos BIGINT NOT NULL,
  preco_anual_centavos  BIGINT NOT NULL DEFAULT 0,
  destaque              BOOLEAN NOT NULL DEFAULT false,
  recursos              JSONB NOT NULL DEFAULT '[]',
  ordem                 INT NOT NULL DEFAULT 0,
  ativo                 BOOLEAN NOT NULL DEFAULT true,
  criado_em             TIMESTAMPTZ NOT NULL DEFAULT now(),
  atualizado_em         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (usuarios_max >= usuarios_min)
);

CREATE TABLE configuracoes (
  chave         TEXT PRIMARY KEY,
  valor         TEXT NOT NULL,
  descricao     TEXT NOT NULL DEFAULT '',
  atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE grupos (
  id                 BIGSERIAL PRIMARY KEY,
  nome               TEXT NOT NULL,
  cnpj_responsavel   TEXT NOT NULL DEFAULT '',
  email_financeiro   TEXT NOT NULL DEFAULT '',
  telefone           TEXT NOT NULL DEFAULT '',
  cobranca_unificada BOOLEAN NOT NULL DEFAULT true,
  observacoes        TEXT NOT NULL DEFAULT '',
  ativo              BOOLEAN NOT NULL DEFAULT true,
  cep        TEXT NOT NULL DEFAULT '',
  logradouro TEXT NOT NULL DEFAULT '',
  numero     TEXT NOT NULL DEFAULT '',
  bairro     TEXT NOT NULL DEFAULT '',
  municipio  TEXT NOT NULL DEFAULT '',
  uf         TEXT NOT NULL DEFAULT '',
  pagador_id TEXT NOT NULL DEFAULT '',
  criado_em     TIMESTAMPTZ NOT NULL DEFAULT now(),
  atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clientes (
  id                   BIGSERIAL PRIMARY KEY,
  grupo_id             BIGINT NOT NULL REFERENCES grupos(id),
  razao_social         TEXT NOT NULL,
  nome_fantasia        TEXT NOT NULL,
  cnpj                 TEXT NOT NULL UNIQUE,
  slug                 TEXT NOT NULL UNIQUE,
  segmento             TEXT NOT NULL DEFAULT '',
  cidade               TEXT NOT NULL DEFAULT '',
  uf                   TEXT NOT NULL DEFAULT '',
  telefone             TEXT NOT NULL DEFAULT '',
  cep                  TEXT NOT NULL DEFAULT '',
  logradouro           TEXT NOT NULL DEFAULT '',
  numero               TEXT NOT NULL DEFAULT '',
  bairro               TEXT NOT NULL DEFAULT '',
  admin_nome           TEXT NOT NULL,
  admin_email          TEXT NOT NULL,
  plano_id             BIGINT REFERENCES planos(id),
  usuarios_contratados INT NOT NULL DEFAULT 5,
  cobranca_propria     BOOLEAN NOT NULL DEFAULT false,
  pagador_id           TEXT NOT NULL DEFAULT '',
  modelo_crm           TEXT NOT NULL DEFAULT '',

  status          TEXT NOT NULL DEFAULT 'pronto'
                  CHECK (status IN ('pronto','provisionando','ativo','suspenso','erro','cancelado')),
  versao          TEXT NOT NULL DEFAULT '',
  auto_atualizar  BOOLEAN NOT NULL DEFAULT true,
  cor_primaria    TEXT NOT NULL DEFAULT '#6d5df6',
  logo_url        TEXT NOT NULL DEFAULT '',
  url             TEXT NOT NULL DEFAULT '',
  provisionado_em TIMESTAMPTZ,
  ultimo_erro     TEXT NOT NULL DEFAULT '',
  suspenso_por_inadimplencia BOOLEAN NOT NULL DEFAULT false,
  criado_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
  atualizado_em   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX clientes_grupo_idx ON clientes (grupo_id);
CREATE INDEX clientes_status_idx ON clientes (status);

CREATE TABLE licencas (
  id             BIGSERIAL PRIMARY KEY,
  grupo_id       BIGINT NOT NULL REFERENCES grupos(id),
  cliente_id     BIGINT REFERENCES clientes(id),
  plano_id       BIGINT NOT NULL REFERENCES planos(id),
  usuarios_max   INT NOT NULL,
  periodicidade  TEXT NOT NULL CHECK (periodicidade IN ('mensal','anual')),
  valor_centavos BIGINT NOT NULL,
  inicio         DATE NOT NULL,
  fim            DATE NOT NULL,
  status         TEXT NOT NULL DEFAULT 'ativa' CHECK (status IN ('ativa','vencida','cancelada')),
  unidades       INT NOT NULL DEFAULT 1,
  observacoes    TEXT NOT NULL DEFAULT '',

  cobranca          TEXT NOT NULL DEFAULT 'recorrente' CHECK (cobranca IN ('recorrente','manual')),
  assinatura_id     TEXT NOT NULL DEFAULT '',
  assinatura_token  TEXT NOT NULL DEFAULT '',
  codigo_checkout   TEXT UNIQUE,
  pagamento_status  TEXT NOT NULL DEFAULT 'aguardando_cartao'
                    CHECK (pagamento_status IN ('aguardando_cartao','ativo','recusado','cancelado','manual')),
  cartao_final      TEXT NOT NULL DEFAULT '',
  cartao_bandeira   TEXT NOT NULL DEFAULT '',
  cartao_em         TIMESTAMPTZ,
  criado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
  atualizado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (fim > inicio)
);
CREATE INDEX licencas_grupo_idx ON licencas (grupo_id);
CREATE INDEX licencas_cliente_idx ON licencas (cliente_id);

CREATE TABLE faturas (
  id             BIGSERIAL PRIMARY KEY,
  licenca_id     BIGINT NOT NULL REFERENCES licencas(id),
  competencia    DATE NOT NULL,
  vencimento     DATE NOT NULL,
  valor_centavos BIGINT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'pendente' CHECK (status IN ('pendente','paga','vencida','cancelada')),
  forma          TEXT NOT NULL DEFAULT '',
  referencia     TEXT NOT NULL DEFAULT '',
  pago_em        TIMESTAMPTZ,
  criado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (licenca_id, competencia)
);
CREATE INDEX faturas_abertas_idx ON faturas (licenca_id) WHERE status IN ('pendente','vencida');

CREATE TABLE plano_solicitacoes (
  id           BIGSERIAL PRIMARY KEY,
  cliente_id   BIGINT NOT NULL REFERENCES clientes(id),
  tipo         TEXT NOT NULL CHECK (tipo IN ('upgrade','cancelamento')),
  plano_id     BIGINT REFERENCES planos(id),
  mensagem     TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'pendente' CHECK (status IN ('pendente','aprovada','recusada')),
  resposta     TEXT NOT NULL DEFAULT '',
  criado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
  decidido_em  TIMESTAMPTZ,
  decidido_por TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX plano_solicitacoes_pendente_uk ON plano_solicitacoes (cliente_id) WHERE status = 'pendente';

CREATE TABLE versoes (
  id           BIGSERIAL PRIMARY KEY,
  tag          TEXT NOT NULL UNIQUE,
  descricao    TEXT NOT NULL DEFAULT '',
  changelog    TEXT NOT NULL DEFAULT '',
  estavel      BOOLEAN NOT NULL DEFAULT true,
  padrao       BOOLEAN NOT NULL DEFAULT false,
  publicada_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX versoes_uma_padrao ON versoes ((true)) WHERE padrao;

CREATE TABLE leads (
  id              BIGSERIAL PRIMARY KEY,
  nome            TEXT NOT NULL,
  email           TEXT NOT NULL,
  telefone        TEXT NOT NULL DEFAULT '',
  empresa         TEXT NOT NULL DEFAULT '',
  segmento        TEXT NOT NULL DEFAULT '',
  cidade          TEXT NOT NULL DEFAULT '',
  usuarios        INT NOT NULL DEFAULT 0,
  plano_interesse TEXT NOT NULL DEFAULT '',
  mensagem        TEXT NOT NULL DEFAULT '',
  origem          TEXT NOT NULL DEFAULT 'landing',
  status          TEXT NOT NULL DEFAULT 'novo' CHECK (status IN ('novo','contato','convertido','perdido')),
  criado_em       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auditoria (
  id          BIGSERIAL PRIMARY KEY,
  usuario_id  BIGINT REFERENCES usuarios(id),
  usuario     TEXT NOT NULL,
  acao        TEXT NOT NULL,
  entidade    TEXT NOT NULL,
  entidade_id BIGINT,
  detalhes    JSONB NOT NULL DEFAULT '{}',
  ip          TEXT NOT NULL DEFAULT '',
  criado_em   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auditoria_criado_idx ON auditoria (criado_em DESC);

CREATE TABLE eventos_ambiente (
  id          BIGSERIAL PRIMARY KEY,
  cliente_id  BIGINT NOT NULL REFERENCES clientes(id) ON DELETE CASCADE,
  tipo        TEXT NOT NULL,
  detalhe     TEXT NOT NULL DEFAULT '',
  criado_em   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX eventos_ambiente_cliente_idx ON eventos_ambiente (cliente_id, criado_em DESC);

CREATE TABLE cobranca_eventos (
  id        BIGSERIAL PRIMARY KEY,
  origem    TEXT NOT NULL,
  corpo     JSONB NOT NULL,
  tratado   BOOLEAN NOT NULL DEFAULT false,
  criado_em TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO planos (codigo, nome, descricao, usuarios_min, usuarios_max, preco_mensal_centavos, preco_anual_centavos, destaque, recursos, ordem) VALUES
 ('essencial', 'Essencial', 'Para equipes pequenas saindo da planilha.', 1, 3, 9900, 0, false,
  '["Até 3 usuários","Contatos, empresas e negócios em kanban","Tarefas, reuniões e timeline","Modelos de e-mail e envio pelo CRM","Formulários de captura","Dashboard de vendas","Suporte por e-mail"]', 1),
 ('profissional', 'Profissional', 'Para times de vendas que vivem de cadência.', 4, 10, 24900, 0, true,
  '["Tudo do Essencial","Até 10 usuários","Sequências de prospecção automáticas","Automações com gatilhos e esperas","Caixa de entrada compartilhada","Previsão, metas e relatórios","Permissões por perfil","Suporte prioritário"]', 2),
 ('escala', 'Escala', 'Para operações maiores, com várias equipes ou unidades.', 11, 30, 59900, 0, false,
  '["Tudo do Profissional","Até 30 usuários","Grupo de CNPJs (filiais e franquias)","Importações em massa com histórico","Relatórios customizáveis e painéis","Auditoria completa","Gerente de contas"]', 3),
 ('sob-medida', 'Sob medida', 'Acima de 30 usuários ou integrações específicas.', 31, 999, 0, 0, false,
  '["Tudo do Escala","Usuários conforme a operação","Implantação acompanhada"]', 4);

INSERT INTO configuracoes (chave, valor, descricao) VALUES
 ('landing.titulo',        'O CRM que a sua equipe realmente usa, com o seu domínio e os seus dados.', 'Título principal da landing'),
 ('landing.subtitulo',     'Contatos, negócios, sequências de e-mail e automações em um ambiente só seu — banco de dados isolado, e-mail com o seu domínio e pronto para usar em minutos.', 'Subtítulo da landing'),
 ('landing.dias_teste',    '14', 'Dias de teste grátis anunciados na landing'),
 ('landing.whatsapp',      '', 'WhatsApp comercial (só dígitos, com DDI)'),
 ('licenca.dias_aviso',    '15', 'Dias antes do vencimento para avisar'),
 ('fatura.dia_vencimento', '10', 'Dia do mês em que vencem as faturas geradas pela rotina');
