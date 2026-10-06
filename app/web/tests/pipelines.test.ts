import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SettingsPipelinesView from '../src/views/SettingsPipelinesView.vue'

const pipelines = [
  {
    id: 1,
    name: 'Prospecção',
    position: 0,
    deals_count: 384,
    stages: [
      { id: 1, pipeline_id: 1, name: 'Prospecção', position: 0, probability: 20, is_won: false, is_lost: false, deals_count: 23 },
      { id: 2, pipeline_id: 1, name: 'Reunião agendada', position: 1, probability: 40, is_won: false, is_lost: false, deals_count: 2 },
      { id: 3, pipeline_id: 1, name: 'Ganho', position: 2, probability: 100, is_won: true, is_lost: false, deals_count: 355 }
    ]
  },
  { id: 2, name: 'Pós-venda', position: 1, deals_count: 0, stages: [] }
]

function stubFetch() {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const u = String(url)
    let body: unknown = {}
    if (u.includes('/pipelines') && (!options?.method || options.method === 'GET')) {
      body = pipelines
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('SettingsPipelinesView (editor de fases)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build(fetchImpl?: any) {
    vi.stubGlobal('fetch', fetchImpl ?? stubFetch())
    return mount(SettingsPipelinesView, {
      global: { stubs: { Teleport: true } }
    })
  }

  it('lista pipelines no seletor com contagem de negócios', async () => {
    const wrapper = build()
    await flushPromises()

    const options = wrapper.findAll('.picker option').map((o) => o.text())
    expect(options[0]).toContain('Prospecção (384 negócios)')
    expect(options[1]).toContain('Pós-venda (0 negócios)')
    wrapper.unmount()
  })

  it('mostra a tabela de fases com nome editável, probabilidade e usado por', async () => {
    const wrapper = build()
    await flushPromises()

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(3)

    const first = rows[0]
    expect((first.find('.stage-name-input').element as HTMLInputElement).value).toBe('Prospecção')
    expect((first.find('.prob-select').element as HTMLSelectElement).value).toBe('20')
    expect(first.text()).toContain('23 negócio(s)')

    // Fase de ganho aparece com a opção especial selecionada.
    const won = rows[2]
    expect((won.find('.prob-select').element as HTMLSelectElement).value).toBe('won')
    wrapper.unmount()
  })

  it('alterar a probabilidade salva a fase via PUT', async () => {
    const fetchMock = stubFetch()
    const wrapper = build(fetchMock)
    await flushPromises()

    await wrapper.findAll('tbody tr')[0].find('.prob-select').setValue('lost')
    await flushPromises()

    const putCall = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'PUT' && String(c[0]).includes('/stages/1'))
    expect(putCall).toBeTruthy()
    const payload = JSON.parse(putCall![1].body)
    expect(payload.is_lost).toBe(true)
    expect(payload.probability).toBe(0)
    wrapper.unmount()
  })

  it('reordenar fase chama o endpoint de reorder com a nova ordem', async () => {
    const fetchMock = stubFetch()
    const wrapper = build(fetchMock)
    await flushPromises()

    // Desce a primeira fase.
    await wrapper.findAll('tbody tr')[0].findAll('.order-btns button')[1].trigger('click')
    await flushPromises()

    const reorderCall = fetchMock.mock.calls.find((c: any[]) => String(c[0]).includes('/stages/reorder'))
    expect(reorderCall).toBeTruthy()
    expect(JSON.parse(reorderCall![1].body).stage_ids).toEqual([2, 1, 3])
    wrapper.unmount()
  })

  it('bloqueia excluir fase com negócios', async () => {
    const wrapper = build()
    await flushPromises()

    vi.stubGlobal('confirm', vi.fn())
    await wrapper.findAll('tbody tr')[0].find('.btn-danger').trigger('click')
    // confirm não deve nem ser chamado: a fase tem 23 negócios
    expect(vi.mocked(confirm)).not.toHaveBeenCalled()
    expect(wrapper.text()).toBeTruthy()
    wrapper.unmount()
  })
})
