INSERT INTO pipelines (name, position)
SELECT 'Pipeline de Vendas', 0
WHERE NOT EXISTS (SELECT 1 FROM pipelines);

INSERT INTO pipeline_stages (pipeline_id, name, position, probability, is_won, is_lost)
SELECT p.id, s.name, s.position, s.probability, s.is_won, s.is_lost
FROM pipelines p,
     (VALUES ('Novo Lead', 0, 10, FALSE, FALSE),
             ('Contato Feito', 1, 25, FALSE, FALSE),
             ('Proposta Enviada', 2, 50, FALSE, FALSE),
             ('Negociação', 3, 75, FALSE, FALSE),
             ('Ganho', 4, 100, TRUE, FALSE),
             ('Perdido', 5, 0, FALSE, TRUE)) AS s(name, position, probability, is_won, is_lost)
WHERE p.name = 'Pipeline de Vendas'
  AND NOT EXISTS (SELECT 1 FROM pipeline_stages WHERE pipeline_id = p.id);
