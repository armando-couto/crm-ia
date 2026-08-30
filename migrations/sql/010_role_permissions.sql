-- Controle de acesso configurável: perfis Seller, Manager e Admin.
-- Renomeia os papéis antigos (pt) para os novos nomes e cria a matriz de
-- permissões editável pela equipe.

UPDATE users SET role = 'seller'  WHERE role = 'vendedor';
UPDATE users SET role = 'manager' WHERE role = 'gestor';

CREATE TABLE IF NOT EXISTS role_permissions (
    role       VARCHAR(20)  NOT NULL, -- seller | manager | admin
    permission VARCHAR(60)  NOT NULL,
    allowed    BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (role, permission)
);
