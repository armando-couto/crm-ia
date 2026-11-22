import { apiBase, chaveLocal } from './tenant'

const TOKEN_KEY = chaveLocal('token')

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function setToken(token: string | null): void {
  try {
    if (token) {
      localStorage.setItem(TOKEN_KEY, token)
    } else {
      localStorage.removeItem(TOKEN_KEY)
    }
  } catch {
    /* armazenamento indisponível: sessão só em memória */
  }
}

export class ApiError extends Error {
  status: number
  /**
   * Corpo JSON da resposta de erro (ou `null`). As telas decidem por ele:
   * `e.data?.codigo`, `e.data?.cota`, `e.data?.job`, `e.data?.estimativa`.
   */
  data: any
  /** Cabeçalho `Retry-After` em segundos, quando o servidor mandou. */
  retryAfter: number | null

  constructor(status: number, message: string, data: any = null, retryAfter: number | null = null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
    this.retryAfter = retryAfter
  }

  /** Código do erro da prospecção (`cota_esgotada`, `job_ativo`...), ou vazio. */
  get codigo(): string {
    return typeof this.data?.codigo === 'string' ? this.data.codigo : ''
  }
}

/** Opções por chamada: cancelamento e envio que sobrevive ao fechamento da aba. */
export interface RequestOptions {
  /** Cancela a chamada; o `fetch` rejeita com um `AbortError` (veja `foiAbortado`). */
  signal?: AbortSignal
  /** Para chamadas feitas no `pagehide` (soltar a trava da fila). */
  keepalive?: boolean
}

/** foiAbortado diz se o erro veio de um `AbortController.abort()`: não é erro para mostrar. */
export function foiAbortado(e: unknown): boolean {
  return (e as any)?.name === 'AbortError'
}

/** Raiz das rotas públicas (formulário e agendamento), com o prefixo do cliente. */
export function publicApiBase(): string {
  return apiBase().replace(/\/v1$/, '/public')
}

function lerRetryAfter(resp: Response): number | null {
  const bruto = resp.headers?.get?.('Retry-After')
  if (!bruto) return null
  const n = Number(bruto)
  return Number.isFinite(n) && n >= 0 ? n : null
}

let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(fn: () => void): void {
  onUnauthorized = fn
}

let onPasswordChangeRequired: (() => void) | null = null

/** Registra o que fazer quando a API exigir a troca da senha temporária. */
export function setPasswordChangeHandler(fn: () => void): void {
  onPasswordChangeRequired = fn
}

async function request<T>(method: string, path: string, body?: unknown, opts?: RequestOptions): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  let payload: BodyInit | undefined
  if (body instanceof FormData) {
    payload = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }

  const init: RequestInit = { method, headers, body: payload }
  if (opts?.signal) init.signal = opts.signal
  if (opts?.keepalive) init.keepalive = true

  const resp = await fetch(`${apiBase()}${path}`, init)

  if (resp.status === 401) {
    setToken(null)
    onUnauthorized?.()
    throw new ApiError(401, 'sessão expirada, faça login novamente')
  }

  const text = await resp.text()
  let data: any = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  if (resp.status === 403 && data?.must_change_password) {
    onPasswordChangeRequired?.()
  }

  if (!resp.ok) {
    let retryAfter = lerRetryAfter(resp)
    if (retryAfter === null && typeof data?.tentar_em_segundos === 'number') retryAfter = data.tentar_em_segundos
    throw new ApiError(resp.status, data?.error || `erro ${resp.status}`, data, retryAfter)
  }
  return data as T
}

/**
 * Baixa um arquivo de uma rota autenticada (exportações, anexos) e entrega ao
 * navegador com o nome dado. `<a href>` puro iria sem o Authorization e sem o
 * prefixo do cliente; aqui o caminho passa por `apiBase()` como todo o resto.
 */
async function download(path: string, nomeArquivo: string, opts?: RequestOptions): Promise<void> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const init: RequestInit = { method: 'GET', headers }
  if (opts?.signal) init.signal = opts.signal

  const resp = await fetch(`${apiBase()}${path}`, init)

  if (resp.status === 401) {
    setToken(null)
    onUnauthorized?.()
    throw new ApiError(401, 'sessão expirada, faça login novamente')
  }
  if (!resp.ok) {
    let data: any = null
    try {
      data = JSON.parse(await resp.text())
    } catch {
      data = null
    }
    throw new ApiError(resp.status, data?.error || 'não foi possível baixar o arquivo', data, lerRetryAfter(resp))
  }

  const blob = await resp.blob()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = nomeArquivo
  link.click()
  URL.revokeObjectURL(url)
}

export const api = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, undefined, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('POST', path, body, opts),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PUT', path, body, opts),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PATCH', path, body, opts),
  delete: <T>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, undefined, opts),
  download
}
