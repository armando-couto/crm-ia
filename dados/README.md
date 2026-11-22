# dados — base pública de empresas do CRM IA

Serviço interno que carrega a base aberta do **CNPJ** (Receita Federal, mensal) e a **dívida ativa** (PGFN, trimestral) e responde contagem, busca e ficha para o painel. É a origem das listas de prospecção.

Contrato: [docs/PROSPECCAO.md](../docs/PROSPECCAO.md), seção 3, com as correções de [docs/PROSPECCAO-ADENDO.md](../docs/PROSPECCAO-ADENDO.md).

* Módulo `github.com/armando-couto/crm-ia/dados`, container `crmia-dados`, banco `crmia_dados`, porta 7896.
* Só rede interna: o painel chama `http://dados:7896` com `Authorization: Bearer <DADOS_TOKEN>`. Sem rota no Traefik.
* Dependências: `pgx/v5` e `golang.org/x/text`. Nenhuma extensão do PostgreSQL é exigida.

## Rodar

```sh
# desenvolvimento: base de demonstração, sem baixar nada
DATABASE_URL='postgres://postgres:postgres@localhost:5432/crmia_dados?sslmode=disable' \
DADOS_TOKEN=$(openssl rand -hex 32) DADOS_AMOSTRA=true APP_ENV=development go run .

go run . -migrate       # só aplica as migrações
go run . -carga         # roda uma carga (ou monta a amostra) e sai; -fonte cnpj|pgfn|tudo
go run . -healthcheck   # consulta /saude (usado pelo Docker)
```

Testes: `go test -race ./...`. Os de integração usam PostgreSQL de verdade (`CRMIA_TEST_PG`, padrão `postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable`) e criam os bancos `crmia_test_dados_carga`, `crmia_test_dados_busca` e `crmia_test_dados_api`; sem banco, são pulados.

## Configuração

| Variável | Padrão | Para quê |
|---|---|---|
| `PORT` | `7896` | porta HTTP |
| `APP_ENV` | `production` | `development` liga o log de depuração |
| `DATABASE_URL` | — | PostgreSQL 16 (obrigatória) |
| `DADOS_TOKEN` | — | token Bearer, mínimo de 24 caracteres (obrigatório) |
| `DADOS_CNPJ_URL` | WebDAV da Receita | pasta raiz das referências `AAAA-MM`; o token do compartilhamento pode mudar |
| `DADOS_PGFN_URL` | `https://dadosabertos.pgfn.gov.br/` | raiz das pastas `AAAA_trimestre_NN` (só `https`) |
| `DADOS_DIR_TRABALHO` | `/trabalho` | onde ficam os zips até a troca de base |
| `DADOS_CARGA_AUTOMATICA` | `true` | confere as fontes na subida e a cada 6 h |
| `DADOS_AMOSTRA` | `false` | serve a base fictícia embutida; ignora a carga automática e nunca acessa a rede |
| `DADOS_UFS` | vazio (Brasil) | recorte de UFs (`CE,SP`): menos disco e menos tempo |
| `DADOS_CONEXOES` | `2` | downloads simultâneos (1 a 4) |
| `DADOS_DISCO_MINIMO_GB` | `70` | espaço livre que o operador declara existir no disco do PostgreSQL (o serviço não enxerga o disco do host) |
| `DADOS_TEMPO_BUSCA` | `4s` | `statement_timeout` das consultas |
| `DADOS_CARGA_INTERVALO`, `DADOS_ESPERA_PUBLICACAO`, `DADOS_ESPACO_REQUISICOES` | `6h`, `30m`, `2.5s` | ajustes finos da carga |

## API

Erros respondem `{"erro": "..."}`: 401 sem token, 422 filtro inválido ou amplo demais, 409 cursor de versão antiga (ou carga em curso), 503 base ainda não carregada, 404 CNPJ fora da base de ativas.

| Rota | Corpo ou consulta | Resposta |
|---|---|---|
| `GET /saude` | — | `{"status":"ok"}` (sem token) |
| `GET /v1/situacao` | — | base em uso (`pronta`, `amostra`, referências, `versao`, totais, `espaco_bytes`, `ufs`) e `carga` |
| `GET /v1/catalogos/cnaes` | `q`, `nivel`, `codigos`, `limite` (≤ 50) | `{ itens: { codigo, descricao, nivel }[] }`: divisão, grupo, classe e subclasse |
| `GET /v1/catalogos/municipios` | `uf`, `q`, `codigos`, `limite` (≤ 50) | `{ itens: { codigo, nome, uf, empresas }[] }`, do maior para o menor |
| `GET /v1/catalogos/naturezas` | — | `{ itens: { codigo, descricao, pessoa_fisica }[] }` |
| `GET /v1/catalogos/ufs` | — | `{ itens: { uf, nome, empresas }[] }` |
| `POST /v1/empresas/contagem` | `{ filtros }` | `Contagem` |
| `POST /v1/empresas/previa` | `{ filtros, ordem?, limite? (≤ 25) }` | `{ itens, versao, ordem_aplicada }` |
| `POST /v1/empresas/candidatos` | `{ filtros, ordem?, limite? (≤ 5000), cursor? }` | `{ cnpjs, cursor, fim, versao, ordem_aplicada }` |
| `POST /v1/empresas/detalhes` | `{ cnpjs (≤ 500), incluir_pf? }` | `{ itens: Empresa[], nao_encontrados }` |
| `GET /v1/empresas/{cnpj}` | `incluir_pf` | `Empresa` ou 404 |
| `POST /v1/cargas` | `{ fonte: cnpj \| pgfn \| tudo }` | 202 `{ ok: true }`; 409 com carga em curso ou em modo amostra |

