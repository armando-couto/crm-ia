import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InboxView from '../src/views/InboxView.vue'

const conversations = [
  {
    id: 1,
    subject: 'Proposta de adquirência',
    contact_id: 1,
    contact_name: 'Carlos Lima',
    peer_email: 'carlos@bompreco.com.br',
    status: 'aberta',
    unread: true,
    last_message_at: new Date().toISOString(),
    last_preview: 'Podemos fechar?',
    created_at: new Date().toISOString()
  }
]

function stubFetch(routes: Record<string, unknown>) {
  return vi.fn().mockImplementation((url: string) => {
    const match = Object.entries(routes).find(([path]) => String(url).startsWith(path))
    return Promise.resolve({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(match ? match[1] : {}))
    })
  })
}

describe('InboxView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('lista conversas com contador de não lidas', async () => {
    vi.stubGlobal('fetch', stubFetch({ '/api/v1/conversations': { data: conversations, unread: 1 } }))

    const wrapper = mount(InboxView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Carlos Lima')
    expect(wrapper.text()).toContain('Proposta de adquirência')
    expect(wrapper.find('.conv-item.unread').exists()).toBe(true)
    wrapper.unmount()
  })

  it('abre o thread ao clicar na conversa', async () => {
    vi.stubGlobal(
      'fetch',
      stubFetch({
        '/api/v1/conversations/1': {
          conversation: { ...conversations[0], unread: false },
          messages: [
            {
              id: 10,
              conversation_id: 1,
              direction: 'recebida',
              from_email: 'carlos@bompreco.com.br',
              to_email: 'vendas@fixpay.com.br',
              subject: 'Proposta',
              body: 'Podemos fechar com a taxa combinada?',
              user_id: null,
              created_at: new Date().toISOString()
            }
          ]
        },
        '/api/v1/conversations': { data: conversations, unread: 1 }
      })
    )

    const wrapper = mount(InboxView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } }
    })
    await flushPromises()

    await wrapper.find('.conv-item').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Podemos fechar com a taxa combinada?')
    expect(wrapper.find('.reply-box textarea').exists()).toBe(true)
    wrapper.unmount()
  })
})
