// Tipos da inteligência de empresas (enriquecimento) — contrato em
// docs/FUNIL-E-INTELIGENCIA.md, Parte 2.
import type { Company } from './types'

export type Provedor = 'receita' | 'bigdatacorp' | 'serasa'
export type NivelSinal = 'ok' | 'atencao' | 'risco'

export interface Sinal {
  tipo: string
  nivel: NivelSinal
  texto: string
}

/** Campos normalizados que podem ser aplicados na ficha da empresa. */
export interface PerfilCampos {
  name?: string
  legal_name?: string
  trade_name?: string
  cnae_code?: string
  cnae_description?: string
  industry?: string
  legal_nature?: string
  founded_at?: string
  capital?: number
  company_size?: string
  registration_status?: string
  registration_status_at?: string
  simples?: boolean
  mei?: boolean
  street?: string
  street_number?: string
  complement?: string
  neighborhood?: string
  zip?: string
  city?: string
  state?: string
  phone?: string
  email?: string
  website?: string
  domain?: string
  employees_range?: string
  revenue_range?: string
  [campo: string]: unknown
}

export interface Socio {
  nome: string
  documento?: string
  qualificacao?: string
  desde?: string
  tipo?: string
}

export interface CnaeSecundario {
  codigo: string
  descricao: string
}

export interface Indicadores {
  nivel_atividade?: number | null
  tem_dominio_ativo?: boolean | null
  tem_ssl?: boolean | null
  tem_email_corporativo?: boolean | null
  funcionarios_faixa?: string
  faturamento_faixa?: string
}

export interface TelefoneEncontrado {
  numero: string
  tipo?: string
  principal?: boolean
  validado?: boolean
}

export interface EmailEncontrado {
  email: string
  tipo?: string
  validado?: boolean
}

export interface Contatos {
  telefones?: TelefoneEncontrado[]
  emails?: EmailEncontrado[]
}

export interface DominioEncontrado {
  dominio: string
  ativo?: boolean
  ssl?: boolean
}

export interface PresencaDigital {
  dominios?: DominioEncontrado[]
  redes?: Record<string, string>
}

export interface Risco {
  score?: number | null
  faixa?: string
  nivel?: 'baixo' | 'medio' | 'alto' | string
  pendencias_qtd?: number
  pendencias_valor?: number
  protestos_qtd?: number
  cheques_sem_fundo?: number
  acoes_qtd?: number
  consultas_90d?: number
  limite_sugerido?: number
  observacao?: string
}

export interface Processo {
  numero?: string
  tipo?: string
  tribunal?: string
  status?: string
  polo?: string
  valor?: number
  data?: string
}

export interface Processos {
  total?: number
  ativos?: number
  como_reu?: number
  valor_total?: number
  ultimos?: Processo[]
}

export interface Conformidade {
  pep?: boolean
  sancionado?: boolean
  listas?: string[]
}

export interface Noticia {
  titulo: string
  url?: string
  sentimento?: 'positivo' | 'neutro' | 'negativo' | string
  data?: string
}

export interface Midia {
  exposicao?: string
  noticias?: Noticia[]
}

/** Perfil normalizado devolvido pelos provedores (e o consolidado em `intel`). */
export interface Perfil {
  provedor?: Provedor | string
  demo?: boolean
  consultado_em?: string
  campos?: PerfilCampos
  socios?: Socio[]
  cnaes_secundarios?: CnaeSecundario[]
  indicadores?: Indicadores
  contatos?: Contatos
  presenca_digital?: PresencaDigital
  risco?: Risco
  processos?: Processos
  conformidade?: Conformidade
  midia?: Midia
  sinais?: Sinal[]
  custo_centavos?: number
}

/** GET /cnpj/{cnpj} */
export interface ConsultaCnpj {
  perfil: Perfil
  existente: { id: number; name: string } | null
}

export interface FonteCampo {
  provedor: Provedor | string
  fetched_at: string
}

export interface HistoricoConsulta {
  id: number
  provedor: Provedor | string
  status: 'ok' | 'sem_resultado' | 'erro' | string
  demo: boolean
  custo_centavos: number
  datasets?: string | null
  user_name?: string
  created_at: string
  aplicado_em?: string | null
}

export interface UsoMes {
  consultas: number
  custo_centavos: number
  limite_centavos: number
}

/** GET /companies/{id}/intel */
export interface IntelEmpresa {
  company: Company
  intel: Perfil | null
  fontes: Record<string, FonteCampo>
  historico: HistoricoConsulta[]
  uso_mes: UsoMes
}

export interface ConsultaResultado {
  provedor: Provedor | string
  status: 'ok' | 'sem_resultado' | 'erro' | string
  mensagem?: string
  demo?: boolean
  custo_centavos?: number
  cache?: boolean
}

export interface DiffCampo {
  campo: string
  label: string
  atual: unknown
  proposto: unknown
  provedor: Provedor | string
  sugerido: boolean
}

/** POST /companies/{id}/enrich/preview */
export interface EnrichPreview {
  enrichment_ids: number[]
  consultas: ConsultaResultado[]
  perfil: Perfil
  diff: DiffCampo[]
  custo_centavos: number
}

/** POST /companies/{id}/enrich/apply */
export interface EnrichApply {
  company: Company
  aplicados: string[]
}

/** POST /companies/enrich/bulk */
export interface EnrichBulk {
  processados: number
  enriquecidos: number
  sem_cnpj: number
  erros: { id: number; mensagem: string }[]
}

export interface BigDataCorpSettings {
  habilitado: boolean
  demo: boolean
  token_id: string
  has_secret?: boolean
  /** Só no PUT, em claro; vazio mantém o atual. */
  access_token?: string
  datasets_padrao: string[]
  precos_centavos: Record<string, number>
}

export interface SerasaSettings {
  habilitado: boolean
  demo: boolean
  ambiente: 'uat' | 'prod' | string
  client_id: string
  has_secret?: boolean
  /** Só no PUT, em claro; vazio mantém o atual. */
  client_secret?: string
  report_name: string
  optional_features: string
  preco_centavos: number
}

export interface EnrichmentSettings {
  auto_receita: boolean
  sobrescrever: 'vazios' | 'sempre' | string
  limite_mensal_centavos: number
  prazo_cache_dias: number
  bigdatacorp: BigDataCorpSettings
  serasa: SerasaSettings
}

export interface DatasetCatalogo {
  codigo: string
  nome?: string
  descricao?: string
  preco_centavos?: number
}

export interface FonteCatalogo {
  codigo: Provedor | string
  nome?: string
  descricao?: string
  custo?: string
  gratis?: boolean
  datasets?: DatasetCatalogo[]
}

/** GET /settings/enrichment */
export interface EnrichmentSettingsResposta {
  settings: EnrichmentSettings
  fontes: FonteCatalogo[]
  uso_mes: UsoMes
}

export interface UsoProvedor {
  consultas?: number
  custo_centavos?: number
}

export interface UsoMensal {
  mes: string
  consultas: number
  custo_centavos: number
  por_provedor?: Record<string, UsoProvedor | number>
}

/** GET /settings/enrichment/usage */
export interface UsoHistorico {
  meses: UsoMensal[]
}

/** POST /settings/enrichment/test */
export interface TesteFonte {
  ok: boolean
  mensagem: string
  custo_centavos?: number
  demo?: boolean
}
