# CRM IA — app do cliente

API Go (Iris) + SPA Vue 3 entregues como uma imagem `crmia/app:<versão>`; cada cliente roda a sua, com PostgreSQL e Redis próprios, em `https://<dominio>/<slug>`.

## Ambiente

| Variável | Uso |
|---|---|
| `APP_ENV`, `PORT`, `APP_URL`, `BASE_PATH` | `production`/`development`; porta (8080); URL pública; prefixo (`/acme`) |
| `TENANT_SLUG`, `TENANT_NOME`, `TENANT_CNPJ`, `VERSAO` | identidade injetada na SPA e no `/health` |
| `DATABASE_URL`, `REDIS_URL` | PostgreSQL (obrigatório); Redis (sem ele, cache em memória) |
| `JWT_SECRET`, `CHAVE_CRIPTO` | sessões; AES-GCM dos segredos de integrações (64 hex) |
| `TOKEN_INTERNO`, `PAINEL_URL` | rotas `/api/interno/*` e proxy do "Meu plano" |
| `LICENCA_USUARIOS_MAX` | teto de usuários ativos |
| `ADMIN_NOME`, `ADMIN_EMAIL`, `ADMIN_SENHA` | admin inicial (`-reset-admin` redefine) |
| `TEMA_COR_PRIMARIA`, `TEMA_LOGO_URL` | identidade visual padrão (editável no CRM) |
| `MANDRILL_API_KEY`, `FROM_EMAIL`, `FROM_NAME`, `SMTP_*` | remetente padrão até o cliente configurar o dele |

## Rotas da API (`/api/v1`)

* **Autenticação**: `POST /auth/login`, `/auth/forgot`, `/auth/reset`, `GET /me`, `GET /me/workspace`.
* **Setup**: `GET /setup`, `GET /setup/templates[/{codigo}]`, `POST /setup/template`, `POST /setup/step`, `POST /setup/complete`.
* **Configurações**: `GET|PUT /settings/workspace`, `GET|PUT /settings/email` + `POST /settings/email/test`, `GET|PUT /settings/sending`, `GET /plano`, `POST /plano/solicitacoes`.
* **CRM**: contacts, companies, deals, pipelines, tasks, activities, notes, timeline, inbox, templates, snippets, sequences, automations, forms, reports, forecast, goals, imports, users, roles, audit (herdadas do CRM original).
* **Internas** (fora do `/api/v1`, em `/api/interno`, `Authorization: Bearer <TOKEN_INTERNO>`, excluídas da rota no Traefik): `GET /api/interno/uso`, `GET /api/interno/desempenho`, `POST /api/interno/admin/senha`.
* `GET /health` (raiz, sem autenticação) devolve `{status, versao, tenant}`.

## Desenvolvimento

```bash
cp .env.example .env && go run .            # :8080
cd web && npm install && npm run dev        # :5173 (proxy para :8080)
go test ./... && (cd web && npm run typecheck && npm test)
```

Build da imagem: `docker build --build-arg VERSAO=1.0.0 -t crmia/app:1.0.0 .`
