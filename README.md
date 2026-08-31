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
- **Usuários e permissões**: perfis `admin`, `manager` e `seller` com matriz de permissões configurável; convite com senha temporária por e-mail; redefinição de senha por link
- **Anexos**: arquivos em contatos, empresas, negócios e tickets (arrastar-e-soltar, até 10 MB)
- **Automações e sequências**: gatilho + ações encadeadas, com espera entre passos para virar sequência de e-mail
- **Formulários públicos**: construtor com endereço próprio e código de iframe; cada envio vira contato
- **Rastreio de e-mail**: aberturas e cliques dos envios feitos pelo CRM, com taxa por período
- **Duplicados**: encontra e mescla contatos e empresas repetidos
- **Agendamento**: link público com a agenda de cada pessoa; a reunião marcada entra no CRM
- **Relatórios**: montados pela equipe (entidade + métrica + agrupamento) em barras, linha, pizza ou tabela
- **Segurança**: webhook do Mandrill com assinatura HMAC, proteção contra força bruta no login, convite que expira em 7 dias com troca obrigatória de senha, perfil lido do banco a cada requisição e trilha de auditoria em Configurações → Auditoria

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

> **Não consegue logar?** O admin é criado apenas quando o banco está vazio — mudar `admin_password` depois não altera a senha existente. Para redefinir:
>
> ```bash
> go run application.go -reset-admin
> ```
>
> O comando aplica o `admin_email`/`admin_password` atuais do `.env` (reativa e garante papel admin) e sai.

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
| `mandrill_webhook_key` | Webhook key do Mandrill (Settings → Webhooks). **Sem ela o endpoint de inbound recusa toda requisição** |
| `mandrill_webhook_url` | URL do webhook exatamente como cadastrada no painel (entra no cálculo da assinatura) |
| `env`, `email_dev` | Fora de `production`, e-mails vão para `email_dev` |

## API

Base: `/api/v1`. Autenticação via `Authorization: Bearer <token>` (obtido em `POST /auth/login`).

Principais rotas: `auth/login`, `auth/forgot`, `auth/reset`, `me`, `users`, `contacts` (+ `import`/`export`), `companies`, `pipelines`, `stages`, `deals` (+ `board`, `stage`, `close`), `tasks` (+ `toggle`), `activities`, `emails` (+ `sent`, `stats`), `attachments`, `duplicates` (+ `merge`), `forms`, `booking`, `automations`, `reports`, `permissions`, `audit`, `dashboard`, `search`. Healthcheck em `GET /health`.

Rotas públicas (sem sessão, consumidas fora do CRM):

| Rota | Para quê |
| ---- | -------- |
| `GET/POST /api/public/forms/{slug}` | Formulário de captura embutido no site |
| `GET/POST /api/public/booking/{slug}` | Página de agendamento de reuniões |
| `GET /api/track/o/{token}/pixel.gif` | Pixel de abertura de e-mail |
| `GET /api/track/c/{token}?u=...` | Redirecionador que conta o clique |
| `POST /api/webhooks/mandrill/inbound` | E-mails de entrada (exige assinatura) |

### Automações

Um gatilho dispara uma lista ordenada de ações. Gatilhos por evento (contato criado, contato mudou de estágio, negócio criado/movido/ganho/perdido, ticket aberto, formulário enviado) rodam na hora, em goroutine. Gatilhos por tempo (negócio parado, tarefa atrasada, contato sem interação há X dias) são varridos de hora em hora pelo worker, no máximo uma vez por registro por dia.

Ações: enviar e-mail (modelo da biblioteca ou texto com `{{nome}}`, `{{empresa}}`…), criar tarefa, mudar dono, mover de etapa, mudar o estágio do contato, adicionar à lista, registrar observação, avisar alguém e **aguardar N dias**. A espera transforma a automação em sequência: o passo agenda a retomada em `sequence_enrollments` e o worker continua depois. Um erro em qualquer passo interrompe só aquele registro e fica no histórico.

### Relatórios

O construtor monta a consulta a partir de listas fechadas no código (entidade → métrica → agrupamento → filtros). Nada do que o usuário digita entra no SQL: só as chaves escolhidas, e os filtros vão como parâmetro. Formatos: barras, linha, pizza e tabela.

### Segurança

- **Login**: 5 tentativas erradas por IP+e-mail bloqueiam o par por 15 minutos (`429`). Um acesso bem-sucedido zera o contador.
- **Convite**: o usuário criado recebe senha temporária válida por 7 dias e entra com `must_change_password`. Enquanto não trocar, só `GET /me` e `GET /me/permissions` respondem — o resto devolve `403` com `must_change_password: true`. Depois do prazo o login devolve `403` e o caminho é o "Esqueci minha senha".
- **Perfil e status**: o middleware lê o usuário do banco (cache de 30s) em vez de confiar no papel gravado no token, então rebaixar, desativar ou trocar a senha de alguém vale na requisição seguinte, sem esperar as 12h do JWT. Trocar a senha invalida os tokens emitidos antes; a própria troca devolve um token novo.
- **Webhook do Mandrill**: `POST /api/webhooks/mandrill/inbound` exige `X-Mandrill-Signature` (HMAC-SHA1 da URL + campos ordenados, em base64) conferido com `mandrill_webhook_key`. Sem a chave configurada o endpoint recusa tudo.
- **Auditoria**: acessos, falhas de acesso, exclusões, exportações, importações, ações em massa e mudanças de permissão vão para `audit_log`, visíveis em Configurações → Auditoria (permissão `settings.audit`, só do Admin por padrão).
