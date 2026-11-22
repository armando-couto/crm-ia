# Prospecção: gerar listas, enriquecer, entregar leads e ligar

Especificação e contrato do fluxo que leva o vendedor de "não tenho para quem ligar" a "reunião marcada":

```
 base pública            lista de prospecção           enriquecimento          caixa de leads            fila de ligações
 (CNPJ + dívida ativa) → (filtros, contagem, prévia) → (Who Lead / Plus)   →   (distribuída entre    →   (ficha pronta, resultado em
  serviço `dados`          guardada por cliente          telefones, decisores     os vendedores)           um clique, reunião → negócio)
```

Referências usadas no desenho: EmpresAqui, Casa dos Dados, Econodata, Speedio e Leads2b (gerador de listas); Who Lead / Plus Soluções (enriquecimento); Pipedrive (caixa de leads, atividade seguinte, semáforo de urgência, conversão em negócio). Os relatórios de pesquisa completos ficaram fora do repositório; o que importa deles está aqui e em [MELHORIAS-PIPEDRIVE.md](MELHORIAS-PIPEDRIVE.md).

Este documento é **contrato**: nomes de rotas, chaves JSON e valores de enumeração valem exatamente como escritos. JSON novo em português, `snake_case`. Enumerações em português sem acento.

---

## 1. Peças e decisões

| Peça | Onde | Papel |
|---|---|---|
| Serviço `dados` | `dados/` (novo módulo Go, container `crmia-dados`, banco `crmia_dados`) | Baixa e carrega a base aberta do CNPJ (Receita Federal, mensal) e a dívida ativa (PGFN, trimestral); responde contagem, busca e ficha. Só rede interna. |
| Painel | `painel/` | Porta de entrada dos ambientes: autentica o cliente (`TOKEN_INTERNO`), aplica a cota mensal do plano, mede o uso e repassa ao `dados`. Página "Base de dados" para a operação. |
| App do cliente | `app/` | Gerador de listas, itens da lista, enriquecimento em lote, envio ao CRM com distribuição, caixa de leads, fila de ligações, bloqueios, configurações. |
| Who Lead | externo (`https://api.wholead.com.br/api/v1`) | Enriquecimento (telefones, sócios, decisores). É a plataforma de dados por trás do solucoes.plus. Credencial de cada cliente, guardada cifrada. |

Decisões e o porquê:

1. **Base própria, não revenda de terceiros.** Os termos da EmpresAqui e da Speedio proíbem extração e redistribuição. Os dois conjuntos são públicos e de livre utilização (Receita: Notas Cocad 47/2024 e 86/2024; PGFN: Decreto 8.777/2016, art. 4º). Custo marginal zero por empresa.
2. **A base mora na plataforma, não em cada cliente.** O conjunto quente da busca soma cerca de 4 GB e a base 12 a 13 GB em disco; o banco de cada cliente tem 512 MB. Um serviço só para todos os clientes.
3. **O app só fala com o painel** (`PAINEL_URL` + `TOKEN_INTERNO`, canal que já existe). O `dados` fica invisível para os ambientes. Cota por plano é conferida no painel a cada pedido, nunca por variável de ambiente.
4. **Lista de prospecção ≠ lead ≠ negócio.** A lista guarda CNPJs que ainda não são registros do CRM. Lead nasce quando o item é enviado ao CRM. Negócio só nasce quando há reunião (ou conversão manual). O funil não é inflado com milhares de empresas frias (mesma separação do Pipedrive).
5. **Só empresas ativas na base de busca.** Índice parcial não é menor e a tabela ficaria 2,65 vezes maior (medido).
6. **Pessoa física fora por padrão.** MEI e empresário individual são pessoas naturais (LGPD). O filtro `incluir_pf` nasce desligado; ligar é escolha explícita, registrada em auditoria. O CPF que a Receita deixa no fim da razão social é cortado na carga.
7. **Contar e pré-visualizar é grátis; a cota desconta por empresa entregue** na geração da lista. Enriquecer tem custo do provedor, sempre mostrado antes.
8. **Trabalho longo é job no servidor** (gerar lista, enriquecer, enviar ao CRM), com progresso consultável e retomada após reinício. Nada depende da aba aberta.
9. **CNPJ é texto de 14 caracteres** (`[0-9A-Z]{12}[0-9]{2}`): o CNPJ alfanumérico entrou em produção em julho de 2026 e já aparece na base.

---

## 2. Tipos comuns

Notação TypeScript só para descrever o JSON. `string` de data é `AAAA-MM-DD`; de instante é RFC 3339.

```ts
type Porte = 'me' | 'epp' | 'demais' | 'nao_informado'          // Receita 01, 03, 05, 00
type Ordem = 'relevancia' | 'recentes' | 'antigas' | 'capital_desc' | 'divida_desc' | 'aleatoria'

interface Filtros {
  ufs?: string[]                 // "CE", "SP"
  municipios?: number[]          // código da Receita (TOM), ex.: 1389 Fortaleza
  cnaes?: number[]               // subclasse, 7 dígitos
  cnae_prefixos?: string[]       // 2 a 5 dígitos: "56", "4711" (o serviço expande pelo catálogo)
  cnae_secundario?: boolean      // true: casa CNAE principal OU secundários
  portes?: Porte[]
  naturezas?: number[]
  matriz?: 'todas' | 'matriz' | 'filial'
  simples?: 'todos' | 'sim' | 'nao'
  mei?: 'todos' | 'sim' | 'nao'
  incluir_pf?: boolean           // padrão false: exclui MEI, empresário individual e naturezas de pessoa física
  abertura_de?: string
  abertura_ate?: string
  abertura_ultimos_dias?: number // atalho "abertas nos últimos N dias"
  capital_min?: number
  capital_max?: number
  com_telefone?: boolean
  com_celular?: boolean
  com_email?: boolean
  ocultar_contador?: boolean     // exclui quem só tem contato compartilhado por 5+ empresas ou e-mail de contabilidade
  divida?: 'todos' | 'com' | 'sem'
  divida_min?: number
  divida_max?: number
  bairros?: string[]             // maiúsculas, sem acento
  ceps?: string[]                // 8 dígitos ou prefixo (mínimo 3)
  nome?: string                  // contém na razão social ou no nome fantasia (mínimo 3 letras)
  cnpjs?: string[]               // lista específica (até 5.000)
}
```

Regras de `Filtros`: é obrigatório ao menos um entre `ufs`, `municipios`, `cnaes`, `cnae_prefixos` e `cnpjs` (422 sem isso). Campo ausente, vazio ou `todos`/`todas` não filtra. Situação cadastral não é filtro: a base só tem ativas.

```ts
interface Telefone {
  numero: string                 // só dígitos, com DDD: 11 = celular, 10 = fixo
  tipo: 'celular' | 'fixo' | 'indefinido'
  compartilhado?: boolean        // mesmo número em 5+ empresas de raízes diferentes (provável contador)
  whatsapp?: boolean | null      // só o enriquecimento sabe
  nao_perturbe?: boolean | null  // bloqueio de telemarketing informado pelo provedor
  origem?: 'receita' | 'wholead' | 'manual'
  pessoa?: string                // de quem é, quando é de sócio ou decisor
  principal?: boolean
  invalido?: boolean             // marcado "número errado" na fila
}
interface SocioBase { nome: string; qualificacao: string; desde: string | null; tipo: 'pf' | 'pj' | 'estrangeiro'; documento: string /* mascarado */; representante?: string }
interface Divida {
  total: number; inscricoes: number
  em_cobranca: number            // situação irregular
  regular: number                // parcelada, garantida, suspensa, benefício fiscal
  tipos: ('geral' | 'previdenciaria' | 'fgts')[]
  ajuizada: boolean; mais_antiga: string | null
  referencia: string             // "2026-T3"
}
interface Empresa {
  cnpj: string; matriz: boolean
  razao_social: string; nome_fantasia: string
  natureza: { codigo: number; descricao: string }; pessoa_fisica: boolean
  porte: Porte; capital: number | null
  abertura: string | null; situacao: 'ATIVA'; situacao_desde: string | null
  simples: boolean; mei: boolean
  cnae: { codigo: number; descricao: string }
  cnaes_secundarios: { codigo: number; descricao: string }[]
  endereco: { tipo_logradouro: string; logradouro: string; numero: string; complemento: string; bairro: string; cep: string; municipio: string; municipio_codigo: number; uf: string }
  telefones: Telefone[]          // 0 a 2, origem "receita"
  email: string; email_tipo: 'corporativo' | 'gratuito' | 'contabilidade' | ''
  socios: SocioBase[]
  divida: Divida | null
  qualidade: number              // 0 a 100, qualidade do contato calculada na carga
  referencia: string             // "2026-11"
}
interface EmpresaPrevia {        // prévia gratuita: sem contato e sem nomes de sócios
  cnpj: string; razao_social: string; nome_fantasia: string
  cnae: { codigo: number; descricao: string }; porte: Porte; abertura: string | null; capital: number | null
  municipio: string; uf: string; pessoa_fisica: boolean; mei: boolean
  tem_celular: boolean; tem_telefone: boolean; tem_email: boolean; tem_divida: boolean
  socios_qtd: number; qualidade: number
}
interface Contagem {
  total: number
  total_modo: 'exato' | 'teto' | 'limite_superior'   // teto: "mais de N"; limite_superior: "até N"
  total_limite_superior?: number
  parciais: { com_celular: number | null; com_email: number | null; com_divida: number | null }
}
```

