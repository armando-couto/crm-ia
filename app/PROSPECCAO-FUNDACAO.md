# Prospecção: fundação do backend do app

O que já existe para os próximos agentes construírem em cima. Contrato: [docs/PROSPECCAO.md](../docs/PROSPECCAO.md) e [docs/PROSPECCAO-ADENDO.md](../docs/PROSPECCAO-ADENDO.md) (o adendo vence). Tudo abaixo está coberto por teste (`go test -race ./...` com `CRMIA_TEST_DSN`).

## Como usar (leia antes das assinaturas)

**Rotas e handlers.** Todas as rotas das seções 6 a 9 já estão em `routes/routes.go`, com a permissão certa, apontando para handlers que respondem 501 (`naoImplementado`). Implementar = trocar o corpo do handler no arquivo do dono; **não mexa em `routes.go`** nem renomeie o handler. `controllers/prospeccao_rotas_test.go` confere que cada rota existe, dá 401 sem sessão e 403 para o perfil sem a permissão (ele não olha o corpo da resposta, então continua verde quando o handler for implementado).

| Arquivo | Handlers |
|---|---|
| `controllers/prospeccao_controller.go` | `ProspeccaoResumo`, `ProspeccaoCatalogo`, `ProspeccaoContagem`, `ProspeccaoPrevia`, `ListProspSegmentos`, `CreateProspSegmento`, `UpdateProspSegmento`, `DeleteProspSegmento`, `ListProspListas`, `CreateProspLista`, `GetProspLista`, `UpdateProspLista`, `DeleteProspLista`, `ListProspItens`, `GetProspItem`, `DescartarProspItens`, `RestaurarProspItens`, `ExportarProspLista`, `GetProspJob`, `CancelarProspJob` |
| `controllers/prospeccao_enriquecer_controller.go` | `EnriquecerProspEstimativa`, `EnriquecerProspLista` |
| `controllers/prospeccao_enviar_controller.go` | `EnviarProspPrevia`, `EnviarProspLista` |
| `controllers/leads_controller.go` | `ListLeads`, `CreateLead`, `ListLeadListas`, `GetLead`, `UpdateLead`, `DeleteLead`, `BulkLeads`, `ConverterLead`, `DescartarLead`, `ReabrirLead` |
| `controllers/fila_controller.go` | `ResultadoLead`, `LiberarLead`, `FilaResumo`, `FilaProximo`, `AgendaDisponibilidade` |
| `controllers/bloqueios_controller.go` | `ListBloqueios`, `CreateBloqueio`, `ImportarBloqueios`, `DeleteBloqueio` |
| `controllers/prospeccao_config_controller.go` | `GetConfigProspeccao`, `UpdateConfigProspeccao` |
| `controllers/deals_perda_controller.go` | `GetMotivosPerda`, `UpdateMotivosPerda` |

Permissões que **o handler** confere (a rota não consegue): `acao = "dono"` em `POST /leads/bulk` exige `leads.assign`; `owner_id` em `POST`/`PUT /leads` de quem não tem `leads.assign` só pode ser o próprio usuário; `user_id` de outra pessoa em `GET /agenda/disponibilidade`.

**Nomes.** Tipos da base pública levam o prefixo `Prosp` (`ProspFiltros`, `ProspEmpresa`, `ProspItem`, `ProspLista`, `ProspJob`...). `Lead`, `Bloqueio`, `ConfigProspeccao`, `MotivosPerda`, `Janela`, `Tentativa`, `Sinal`, `ResumoLead`, `PerfilContato` e `PessoaContato` não levam prefixo. As chaves JSON são as do contrato.

**`models.Executor`.** Interface com `Exec`/`Query`/`QueryRow`: aceita `*sql.DB` e `*sql.Tx`. Toda função de escrita que recebe `Executor` pode entrar na transação de quem chama (é assim que o resultado da ligação grava chamada + lead + contato de uma vez).

