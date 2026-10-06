import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ImportsView from '../src/views/ImportsView.vue'

const history = [
  {
    id: 1,
    file_name: 'Carga 458 Sim.xlsx',
    entity: 'contatos',
    status: 'concluida',
    total_rows: 8746,
    new_records: 3,
    updated_records: 8742,
    new_associations: 4,
    error_count: 1,
    errors: ['linha 12: sem e-mail'],
    created_by: 1,
    created_by_name: 'Armando Couto',
    created_at: new Date().toISOString()
  }
]

function stubFetch(routes: Record<string, unknown>) {
  const calls: { url: string; init?: RequestInit }[] = []
  const fn = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    calls.push({ url: String(url), init })
    const match = Object.entries(routes).find(([path]) => String(url).startsWith(path))
    return Promise.resolve({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(match ? match[1] : {}))
    })
  })
  return { fn, calls }
}

describe('ImportsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('mostra o histórico com novos, atualizados, associações e erros', async () => {
    vi.stubGlobal('fetch', stubFetch({ '/api/v1/imports': { data: history } }).fn)

    const wrapper = mount(ImportsView)
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('Carga 458 Sim.xlsx')
    expect(text).toContain('Concluída')
    expect(text.replace(/ /g, ' ')).toContain('8.742')
    expect(text).toContain('Armando Couto')

    // Clicar no número de erros abre o detalhe.
    await wrapper.find('.link-btn').trigger('click')
    expect(wrapper.text()).toContain('linha 12: sem e-mail')
    wrapper.unmount()
  })

  it('envia o arquivo como multipart com a entidade escolhida', async () => {
    const calls: { url: string; init?: RequestInit }[] = []
    const fn = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url: String(url), init })
      const payload =
        init?.method === 'POST'
          ? { ...history[0], id: 2, file_name: 'contatos.csv', errors: [] }
          : { data: [] }
      return Promise.resolve({
        ok: true,
        status: 200,
        text: () => Promise.resolve(JSON.stringify(payload))
      })
    })
    vi.stubGlobal('fetch', fn)

    const wrapper = mount(ImportsView)
    await flushPromises()

    // Injeta o arquivo escolhido direto no estado interno do componente
    // (input[type=file] não aceita setValue no jsdom).
    const file = new File(['Nome;E-mail\nAna;ana@ex.com.br'], 'contatos.csv', {
      type: 'text/csv'
    })
    ;(wrapper.vm as any).file = file
    await wrapper.vm.$nextTick()

    await wrapper.find('.btn-primary').trigger('click')
    await flushPromises()

    const post = calls.find((c) => c.init?.method === 'POST')
    expect(post).toBeTruthy()
    const body = post!.init!.body as FormData
    expect(body).toBeInstanceOf(FormData)
    expect(body.get('entity')).toBe('contatos')
    expect((body.get('file') as File).name).toBe('contatos.csv')
    wrapper.unmount()
  })
})
