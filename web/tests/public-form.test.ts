import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PublicFormView from '../src/views/PublicFormView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: { slug: 'contato-do-site' }, query: {} })
}))

const definition = {
  slug: 'contato-do-site',
  name: 'Contato do site',
  headline: 'Fale com a Fix Pay',
  description: 'Respondemos em até 1 dia útil.',
  submit_label: 'Quero falar',
  fields: [
    { key: 'first_name', label: 'Nome', type: 'texto', required: true },
    { key: 'email', label: 'E-mail', type: 'email', required: true },
    { key: 'porte', label: 'Porte', type: 'selecao', required: false, options: ['Pequeno', 'Médio'] },
    { key: 'message', label: 'Mensagem', type: 'textarea', required: false }
  ]
}

function stubFetch(submitResponse: any = { message: 'Recebemos seus dados.' }, submitOk = true) {
  return vi.fn().mockImplementation((_url: string, options?: any) => {
    if (options?.method === 'POST') {
      return Promise.resolve({
        ok: submitOk,
        status: submitOk ? 200 : 400,
        json: () => Promise.resolve(submitResponse)
      })
    }
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(definition) })
  })
}

describe('PublicFormView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renderiza os campos conforme a definição', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(PublicFormView)
    await flushPromises()

    expect(wrapper.text()).toContain('Fale com a Fix Pay')
    expect(wrapper.text()).toContain('Respondemos em até 1 dia útil.')
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('select').exists()).toBe(true)
    expect(wrapper.find('button.submit').text()).toBe('Quero falar')
  })

  it('usa o tipo de input certo para e-mail e telefone', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(PublicFormView)
    await flushPromises()

    expect(wrapper.find('#email').attributes('type')).toBe('email')
    expect(wrapper.find('#first_name').attributes('type')).toBe('text')
  })

  it('envia os valores preenchidos e mostra a mensagem de sucesso', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(PublicFormView)
    await flushPromises()

    await wrapper.find('#first_name').setValue('Ana')
    await wrapper.find('#email').setValue('ana@cliente.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    const body = JSON.parse(post![1].body)
    expect(body.first_name).toBe('Ana')
    expect(body.email).toBe('ana@cliente.com')
    expect(wrapper.text()).toContain('Recebemos seus dados.')
  })

  it('envia o campo isca vazio para o backend detectar robô', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(PublicFormView)
    await flushPromises()

    await wrapper.find('#email').setValue('ana@cliente.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(JSON.parse(post![1].body)).toHaveProperty('_gotcha', '')
  })

  it('mostra o erro devolvido pelo backend', async () => {
    vi.stubGlobal('fetch', stubFetch({ error: 'preencha o campo Nome' }, false))
    const wrapper = mount(PublicFormView)
    await flushPromises()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('preencha o campo Nome')
  })

  it('avisa quando o formulário não existe', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 404, json: () => Promise.resolve({}) })
    )
    const wrapper = mount(PublicFormView)
    await flushPromises()

    expect(wrapper.text()).toContain('Formulário não encontrado')
  })
})
