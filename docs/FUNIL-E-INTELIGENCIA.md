# Funil visual e Inteligência de empresas — especificação

Duas funcionalidades do app do cliente (`app/`), desenhadas a partir de referências de mercado (Pipedrive, HubSpot, Salesforce Pipeline Inspection, Close, Attio para funil; HubSpot Breeze, Pipedrive/Surfe, Apollo e Attio para enriquecimento) e das APIs do BigDataCorp, Serasa Experian e fontes públicas de CNPJ.

Este documento é o **contrato** entre backend e frontend. Nomes de campos JSON em português, snake_case, exatamente como abaixo.

---

## Parte 1 — Funil de vendas visual (`/funil`)

### Objetivo

Uma tela que o gestor abre na reunião semanal e responde, em ordem: *estamos batendo a meta?* → *o que aconteceu no pipeline neste período?* → *onde o funil vaza e onde trava?* → *quais negócios estão em risco?* → *como está cada vendedor?* Tudo clicável: qualquer segmento filtra a lista de negócios abaixo.

### Conceitos e regras

* **Período**: `periodo` ∈ `7|30|90|180|365|mes|trimestre|ano` (dias corridos até hoje, ou calendário corrente). Default `30`.
* **Modo**:
  * `coorte` — negócios **criados** no período; o funil mostra quantos alcançaram cada etapa (funil "solto": pular etapa conta como alcançou, como Attio).
  * `fluxo` — o que **aconteceu** no período: criados, ganhos, perdidos, movimentados, independentemente da data de criação. Default `fluxo` (é o que se usa em reunião).
* **Estagnado (apodrecendo)**: negócio aberto cujo tempo na etapa atual > `prazo_dias` da etapa (`pipeline_stages.prazo_dias`; se nulo, usa o padrão do workspace, 14 dias). Regra do Pipedrive: o contador reinicia ao mudar de etapa.
* **Sem atividade**: `last_activity_at` (ou `created_at`) há mais de 7 dias.
* **Adiado**: `close_date` foi empurrada para frente pelo menos uma vez no período (`deal_changes.campo='close_date'` com `para > de`).
* **Vencido**: aberto com `close_date < hoje`.
* **Em risco** = qualquer um dos quatro acima. Cada negócio carrega `motivos: []string` com os que se aplicam.
* **Ponderado** = `amount × probability/100` da etapa atual (categoria de forecast não entra aqui; é a visão simples de pipeline).
* **Mediana de dias por etapa**: só negócios que **saíram** da etapa dentro do período (entrada em `deal_stage_history` seguida de outra entrada); mediana, não média.
* **Meta** do período: soma de `goals` dos donos filtrados para o mês corrente, proporcional aos dias quando o período não é o mês (se não houver meta, `meta = 0` e a cobertura não é exibida).
* **Cobertura** = `ponderado / max(meta − ganho, 0)`; semáforo: `< 2` vermelho, `2–3` âmbar, `> 3` verde.

### Rastro de mudanças (novo)

Tabela `deal_changes` (migração 027): toda alteração de `amount`, `close_date`, `stage_id`, `owner_id` e `status` gera uma linha `{deal_id, campo, de, para, user_id, created_at}` (valores como texto). Escrita em `UpdateDeal`, `MoveDealStage`, `CloseDeal` (que também passa a gravar `deal_stage_history`). É o que permite reconstruir o waterfall de qualquer período sem snapshot diário.

Reconstrução do **início do período** para um negócio: existia se `created_at < de`; estava aberto se `closed_at IS NULL OR closed_at >= de`; valor no início = `amount` atual revertendo as mudanças de `amount` com `created_at >= de` (de trás para frente).

### Endpoint

`GET /api/v1/funil?pipeline_id=&periodo=30&modo=fluxo&owner_id=` — permissão `deals.view`. Vendedor sem `forecast.view` vê só os próprios negócios (`owner_id` forçado para ele).

Resposta:

