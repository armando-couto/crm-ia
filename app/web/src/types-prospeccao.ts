/**
 * Tipos da prospecção (listas, enriquecimento, leads, fila de ligações,
 * bloqueios e configurações), exatamente como no contrato
 * docs/PROSPECCAO.md + docs/PROSPECCAO-ADENDO.md (o adendo prevalece).
 *
 * Os mapas de rótulos das enumerações ficam aqui, exceto os de Resultado,
 * EstadoLead, Urgencia e do motivo de descarte, que vivem só em src/format.ts.
 */
import type { Call, Contact, Deal, Meeting, Pagination } from './types'

// ===== 2. Tipos comuns =====

export type Porte = 'me' | 'epp' | 'demais' | 'nao_informado'
export type Ordem = 'relevancia' | 'recentes' | 'antigas' | 'capital_desc' | 'divida_desc' | 'aleatoria'
/** `base` só aparece em respostas: conjunto grande demais para a ordem pedida. */
export type OrdemAplicada = Ordem | 'base'

export interface Filtros {
  ufs?: string[]
  /** Código da Receita (TOM), ex.: 1389 Fortaleza. */
  municipios?: number[]
  /** Subclasse, 7 dígitos. */
  cnaes?: number[]
  /** 2 a 5 dígitos: "56", "4711". */
  cnae_prefixos?: string[]
  cnae_secundario?: boolean
  portes?: Porte[]
  naturezas?: number[]
  matriz?: 'todas' | 'matriz' | 'filial'
  simples?: 'todos' | 'sim' | 'nao'
  /** `sim` exige `incluir_pf: true` (422 sem isso). */
  mei?: 'todos' | 'sim' | 'nao'
  incluir_pf?: boolean
  abertura_de?: string
  abertura_ate?: string
  abertura_ultimos_dias?: number
  capital_min?: number
  capital_max?: number
  /** Qualquer telefone válido (celular, fixo ou indefinido). */
  com_telefone?: boolean
  com_celular?: boolean
  com_email?: boolean
  ocultar_contador?: boolean
  divida?: 'todos' | 'com' | 'sem'
  divida_min?: number
  divida_max?: number
  /** Exigem `municipios` ou `cnaes`/`cnae_prefixos` no mesmo filtro. */
  bairros?: string[]
  ceps?: string[]
  nome?: string
  /** Lista específica (até 5.000); com ela os demais filtros são ignorados, exceto `incluir_pf`. */
  cnpjs?: string[]
}

/** Chaves das quais ao menos uma é obrigatória em `Filtros`. */
export const FILTROS_OBRIGATORIOS = ['ufs', 'municipios', 'cnaes', 'cnae_prefixos', 'cnpjs'] as const

/** temFiltroObrigatorio diz se já dá para contar, pré-visualizar ou gerar. */
export function temFiltroObrigatorio(f: Filtros | null | undefined): boolean {
  if (!f) return false
  return FILTROS_OBRIGATORIOS.some((k) => (f[k]?.length ?? 0) > 0)
}

export type TipoTelefone = 'celular' | 'fixo' | 'indefinido'
export type OrigemContato = 'receita' | 'wholead' | 'manual'

export interface Telefone {
  /** Só dígitos, com DDD: celular = 11; fixo e indefinido = 10 (sem o 9). */
  numero: string
  tipo: TipoTelefone
  compartilhado?: boolean
  whatsapp?: boolean | null
  nao_perturbe?: boolean | null
  origem?: OrigemContato
  pessoa?: string
  principal?: boolean
  invalido?: boolean
}

export interface SocioBase {
  nome: string
  qualificacao: string
  desde: string | null
  tipo: 'pf' | 'pj' | 'estrangeiro'
  /** Mascarado. */
  documento: string
  representante?: string
}

export type TipoDivida = 'geral' | 'previdenciaria' | 'fgts'

export interface Divida {
  total: number
  inscricoes: number
  em_cobranca: number
  regular: number
  tipos: TipoDivida[]
  ajuizada: boolean
  mais_antiga: string | null
  /** "2026-T3". */
  referencia: string
}

export interface CodigoDescricao {
  codigo: number
  descricao: string
}

export type EmailTipo = 'corporativo' | 'gratuito' | 'contabilidade' | ''

export interface Endereco {
  tipo_logradouro: string
  logradouro: string
  numero: string
  complemento: string
  bairro: string
  cep: string
  municipio: string
  municipio_codigo: number
  uf: string
}

export interface Empresa {
  cnpj: string
  matriz: boolean
  razao_social: string
  nome_fantasia: string
  natureza: CodigoDescricao
  pessoa_fisica: boolean
  porte: Porte
  capital: number | null
  abertura: string | null
  situacao: 'ATIVA'
  situacao_desde: string | null
  simples: boolean
  mei: boolean
  cnae: CodigoDescricao
  cnaes_secundarios: CodigoDescricao[]
  endereco: Endereco
  telefones: Telefone[]
  email: string
  email_tipo: EmailTipo
  socios: SocioBase[]
  divida: Divida | null
  /** 0 a 100. */
  qualidade: number
  /** "2026-11". */
  referencia: string
}