`pessoa_fisica` = natureza jurídica 2135 (empresário individual) **ou** natureza na faixa 4000–4999 **ou** optante do MEI.

---

## 3. Serviço `dados`

Módulo `github.com/armando-couto/crm-ia/dados`, no molde do painel e do provisionador: `internal/config`, `slog` em JSON, flag `-healthcheck`, token Bearer (`DADOS_TOKEN`, mínimo 24 caracteres) comparado em tempo constante, migrações embutidas, `pgx/v5`, `ServeMux` da biblioteca padrão. Porta 7896. Sem rota pública no Traefik.

### 3.1 Fontes (conferidas em 21 e 22/11/2026)

**CNPJ, Receita Federal (mensal).** O endereço antigo (`/dados/cnpj/dados_abertos_cnpj/`) responde 404; a Receita migrou para um Nextcloud.

* Listagem: `PROPFIND https://arquivos.receitafederal.gov.br/public.php/dav/files/YggdBLfdninEJX9/` com `Depth: 1` (pastas `AAAA-MM`); mesma chamada na pasta do mês lista os arquivos com `getcontentlength`. O token `YggdBLfdninEJX9` fica em configuração (`DADOS_CNPJ_URL`), porque pode mudar.
* Download: `GET .../YggdBLfdninEJX9/{AAAA-MM}/{Arquivo}.zip`, sem autenticação. `Range` funciona (206), mas o servidor não anuncia `Accept-Ranges` e não publica checksum: a conferência é o tamanho e o CRC do zip.
* **Firewall:** cerca de 30 requisições em 30 s e o servidor passa a devolver **HTTP 200 com uma página HTML "Request Rejected"** por 20 a 30 s. O downloader valida `Content-Type`, os bytes `PK` e o tamanho; espaça requisições novas em pelo menos 2,5 s; usa no máximo 2 conexões (configurável). O servidor também corta transferências longas (visto aos 110 s): gravar em `.part` e retomar com `Range` + `If-Range`.
* 37 arquivos por mês, 7,9 GB compactados, 29 GB descompactados: `Empresas0..9`, `Estabelecimentos0..9`, `Socios0..9`, `Simples`, `Cnaes`, `Motivos`, `Municipios`, `Naturezas`, `Paises`, `Qualificacoes`. Um CSV por zip, com nome interno que muda todo mês.
* Formato: sem cabeçalho, separador `;`, todos os campos entre aspas, aspas internas duplicadas, LF, **ISO-8859-1**. Colunas: Empresas 7, Estabelecimentos 30, Sócios 11, Simples 7, domínios 2.
* As partes numeradas **não são alinhadas** entre si: `Empresas` é fatiado por faixa de CNPJ básico; `Estabelecimentos` e `Socios` seguem a ordem de cadastro.
* Volume: 74,4 milhões de estabelecimentos, 28,7 milhões ativos.

**Dívida ativa, PGFN (trimestral).** `https://dadosabertos.pgfn.gov.br/{AAAA}_trimestre_{NN}/` (só `https`; `http` devolve 503). Três zips: FGTS (18 MB), Previdenciário (79 MB) e o não previdenciário (1,44 GB), **cujo nome mudou** em 2026T3 (`Dados_abertos_Sistema_SIDA.zip`): descobrir o nome pela listagem da pasta. CSVs por região da PGFN, ISO-8859-1, `;`, **com cabeçalho**, sem aspas. SIDA e PREV têm 13 colunas, FGTS 15.

Regras de leitura da PGFN: carregar só `TIPO_PESSOA = Pessoa jurídica` e só o devedor principal (`TIPO_DEVEDOR` é `PRINCIPAL` no SIDA e `Principal` nos outros: comparar sem diferenciar maiúsculas); no SIDA os corresponsáveis repetem o valor cheio, somar só a linha principal; `TIPO_SITUACAO_INSCRICAO = "Em cobrança"` é irregular, os outros quatro valores são regulares; `NUMERO_INSCRICAO` do SIDA pode vir com sufixo `.0`; data `dd/mm/aaaa` com sentinela `01/01/1000`; valor com ponto decimal (`.00` para zero). Agregar por CNPJ de 14 caracteres.

### 3.2 Carga

Uma carga por vez (trava consultiva no banco), retomável, em segundo plano no mesmo processo; também pela flag `-carga`. Erro de carga **nunca derruba o serviço**: fica registrado e a API continua respondendo com a base anterior.

1. Descobrir a referência mais nova de cada fonte; se já carregada, nada a fazer.
2. Baixar para o volume de trabalho (`/trabalho`), arquivo por arquivo, com retomada. Estado por arquivo em tabela de controle.
3. Ler o único membro de cada zip **em fluxo**, removendo bytes `0x00` (um arquivo de 2026-11 tem NUL e aborta o `COPY` do PostgreSQL 16), validando o número de campos com `encoding/csv` estrito, e enviar por `COPY` (protocolo do `pgx`) para tabelas de carga `UNLOGGED`, tudo `text`. Nunca dividir o arquivo por linhas (há quebra de linha dentro de aspas). Número de campos errado aborta a referência e mantém a base anterior.
4. Guardas: apagar linhas fantasma de Empresas (razão vazia e natureza `0000`), conferir `cnpj_basico` único, comparar contagens com a carga anterior.
5. Construir o esquema novo ao lado do que está em uso, só com **estabelecimentos ativos** (situação `02`): tabela de busca enxuta (só colunas de filtro), ficha, sócios, índices de cobertura, GIN de CNAEs, cubos de contagem, faixas de `id` por UF e município. Linhas gravadas em ordem `(uf, municipio, cnae, cnpj)`.
6. Trocar os esquemas numa transação, `VACUUM (ANALYZE)`, aquecer (`pg_prewarm` quando existir), apagar as tabelas de carga e os zips.
7. Quando só a PGFN mudou, refazer do passo 5 (a construção leva minutos).

Limpezas obrigatórias na transformação:

* **CPF na razão social:** em natureza 2135, cortar os 11 dígitos finais (medido: 55% a 58% dessas razões sociais terminam com CPF completo e válido). Nunca devolver CPF completo em nenhuma rota.
* Texto: remover caracteres de controle, juntar espaços, aparar; vazio vira `NULL`.
* Datas `0`/`00000000`/inválidas viram `NULL`. Capital com vírgula decimal.
* **Telefone:** a Receita publica no máximo 8 dígitos. Regra: 8 dígitos começando por 6 a 9 → celular, repõe o `9` (11 dígitos); começando por 2 a 5 → fixo (10 dígitos), exceto DDD 11 começando por 4 ou 5 → `indefinido`; começando por 1 → celular com `9` só no DDD 11, senão descarta; dígitos repetidos, DDD inválido ou menos de 8 dígitos → descarta.
* **Contato compartilhado:** telefone presente em 5 ou mais raízes de CNPJ ativas → `compartilhado`. E-mail com `contab`, `contador`, `escritorio` ou `assessoria`, ou presente em 5 ou mais raízes → `email_tipo = contabilidade`. Domínio de provedor gratuito → `gratuito`.
* **Sócios:** descartar sócios com faixa etária 1 e 2 (menores); CPF já vem mascarado e segue mascarado; documento de sócio pessoa jurídica passou a ter 8 dígitos em 2026-08.
* `qualidade` (0 a 100): 60 se tem celular próprio; 35 se só fixo próprio; +20 e-mail não contábil; +10 segundo telefone; teto 100; contato só compartilhado vale 15; sem contato, 0.

Parâmetros de banco na sessão de busca: `jit = off`, `work_mem = 64MB`, `statement_timeout` curto (padrão 4 s).