```json
{
  "pipeline": { "id": 1, "name": "Vendas", "stages": [ { "id": 1, "name": "Novo Lead", "position": 0, "probability": 10, "is_won": false, "is_lost": false, "prazo_dias": 7 } ] },
  "periodo": { "de": "2026-09-24", "ate": "2026-10-24", "modo": "fluxo", "rotulo": "Últimos 30 dias" },
  "resumo": {
    "abertos": 31, "valor_aberto": 148000.0, "ponderado": 61200.0,
    "ganhos": 8, "valor_ganho": 62000.0, "perdidos": 5, "valor_perdido": 21000.0,
    "taxa_ganho": 0.615, "ticket_medio": 7750.0, "ciclo_mediano_dias": 23,
    "meta": 150000.0, "gap": 88000.0, "cobertura": 0.7,
    "em_risco": 6, "valor_em_risco": 34000.0
  },
  "funil": [
    { "stage_id": 1, "nome": "Novo Lead", "alcancaram": 40, "valor": 310000.0,
      "conversao_total": 1.0, "conversao_etapa": 1.0, "mediana_dias": 3,
      "abertos_agora": 12, "valor_aberto": 60000.0, "ponderado": 6000.0, "estagnados": 2 }
  ],
  "movimentacao": {
    "inicio":   { "qtd": 28, "valor": 120000.0 },
    "criados":  { "qtd": 14, "valor": 70000.0 },
    "aumentos": { "qtd": 3,  "valor": 12000.0 },
    "reducoes": { "qtd": 2,  "valor": -8000.0 },
    "adiados":  { "qtd": 4,  "valor": 25000.0 },
    "ganhos":   { "qtd": 8,  "valor": -62000.0 },
    "perdidos": { "qtd": 5,  "valor": -21000.0 },
    "fim":      { "qtd": 31, "valor": 148000.0 }
  },
  "ganhos_por_semana": [ { "semana": "2026-09-28", "qtd": 2, "valor": 15000.0 } ],
  "por_dono": [
    { "owner_id": 3, "nome": "Ana", "abertos": 10, "valor_aberto": 50000.0, "ponderado": 20000.0,
      "ganhos": 3, "valor_ganho": 22000.0, "perdidos": 1, "taxa_ganho": 0.75, "em_risco": 2, "meta": 50000.0 }
  ],
  "negocios": [
    { "id": 10, "name": "ACME — expansão", "amount": 12000.0, "ponderado": 6000.0,
      "stage_id": 3, "stage_name": "Proposta", "status": "aberto",
      "owner_id": 3, "owner_name": "Ana", "company_id": 5, "company_name": "ACME",
      "close_date": "2026-11-10", "created_at": "2026-09-01T12:00:00Z", "closed_at": null,
      "dias_na_etapa": 19, "prazo_dias": 10, "dias_sem_atividade": 9, "adiamentos": 2,
      "variacao_7d": { "valor": -2000.0, "close_date_dias": 14, "etapa": "Negociação→Proposta" },
      "motivos": ["estagnado", "sem_atividade", "adiado"] }
  ]
}
```

`negocios` traz **todos** os negócios relevantes ao modo (abertos agora + ganhos/perdidos no período; em `coorte`, os criados no período), limitado a 1000, para o drill-down ser instantâneo no cliente. `variacao_7d` só vem quando houve mudança nos últimos 7 dias; `adiados.valor` no waterfall é informativo (valor dos negócios adiados), não entra na soma; a soma `inicio + criados + aumentos + reducoes + ganhos + perdidos = fim` deve fechar (ganhos/perdidos negativos porque saem do pipeline aberto).

### Board (kanban) — complemento

`GET /deals/board` passa a devolver em cada negócio `stage_entered_at`, `dias_na_etapa`, `prazo_dias` (da etapa) e `dias_sem_atividade`. O card mostra um selo "X d" (cinza), que vira âmbar ao atingir 70% do prazo e vermelho (borda esquerda vermelha + ícone) ao ultrapassar. O cabeçalho da coluna mostra `qtd · total · ponderado` e um contador de estagnados quando > 0. Filtro rápido "Em risco".

`pipeline_stages.prazo_dias INT NULL`: editável em Configurações → Pipelines ("prazo saudável nesta etapa, em dias").

### Tela `/funil` (PipelineInsightsView.vue)

Layout em "história", de cima para baixo, cada bloco é um `<section class="bloco">` com título e subtítulo explicativo:

