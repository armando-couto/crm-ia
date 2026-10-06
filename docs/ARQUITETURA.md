# Arquitetura

## Visão geral

```
                      ┌────────────────────────────── nó Docker ──────────────────────────────┐
 DNS crmia.com.br ──► │ Traefik :80/:443 (Let's Encrypt)                                       │
                      │   /            → landing   (nginx, HTML pré-renderizado)              │
                      │   /painel      → painel    (Go, chi, html/template)                   │
                      │   /<slug>      → ci-<slug>-app  (Go + Vue, um container por cliente)  │
                      │   /<slug>  (suspenso) → ci-<slug>-suspenso  (página estática, prio 200)│
                      │                                                                       │
                      │ rede crmia_public ── landing, painel, provisioner, ci-*-app, ci-*-susp │
                      │ rede crmia_interna (internal) ── painel ↔ provisioner ↔ postgres       │
                      │ rede ci-<slug>_interna (internal) ── ci-<slug>-app ↔ db ↔ redis        │
                      └───────────────────────────────────────────────────────────────────────┘
```

Três coisas separadas, de propósito:

* **Plataforma** (`docker-compose.yml` / `stack-portainer.yml`): Traefik, landing, painel, provisionador e o PostgreSQL do painel. Muda raramente.
* **Ambientes dos clientes**: uma stack Compose por cliente (`ci-<slug>`), gerada em tempo de execução pelo provisionador a partir de `infra/tenant/docker-compose.tenant.tmpl`. Cada uma tem `db`, `redis`, `app` e `suspenso`, volumes próprios e rede interna própria. Nenhum cliente enxerga o banco de outro; o banco não tem porta publicada.
* **Versões do app**: a imagem `crmia/app:<tag>` é a unidade de release. O painel mantém o catálogo de versões; cada cliente aponta para uma tag e pode estar em `auto_atualizar` ou fixo numa versão.

## Roteamento por caminho

O Traefik só enxerga containers com o rótulo `crmia.roteado=sim` (plataforma) ou `crmia.tenant=<slug>` (clientes), o que permite conviver com outros projetos na mesma máquina. Cada app de cliente é publicado com `PathPrefix(/<slug>)` + `stripprefix`, e a rota interna `/<slug>/api/interno` é excluída da regra para nunca ser alcançável pela internet. O container `suspenso` tem prioridade maior e só é iniciado quando o ambiente está suspenso.

O app sabe que vive em `/slug` por `BASE_PATH`: o backend serve a SPA injetando `__TENANT_BASE__`, nome, slug, cor primária e um JSON de identidade no `index.html` (`app/routes/spa.go`), e o Vue usa `createWebHistory(basePath())`, chaves de `localStorage` por tenant e tema por variável CSS (`app/web/src/tenant.ts`).

## Provisionador

Serviço Go sem dependências externas (`provisioner/`), autenticado por token Bearer, exposto apenas na rede interna. Para cada pedido:

1. valida o slug (`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`, lista de reservados: `painel`, `api`, `assets`, …);
2. gera ou reaproveita os segredos do cliente (senha do banco, `JWT_SECRET`, `CHAVE_CRIPTO`, `TOKEN_INTERNO`) em `/tenants/<slug>/segredos.json`;
3. renderiza o template Compose e roda `docker compose up -d` com a tag da versão pedida;
4. espera o healthcheck do app, reseta a senha do administrador (`crm-ia -reset-admin`) e devolve a senha inicial ao painel, que a mostra uma única vez.

Atualizar versão é trocar a tag e `up -d` (as migrations rodam na subida). Suspender sobe o `suspenso` e para o `app`; reativar faz o inverso. Backup usa `pg_dump` no container do banco para o volume `crmia_backups`. Remover exige confirmação com o slug e faz backup antes.

## Painel

Go + chi + pgx, HTML renderizado no servidor, sessão em cookie assinado (HMAC), CSRF por cookie duplo, bcrypt. Perfis: `admin`, `comercial`, `financeiro`, `suporte`, `leitura` — cada rota checa a permissão. Toda ação relevante vai para `auditoria`.

