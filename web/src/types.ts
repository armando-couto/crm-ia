export interface User {
  id: number
  name: string
  email: string
  role: 'admin' | 'gestor' | 'vendedor'
  active: boolean
  created_at: string
  updated_at: string
}

export interface Company {
  id: number
  name: string
  domain: string
  phone: string
  industry: string
  city: string
  state: string
  owner_id: number | null
  owner_name?: string
  contacts_count?: number
  created_at: string
  updated_at: string
}

export interface Contact {
  id: number
  first_name: string
  last_name: string
  email: string
  phone: string
  job_title: string
  lifecycle_stage: string
  source: string
  company_id: number | null
  company_name?: string
  owner_id: number | null
  owner_name?: string
  created_at: string
  updated_at: string
}

export interface PipelineStage {
  id: number
  pipeline_id: number
  name: string
  position: number
  probability: number
  is_won: boolean
  is_lost: boolean
}

export interface Pipeline {
  id: number
  name: string
  position: number
  stages: PipelineStage[]
}

export interface Deal {
  id: number
  name: string
  amount: number
  currency: string
  pipeline_id: number
  stage_id: number
  stage_name?: string
  contact_id: number | null
  contact_name?: string
  company_id: number | null
  company_name?: string
  owner_id: number | null
  owner_name?: string
  status: 'aberto' | 'ganho' | 'perdido'
  close_date: string | null
  position: number
  closed_at: string | null
  created_at: string
  updated_at: string
}

export interface Task {
  id: number
  title: string
  description: string
  type: string
  priority: string
  due_date: string | null
  completed_at: string | null
  owner_id: number | null
  owner_name?: string
  contact_id: number | null
  contact_name?: string
  company_id: number | null
  deal_id: number | null
  created_at: string
  updated_at: string
}

export interface Activity {
  id: number
  kind: string
  content: string
  metadata?: Record<string, unknown>
  user_id: number | null
  user_name?: string
  contact_id: number | null
  company_id: number | null
  deal_id: number | null
  created_at: string
}

export interface Pagination {
  page: number
  per_page: number
  total: number
}

export interface Paginated<T> {
  data: T[]
  pagination: Pagination
}

export interface StageMetric {
  stage_id: number
  stage_name: string
  count: number
  amount: number
}

export interface MonthMetric {
  month: string
  count: number
  amount: number
}

export interface OwnerMetric {
  owner_id: number
  owner_name: string
  won: number
  amount: number
}

export interface Dashboard {
  total_contacts: number
  total_companies: number
  open_deals: number
  open_amount: number
  forecast_amount: number
  won_this_month: number
  won_amount_month: number
  lost_this_month: number
  tasks_pending: number
  tasks_overdue: number
  deals_by_stage: StageMetric[]
  won_by_month: MonthMetric[]
  ranking_owners: OwnerMetric[]
  new_contacts_week: number
}

export interface SearchResult {
  type: 'contato' | 'empresa' | 'negocio'
  id: number
  title: string
  sub: string
}
