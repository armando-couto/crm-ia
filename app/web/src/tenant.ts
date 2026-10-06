/**
 * Identidade do ambiente do cliente, injetada no index.html pela API ao
 * servir a página. Ler daqui (e não de import.meta.env) é o que permite um
 * único build do Vue servir todos os clientes — cada um no seu endereço
 * (/<slug>/), com o próprio nome, cor e logo.
 */
export interface TenantInfo {
  slug: string
  name: string
  color: string
  logo_url: string
  version: string
  base_path: string
  app_url: string
  plan_enabled: boolean
  setup_done: boolean
}

declare global {
  interface Window {
    __TENANT__?: Partial<TenantInfo> | string
  }
}

const PADRAO: TenantInfo = {
  slug: 'dev',
  name: 'CRM IA',
  color: '#6d5df6',
  logo_url: '',
  version: 'dev',
  base_path: '',
  app_url: '',
  plan_enabled: false,
  setup_done: true
}

function lerBruto(): Partial<TenantInfo> {
  const bruto = typeof window !== 'undefined' ? window.__TENANT__ : undefined
  // Em dev sem o plugin, o placeholder fica cru no HTML.
  if (!bruto || typeof bruto === 'string') return {}
  return bruto
}

const bruto = lerBruto()

/** Identidade carregada no boot (a do servidor; o store atualiza depois). */
export const tenant: TenantInfo = {
  ...PADRAO,
  ...Object.fromEntries(Object.entries(bruto).filter(([, v]) => v !== undefined && v !== null && v !== ''))
} as TenantInfo

/** Base das rotas do SPA: "/" na raiz, "/minhaempresa/" num cliente. */
export function basePath(): string {
  const b = (tenant.base_path || '').replace(/\/+$/, '')
  return b ? `${b}/` : '/'
}

/** Raiz da API, respeitando o prefixo do cliente: "/minhaempresa/api/v1". */
export function apiBase(): string {
  return `${basePath().replace(/\/$/, '')}/api/v1`
}

/** Chave de armazenamento local isolada por cliente (todos dividem a origem). */
export function chaveLocal(nome: string): string {
  return `crmia:${tenant.slug}:${nome}`
}

/**
 * Aplica a cor primária como variáveis CSS, derivando os tons escuros e
 * claros a partir de um único hex. O cliente escolhe uma cor e o sistema
 * inteiro acompanha.
 */
export function aplicarTema(cor: string, logo?: string): void {
  const hex = /^#[0-9a-f]{6}$/i.test(cor || '') ? cor : PADRAO.color
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16))
  const root = document.documentElement.style
  root.setProperty('--ci-purple', hex)
  root.setProperty('--ci-purple-dark', escurecer(r, g, b, 0.22))
  root.setProperty('--ci-purple-deep', escurecer(r, g, b, 0.72))
  root.setProperty('--ci-purple-tint', clarear(r, g, b, 0.9))
  root.setProperty('--ci-purple-rgb', `${r}, ${g}, ${b}`)
  if (logo !== undefined) document.documentElement.dataset.logo = logo ? '1' : ''
  const meta = document.querySelector('meta[name="theme-color"]')
  if (meta) meta.setAttribute('content', hex)
}

const clamp = (n: number) => Math.max(0, Math.min(255, Math.round(n)))
const toHex = (r: number, g: number, b: number) =>
  '#' + [r, g, b].map((v) => clamp(v).toString(16).padStart(2, '0')).join('')
const escurecer = (r: number, g: number, b: number, f: number) => toHex(r * (1 - f), g * (1 - f), b * (1 - f))
const clarear = (r: number, g: number, b: number, f: number) =>
  toHex(r + (255 - r) * f, g + (255 - g) * f, b + (255 - b) * f)