Recorte opcional: `DADOS_UFS` (lista de UFs; vazio = Brasil) reduz disco e tempo em servidores pequenos.

**Modo amostra** (`DADOS_AMOSTRA=true`, padrão em desenvolvimento e na CI): monta o mesmo esquema de serviço a partir de uma base fictícia embutida (alguns milhares de empresas inventadas, determinísticas, em várias UFs e CNAEs, com sócios e dívidas), sem baixar nada. Toda resposta de `situacao` traz `amostra: true` e o app mostra a faixa "dados de demonstração".

### 3.3 API (`Authorization: Bearer <DADOS_TOKEN>`; erros `{"erro": "..."}`)

| Rota | Corpo ou consulta | Resposta |
|---|---|---|
| `GET /saude` | — | `{"status":"ok"}` (sem token) |
| `GET /v1/situacao` | — | `Situacao` |
| `GET /v1/catalogos/cnaes` | `q`, `limite` (≤ 50) | `{ itens: { codigo: number; descricao: string }[] }` (busca por texto sem acento ou por prefixo de código) |
| `GET /v1/catalogos/municipios` | `uf`, `q`, `limite` (≤ 50) | `{ itens: { codigo: number; nome: string; uf: string; empresas: number }[] }` (ordenado por `empresas` desc) |
| `GET /v1/catalogos/naturezas` | — | `{ itens: { codigo: number; descricao: string; pessoa_fisica: boolean }[] }` |
| `GET /v1/catalogos/ufs` | — | `{ itens: { uf: string; nome: string; empresas: number }[] }` |
| `POST /v1/empresas/contagem` | `{ filtros }` | `Contagem` |
| `POST /v1/empresas/previa` | `{ filtros, ordem?, limite? (≤ 25) }` | `{ itens: EmpresaPrevia[]; versao: string }` |
| `POST /v1/empresas/candidatos` | `{ filtros, ordem?, limite? (≤ 5000), cursor? }` | `{ cnpjs: string[]; cursor: string; fim: boolean; versao: string; ordem_aplicada: Ordem }` |
| `POST /v1/empresas/detalhes` | `{ cnpjs: string[] (≤ 500) }` | `{ itens: Empresa[]; nao_encontrados: string[] }` |
| `GET /v1/empresas/{cnpj}` | — | `Empresa` ou 404 |
| `POST /v1/cargas` | `{ fonte: 'cnpj' \| 'pgfn' \| 'tudo' }` | 202 `{ ok: true }`; 409 se já há carga em curso |

```ts
interface Situacao {
  pronta: boolean; amostra: boolean
  referencia_cnpj: string; referencia_pgfn: string
  versao: string                 // muda a cada troca de base; os cursores carregam a versão
  empresas: number; socios: number; devedores: number
  atualizada_em: string | null; espaco_bytes: number
  ufs: string[]                  // vazio = Brasil
  carga: {
    estado: 'ociosa' | 'baixando' | 'carregando' | 'construindo' | 'erro'
    fonte: string; referencia: string; etapa: string; percentual: number
    iniciada_em: string | null; erro: string
    arquivos: { nome: string; estado: 'pendente' | 'baixando' | 'baixado' | 'carregado' | 'erro'; bytes: number; total_bytes: number; linhas: number }[]
  }
}
```

Regras da busca:

* **Contagem:** exata pelos cubos ou pelo índice de cobertura sempre que der (meta: menos de 150 ms); com filtro fora dos cubos ou CNAE em qualquer posição com volume alto, contar com teto de 10.001 e devolver `total_modo: 'teto'` (ou `limite_superior` com `total_limite_superior`). Nunca usar a estimativa do planejador (erra de −94% a +120%).
* **Paginação por chave, nunca `OFFSET`.** O `cursor` é opaco e inclui a `versao`; cursor de versão antiga responde 409 `{"erro":"a base foi atualizada; refaça a busca"}`.
* **Ordem:** `relevancia` (padrão) = `qualidade` desc e depois `id`; as demais só são aplicadas quando o total filtrado é de até 200.000, senão vale `relevancia` e `ordem_aplicada` diz isso.
* `candidatos` devolve só CNPJs (barato); `detalhes` devolve a ficha inteira, com sócios e dívida.
* `nome` e `bairros`/`ceps` são filtros aplicados sobre o resultado dos demais (sem índice próprio).

### 3.4 Configuração

`PORT` (7896), `APP_ENV`, `DATABASE_URL`, `DADOS_TOKEN`, `DADOS_CNPJ_URL` (padrão o WebDAV acima), `DADOS_PGFN_URL` (padrão o host da PGFN), `DADOS_DIR_TRABALHO` (`/trabalho`), `DADOS_CARGA_AUTOMATICA` (`true`: confere fontes na subida e a cada 6 h), `DADOS_AMOSTRA` (`false`), `DADOS_UFS` (vazio), `DADOS_CONEXOES` (2).

---

## 4. Painel

### 4.1 Rotas para os ambientes (`Authorization: Bearer <TOKEN_INTERNO>` do slug)

Prefixo `/painel/api/empresas/{slug}/prospeccao`. Erros respondem **com as duas chaves** `{"erro": "...", "error": "..."}` (o app lê `error`).

| Rota | Efeito | Resposta |
|---|---|---|
| `GET ""` | resumo | `ResumoPlataforma` |
| `GET /catalogos/{nome}` | repassa a consulta | igual ao `dados` |
| `POST /contagem` | repassa; limite de 120 por minuto por cliente | `Contagem` |
| `POST /previa` | repassa (≤ 25) | `{ itens: EmpresaPrevia[] }` |
| `POST /candidatos` | repassa (≤ 5.000 por chamada) | igual ao `dados` |
| `POST /extracoes` | `{ pedido_id: string; cnpjs: string[] (≤ 500) }` → **desconta da cota** o número de empresas devolvidas | `{ itens: Empresa[]; nao_encontrados: string[]; cota: Cota }` |
| `GET /cnpj/{cnpj}` | ficha de um CNPJ (grátis, 60 por minuto) | `Empresa` |

```ts
interface Cota { mes: string /* "2026-11" */; limite: number; usado: number; restante: number }
interface ResumoPlataforma {
  habilitada: boolean; motivo: string
  cota: Cota
  base: { pronta: boolean; amostra: boolean; referencia_cnpj: string; referencia_pgfn: string; versao: string; empresas: number; carga: { estado: string; percentual: number; etapa: string; erro: string } }
}
```

* `habilitada: false` com `motivo` quando o `dados` não está configurado ou não responde, ou quando a cota do plano é zero.
* **Cota** = `planos.prospeccao_leads_mes` do plano da licença vigente; sem licença vigente, a chave `prospeccao.cota_sem_licenca` de `configuracoes` (padrão 300). Mês-calendário.
* **Extração:** conferir o que resta → pedir ao `dados` → descontar atomicamente a quantidade devolvida → só então entregar. Sem saldo para o lote inteiro: 429 com `{"erro","error","cota"}` e nada é entregue (o app pede lotes do tamanho do que resta). **Idempotente:** repetir o mesmo `pedido_id` com o mesmo lote de CNPJs não desconta de novo.
* Status: 422 filtro inválido, 429 cota ou limite de requisições, 503 `dados` fora do ar ou base em carga, 409 cursor de versão antiga.

### 4.2 Banco (`0002_prospeccao.sql`)

`planos.prospeccao_leads_mes INT NOT NULL DEFAULT 0` (valores iniciais: essencial 1.000, profissional 5.000, escala 20.000, sob-medida 50.000); `prospeccao_uso (cliente_id, competencia, leads, consultas, atualizado_em)` com chave `(cliente_id, competencia)`; `prospeccao_extracoes (id, cliente_id, pedido_id, lote, competencia, quantidade, criado_em)` com `UNIQUE (cliente_id, pedido_id, lote)`, onde `lote` é o hash dos CNPJs ordenados.

### 4.3 Telas

* **Base de dados** (menu): situação da base (referências, total de empresas, espaço, estado da carga com barra e arquivos), botão "Verificar e carregar agora" (administrador e suporte), uso do mês por cliente (cota, usado, %).
* **Plano:** campo "Leads de prospecção por mês" no formulário e coluna na lista.
* **Ficha do cliente:** linha "Prospecção no mês: usado de limite".
* "Meu plano" (`GET /api/empresas/{slug}/plano`) ganha `prospeccao: Cota`.

---

## 5. App: banco e jobs

