-- Temperatura do negócio (fria | media | quente), como a propriedade
-- "Temperatura do Deal" usada pela equipe no HubSpot.

ALTER TABLE deals ADD COLUMN IF NOT EXISTS temperature VARCHAR(10);
