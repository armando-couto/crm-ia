import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import SequencesView from '../src/views/SequencesView.vue'
import SequenceEditorView from '../src/views/SequenceEditorView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const routerStubs = { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } }

const sequence = {
  id: 1,
  name: 'Prospecção - E-mail forte',
  description: '',
  steps: [
    { kind: 'email_auto', delay_days: 0, subject: 'Oi', body: 'Olá' },
    { kind: 'email_auto', delay_days: 3, subject: 'Retomando', body: 'Seguindo' },
    { kind: 'task_call', delay_days: 2, title: 'Ligar para o prospect' }
  ],
  active: true,
  dynamic: true,
  exit_on_reply: true,
  exit_on_meeting: true,
  owner_id: 1,
  owner_name: 'Eliseu',
  created_by: 1,
  created_at: '',
  updated_at: '2026-08-30T10:00:00Z',
  enrolled: 12,
  active_members: 5,
  open_rate: 58.3,
  reply_rate: 16.7
}

const members = [
  {
    id: 1,
    sequence_id: 1,
    contact_id: 3,
    contact_name: 'Ana Silva',
    contact_email: 'ana@cliente.com',
    step: 2,
    step_label: 'Tarefa de chamada',
    next_run_at: '',
    status: 'aguardando_tarefa',
    exit_reason: '',
    task_id: 9,
    engaged: true,
    enrolled_at: '2026-08-20T10:00:00Z',
    finished_at: null
  },
  {
    id: 2,
    sequence_id: 1,
    contact_id: 4,
    contact_name: 'João Souza',
    contact_email: 'joao@cliente.com',
    step: 0,
    next_run_at: '',
    status: 'cancelada',
    exit_reason: 'respondeu',
    task_id: null,
    engaged: true,
    enrolled_at: '2026-08-18T10:00:00Z',
    finished_at: '2026-08-19T10:00:00Z'
  }
]

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Eliseu', email: 'e@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

function stubFetch() {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const path = String(url)
    if (options?.method === 'POST' || options?.method === 'PUT') {
      return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(sequence)) })
    }
    let body: unknown = {}
    if (path.includes('/members')) body = { data: members }
    else if (path.match(/\/sequences\/\d+/)) body = sequence
    else if (path.includes('/sequences')) body = { data: [sequence], steps: {} }
    else if (path.includes('/templates')) body = [{ id: 7, name: 'Boas-vindas' }]
    else if (path.includes('/users')) body = [{ id: 1, name: 'Eliseu' }]
    else if (path.includes('/contacts')) body = { data: [] }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('SequencesView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('fetch', stubFetch())
  })

  it('lista com inscritos, abertura e resposta', async () => {
    const wrapper = mount(SequencesView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Prospecção - E-mail forte')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('58%')
    expect(wrapper.text()).toContain('17%')
    expect(wrapper.text()).toContain('dinâmica')
    expect(wrapper.text()).toContain('2 e-mails automáticos · 1 tarefa manual')
  })

  it('abre a sequência pelo nome', async () => {
    const wrapper = mount(SequencesView, { global: { stubs: routerStubs } })
    await flushPromises()

    await wrapper.find('.link').trigger('click')
    expect(wrapper.emitted('open')?.[0]).toEqual([1])
  })

  it('esconde criação de quem não gerencia', async () => {
    loginAs('seller', { 'automations.view': true })
    const wrapper = mount(SequencesView, { global: { stubs: routerStubs } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('Criar sequência')
  })
})

describe('SequenceEditorView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('fetch', stubFetch())
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  it('mostra as quatro abas do editor', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 1 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    const abas = wrapper.findAll('.seq-tabs button').map((b) => b.text())
    expect(abas[0]).toBe('Etapas')
    expect(abas[1]).toContain('Inscritos')
    expect(abas[2]).toBe('Configurações')
    expect(abas[3]).toBe('Automatizar')
  })

  it('desenha as etapas com a espera entre elas', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 1 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    expect(wrapper.findAll('.step')).toHaveLength(3)
    // Duas esperas: antes da etapa 2 e da 3.
    expect(wrapper.findAll('.delay')).toHaveLength(2)
    // Etapa manual avisa que pausa a cadência.
    expect(wrapper.text()).toContain('pausa a cadência')
  })

  it('mostra os inscritos com etapa e motivo de saída', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 1 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    await wrapper.findAll('.seq-tabs button')[1].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Ana Silva')
    expect(wrapper.text()).toContain('3 de 3')
    expect(wrapper.text()).toContain('aguardando tarefa')
    expect(wrapper.text()).toContain('respondeu')
  })

  it('mostra as regras de saída na aba Automatizar', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 1 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    await wrapper.findAll('.seq-tabs button')[3].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Quando o contato responde a qualquer e-mail')
    expect(wrapper.text()).toContain('Quando uma reunião é agendada')
  })

  it('adiciona uma etapa nova', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 1 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    await wrapper.findAll('.add-step .btn')[2].trigger('click')
    expect(wrapper.findAll('.step')).toHaveLength(4)
  })

  it('salva a sequência com as etapas', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 1 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    await wrapper.find('.head-actions .btn-primary').trigger('click')
    await flushPromises()

    const put = (globalThis.fetch as any).mock.calls.find((c: any[]) => c[1]?.method === 'PUT')
    expect(put).toBeTruthy()
    const body = JSON.parse(put![1].body)
    expect(body.steps).toHaveLength(3)
    expect(body.dynamic).toBe(true)
  })

  it('começa uma sequência nova com um e-mail automático', async () => {
    const wrapper = mount(SequenceEditorView, {
      props: { sequenceId: 0 },
      global: { stubs: routerStubs }
    })
    await flushPromises()

    expect(wrapper.findAll('.step')).toHaveLength(1)
    expect((wrapper.vm as any).seq.steps[0].kind).toBe('email_auto')
  })
})