Migração `028_prospeccao.sql` (fonte da verdade das colunas). Tabelas novas: `prospeccao_segmentos`, `prospeccao_listas`, `prospeccao_itens`, `prospeccao_jobs`, `leads`, `contato_bloqueios`. Colunas novas: `calls.lead_id`, `calls.desfecho`, `calls.numero`; `deals.lost_reason`, `deals.lost_note`, `deals.lead_id`, `deals.origem`; `companies.industry` alargada para 255. Índice de expressão para o CNPJ normalizado de `companies`.

**CNPJ normalizado** em todo código novo: `models.NormalizarCNPJ` (maiúsculas, só `[0-9A-Z]`, 14 caracteres) e `models.CNPJValido` (dígitos verificadores pela regra alfanumérica: valor do caractere = código ASCII − 48). `models.CompanyByCNPJ` passa a comparar por `upper(regexp_replace(cnpj, '[^0-9A-Za-z]', '', 'g'))`.

**Jobs** (`prospeccao_jobs`): `tipo` em `gerar_lista | enriquecer | enviar_crm`; `status` em `pendente | rodando | concluido | falhou | cancelado`; `total`, `feitos`, `erros`, `custo_centavos`, `mensagem`, `parametros` (entrada), `estado` (cursor de retomada), `resultado` (resumo final). Um worker próprio (goroutine com `recover`, desligado nos testes de controller) pega jobs com `FOR UPDATE SKIP LOCKED`, processa em fatias curtas atualizando o progresso e retoma jobs `rodando` órfãos na subida. No máximo um job ativo por lista. Cancelar marca `cancelado` e o worker para na próxima fatia.

```ts
interface Job {
  id: number; tipo: 'gerar_lista' | 'enriquecer' | 'enviar_crm'; lista_id: number | null
  status: 'pendente' | 'rodando' | 'concluido' | 'falhou' | 'cancelado'
  total: number; feitos: number; erros: number; custo_centavos: number
  mensagem: string; resultado: Record<string, unknown> | null
  criado_por: number | null; criado_em: string; iniciado_em: string | null; finalizado_em: string | null
}
```

**Permissões** (constante + catálogo + padrões; sem migração):

| Chave | Rótulo | Grupo | Seller | Manager |
|---|---|---|---|---|
| `prospecting.view` | Ver listas de prospecção | Prospecção | não | sim |
| `prospecting.manage` | Gerar listas, enriquecer e enviar ao CRM | Prospecção | não | sim |
| `leads.view` | Ver leads | Prospecção | sim | sim |
| `leads.edit` | Trabalhar leads (ligar, registrar, converter) | Prospecção | sim | sim |
| `leads.assign` | Distribuir e redistribuir leads | Prospecção | não | sim |

`prospecting.manage` entra em `auth.canManage` (menu de configurações).

---

## 6. App: API de prospecção (`/api/v1`)

Leitura exige `prospecting.view`; escrita, `prospecting.manage`. Erros `{"error": "..."}`.

### 6.1 Plataforma

| Rota | Resposta |
|---|---|
| `GET /prospeccao` | `ResumoPlataforma` (com `habilitada: false` e `motivo` quando o ambiente não tem painel configurado) |
| `GET /prospeccao/catalogos/{nome}` | repasse (cache de 1 h no app) |
| `POST /prospeccao/contagem` `{ filtros }` | `Contagem` |
| `POST /prospeccao/previa` `{ filtros, ordem? }` | `{ itens: EmpresaPrevia[] }` |

Erros da plataforma viram mensagens do app: 429 de cota → 429 `{"error":"cota mensal de prospecção esgotada","cota":Cota}`; 503 → 503 `{"error":"a base de empresas está sendo atualizada, tente em alguns minutos"}`.

### 6.2 Segmentos (filtro salvo, não resultado)

`GET /prospeccao/segmentos` → `Segmento[]`; `POST` `{ nome, filtros, ordem, compartilhado }`; `PUT /prospeccao/segmentos/{id}`; `DELETE`. Cada contagem feita a partir de um segmento atualiza `ultima_contagem`.

```ts
interface Segmento { id: number; nome: string; filtros: Filtros; ordem: Ordem; compartilhado: boolean; ultima_contagem: number | null; ultima_contagem_em: string | null; criado_por: number | null; criado_por_nome: string; criado_em: string; atualizado_em: string }
```

### 6.3 Listas e itens

| Rota | Corpo | Resposta |
|---|---|---|
| `GET /prospeccao/listas` | `page`, `per_page`, `q`, `status` | `{ data: Lista[]; pagination }` |
| `POST /prospeccao/listas` | `CriarLista` | 201 `{ lista: Lista; job: Job }` |
| `GET /prospeccao/listas/{id}` | — | `{ lista: Lista; jobs: Job[] /* ativos e o último de cada tipo */ }` |
| `PUT /prospeccao/listas/{id}` | `{ nome?, descricao?, playbook_id?, pipeline_id?, stage_id? }` | `Lista` |
| `DELETE /prospeccao/listas/{id}` | — | 204 (apaga a lista e os itens; leads já criados ficam) |
| `GET /prospeccao/listas/{id}/itens` | `page`, `per_page`, `estado`, `q`, `com_celular`, `com_decisor`, `nota_min`, `sort` (`nota` padrão, `razao_social`, `municipio`, `estado`), `dir` | `{ data: Item[]; pagination }` |
| `GET /prospeccao/itens/{id}` | — | `Item` com `dados` e `enriquecimento` |
| `POST /prospeccao/listas/{id}/itens/descartar` | `{ selecao: Selecao }` | `{ afetados: number }` |
| `POST /prospeccao/listas/{id}/itens/restaurar` | `{ selecao: Selecao }` | `{ afetados: number }` |
| `GET /prospeccao/listas/{id}/exportar` | — | CSV (`;`, UTF-8 com BOM), auditado; uma linha por empresa |
| `GET /prospeccao/jobs/{id}` | — | `Job` |
| `POST /prospeccao/jobs/{id}/cancelar` | — | `Job` |

```ts
interface CriarLista {
  nome: string; descricao?: string
  origem?: 'base' | 'cnpjs'       // cnpjs: lista colada ou vinda de arquivo, em filtros.cnpjs
  filtros: Filtros; ordem?: Ordem
  quantidade: number              // 1 a 20.000; limitada à cota restante
  excluir_no_crm?: boolean        // padrão true: pula CNPJ que já é empresa do CRM
  excluir_em_listas?: boolean     // padrão true: pula CNPJ que já está em outra lista
  segmento_id?: number
}
interface Lista {
  id: number; nome: string; descricao: string; origem: 'base' | 'cnpjs'
  filtros: Filtros; ordem: Ordem; quantidade_pedida: number
  status: 'gerando' | 'pronta' | 'falhou'; mensagem: string
  versao_base: string; incluir_pf: boolean
  playbook_id: number | null; pipeline_id: number | null; stage_id: number | null
  criado_por: number | null; criado_por_nome: string; criado_em: string; atualizado_em: string
  contadores: ContadoresLista
}
interface ContadoresLista {
  total: number; novo: number; enriquecido: number; sem_resultado: number; erro: number
  enviado: number; descartado: number; duplicado: number; bloqueado: number
  com_celular: number; com_email: number; com_decisor: number; com_whatsapp: number
  pessoa_fisica: number; custo_centavos: number
}
type EstadoItem = 'novo' | 'enriquecido' | 'sem_resultado' | 'erro' | 'enviado' | 'descartado' | 'duplicado' | 'bloqueado'
interface Item {
  id: number; lista_id: number; cnpj: string
  razao_social: string; nome_fantasia: string; uf: string; municipio: string
  cnae_codigo: number; cnae_descricao: string; porte: Porte; pessoa_fisica: boolean
  estado: EstadoItem
  nota: number; nota_motivos: { pontos: number; texto: string }[]
  tem_celular: boolean; tem_email: boolean; tem_decisor: boolean; tem_whatsapp: boolean; tem_divida: boolean
  telefone_principal: string
  enriquecido_em: string | null; enriquecimento_provedor: string; custo_centavos: number; erro: string
  company_id: number | null; lead_id: number | null; enviado_em: string | null; criado_em: string
  dados?: Empresa                          // só no detalhe
  enriquecimento?: PerfilContato | null    // só no detalhe
}
interface Selecao { ids?: number[]; todos?: boolean; filtro?: { estado?: EstadoItem; q?: string; com_celular?: boolean; com_decisor?: boolean; nota_min?: number } }
```

`Selecao`: `ids` (até 5.000) **ou** `todos: true` com `filtro` opcional (todos os itens da lista que casam com o filtro). É assim que "selecionar todos os N" funciona além da página.

