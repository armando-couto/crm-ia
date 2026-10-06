import { describe, expect, it } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AdvancedFilters from '../src/components/AdvancedFilters.vue'
import type { FilterFieldDef, FilterGroup } from '../src/types'

const fields: FilterFieldDef[] = [
  { key: 'name', label: 'Nome', kind: 'text' },
  {
    key: 'lifecycle_stage',
    label: 'Fase do ciclo de vida',
    kind: 'enum',
    options: [
      { value: 'lead', label: 'Lead' },
      { value: 'cliente', label: 'Cliente' }
    ]
  },
  { key: 'last_activity', label: 'Última atividade', kind: 'date' }
]

function build(modelValue: FilterGroup[] = []) {
  return mount(AdvancedFilters, {
    props: { open: true, fields, modelValue },
    global: { stubs: { Teleport: true } },
    attachTo: document.body
  })
}

async function open(wrapper: ReturnType<typeof build>) {
  // O drawer hidrata os grupos ao abrir (watch em open).
  await wrapper.setProps({ open: false })
  await wrapper.setProps({ open: true })
  await flushPromises()
}

describe('AdvancedFilters', () => {
  it('monta uma condição enum e emite apply com a estrutura correta', async () => {
    const wrapper = build()
    await open(wrapper)

    await wrapper.find('.af-add').trigger('click')
    const selects = wrapper.findAll('.af-editor select')
    await selects[0].setValue('lifecycle_stage')
    await flushPromises()

    // Operador padrão do enum: é qualquer de
    const checkboxes = wrapper.findAll('.af-options input[type="checkbox"]')
    await checkboxes[1].setValue(true) // Cliente
    await wrapper.find('.af-editor-actions .btn-primary').trigger('click')

    expect(wrapper.text()).toContain('Fase do ciclo de vida é qualquer de Cliente')

    await wrapper.find('footer .btn-primary').trigger('click')
    const emitted = wrapper.emitted('apply')![0][0] as FilterGroup[]
    expect(emitted).toEqual([
      { conditions: [{ field: 'lifecycle_stage', op: 'any_of', values: ['cliente'] }] }
    ])
    wrapper.unmount()
  })

  it('grupos são unidos por "ou" e condições por "e"', async () => {
    const wrapper = build([
      {
        conditions: [
          { field: 'name', op: 'contains', value: 'silva' },
          { field: 'lifecycle_stage', op: 'any_of', values: ['lead'] }
        ]
      },
      { conditions: [{ field: 'last_activity', op: 'older_than', value: '30' }] }
    ])
    await open(wrapper)

    expect(wrapper.text()).toContain('Nome contém silva')
    expect(wrapper.text()).toContain('Fase do ciclo de vida é qualquer de Lead')
    expect(wrapper.text()).toContain('Última atividade há mais de X dias (ou nunca) 30 dias')
    expect(wrapper.findAll('.af-group')).toHaveLength(2)
    expect(wrapper.find('.af-or').exists()).toBe(true)
    wrapper.unmount()
  })

  it('duplica e remove grupos', async () => {
    const wrapper = build([{ conditions: [{ field: 'name', op: 'not_empty' }] }])
    await open(wrapper)

    await wrapper.find('[title="Duplicar grupo"]').trigger('click')
    expect(wrapper.findAll('.af-group')).toHaveLength(2)

    await wrapper.findAll('[title="Excluir grupo"]')[0].trigger('click')
    expect(wrapper.findAll('.af-group')).toHaveLength(1)
    wrapper.unmount()
  })

  it('limpar tudo emite apply com lista vazia', async () => {
    const wrapper = build([{ conditions: [{ field: 'name', op: 'not_empty' }] }])
    await open(wrapper)

    await wrapper.find('footer .btn-outline').trigger('click')
    const emitted = wrapper.emitted('apply')![0][0] as FilterGroup[]
    expect(emitted).toEqual([])
    wrapper.unmount()
  })
})