/** Prévia gratuita: sem contato e sem nomes de sócios. */
export interface EmpresaPrevia {
  cnpj: string
  razao_social: string
  nome_fantasia: string
  cnae: CodigoDescricao
  porte: Porte
  abertura: string | null
  capital: number | null
  municipio: string
  uf: string
  pessoa_fisica: boolean
  mei: boolean
  tem_celular: boolean
  tem_telefone: boolean
  tem_email: boolean
  tem_divida: boolean
  socios_qtd: number
  qualidade: number
}

export interface Contagem {
  /** Em `teto` vale 10000 e a tela mostra "mais de 10.000". */
  total: number
  total_modo: 'exato' | 'teto'
  /** Com `teto`, quando o cubo dá um limite: "mais de 10.000, até N". */
  total_limite_superior?: number
  /** `null` sempre que `total_modo` é `teto`; parcial nula é omitida na tela. */
  parciais: { com_celular: number | null; com_email: number | null; com_divida: number | null } | null
}

export interface PreviaResposta {
  itens: EmpresaPrevia[]
  /** Diferente da pedida: "muitas empresas para esta ordenação; mostrando os melhores contatos". */
  ordem_aplicada?: OrdemAplicada
}

// ===== Catálogos (GET /prospeccao/catalogos/{nome}) =====

export type NomeCatalogo = 'cnaes' | 'municipios' | 'naturezas' | 'ufs'
export type NivelCnae = 'divisao' | 'grupo' | 'classe' | 'subclasse'

export interface CatalogoCnae {
  /** 2 a 7 dígitos. Subclasse vai em `filtros.cnaes`; os demais níveis em `filtros.cnae_prefixos`. */
  codigo: string
  descricao: string
  nivel: NivelCnae
}
export interface CatalogoMunicipio {
  codigo: number
  nome: string
  uf: string
  empresas: number
}
export interface CatalogoNatureza {
  codigo: number
  descricao: string
  pessoa_fisica: boolean
}
export interface CatalogoUf {
  uf: string
  nome: string
  empresas: number
}
export interface Catalogo<T> {
  itens: T[]
}

// ===== 4.1 Plataforma =====

export interface Cota {
  /** "2026-11". */
  mes: string
  limite: number
  usado: number
  restante: number
}

export type EstadoCarga = 'ociosa' | 'baixando' | 'carregando' | 'construindo' | 'erro'

export interface ResumoPlataforma {
  habilitada: boolean
  motivo: string
  cota: Cota
  base: {
    /** Falso só antes da primeira carga terminar; com `true` a carga em curso é só um aviso. */
    pronta: boolean
    amostra: boolean
    referencia_cnpj: string
    referencia_pgfn: string
    versao: string
    empresas: number
    carga: { estado: EstadoCarga | string; percentual: number; etapa: string; erro: string }
  }
}

/** Códigos que acompanham os erros da prospecção (`e.data.codigo` em ApiError). */
export type CodigoErro =
  | 'cota_esgotada'
  | 'muitas_requisicoes'
  | 'base_em_carga'
  | 'base_atualizada'
  | 'filtro_invalido'
  | 'job_ativo'
  | 'custo_mudou'
  | 'estado'
  | 'agenda_ocupada'
  | 'motivo_obrigatorio'

/** Corpo de erro da prospecção, guardado em `ApiError.data`. */
export interface ErroProspeccao {
  error: string
  codigo?: CodigoErro
  cota?: Cota
  tentar_em_segundos?: number
  job?: Job
  estimativa?: EnriquecerEstimativa
  /** 423: lead travado por outra pessoa. */
  em_atendimento_por?: number
  nome?: string
}

// ===== 5. Jobs =====

export type TipoJob = 'gerar_lista' | 'enriquecer' | 'enviar_crm'
export type StatusJob = 'pendente' | 'rodando' | 'concluido' | 'falhou' | 'cancelado'

export interface Job {
  id: number
  tipo: TipoJob
  lista_id: number | null
  status: StatusJob
  total: number
  feitos: number
  erros: number
  custo_centavos: number
  mensagem: string
  resultado: Record<string, unknown> | null
  criado_por: number | null
  criado_em: string
  iniciado_em: string | null
  finalizado_em: string | null
}

export const STATUS_JOB_ATIVOS: StatusJob[] = ['pendente', 'rodando']

/** jobAtivo diz se o job ainda está na fila ou em execução. */
export function jobAtivo(job: Pick<Job, 'status'> | null | undefined): boolean {
  return !!job && STATUS_JOB_ATIVOS.includes(job.status)
}

export interface ResultadoGerarLista {
  pedidos: number
  entregues: number
  pulados_no_crm: number
  pulados_em_listas: number
  pulados_bloqueio: number
  pulados_pf?: number
  nao_encontrados: number
}

