# Fix CRM

CRM da Fix Pay — substituto interno do HubSpot. Backend em **Go (Iris, MVC)**, frontend em **Vue 3**, banco **PostgreSQL** e e-mails transacionais via **Mandrill**. Em produção roda como **um único container Docker** (Ubuntu Server), com o banco em outro servidor.

## Funcionalidades

- **Contatos**: CRUD completo, busca, filtros por estágio/dono, estágios de ciclo de vida (lead → cliente), importação e exportação CSV
- **Empresas**: CRUD, vínculo com contatos e negócios
- **Negócios**: kanban com drag & drop, múltiplos pipelines com etapas configuráveis, probabilidade por etapa, ganho/perda, previsão ponderada
- **Tarefas**: ligação/e-mail/reunião/tarefa, prioridade, vencimento, atrasadas em destaque
- **Timeline**: notas, ligações, reuniões, e-mails e eventos do sistema por contato/empresa/negócio
- **E-mail**: envio ao contato direto do CRM via Mandrill, registrado na timeline
- **Dashboard**: funil de vendas, receita ganha por mês, ranking de vendedores, previsão ponderada
- **Busca global**: contatos, empresas e negócios em uma única busca
- **Usuários e permissões**: papéis `admin`, `gestor` e `vendedor`; convite com senha temporária por e-mail; redefinição de senha por link

## Stack

| Camada  | Tecnologia |
| ------- | ---------- |
| Backend | Go 1.24 + Iris v12 (MVC: `controllers/`, `models/`, `routes/`, `services/`, `middleware/`) |
| Frontend| Vue 3 + TypeScript + Pinia + Vue Router (SPA em `web/`, servida pelo próprio binário) |
| Banco   | PostgreSQL (migrations automáticas na subida, em `migrations/sql/`) |
| Auth    | E-mail + senha (bcrypt) com JWT HS256 |
| E-mail  | Mandrill (`github.com/keighl/mandrill`) |
| CI/CD   | GitHub Actions → Docker Hub (`fixpay/fix-crm`) → Docker Swarm |

## Desenvolvimento local

Pré-requisitos: Go 1.24+, Node 22+, PostgreSQL.

```bash
# 1. Configuração
cp .env.example .env          # ajuste host/user/senha do seu PostgreSQL local
createdb fixcrm               # crie o banco (as migrations rodam sozinhas na subida)

# 2. Backend (porta 6998 em desenvolvimento)
go run application.go

# 3. Frontend com hot reload (porta 5173, proxy /api -> 6998)
cd web && npm install && npm run dev
```

No primeiro acesso o sistema cria o usuário administrador com `admin_email` / `admin_password` do `.env`.

Alternativa com Docker (sobe PostgreSQL + app juntos):

```bash
cp .env.example .env.production   # use host=db, user/senha/dbname=fixcrm
docker compose up --build
```

## Testes

```bash
# Backend
go test ./...

# Frontend
cd web && npm test && npm run typecheck
```

Os testes do backend usam `go-sqlmock` (não precisam de banco). A pipeline `CI` roda tudo em cada push/PR na `main`.

## Deploy (produção)

1. Configure os secrets no GitHub (mesmo padrão dos demais projetos Fix Pay):
   - `ENV_PRODUCTION` — conteúdo do `.env.production` (ver `.env.example`)
   - `DOCKER_USERNAME` / `DOCKER_PASSWORD` — Docker Hub da Fix Pay
   - `HUB_USERNAME` / `ACCESS_TOKEN` — GitHub (módulos privados, se necessário)
   - `OVPN_CONFIG` / `OVPN_USERNAME` / `OVPN_PASSWORD` / `OVPN_CLIENT_KEY` — VPN
   - `SSH_HOST` / `SSH_USER` / `SSH_KEY` — servidor do Swarm
2. Crie a stack `fix-crm` no Swarm/Portainer usando `stack-portainer.yml` (serviço `fix-crm_app`).
3. Publique uma tag para disparar o build e o deploy:

```bash
git tag v1.0.0 && git push origin v1.0.0
```

O workflow `docker-publish.yml` roda os testes, builda o front e o binário, publica `fixpay/fix-crm` no Docker Hub e atualiza o serviço no Swarm via VPN + SSH.

## Variáveis de ambiente

Ver [.env.example](.env.example). No Linux o binário lê `.env.production`; nos demais sistemas, `.env` (padrão `goutils`).

| Chave | Descrição |
| ----- | --------- |
| `host`, `port_banco`, `user`, `password`, `dbname` | Conexão PostgreSQL |
| `port_server` | Porta HTTP (padrão: 6998 em desenvolvimento, 9000 em produção) |
| `app_url` | URL pública do CRM (links dos e-mails) |
| `jwt_secret` | Segredo do JWT (obrigatório) |
| `admin_email`, `admin_password` | Admin inicial (criado só com o banco vazio) |
| `mandrill`, `from` | Chave e remetente do Mandrill |
| `env`, `email_dev` | Fora de `production`, e-mails vão para `email_dev` |

## API

Base: `/api/v1`. Autenticação via `Authorization: Bearer <token>` (obtido em `POST /auth/login`).

Principais rotas: `auth/login`, `auth/forgot`, `auth/reset`, `me`, `users`, `contacts` (+ `import`/`export`), `companies`, `pipelines`, `stages`, `deals` (+ `board`, `stage`, `close`), `tasks` (+ `toggle`), `activities`, `emails`, `dashboard`, `search`. Healthcheck em `GET /health`.