**Job `gerar_lista`:** (1) lê a cota e limita a quantidade; (2) pede `candidatos` em páginas, descartando CNPJ bloqueado, já no CRM (`excluir_no_crm`), já em outra lista (`excluir_em_listas`) e repetido, até juntar a quantidade ou esgotar (teto de candidatos lidos: 20 vezes a quantidade); (3) chama `extracoes` em lotes de 200 com o `pedido_id` da lista; (4) grava os itens com o retrato da empresa, a nota e as marcas; (5) lista `pronta`, auditoria "lista gerada" com filtros, total e quantos de pessoa física. `resultado`: `{ pedidos, entregues, pulados_no_crm, pulados_em_listas, pulados_bloqueio, nao_encontrados }`. Origem `cnpjs`: pula o passo 2 (valida os CNPJs antes).

**Nota do item** (0 a 100, sempre com os motivos): +35 celular próprio; +10 WhatsApp confirmado; +15 decisor com telefone (enriquecido) ou +8 sócio-administrador identificado; +10 e-mail corporativo ou +4 gratuito; +10 fixo próprio; +5 site; +5 empresa com mais de 2 anos; +5 porte EPP ou demais; −25 se o único contato é compartilhado ou de contabilidade; −40 sem telefone; −10 por telefone com "não perturbe". Recalculada depois do enriquecimento.

---

## 7. App: enriquecimento pelo Who Lead

Provedor novo no pacote `services/enriquecimento`, código **`wholead`**, nome exibido "Who Lead (Plus Soluções)". Configuração em `EnrichmentSettings.wholead`:

```ts
interface WhoLeadSettings {
  habilitado: boolean; demo: boolean
  client_id: string
  client_secret?: string          // só na gravação; vazio mantém o guardado
  has_secret: boolean
  base_url?: string               // padrão https://api.wholead.com.br/api/v1
  incluir_socios: boolean         // padrão true
  incluir_responsavel_legal: boolean  // padrão true
  incluir_portabilidade: boolean  // padrão false
  preco_centavos: number          // estimativa por CNPJ; padrão 80
  lote_tamanho: number            // padrão 500, máximo 1000
}
```

Contrato do provedor (especificação pública em `/openapi.json`):

* Token: `POST /oauth/token`, formulário `grant_type=client_credentials&client_id&client_secret` → `{ access_token, token_type, expires_in }`. Guardar em cache até `expires_in − 60 s`; 401 emite outro uma vez.
* Conta: `GET /account` → `{ plan_name, quota_total, quota_used, quota_remaining, cycle_start, cycle_end }` (campos nulos no pré-pago).
* Um CNPJ (síncrono): `GET /enrichment/cnpj/{cnpj}` → `{ cnpj, found, data, source, cost, quota_remaining }`. É o `Provedor.Consultar` do registro (quarta fonte no modal de enriquecer da ficha da empresa).
* Lote (assíncrono): `POST /enrichment` `{ cnpjs, include_socios, include_responsavel_legal, include_portability }` → 202 `{ consultation_id, status, total_cost, units_debited, item_count }`; `GET /enrichment/{id}` → `{ status: pending|processing|ready|failed, items_total, items_processed }`; `GET /enrichment/{id}/results?offset&limit` (≤ 500) → `{ items: [{ cnpj, found, result: found|not_found|rate_limited|error, refunded, cnpj_data, nome_fantasia, persons: [{ cpf, tipo_pessoa, nome, qualificacao, data_entrada_sociedade, telefones: string[] }], portability }] }`.
* Erros: 401 credencial; **402 saldo ou franquia esgotados** (estado de negócio, mensagem clara, o job para e os itens ficam como estavam); 409 e 429 → nova tentativa espaçada; dinheiro vem como **texto decimal**.

**`cnpj_data` e `data` são objetos sem esquema publicado.** O mapeador é tolerante e guarda o bruto: reconhece o formato de consulta da Data Stone (`mobile_phones[{ddd,number,whatsapp,active_nao_me_ligue}]`, `land_lines`, `emails[{email}]`, `partners`, `legal_representative`, `related_company_members`, `employee_count`, `estimated_revenue`), o formato B2B (`contatos.telefones.celular|fixo`, `contatos.emails`, `socios.itens`, `cadastro.faturamento_estimado`) e, fora disso, procura chaves conhecidas em qualquer nível. Formato desconhecido não é erro: fica só com `persons`. **CPF de pessoa é mascarado** (`***123456**`) antes de gravar o perfil; o completo fica só no bruto, que não vai para a tela.

```ts
interface PerfilContato {
  provedor: string; demo: boolean; consultado_em: string
  telefones: Telefone[]                                   // da empresa
  emails: { email: string; tipo?: string; validado?: boolean | null; pessoa?: string }[]
  pessoas: Pessoa[]
  funcionarios_faixa: string; faturamento_faixa: string; site: string; observacao: string
}
interface Pessoa { nome: string; documento: string /* mascarado */; papel: 'socio' | 'representante' | 'funcionario'; cargo: string; telefones: Telefone[]; emails: string[] }
```

O `Perfil` da inteligência de empresas ganha, de forma aditiva: `Telefone.whatsapp` e `Telefone.nao_perturbe` (opcionais) e a seção `pessoas: Pessoa[]` (opcional).

**Modo demonstração** (`demo: true`): contatos fictícios determinísticos pelo CNPJ, custo zero, sempre rotulados.

Rotas do lote:

| Rota | Corpo | Resposta |
|---|---|---|
| `POST /prospeccao/listas/{id}/enriquecer/estimativa` | `{ selecao: Selecao; reprocessar?: boolean }` | `EnriquecerEstimativa` |
| `POST /prospeccao/listas/{id}/enriquecer` | `{ selecao; reprocessar?; custo_confirmado_centavos: number }` | 202 `{ job: Job }` |

```ts
interface EnriquecerEstimativa {
  selecionados: number; a_enriquecer: number; ja_enriquecidos: number
  provedor: { codigo: 'wholead'; nome: string; pronto: boolean; demo: boolean; motivo: string }
  custo_unitario_centavos: number; custo_estimado_centavos: number
  uso_mes: { custo_centavos: number; limite_centavos: number }
  saldo: { plano: string; franquia_total: number | null; franquia_restante: number | null } | null
}
```

`enriquecer` recusa (409) se `custo_confirmado_centavos` for menor que a estimativa atual, e (429) se a estimativa estourar o limite mensal de enriquecimento do cliente. O uso mensal passa a somar `company_enrichments` **e** o custo dos itens de lista.

**Job `enriquecer`:** itens em `novo`, `sem_resultado` ou `erro` (e `enriquecido` só com `reprocessar`), em lotes de `lote_tamanho`. Para cada lote: `POST /enrichment`, **grava o `consultation_id` no `estado` do job antes de qualquer outra coisa** (retomada sem pagar duas vezes), consulta o andamento com espera crescente (5 s até 30 s), lê os resultados em páginas e atualiza cada item (`enriquecido` ou `sem_resultado`, perfil, marcas, nota, custo). `rate_limited` e `error` deixam o item em `erro` com a mensagem. Auditoria "lista enriquecida" com provedor, quantidade e custo.

**Fonte gratuita melhorada:** quando a plataforma responde, o provedor `receita` consulta primeiro `GET /prospeccao/cnpj/{cnpj}` (base própria: traz e-mail e telefone, sem limite de terceiros) e só depois cai na cadeia de APIs públicas.

---

## 8. App: leads, fila de ligações, bloqueios e configurações

### 8.1 Enviar ao CRM

| Rota | Corpo | Resposta |
|---|---|---|
| `POST /prospeccao/listas/{id}/enviar/previa` | `EnviarRequest` | `EnviarPrevia` |
| `POST /prospeccao/listas/{id}/enviar` | `EnviarRequest` | 202 `{ job: Job }` |

```ts
interface EnviarRequest {
  selecao: Selecao
  duplicados: 'pular' | 'vincular'        // empresa já existe no CRM: pular, ou criar o lead ligado a ela
  criar_contatos: boolean                 // decisores e sócios viram contatos (padrão true)
  distribuicao: { modo: 'unico' | 'rodizio'; usuarios: number[]; por_dia?: number }
  temperatura?: '' | 'fria' | 'media' | 'quente'
  etiquetas?: string[]
  conta_alvo_tier?: 0 | 1 | 2 | 3         // 0 = não marcar
}
interface EnviarPrevia {
  selecionados: number; aptos: number
  duplicados_no_crm: number; bloqueados: number; ja_enviados: number; descartados: number
  pessoa_fisica: number; sem_telefone: number
  por_usuario: { user_id: number; nome: string; quantidade: number }[]
}
```