export interface ResultadoEnviarCrm {
  leads: number
  empresas_criadas: number
  empresas_existentes: number
  contatos: number
  duplicados: number
  bloqueados: number
  por_usuario: { user_id: number; nome: string; quantidade: number }[]
}

// ===== 6.2 Segmentos =====

export interface Segmento {
  id: number
  nome: string
  filtros: Filtros
  ordem: Ordem
  compartilhado: boolean
  ultima_contagem: number | null
  ultima_contagem_em: string | null
  criado_por: number | null
  criado_por_nome: string
  criado_em: string
  atualizado_em: string
}

export interface SegmentoRequest {
  nome: string
  filtros: Filtros
  ordem: Ordem
  compartilhado: boolean
}

export interface ContagemRequest {
  filtros: Filtros
  /** Só com ele a contagem atualiza `ultima_contagem` do segmento. */
  segmento_id?: number
}

// ===== 6.3 Listas e itens =====

export type OrigemLista = 'base' | 'cnpjs'
export type StatusLista = 'gerando' | 'pronta' | 'falhou'

export interface CriarLista {
  nome: string
  descricao?: string
  origem?: OrigemLista
  filtros: Filtros
  ordem?: Ordem
  /** 1 a 20.000; em `origem: 'cnpjs'` é o número de CNPJs válidos colados. */
  quantidade: number
  excluir_no_crm?: boolean
  excluir_em_listas?: boolean
  segmento_id?: number
}

export interface ContadoresLista {
  total: number
  novo: number
  enriquecido: number
  sem_resultado: number
  erro: number
  enviado: number
  descartado: number
  duplicado: number
  bloqueado: number
  com_celular: number
  com_email: number
  com_decisor: number
  com_whatsapp: number
  pessoa_fisica: number
  custo_centavos: number
}

export interface Lista {
  id: number
  nome: string
  descricao: string
  origem: OrigemLista
  filtros: Filtros
  ordem: Ordem
  quantidade_pedida: number
  status: StatusLista
  mensagem: string
  versao_base: string
  incluir_pf: boolean
  playbook_id: number | null
  pipeline_id: number | null
  stage_id: number | null
  criado_por: number | null
  criado_por_nome: string
  criado_em: string
  atualizado_em: string
  contadores: ContadoresLista
}

export interface ListasResposta {
  data: Lista[]
  pagination: Pagination
}
export interface CriarListaResposta {
  lista: Lista
  job: Job
}
/** `jobs`: os ativos e o último de cada tipo. */
export interface ListaDetalhe {
  lista: Lista
  jobs: Job[]
}
export interface AtualizarLista {
  nome?: string
  descricao?: string
  playbook_id?: number | null
  pipeline_id?: number | null
  stage_id?: number | null
}

export type EstadoItem = 'novo' | 'enriquecido' | 'sem_resultado' | 'erro' | 'enviado' | 'descartado' | 'duplicado' | 'bloqueado'

export interface NotaMotivo {
  pontos: number
  texto: string
}

export interface Item {
  id: number
  lista_id: number
  cnpj: string
  razao_social: string
  nome_fantasia: string
  uf: string
  municipio: string
  cnae_codigo: number
  cnae_descricao: string
  porte: Porte
  pessoa_fisica: boolean
  estado: EstadoItem
  nota: number
  nota_motivos: NotaMotivo[]
  tem_celular: boolean
  tem_email: boolean
  tem_decisor: boolean
  tem_whatsapp: boolean
  tem_divida: boolean
  /** Selo "contador": o único contato é compartilhado ou de contabilidade. */
  contato_compartilhado: boolean
  telefone_principal: string
  enriquecido_em: string | null
  enriquecimento_provedor: string
  custo_centavos: number
  erro: string
  company_id: number | null
  lead_id: number | null
  enviado_em: string | null
  criado_em: string
  /** Só no detalhe. */
  dados?: Empresa
  /** Só no detalhe. */
  enriquecimento?: PerfilContato | null
}

export interface ItensResposta {
  data: Item[]
  pagination: Pagination
}

export type OrdemItens = 'nota' | 'razao_social' | 'municipio' | 'estado'

export interface FiltroItens {
  estado?: EstadoItem
  q?: string
  com_celular?: boolean
  com_decisor?: boolean
  nota_min?: number
}

/** `ids` (até 5.000) OU `todos: true` com `filtro` e `exceto` opcionais. */
export interface Selecao {
  ids?: number[]
  todos?: boolean
  filtro?: FiltroItens
  /** Só com `todos: true` (até 5.000): linhas desmarcadas depois de "selecionar todos os N". */
  exceto?: number[]
}

export interface Afetados {
  afetados: number
}

// ===== 7. Enriquecimento =====

export interface WhoLeadSettings {
  habilitado: boolean
  demo: boolean
  client_id: string
  /** Só na gravação; vazio mantém o guardado. */
  client_secret?: string
  has_secret: boolean
  base_url?: string
  incluir_socios: boolean
  incluir_responsavel_legal: boolean
  incluir_portabilidade: boolean
  preco_centavos: number
  lote_tamanho: number
}

