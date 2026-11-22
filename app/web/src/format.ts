import type { EstadoLead, Resultado, Urgencia } from './types-prospeccao'

const brl = new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' })

export function formatMoney(value: number | null | undefined): string {
  return brl.format(value ?? 0)
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return '—'
  const d = new Date(value)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleDateString('pt-BR')
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  const d = new Date(value)
  if (isNaN(d.getTime())) return '—'
  return `${d.toLocaleDateString('pt-BR')} ${d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}`
}

/** Data relativa curta para a timeline (hoje, ontem, N dias atrás ou data). */
export function relativeDate(value: string | null | undefined): string {
  if (!value) return '—'
  const d = new Date(value)
  if (isNaN(d.getTime())) return '—'
  const days = Math.floor((Date.now() - d.getTime()) / 86400000)
  if (days <= 0) return 'hoje'
  if (days === 1) return 'ontem'
  if (days < 30) return `${days} dias atrás`
  return d.toLocaleDateString('pt-BR')
}

const compact = new Intl.NumberFormat('pt-BR', { notation: 'compact', maximumFractionDigits: 2 })

/** Número compacto no estilo HubSpot: 3180 -> "3,18 mil". */
export function formatCompact(value: number | null | undefined): string {
  return compact.format(value ?? 0)
}

export function initials(name: string | undefined | null): string {
  if (!name) return '?'
  const parts = name.trim().split(/\s+/)
  const first = parts[0]?.[0] ?? ''
  const last = parts.length > 1 ? parts[parts.length - 1][0] : ''
  return (first + last).toUpperCase() || '?'
}

export const lifecycleLabels: Record<string, string> = {
  lead: 'Lead',
  mql: 'MQL',
  sql: 'SQL',
  oportunidade: 'Oportunidade',
  cliente: 'Cliente',
  perdido: 'Perdido'
}

export const taskTypeLabels: Record<string, string> = {
  ligacao: 'Ligação',
  email: 'E-mail',
  reuniao: 'Reunião',
  tarefa: 'Tarefa'
}

export const roleLabels: Record<string, string> = {
  admin: 'Admin',
  manager: 'Manager',
  seller: 'Seller'
}

// ===== Prospecção =====
// CNPJ alfanumérico: a implementação mora em cnpj.ts; as telas importam daqui.
export { normalizarCNPJ, cnpjValido, mascaraCNPJ, separarCNPJs } from './cnpj'

const inteiro = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 0 })

/** Número inteiro com separador de milhar: 12480 -> "12.480". */
export function formatNumber(value: number | null | undefined): string {
  return inteiro.format(value ?? 0)
}

/** Dinheiro guardado em centavos (custo de enriquecimento): 8050 -> "R$ 80,50". */
export function formatCentavos(value: number | null | undefined): string {
  return brl.format((value ?? 0) / 100)
}

/** Duração de ligação: 75 -> "1:15"; 3725 -> "1:02:05". */
export function formatDuracao(segundos: number | null | undefined): string {
  const total = Math.max(0, Math.floor(segundos ?? 0))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = String(total % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`
}

/** Mês de competência da cota: "2026-11" -> "novembro". */
export function formatCompetencia(mes: string | null | undefined): string {
  const m = /^(\d{4})-(\d{2})$/.exec(mes ?? '')
  if (!m) return mes ?? ''
  return new Date(Number(m[1]), Number(m[2]) - 1, 1).toLocaleDateString('pt-BR', { month: 'long' })
}

/**
 * Total da contagem ao vivo conforme `total_modo`: "12.480", "mais de 10.000"
 * ou "mais de 10.000, até 48.200" quando o serviço informa o limite superior.
 */
export function formatContagem(
  c: { total: number; total_modo: string; total_limite_superior?: number | null } | null | undefined
): string {
  if (!c) return '—'
  if (c.total_modo !== 'teto') return formatNumber(c.total)
  const base = `mais de ${formatNumber(c.total)}`
  return c.total_limite_superior ? `${base}, até ${formatNumber(c.total_limite_superior)}` : base
}

export const estadoLeadLabels: Record<EstadoLead, string> = {
  novo: 'Novo',
  tentando: 'Tentando contato',
  retornar: 'Retornar',
  convertido: 'Convertido',
  descartado: 'Descartado'
}

/** Classe de `.badge` (styles.css) de cada estado de lead. */
export const estadoLeadTons: Record<EstadoLead, string> = {
  novo: 'blue',
  tentando: 'amber',
  retornar: '',
  convertido: 'green',
  descartado: 'gray'
}

export const resultadoLabels: Record<Resultado, string> = {
  atendeu: 'Atendeu',
  reuniao: 'Reunião marcada',
  retornar: 'Retornar',
  sem_interesse: 'Sem interesse',
  nao_e_decisor: 'Não é o decisor',
  nao_atendeu: 'Não atendeu',
  caixa_postal: 'Caixa postal',
  ocupado: 'Ocupado',
  numero_errado: 'Número errado',
  nao_ligar_mais: 'Não ligar mais'
}

export const urgenciaLabels: Record<Urgencia, string> = {
  atrasado: 'Atrasado',
  hoje: 'Hoje',
  futuro: 'Agendado',
  nenhuma: 'Sem próxima ação'
}

/** Semáforo: vermelho = atrasado, verde = hoje, cinza = futuro, amarelo = sem próxima ação. */
export const urgenciaTons: Record<Urgencia, string> = {
  atrasado: 'red',
  hoje: 'green',
  futuro: 'gray',
  nenhuma: 'amber'
}

const motivosDescarteSistema: Record<string, string> = {
  sem_telefone: 'Sem telefone válido',
  opt_out: 'Pediu para não ligar',
  sem_contato: 'Sem contato após as tentativas',
  manual: 'Descartado manualmente'
}

/**
 * `Lead.motivo_descarte` guarda o texto escolhido em `motivos_descarte` ou um
 * código reservado do sistema; aqui o código vira rótulo e o texto passa direto.
 */
export function motivoDescarteLabel(motivo: string | null | undefined): string {
  if (!motivo) return ''
  return motivosDescarteSistema[motivo] ?? motivo
}

/**
 * Semáforo da próxima atividade de um negócio (cartão do quadro) ou da
 * próxima ação de um lead, calculado no navegador a partir do instante.
 */
export function urgenciaDe(instante: string | null | undefined, agora: Date = new Date()): Urgencia {
  if (!instante) return 'nenhuma'
  const d = new Date(instante)
  if (isNaN(d.getTime())) return 'nenhuma'
  const inicioHoje = new Date(agora.getFullYear(), agora.getMonth(), agora.getDate()).getTime()
  if (d.getTime() < inicioHoje) return 'atrasado'
  return d.getTime() < inicioHoje + 86400000 ? 'hoje' : 'futuro'
}
