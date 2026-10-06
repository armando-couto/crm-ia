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

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
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

  const resp = await fetch(`${apiBase()}${path}`, { method, headers, body: payload })

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
    throw new ApiError(resp.status, data?.error || `erro ${resp.status}`)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path)
}