Entidades: `grupos` (o CNPJ pagador; filiais e franquias ficam no mesmo grupo), `clientes` (um ambiente cada; `status` pronto → provisionando → ativo ⇄ suspenso, ou erro/cancelado), `planos` (faixa de usuários e preço), `licencas` (grupo × plano × unidades × periodicidade, `cobranca` recorrente ou manual), `faturas`, `plano_solicitacoes` (upgrades pedidos pelo cliente), `versoes`, `leads`, `eventos_ambiente`, `cobranca_eventos`.

Rotinas (`internal/jobs`): **diária** às 6h marca licenças vencidas, gera faturas da competência, suspende por inadimplência depois de `DIAS_ATRASO_SUSPENSAO` e aplica a versão padrão nos clientes com `auto_atualizar`; **conferente** a cada 30 min consulta o relatório de cobranças da recorrência e baixa as faturas pagas. Detalhes em [COBRANCA.md](COBRANCA.md).

APIs públicas do painel (sem sessão): `GET /painel/api/planos` e `POST /painel/api/leads` para a landing, `GET|POST /painel/assinar/{codigo}` para o cadastro do cartão, `POST /painel/api/webhooks/recorrencia`. Internas (token): `POST /painel/api/interno/versoes` (pipeline), `GET /painel/api/empresas/{slug}/plano` e `POST .../plano/solicitacoes` (tela "Meu plano" do app, autenticada com o `TOKEN_INTERNO` do ambiente).

## App do cliente

Go 1.26 + Iris, PostgreSQL com migrations embutidas, Redis via go-redis (com fallback em memória para dev), JWT HS256, AES-GCM para segredos de integrações (chave SMTP/Mandrill do cliente), Vue 3 + TypeScript + Pinia + Vite.

O que a plataforma acrescentou ao CRM original:

* **Configuração por ambiente** (`utils.Config`): tudo vem do ambiente injetado pelo provisionador; nada de credenciais no código.
* **Assistente de configuração** (`/configurar`): empresa, modelo de CRM, e-mail e disparo. Até concluir, o router leva para lá. Os modelos (`services/catalogo.go`) criam funil, etapas, campos, modelos de e-mail e uma sequência inicial de forma idempotente.
* **E-mail do cliente**: Mandrill (API ou SMTP), Maileroo (SMTP) ou SMTP genérico, com STARTTLS obrigatório, teste de envio na tela e segredo cifrado no banco.
* **Regras de disparo**: janela de horário e dias da semana, teto diário (contado no Redis) e pausa geral. Sequências e automações consultam `EnvioAutomaticoPermitido` antes de enviar.
* **Licença**: `LICENCA_USUARIOS_MAX` limita a criação/reativação de usuários; **Meu plano** mostra uso e permite pedir upgrade (proxy para o painel).
* **Rotas internas** (`/api/interno/*`, token): uso, desempenho e redefinição da senha do admin, usadas pelo painel e pelo provisionador e bloqueadas no Traefik.

## Dados e segredos

| Onde | O quê |
|---|---|
| `.env` da plataforma | domínio, chaves do painel, token do provisionador, credenciais da recorrência e do Mandrill |
| volume `crmia_tenants_config` | `docker-compose.yml` gerado e `segredos.json` de cada cliente |
| volume `ci-<slug>_dados` | PostgreSQL do cliente |
| volume `crmia_backups` | dumps (`<slug>-<data>.sql.gz`) |
| banco `crmia_painel` | tudo do painel; tokens de assinatura ficam cifrados |

Rotação: o `JWT_SECRET` de um cliente pode ser trocado em `segredos.json` + reprovisionamento (derruba as sessões dele). `CHAVE_CRIPTO` **não** pode ser trocada sem recifrar as integrações; por isso o provisionador nunca a regenera para um cliente existente.
