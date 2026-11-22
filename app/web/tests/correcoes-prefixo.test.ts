// Correções da seção 9 do contrato da prospecção: chamadas que ignoravam o
// prefixo do cliente (/<slug>) e telas que liam .data de rotas que devolvem
// array. Os testes rodam com um cliente em /acme para o prefixo aparecer.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { api, ApiError, publicApiBase, setToken } from '../src/api'
import { tenant } from '../src/tenant'
import { useAuthStore } from '../src/stores/auth'
import AttachmentsPanel from '../src/components/AttachmentsPanel.vue'
import ContactsView from '../src/views/ContactsView.vue'
import DealsBoardView from '../src/views/DealsBoardView.vue'
import PublicFormView from '../src/views/PublicFormView.vue'
import PublicBookingView from '../src/views/PublicBookingView.vue'
import SettingsFormsView from '../src/views/SettingsFormsView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ params: { slug: 'fale-conosco' }, query: {} })
}))

const stubs = { 'router-link': { template: '<a><slot /></a>' }, Teleport: true }

function resposta(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(body)),
    json: () => Promise.resolve(body),
    blob: () => Promise.resolve(new Blob(['a;b']))
  }
}

function urls(fetchMock: any): string[] {
  return fetchMock.mock.calls.map((c: any[]) => String(c[0]))
}

