import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, getToken, setToken, setUnauthorizedHandler } from '../src/api'

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body))
  })
}

describe('api client', () => {
  beforeEach(() => {
    setToken(null)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    setToken(null)
  })

  it('armazena e recupera o token', () => {
    setToken('abc123')
    expect(getToken()).toBe('abc123')
    setToken(null)
    expect(getToken()).toBeNull()
  })

  it('envia o Authorization quando há token', async () => {
    setToken('meu-token')
    const fetchMock = mockFetch(200, { ok: true })
    vi.stubGlobal('fetch', fetchMock)

    await api.get('/contacts')

    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/contacts')
    expect(options.headers['Authorization']).toBe('Bearer meu-token')
  })

  it('serializa JSON no POST', async () => {
    const fetchMock = mockFetch(201, { id: 1 })
    vi.stubGlobal('fetch', fetchMock)

    const resp = await api.post<{ id: number }>('/contacts', { first_name: 'Ana' })

    const [, options] = fetchMock.mock.calls[0]
    expect(options.method).toBe('POST')
    expect(options.headers['Content-Type']).toBe('application/json')
    expect(JSON.parse(options.body)).toEqual({ first_name: 'Ana' })
    expect(resp.id).toBe(1)
  })

  it('lança ApiError com a mensagem do backend', async () => {
    vi.stubGlobal('fetch', mockFetch(400, { error: 'informe o nome' }))

    await expect(api.post('/contacts', {})).rejects.toThrowError('informe o nome')
    await expect(api.post('/contacts', {})).rejects.toBeInstanceOf(ApiError)
  })

  it('limpa o token e chama o handler em 401', async () => {
    setToken('expirado')
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    vi.stubGlobal('fetch', mockFetch(401, { error: 'não autenticado' }))

    await expect(api.get('/me')).rejects.toThrowError()
    expect(getToken()).toBeNull()
    expect(handler).toHaveBeenCalledOnce()
  })
})
