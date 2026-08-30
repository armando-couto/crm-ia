import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import NotificationsView from '../src/views/NotificationsView.vue'

describe('NotificationsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function stubFetch() {
    return vi.fn().mockImplementation((url: string, options?: any) => {
      let body: unknown = { negocio_atribuido: true, tarefa_atribuida: false, ticket_atribuido: true }
      if (options?.method === 'PUT') {
        body = JSON.parse(options.body)
      }
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
    })
  }

  it('carrega as preferências e reflete nos toggles', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(NotificationsView)
    await flushPromises()

    const toggles = wrapper.findAll('.switch input')
    expect((toggles[0].element as HTMLInputElement).checked).toBe(true) // negócio
    expect((toggles[1].element as HTMLInputElement).checked).toBe(false) // tarefa
    expect((toggles[2].element as HTMLInputElement).checked).toBe(true) // ticket
    expect(wrapper.text()).toContain('Negócio atribuído a você')
    wrapper.unmount()
  })

  it('salvar envia as preferências via PUT /me/notifications', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(NotificationsView)
    await flushPromises()

    await wrapper.findAll('.switch input')[1].setValue(true) // liga tarefa
    await wrapper.find('.btn-primary').trigger('click')
    await flushPromises()

    const putCall = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'PUT')
    expect(putCall).toBeTruthy()
    expect(String(putCall![0])).toContain('/me/notifications')
    expect(JSON.parse(putCall![1].body).tarefa_atribuida).toBe(true)
    wrapper.unmount()
  })
})