describe('prefixo do cliente nas chamadas diretas', () => {
  let clique: ReturnType<typeof vi.fn>

  beforeEach(() => {
    setActivePinia(createPinia())
    tenant.base_path = '/acme'
    setToken('tok')
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
    clique = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(clique)
    ;(URL as any).createObjectURL = vi.fn(() => 'blob:x')
    ;(URL as any).revokeObjectURL = vi.fn()
  })

  afterEach(() => {
    tenant.base_path = ''
    setToken(null)
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('publicApiBase acompanha o prefixo', () => {
    expect(publicApiBase()).toBe('/acme/api/public')
    tenant.base_path = ''
    expect(publicApiBase()).toBe('/api/public')
  })

  it('api.download usa apiBase(), manda o token e entrega o arquivo com o nome pedido', async () => {
    const fetchMock = vi.fn().mockResolvedValue(resposta({}))
    vi.stubGlobal('fetch', fetchMock)
    let nome = ''
    clique.mockImplementation(function (this: HTMLAnchorElement) {
      nome = this.download
    })

    await api.download('/prospeccao/listas/7/exportar', 'lista-7.csv')

    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toBe('/acme/api/v1/prospeccao/listas/7/exportar')
    expect(options.headers['Authorization']).toBe('Bearer tok')
    expect(clique).toHaveBeenCalledTimes(1)
    expect(nome).toBe('lista-7.csv')
    expect((URL as any).revokeObjectURL).toHaveBeenCalledWith('blob:x')
  })

  it('api.download devolve ApiError com o corpo quando a rota recusa', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(resposta({ error: 'sem permissão', codigo: 'x' }, 403)))

    const erro = await api.download('/contacts/export', 'c.csv').catch((e) => e)

    expect(erro).toBeInstanceOf(ApiError)
    expect(erro.status).toBe(403)
    expect(erro.message).toBe('sem permissão')
    expect(erro.data).toEqual({ error: 'sem permissão', codigo: 'x' })
    expect(clique).not.toHaveBeenCalled()
  })

  it('download de anexo passa pelo prefixo', async () => {
    const arquivo = { id: 5, entity: 'contato', entity_id: 7, filename: 'proposta.pdf', content_type: 'application/pdf', size_bytes: 10, uploaded_by: 1, uploader_name: 'Ana', created_at: new Date().toISOString() }
    const fetchMock = vi.fn().mockResolvedValue(resposta([arquivo]))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(AttachmentsPanel, { props: { entity: 'contato', entityId: 7 } })
    await flushPromises()

    await wrapper.find('button.name').trigger('click')
    await flushPromises()

    expect(urls(fetchMock)).toContain('/acme/api/v1/attachments/5/download')
    expect(urls(fetchMock).every((u) => u.startsWith('/acme/'))).toBe(true)
    expect(clique).toHaveBeenCalled()
  })

  it('exportação de contatos passa pelo prefixo', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      const u = String(url)
      let body: unknown = []
      if (u.includes('/contacts/stats')) body = { total: 0, sem_dono: 0, sem_email: 0, leads_sem_avanco: 0, sem_atividade_30d: 0 }
      else if (u.includes('/contacts?')) body = { data: [], pagination: { page: 1, per_page: 25, total: 0 } }
      return Promise.resolve(resposta(body))
    })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(ContactsView, { global: { stubs } })
    await flushPromises()

    const botao = wrapper.findAll('button').find((b) => b.text() === 'Exportar')
    await botao!.trigger('click')
    await flushPromises()

    const exportacao = urls(fetchMock).find((u) => u.includes('/contacts/export'))
    expect(exportacao).toMatch(/^\/acme\/api\/v1\/contacts\/export\?/)
    expect(clique).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('exportação de negócios passa pelo prefixo', async () => {
    const pipelines = [{ id: 1, name: 'Vendas', is_default: true, position: 0, stages: [{ id: 10, pipeline_id: 1, name: 'Novo', position: 0, probability: 10, is_won: false, is_lost: false }] }]
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      const u = String(url)
      let body: unknown = []
      if (u.includes('/pipelines')) body = pipelines
      else if (u.includes('/deals/board')) body = { data: [] }
      else if (u.includes('/contacts?') || u.includes('/companies?')) body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
      return Promise.resolve(resposta(body))
    })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(DealsBoardView, { global: { stubs } })
    await flushPromises()

    const botao = wrapper.findAll('button').find((b) => b.text() === 'Exportar')
    await botao!.trigger('click')
    await flushPromises()

    const exportacao = urls(fetchMock).find((u) => u.includes('/deals/export'))
    expect(exportacao).toMatch(/^\/acme\/api\/v1\/deals\/export\?pipeline_id=1/)
    wrapper.unmount()
  })

  it('formulário público lê e envia pelo prefixo', async () => {
    const definicao = { slug: 'fale-conosco', title: 'Fale', description: '', submit_label: 'Enviar', fields: [{ key: 'first_name', label: 'Nome', type: 'texto', required: true }] }
    const fetchMock = vi.fn().mockImplementation((_u: string, o?: any) =>
      Promise.resolve(resposta(o?.method === 'POST' ? { message: 'ok' } : definicao))
    )
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(PublicFormView)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(urls(fetchMock)).toEqual(['/acme/api/public/forms/fale-conosco', '/acme/api/public/forms/fale-conosco'])
    expect(fetchMock.mock.calls[1][1].method).toBe('POST')
  })

  it('agendamento público lê e envia pelo prefixo', async () => {
    const definicao = { slug: 'ana', title: 'Reunião', description: '', duration_min: 30, owner_name: 'Ana', days: [{ date: '2026-12-01', slots: ['09:00', '09:30'] }] }
    const fetchMock = vi.fn().mockImplementation((_u: string, o?: any) =>
      Promise.resolve(resposta(o?.method === 'POST' ? { message: 'agendado' } : definicao))
    )
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    expect(urls(fetchMock)[0]).toBe('/acme/api/public/booking/fale-conosco')

    await wrapper.findAll('.slot')[0].trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(String(post![0])).toBe('/acme/api/public/booking/ana')
  })
})

describe('SettingsFormsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('carrega as listas da rota que devolve array puro', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        const u = String(url)
        let body: unknown = []
        if (u.includes('/forms')) body = { data: [], default_fields: [], embed_base: 'https://crm.exemplo.com.br' }
        else if (u.includes('/users')) body = [{ id: 1, name: 'Ana' }]
        else if (u.includes('/lists')) body = [{ id: 3, name: 'Clientes', kind: 'estatica' }]
        return Promise.resolve(resposta(body))
      })
    )
    const wrapper = mount(SettingsFormsView, { global: { stubs } })
    await flushPromises()

    expect((wrapper.vm as any).lists.map((l: any) => l.name)).toEqual(['Clientes'])
    wrapper.unmount()
  })
})
