# Desenvolvimento

## Ferramentas

Go 1.26, Node 22, Docker com Compose v2, PostgreSQL 16 local (`localhost:5432`, `postgres`/`postgres`). `make deps` confere.

## Plataforma completa em Docker

```bash
make dev          # https://crmia.localhost  (painel, landing, provisionador)
make dev-tenant   # cria o cliente "acme" em https://crmia.localhost/acme
make dev-logs
make dev-down
```

Portas 80/443 ocupadas? Coloque `HTTP_PORT=8180` e `HTTPS_PORT=8543` no `.env`; os scripts respeitam. O Traefik de dev usa certificado autoassinado, por isso o aviso do navegador (e o `-k` do curl nos scripts).

O painel de dev aponta para o PostgreSQL **da máquina** via `host.docker.internal`; `infra/scripts/preparar-postgres-dev.sh` cria o papel e o banco `crmia_painel`. Os ambientes de clientes sempre sobem com o próprio PostgreSQL em container, até em dev.

## Cada parte sozinha

### App do cliente (`app/`)

```bash
cd app && cp .env.example .env            # DATABASE_URL, REDIS_URL (opcional), JWT_SECRET…
go run . &                                # API em :8080, migrations automáticas
cd web && npm install && npm run dev      # Vite em :5173 com proxy para :8080
```

Sem `REDIS_URL` o cache é em memória. Sem `web/dist` o Go serve só a API. Em dev o plugin do Vite injeta um tenant fictício (`VITE_TENANT_SLUG`, `VITE_TENANT_NOME`) para simular `BASE_PATH`.

Testes: `go test ./...` (os de integração exigem `CRMIA_TEST_DSN`), `cd web && npm run typecheck && npm test`.

Flags do binário: `-healthcheck` (usado pelo Docker), `-reset-admin` (redefine a senha de `ADMIN_EMAIL` com `ADMIN_SENHA` e sai; o provisionador usa para gerar a senha inicial).

### Painel (`painel/`)

```bash
cd painel && DATABASE_URL=postgres://postgres:postgres@localhost:5432/crmia_painel?sslmode=disable \
  SESSION_KEY=$(openssl rand -hex 32) CSRF_KEY=$(openssl rand -hex 32) ADMIN_EMAIL=admin@local ADMIN_SENHA=admin \
  PROVISIONER_URL=http://localhost:7998 PROVISIONER_TOKEN=dev FIXPAY_SIMULADO=true BASE_PATH=/painel go run .
```

Abre em `http://localhost:7894/painel`. Testes: `CRMIA_TEST_PG=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable go test ./...` (cria e destrói o banco `crmia_test_painel`).

### Provisionador (`provisioner/`)

Precisa do socket Docker e das imagens `crmia/app` e `crmia/suspenso` (`make images`). Testes usam um `docker` falso em `internal/tenant/testdata`, então `go test ./...` roda sem Docker.

### Landing (`landing/`)

```bash
cd landing && npm install && npm run dev   # :5174, /painel proxied para :7894
npm test && npm run build                  # build = typecheck + SPA + SSR + pré-render
```

A landing lê planos e textos de `GET /painel/api/planos` em tempo real e envia leads para `POST /painel/api/leads`. Sem painel, usa os planos padrão embutidos.

## Convenções

* Código, comentários, nomes de tabelas e mensagens em **português**; identificadores herdados do CRM original ficaram em inglês onde mudar não valia a pena.
* Nada de credencial no código: tudo por ambiente (`utils.Config` no app, `internal/config` no painel e no provisionador).
* Nunca mencionar o processador de pagamento em tela, e-mail ou documento voltado ao cliente. Há testes para a página de checkout e para a landing.
* Migrations do app em `app/migrations` (embutidas, numeradas); do painel em `painel/internal/banco/migracoes`.
* `gofmt` e `go vet` limpos; a CI falha se não.

## Release do app dos clientes

```bash
make publicar                 # patch
make publicar SALTO=minor
make publicar VERSAO=2.0.0
```

O script exige árvore limpa, roda os testes, constrói `crmia/app:X.Y.Z`, cria e envia a tag `vX.Y.Z` e registra a versão no painel (`POST /painel/api/interno/versoes`). A pipeline `deploy.yml` publica as imagens no GHCR a partir da tag. No painel, em **Versões**, a nova tag aparece; clientes com `auto_atualizar` recebem-na no job diário, os demais quando alguém clicar em "atualizar" na ficha do cliente. Voltar um cliente para uma versão anterior é escolher a tag antiga na mesma tela.
