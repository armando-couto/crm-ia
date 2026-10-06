import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import AutomationsView from '../src/views/AutomationsView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const automations = [
  {
    id: 1,
    name: 'Boas-vindas do site',
    description: 'Recebe o lead do formulário',
    trigger_kind: 'formulario_enviado',
    trigger_config: {},
    actions: [
      { kind: 'enviar_email', config: { subject: 'Olá' } },
      { kind: 'aguardar', config: { days: 3 } },
      { kind: 'criar_tarefa', config: { title: 'Ligar' } }
    ],
    active: true,
    runs: 12,
    last_run_at: '2026-08-30T10:00:00Z',
    created_by: 1,
    created_at: '',
    updated_at: ''
  }
]

const catalog = {
  data: automations,
  triggers: {
    contato_criado: 'Contato criado',
    formulario_enviado: 'Formulário enviado',
    negocio_parado: 'Negócio parado há X dias'
  },
  actions: {
    enviar_email: 'Enviar e-mail',
    criar_tarefa: 'Criar tarefa',
    aguardar: 'Aguardar X dias'
  },
  time_triggers: ['negocio_parado'],
  pending: { 1: 4 }
}

function stubFetch(list = catalog) {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const path = String(url)
    if (options?.method === 'POST' || options?.method === 'PUT') {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve('{"id":1}') })
    }
    let body: unknown = {}
    if (path.includes('/automations') && path.includes('/runs')) {
      body = { data: [{ id: 1, status: 'sucesso', detail: 'Enviar e-mail', created_at: '2026-08-30T10:00:00Z' }] }
    } else if (path.includes('/automations')) body = list
    else if (path.includes('/users')) body = [{ id: 1, name: 'Ana' }]
    else if (path.includes('/lists')) body = { data: [{ id: 3, name: 'Clientes' }] }
    else if (path.includes('/templates')) body = [{ id: 7, name: 'Boas-vindas' }]
    else if (path.includes('/pipelines')) {
      body = { data: [{ id: 1, name: 'Vendas', stages: [{ id: 10, name: 'Proposta' }] }] }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'T', email: 't@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

describe('AutomationsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  it('mostra o gatilho e a sequência de ações em português', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AutomationsView)
    await flushPromises()

    expect(wrapper.text()).toContain('Boas-vindas do site')
    expect(wrapper.text()).toContain('Formulário enviado')
    expect(wrapper.text()).toContain('Enviar e-mail → Aguardar X dias → Criar tarefa')
  })

  it('mostra quantos contatos estão no meio da sequência', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AutomationsView)
    await flushPromises()

    expect(wrapper.text()).toContain('4 em sequência')
    expect(wrapper.text()).toContain('12 execução(ões)')
  })

  it('mostra estado vazio com exemplos', async () => {
    vi.stubGlobal('fetch', stubFetch({ ...catalog, data: [] }))
    const wrapper = mount(AutomationsView)
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhuma automação ainda')
  })

  it('esconde a edição de quem não pode gerenciar', async () => {
    loginAs('seller', { 'automations.view': true })
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AutomationsView)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Nova automação')
    expect(wrapper.text()).not.toContain('Editar')
    // Ver o histórico continua liberado.
    expect(wrapper.text()).toContain('Histórico')
  })

  it('monta uma automação nova com o primeiro passo já pronto', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AutomationsView)
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    await flushPromises()

    const vm = wrapper.vm as any
    expect(vm.editing.trigger_kind).toBe('contato_criado')
    expect(vm.editing.actions).toHaveLength(1)
  })

  it('reordena e remove passos da sequência', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AutomationsView)
    await flushPromises()

    const vm = wrapper.vm as any
    vm.edit(automations[0])
    await flushPromises()

    vm.moveAction(0, 1)
    expect(vm.editing.actions[0].kind).toBe('aguardar')
    expect(vm.editing.actions[1].kind).toBe('enviar_email')

    vm.removeAction(0)
    expect(vm.editing.actions).toHaveLength(2)
  })

  it('salva enviando gatilho e ações', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(AutomationsView)
    await flushPromises()

    const vm = wrapper.vm as any
    vm.edit(automations[0])
    await vm.save()
    await flushPromises()

    const put = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'PUT')
    const body = JSON.parse(put![1].body)
    expect(body.trigger_kind).toBe('formulario_enviado')
    expect(body.actions).toHaveLength(3)
  })

  it('abre o histórico de execuções', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(AutomationsView)
    await flushPromises()

    await wrapper.find('.auto-actions .btn').trigger('click')
    await flushPromises()

    expect((wrapper.vm as any).history.runs).toHaveLength(1)
  })
})