**CNPJ.** Sempre `models.NormalizarCNPJ` antes de comparar ou gravar em tabela nova; `models.CNPJValido` valida (regra alfanumérica). Em SQL sobre `companies` use `models.CNPJNormSQL` (alias `c`) ou `models.CNPJNormSQLColuna("cnpj")`: é a expressão do índice `idx_companies_cnpj_norm`. Nos provedores de enriquecimento use `enriquecimento.NormalizarCNPJ` (não `SomenteDigitos`) e `enriquecimento.CNPJSoNumeros` para responder `FalhaSemResultado` quando o provedor não aceita letras.

**Plataforma (`services/plataforma.go`).** `Plataforma*` são as chamadas tipadas; todo erro delas é `*services.ErroPlataforma`, já com o status e a mensagem que o app devolve. No controller:

```go
if e, ok := services.ComoErroPlataforma(err); ok {
	if e.TentarEmSegundos > 0 {
		ctx.Header("Retry-After", strconv.Itoa(e.TentarEmSegundos))
	}
	ctx.StopWithJSON(e.Status, e.Corpo()) // {"error", "codigo", "cota"?, "tentar_em_segundos"?}
	return
}
```

No worker passe `context.Background()` com prazo próprio (nunca o contexto da requisição) e deixe o erro subir: `ErroPlataforma.Transitorio()` já faz o worker tentar de novo. Em testes: `t.Cleanup(services.UsarPlataformaDeTeste(servidorDeTeste.URL))`; o servidor recebe os caminhos sem `/api/empresas/<slug>` (`/prospeccao/contagem`, `/prospeccao/extracoes`...).

**Jobs e worker (`services/prospeccao_worker.go`).** O handler HTTP só insere o job (`models.CreateProspJob` ou `models.CreateProspListaComJob`) e responde; `models.ErrJobAtivo` vira 409 `{error, codigo: "job_ativo", job}` (pegue o job com `models.ProspJobAtivoDaLista`). Cada tipo registra o seu processador uma vez:

```go
// services/prospeccao_enriquecer.go
func init() { RegistrarProcessadorJob(models.JobEnriquecer, processadorEnriquecer{}) }

func (processadorEnriquecer) Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error) {
	var par parametrosEnriquecer // o que o handler gravou em job.Parametros
	var est estadoEnriquecer     // cursor de retomada (vazio na 1ª fatia)
	_ = e.Parametros(&par)
	_ = e.Estado(&est)
	// ... trabalhar UM pedaço curto ...
	_ = e.SalvarEstado(est)          // ANTES de qualquer efeito que não pode repetir
	_ = e.Progresso(feitos, erros, custoCentavos) // deltas
	return Fatia{Espera: 5 * time.Second, Mensagem: "aguardando o provedor"}, nil
	// no fim: return Fatia{Concluido: true, Mensagem: "...", Resultado: resumo}, nil
}
```

Regras do worker: uma fatia por vez, curta (prazo de 2 min no contexto); `Fatia.Espera` adia a próxima; erro comum → job `falhou`; erro transitório (`services.Transitorio(err)`, `ErroPlataforma` transitório, `context.DeadlineExceeded`) → nova tentativa em 30 s, 2 min e 10 min, e `falhou` na quarta seguida; pânico é recuperado e tratado como transitório. Implemente também `EncerradorJob.Encerrar(ctx, e, status)` se o tipo precisa arrumar algo quando o job termina em `falhou` ou `cancelado` (gerar_lista: lista `pronta` parcial ou `falhou`; enriquecer: ler e gravar o lote já pago). Cancelar (`models.CancelarProspJob`) marca `cancelado` na hora; a fatia em curso não consegue mais concluir nem reagendar o job, e o worker chama `Encerrar` uma vez. `e.Cancelado()` serve para parar cedo no meio de uma fatia. O custo do enriquecimento é o `custoCentavos` passado a `e.Progresso` (fica em `prospeccao_jobs` e sobrevive à exclusão da lista: `models.CustoEnriquecimentoJobs`). O worker só sobe em `application.go`; em teste, rode as fatias com `services.ProcessarProximoJob` ou `services.ProcessarJobsAte` e zere `services.ProspeccaoEsperas`.

