import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CompanyDetailView from '../src/views/CompanyDetailView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: { id: '1' } })
}))

const company = {
  id: 1,
  name: 'LUX 7',
  domain: 'lux7.com.br',
  phone: '',
  industry: 'iluminação',
  city: 'Fortaleza',
  state: 'CE',
  owner_id: null,
  owner_name: '',
  contacts_count: 1,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ec_number: '17240',
  economic_group: 'JMX SOLUCOES EM LED LTDA',
  cnpj: '20.436.891/0002-11',
  accredited_at: '2024-05-10T00:00:00Z',
  representative: 'GUILHERME MEDEIROS BECCO',
  instagram: '',
  products: ['Link de pagamento', 'Pix', 'Safe Link'],
  machines_count: 0,
  is_client: true,
  anticipation_mode: 'pontual',
  validator: true,
  do_not_disturb: false
}

const contact = {
  id: 9,
  first_name: 'Joao Paulo',
  last_name: 'Frota Maia',
  email: 'financeiro@jmxled.com.br',
  phone: '+558530169152',
  job_title: 'Financeiro',
  lifecycle_stage: 'cliente',
  source: '',
  company_id: 1,
  owner_id: null,
  created_at: '',
  updated_at: ''
}

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/companies/1')) {
      body = company
    } else if (u.includes('/contacts?')) {
      body = { data: [contact], pagination: { page: 1, per_page: 100, total: 1 } }
    } else if (u.includes('/deals') || u.includes('/tickets') || u.includes('/tasks') || u.includes('/meetings')) {
      body = { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
    } else if (u.includes('/activities')) {
      body = {
        data: [
          {
            id: 1,
            kind: 'sistema',
            content: 'Empresa criada',
            user_id: 1,
            user_name: 'Administrador',
            contact_id: null,
            company_id: 1,
            deal_id: null,
            created_at: '2026-06-11T16:07:00Z'
          }
        ]
      }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('CompanyDetailView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build() {
    return mount(CompanyDetailView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
  }

  it('mostra os campos de negócio da Fix Pay em "Sobre essa empresa"', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('LUX 7')
    expect(wrapper.text()).toContain('Número do EC')
    expect(wrapper.text()).toContain('17240')
    expect(wrapper.text()).toContain('20.436.891/0002-11')
    expect(wrapper.text()).toContain('JMX SOLUCOES EM LED LTDA')
    expect(wrapper.text()).toContain('Link de pagamento')
    expect(wrapper.text()).toContain('Safe Link')
    expect(wrapper.text()).toContain('Modalidade de antecipação')
    wrapper.unmount()
  })

  it('ações rápidas incluem observação, e-mail e tarefa', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    const actions = wrapper.findAll('.quick-actions button').map((b) => b.text())
    expect(actions.join(' ')).toContain('Observ.')
    expect(actions.join(' ')).toContain('E-mail')
    expect(actions.join(' ')).toContain('Tarefa')

    // Com contato com e-mail, o botão E-mail fica habilitado.
    const emailBtn = wrapper.findAll('.quick-actions button')[1]
    expect(emailBtn.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('aba de atividades tem sub-abas e agrupa por mês', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    await wrapper.findAll('.center-tabs > button')[1].trigger('click')
    await flushPromises()

    const subTabs = wrapper.findAll('.sub-tabs button').map((b) => b.text())
    expect(subTabs).toEqual(['Todas as atividades', 'Observações', 'E-mails', 'Chamadas', 'Tarefas', 'Reuniões'])
    expect(wrapper.text()).toContain('Junho 2026')
    expect(wrapper.text()).toContain('Empresa criada')
    wrapper.unmount()
  })

  it('coluna direita mostra o contato com e-mail e telefone', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Contatos (1)')
    expect(wrapper.text()).toContain('Joao Paulo Frota Maia')
    expect(wrapper.find('a[href="mailto:financeiro@jmxled.com.br"]').exists()).toBe(true)
    expect(wrapper.find('a[href="tel:+558530169152"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
