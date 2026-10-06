# Docker Swarm / Portainer

`stack-portainer.yml` sobe a plataforma como stack Swarm num **único nó manager**, com as imagens já publicadas (GHCR) e o PostgreSQL do painel como serviço.

## Por que um nó só

Os ambientes dos clientes são criados pelo provisionador com `docker compose` no **socket do nó onde ele roda**, fora do Swarm. Isso dá a cada cliente uma stack pequena e independente, com volume local, versão própria e rede interna própria, sem o custo de um serviço Swarm por cliente. O preço é que app, Traefik e provisionador precisam estar no mesmo nó (restrição `node.role == manager`). Para crescer horizontalmente, o caminho é um segundo nó com a plataforma inteira e clientes distribuídos por DNS, não um Swarm multi-nó.

## Passos

1. `docker swarm init` no servidor; DNS do domínio apontando para ele; portas 80/443 livres.
2. `bash infra/scripts/gerar-env.sh .env.production` e preencha o restante. Deixe `PGHOST`, `PGPORT` e `PGSSLMODE` vazios para usar o `postgres` da stack.
3. Publique as imagens (tag `vX.Y.Z` → `deploy.yml`) ou construa no servidor com `make images` e ajuste `IMAGEM_*`/`TENANT_IMAGE_*` no `.env`.
4. Se as imagens forem privadas, `docker login ghcr.io` no nó e preencha `REGISTRY_USER`/`REGISTRY_TOKEN` (o provisionador faz `docker login` antes do `pull` dos clientes).
5. Suba:

```bash
set -a; source .env.production; set +a
docker stack deploy -c stack-portainer.yml crmia --with-registry-auth
```

   No Portainer: Stacks → Add stack → Web editor, cole o arquivo, em *Environment variables* use *Load variables from .env file*.

6. Entre em `https://<dominio>/painel` com `PAINEL_ADMIN_EMAIL`/`PAINEL_ADMIN_SENHA`, cadastre a primeira versão em **Versões** (ou rode `make publicar`) e crie o primeiro cliente.

## Atualizar a plataforma

`docker service update --image ghcr.io/<owner>/crmia-painel:X.Y.Z crmia_painel` (idem provisioner e landing), ou `docker stack deploy` de novo com as variáveis `IMAGEM_*` apontando para a nova tag. Os ambientes dos clientes não são afetados: a versão deles é gerida pelo painel.

## Backups

`make backup` (ou um cron chamando `infra/scripts/backup.sh`) gera `pg_dump` de cada cliente e do painel no volume `crmia_backups`. Copie esse volume para fora do servidor; o restante (configs e segredos dos clientes) está em `crmia_tenants_config`. Restaurar um cliente: `gunzip -c <dump> | docker exec -i ci-<slug>-db psql -U <usuario> <banco>` com o app parado.

## Rede

* `crmia_public` (overlay, attachable): Traefik alcança a plataforma e os containers dos clientes, que o provisionador conecta à mesma rede pelo nome.
* `crmia_interna` (overlay, internal): painel ↔ provisionador ↔ postgres.
* `ci-<slug>_interna` (bridge, internal): app ↔ db ↔ redis de cada cliente.

O Traefik usa dois providers: `swarm` para os serviços da stack (rótulos em `deploy.labels`) e `docker` para os containers dos clientes (rótulo `crmia.tenant`). As middlewares vêm de `infra/traefik/dynamic` montadas como `configs`.
