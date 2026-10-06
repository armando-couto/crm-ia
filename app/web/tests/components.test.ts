import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import StatCard from '../src/components/StatCard.vue'
import ModalDialog from '../src/components/ModalDialog.vue'

describe('StatCard', () => {
  it('renderiza rótulo, valor e dica', () => {
    const wrapper = mount(StatCard, {
      props: { label: 'Negócios abertos', value: '12', hint: 'R$ 45.000,00' }
    })
    expect(wrapper.text()).toContain('Negócios abertos')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('R$ 45.000,00')
  })

  it('aplica o tom de cor', () => {
    const wrapper = mount(StatCard, { props: { label: 'X', value: '1', tone: 'green' } })
    expect(wrapper.find('.stat').classes()).toContain('green')
  })
})

describe('ModalDialog', () => {
  it('não renderiza quando fechado', () => {
    const wrapper = mount(ModalDialog, {
      props: { title: 'Teste', open: false },
      slots: { default: '<p>conteúdo</p>' }
    })
    expect(document.body.textContent).not.toContain('conteúdo')
    wrapper.unmount()
  })

  it('renderiza título e conteúdo quando aberto', () => {
    const wrapper = mount(ModalDialog, {
      props: { title: 'Novo contato', open: true },
      slots: { default: '<p>formulário aqui</p>' }
    })
    expect(document.body.textContent).toContain('Novo contato')
    expect(document.body.textContent).toContain('formulário aqui')
    wrapper.unmount()
  })

  it('emite close ao clicar no botão de fechar', async () => {
    const wrapper = mount(ModalDialog, {
      props: { title: 'Teste', open: true }
    })
    await document.body.querySelector<HTMLButtonElement>('.close')!.click()
    expect(wrapper.emitted('close')).toBeTruthy()
    wrapper.unmount()
  })
})
