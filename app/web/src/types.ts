export interface User {
  id: number
  name: string
  email: string
  role: 'admin' | 'manager' | 'seller'
  active: boolean
  team_id?: number | null
  team_name?: string
  created_at: string
  updated_at: string
  /** Convite ainda com a senha temporária: precisa trocar antes de usar o CRM. */
  must_change_password?: boolean
  invite_expires_at?: string | null
}

export interface Attachment {
  id: number
  entity: 'contato' | 'empresa' | 'negocio' | 'ticket'
  entity_id: number
  filename: string
  content_type: string
  size_bytes: number
  uploaded_by: number | null
  uploader_name?: string
  created_at: string
}

export interface PanelItem {
  id: number
  report_id: number
  name: string
  position: number
  width: string
  result?: ReportResult
}

export interface Panel {
  id: number
  name: string
  shared: boolean
  created_by: number | null
  items: PanelItem[]
  created_at: string
  updated_at: string
}

export interface WorkspaceItem {
  id: number
  title: string
  subtitle: string
  due: string | null
  amount?: number
  contact_id?: number | null
  company_id?: number | null
  deal_id?: number | null
  reason?: string
}

export interface Workspace {
  overdue_tasks: WorkspaceItem[]
  today_tasks: WorkspaceItem[]
  today_meetings: WorkspaceItem[]
  stale_deals: WorkspaceItem[]
  closing_soon: WorkspaceItem[]
  untouched_leads: WorkspaceItem[]
  open_deals: number
  open_amount: number
  won_month: number
  goal_month: number
  tasks_pending: number
  meetings_week: number
  target_accounts: number
}

export interface TrackedGoal {
  id: number
  kind: string
  metric: string
  user_id: number | null
  user_name?: string
  pipeline_id: number | null
  pipeline_name?: string
  stage_id: number | null
  stage_name?: string
  activity_kind: string
  amount: number
  start_period: string
  end_period: string
  created_by: number | null
  created_at: string
  updated_at: string
  finished: boolean
  current_value: number
  attainment: number
}

export interface GoalPoint {
  month: string
  actual: number
  target: number
  attainment: number
}

export interface SalesGoal {
  id: number
  user_id: number | null
  period: string
  amount: number
}

export interface ForecastRow {
  owner_id: number | null
  owner_name: string
  won: number
  committed: number
  weighted: number
  open_deals: number
  goal: number
  attainment: number
}

export interface Forecast {
  period: string
  rows: ForecastRow[]
  team_goal: number
  won: number
  committed: number
  weighted: number
  projected: number
  gap: number
}

export interface CategoryRow {
  owner_id: number | null
  owner_name: string
  pipeline: number
  best_case: number
  committed: number
  closed: number
  goal: number
  submitted: number
  submitted_note?: string
}

export interface CategoryForecast {
  period: string
  rows: CategoryRow[]
  pipeline: number
  best_case: number
  committed: number
  closed: number
  submitted: number
  team_goal: number
  gap: number
}

export interface ForecastSubmission {
  id: number
  user_id: number
  user_name?: string
  period: string
  amount: number
  note: string
  updated_at: string
}

export interface StageConversion {
  stage_id: number
  stage_name: string
  count: number
  amount: number
  rate: number
}

export interface SalesAnalytics {
  days: number
  created: number
  won: number
  lost: number
  win_rate: number
  avg_ticket: number
  avg_cycle_days: number
  won_amount: number
  funnel: StageConversion[]
  activity_by_kind: ReportRow[]
}

export interface ReportFilters {
  days: number
  owner_id: number
  status: string
  pipeline_id?: number
}

export interface Report {
  id: number
  kind?: string
  name: string
  description: string
  entity: string
  metric: string
  dimension: string
  filters: ReportFilters
  chart: string
  shared: boolean
  position: number
  created_by: number | null
  created_at: string
  updated_at: string
}

export interface ReportRow {
  label: string
  value: number
  percent?: number
}

export interface ReportSeries {
  name: string
  points: ReportRow[]
}

export interface ReportResult {
  rows: ReportRow[]
  series?: ReportSeries[]
  total: number
  metric_label: string
  is_money: boolean
  dimension_label: string
}

export interface ReportCatalogEntry {
  key: string
  label: string
  metrics: Record<string, string>
  dimensions: Record<string, string>
}

export interface SequenceStep {
  kind: string
  delay_days: number
  subject?: string
  body?: string
  template_id?: number
  title?: string
  note?: string
}

export interface Sequence {
  id: number
  name: string
  description: string
  steps: SequenceStep[]
  active: boolean
  dynamic: boolean
  exit_on_reply: boolean
  exit_on_meeting: boolean
  owner_id: number | null
  owner_name?: string
  created_by: number | null
  created_at: string
  updated_at: string
  enrolled: number
  active_members: number
  open_rate: number
  reply_rate: number
}

