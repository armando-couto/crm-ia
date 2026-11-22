# CRM IA — atalhos de operação. `make` ou `make ajuda` lista tudo.
SHELL := /bin/bash
.DEFAULT_GOAL := ajuda
COMPOSE     := docker compose
COMPOSE_DEV := $(COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml
VERSAO      ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)

.PHONY: ajuda deps env dev dev-logs dev-down dev-tenant up down logs ps images test test-go test-web publicar backup tenant lint

ajuda: ## mostra esta ajuda
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}'

deps: ## confere docker, compose, openssl, curl
	@bash infra/scripts/checar-dependencias.sh

env: ## cria o .env sorteando os segredos (não sobrescreve)
	@bash infra/scripts/gerar-env.sh .env

# ---- desenvolvimento ---------------------------------------------------------
dev: deps ## sobe a plataforma local em https://crmia.localhost (usa o PostgreSQL da máquina)
	@[ -f .env ] || bash infra/scripts/gerar-env.sh .env
	@bash infra/scripts/preparar-postgres-dev.sh
	@DOMAIN=crmia.localhost ACME_EMAIL=dev@crmia.localhost PGHOST=host.docker.internal $(COMPOSE_DEV) up -d --build
	@bash -c 'source .env; echo ""; echo "  landing  https://crmia.localhost$${HTTPS_PORT:+:$$HTTPS_PORT}/"; echo "  painel   https://crmia.localhost$${HTTPS_PORT:+:$$HTTPS_PORT}/painel  ($$PAINEL_ADMIN_EMAIL / $$PAINEL_ADMIN_SENHA)"; echo "  traefik  http://localhost:$${TRAEFIK_DASH_PORT:-8190}"; echo ""; echo "  próximo: make dev-tenant  (cria o cliente de exemplo \"acme\")"'

dev-tenant: ## cria e provisiona o cliente de exemplo (acme) no ambiente de dev
	@bash infra/scripts/dev-tenant.sh

dev-logs: ## segue os logs do painel e do provisionador
	@DOMAIN=crmia.localhost ACME_EMAIL=dev@crmia.localhost PGHOST=host.docker.internal $(COMPOSE_DEV) logs -f --tail=100 painel provisioner

dev-down: ## derruba a plataforma de dev (os ambientes de clientes ficam; remova-os pelo painel)
	@DOMAIN=crmia.localhost ACME_EMAIL=dev@crmia.localhost PGHOST=host.docker.internal $(COMPOSE_DEV) down

# ---- produção (docker compose num único nó) ----------------------------------
up: ## sobe/atualiza a plataforma com o .env de produção
	@$(COMPOSE) up -d --build --remove-orphans

down: ## para a plataforma (ambientes de clientes continuam)
	@$(COMPOSE) down

logs: ## segue os logs da plataforma
	@$(COMPOSE) logs -f --tail=100

ps: ## estado da plataforma e dos ambientes de clientes
	@$(COMPOSE) ps
	@echo ""; docker ps --filter label=crmia.tenant --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'

images: ## constrói todas as imagens (plataforma + app dos clientes + página de suspensão)
	@docker build -t crmia/landing:$(VERSAO) -t crmia/landing:latest ./landing
	@docker build -t crmia/painel:$(VERSAO) -t crmia/painel:latest ./painel
	@docker build -t crmia/provisioner:$(VERSAO) -t crmia/provisioner:latest ./provisioner
	@docker build -t crmia/suspenso:latest ./infra/suspenso
	@docker build --build-arg VERSAO=$(VERSAO) -t crmia/app:$(VERSAO) -t crmia/app:latest ./app

publicar: ## publica uma nova versão do app dos clientes (SALTO=patch|minor|major ou VERSAO=x.y.z)
	@bash infra/scripts/publicar-versao.sh

backup: ## backup de todos os bancos (clientes e painel)
	@bash infra/scripts/backup.sh

tenant: ## operação direta de um ambiente: make tenant ARGS="status acme"
	@bash infra/scripts/tenant.sh $(ARGS)

# ---- qualidade ---------------------------------------------------------------
test: test-go test-web ## roda todos os testes

test-go: ## testes Go (app, painel, provisioner). PG local liga os testes de integração.
	@cd app && go vet ./... && go test ./...
	@cd painel && go vet ./... && CRMIA_TEST_PG=$${CRMIA_TEST_PG:-postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable} go test ./...
	@cd provisioner && go vet ./... && go test ./...

test-web: ## testes e typecheck do frontend do app e da landing
	@cd app/web && npm ci --silent && npm run typecheck --silent && npm test --silent
	@cd landing && npm ci --silent && npm run typecheck --silent && npm test --silent

lint: ## gofmt e vet
	@test -z "$$(gofmt -l app painel provisioner)" || { gofmt -l app painel provisioner; exit 1; }
	@cd app && go vet ./... && cd ../painel && go vet ./... && cd ../provisioner && go vet ./...