export interface EmailContato {
  email: string
  tipo?: string
  validado?: boolean | null
  pessoa?: string
}

export type PapelPessoa = 'socio' | 'representante' | 'funcionario'

export interface Pessoa {
  nome: string
  /** Mascarado. */
  documento: string
  papel: PapelPessoa
  cargo: string
  telefones: Telefone[]
  emails: string[]
}

export interface PerfilContato {
  provedor: string
  demo: boolean
  consultado_em: string
  telefones: Telefone[]
  emails: EmailContato[]
  pessoas: Pessoa[]
  funcionarios_faixa: string
  faturamento_faixa: string
  site: string
  observacao: string
}

export interface EnriquecerEstimativaRequest {
  selecao: Selecao
  reprocessar?: boolean
}
export interface EnriquecerRequest extends EnriquecerEstimativaRequest {
  custo_confirmado_centavos: number
}

export interface EnriquecerEstimativa {
  selecionados: number
  a_enriquecer: number
  ja_enriquecidos: number
  provedor: { codigo: 'wholead'; nome: string; pronto: boolean; demo: boolean; motivo: string }
  custo_unitario_centavos: number
  custo_estimado_centavos: number
  uso_mes: { custo_centavos: number; limite_centavos: number }
  saldo: { plano: string; franquia_total: number | null; franquia_restante: number | null } | null
}

export interface JobResposta {
  job: Job
}

// ===== 8.1 Enviar ao CRM =====

export type Temperatura = '' | 'fria' | 'media' | 'quente'
export type ModoDistribuicao = 'unico' | 'rodizio'

export interface Distribuicao {
  modo: ModoDistribuicao
  usuarios: number[]
  por_dia?: number
}

export interface EnviarRequest {
  selecao: Selecao
  duplicados: 'pular' | 'vincular'
  criar_contatos: boolean
  distribuicao: Distribuicao
  temperatura?: Temperatura
  etiquetas?: string[]
  /** 0 = não marcar. */
  conta_alvo_tier?: 0 | 1 | 2 | 3
}

export interface EnviarPrevia {
  selecionados: number
  aptos: number
  duplicados_no_crm: number
  bloqueados: number
  ja_enviados: number
  descartados: number
  pessoa_fisica: number
  sem_telefone: number
  por_usuario: { user_id: number; nome: string; quantidade: number }[]
}

// ===== 8.2 Leads =====

export type EstadoLead = 'novo' | 'tentando' | 'retornar' | 'convertido' | 'descartado'
export type Resultado =
  | 'atendeu'
  | 'reuniao'
  | 'retornar'
  | 'sem_interesse'
  | 'nao_e_decisor'
  | 'nao_atendeu'
  | 'caixa_postal'
  | 'ocupado'
  | 'numero_errado'
  | 'nao_ligar_mais'
export type Urgencia = 'atrasado' | 'hoje' | 'futuro' | 'nenhuma'
export type OrigemLead = 'prospeccao' | 'manual' | 'formulario' | 'importacao'
export type FilaLeads = 'meus' | 'sem_dono' | 'todos'
export type OrdemLeads = 'urgencia' | 'nota' | 'criado_em' | 'titulo'

/** Estados em que o lead ainda aceita resultado, conversão e entra na fila. */
export const ESTADOS_LEAD_ABERTOS: EstadoLead[] = ['novo', 'tentando', 'retornar']

/** Ordem dos botões de resultado na fila, com a tecla de atalho de cada um. */
export const RESULTADOS: { resultado: Resultado; tecla: string }[] = [
  { resultado: 'atendeu', tecla: '1' },
  { resultado: 'reuniao', tecla: '2' },
  { resultado: 'retornar', tecla: '3' },
  { resultado: 'sem_interesse', tecla: '4' },
  { resultado: 'nao_e_decisor', tecla: '5' },
  { resultado: 'nao_atendeu', tecla: '6' },
  { resultado: 'caixa_postal', tecla: '7' },
  { resultado: 'ocupado', tecla: '8' },
  { resultado: 'numero_errado', tecla: '9' },
  { resultado: 'nao_ligar_mais', tecla: '0' }
]

/** Códigos reservados de `Lead.motivo_descarte` (fora deles é o rótulo de `motivos_descarte`). */
export type MotivoDescarteSistema = 'sem_telefone' | 'sem_contato' | 'opt_out' | 'manual'

