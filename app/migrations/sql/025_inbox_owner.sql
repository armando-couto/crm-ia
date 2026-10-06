-- Caixa de entrada: dono da conversa, para as filas "Não atribuído" e
-- "Atribuído a mim". Comentários internos usam direction = 'comentario' na
-- tabela de mensagens que já existe.
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS owner_id BIGINT
    REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_conversations_owner ON conversations (owner_id, status);
