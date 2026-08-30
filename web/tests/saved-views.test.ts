import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CompaniesView from '../src/views/CompaniesView.vue'
import ContactsView from '../src/views/ContactsView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {} })
}))

const companyViews = [
  {
    id: 1,
    entity: 'companies',
    name: 'Clientes ativos',
    filters: { criado_dias: 90, sort: 'created_at', dir: 'desc' },
    position: 0,
    created_by: 1,
    created_at: ''
  },
  { id: 2, entity: 'companies', name: 'Churn', filters: { sem_dono: true }, position: 1, created_by: 1, created_at: '' }
]

const contactFormFields = [
  { key: 'email', visible: true, required: false },
  { key: 'first_name', visible: true, required: true },
  { key: 'phone', visible: false, required: false },
  { key: 'lifecycle_stage', visible: true, required: false },
  { key: 'last_name', visible: true, required: false },
  { key: 'owner_id', visible: false, required: false },
  { key: 'job_title', visible: false, required: false },
  { key: 'source', visible: false, required: false },
  { key: 'company_id', visible: false, required: false }
]

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/views?entity=companies')) {
      body = companyViews
    } else if (u.includes('/views?entity=contacts')) {
      body = []
    } else if (u.includes('/settings/contact-form')) {
      body = { fields: contactFormFields }
    } else if (u.includes('/contacts/stats')) {
      body = { total: 0, sem_dono: 0, sem_email: 0, leads_sem_avanco: 0, sem_atividade_30d: 0 }
    } else if (u.includes('/companies?') || u.includes('/contacts?')) {
      body = { data: [], pagination: { page: 1, per_page: 25, total: 0 } }
    } else if (u.includes('/lists')) {
      body = []
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('Visualizações salvas em Empresas', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renderiza as visualizações como abas', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompaniesView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Todas as empresas')
    expect(wrapper.text()).toContain('Clientes ativos')
    expect(wrapper.text()).toContain('Churn')
    wrapper.unmount()
  })

  it('aplica os filtros salvos ao selecionar a aba', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(CompaniesView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    const churnTab = wrapper.findAll('.tab').find((t) => t.text() === 'Churn')!
    await churnTab.trigger('click')
    await flushPromises()

    const calls = fetchMock.mock.calls.map((c: any[]) => String(c[0]))
    expect(calls.some((u: string) => u.includes('sem_dono=true'))).toBe(true)
    wrapper.unmount()
  })

  it('mostra o botão salvar visualização quando há filtro ativo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompaniesView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    expect(wrapper.find('.save-view').exists()).toBe(false)
    await wrapper.find('.check input').setValue(true)
    await flushPromises()
    expect(wrapper.find('.save-view').exists()).toBe(true)
    wrapper.unmount()
  })
})

describe('Formulário de contato personalizável', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renderiza somente os campos visíveis, na ordem configurada', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(ContactsView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
    })
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    await flushPromises()

    const labels = wrapper.findAll('.field label').map((l) => l.text())
    expect(labels[0]).toContain('E-mail')
    expect(labels[1]).toContain('Nome')
    expect(labels.join(' ')).not.toContain('Número de telefone') // oculto na config
    expect(wrapper.text()).toContain('Criar e adicionar outro')
    wrapper.unmount()
  })
})
