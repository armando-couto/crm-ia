import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SettingsPropertiesView from '../src/views/SettingsPropertiesView.vue'
import CustomProperties from '../src/components/CustomProperties.vue'

const properties = [
  {
    id: 1,
    entity: 'companies',
    key: 'numero_do_ec',
    label: 'Número do EC',
    description: 'Código do estabelecimento',
    field_type: 'texto',
    options: [],
    group_name: 'Credenciamento',
    position: 0,
    created_by: 1,
    creator_name: 'Eliseu Becco',
    used_count: 4,
    created_at: new Date().toISOString(),
    updated_at: ''
  },
  {
    id: 2,
    entity: 'companies',
    key: 'produtos',
    label: 'Produtos contratados',
    description: '',
    field_type: 'multipla',
    options: [
      { value: 'pix', label: 'Pix' },
      { value: 'link', label: 'Link de pagamento' }
    ],
    group_name: 'Credenciamento',
    position: 1,
    created_by: 1,
    creator_name: 'Eliseu Becco',
    used_count: 0,
    created_at: new Date().toISOString(),
    updated_at: ''
  }
]

function stubFetch(values: Record<string, unknown> = {}) {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const u = String(url)
    let body: unknown = []
    if (u.includes('/properties/values')) {
      body = options?.method === 'PUT' ? { values: JSON.parse(options.body).values } : { values }
    } else if (u.includes('/properties')) {
      body = options?.method === 'POST' || options?.method === 'PUT' ? properties[0] : properties
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('SettingsPropertiesView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('lista propriedades com tipo, grupo, criador e uso', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsPropertiesView, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('Número do EC')
    expect(wrapper.text()).toContain('numero_do_ec')
    expect(wrapper.text()).toContain('Texto de uma linha')
    expect(wrapper.text()).toContain('Selecionar múltiplas opções')
    expect(wrapper.text()).toContain('Credenciamento')
    expect(wrapper.text()).toContain('Eliseu Becco')
    expect(wrapper.text()).toContain('4 registro(s)')
    wrapper.unmount()
  })

  it('editor mostra opções apenas para tipos de seleção', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsPropertiesView, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    expect(wrapper.find('.options-list').exists()).toBe(false)

    const typeSelect = wrapper.findAll('.field select')[0]
    await typeSelect.setValue('selecao')
    expect(wrapper.text()).toContain('Adicionar opção')

    await wrapper.find('.add-option').trigger('click')
    expect(wrapper.findAll('.options-list li')).toHaveLength(1)
    wrapper.unmount()
  })

  it('cria propriedade enviando entity e tipo', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SettingsPropertiesView, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    await wrapper.find('.field input').setValue('Origem do lead')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(post).toBeTruthy()
    const payload = JSON.parse(post![1].body)
    expect(payload.label).toBe('Origem do lead')
    expect(payload.entity).toBe('contacts')
    expect(payload.field_type).toBe('texto')
    wrapper.unmount()
  })
})

describe('CustomProperties (no registro)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('exibe os valores formatados por tipo', async () => {
    vi.stubGlobal('fetch', stubFetch({ numero_do_ec: '17240', produtos: ['pix', 'link'] }))
    const wrapper = mount(CustomProperties, { props: { entity: 'companies', recordId: 1 } })
    await flushPromises()

    expect(wrapper.text()).toContain('Propriedades personalizadas')
    expect(wrapper.text()).toContain('17240')
    expect(wrapper.text()).toContain('Pix, Link de pagamento')
    wrapper.unmount()
  })

  it('salva os valores editados via PUT', async () => {
    const fetchMock = stubFetch({ numero_do_ec: '17240' })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(CustomProperties, { props: { entity: 'companies', recordId: 1 } })
    await flushPromises()

    await wrapper.find('.props-head .btn-outline').trigger('click')
    await wrapper.find('.field input[type="text"]').setValue('99999')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const put = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'PUT')
    expect(put).toBeTruthy()
    const payload = JSON.parse(put![1].body)
    expect(payload.entity).toBe('companies')
    expect(payload.record_id).toBe(1)
    expect(payload.values.numero_do_ec).toBe('99999')
    wrapper.unmount()
  })
})