**Seleção.** `models.ProspSelecao` é o JSON `Selecao` (com `exceto`). `Validar()` → 422. Para processar: `ResolveProspSelecao(db, listaID, sel, estados, aposID, limite)` em páginas por id (guarde o último id no estado do job) e `ProspItensCompletos(db, ids)` para ler os itens com `dados` e `enriquecimento`. Prévia e estimativa: `ResumoProspSelecao` (contagem por estado, PF e sem telefone) e `CountProspSelecao`.

**Envio ao CRM (idempotência).** Depois de criar a empresa: `SetProspItemEmpresa` (mantém o estado). Lead: `CreateLead` com `ItemID` devolve `criado = false` e o id existente se o item já tem lead. `LeadAbertoDaEmpresa` responde a regra de `duplicados = "vincular"`. Fim do item: `SetProspItemEstado`.

**Leads e visibilidade.** Monte `models.LeadVisao{UserID, VeTodos: models.RoleCan(role, models.PermLeadsAssign)}` e passe a toda consulta: fora do predicado (meu, sem dono ou de dono inativo) `LeadByID` devolve `sql.ErrNoRows` (404) e `BulkLeads` ignora o id. `ListLeads` aplica o predicado mesmo com `Fila = "todos"`; se o contrato pedir 403 para `fila=todos` ou `owner_id` alheio sem `leads.assign`, é o handler que responde. Jobs usam `models.VisaoTotal(userID)`.

**Fila.** `ProximoLeadDaFila` não olha a janela (confira antes com `ConfigProspeccao.JanelaAberta`). `POST /leads/{id}/resultado`: abra a transação, chame `PrepararResultadoLead(tx, id, visao)` (trata 404, 409 `ErrLeadEncerrado` e 423 `*ErroLeadEmAtendimento`, e solta a trava), grave a chamada com `CreateCallTx` (com `Chave`; `ErrChamadaRepetida` = devolver a resposta original via `CallByChave`, sem abortar a transação) e o lead com `UpdateLead(tx, id, LeadPatch{...})`. Cadência: `ConfigProspeccao.IntervaloCadencia`, `AjustarJanela`, `ProximoDiaPermitido` e `TentativasSemContatoDesde`. Tabelas de apoio: `OutcomeDoResultado`, `ResultadoConectado`, `ResultadoContaTentativa`.

**Bloqueios.** `ConferirBloqueios(db, cnpj, telefones, emails)` responde tudo de uma empresa numa chamada; `CNPJsBloqueados` é o que `gerar_lista` usa. Bloqueio criado pelo resultado da fila usa `CreateBloqueio(tx, ...)` (valor repetido não é erro).

**Horas.** Travas da fila e fila de jobs usam o relógio do banco (`NOW()`); janela, feriados e "hoje" usam `time.Local`.

## O que NÃO foi feito aqui (é de outro dono)

* `Deal` e suas listas de colunas (`lost_reason`, `lost_note`, `lead_id`, `origem`), `PATCH /deals/{id}/close|stage` com motivo, `perdas_por_motivo`, semáforo do quadro. As colunas já existem na migração 028; `models.MotivosPerda.ValidarPerda` e `models.MotivoPerdaVazio` estão prontos para uso.
* `models.EnrichmentUsage`/`EnrichmentUsageMeses` somando `company_enrichments` + `prospeccao_jobs` numa consulta (o adendo pede `UNION ALL`); `models.CustoEnriquecimentoJobs` existe para conferência.
* Troca de `SomenteDigitos` por `NormalizarCNPJ` em `receita.go`, `bigdatacorp.go`, `serasa.go`, `demo.go`, `provedor.go` e `enrichment_controller.go`; provedor `wholead`; `WhoLeadSettings`.
* O provedor `receita` consultar a base própria: use `enriquecimento.FichaDaPlataforma` (já ligada em `application.go` quando há painel; `nil` sem painel).
* Descer `gravarIntel`/`aplicarCampos` de `controllers` para `services`.

