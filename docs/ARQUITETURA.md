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
                      │ rede crmia_interna (internal) ── painel ↔ provisioner ↔ postgres ↔ dados│
                      │ rede crmia_saida ── só o serviço dados (internet e PostgreSQL do host) │
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

APIs públicas do painel (sem sessão): `GET /painel/api/planos` e `POST /painel/api/leads` para a landing, `GET|POST /painel/assinar/{codigo}` para o cadastro do cartão, `POST /painel/api/webhooks/recorrencia`. Internas (token): `POST /painel/api/interno/versoes` (pipeline), `GET /painel/api/empresas/{slug}/plano` e `POST .../plano/solicitacoes` (tela "Meu plano" do app, autenticada com o `TOKEN_INTERNO` do ambiente) e `.../empresas/{slug}/prospeccao/*` (prospecção, mesma autenticação). Tudo sob `/painel/api/empresas` é excluído da regra pública do Traefik: os ambientes chegam ao painel pela rede (`http://painel:7894`), nunca pela internet.

### Prospecção: cota, uso e a base pública

O app do cliente nunca fala com o serviço `dados`; fala com o painel, que autentica o ambiente, aplica a cota do plano, mede o uso e repassa. O contrato completo está em [PROSPECCAO.md](PROSPECCAO.md) (seção 4) e no [adendo](PROSPECCAO-ADENDO.md).

| Rota (`/painel/api/empresas/{slug}/prospeccao`) | Efeito | Limite por cliente |
|---|---|---|
| `GET ""` | resumo: `habilitada`, `motivo`, `cota` e situação da base | — |
| `GET /catalogos/{cnaes\|municipios\|naturezas\|ufs}` | repassa a consulta | 600/min |
| `POST /contagem` | repassa (grátis) | 600/min |
| `POST /previa` | repassa (grátis) | 60/min |
| `POST /candidatos` | repassa; exige `pedido_id` | 30/min |
| `POST /extracoes` | entrega as fichas e **desconta da cota** | a própria cota |
| `GET /cnpj/{cnpj}` | ficha avulsa (grátis; pessoa física sai sem contatos) | 60/min e 300/dia |

* **Cota** = `planos.prospeccao_leads_mes` do plano da licença vigente; na falta, o da licença mais recente ativa ou vencida; sem licença alguma, a configuração `prospeccao.cota_sem_licenca` (padrão 300, 0 desabilita). O mês vira à meia-noite de `America/Sao_Paulo`. Licença de grupo: cada CNPJ tem a cota inteira.
* **Extração idempotente:** `lote` = sha256 dos CNPJs normalizados e ordenados. O registro em `prospeccao_extracoes` (`UNIQUE (cliente_id, pedido_id, lote)`) e o débito condicional em `prospeccao_uso` são uma transação só; repetir o mesmo pedido com o mesmo lote entrega de novo sem descontar. Sem saldo para o lote, 429 e nada é entregue.
* **Erros** saem com `erro` e `error` (o app lê `error`) e com `codigo`: `cota` (429, com `cota`), `limite_requisicoes` (429, com `Retry-After` e `tentar_em_segundos`), `base_em_carga` (503), `base_atualizada` (409), `filtro_invalido` (422), `nao_encontrado` (404), `nao_autorizado` (401).
* Os limites por minuto ficam em memória (uma réplica; zeram no reinício).
* **O painel funciona sem o serviço:** sem `DADOS_TOKEN`, ou com o serviço fora do ar, o resumo responde `habilitada: false` com o motivo, as demais rotas respondem 503 e o resto do painel segue igual.
* Telas: **Base de dados** (situação, carga, botão "Verificar e carregar agora" para administrador e suporte, uso do mês por cliente), campo "Leads de prospecção por mês" no plano e a linha "Prospecção no mês" na ficha do cliente.

## Serviço de dados (`dados/`)

Container `crmia-dados`, porta 7896, banco `crmia_dados`. Baixa e carrega a base aberta do CNPJ (mensal) e a dívida ativa da PGFN (trimestral) e responde contagem, busca e ficha ao painel, com `Authorization: Bearer <DADOS_TOKEN>`.

* **Redes:** `crmia_interna` (o painel chama `http://dados:7896`) e `crmia_saida`, uma ponte comum só dele, porque a rede interna não tem caminho para a internet nem para `host.docker.internal`. Nunca entra em `crmia_public` e não tem rótulos do Traefik.
* **Onde é definido:** no `docker-compose.yml` (o `up --remove-orphans` remove serviço que só exista numa sobreposição). `docker-compose.servidor.yml` troca a `DATABASE_URL` para o PostgreSQL do host; `docker-compose.dev.yml` liga `DADOS_AMOSTRA` (empresas fictícias, nada é baixado) e publica a porta `DADOS_PORT` (7996); `stack-portainer.yml` tem o bloco equivalente.
* **Banco e papel** são criados por `infra/scripts/preparar-postgres-dev.sh` (servidor e desenvolvimento) e por `infra/postgres/bancos.sh` (Swarm). O serviço não depende de extensão; `pg_prewarm` é criada quando existe e a falta dela não é erro.
* **Volume** `crmia_dados_trabalho` em `/trabalho`: zips em download, apagados ao fim de cada carga.
* **Disco:** a carga mensal completa pede cerca de 70 GB livres no PostgreSQL (carga, WAL e duas versões da base lado a lado) e 8 GB no volume; depois ficam 12 a 13 GB. Em servidor pequeno use `DADOS_UFS` (ex.: `CE,PI`) ou `DADOS_CARGA_AUTOMATICA=false` e dispare a carga pelo painel ou por `make dados-carga`.
* **Variáveis:** `DADOS_TOKEN`, `DADOS_DB_NAME`, `DADOS_DB_USER`, `DADOS_DB_PASSWORD` (as duas secretas são sorteadas por `gerar-env.sh`), `DADOS_AMOSTRA`, `DADOS_CARGA_AUTOMATICA`, `DADOS_UFS`, `DADOS_DISCO_MINIMO_GB`, `DADOS_CONEXOES`, `DADOS_MEM_LIMIT`, `DADOS_CNPJ_URL`, `DADOS_PGFN_URL`. No painel: `DADOS_URL` (padrão `http://dados:7896`) e `DADOS_TOKEN`.
* Cada implantação que muda a imagem reinicia o container; a carga é retomável e erro de carga não derruba o serviço.

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
| `.env` da plataforma | domínio, chaves do painel, token do provisionador, token e senha do banco do serviço de dados, credenciais da recorrência e do Mandrill |
| volume `crmia_tenants_config` | `docker-compose.yml` gerado e `segredos.json` de cada cliente |
| volume `ci-<slug>_dados` | PostgreSQL do cliente |
| volume `crmia_backups` | dumps (`<slug>-<data>.sql.gz`) |
| banco `crmia_painel` | tudo do painel, inclusive cota e uso da prospecção; tokens de assinatura ficam cifrados |
| banco `crmia_dados` | base pública de empresas (dado aberto e recarregável: **não** entra no backup) |
| volume `crmia_dados_trabalho` | arquivos das fontes em download, apagados ao fim da carga |

Rotação: o `JWT_SECRET` de um cliente pode ser trocado em `segredos.json` + reprovisionamento (derruba as sessões dele). `CHAVE_CRIPTO` **não** pode ser trocada sem recifrar as integrações; por isso o provisionador nunca a regenera para um cliente existente.
