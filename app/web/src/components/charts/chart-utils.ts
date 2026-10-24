import { reactive } from 'vue'
import { formatCompact, formatMoney } from '../../format'

const pct0 = new Intl.NumberFormat('pt-BR', { style: 'percent', maximumFractionDigits: 0 })
const pct1 = new Intl.NumberFormat('pt-BR', { style: 'percent', maximumFractionDigits: 1 })

/** Fração (0–1) como percentual pt-BR: 0.615 -> "62%" (ou "61,5%" com uma casa). */
export function formatPct(fraction: number | null | undefined, digits: 0 | 1 = 0): string {
  const v = Number.isFinite(fraction as number) ? (fraction as number) : 0
  return (digits === 1 ? pct1 : pct0).format(v)
}

/** Dinheiro curto para rótulos de gráfico: 148000 -> "R$ 148 mil". */
export function formatMoneyShort(value: number | null | undefined): string {
  const v = value ?? 0
  if (Math.abs(v) < 1000) return formatMoney(v)
  return `R$ ${formatCompact(v)}`
}

/** Dinheiro curto com sinal explícito (para o waterfall). */
export function formatSigned(value: number): string {
  if (value === 0) return formatMoneyShort(0)
  return `${value > 0 ? '+' : '−'}${formatMoneyShort(Math.abs(value))}`
}

export function formatDias(dias: number | null | undefined): string {
  if (dias == null) return '—'
  return `${Math.round(dias)} d`
}

/** Respeita a preferência do sistema por menos movimento; seguro fora do navegador. */
export function prefersReducedMotion(): boolean {
  try {
    return typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  } catch {
    return false
  }
}

export interface TooltipLinha {
  rotulo: string
  valor: string
  tom?: 'ok' | 'danger' | 'warn' | 'muted'
}

export interface TooltipEstado {
  visivel: boolean
  x: number
  y: number
  titulo: string
  linhas: TooltipLinha[]
}

/** Estado compartilhado de tooltip: os gráficos chamam mostrar/mover/esconder nos eventos do mouse. */
export function useTooltip() {
  const tooltip = reactive<TooltipEstado>({ visivel: false, x: 0, y: 0, titulo: '', linhas: [] })

  function mostrar(event: MouseEvent, titulo: string, linhas: TooltipLinha[]) {
    tooltip.visivel = true
    tooltip.titulo = titulo
    tooltip.linhas = linhas
    mover(event)
  }

  function mover(event: MouseEvent) {
    tooltip.x = event.clientX
    tooltip.y = event.clientY
  }

  function esconder() {
    tooltip.visivel = false
  }

  return { tooltip, mostrar, mover, esconder }
}

/** Gera um id estável por instância para <pattern>/<clipPath> dentro do SVG. */
let seq = 0
export function chartId(prefix: string): string {
  seq += 1
  return `${prefix}-${seq}`
}