## Assinaturas por arquivo

### `models/cnpj.go`

Constantes: `CNPJNormSQL`.

Funções:

```go
func CNPJNormSQLColuna(coluna string) string
func NormalizarCNPJ(s string) string
func CNPJFormatoValido(s string) bool
func CNPJValido(s string) bool
func CNPJDigitosVerificadores(base string) string
func FormatarCNPJ(s string) string
func CNPJRaiz(s string) string
```

### `models/prospeccao.go`

Tipos:

```go
type Executor interface
type ProspFiltros struct
type ProspTelefone struct
type ProspSocio struct
type ProspDivida struct
type ProspCodigoDescricao struct
type ProspEndereco struct
type ProspEmpresa struct
type ProspEmpresaPrevia struct
type ProspContagem struct
type ProspContagemParciais struct
type ProspCota struct
type ProspResumoPlataforma struct
type ProspBaseResumo struct
type ProspBaseCarga struct
type PerfilContato struct
type EmailContato struct
type PessoaContato struct
type NotaMotivo struct
```

Constantes: `PorteME`, `PorteEPP`, `PorteDemais`, `PorteNaoInformado`, `OrdemRelevancia`, `OrdemRecentes`, `OrdemAntigas`, `OrdemCapitalDesc`, `OrdemDividaDesc`, `OrdemAleatoria`, `OrdemBase`, `MaxCNPJsFiltro`, `TelefoneCelular`, `TelefoneFixo`, `TelefoneIndefinido`, `OrigemContatoReceita`, `OrigemContatoWhoLead`, `OrigemContatoManual`.

Variáveis: `ProspOrdens`.

Funções:

```go
func ProspOrdemValida(o string) bool
func (f ProspFiltros) TemObrigatorio() bool
func (f *ProspFiltros) Normalizar()
func (f ProspFiltros) Validar() error
func MascararCPF(s string) string
func NovoPedidoID() string
```

### `models/prospeccao_segmento.go`

Tipos:

```go
type ProspSegmento struct
```

Funções:

```go
func (s *ProspSegmento) Validar() error
func ListProspSegmentos(db *sql.DB, userID int64) ([]ProspSegmento, error)
func ProspSegmentoByID(db *sql.DB, id int64) (*ProspSegmento, error)
func CreateProspSegmento(db *sql.DB, s *ProspSegmento) error
func UpdateProspSegmento(db *sql.DB, s *ProspSegmento) error
func DeleteProspSegmento(db *sql.DB, id int64) error
func RegistrarContagemProspSegmento(db *sql.DB, id int64, total int) error
```

### `models/prospeccao_lista.go`

Tipos:

```go
type ProspContadores struct
type ProspLista struct
type ProspListaFilter struct
type OpcionalID struct
type ProspListaPatch struct
```

Constantes: `ListaOrigemBase`, `ListaOrigemCNPJs`, `ListaGerando`, `ListaPronta`, `ListaFalhou`, `MaxQuantidadeLista`.

Funções:

```go
func (l *ProspLista) Validar() error
func ListProspListas(db *sql.DB, f ProspListaFilter) ([]ProspLista, int, error)
func ProspListaByID(db Executor, id int64) (*ProspLista, error)
func ProspContadoresDaLista(db Executor, listaID int64) (ProspContadores, error)
func CreateProspLista(db Executor, l *ProspLista) error
func CreateProspListaComJob(db *sql.DB, l *ProspLista, job *ProspJob) error
func DefinirID(id int64) OpcionalID
func DefinirIDPtr(id *int64) OpcionalID
func (p ProspListaPatch) MudaDestino() bool
func UpdateProspLista(db Executor, id int64, p ProspListaPatch) error
func SetProspListaStatus(db Executor, id int64, status, mensagem, versaoBase string) error
func DeleteProspLista(db *sql.DB, id int64) error
```

### `models/prospeccao_item.go`

Tipos:

```go
type ProspItem struct
type ProspItemFiltro struct
type ProspItemLista struct
type ProspSelecao struct
type ProspSelecaoResumo struct
type ProspItemEstado struct
type ProspItemEnriquecimento struct
```