1. **Barra de filtros fixa**: pipeline (select), período (segmented), modo (coorte/fluxo com tooltip explicando), dono (select; "Todos"), botão **Modo reunião**. Filtros refletidos na query string da URL (link compartilhável).
2. **Meta × realizado**: tiles grandes (Ganho, Ponderado, Aberto, Gap, Cobertura com semáforo, Taxa de ganho, Ticket médio, Ciclo mediano) + barra de progresso: ganho sólido, ponderado hachurado, marcador da meta; sparkline de ganhos por semana.
3. **Movimentação do pipeline (waterfall)**: barras flutuantes `inicio → criados → aumentos → reduções → ganhos → perdidos → fim`; cores: entrada teal, ganho verde, perdido vermelho, redução laranja, início/fim azul; rótulos com valor e quantidade; clique filtra a lista (ex.: clicar em "perdidos" mostra os perdidos).
4. **Funil por etapa**: trapézios horizontais proporcionais a `alcancaram` (ou a `abertos_agora` em um toggle "agora"), com `% conversão total` dentro e `% etapa→etapa` nas junções; sob cada etapa `mediana_dias` (destaque na mais lenta) e `estagnados` em vermelho. Clique filtra a lista pela etapa.
5. **Negócios em risco**: lista ordenada por valor com chips de motivo (estagnado/sem atividade/adiado/vencido), dias na etapa, última atividade, setas de variação em 7 dias (valor ↓ vermelho, data → laranja). Link para o negócio.
6. **Por vendedor**: tabela com barras inline (ponderado vs meta), ganhos, taxa, em risco; clique filtra.
7. **Lista (drill-down)**: tabela de `negocios` filtrada pelo segmento selecionado (chip "Filtrando por: Perdidos ✕"), colunas negócio, empresa, dono, valor, ponderado, etapa, dias na etapa, última atividade, fechamento.

**Modo reunião**: `document.documentElement.requestFullscreen()`, esconde a AppShell (classe `modo-reuniao` no `body`), fonte 1,25×, cada bloco vira um "slide" navegável por `←/→` e `Esc` sai; cabeçalho discreto com nome do pipeline, período e hora da última atualização; auto-refresh a cada 60 s.

Gráficos em **SVG inline** (componentes próprios em `web/src/components/charts/`: `FunnelChart.vue`, `WaterfallChart.vue`, `GoalBar.vue`, `Sparkline.vue`, `ChartTooltip.vue`), sem biblioteca. Cores por estado via variáveis CSS (`--ok`, `--danger`, `--warn`, `--ci-purple`, teal para entradas), nunca por etapa. Transição de entrada (`stroke-dashoffset`/`scaleX`) respeitando `prefers-reduced-motion`. Acessibilidade: `role="img"` + `<title>` por gráfico e tabela equivalente oculta para leitores de tela.

---

## Parte 2 — Inteligência de empresas (enriquecimento)

### Objetivo

"Hidratar" o lead: a partir do **CNPJ**, buscar tudo que é público (Receita Federal) de graça e, sob demanda e com custo visível, dados dos provedores pagos (BigDataCorp, Serasa Experian). Nunca sobrescrever o que o vendedor digitou sem ele ver; toda informação mostra a fonte e a data.

### Fontes

| Código | Fonte | Custo | O que traz |
|---|---|---|---|
| `receita` | Receita Federal via BrasilAPI (`https://brasilapi.com.br/api/cnpj/v1/{cnpj}`), fallback OpenCNPJ (`https://api.opencnpj.org/{cnpj}`) e CNPJ.ws pública (`https://publica.cnpj.ws/cnpj/{cnpj}`, 3 req/min) | grátis | razão social, fantasia, situação, abertura, CNAE principal e secundários, natureza, porte, capital, endereço completo, e-mail, telefones, Simples/MEI, QSA (CPF mascarado) |
| `cep` | BrasilAPI CEP v2 (`/api/cep/v2/{cep}`), fallback ViaCEP | grátis | normaliza endereço e traz lat/long |
| `bigdatacorp` | `POST https://plataforma.bigdatacorp.com.br/empresas` com headers `AccessToken` e `TokenId`, body `{"q":"doc{CNPJ}","Datasets":"...","Limit":1}` | por dataset (tabela abaixo) | indicadores de atividade (faixa de funcionários, faturamento presumido, nível de atividade, domínio/SSL/e-mail corporativo), telefones/e-mails/endereços validados, sócios com nível, presença digital, processos, KYC/sanções, mídia |
| `serasa` | `POST {base}/security/iam/v1/client-identities/login` (Basic client_id:secret → `accessToken`), depois `GET {base}/credit-services/business-information-report/v1/reports?reportName=RELATORIO_AVANCADO_PJ_PME&optionalFeatures=...` com o CNPJ | por contrato | score positivo, anotações negativas (pendências, protestos, cheques), consultas recentes, limite de crédito, participações. **Schema de resposta não é público**: o parser é tolerante (procura chaves conhecidas, guarda o bruto) e a tela mostra o que achou |

