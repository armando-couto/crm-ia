# Prospecção: fundação do frontend

O que já existe para as telas da prospecção. Contrato: `docs/PROSPECCAO.md` + `docs/PROSPECCAO-ADENDO.md` (o adendo prevalece). **Importe daqui; não recrie.** Testes: `tests/prospeccao-fundacao.test.ts` e `tests/correcoes-prefixo.test.ts`.

## Rotas, menu e permissões (prontos)

| Rota | Nome | Permissão | View (provisória, substitua o conteúdo) |
|---|---|---|---|
| `/prospeccao` | `prospecting` | `prospecting.view` | `views/ProspectingView.vue` |
| `/prospeccao/listas/:id` | `prospect-list` | `prospecting.view` | `views/ProspectListDetailView.vue` |
| `/leads` (`?lead=<id>`) | `leads` | `leads.view` | `views/LeadsInboxView.vue` |
| `/fila-de-ligacoes` (`?lista=<id>`) | `call-queue` | `leads.edit` | `views/CallQueueView.vue` |
| `/configuracoes/prospeccao` | `settings-prospecting` | `prospecting.manage` | `views/SettingsProspectingView.vue` |

* Menu lateral (`AppShell.vue`): seção **Prospecção** com "Gerar listas", "Leads de prospecção" e "Fila de ligações"; a seção aparece com `prospecting.view` ou `leads.view` e cada link tem o `v-if` da sua permissão.
* Menu de configurações (`SettingsLayout.vue`): "Prospecção", com `prospecting.manage`, que também entrou em `auth.canManage`.
* O `<router-view>` não tem `:key`: observe `route.params.id` / `route.query` com `watch` e recarregue.

## Tipos: `src/types-prospeccao.ts`

Todos os tipos do contrato já com as correções do adendo: `Filtros`, `Telefone`, `Empresa`, `EmpresaPrevia`, `Contagem` (só `exato | teto`), `PreviaResposta` (`ordem_aplicada`), catálogos (`CatalogoCnae` com `codigo: string` e `nivel`, `CatalogoMunicipio`, `CatalogoNatureza`, `CatalogoUf`), `Cota`, `ResumoPlataforma`, `Job`, `Segmento`, `CriarLista`, `Lista`, `ListaDetalhe`, `Item` (`contato_compartilhado`), `Selecao` (`exceto`), `WhoLeadSettings`, `PerfilContato`, `Pessoa`, `EnriquecerEstimativa`, `EnviarRequest`, `EnviarPrevia`, `Lead`, `LeadCompleto`, `ContadoresLeads`, `LeadsResposta`, `LeadLista` (`GET /leads/listas`), `AtualizarLead` (`whatsapp_optin`), `LeadsBulkRequest`, `ResultadoRequest` (`chave`), `ResultadoResposta`, `ResumoFila`, `ProximoRequest` (`pular_ids`), `ProximoResposta`, `Disponibilidade` (horários `{ inicio, rotulo }` e `ocupados`), `Bloqueio`, `ConfigProspeccao`, `MotivosPerda`, `PerdaRequest`, `DealProspeccao`, `ProximaAtividade`, `PerdaPorMotivo`, `ErroProspeccao` e `CodigoErro`.

Constantes e funções: `temFiltroObrigatorio(filtros)`, `FILTROS_OBRIGATORIOS`, `jobAtivo(job)`, `STATUS_JOB_ATIVOS`, `ESTADOS_LEAD_ABERTOS`, `RESULTADOS` (ordem dos botões com a tecla 1…0), `ORDENS`, `CONFIG_PROSPECCAO_PADRAO`, `VARIAVEIS_ROTEIRO`, `PERMISSOES_PROSPECCAO`.