export interface Lead {
  id: number
  titulo: string
  company_id: number | null
  company_name: string
  contact_id: number | null
  contact_name: string
  cnpj: string
  cidade: string
  uf: string
  origem: OrigemLead
  lista_id: number | null
  lista_nome: string
  item_id: number | null
  owner_id: number | null
  owner_name: string
  estado: EstadoLead
  urgencia: Urgencia
  temperatura: Temperatura
  etiquetas: string[]
  nota: number
  valor_estimado: number | null
  /** Já ordenados do melhor para o pior; é a única lista que a coluna "Ligar" renderiza. */
  telefones: Telefone[]
  emails: string[]
  tentativas: number
  ultimo_contato_em: string | null
  ultimo_resultado: Resultado | ''
  proxima_acao_em: string | null
  motivo_descarte: string
  deal_id: number | null
  meeting_id: number | null
  convertido_em: string | null
  whatsapp_optin: boolean
  pessoa_fisica: boolean
  em_atendimento_por: number | null
  visto: boolean
  criado_em: string
  atualizado_em: string
}

/**
 * `meus`, `sem_dono` e `todos` ignoram filtros; os de estado e de urgência
 * valem para a `fila` atual, ignorando `estado` e `urgencia`.
 */
export interface ContadoresLeads {
  meus: number
  sem_dono: number
  todos: number
  novo: number
  tentando: number
  retornar: number
  convertido: number
  descartado: number
  atrasados: number
  hoje: number
}

export interface LeadsResposta {
  data: Lead[]
  pagination: Pagination
  contadores: ContadoresLeads
}

/** GET /leads/listas: listas com leads visíveis ao usuário (filtro de /leads e seletor da fila). */
export interface LeadLista {
  id: number
  nome: string
  abertos: number
}

export interface CriarLead {
  titulo: string
  company_id?: number | null
  contact_id?: number | null
  owner_id?: number | null
  telefones?: Telefone[]
  temperatura?: Temperatura
  etiquetas?: string[]
  valor_estimado?: number | null
}

/** PUT /leads/{id} (parcial). */
export interface AtualizarLead {
  titulo?: string
  owner_id?: number | null
  contact_id?: number | null
  temperatura?: Temperatura
  etiquetas?: string[]
  valor_estimado?: number | null
  telefones?: Telefone[]
  proxima_acao_em?: string | null
  /** Caixa "Cliente autorizou WhatsApp" (auditado). */
  whatsapp_optin?: boolean
}

export type AcaoLeadsEmMassa = 'dono' | 'temperatura' | 'etiqueta' | 'descartar' | 'reabrir'

export interface LeadsBulkRequest {
  /** Até 500. */
  ids: number[]
  acao: AcaoLeadsEmMassa
  owner_id?: number | null
  temperatura?: Temperatura
  etiqueta?: string
  /** Obrigatório em `descartar`. */
  motivo?: string
}

export interface ConverterRequest {
  pipeline_id?: number
  stage_id?: number
  nome?: string
  valor?: number
  owner_id?: number
}
export interface ConverterResposta {
  lead: Lead
  deal: Deal
}

export interface DescartarRequest {
  motivo: string
  observacao?: string
  bloquear?: boolean
}

export type NivelSinal = 'ok' | 'atencao' | 'risco'
export interface Sinal {
  tipo: string
  nivel: NivelSinal
  texto: string
}

export interface Decisor {
  nome: string
  cargo: string
  telefones: Telefone[]
}

export interface CadastroLead {
  razao_social: string
  nome_fantasia: string
  cnae: string
  porte: Porte
  abertura: string | null
  capital: number | null
  natureza: string
  simples: boolean
  mei: boolean
  endereco: string
  municipio: string
  uf: string
  site: string
  email: string
  socios: SocioBase[]
  divida: Divida | null
}

export interface ResumoLead {
  frases: string[]
  decisores: Decisor[]
  sinais: Sinal[]
  cadastro: CadastroLead
}

export interface Tentativa {
  id: number
  resultado: Resultado
  numero: string
  duracao_segundos: number
  notas: string
  user_name: string
  criado_em: string
}

export type TipoAlerta =
  | 'pessoa_fisica'
  | 'nao_perturbe'
  | 'bloqueado'
  | 'fora_da_janela'
  | 'contador'
  | 'divida'
  | 'whatsapp_sem_optin'
  | 'limite_tentativas'
export type NivelAlerta = 'info' | 'atencao' | 'risco'

export interface Alerta {
  tipo: TipoAlerta
  nivel: NivelAlerta
  texto: string
}

export interface Janela {
  aberta: boolean
  /** "segunda a sexta, 9h às 18h". */
  texto: string
  reabre_em: string | null
}

export interface Roteiro {
  titulo: string
  corpo: string
}

export interface LeadCompleto {
  lead: Lead
  resumo: ResumoLead
  contatos: Contact[]
  tentativas: Tentativa[]
  roteiro: Roteiro | null
  alertas: Alerta[]
  janela: Janela
}

export interface ReuniaoRequest {
  titulo?: string
  /** Sempre o `inicio` RFC 3339 devolvido por /agenda/disponibilidade, nunca `datetime-local`. */
  inicio: string
  duracao_min?: number
  local?: string
  notas?: string
  user_id?: number
  pipeline_id?: number
  stage_id?: number
  valor?: number
  /** "Marcar mesmo assim" depois de um 409 `agenda_ocupada`. */
  forcar?: boolean
}