Datasets BigDataCorp e preço de referência (1ª faixa): `basic_data` R$ 0,03 · `registration_data` R$ 0,14 · `activity_indicators` R$ 0,06 · `relationships` R$ 0,04 · `phones_extended` R$ 0,06 · `emails_extended` R$ 0,06 · `addresses_extended` R$ 0,06 · `domains_extended` R$ 0,06 · `processes` R$ 0,08 · `kyc` R$ 0,06 · `media_profile_and_exposure` R$ 0,10. Conjunto padrão sugerido: `basic_data,registration_data,activity_indicators,relationships` (≈ R$ 0,27). Preços ficam numa tabela no código, editáveis nas configurações (`precos_centavos`), só para **estimativa**.

Rate limits: BrasilAPI sem número publicado (tratar 429 com fallback), CNPJ.ws 3/min, BigDataCorp 5.000/5 min. Cache da consulta pública por CNPJ: 30 dias em `utils.Cache` e na própria tabela `company_enrichments` (última consulta `receita` com menos de 30 dias é reaproveitada sem bater na API, salvo `forcar=true`).

Modo **demonstração** por provedor pago (`demo: true` nas configurações): devolve um perfil fictício, determinístico a partir do CNPJ, **sempre rotulado "dados de demonstração"** na resposta (`demo: true`) e na tela. Serve para apresentar o recurso antes de ter contrato.

### Dados (migração 027)

Colunas novas em `companies`:

```sql
legal_name VARCHAR(255), trade_name VARCHAR(255),
cnae_code VARCHAR(10), cnae_description VARCHAR(255), legal_nature VARCHAR(120),
founded_at DATE, capital NUMERIC(16,2), company_size VARCHAR(30),
registration_status VARCHAR(30), registration_status_at DATE,
simples BOOLEAN, mei BOOLEAN,
street VARCHAR(255), street_number VARCHAR(20), complement VARCHAR(120),
neighborhood VARCHAR(120), zip VARCHAR(10), email VARCHAR(255), website VARCHAR(255),
employees_range VARCHAR(40), revenue_range VARCHAR(60),
intel JSONB NOT NULL DEFAULT '{}'::jsonb, enriched_at TIMESTAMPTZ
```

`city`, `state`, `phone`, `domain`, `industry`, `cnpj` já existem. `industry` recebe a descrição do CNAE principal quando vazio.

```sql
CREATE TABLE company_enrichments (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  provedor VARCHAR(20) NOT NULL,          -- receita|bigdatacorp|serasa
  status VARCHAR(20) NOT NULL,            -- ok|sem_resultado|erro
  mensagem TEXT,
  datasets TEXT,                          -- bigdatacorp: lista pedida
  demo BOOLEAN NOT NULL DEFAULT FALSE,
  custo_centavos INT NOT NULL DEFAULT 0,
  perfil JSONB NOT NULL DEFAULT '{}'::jsonb,  -- normalizado (Perfil)
  bruto JSONB,                            -- resposta original (sem segredos)
  aplicado_em TIMESTAMPTZ,
  campos_aplicados TEXT[],
  user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON company_enrichments (company_id, created_at DESC);

CREATE TABLE company_field_sources (
  company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  campo VARCHAR(60) NOT NULL,
  provedor VARCHAR(20) NOT NULL,
  enrichment_id BIGINT REFERENCES company_enrichments(id) ON DELETE SET NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (company_id, campo)
);
```

Configuração em `app_settings` chave `enrichment`:

