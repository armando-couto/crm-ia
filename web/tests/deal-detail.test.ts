import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import DealDetailView from '../src/views/DealDetailView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: { id: '1' } })
}))

const deal = {
  id: 1,
  name: 'RICARDO SAMPAIO [CONSTRUFORT] - Closer',
  amount: 100000,
  currency: 'BRL',
  pipeline_id: 1,
  stage_id: 2,
  stage_name: 'Negociação',
  contact_id: 5,
  contact_name: 'Ricardo Sampaio',
  company_id: 3,
  company_name: 'Construfort',
  owner_id: 1,
  owner_name: 'Luiz Rocha',
  status: 'aberto',
  temperature: 'quente',
  close_date: null,
  position: 0,
  closed_at: null,
  last_activity_at: new Date().toISOString(),
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString()
}

const contact = {
  id: 5,
  first_name: 'Ricardo',
  last_name: 'Sampaio',
  email: 'ricardo@construfort.com.br',
  phone: '+5585999990000',
  job_title: 'Sócio',
  lifecycle_stage: 'oportunidade',
  source: '',
  company_id: 3,
  owner_id: 1,
  created_at: '',
  updated_at: ''
}

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/deals/1')) {
      body = deal
    } else if (u.includes('/contacts/5')) {
      body = contact
    } else if (u.includes('/tasks')) {
      body = {
        data: [
          {
            id: 7,
            title: 'Enviar proposta revisada',
            description: '',
            type: 'tarefa',
            priority: 'alta',
            due_date: new Date().toISOString(),
            completed_at: null,
            owner_id: 1,
            contact_id: 5,
            company_id: null,
            deal_id: 1,
            created_at: '',
            updated_at: ''
          }
        ],
        pagination: { page: 1, per_page: 50, total: 1 }
      }
    } else if (u.includes('/meetings')) {
      body = { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
    } else if (u.includes('/activities')) {
      body = {
        data: [
          {
            id: 1,
            kind: 'nota',
            content: 'AGENDAMENTO - Construfort',
            user_id: 1,
            user_name: 'Shayanna',
            contact_id: 5,
            company_id: null,
            deal_id: 1,
            created_at: '2026-07-10T10:00:00Z'
          }
        ]
      }
    } else if (u.includes('/pipelines')) {
      body = [
        {
          id: 1,
          name: 'Pipeline de Vendas',
          position: 0,
          stages: [
            { id: 1, pipeline_id: 1, name: 'Novo Lead', position: 0, probability: 10, is_won: false, is_lost: false },
            { id: 2, pipeline_id: 1, name: 'Negociação', position: 1, probability: 80, is_won: false, is_lost: false }
          ]
        }
      ]
    } else if (u.includes('/contacts?') || u.includes('/companies?')) {
      body = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('DealDetailView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build() {
    return mount(DealDetailView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
  }

  it('mostra o perfil do negócio com valor, temperatura e propriedades', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('RICARDO SAMPAIO [CONSTRUFORT] - Closer')
    expect(wrapper.text().replace(/\u00a0/g, ' ')).toContain('R$ 100.000,00')
    expect(wrapper.text()).toContain('🔴 Quente')
    expect(wrapper.text()).toContain('Sobre esse negócio')
    expect(wrapper.text()).toContain('Pipeline de Vendas')
    expect(wrapper.text()).toContain('Luiz Rocha')
    wrapper.unmount()
  })

  it('aba de atividades tem sub-abas com timeline agrupada por mês', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    await wrapper.findAll('.center-tabs > button')[1].trigger('click')
    await flushPromises()

    const subTabs = wrapper.findAll('.sub-tabs button').map((b) => b.text())
    expect(subTabs).toEqual(['Todas as atividades', 'Observações', 'E-mails', 'Chamadas', 'Tarefas', 'Reuniões'])
    expect(wrapper.text()).toContain('Julho 2026')
    expect(wrapper.text()).toContain('AGENDAMENTO - Construfort')
    expect(wrapper.text()).toContain('por Shayanna')
    wrapper.unmount()
  })

  it('coluna direita mostra contato, empresa principal e tarefas', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Contatos (1)')
    expect(wrapper.text()).toContain('Ricardo Sampaio')
    expect(wrapper.find('a[href="mailto:ricardo@construfort.com.br"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Empresas (1)')
    expect(wrapper.text()).toContain('Principal')
    expect(wrapper.text()).toContain('Tarefas (1)')
    expect(wrapper.text()).toContain('Enviar proposta revisada')
    wrapper.unmount()
  })

  it('ações rápidas de e-mail e chamada habilitadas quando há contato', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const buttons = wrapper.findAll('.quick-actions button')
    const emailBtn = buttons[1]
    const callBtn = buttons[2]
    expect(emailBtn.attributes('disabled')).toBeUndefined()
    expect(callBtn.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
})