Constantes: `ItemNovo`, `ItemEnriquecido`, `ItemSemResultado`, `ItemErro`, `ItemEnviado`, `ItemDescartado`, `ItemDuplicado`, `ItemBloqueado`, `MaxSelecaoIDs`.

Variáveis: `ItemEstados`, `ItemEstadosAptosEnvio`, `ItemEstadosAEnriquecer`.

Funções:

```go
func ItemEstadoValido(s string) bool
func InsertProspItens(db Executor, listaID int64, itens []ProspItem) (int, error)
func ListProspItens(db *sql.DB, listaID int64, f ProspItemLista) ([]ProspItem, int, error)
func ProspItemByID(db Executor, id int64) (*ProspItem, error)
func ProspItensCompletos(db Executor, ids []int64) ([]ProspItem, error)
func (s ProspSelecao) Validar() error
func CountProspSelecao(db Executor, listaID int64, s ProspSelecao, estados []string) (int, error)
func ResumoProspSelecao(db Executor, listaID int64, s ProspSelecao) (ProspSelecaoResumo, error)
func ResolveProspSelecao(db Executor, listaID int64, s ProspSelecao, estados []string, aposID int64, limite int) ([]int64, error)
func DescartarProspItens(db Executor, listaID int64, s ProspSelecao) (int, error)
func RestaurarProspItens(db Executor, listaID int64, s ProspSelecao) (int, error)
func SetProspItemEstado(db Executor, id int64, e ProspItemEstado) error
func SetProspItemEmpresa(db Executor, id, companyID int64) error
func UpdateProspItemEnriquecimento(db Executor, id int64, e ProspItemEnriquecimento) error
func ProspItensPorCNPJ(db Executor, listaID int64, cnpjs []string) (map[string]int64, error)
func CNPJsEmListas(db Executor, cnpjs []string, exceto int64) (map[string]bool, error)
func CNPJsNoCRM(db Executor, cnpjs []string) (map[string]int64, error)
```

### `models/prospeccao_job.go`

Tipos:

```go
type ProspJob struct
```

Constantes: `JobGerarLista`, `JobEnriquecer`, `JobEnviarCRM`, `JobPendente`, `JobRodando`, `JobConcluido`, `JobFalhou`, `JobCancelado`.

Variáveis: `JobTipos`, `ErrJobAtivo`.

Funções:

```go
func (j *ProspJob) Ativo() bool
func CreateProspJob(db Executor, j *ProspJob) error
func ProspJobByID(db Executor, id int64) (*ProspJob, error)
func ProspJobAtivoDaLista(db Executor, listaID int64) (*ProspJob, error)
func ProspJobsDaLista(db *sql.DB, listaID int64) ([]ProspJob, error)
func ReivindicarProspJob(db *sql.DB, tipos []string, arrendamento time.Duration) (*ProspJob, error)
func RecuperarProspJobsOrfaos(db *sql.DB) (int, error)
func SetProspJobEstado(db Executor, id int64, estado any) error
func SetProspJobTotal(db Executor, id int64, total int) error
func SomarProspJobProgresso(db Executor, id int64, feitos, erros, custoCentavos int) error
func SetProspJobMensagem(db Executor, id int64, mensagem string) error
func ReagendarProspJob(db Executor, id int64, espera time.Duration, tentativas int, mensagem string) error
func FinalizarProspJob(db Executor, id int64, status, mensagem string, resultado any) (bool, error)
func CancelarProspJob(db *sql.DB, id int64) (*ProspJob, error)
func ConcluirLimpezaProspJob(db Executor, id int64, mensagem string, resultado any) error
func ProspJobCancelado(db Executor, id int64) (bool, error)
func CustoEnriquecimentoJobs(db Executor, de, ate time.Time) (consultas, custoCentavos int, err error)
```

### `models/lead.go`

Tipos:

```go
type ErroLeadEmAtendimento struct
type Sinal struct
type ResumoDecisor struct
type ResumoCadastro struct
type ResumoLead struct
type Lead struct
type LeadVisao struct
type LeadFilter struct
type ContadoresLeads struct
type OpcionalFloat struct
type OpcionalTempo struct
type LeadPatch struct
type LeadBulk struct
type LeadListaResumo struct
type FilaOpcoes struct
type Tentativa struct
type NumerosDaFila struct
```

Constantes: `LeadNovo`, `LeadTentando`, `LeadRetornar`, `LeadConvertido`, `LeadDescartado`, `LeadOrigemProspeccao`, `LeadOrigemManual`, `LeadOrigemFormulario`, `LeadOrigemImportacao`, `UrgenciaAtrasado`, `UrgenciaHoje`, `UrgenciaFuturo`, `UrgenciaNenhuma`, `ResultadoAtendeu`, `ResultadoReuniao`, `ResultadoRetornar`, `ResultadoSemInteresse`, `ResultadoNaoEDecisor`, `ResultadoNaoAtendeu`, `ResultadoCaixaPostal`, `ResultadoOcupado`, `ResultadoNumeroErrado`, `ResultadoNaoLigarMais`, `TravaFilaDuracao`, `FROM`, `LEFT`, `LEFT`, `LEFT`, `LEFT`, `WHERE`, `LeadBulkDono`, `LeadBulkTemperatura`, `LeadBulkEtiqueta`, `LeadBulkDescartar`, `LeadBulkReabrir`, `MaxLeadsBulk`, `MaxPularIDs`.

Variáveis: `LeadEstados`, `LeadEstadosAbertos`, `LeadOrigens`, `LeadTemperaturas`, `LeadUrgencias`, `ResultadosLead`, `ResultadosSemContato`, `ErrLeadEncerrado`, `ErrLeadEstado`.

Funções:

```go
func LeadEstadoValido(s string) bool
func LeadEstadoAberto(s string) bool
func LeadTemperaturaValida(s string) bool
func ResultadoValido(r string) bool
func OutcomeDoResultado(r string) string
func ResultadoConectado(r string) bool
func ResultadoContaTentativa(r string) bool
func (e *ErroLeadEmAtendimento) Error() string
func (l *Lead) Aberto() bool
func (l *Lead) SemDono() bool
func VisaoTotal(userID int64) LeadVisao
func LeadByID(db Executor, id int64, v LeadVisao) (*Lead, error)
func ListLeads(db *sql.DB, f LeadFilter) ([]Lead, int, ContadoresLeads, error)
func CreateLead(db Executor, l *Lead) (criado bool, err error)
func DefinirTempo(t time.Time) OpcionalTempo
func LimparTempo() OpcionalTempo
func UpdateLead(db Executor, id int64, p LeadPatch) error
func DeleteLead(db Executor, id int64) error
func MarcarLeadVisto(db Executor, id int64) error
func BulkLeads(db Executor, v LeadVisao, b LeadBulk) (int, error)
func DescartarLead(db Executor, id int64, motivo string) error
func ReabrirLead(db Executor, id int64, quando time.Time) error
func ConverterLead(db Executor, id, dealID int64, meetingID *int64) error
func LeadAbertoDaEmpresa(db Executor, companyID int64) (*int64, error)
func ListasDosLeads(db *sql.DB, v LeadVisao) ([]LeadListaResumo, error)
func ProximoLeadDaFila(db *sql.DB, o FilaOpcoes) (*Lead, error)
func LiberarLead(db Executor, id, userID int64) (bool, error)
func RenovarTravaLead(db Executor, id, userID int64) error
func PrepararResultadoLead(tx *sql.Tx, id int64, v LeadVisao) (*Lead, error)
func TentativasDoLead(db Executor, leadID int64) ([]Tentativa, error)
func TentativasSemContatoDesde(db Executor, leadID int64, desde time.Time) (int, error)
func LeadTemLigacao(db Executor, leadID int64) (bool, error)
func ResumoDaFila(db *sql.DB, o FilaOpcoes, agora time.Time) (NumerosDaFila, error)
```

