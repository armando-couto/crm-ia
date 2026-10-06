import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ContactDetailView from '../src/views/ContactDetailView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: { id: '1' } })
}))

const contact = {
  id: 1,
  first_name: 'Vilmar',
  last_name: 'Landulfo',
  email: 'alessandra@h2i.com.br',
  phone: '+55-71-3023-7752',
  job_title: '',
  lifecycle_stage: 'lead',
  source: 'Integração',
  company_id: 2,
  company_name: 'H2i Informatica',
  owner_id: null,
  owner_name: '',
  last_activity_at: null,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString()
}

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/contacts/1')) {
      body = contact
    } else if (u.includes('/deals')) {
      body = {
        data: [
          {
            id: 3,
            name: 'Adquirência H2i',
            amount: 12000,
            status: 'aberto',
            stage_name: 'Proposta Enviada',
            contact_id: 1,
            company_id: 2,
            pipeline_id: 1,
            stage_id: 3,
            currency: 'BRL',
            close_date: null,
            position: 0,
            closed_at: null,
            owner_id: 1,
            created_at: '',
            updated_at: ''
          }
        ],
        pagination: { page: 1, per_page: 50, total: 1 }
      }
    } else if (u.includes('/tasks')) {
      body = { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
    } else if (u.includes('/tickets')) {
      body = { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
    } else if (u.includes('/activities')) {
      body = { data: [] }
    } else if (u.includes('/companies')) {
      body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('ContactDetailView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build() {
    return mount(ContactDetailView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
  }

  it('mostra o perfil com ações rápidas e a seção sobre o contato', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Vilmar Landulfo')
    expect(wrapper.text()).toContain('H2i Informatica')
    expect(wrapper.text()).toContain('Sobre esse contato')
    expect(wrapper.text()).toContain('Nenhum proprietário')
    expect(wrapper.text()).toContain('Fonte do registro')

    // Ações rápidas: Nota, E-mail, Chamada, Tarefa, Reunião
    const actions = wrapper.findAll('.quick-actions button').map((b) => b.text())
    expect(actions.join(' ')).toContain('Nota')
    expect(actions.join(' ')).toContain('Chamada')
    expect(actions.join(' ')).toContain('Reunião')
    wrapper.unmount()
  })

  it('mostra as associações da coluna direita', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Empresas (1)')
    expect(wrapper.text()).toContain('Principal')
    expect(wrapper.text()).toContain('Negócios (1)')
    expect(wrapper.text()).toContain('Adquirência H2i')
    expect(wrapper.text()).toContain('Tickets (0)')
    wrapper.unmount()
  })

  it('estado vazio da visão geral convida a criar atividade', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhuma atividade neste registro.')
    expect(wrapper.text()).toContain('Criar atividade')
    wrapper.unmount()
  })

  it('alterna para a aba de atividades', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const tabs = wrapper.findAll('.center-tabs button')
    await tabs[1].trigger('click')
    await flushPromises()

    expect(tabs[1].classes()).toContain('active')
    expect(wrapper.find('.timeline').exists()).toBe(true)
    wrapper.unmount()
  })
})