```json
{
  "auto_receita": true,
  "sobrescrever": "vazios",
  "limite_mensal_centavos": 20000,
  "prazo_cache_dias": 30,
  "bigdatacorp": { "habilitado": false, "demo": false, "token_id": "", "access_token_cifrado": "", "datasets_padrao": ["basic_data","registration_data","activity_indicators","relationships"], "precos_centavos": { "basic_data": 3, "registration_data": 14, "activity_indicators": 6, "relationships": 4, "phones_extended": 6, "emails_extended": 6, "addresses_extended": 6, "domains_extended": 6, "processes": 8, "kyc": 6, "media_profile_and_exposure": 10 } },
  "serasa": { "habilitado": false, "demo": false, "ambiente": "uat", "client_id": "", "client_secret_cifrado": "", "report_name": "RELATORIO_AVANCADO_PJ_PME", "optional_features": "", "preco_centavos": 1500 }
}
```

Segredos cifrados com `utils.Cifrar`; a API pública devolve `has_secret` em vez do valor (mesmo padrão de `EmailSettings`).

### Perfil normalizado (`services/enriquecimento.Perfil`)

```json
{
  "provedor": "receita", "demo": false, "consultado_em": "2026-10-24T12:00:00Z",
  "campos": {
    "legal_name": "ACME VENDAS LTDA", "trade_name": "ACME", "name": "ACME",
    "cnae_code": "6201-5/01", "cnae_description": "Desenvolvimento de programas...", "industry": "...",
    "legal_nature": "Sociedade Empresária Limitada", "founded_at": "2015-03-10", "capital": 100000,
    "company_size": "ME", "registration_status": "ATIVA", "registration_status_at": "2015-03-10",
    "simples": true, "mei": false,
    "street": "Rua X", "street_number": "100", "complement": "", "neighborhood": "Centro",
    "zip": "60000000", "city": "Fortaleza", "state": "CE",
    "phone": "(85) 3333-4444", "email": "contato@acme.com.br", "website": "https://acme.com.br", "domain": "acme.com.br",
    "employees_range": "10 a 49", "revenue_range": "R$ 1M a 5M"
  },
  "socios": [ { "nome": "FULANO", "documento": "***123456**", "qualificacao": "Sócio-Administrador", "desde": "2015-03-10", "tipo": "PF" } ],
  "cnaes_secundarios": [ { "codigo": "...", "descricao": "..." } ],
  "indicadores": { "nivel_atividade": 0.82, "tem_dominio_ativo": true, "tem_ssl": true, "tem_email_corporativo": true, "funcionarios_faixa": "10 a 49", "faturamento_faixa": "R$ 1M a 5M" },
  "contatos": { "telefones": [ { "numero": "+5585999990000", "tipo": "mobile", "principal": true, "validado": true } ], "emails": [ { "email": "...", "tipo": "corporativo", "validado": true } ] },
  "presenca_digital": { "dominios": [ { "dominio": "acme.com.br", "ativo": true, "ssl": true } ], "redes": { "instagram": "...", "linkedin": "..." } },
  "risco": { "score": 720, "faixa": "B", "nivel": "baixo", "pendencias_qtd": 0, "pendencias_valor": 0, "protestos_qtd": 0, "cheques_sem_fundo": 0, "acoes_qtd": 0, "consultas_90d": 3, "limite_sugerido": 50000, "observacao": "" },
  "processos": { "total": 2, "ativos": 1, "como_reu": 1, "valor_total": 35000, "ultimos": [ { "numero": "...", "tipo": "Cível", "tribunal": "TJCE", "status": "Ativo", "polo": "Réu", "valor": 20000, "data": "2026-01-10" } ] },
  "conformidade": { "pep": false, "sancionado": false, "listas": [] },
  "midia": { "exposicao": "baixa", "noticias": [ { "titulo": "...", "url": "...", "sentimento": "neutro", "data": "..." } ] },
  "sinais": [ { "tipo": "situacao", "nivel": "ok", "texto": "Situação cadastral ativa desde 2015" } ],
  "custo_centavos": 27
}
```

`sinais` é calculado pelo backend a partir do que existe (situação cadastral, idade da empresa, porte, atividade digital, risco, processos, conformidade); níveis `ok|atencao|risco`. É o resumo que aparece no topo da aba Inteligência.

### Endpoints (todos em `/api/v1`)

