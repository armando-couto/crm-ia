import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InboxView from '../src/views/InboxView.vue'
import { useAuthStore } from '../src/stores/auth'

const counters = { unassigned: 42, mine: 9, open: 118, closed: 3 }

const conversations = [
  {
    id: 1,
    subject: 'Proposta de adquirência',
    contact_id: 1,
    contact_name: 'Carlos Lima',
    peer_email: 'carlos@bompreco.com.br',
    status: 'aberta',
    unread: true,
    owner_id: null,
    last_message_at: new Date().toISOString(),
    last_preview: 'Podemos fechar?',
    created_at: new Date().toISOString()
  }
]

const contact = {
  id: 1,
  first_name: 'Carlos',
  last_name: 'Lima',
  email: 'carlos@bompreco.com.br',
  phone: '11 99999-0000',
  job_title: '',
  lifecycle_stage: 'lead',
  source: 'importacao',
  company_id: 2,
  company_name: 'Bom Preço',
  owner_id: null,
  buying_role: '',
  created_at: '',
  updated_at: ''
}

const threadRoutes = {
  '/api/v1/conversations/1': {
    conversation: { ...conversations[0], unread: false },
    messages: [
      {
        id: 10,
        conversation_id: 1,
        direction: 'recebida',
        from_email: 'carlos@bompreco.com.br',
        to_email: 'vendas@exemplo.com.br',
        subject: 'Proposta',
        body: 'Podemos fechar com a taxa combinada?',
        user_id: null,
        created_at: new Date().toISOString()
      },
      {
        id: 11,
        conversation_id: 1,
        direction: 'comentario',
        from_email: '',
        to_email: '',
        subject: 'Proposta',
        body: 'Nota interna: validar taxa com o financeiro.',
        user_id: 2,
        user_name: 'Bia',
        created_at: new Date().toISOString()
      }
    ]
  },
  '/api/v1/conversations': { data: conversations, unread: 1, counters },
  '/api/v1/contacts/1': contact,
  '/api/v1/users': [
    { id: 1, name: 'Admin' },
    { id: 2, name: 'Bia' }
  ]
}

function stubFetch(routes: Record<string, unknown>) {
  const calls: { url: string; init?: RequestInit }[] = []
  const fn = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    calls.push({ url: String(url), init })
    const match = Object.entries(routes)
      .sort((a, b) => b[0].length - a[0].length)
      .find(([path]) => String(url).startsWith(path))
    return Promise.resolve({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(match ? match[1] : {}))
    })
  })
  return { fn, calls }
}

function mountInbox() {
  return mount(InboxView, {
    global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } }
  })
}

function loginAdmin() {
  const auth = useAuthStore()
  auth.user = {
    id: 1,
    name: 'Admin',
    email: 'admin@exemplo.com.br',
    role: 'admin',
    active: true,
    created_at: '',
    updated_at: ''
  } as any
}

describe('InboxView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAdmin()
  })

  it('mostra as filas com contadores', async () => {
    vi.stubGlobal('fetch', stubFetch(threadRoutes).fn)

    const wrapper = mountInbox()
    await flushPromises()

    const buttons = wrapper.findAll('.queues button')
    const byLabel = (label: string) => buttons.find((b) => b.text().includes(label))
    expect(byLabel('Não atribuído')?.text()).toContain('42')
    expect(byLabel('Atribuído a mim')?.text()).toContain('9')
    expect(byLabel('Tudo aberto')?.text()).toContain('118')
    expect(byLabel('Fechadas')?.text()).toContain('3')
    expect(wrapper.text()).toContain('Carlos Lima')
    wrapper.unmount()
  })

  it('a fila "minhas" refaz a busca com o parâmetro queue', async () => {
    const { fn, calls } = stubFetch(threadRoutes)
    vi.stubGlobal('fetch', fn)

    const wrapper = mountInbox()
    await flushPromises()

    const mine = wrapper.findAll('.queues button').find((b) => b.text().includes('Atribuído a mim'))
    await mine!.trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.url.includes('queue=minhas'))).toBe(true)
    wrapper.unmount()
  })

  it('abre o thread com dono, comentário interno destacado e ficha do contato', async () => {
    vi.stubGlobal('fetch', stubFetch(threadRoutes).fn)

    const wrapper = mountInbox()
    await flushPromises()

    await wrapper.find('.conv').trigger('click')
    await flushPromises()

    // Thread e composer.
    expect(wrapper.text()).toContain('Podemos fechar com a taxa combinada?')
    expect(wrapper.find('.owner-row select').exists()).toBe(true)
    expect(wrapper.text()).toContain('Proprietário')

    // Comentário interno vem destacado com o autor.
    const note = wrapper.find('.msg.note')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('comentário interno')
    expect(note.text()).toContain('Bia')

    // Ficha do contato no painel direito.
    expect(wrapper.text()).toContain('Sobre esse contato')
    expect(wrapper.text()).toContain('carlos@bompreco.com.br')
    expect(wrapper.text()).toContain('Bom Preço')
    wrapper.unmount()
  })

  it('registra comentário interno sem enviar resposta', async () => {
    const { fn, calls } = stubFetch({
      ...threadRoutes,
      '/api/v1/conversations/1/comments': {
        id: 12,
        conversation_id: 1,
        direction: 'comentario',
        body: 'Aprovem o desconto.',
        user_id: 1,
        user_name: 'Admin',
        created_at: new Date().toISOString()
      }
    })
    vi.stubGlobal('fetch', fn)

    const wrapper = mountInbox()
    await flushPromises()
    await wrapper.find('.conv').trigger('click')
    await flushPromises()

    const tabs = wrapper.findAll('.composer-tabs button')
    await tabs.find((b) => b.text().includes('Comentário'))!.trigger('click')
    await wrapper.find('.composer textarea').setValue('Aprovem o desconto.')
    await wrapper.find('.composer-actions .btn-primary').trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.url.includes('/conversations/1/comments'))).toBe(true)
    expect(calls.some((c) => c.url.includes('/conversations/1/reply'))).toBe(false)
    expect(wrapper.text()).toContain('Aprovem o desconto.')
    wrapper.unmount()
  })

  it('atribui a conversa pelo dropdown de proprietário', async () => {
    const { fn, calls } = stubFetch({
      ...threadRoutes,
      '/api/v1/conversations/1/owner': { ...conversations[0], owner_id: 2, owner_name: 'Bia' }
    })
    vi.stubGlobal('fetch', fn)

    const wrapper = mountInbox()
    await flushPromises()
    await wrapper.find('.conv').trigger('click')
    await flushPromises()

    await wrapper.find('.owner-row select').setValue('2')
    await flushPromises()

    const patch = calls.find((c) => c.url.includes('/conversations/1/owner'))
    expect(patch).toBeTruthy()
    expect(JSON.parse(String(patch!.init?.body))).toEqual({ owner_id: 2 })
    wrapper.unmount()
  })
})
