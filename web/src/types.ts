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
  ec_number: string
  economic_group: string
  cnpj: string
  accredited_at: string | null
  representative: string
  instagram: string
  products: string[]
  machines_count: number
  is_client: boolean
  anticipation_mode: string
  validator: boolean
  do_not_disturb: boolean
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
  last_activity_at?: string | null
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
  temperature?: string
  close_date: string | null
  position: number
  closed_at: string | null
  last_activity_at?: string | null
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

export interface Ticket {
  id: number
  subject: string
  description: string
  status: 'aberto' | 'pendente' | 'resolvido' | 'fechado'
  priority: string
  contact_id: number | null
  contact_name?: string
  company_id: number | null
  company_name?: string
  owner_id: number | null
  owner_name?: string
  closed_at: string | null
  created_at: string
  updated_at: string
}

export interface ListRules {
  lifecycle_stage?: string
  owner_id?: number
  source?: string
}

export interface ContactList {
  id: number
  name: string
  kind: 'estatica' | 'dinamica'
  rules?: ListRules
  created_by: number | null
  members_count: number
  created_at: string
  updated_at: string
}

export interface Project {
  id: number
  name: string
  description: string
  status: 'ativo' | 'concluido' | 'arquivado'
  due_date: string | null
  owner_id: number | null
  owner_name?: string
  tasks_total: number
  tasks_done: number
  created_at: string
  updated_at: string
}

export interface Conversation {
  id: number
  subject: string
  contact_id: number | null
  contact_name?: string
  peer_email: string
  status: 'aberta' | 'fechada'
  unread: boolean
  last_message_at: string
  last_preview?: string
  created_at: string
}

export interface ConversationMessage {
  id: number
  conversation_id: number
  direction: 'recebida' | 'enviada'
  from_email: string
  to_email: string
  subject: string
  body: string
  user_id: number | null
  user_name?: string
  created_at: string
}

export interface Call {
  id: number
  direction: 'entrada' | 'saida'
  outcome: string
  duration_seconds: number
  notes: string
  called_at: string
  contact_id: number | null
  contact_name?: string
  company_id: number | null
  deal_id: number | null
  user_id: number | null
  user_name?: string
  created_at: string
}

export interface Meeting {
  id: number
  title: string
  status: 'agendada' | 'realizada' | 'cancelada' | 'nao_compareceu'
  starts_at: string
  ends_at: string | null
  location: string
  notes: string
  contact_id: number | null
  contact_name?: string
  company_id: number | null
  deal_id: number | null
  user_id: number | null
  user_name?: string
  created_at: string
  updated_at: string
}

export interface Playbook {
  id: number
  name: string
  description: string
  body: string
  active: boolean
  created_by: number | null
  created_at: string
  updated_at: string
}

export interface MessageTemplate {
  id: number
  name: string
  subject: string
  body: string
  created_by: number | null
  created_at: string
  updated_at: string
}

export interface Snippet {
  id: number
  name: string
  shortcut: string
  body: string
  created_by: number | null
  created_at: string
  updated_at: string
}

export interface SavedView {
  id: number
  entity: 'contacts' | 'companies' | 'deals'
  name: string
  filters: Record<string, unknown>
  position: number
  created_by: number | null
  created_by_name?: string
  created_at: string
}

export interface FormField {
  key: string
  visible: boolean
  required: boolean
}

export interface FilterCondition {
  field: string
  op: string
  value?: string
  values?: string[]
}

export interface FilterGroup {
  conditions: FilterCondition[]
}

export interface FilterFieldDef {
  key: string
  label: string
  kind: 'text' | 'enum' | 'ref' | 'date' | 'number'
  options?: { value: string; label: string }[]
}