| Método e caminho | Permissão | Descrição |
|---|---|---|
| `GET /cnpj/{cnpj}` | `companies.edit` | Consulta pública (receita + cep) para pré-preencher o formulário. Resposta: `{ perfil: Perfil, existente: { id, name } \| null }` (avisa se já há empresa com o CNPJ). |
| `GET /companies/{id}/intel` | `companies.view` | `{ company: Company, intel: Perfil consolidado, fontes: { campo: { provedor, fetched_at } }, historico: [ { id, provedor, status, demo, custo_centavos, datasets, user_name, created_at, aplicado_em } ], uso_mes: { consultas, custo_centavos, limite_centavos } }`. O `intel` consolidado é a última consulta ok de cada provedor mesclada (receita como base; pagos sobrepõem indicadores/risco/processos/conformidade/midia/contatos). |
| `POST /companies/{id}/enrich/preview` | `companies.enrich` | Body `{ provedores: ["receita","bigdatacorp","serasa"], datasets?: [..], forcar?: bool }`. Executa as consultas (respeitando cache e limite mensal), grava `company_enrichments` com `aplicado_em = NULL` e devolve `{ enrichment_ids: [..], consultas: [ { provedor, status, mensagem, demo, custo_centavos, cache: bool } ], perfil: Perfil mesclado, diff: [ { campo, label, atual, proposto, provedor, sugerido: bool } ], custo_centavos }`. `sugerido` = true quando o campo atual está vazio (ou `sobrescrever = "sempre"`) e o proposto é diferente. |
| `POST /companies/{id}/enrich/apply` | `companies.enrich` | Body `{ enrichment_ids: [..], campos: ["legal_name", ...] }`. Aplica só os campos listados (validando contra o diff recalculado), grava `company_field_sources`, `intel` (perfil mesclado), `enriched_at`, marca `aplicado_em`/`campos_aplicados`, registra atividade de sistema na timeline da empresa ("Dados enriquecidos pela Receita Federal: 7 campos") e auditoria. Resposta: `{ company: Company, aplicados: [..] }`. |
| `POST /companies/enrich/bulk` | `companies.enrich` | Body `{ ids: [..≤100], provedores: ["receita"] }`. Só `receita` em massa (grátis); aplica automaticamente com a regra `sobrescrever`. Resposta `{ processados, enriquecidos, sem_cnpj, erros: [ { id, mensagem } ] }`. |
| `DELETE /companies/{id}/intel` | `companies.enrich` | Remove `intel`, fontes e histórico (LGPD / limpeza). Os campos já aplicados ficam. |
| `GET /settings/enrichment` | `settings.integrations` | `{ settings: (sem segredos, com has_secret), fontes: [catálogo com nome, descrição, custo, datasets disponíveis e preços], uso_mes }`. |
| `PUT /settings/enrichment` | `settings.integrations` | Mesmo objeto + `bigdatacorp.access_token`, `serasa.client_secret` em claro (vazio mantém). |
| `POST /settings/enrichment/test` | `settings.integrations` | Body `{ provedor, cnpj? }` — faz uma consulta mínima (BDC: `basic_data`; Serasa: login) e devolve `{ ok, mensagem, custo_centavos, demo }`. |
| `GET /settings/enrichment/usage` | `settings.integrations` | `{ meses: [ { mes: "2026-10", consultas, custo_centavos, por_provedor: {..} } ] }`. |

Criação/edição de empresa (`POST/PUT /companies`) aceita os campos novos. Quando `auto_receita = true` e a empresa é criada com CNPJ válido e sem `legal_name`, o backend consulta a Receita de forma síncrona (timeout 6 s; falha silenciosa) e preenche os campos vazios, registrando fontes.

Erros: provedor não configurado → 422 `{ "error": "BigDataCorp não configurado. Peça ao administrador para ativar em Configurações → Integrações." }`; limite mensal atingido → 429; CNPJ inválido → 422; falha externa → 502 com mensagem amigável; nunca vazar tokens em logs ou respostas.

### Frontend

**Ficha da empresa (`CompanyDetailView.vue`)**

