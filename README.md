# CRM IA

Plataforma de CRM vendida por assinatura, com **um ambiente isolado por cliente**: cada empresa tem o próprio PostgreSQL, o próprio Redis e a própria instância da aplicação (Go + Vue), no endereço `https://<dominio>/<slug>`, com versão controlada individualmente.

```
https://crmia.com.br/            landing comercial (Vue, pré-renderizada)
https://crmia.com.br/painel      painel administrativo (Go) — clientes, licenças, cobrança, versões
https://crmia.com.br/acme        CRM da empresa "acme"  (PostgreSQL + Redis + app próprios)
https://crmia.com.br/outra       CRM da empresa "outra" (…)
```

| Pasta | O que é |
|---|---|
| `app/` | O CRM entregue a cada cliente: API Go (Iris) + SPA Vue 3. Imagem `crmia/app:<versão>`. |
| `painel/` | Painel administrativo: grupos de CNPJ, clientes, planos, licenças, faturas, cobrança recorrente no cartão, versões, leads, auditoria. |
| `provisioner/` | Serviço interno que cria, atualiza, suspende, faz backup e remove os ambientes dos clientes (Docker Compose via socket). |
| `landing/` | Site comercial, estático e pré-renderizado, com planos e formulário de contato lidos do painel. |
| `infra/` | Traefik, página de suspensão, imagens de PostgreSQL, template da stack de cada cliente e scripts de operação. |
| `docs/` | [Arquitetura](docs/ARQUITETURA.md), [cobrança](docs/COBRANCA.md), [desenvolvimento](docs/DESENVOLVIMENTO.md), [Swarm/Portainer](docs/swarm.md). |

## Subir em desenvolvimento

Pré-requisitos: Docker com Compose v2, um PostgreSQL local em `localhost:5432` (usuário `postgres`/`postgres`) e, para rodar fora do Docker, Go 1.26 e Node 22.

```bash
make dev
```

Isso gera o `.env` com segredos sorteados, prepara o banco `crmia_painel` no PostgreSQL da máquina e sobe Traefik, landing, painel e provisionador em `https://crmia.localhost` (certificado autoassinado). A senha inicial do painel é impressa no terminal e fica em `PAINEL_ADMIN_SENHA` no `.env`.

Se as portas 80/443 já estiverem em uso, defina `HTTP_PORT` e `HTTPS_PORT` no `.env` antes.

```bash
make dev-tenant
```

Cria o cliente de exemplo **acme** pelo painel e provisiona o ambiente dele em `https://crmia.localhost/acme`. A cobrança roda em modo simulado: o cartão `4111 1111 1111 1111` aprova e qualquer cartão terminado em `0002` recusa.

## Produção

1. `bash infra/scripts/gerar-env.sh .env.production` e preencha domínio, e-mail do Let's Encrypt, credenciais da recorrência e do Mandrill.
2. Num único nó Docker: copie o repositório, renomeie para `.env` e rode `make up`. Em Swarm/Portainer, use `stack-portainer.yml` ([docs/swarm.md](docs/swarm.md)).
3. Publique o app dos clientes com `make publicar` (cria a tag `vX.Y.Z`; a pipeline envia as imagens e registra a versão no painel).

## Fluxo comercial

1. O lead pede o teste na landing (ou o comercial cadastra direto no painel).
2. No painel: grupo (CNPJ pagador) → cliente (slug, modelo de CRM, admin) → **Provisionar**. Em um ou dois minutos o ambiente está no ar e a senha inicial do administrador é exibida uma única vez.
3. O cliente entra, troca a senha e segue o **assistente de configuração**: identidade da empresa, modelo (vendas B2B, serviços locais, imobiliária, varejo, agência ou em branco), provedor de e-mail (Mandrill ou Maileroo) e regras de disparo.
4. O comercial contrata a licença; o cliente recebe um link com a marca CRM IA para cadastrar o cartão. A mensalidade é cobrada automaticamente; o painel baixa as faturas, suspende por inadimplência após a tolerância e reativa quando o pagamento entra.
5. Dentro do CRM, em **Meu plano**, o cliente vê o plano, o uso de usuários e pede upgrade; o painel aprova e o limite de usuários do ambiente muda sem reprovisionar.

## Testes

```bash
make test
```

Roda `go vet` e `go test` nos três módulos Go (os testes de integração do painel usam o PostgreSQL local, se houver) e typecheck + Vitest no frontend do app e na landing.
