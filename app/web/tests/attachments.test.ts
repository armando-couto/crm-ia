import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import AttachmentsPanel from '../src/components/AttachmentsPanel.vue'

const files = [
  {
    id: 1,
    entity: 'contato',
    entity_id: 7,
    filename: 'proposta.pdf',
    content_type: 'application/pdf',
    size_bytes: 2048,
    uploaded_by: 1,
    uploader_name: 'Ana',
    created_at: new Date().toISOString()
  }
]

function stubFetch(list = files) {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    if (options?.method === 'POST') {
      return Promise.resolve({ ok: true, status: 201, text: () => Promise.resolve(JSON.stringify(list[0])) })
    }
    if (options?.method === 'DELETE') {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{}') })
    }
    if (String(url).includes('/attachments')) {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(list)) })
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('[]') })
  })
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'T', email: 't@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
  return auth
}

describe('AttachmentsPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
  })

  it('lista os arquivos do registro com tamanho e autor', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AttachmentsPanel, { props: { entity: 'contato', entityId: 7 } })
    await flushPromises()

    expect(wrapper.text()).toContain('proposta.pdf')
    expect(wrapper.text()).toContain('2 KB')
    expect(wrapper.text()).toContain('Ana')
  })

  it('busca apenas os anexos daquele registro', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    mount(AttachmentsPanel, { props: { entity: 'negocio', entityId: 42 } })
    await flushPromises()

    const url = String(fetchMock.mock.calls[0][0])
    expect(url).toContain('entity=negocio')
    expect(url).toContain('entity_id=42')
  })

  it('esconde envio e remoção de quem não tem files.manage', async () => {
    loginAs('seller', { 'files.view': true })
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AttachmentsPanel, { props: { entity: 'contato', entityId: 7 } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('Anexar arquivo')
    expect(wrapper.find('.dropzone').exists()).toBe(false)
    expect(wrapper.find('.remove').exists()).toBe(false)
  })

  it('mostra estado vazio quando não há arquivos', async () => {
    vi.stubGlobal('fetch', stubFetch([]))
    const wrapper = mount(AttachmentsPanel, { props: { entity: 'ticket', entityId: 1 } })
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum arquivo anexado')
  })

  it('envia o arquivo como multipart com a entidade do registro', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(AttachmentsPanel, { props: { entity: 'empresa', entityId: 9 } })
    await flushPromises()

    const file = new File(['conteudo'], 'contrato.pdf', { type: 'application/pdf' })
    await (wrapper.vm as any).upload([file] as unknown as FileList)
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(post).toBeTruthy()
    const body = post![1].body as FormData
    expect(body.get('entity')).toBe('empresa')
    expect(body.get('entity_id')).toBe('9')
    expect((body.get('file') as File).name).toBe('contrato.pdf')
  })
})