**Job `enviar_crm`**, por item apto (`novo`, `enriquecido`, `sem_resultado`):

1. CNPJ, telefone ou e-mail em `contato_bloqueios` → item `bloqueado`.
2. Empresa: procura por CNPJ. Existe e `duplicados = 'pular'` → item `duplicado` (guarda `company_id`). Não existe → cria com todo o cadastro da base e do enriquecimento, grava a inteligência (`intel`), as fontes por campo e a atividade de **sistema** "Empresa criada pela lista de prospecção X".
3. Contatos (se `criar_contatos`): pessoas do enriquecimento com telefone primeiro, depois sócios-administradores; no máximo 3 por empresa; papel de compra `decisor`; sem repetir nome na mesma empresa. Nunca cria contato para sócio menor de idade nem para lista de pessoa física sem `incluir_pf`.
4. Lead: título = nome fantasia ou razão social; dono pelo rodízio (ordem fixa, pulando inativos); telefones ordenados do melhor para o pior (celular com WhatsApp, celular de decisor, celular da empresa, fixo; compartilhados e "não perturbe" no fim); `resumo` pronto; `proxima_acao_em` = agora, ou distribuído em dias úteis quando `por_dia` vem (N leads por vendedor por dia, pulando sábado, domingo e feriados nacionais).
5. Item `enviado` com `company_id` e `lead_id`.

Não dispara automações de "contato criado" nem um e-mail por registro. `resultado`: `{ leads, empresas_criadas, empresas_existentes, contatos, duplicados, bloqueados, por_usuario }`. Auditoria "lista enviada ao CRM".

### 8.2 Leads

Exigem `leads.view` (leitura) e `leads.edit` (trabalho); atribuição em massa exige `leads.assign`. **Quem não tem `leads.assign` só enxerga e trabalha os próprios leads e os sem dono.**

| Rota | Corpo ou consulta | Resposta |
|---|---|---|
| `GET /leads` | `fila` (`meus` padrão, `sem_dono`, `todos`), `estado`, `urgencia`, `owner_id`, `lista_id`, `temperatura`, `etiqueta`, `q`, `page`, `per_page`, `sort` (`urgencia` padrão, `nota`, `criado_em`, `titulo`), `dir` | `{ data: Lead[]; pagination; contadores: ContadoresLeads }` |
| `POST /leads` | `{ titulo, company_id?, contact_id?, owner_id?, telefones?, temperatura?, etiquetas?, valor_estimado? }` | 201 `Lead` |
| `GET /leads/{id}` | — | `LeadCompleto` (marca como visto) |
| `PUT /leads/{id}` | `{ titulo?, owner_id?, contact_id?, temperatura?, etiquetas?, valor_estimado?, telefones?, proxima_acao_em? }` (parcial) | `Lead` |
| `DELETE /leads/{id}` | — | 204 |
| `POST /leads/bulk` | `{ ids: number[] (≤ 500); acao: 'dono' \| 'temperatura' \| 'etiqueta' \| 'descartar' \| 'reabrir'; owner_id?; temperatura?; etiqueta?; motivo? }` | `{ afetados: number }` |
| `POST /leads/{id}/converter` | `{ pipeline_id?, stage_id?, nome?, valor?, owner_id? }` | `{ lead: Lead; deal: Deal }` |
| `POST /leads/{id}/descartar` | `{ motivo: string; observacao?: string; bloquear?: boolean }` | `Lead` |
| `POST /leads/{id}/reabrir` | — | `Lead` |
| `POST /leads/{id}/resultado` | `ResultadoRequest` | `ResultadoResposta` |
| `POST /leads/{id}/liberar` | — | 204 (solta a trava da fila) |
| `GET /leads/fila/resumo` | — | `ResumoFila` |
| `POST /leads/fila/proximo` | `{ lista_id?: number; pular_id?: number }` | `{ lead: LeadCompleto \| null; motivo_vazio: '' \| 'fila_vazia' \| 'fora_da_janela'; janela: Janela }` |
| `GET /agenda/disponibilidade` | `user_id` (padrão: eu), `dias` (≤ 14) | `{ user_id: number; duracao_min: number; dias: { data: string; horarios: string[] /* "HH:MM" */ }[] }` |

```ts
type EstadoLead = 'novo' | 'tentando' | 'retornar' | 'convertido' | 'descartado'
type Resultado = 'atendeu' | 'reuniao' | 'retornar' | 'sem_interesse' | 'nao_e_decisor'
               | 'nao_atendeu' | 'caixa_postal' | 'ocupado' | 'numero_errado' | 'nao_ligar_mais'
type Urgencia = 'atrasado' | 'hoje' | 'futuro' | 'nenhuma'

interface Lead {
  id: number; titulo: string
  company_id: number | null; company_name: string
  contact_id: number | null; contact_name: string
  cnpj: string; cidade: string; uf: string
  origem: 'prospeccao' | 'manual' | 'formulario' | 'importacao'
  lista_id: number | null; lista_nome: string; item_id: number | null
  owner_id: number | null; owner_name: string
  estado: EstadoLead; urgencia: Urgencia
  temperatura: '' | 'fria' | 'media' | 'quente'; etiquetas: string[]
  nota: number; valor_estimado: number | null
  telefones: Telefone[]; emails: string[]
  tentativas: number; ultimo_contato_em: string | null; ultimo_resultado: Resultado | ''
  proxima_acao_em: string | null
  motivo_descarte: string
  deal_id: number | null; meeting_id: number | null; convertido_em: string | null
  whatsapp_optin: boolean; pessoa_fisica: boolean
  em_atendimento_por: number | null
  visto: boolean; criado_em: string; atualizado_em: string
}
interface ContadoresLeads { meus: number; sem_dono: number; todos: number; novo: number; tentando: number; retornar: number; convertido: number; descartado: number; atrasados: number; hoje: number }
interface Sinal { tipo: string; nivel: 'ok' | 'atencao' | 'risco'; texto: string }
interface ResumoLead {
  frases: string[]                 // "Padaria em Fortaleza/CE, aberta há 6 anos, pequeno porte"
  decisores: { nome: string; cargo: string; telefones: Telefone[] }[]
  sinais: Sinal[]                  // por que ligar e o que observar
  cadastro: { razao_social: string; nome_fantasia: string; cnae: string; porte: Porte; abertura: string | null; capital: number | null; natureza: string; simples: boolean; mei: boolean; endereco: string; municipio: string; uf: string; site: string; email: string; socios: SocioBase[]; divida: Divida | null }
}
interface Tentativa { id: number; resultado: Resultado; numero: string; duracao_segundos: number; notas: string; user_name: string; criado_em: string }
interface Alerta { tipo: 'pessoa_fisica' | 'nao_perturbe' | 'bloqueado' | 'fora_da_janela' | 'contador' | 'divida' | 'whatsapp_sem_optin' | 'limite_tentativas'; nivel: 'info' | 'atencao' | 'risco'; texto: string }
interface Janela { aberta: boolean; texto: string /* "segunda a sexta, 9h às 18h" */; reabre_em: string | null }
interface LeadCompleto {
  lead: Lead; resumo: ResumoLead
  contatos: Contact[]              // contatos da empresa (tipo já existente)
  tentativas: Tentativa[]
  roteiro: { titulo: string; corpo: string } | null   // manual da lista ou o padrão, com variáveis preenchidas
  alertas: Alerta[]; janela: Janela
}
interface ResultadoRequest {
  resultado: Resultado
  numero?: string; duracao_segundos?: number; notas?: string
  retornar_em?: string             // obrigatório em "retornar"; opcional em "atendeu"
  motivo?: string                  // "sem_interesse": um dos motivos de descarte
  contato?: { nome: string; cargo?: string; telefone?: string; email?: string }   // "nao_e_decisor": quem é o decisor
  reuniao?: { titulo?: string; inicio: string; duracao_min?: number; local?: string; notas?: string; user_id?: number; pipeline_id?: number; stage_id?: number; valor?: number; forcar?: boolean }
  whatsapp_optin?: boolean; temperatura?: '' | 'fria' | 'media' | 'quente'
}
interface ResultadoResposta {
  lead: Lead; call: Call
  meeting?: Meeting; deal?: Deal
  proximo_passo: { tipo: 'ligar' | 'retornar' | 'reuniao' | 'nenhum'; em: string | null; texto: string }
}
interface ResumoFila { na_fila: number; novos: number; retornos_vencidos: number; retornos_hoje: number; feitas_hoje: number; conectadas_hoje: number; reunioes_hoje: number; meta_dia: number; janela: Janela }
```