Rótulos (`Record<enum, string>`): `porteLabels`, `ordemLabels`, `matrizLabels`, `simNaoLabels`, `dividaFiltroLabels`, `tipoTelefoneLabels`, `origemContatoLabels`, `emailTipoLabels`, `tipoDividaLabels`, `tipoSocioLabels`, `nivelCnaeLabels`, `estadoCargaLabels`, `tipoJobLabels`, `statusJobLabels`, `origemListaLabels`, `statusListaLabels`, `estadoItemLabels` (+ `estadoItemTons`), `ordemItensLabels`, `papelPessoaLabels`, `duplicadosLabels`, `modoDistribuicaoLabels`, `temperaturaLabels`, `origemLeadLabels`, `filaLeadsLabels`, `ordemLeadsLabels`, `acaoLeadsEmMassaLabels`, `tipoAlertaLabels`, `nivelTons`, `tipoProximoPassoLabels`, `motivoFilaVaziaLabels`, `tipoBloqueioLabels`, `motivoBloqueioLabels`, `codigoErroLabels`. Os "tons" são classes de `.badge` (`green`, `red`, `amber`, `blue`, `gray`).

## `src/format.ts` (acréscimos)

* Rótulos que o adendo manda viver **só aqui**: `resultadoLabels`, `estadoLeadLabels` (+ `estadoLeadTons`), `urgenciaLabels` (+ `urgenciaTons`: vermelho, verde, cinza, amarelo), `motivoDescarteLabel(motivo)`.
* `urgenciaDe(instante, agora?)` → `Urgencia` (semáforo do cartão do negócio a partir de `proxima_atividade_em`).
* `formatNumber(n)` ("12.480"), `formatCentavos(c)` ("R$ 80,50"), `formatDuracao(seg)` ("1:15"), `formatCompetencia('2026-11')` ("novembro"), `formatContagem(contagem)` ("mais de 10.000, até 48.200").
* Reexporta de `cnpj.ts`: `normalizarCNPJ`, `cnpjValido`, `mascaraCNPJ`, `separarCNPJs`.

## `src/cnpj.ts` (CNPJ alfanumérico)

`normalizarCNPJ(v)` (maiúsculas, `[0-9A-Z]`, 14), `cnpjValido(v)` (regra ASCII − 48), `formatarCNPJ(v)` = `mascaraCNPJ(v)` (máscara progressiva com letras), `ehCNPJ(v)`, `formatarDocumento(v)`, `separarCNPJs(texto)` → `{ validos, invalidos, repetidos }` para "Colar CNPJs" (a tela recusa mais de 5.000). `somenteDigitos` continua existindo só para CPF, CEP e telefone. `components/intel/CnpjField.vue` já aceita letras.

## `src/telefone.ts`

| Função | Uso |
|---|---|
| `normalizarTelefone(v)` | só dígitos com DDD (tira máscara, DDI 55 e zero de operadora) |
| `formatarTelefone(v)` | `(85) 99999-0001` / `(85) 3000-0001` |
| `telefoneValido(v)`, `tipoPeloNumero(v)` | validação e tipo pelo tamanho |
| `linkTel(v)` | `tel:+55<numero>` |
| `linkWhatsApp(v, texto?)` | `https://wa.me/55<numero>`; vazio fora de celular (11 dígitos) |
| `comNonoDigito(v)`, `opcoesDiscagem(tel)` | telefone `indefinido`: duas opções de discagem (sem e com o 9) |
| `podeLigar(tel, { amostra })` | falso para `invalido` e para base de demonstração |
| `podeWhatsApp(tel, { exigirOptin, optin, amostra })` | regra do botão WhatsApp (opt-in, tipo, `whatsapp: false`, demonstração) |
| `agruparTelefones(lead.telefones, resumo.decisores)` | coluna "Ligar": grupos por `pessoa` (vazio = "Empresa") com `cargo` |
| `telefonePrincipal(telefones)` | `numero` padrão do resultado quando não houve clique em "ligar" |

## `src/api.ts`

* `ApiError`: `status`, `message`, **`data`** (corpo JSON do erro), `codigo` (atalho de `data.codigo`) e `retryAfter` (cabeçalho `Retry-After` ou `tentar_em_segundos`). Ex.: `if (e instanceof ApiError && e.codigo === 'job_ativo') polling.acompanhar(e.data.job)`.
* Todas as chamadas aceitam opções no último argumento: `api.get(path, { signal })`, `api.post(path, body, { signal, keepalive })`, idem `put`, `patch`, `delete`. `foiAbortado(e)` reconhece o `AbortError`.
* `api.download(path, nomeArquivo)`: baixa de rota autenticada por `apiBase()` (ex.: `api.download(`/prospeccao/listas/${id}/exportar`, 'lista.csv')`). Erro vira `ApiError` com o corpo.
* `publicApiBase()`: raiz de `/api/public` com o prefixo do cliente.
* Soltar a trava no `pagehide`: `api.post(`/leads/${id}/liberar`, undefined, { keepalive: true }).catch(() => {})`.