export interface ResultadoRequest {
  resultado: Resultado
  /** UUID gerado pela tela: repetir a mesma chave não grava de novo. */
  chave?: string
  /** Último número em que o usuário clicou em "ligar" (ou o principal). Obrigatório em `numero_errado`. */
  numero?: string
  /** Cronômetro; 0 sem clique em "ligar". */
  duracao_segundos?: number
  notas?: string
  /** Obrigatório em `retornar`; opcional em `atendeu`. */
  retornar_em?: string
  /** Obrigatório em `sem_interesse`. */
  motivo?: string
  /** `nao_e_decisor`: quem é o decisor. */
  contato?: { nome: string; cargo?: string; telefone?: string; email?: string }
  reuniao?: ReuniaoRequest
  whatsapp_optin?: boolean
  temperatura?: Temperatura
}

export type TipoProximoPasso = 'ligar' | 'retornar' | 'reuniao' | 'nenhum'

export interface ResultadoResposta {
  lead: Lead
  call: Call
  meeting?: Meeting
  deal?: Deal
  proximo_passo: { tipo: TipoProximoPasso; em: string | null; texto: string }
}

export interface ResumoFila {
  na_fila: number
  novos: number
  retornos_vencidos: number
  retornos_hoje: number
  feitas_hoje: number
  conectadas_hoje: number
  reunioes_hoje: number
  meta_dia: number
  janela: Janela
}

/** POST /leads/fila/proximo. O cockpit acumula os pulados da sessão e manda todos. */
export interface ProximoRequest {
  lista_id?: number
  /** Até 200. */
  pular_ids?: number[]
}
export type MotivoFilaVazia = '' | 'fila_vazia' | 'fora_da_janela'
export interface ProximoResposta {
  lead: LeadCompleto | null
  motivo_vazio: MotivoFilaVazia
  janela: Janela
}

export interface HorarioLivre {
  /** RFC 3339 com offset; é o valor de `reuniao.inicio`. */
  inicio: string
  /** "HH:MM" no fuso do ambiente. */
  rotulo: string
}
export interface HorarioOcupado {
  inicio: string
  fim: string
  titulo: string
}
export interface DiaDisponibilidade {
  data: string
  horarios: HorarioLivre[]
  ocupados: HorarioOcupado[]
}
/** GET /agenda/disponibilidade?user_id=&dias= (até 14). */
export interface Disponibilidade {
  user_id: number
  duracao_min: number
  dias: DiaDisponibilidade[]
}

// ===== 8.3 Bloqueios =====

export type TipoBloqueio = 'telefone' | 'email' | 'cnpj'
export type MotivoBloqueio = 'opt_out' | 'numero_errado' | 'pedido_titular' | 'cliente' | 'manual' | 'lista_externa'

export interface Bloqueio {
  id: number
  tipo: TipoBloqueio
  valor: string
  motivo: MotivoBloqueio
  canal: string
  observacao: string
  criado_por_nome: string
  criado_em: string
}
export interface BloqueiosResposta {
  data: Bloqueio[]
  pagination: Pagination
}
export interface CriarBloqueio {
  tipo: TipoBloqueio
  valor: string
  motivo: MotivoBloqueio
  observacao?: string
}
export interface ImportarBloqueios {
  tipo: TipoBloqueio
  motivo: MotivoBloqueio
  /** Até 5.000. */
  valores: string[]
}
export interface ImportarBloqueiosResposta {
  criados: number
  repetidos: number
  invalidos: number
}

// ===== 8.4 Configurações =====

export interface ConfigProspeccao {
  janela_inicio: number
  janela_fim: number
  sabado: boolean
  max_tentativas: number
  intervalos_horas: number[]
  tentativas_dia: number
  retorno_padrao_horas: number
  meta_ligacoes_dia: number
  exigir_optin_whatsapp: boolean
  incluir_sem_dono_na_fila: boolean
  pipeline_id: number | null
  stage_id: number | null
  playbook_id: number | null
  motivos_descarte: string[]
}

/** Padrões do contrato, para a tela nascer preenchida enquanto a API não responde. */
export const CONFIG_PROSPECCAO_PADRAO: ConfigProspeccao = {
  janela_inicio: 9,
  janela_fim: 18,
  sabado: false,
  max_tentativas: 6,
  intervalos_horas: [3, 24, 48, 72, 120],
  tentativas_dia: 2,
  retorno_padrao_horas: 48,
  meta_ligacoes_dia: 40,
  exigir_optin_whatsapp: true,
  incluir_sem_dono_na_fila: false,
  pipeline_id: null,
  stage_id: null,
  playbook_id: null,
  motivos_descarte: ['Sem interesse', 'Já tem fornecedor', 'Sem orçamento', 'Fora do perfil', 'Empresa fechada']
}