Regras da busca:

* **Filtros.** É obrigatório ao menos um entre `ufs`, `municipios`, `cnaes`, `cnae_prefixos` e `cnpjs`. Tudo vale para o estabelecimento (CNPJ de 14). Campos diferentes combinam com E (UF e município: interseção). Pessoa física (MEI, empresário individual, naturezas 4000 a 4999) fica fora sem `incluir_pf`; `mei: "sim"` sem ele responde 422. Com `cnpjs` os demais filtros são ignorados, exceto `incluir_pf`. `nome`, `bairros` e `ceps` exigem `municipios` ou CNAE.
* **Contagem.** Exata pelos cubos quando o filtro só usa lugar, CNAE e os sinais (porte, MEI, Simples, pessoa física, celular, telefone, e-mail, dívida, matriz). Fora disso, exata na tabela de busca enquanto o limite superior dado pelos cubos for de até 1.000.000; acima, conta com teto: `total_modo: "teto"`, `total: 10000` ("mais de 10.000"), `total_limite_superior` quando há e `parciais` nulas. Estouro de tempo também vira teto. A estimativa do planejador nunca é usada.
* **Ordem.** Só em conjunto pequeno (total exato ou limite superior de até 200.000) a ordem pedida é aplicada (`relevancia` = `qualidade` decrescente e `id`; `aleatoria` usa uma semente guardada no cursor). Fora disso a ordem é a da base (`id` dentro da faixa da UF ou do município) e `ordem_aplicada` vem `"base"`.
* **Paginação por chave**, nunca `OFFSET`. O cursor é opaco e carrega a versão da base: depois de uma troca responde 409 `{"erro":"a base foi atualizada; refaça a busca"}`.
* **Pessoa física e CPF.** Sem `incluir_pf`, `detalhes` e `empresas/{cnpj}` devolvem pessoa física sem telefones, sem e-mail e sem sócios. CPF completo não é gravado nem devolvido: é cortado dos nomes na carga e o documento de sócio segue mascarado.

## Carga

Uma por vez (trava consultiva no banco), retomável, em segundo plano no mesmo processo ou pela flag `-carga`. Erro de carga nunca derruba o serviço: fica em `situacao.carga` e a API continua com a base anterior.

1. **Descobrir.** CNPJ: `PROPFIND` na raiz e na pasta do mês. A referência só é elegível com os 37 arquivos esperados e duas listagens iguais (tamanho e ETag) separadas por 30 minutos; antes disso a etapa fica "aguardando publicação completa" e nada é baixado. PGFN: índice HTML; a pasta `AAAA_trimestre_NN` vira `AAAA-TN` e precisa dos três zips (o não previdenciário é o que não é FGTS nem Previdenciário).
2. **Baixar** para `DADOS_DIR_TRABALHO`, em `.part`, com no máximo `DADOS_CONEXOES` transferências e 2,5 s entre requisições novas. A retomada manda `Range` + `If-Range` com o ETag guardado; 200 no lugar de 206 recomeça o arquivo. A página "Request Rejected" do firewall (HTTP 200 com HTML) é reconhecida pelo `Content-Type` e pelos bytes `PK`, e não toca o `.part`. Antes de baixar há conferência de espaço: 10 GB livres no diretório de trabalho e `DADOS_DISCO_MINIMO_GB` de ao menos 70 (20 com `DADOS_UFS`).
3. **Ler em fluxo** o único membro de cada zip: remove bytes `0x00`, converte ISO-8859-1, valida com `encoding/csv` estrito (número de campos fixo, quebra de linha dentro de aspas aceita) e envia por `COPY` (`pgx`) para tabelas `UNLOGGED`, tudo `text`, no esquema `carga`. Número de campos errado aborta a referência. A limpeza é feita em Go neste passo (pacote `internal/texto`): só estabelecimentos ativos (situação `02`) e do recorte de UFs entram; CPF cortado de razão social, nome fantasia e nome de sócio; telefone classificado; e-mail validado; sócios de faixa etária 1 e 2 descartados; colunas de busca sem acento. A PGFN tem leitor próprio, linha a linha: cabeçalho conferido, só pessoa jurídica e devedor principal, inscrição sem `.0`, sentinela `01/01/1000` nula, nome do devedor nunca gravado, aborto acima de 1 rejeito em 100 mil.
4. **Guardas:** linhas fantasma de Empresas, `cnpj_basico` único, CNPJ único, queda de mais de 20% contra a base em uso.
5. **Construir** `prospec_novo` ao lado do que está em uso (`internal/carga/construcao.go`) e fazer `VACUUM (ANALYZE)` em tudo.
6. **Trocar** numa transação (`prospec` → `prospec_velho`, `prospec_novo` → `prospec`), aquecer (`pg_prewarm` se a extensão existir, senão leitura), apagar o esquema velho, as tabelas de carga e os zips.
7. **Só a PGFN mudou:** a dívida é recarregada e a construção parte de `prospec.base_ativa`, sem baixar a Receita.