`urgencia`: `atrasado` = `proxima_acao_em` antes de hoje; `hoje`; `futuro`; `nenhuma` = sem próxima ação (leads abertos sem próxima ação são um problema, como o aviso amarelo do Pipedrive).

**O que cada resultado faz** (uma chamada grava tudo: registro em `calls` com `lead_id`, `desfecho`, `numero`; atividade `ligacao` na empresa e no contato; estado do lead):

| Resultado | `calls.outcome` | Estado do lead | Próxima ação |
|---|---|---|---|
| `atendeu` | `conectada` | `tentando` | `retornar_em`, ou o retorno padrão (48 h úteis) |
| `reuniao` | `conectada` | `convertido` | cria a reunião, converte em negócio e sai da fila |
| `retornar` | `conectada` | `retornar` | `retornar_em` (obrigatório) |
| `sem_interesse` | `conectada` | `descartado` (`motivo`) | nenhuma |
| `nao_e_decisor` | `conectada` | `tentando` | 24 h; se veio `contato`, cria o contato decisor e põe o telefone dele no topo |
| `nao_atendeu` | `sem_resposta` | `tentando` | intervalo da cadência para a tentativa |
| `caixa_postal` | `caixa_postal` | `tentando` | intervalo da cadência |
| `ocupado` | `ocupado` | `tentando` | 1 h |
| `numero_errado` | `numero_errado` | `tentando`; sem outro telefone válido → `descartado` (`sem_telefone`) | marca o telefone `invalido`, bloqueia o número (`numero_errado`) e volta à fila na hora |
| `nao_ligar_mais` | `conectada` | `descartado` (`opt_out`) | bloqueia CNPJ e telefones, marca a empresa "não perturbe" |

Regras da cadência: só `nao_atendeu`, `caixa_postal` e `ocupado` contam tentativa; ao atingir `max_tentativas` o lead vai a `descartado` (`sem_contato`); no máximo `tentativas_dia` tentativas no mesmo dia (a seguinte vai para o próximo dia útil); toda próxima ação cai dentro da janela de contato.

**`reuniao`:** confere a agenda do dono da reunião (409 se o horário está ocupado, salvo `forcar`), cria a reunião (com contato, empresa e negócio), a atividade `reuniao`, tira o contato de sequências, cria o **negócio** (`origem = 'prospeccao'`, `lead_id`) no funil e etapa informados, ou os da lista, ou os das configurações, ou a primeira etapa cujo nome contém "reuni", ou a primeira etapa do primeiro funil. O dono do negócio é o dono da reunião.

**`proximo`:** só leads do usuário (e os sem dono, se a configuração permitir) em `novo`, `tentando` ou `retornar`, com próxima ação vencida, sem trava de outra pessoa e sem bloqueio. Ordem: retornos vencidos (mais antigo primeiro), depois tentativas vencidas, depois novos por nota. Pega com `FOR UPDATE SKIP LOCKED` e trava por 20 minutos (`em_atendimento_por`). **Fora da janela de contato devolve `lead: null` e `motivo_vazio: 'fora_da_janela'`.**

**Janela de contato:** dias úteis entre `janela_inicio` e `janela_fim` no fuso do ambiente; sábado só se ligado; domingos e feriados nacionais nunca. Teto do sistema: 8h às 20h.

### 8.3 Bloqueios ("não contatar")

`contato_bloqueios (tipo, valor, motivo, canal, observacao, criado_por, criado_em)` com `UNIQUE (tipo, valor)`. `valor` normalizado: telefone só dígitos com DDD; e-mail minúsculo; CNPJ normalizado. **Sobrevive à exclusão do lead** e é conferido na geração da lista, no envio ao CRM e na fila.

`GET /bloqueios` (`q`, `tipo`, `page`, `per_page`) → `{ data: Bloqueio[]; pagination }`; `POST /bloqueios` `{ tipo, valor, motivo, observacao? }`; `POST /bloqueios/importar` `{ tipo, motivo, valores: string[] (≤ 5.000) }` → `{ criados, repetidos, invalidos }`; `DELETE /bloqueios/{id}`. Leitura `prospecting.view` ou `leads.view`; escrita `prospecting.manage`.

```ts
interface Bloqueio { id: number; tipo: 'telefone' | 'email' | 'cnpj'; valor: string; motivo: 'opt_out' | 'numero_errado' | 'pedido_titular' | 'cliente' | 'manual' | 'lista_externa'; canal: string; observacao: string; criado_por_nome: string; criado_em: string }
```

### 8.4 Configurações de prospecção

`GET /settings/prospeccao` (qualquer um com `leads.view`) e `PUT` (`prospecting.manage`), guardadas em `app_settings` na chave `prospeccao`:

```ts
interface ConfigProspeccao {
  janela_inicio: number            // 9
  janela_fim: number               // 18
  sabado: boolean                  // false
  max_tentativas: number           // 6
  intervalos_horas: number[]       // [3, 24, 48, 72, 120]: espera depois da 1ª, 2ª, ... tentativa sem contato
  tentativas_dia: number           // 2
  retorno_padrao_horas: number     // 48
  meta_ligacoes_dia: number        // 40
  exigir_optin_whatsapp: boolean   // true
  incluir_sem_dono_na_fila: boolean // false
  pipeline_id: number | null; stage_id: number | null   // destino padrão da conversão
  playbook_id: number | null       // roteiro padrão
  motivos_descarte: string[]       // ["Sem interesse", "Já tem fornecedor", "Sem orçamento", "Fora do perfil", "Empresa fechada"]
}
```

**Roteiro:** o manual de atividades (`playbooks`) escolhido na lista, ou o padrão; no cockpit as variáveis são trocadas: `{{empresa}}`, `{{razao_social}}`, `{{cidade}}`, `{{uf}}`, `{{segmento}}`, `{{decisor}}`, `{{vendedor}}`, `{{minha_empresa}}`.

---

## 9. App: melhorias no funil e correções

**Próxima atividade (semáforo).** `GET /deals/board` passa a devolver em cada negócio `proxima_atividade_em: string | null` e `proxima_atividade_tipo: string` (a tarefa pendente mais próxima do negócio). No cartão: vermelho = atrasada, verde = hoje, cinza = futura, **amarelo = nenhuma atividade agendada**. Filtro rápido "Sem próxima atividade".

**Motivo de perda.** `PATCH /deals/{id}/close` aceita `{ won: false, motivo: string, observacao?: string }` e grava `lost_reason` e `lost_note`. `GET /settings/motivos-perda` → `{ motivos: string[]; exigir: boolean }` (padrão: "Preço", "Sem orçamento", "Escolheu concorrente", "Sem interesse", "Sem resposta", "Momento errado", "Outro"; `exigir: true`); `PUT` com `settings.pipelines`. Com `exigir`, perder sem motivo responde 422. `Deal` ganha `lost_reason`, `lost_note`, `lead_id`, `origem`. `GET /funil` ganha `perdas_por_motivo: { motivo: string; qtd: number; valor: number }[]` (perdidos no período).

**Correções de defeitos antigos que este fluxo tornaria visíveis:**

1. `companies.industry` era `VARCHAR(100)` e recebia descrições de CNAE maiores (erro 500 ao criar empresa de vários CNAEs do varejo): coluna alargada e texto truncado no tamanho da coluna ao aplicar campos.
2. CNPJ alfanumérico: validação, máscara e comparação (backend e frontend).
3. Sete `fetch` do frontend ignoravam o prefixo do cliente (`/<slug>`): exportações de contatos e negócios, download de anexo, formulário público e agendamento público.
4. Quatro telas liam `.data` de rotas que devolvem array (`/pipelines`, `/lists`): Previsão, Metas, Automações e Formulários ficavam com seletores vazios.
5. Botão "Concluir" do espaço de trabalho mandava `PATCH` sem corpo.
6. `InternoDesempenho` filtrava `won`/`lost` em vez de `ganho`/`perdido`.
7. Tarefa de sequência saía com `{{nome}}` cru no título.

---

## 10. Frontend

Rotas novas: `/prospeccao` (`prospecting`, `prospecting.view`), `/prospeccao/listas/:id` (`prospect-list`, `prospecting.view`), `/leads` (`leads`, `leads.view`), `/fila-de-ligacoes` (`call-queue`, `leads.edit`), `/configuracoes/prospeccao` (`settings-prospecting`, `prospecting.manage`). Menu: seção **Prospecção** com "Gerar listas", "Leads" e "Fila de ligações". Tipos em `src/types-prospeccao.ts`.

### 10.1 Gerar listas (`/prospeccao`)