* Ação rápida **Enriquecer** (ícone ✦). Desabilitada com tooltip "Informe o CNPJ para enriquecer" quando não há CNPJ válido.
* Modal em três passos: (1) escolher fontes — cards com nome, o que traz, custo estimado ("≈ R$ 0,27 · 4 conjuntos" / "grátis" / "demonstração"), uso do mês; (2) resultado — tabela de diff: campo, valor atual, valor proposto (com badge da fonte), checkbox marcado quando `sugerido`; botões "Marcar só vazios" / "Marcar todos"; (3) resumo "7 campos atualizados pela Receita Federal". Erros por fonte aparecem inline sem bloquear as outras.
* Nova aba **Inteligência**: no topo os `sinais` como chips coloridos; depois cartões: Cadastro (razão social, fantasia, situação, abertura, natureza, porte, capital, Simples/MEI, CNAE + secundários em `<details>`), Endereço (com link para o mapa), Contatos encontrados (telefones/e-mails com validado ✓ e botão "usar"), Sócios e administradores (nome, qualificação, desde; CPF mascarado como vem), Atividade e porte (indicadores com barrinha de nível), Presença digital, Risco de crédito (score em arco, faixa, pendências/protestos/ações, consultas 90 d, limite sugerido; só se houver fonte paga), Processos, Conformidade (PEP/sanções), Mídia. Cada cartão mostra "Fonte · data" no rodapé. Estado vazio da aba: explicação + botão Enriquecer. Rodapé com "Histórico de consultas" e botão "Remover dados de inteligência".
* Em "Sobre essa empresa", campos preenchidos por enriquecimento ganham um pequeno ✦ com tooltip "Receita Federal · 24/10/2026" (dados de `fontes`).
* Faixa "dados de demonstração" (âmbar) sempre que `demo = true` em qualquer fonte usada.

**Formulário de empresa (criar/editar em `CompaniesView.vue` e no modal da ficha)**: ao sair do campo CNPJ com 14 dígitos válidos, chama `GET /cnpj/{cnpj}`; se achar, preenche os campos vazios (nome fantasia → nome, razão social, cidade/UF, telefone, e-mail, CNAE → segmento, endereço) e mostra o chip "Preenchido pela Receita Federal — revise antes de salvar"; se já existir empresa com o CNPJ, mostra aviso com link para ela. Máscara `00.000.000/0000-00` e validação de dígitos verificadores no cliente.

**Lista de empresas**: ação em massa "Enriquecer pela Receita" para a seleção (≤100), com barra de progresso e resumo.

**Configurações → Integrações (`SettingsIntegrationsView.vue`, rota `/configuracoes/integracoes`, permissão `settings.integrations`)**: três cartões de fonte no padrão de `SettingsEmailView.vue` (Receita Federal: sempre ativa, toggle "preencher automaticamente ao criar"; BigDataCorp: Token ID, Access Token (mantido se vazio), datasets padrão com preço, botão Testar; Serasa: ambiente, client id/secret, reportName, optionalFeatures, Testar), regras (sobrescrever vazios/sempre; limite mensal em R$), e um painel "Uso neste mês" (consultas e custo por fonte, barra até o limite). Toggle "Modo demonstração" dentro de cada fonte paga, com aviso claro.

---

## Permissões novas

* `companies.enrich` — "Enriquecer empresas (consultas externas)" — grupo Empresas. Padrão: Seller e Manager.
* `settings.integrations` — "Configurar integrações de dados" — grupo Configurações. Padrão: Manager. Entra em `auth.canManage`.

## Navegação

* Menu principal, seção Vendas: **Funil** (`/funil`, `deals.view`), logo depois de Negócios.
* Configurações → Empresa: **Integrações de dados** (`/configuracoes/integracoes`).

## Testes mínimos

* Go (sqlmock): cálculo do funil em coorte e fluxo com histórico sintético; waterfall fecha a conta; motivos de risco; reconstrução do valor no início do período; parser BrasilAPI/OpenCNPJ/CNPJ.ws → Perfil; parser BigDataCorp (fixture de resposta); diff e aplicação respeitando `sobrescrever`; limite mensal; demo determinístico; segredos nunca saem na API.
* Vitest: `/funil` renderiza blocos com dados mockados, clique em segmento filtra a lista, modo reunião alterna classe; modal de enriquecimento mostra diff e envia só os campos marcados; formulário de empresa pré-preenche pelo CNPJ; configurações de integrações salvam sem reenviar segredo.