### `models/bloqueio.go`

Tipos:

```go
type Bloqueio struct
type BloqueioFilter struct
type BloqueiosDoContato struct
```

Constantes: `BloqueioTelefone`, `BloqueioEmail`, `BloqueioCNPJ`, `BloqueioOptOut`, `BloqueioNumeroErrado`, `BloqueioPedidoTitular`, `BloqueioCliente`, `BloqueioManual`, `BloqueioListaExterna`, `MaxBloqueiosImportacao`.

Variáveis: `BloqueioTipos`, `BloqueioMotivos`.

Funções:

```go
func BloqueioTipoValido(t string) bool
func BloqueioMotivoValido(m string) bool
func NormalizarTelefone(s string) string
func NormalizarValorBloqueio(tipo, valor string) (string, error)
func CreateBloqueio(db Executor, b *Bloqueio) (criado bool, err error)
func ImportarBloqueios(db Executor, tipo, motivo string, valores []string, criadoPor *int64) (criados, repetidos, invalidos int, err error)
func ListBloqueios(db *sql.DB, f BloqueioFilter) ([]Bloqueio, int, error)
func BloqueioByID(db Executor, id int64) (*Bloqueio, error)
func DeleteBloqueio(db *sql.DB, id int64) (*Bloqueio, error)
func ValoresBloqueados(db Executor, tipo string, valores []string) (map[string]bool, error)
func CNPJsBloqueados(db Executor, cnpjs []string) (map[string]bool, error)
func Bloqueado(db Executor, tipo, valor string) (bool, error)
func (b BloqueiosDoContato) TodosTelefonesBloqueados(telefones []string) bool
func ConferirBloqueios(db Executor, cnpj string, telefones, emails []string) (BloqueiosDoContato, error)
```

### `models/prospeccao_config.go`

Tipos:

```go
type ConfigProspeccao struct
type Janela struct
type MotivosPerda struct
```

Constantes: `SettingProspeccao`, `SettingMotivosPerda`, `JanelaHoraMinima`, `JanelaHoraMaxima`, `DescarteSemTelefone`, `DescarteSemContato`, `DescarteOptOut`, `DescarteManual`, `MotivoPerdaVazio`.

Variáveis: `MotivosDescarteReservados`, `ErrMotivoPerdaObrigatorio`.

Funções:

```go
func DefaultConfigProspeccao() ConfigProspeccao
func LoadConfigProspeccao(db *sql.DB) ConfigProspeccao
func SaveConfigProspeccao(db *sql.DB, c *ConfigProspeccao, userID *int64) error
func (c *ConfigProspeccao) Validate() error
func (c ConfigProspeccao) MotivoDescarteValido(motivo string) bool
func (c ConfigProspeccao) IntervaloCadencia(tentativas int) time.Duration
func (c ConfigProspeccao) DiaPermitido(t time.Time) bool
func (c ConfigProspeccao) JanelaAberta(t time.Time) bool
func (c ConfigProspeccao) ProximoDiaPermitido(t time.Time) time.Time
func (c ConfigProspeccao) AjustarJanela(t time.Time) time.Time
func (c ConfigProspeccao) TextoJanela() string
func (c ConfigProspeccao) JanelaEm(agora time.Time) Janela
func FeriadoNacional(t time.Time) bool
func DefaultMotivosPerda() MotivosPerda
func LoadMotivosPerda(db *sql.DB) MotivosPerda
func SaveMotivosPerda(db *sql.DB, m *MotivosPerda, userID *int64) error
func (m *MotivosPerda) Validate() error
func (m MotivosPerda) Contem(motivo string) bool
func (m MotivosPerda) ValidarPerda(motivo string) (string, error)
```

### `models/call.go`

Variáveis: `ErrChamadaRepetida`.

Funções:

```go
func CreateCallTx(db Executor, c *Call) error
func CallByChave(db Executor, chave string) (*Call, error)
```

### `services/plataforma.go`

Tipos:

```go
type RespostaPainel struct
type ErroPlataforma struct
type PreviaPlataforma struct
type CandidatosPedido struct
type CandidatosPlataforma struct
type ExtracaoPlataforma struct
```

Constantes: `CodigoCotaEsgotada`, `CodigoMuitasRequisicoes`, `CodigoBaseEmCarga`, `CodigoBaseAtualizada`, `CodigoFiltroInvalido`, `CodigoJobAtivo`, `CodigoCustoMudou`, `CodigoNaoEncontrado`, `CodigoPlataforma`, `MaxCNPJsExtracao`.

Variáveis: `PlataformaBaseURL`, `ErrPlataformaNaoConfigurada`, `ErrPlataformaInacessivel`, `CatalogosPlataforma`.

Funções:

```go
func UsarPlataformaDeTeste(baseURL string) (desfazer func())
func PlataformaConfigurada() bool
func ChamarPainelBruto(ctx context.Context, metodo, caminho string, consulta url.Values, corpo any) (*RespostaPainel, error)
func ChamarPainel(ctx context.Context, metodo, caminho string, corpo any) (status int, resposta []byte, err error)
func (e *ErroPlataforma) Error() string
func (e *ErroPlataforma) Transitorio() bool
func (e *ErroPlataforma) Corpo() map[string]any
func ComoErroPlataforma(err error) (*ErroPlataforma, bool)
func PlataformaResumo(ctx context.Context) (*models.ProspResumoPlataforma, error)
func PlataformaCatalogo(ctx context.Context, nome string, consulta url.Values) (json.RawMessage, error)
func PlataformaContagem(ctx context.Context, filtros models.ProspFiltros) (*models.ProspContagem, error)
func PlataformaPrevia(ctx context.Context, filtros models.ProspFiltros, ordem string, limite int) (*PreviaPlataforma, error)
func PlataformaCandidatos(ctx context.Context, p CandidatosPedido) (*CandidatosPlataforma, error)
func PlataformaExtracao(ctx context.Context, pedidoID string, cnpjs []string, incluirPF bool) (*ExtracaoPlataforma, error)
func PlataformaCNPJ(ctx context.Context, cnpj string) (*models.ProspEmpresa, error)
```

### `services/prospeccao_worker.go`

Tipos:

```go
type Fatia struct
type ProcessadorJob interface
type EncerradorJob interface
type ProcessadorFunc func(ctx context.Context, e *ExecucaoJob) (Fatia, error)
type ErroTransitorio struct{ Causa error }
type ExecucaoJob struct
```

Variáveis: `ProspeccaoWorkerIntervalo`, `ProspeccaoFatiaPrazo`, `ProspeccaoArrendamento`, `ProspeccaoEsperas`, `ProspeccaoWorkerAtivo`.

Funções:

```go
func (f ProcessadorFunc) Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error)
func RegistrarProcessadorJob(tipo string, p ProcessadorJob)
func (e *ErroTransitorio) Error() string
func (e *ErroTransitorio) Unwrap() error
func Transitorio(err error) error
func (e *ExecucaoJob) Parametros(v any) error
func (e *ExecucaoJob) Estado(v any) error
func (e *ExecucaoJob) SalvarEstado(v any) error
func (e *ExecucaoJob) DefinirTotal(total int) error
func (e *ExecucaoJob) Progresso(feitos, erros, custoCentavos int) error
func (e *ExecucaoJob) Mensagem(m string) error
func (e *ExecucaoJob) Cancelado() bool
func (e *ExecucaoJob) UserID() *int64
func StartProspeccaoWorker(db *sql.DB)
func ProcessarProximoJob(ctx context.Context, db *sql.DB) (bool, error)
func ProcessarJobsAte(ctx context.Context, db *sql.DB, limite int) (int, error)
```

### `services/enriquecimento/cnpj.go`

Variáveis: `FichaDaPlataforma`.

Funções:

```go
func NormalizarCNPJ(s string) string
func ValidarCNPJ(s string) bool
func CNPJSoNumeros(s string) bool
func FormatarCNPJ(s string) string
```

### `controllers/prospeccao_stub.go`