## Componentes: `src/components/ui/`

| Componente | Props | Emits | Slots |
|---|---|---|---|
| `PaginationBar.vue` | `page`, `perPage`, `total`, `perPageOptions?: number[]`, `disabled?`, `rotulo?` ("leads") | `update:page`, `update:perPage` | — |
| `ProgressBar.vue` | `valor`, `total?`, `rotulo?`, `mostrarNumeros?` (padrão `true`), `tom?` (`'' \| green \| amber \| red \| blue`), `indeterminado?`, `fina?` | — | `default` (rótulo) |
| `StepperSteps.vue` | `passos: string[]`, `atual: number` (base 0), `clicavel?` (voltar a passo concluído) | `update:atual` | — |
| `SideDrawer.vue` | `open`, `title?`, `largura?` (px, 520), `fundo?` (padrão `true`), `fecharComEsc?` (padrão `true`) | `close` | `default`, `title`, `actions`, `footer` |

`PaginationBar` e `StepperSteps` funcionam com `v-model:page` / `v-model:atual`. `SideDrawer` usa `Teleport` para o `body`, é `aria-modal="false"` (os atalhos da tela por baixo continuam valendo) e não fecha com Esc se houver um `ModalDialog` aberto por cima. Para modais continue usando `components/ModalDialog.vue`.

## Composables: `src/composables/`

* **`useJobPolling({ intervalo = 2000, aoMudar?, aoTerminar? })`** → `{ job, ativo, consultando, erro, percentual, acompanhar(job | id), parar(), cancelar() }`. Consulta `GET /prospeccao/jobs/{id}`, uma chamada por vez; `aoMudar` dispara a cada mudança de `feitos`, `erros` ou `status` (hora de recarregar a lista e a página de itens); para sozinho ao terminar e **limpa tudo ao desmontar** (`onScopeDispose`). Ao abrir a tela: `const ativo = detalhe.jobs.find(jobAtivo); if (ativo) polling.acompanhar(ativo)`.
* **`useConsultaAdiada(fn(args, signal), { atraso = 400, repetirEmLimite = true, aoErro?, aoResultado? })`** → `{ dados, carregando, erro, pedir(args), agora(args), limpar() }`. Contagem ao vivo: espera o usuário parar, cancela a consulta anterior, **descarta resposta atrasada** e, em 429 `muitas_requisicoes`, mantém o último número e tenta de novo depois de `Retry-After`, sem erro.
* **`useModoFoco()`**: põe `modo-foco` no `body` no `onMounted` e tira no `onBeforeUnmount`.
* **`useAtalhos(mapa, { ativo?, sempre? })`**: atalhos por tecla em minúsculas (`'1'`, `'j'`, `'s'`, `'escape'`), ignorados com foco em `input`/`textarea`/`select`, com modal aberto e com Ctrl/Alt/Cmd. Também exporta `focoEmCampo()` e `modalAberto()`.

## `src/styles.css` (acréscimos globais)

* `body.modo-foco .sidebar, body.modo-foco .topbar { display: none !important }` (vale também para `body.modo-reuniao`).
* `.semaforo` (+ `.red`, `.green`, `.amber`, `.gray`), `.faixa` (+ `.amber`, `.red`) para avisos como "dados de demonstração", `.chip` (+ `.active`, `.chip-x`), `.tecla` (dica de atalho) e `.telefone-invalido` (riscado).

## Convenções para as próximas telas

* Chaves de `localStorage` sempre com `chaveLocal(...)` de `src/tenant.ts` (ex.: `chaveLocal('fila_notas_' + lead.id)`).
* Nunca `fetch` com caminho fixo: use `api.*`, `api.download` ou `publicApiBase()`.
* `/pipelines`, `/lists` e `/users` devolvem **array puro**.
* `reuniao.inicio` é sempre o `inicio` RFC 3339 de `/agenda/disponibilidade`.
