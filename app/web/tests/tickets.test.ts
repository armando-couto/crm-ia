import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import TicketsView from '../src/views/TicketsView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {} })
}))

const tickets = [
  {
    id: 1,
    subject: 'Maquininha não liga',
    description: '',
    status: 'aberto',
    priority: 'alta',
    contact_id: 1,
    contact_name: 'Carlos Lima',
    company_id: null,
    owner_id: 1,
    owner_name: 'Administrador',
    closed_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString()
  }
]

describe('TicketsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('lista tickets com status e prioridade', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        const body = String(url).includes('/tickets')
          ? { data: tickets, pagination: { page: 1, per_page: 25, total: 1 } }
          : []
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
      })
    )

    const wrapper = mount(TicketsView)
    await flushPromises()

    expect(wrapper.text()).toContain('Maquininha não liga')
    expect(wrapper.text()).toContain('Aberto')
    expect(wrapper.text()).toContain('alta')
    expect(wrapper.text()).toContain('Carlos Lima')
    wrapper.unmount()
  })

  it('mostra estado vazio sem tickets', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        text: () => Promise.resolve(JSON.stringify({ data: [], pagination: { page: 1, per_page: 25, total: 0 } }))
      })
    )

    const wrapper = mount(TicketsView)
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum ticket aqui')
    wrapper.unmount()
  })
})
