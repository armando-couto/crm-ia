-- Prospecção: cota mensal de leads por plano, uso por cliente e registro das
-- extrações (idempotência por pedido e lote). Contrato em docs/PROSPECCAO.md,
-- seção 4, com as correções de docs/PROSPECCAO-ADENDO.md.

ALTER TABLE planos ADD COLUMN prospeccao_leads_mes INT NOT NULL DEFAULT 0
  CHECK (prospeccao_leads_mes >= 0);

-- Valores iniciais por código. Código editado ou plano apagado: o UPDATE não
-- alcança nada e a migração segue (o plano fica com 0 até alguém ajustar).
UPDATE planos SET prospeccao_leads_mes = 1000  WHERE codigo = 'essencial';
UPDATE planos SET prospeccao_leads_mes = 5000  WHERE codigo = 'profissional';
UPDATE planos SET prospeccao_leads_mes = 20000 WHERE codigo = 'escala';
UPDATE planos SET prospeccao_leads_mes = 50000 WHERE codigo = 'sob-medida';

-- Uso por cliente e mês-calendário (competencia = dia 1 em America/Sao_Paulo).
-- leads: empresas entregues por /extracoes (é o que a cota desconta).
-- consultas: fichas avulsas por /cnpj no mês; consultas_dia e consultas_no_dia
-- sustentam o limite diário dessas fichas.
CREATE TABLE prospeccao_uso (
  cliente_id       BIGINT NOT NULL REFERENCES clientes(id) ON DELETE CASCADE,
  competencia      DATE   NOT NULL,
  leads            INT    NOT NULL DEFAULT 0 CHECK (leads >= 0),
  consultas        INT    NOT NULL DEFAULT 0 CHECK (consultas >= 0),
  consultas_dia    DATE,
  consultas_no_dia INT    NOT NULL DEFAULT 0,
  atualizado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (cliente_id, competencia)
);

-- Uma linha por lote entregue e cobrado. lote = sha256 (hex) dos CNPJs
-- normalizados, ordenados e unidos por vírgula. Repetir o mesmo pedido com o
-- mesmo lote não desconta de novo. competencia é a do primeiro (e único) débito.
CREATE TABLE prospeccao_extracoes (
  id          BIGSERIAL PRIMARY KEY,
  cliente_id  BIGINT NOT NULL REFERENCES clientes(id) ON DELETE CASCADE,
  pedido_id   TEXT   NOT NULL,
  lote        TEXT   NOT NULL,
  competencia DATE   NOT NULL,
  quantidade  INT    NOT NULL CHECK (quantidade > 0),
  criado_em   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (cliente_id, pedido_id, lote)
);
CREATE INDEX prospeccao_extracoes_mes_idx ON prospeccao_extracoes (cliente_id, competencia);

INSERT INTO configuracoes (chave, valor, descricao) VALUES
 ('prospeccao.cota_sem_licenca', '300', 'Leads de prospecção por mês para cliente sem licença alguma (0 desabilita)')
ON CONFLICT (chave) DO NOTHING;