Retomada: o estado de cada arquivo fica em `carga_arquivos`. Na subida, arquivo "carregado" cuja contagem na tabela de carga não bate com o controle (tabelas `UNLOGGED` são esvaziadas depois de desligamento sujo) volta a "baixado" e é relido do zip. Construção interrompida apaga `prospec_novo` e recomeça. `SIGTERM` encerra de forma limpa e a próxima subida retoma, mesmo com `DADOS_CARGA_AUTOMATICA=false`.

### Esquema de serviço (`prospec`)

| Tabela | Conteúdo |
|---|---|
| `base_ativa` | a ficha: estabelecimentos ativos já limpos, sem os sinais de dívida; gravada em ordem `(uf, municipio, cnae, cnpj)`, com `id` igual à posição. Guarda o telefone cru da Receita e `regra_telefone` |
| `busca` | tabela enxuta só com colunas de filtro; índices de cobertura `busca_c1` (uf, município, CNAE, sinais) e `busca_c2` (CNAE, uf, município, sinais) e GIN em `cnaes` |
| `socio`, `divida` | sócios das empresas ativas; dívida agregada por CNPJ de 14 |
| `cubo_mun`, `cubo_uf_cnae`, `cubo_cnae_qq` | cubos de contagem |
| `faixa_uf`, `faixa_mun` | faixa de `id` por UF e por município |
| `cnae`, `municipio`, `natureza`, `meta` | catálogos e a versão da base |

Fora dele: `pgfn.divida` (dívida de todos os CNPJs, guardada entre as cargas mensais) e as tabelas de controle `cargas`, `carga_arquivos` e `publicacoes`.

### Regras de limpeza

* **Telefone** (a Receita publica 8 dígitos): começa por 6 a 9 → celular, repõe o 9 (11 dígitos); 2 a 5 → fixo (10 dígitos), exceto DDD 11 com 4 ou 5 → `indefinido` (10 dígitos); 1 → celular só no DDD 11; descarta dígito repetido, sequência, `X0000000`, começo por 0, menos de 8 dígitos e DDD inexistente.
* **Contato compartilhado:** telefone em 5 ou mais raízes de CNPJ ativas → `compartilhado`. E-mail com `contab`, `contador`, `escritorio` ou `assessoria`, ou em 5 ou mais raízes → `contabilidade`; provedor gratuito → `gratuito`.
* **Qualidade** (0 a 100): 60 com celular próprio; 35 com só fixo próprio (indefinido conta como fixo); +20 e-mail que não é de contabilidade; +10 segundo telefone; contato só compartilhado vale 15; sem contato, 0.
* **Pessoa física:** natureza 2135, naturezas 4000 a 4999 ou optante do MEI.

## Modo amostra

`DADOS_AMOSTRA=true` monta o esquema `prospec_amostra`, separado do real, pelo mesmo passo de construção: 99 raízes de CNPJ (00.000.001 a 00.000.099, com dígitos verificadores válidos) e cerca de 3.300 estabelecimentos em 22 UFs, com sócios e dívidas. Telefones com DDD real e assinante `90000-0NNN` (celular) ou `3000-0NNN` (fixo), e-mails em `@exemplo.invalid`, sócios com nomes genéricos numerados. Só os catálogos são os oficiais. `versao` começa por `amostra-`, `situacao.amostra` vem `true` e `POST /v1/cargas` responde 409.

## Imagem

Distroless `nonroot`. O diretório `/trabalho` é criado no estágio de build e copiado com dono `65532`, para o volume nomeado (`crmia_dados_trabalho`) nascer gravável. Limite de memória sugerido: 512 MB (toda leitura é em fluxo).
