// Tipos do funil de vendas visual (/funil). Contrato: docs/FUNIL-E-INTELIGENCIA.md, Parte 1.
// Nomes em português e snake_case, exatamente como o backend devolve.

export type FunilPeriodo = '7' | '30' | '90' | '180' | '365' | 'mes' | 'trimestre' | 'ano'
export type FunilModo = 'coorte' | 'fluxo'
export type MotivoRisco = 'estagnado' | 'sem_atividade' | 'adiado' | 'vencido'

export interface FunilPipelineStage {
  id: number
  name: string
  position: number
  probability: number
  is_won: boolean
  is_lost: boolean
  prazo_dias?: number | null
}

export interface FunilPipeline {
  id: number
  name: string
  stages: FunilPipelineStage[]
}

export interface FunilPeriodoInfo {
  de: string
  ate: string
  modo: FunilModo
  rotulo: string
}

export interface FunilResumo {
  abertos: number
  valor_aberto: number
  ponderado: number
  ganhos: number
  valor_ganho: number
  perdidos: number
  valor_perdido: number
  taxa_ganho: number
  ticket_medio: number
  ciclo_mediano_dias: number
  meta: number
  gap: number
  cobertura: number
  em_risco: number
  valor_em_risco: number
}

export interface FunilEtapa {
  stage_id: number
  nome: string
  alcancaram: number
  valor: number
  conversao_total: number
  conversao_etapa: number
  mediana_dias: number
  abertos_agora: number
  valor_aberto: number
  ponderado: number
  estagnados: number
}

export interface MovimentacaoItem {
  qtd: number
  valor: number
}

export interface FunilMovimentacao {
  inicio: MovimentacaoItem
  criados: MovimentacaoItem
  aumentos: MovimentacaoItem
  reducoes: MovimentacaoItem
  adiados: MovimentacaoItem
  ganhos: MovimentacaoItem
  perdidos: MovimentacaoItem
  fim: MovimentacaoItem
}

export type MovimentacaoChave = keyof FunilMovimentacao

export interface GanhoSemana {
  semana: string
  qtd: number
  valor: number
}

export interface FunilDono {
  owner_id: number
  nome: string
  abertos: number
  valor_aberto: number
  ponderado: number
  ganhos: number
  valor_ganho: number
  perdidos: number
  taxa_ganho: number
  em_risco: number
  meta: number
}

export interface Variacao7d {
  valor?: number
  close_date_dias?: number
  etapa?: string
}

export interface FunilNegocio {
  id: number
  name: string
  amount: number
  ponderado: number
  stage_id: number
  stage_name: string
  status: 'aberto' | 'ganho' | 'perdido'
  owner_id: number | null
  owner_name: string
  company_id: number | null
  company_name: string
  close_date: string | null
  created_at: string
  closed_at: string | null
  dias_na_etapa: number
  prazo_dias: number
  dias_sem_atividade: number
  adiamentos: number
  variacao_7d?: Variacao7d | null
  motivos: MotivoRisco[]
}

export interface FunilResposta {
  pipeline: FunilPipeline
  periodo: FunilPeriodoInfo
  resumo: FunilResumo
  funil: FunilEtapa[]
  movimentacao: FunilMovimentacao
  ganhos_por_semana: GanhoSemana[]
  por_dono: FunilDono[]
  negocios: FunilNegocio[]
}

/** Segmento clicado em algum gráfico; filtra a lista de negócios. */
export type SelecaoTipo = 'etapa' | 'movimentacao' | 'dono' | 'risco'

export interface FunilSelecao {
  tipo: SelecaoTipo | null
  valor: number | string | null
}

// ===== Tipos de apoio dos gráficos =====

export type WaterfallTipo = 'inicio' | 'entrada' | 'ganho' | 'perdido' | 'reducao' | 'fim'

export interface WaterfallPasso {
  key: MovimentacaoChave
  rotulo: string
  valor: number
  qtd: number
  tipo: WaterfallTipo
}

export interface SparkPonto {
  rotulo: string
  valor: number
}