/** Variáveis trocadas no roteiro do cockpit. */
export const VARIAVEIS_ROTEIRO = [
  '{{empresa}}',
  '{{razao_social}}',
  '{{cidade}}',
  '{{uf}}',
  '{{segmento}}',
  '{{decisor}}',
  '{{vendedor}}',
  '{{minha_empresa}}'
] as const

// ===== 9. Funil =====

/** GET /settings/motivos-perda (só autenticação). */
export interface MotivosPerda {
  motivos: string[]
  exigir: boolean
}
/** Corpo de PATCH /deals/{id}/close e, em etapa de perda, de /deals/{id}/stage. */
export interface PerdaRequest {
  motivo?: string
  observacao?: string
}
/** Campos que `Deal` ganha com a prospecção. */
export interface DealProspeccao {
  lost_reason: string
  lost_note: string
  lead_id: number | null
  origem: OrigemLead | ''
}
/** Campos que cada negócio de GET /deals/board ganha (semáforo). */
export interface ProximaAtividade {
  proxima_atividade_em: string | null
  proxima_atividade_tipo: string
}
/** GET /funil: perdidos no período; motivo vazio vem como "Sem motivo informado". */
export interface PerdaPorMotivo {
  motivo: string
  qtd: number
  valor: number
}

// ===== Rótulos das enumerações =====

export const porteLabels: Record<Porte, string> = {
  me: 'Microempresa',
  epp: 'Pequeno porte',
  demais: 'Médio ou grande',
  nao_informado: 'Não informado'
}

export const ordemLabels: Record<OrdemAplicada, string> = {
  relevancia: 'Melhores contatos',
  recentes: 'Mais recentes',
  antigas: 'Mais antigas',
  capital_desc: 'Maior capital',
  divida_desc: 'Maior dívida',
  aleatoria: 'Aleatória',
  base: 'Ordem da base'
}

/** Ordens que o usuário pode escolher (sem `base`). */
export const ORDENS: Ordem[] = ['relevancia', 'recentes', 'antigas', 'capital_desc', 'divida_desc', 'aleatoria']

export const matrizLabels: Record<NonNullable<Filtros['matriz']>, string> = {
  todas: 'Todas',
  matriz: 'Só matriz',
  filial: 'Só filial'
}

export const simNaoLabels: Record<'todos' | 'sim' | 'nao', string> = {
  todos: 'Todos',
  sim: 'Sim',
  nao: 'Não'
}

export const dividaFiltroLabels: Record<NonNullable<Filtros['divida']>, string> = {
  todos: 'Todas',
  com: 'Com dívida ativa',
  sem: 'Sem dívida ativa'
}

export const tipoTelefoneLabels: Record<TipoTelefone, string> = {
  celular: 'Celular',
  fixo: 'Fixo',
  indefinido: 'Telefone'
}

export const origemContatoLabels: Record<OrigemContato, string> = {
  receita: 'Receita Federal',
  wholead: 'Who Lead',
  manual: 'Manual'
}

export const emailTipoLabels: Record<EmailTipo, string> = {
  corporativo: 'Corporativo',
  gratuito: 'Gratuito',
  contabilidade: 'Contabilidade',
  '': ''
}

export const tipoDividaLabels: Record<TipoDivida, string> = {
  geral: 'Geral',
  previdenciaria: 'Previdenciária',
  fgts: 'FGTS'
}

export const tipoSocioLabels: Record<SocioBase['tipo'], string> = {
  pf: 'Pessoa física',
  pj: 'Pessoa jurídica',
  estrangeiro: 'Estrangeiro'
}

export const nivelCnaeLabels: Record<NivelCnae, string> = {
  divisao: 'Divisão',
  grupo: 'Grupo',
  classe: 'Classe',
  subclasse: 'Subclasse'
}

export const estadoCargaLabels: Record<EstadoCarga, string> = {
  ociosa: 'Em dia',
  baixando: 'Baixando',
  carregando: 'Carregando',
  construindo: 'Construindo',
  erro: 'Erro'
}

export const tipoJobLabels: Record<TipoJob, string> = {
  gerar_lista: 'Gerando a lista',
  enriquecer: 'Enriquecendo',
  enviar_crm: 'Enviando ao CRM'
}

export const statusJobLabels: Record<StatusJob, string> = {
  pendente: 'Na fila',
  rodando: 'Em andamento',
  concluido: 'Concluído',
  falhou: 'Falhou',
  cancelado: 'Cancelado'
}

export const origemListaLabels: Record<OrigemLista, string> = {
  base: 'Base de empresas',
  cnpjs: 'CNPJs colados'
}

export const statusListaLabels: Record<StatusLista, string> = {
  gerando: 'Gerando',
  pronta: 'Pronta',
  falhou: 'Falhou'
}

export const estadoItemLabels: Record<EstadoItem, string> = {
  novo: 'Novo',
  enriquecido: 'Enriquecido',
  sem_resultado: 'Sem resultado',
  erro: 'Erro',
  enviado: 'Enviado ao CRM',
  descartado: 'Descartado',
  duplicado: 'Já no CRM',
  bloqueado: 'Bloqueado'
}