export interface SequenceMember {
  id: number
  sequence_id: number
  contact_id: number
  contact_name?: string
  contact_email?: string
  step: number
  step_label?: string
  next_run_at: string
  status: string
  exit_reason: string
  task_id: number | null
  engaged: boolean
  enrolled_at: string
  finished_at: string | null
}

export interface AutomationAction {
  kind: string
  config?: Record<string, unknown>
}

export interface Automation {
  id: number
  name: string
  description: string
  trigger_kind: string
  trigger_config: Record<string, unknown>
  actions: AutomationAction[]
  active: boolean
  runs: number
  last_run_at: string | null
  created_by: number | null
  created_at: string
  updated_at: string
}

export interface TimeWindow {
  start: string
  end: string
}

export interface BookingPage {
  id: number
  user_id: number
  user_name?: string
  user_email?: string
  slug: string
  title: string
  description: string
  location: string
  duration_min: number
  buffer_min: number
  days_ahead: number
  notice_hours: number
  weekly_hours: Record<string, TimeWindow[]>
  active: boolean
  bookings: number
  created_at: string
  updated_at: string
}

export interface PublicFormField {
  key: string
  label: string
  type: string
  required: boolean
  options?: string[]
}

export interface PublicForm {
  id: number
  slug: string
  name: string
  headline: string
  description: string
  fields: PublicFormField[]
  submit_label: string
  success_message: string
  redirect_url: string
  owner_id: number | null
  owner_name?: string
  list_id: number | null
  list_name?: string
  lifecycle_stage: string
  source: string
  active: boolean
  submissions: number
  created_by: number | null
  created_at: string
  updated_at: string
}

export interface EmailMessage {
  id: number
  subject: string
  to_email: string
  contact_id: number | null
  contact_name?: string
  deal_id: number | null
  user_id: number | null
  user_name?: string
  source: 'manual' | 'automacao'
  opens: number
  clicks: number
  first_open_at: string | null
  last_open_at: string | null
  first_click_at: string | null
  sent_at: string
}

export interface EmailEvent {
  id: number
  message_id: number
  kind: 'abertura' | 'clique'
  url?: string
  ip?: string
  user_agent?: string
  created_at: string
}

export interface AuditEntry {
  id: number
  user_id: number | null
  user_name: string
  action: string
  entity: string
  entity_id: number | null
  summary: string
  ip: string
  created_at: string
}

export interface Team {
  id: number
  name: string
  members_count: number
  created_at: string
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
  is_target: boolean
  target_tier: number
  target_notes: string
  target_since: string | null
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
  buying_role: string
  last_activity_at?: string | null
  created_at: string
  updated_at: string
}

export interface TargetAccount extends Company {
  contacts_count_total: number
  decision_makers: number
  open_deals: number
  open_amount: number
  won_amount: number
  last_activity_at: string | null
  open_tasks: number
}

export interface TargetSummary {
  total: number
  tier1: number
  with_deals: number
  open_amount: number
  no_activity_30d: number
  without_decision_maker: number
}

export interface PipelineStage {
  id: number
  pipeline_id: number
  name: string
  position: number
  probability: number
  is_won: boolean
  is_lost: boolean
  deals_count?: number
}

export interface Pipeline {
  id: number
  name: string
  position: number
  deals_count?: number
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
  forecast_category?: string
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
  ticket_id?: number | null
  contact_name?: string
  company_name?: string
  deal_name?: string
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
  owner_id: number | null
  owner_name?: string
  last_message_at: string
  last_preview?: string
  created_at: string
}

export interface ImportRecord {
  id: number
  file_name: string
  entity: 'contatos' | 'empresas'
  status: 'concluida' | 'falhou'
  total_rows: number
  new_records: number
  updated_records: number
  new_associations: number
  error_count: number
  errors: string[]
  created_by: number | null
  created_by_name?: string
  created_at: string
}

export interface InboxCounters {
  unassigned: number
  mine: number
  open: number
  closed: number
}

export interface ConversationMessage {
  id: number
  conversation_id: number
  direction: 'recebida' | 'enviada' | 'comentario'
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

export interface PropertyOption {
  value: string
  label: string
}

export interface CustomProperty {
  id: number
  entity: 'contacts' | 'companies' | 'deals' | 'tickets'
  key: string
  label: string
  description: string
  field_type: 'texto' | 'texto_longo' | 'numero' | 'data' | 'selecao' | 'multipla' | 'booleano'
  options: PropertyOption[]
  group_name: string
  position: number
  created_by: number | null
  creator_name?: string
  used_count: number
  created_at: string
  updated_at: string
}