Abas: **Nova lista**, **Minhas listas**, **Segmentos salvos**. Topo: medidor de cota ("1.240 de 5.000 leads usados em novembro") e a referência da base ("Receita 2026-11 · PGFN 2026-T3"); faixa âmbar quando a base é de demonstração; estado "base em carga, 37%" quando não está pronta.

Formulário em quatro blocos numerados, do macro ao micro (os dois últimos recolhidos):

1. **Que tipo de empresa?** Seletor de CNAE com busca por palavra ou código, múltipla seleção em chips, opção de escolher a divisão inteira (prefixo), chave "incluir CNAE secundário" (desligada).
2. **Onde?** UF (múltipla), cidades dependentes da UF com busca, e recolhido: bairros e CEP.
3. **Perfil da empresa:** porte, matriz ou filial, natureza jurídica, abertura (atalhos 30, 90, 180 dias e 1 ano, ou intervalo), capital (mínimo e máximo), Simples, MEI, dívida ativa (todas, com, sem, e faixa de valor), nome contém. Chave **"Incluir MEI e empresário individual"** desligada, com o aviso "são pessoas físicas: a LGPD se aplica ao contato".
4. **Qualidade do contato e ordem:** só com celular, só com telefone, só com e-mail, **ocultar contato de contador** (ligada por padrão), ordenação (melhores contatos, mais recentes, mais antigas, maior capital, maior dívida, aleatória).

Rodapé fixo com a **contagem ao vivo** (atraso de 400 ms, resposta atrasada descartada): "12.480 empresas · 7.910 com celular · 4.120 com e-mail", com "mais de 10.000" ou "até N" conforme `total_modo`. Botões "Ver prévia" (tabela de até 25 empresas, sem contatos), "Salvar segmento" e **"Gerar lista"**, que abre a confirmação: nome, quantidade (limitada ao total e à cota restante), exclusões ("pular empresas que já estão no CRM", "pular empresas de outras listas"), e a frase "esta lista usa N dos seus M leads restantes no mês". Depois de criar, vai para o detalhe da lista com o progresso.

Também: "Colar CNPJs" (lista específica, validada no navegador).

### 10.2 Lista (`/prospeccao/listas/:id`)

Cabeçalho com contadores (total, com celular, com decisor, enriquecidos, enviados) e o job em andamento com barra de progresso (consulta a cada 2 s, sobrevive a recarregar a página). Tabela de itens com seleção, "selecionar todos os N", filtros (estado, com celular, com decisor, nota mínima, busca), ordenação, nota com os motivos ao passar o mouse, selos (PF, dívida, WhatsApp, contador). Clicar abre a gaveta com a ficha: cadastro, contatos, sócios, dívida ativa, enriquecimento (com a origem de cada contato).

Ações: **Enriquecer** (modal com custo estimado, saldo do provedor, uso do mês e confirmação), **Enviar ao CRM** (assistente em três passos: o que criar e como tratar duplicados; distribuição com prévia "≈ N por pessoa" e carga diária opcional; resumo da prévia e confirmação), **Descartar**, **Exportar CSV**, e escolha do roteiro e do funil de destino da lista.

### 10.3 Leads (`/leads`)

Filas no topo (Meus, Sem dono, Todos) e chips de estado com contadores; tabela com semáforo de urgência, nota, empresa, cidade, telefone principal, tentativas, último resultado, próxima ação, dono. Seleção e ações em massa (atribuir, temperatura, etiqueta, descartar). Clicar abre o **painel contextual** sobre a lista, com teclas **J** (próximo), **K** (anterior) e **Esc**. Botão **"Começar a ligar"** leva à fila. Ações no painel: registrar resultado, converter em negócio, descartar, editar.

### 10.4 Fila de ligações (`/fila-de-ligacoes`)

Modo foco (sem menu lateral), um lead por vez, três colunas:

* **Quem é:** título, frases do resumo, sinais, cadastro (CNAE, porte, abertura, capital, endereço), sócios, dívida ativa, alertas (pessoa física, não perturbe, contador).
* **Ligar:** decisores e telefones em ordem, cada um com **ligar** (`tel:`), **copiar** e **WhatsApp** (`wa.me`, desabilitado sem opt-in quando a configuração exige); cronômetro que começa ao clicar em ligar; roteiro; campo de notas (rascunho guardado no navegador); **botões grandes de resultado com atalho de teclado** (1 Atendeu, 2 Reunião marcada, 3 Retornar, 4 Sem interesse, 5 Não é o decisor, 6 Não atendeu, 7 Caixa postal, 8 Ocupado, 9 Número errado, 0 Não ligar mais). Resultados que pedem mais um dado abrem só o que falta: data do retorno, motivo, novo decisor, ou a reunião (dia e horário livres do vendedor escolhido, com a agenda ao lado).
* **Histórico e progresso:** tentativas anteriores, barra do dia (feitas de meta), conectadas, reuniões.

Depois de salvar o resultado, **avança sozinho para o próximo lead**. Estados vazios: "fila em dia" e "fora da janela de contato (reabre às 9h)". Ao sair, solta a trava.

### 10.5 Demais telas

* **Configurações → Prospecção:** janela de contato, cadência, meta diária, WhatsApp com opt-in, destino padrão da conversão, roteiro padrão, motivos de descarte, e a lista de **bloqueios** (buscar, adicionar, importar colando, remover).
* **Configurações → Integrações de dados:** cartão do Who Lead (habilitar, demonstração, client id e secret, opções do lote, preço estimado, testar) e o provedor como quarta fonte no modal de enriquecer da ficha da empresa.
* **Negócios:** semáforo de próxima atividade no cartão, filtro "Sem próxima atividade", diálogo de motivo ao marcar perdido (quadro e ficha).
* **Funil:** perdas por motivo.
* **Espaço de trabalho:** atalho "Sua fila de ligações: N leads".

---

## 11. Salvaguardas de LGPD e de telemarketing já no produto

1. Pessoa física (MEI, empresário individual) fora das listas por padrão; incluir é escolha explícita e auditada; selo "PF" e alerta na ficha.
2. CPF nunca aparece: cortado da razão social na carga, mascarado em sócios e pessoas.
3. Origem carimbada em cada contato (`receita`, `wholead`, `manual`) e em cada lead (lista, data, base legal).
4. Lista de bloqueio por telefone, e-mail e CNPJ, que sobrevive à exclusão e é conferida ao gerar, ao enviar e ao ligar; "não ligar mais" em um clique na fila.
5. Janela de contato com teto do sistema, sem domingos e feriados; limite de tentativas por dia e por lead.
6. WhatsApp desabilitado sem opt-in registrado (configurável pelo gestor).
7. Dívida ativa só de pessoa jurídica e só do devedor principal, sempre com o trimestre de referência.
8. Trilha de auditoria: lista gerada (filtros, total, quantos de pessoa física), enriquecida (provedor, custo), exportada, enviada ao CRM; bloqueios criados e removidos.
9. Número com "não perturbe" informado pelo provedor vai para o fim da lista de telefones e aparece marcado.

O que **depende do cliente** (controlador dos dados) e não é resolvido por software: teste de legítimo interesse documentado, contrato de tratamento de dados com o provedor de enriquecimento, canal para pedidos de titulares e consulta aos cadastros estaduais de bloqueio de telemarketing.

---

## 12. Testes mínimos

* `dados`: leitores da Receita e da PGFN com recortes reais dos arquivos (ISO-8859-1, NUL, quebra de linha em aspas, `.0`, sentinelas); corte de CPF; regra de telefone; filtros → SQL; contagem exata, com teto e limite superior; cursor de versão antiga; modo amostra de ponta a ponta contra PostgreSQL real; downloader com servidor de teste que corta a conexão e devolve a página do firewall.
* Painel: cota (consumo atômico, estouro, idempotência por `pedido_id`), repasse com o `dados` simulado, erro com as duas chaves, páginas compilando.
* App backend: job de gerar lista com plataforma simulada (exclusões, cota); mapeador do Who Lead nos dois formatos e em formato desconhecido; lote com retomada pelo `consultation_id` e com 402; envio ao CRM (duplicado, bloqueado, rodízio, carga diária); máquina de estados dos resultados; `proximo` com trava e janela; conversão com reunião; CNPJ alfanumérico; motivo de perda obrigatório.
* Frontend: contagem ao vivo descartando resposta atrasada; gerar lista; progresso de job; seleção "todos"; modal de custo; assistente de envio; painel de leads com J e K; fila com atalhos, cronômetro e avanço automático; diálogo de motivo de perda; semáforo no cartão.