/** Classe de `.badge` (styles.css) para cada estado de item. */
export const estadoItemTons: Record<EstadoItem, string> = {
  novo: 'gray',
  enriquecido: 'blue',
  sem_resultado: 'amber',
  erro: 'red',
  enviado: 'green',
  descartado: 'gray',
  duplicado: 'amber',
  bloqueado: 'red'
}

export const ordemItensLabels: Record<OrdemItens, string> = {
  nota: 'Nota',
  razao_social: 'Razão social',
  municipio: 'Cidade',
  estado: 'Estado'
}

export const papelPessoaLabels: Record<PapelPessoa, string> = {
  socio: 'Sócio',
  representante: 'Representante legal',
  funcionario: 'Funcionário'
}

export const duplicadosLabels: Record<EnviarRequest['duplicados'], string> = {
  pular: 'Pular empresas que já estão no CRM',
  vincular: 'Criar o lead ligado à empresa existente'
}

export const modoDistribuicaoLabels: Record<ModoDistribuicao, string> = {
  unico: 'Uma pessoa',
  rodizio: 'Rodízio entre várias pessoas'
}

export const temperaturaLabels: Record<Temperatura, string> = {
  '': 'Sem temperatura',
  fria: 'Fria',
  media: 'Média',
  quente: 'Quente'
}

export const origemLeadLabels: Record<OrigemLead, string> = {
  prospeccao: 'Prospecção',
  manual: 'Manual',
  formulario: 'Formulário',
  importacao: 'Importação'
}

export const filaLeadsLabels: Record<FilaLeads, string> = {
  meus: 'Meus',
  sem_dono: 'Sem dono',
  todos: 'Todos'
}

export const ordemLeadsLabels: Record<OrdemLeads, string> = {
  urgencia: 'Urgência',
  nota: 'Nota',
  criado_em: 'Criação',
  titulo: 'Título'
}

export const acaoLeadsEmMassaLabels: Record<AcaoLeadsEmMassa, string> = {
  dono: 'Atribuir',
  temperatura: 'Temperatura',
  etiqueta: 'Etiqueta',
  descartar: 'Descartar',
  reabrir: 'Reabrir'
}

export const tipoAlertaLabels: Record<TipoAlerta, string> = {
  pessoa_fisica: 'Pessoa física',
  nao_perturbe: 'Não perturbe',
  bloqueado: 'Bloqueado',
  fora_da_janela: 'Fora da janela de contato',
  contador: 'Contato de contador',
  divida: 'Dívida ativa',
  whatsapp_sem_optin: 'WhatsApp sem autorização',
  limite_tentativas: 'Limite de tentativas'
}

/** Classe de `.badge` para os níveis de sinal e de alerta. */
export const nivelTons: Record<NivelSinal | NivelAlerta, string> = {
  ok: 'green',
  info: 'blue',
  atencao: 'amber',
  risco: 'red'
}

export const tipoProximoPassoLabels: Record<TipoProximoPasso, string> = {
  ligar: 'Ligar de novo',
  retornar: 'Retornar',
  reuniao: 'Reunião marcada',
  nenhum: 'Sem próximo passo'
}

export const motivoFilaVaziaLabels: Record<MotivoFilaVazia, string> = {
  '': '',
  fila_vazia: 'Fila em dia',
  fora_da_janela: 'Fora da janela de contato'
}

export const tipoBloqueioLabels: Record<TipoBloqueio, string> = {
  telefone: 'Telefone',
  email: 'E-mail',
  cnpj: 'CNPJ'
}

export const motivoBloqueioLabels: Record<MotivoBloqueio, string> = {
  opt_out: 'Pediu para não ser contatado',
  numero_errado: 'Número errado',
  pedido_titular: 'Pedido do titular (LGPD)',
  cliente: 'Já é cliente',
  manual: 'Manual',
  lista_externa: 'Lista externa'
}

export const codigoErroLabels: Record<CodigoErro, string> = {
  cota_esgotada: 'Cota mensal de prospecção esgotada',
  muitas_requisicoes: 'Muitas consultas em pouco tempo, aguarde alguns segundos',
  base_em_carga: 'A base de empresas ainda não está disponível, tente em alguns minutos',
  base_atualizada: 'A base foi atualizada; refaça a busca',
  filtro_invalido: 'Filtro inválido',
  job_ativo: 'Já existe um processamento em andamento nesta lista',
  custo_mudou: 'O custo estimado mudou; confira de novo',
  estado: 'Este lead já foi encerrado',
  agenda_ocupada: 'Horário ocupado na agenda',
  motivo_obrigatorio: 'Informe o motivo da perda'
}

/** Permissões novas (constantes para `auth.can`). */
export const PERMISSOES_PROSPECCAO = {
  listasVer: 'prospecting.view',
  listasGerenciar: 'prospecting.manage',
  leadsVer: 'leads.view',
  leadsEditar: 'leads.edit',
  leadsDistribuir: 'leads.assign'
} as const
