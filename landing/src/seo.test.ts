import { describe, expect, it, vi } from 'vitest'
import { createSSRApp } from 'vue'
import { renderToString } from '@vue/server-renderer'
import App from './App.vue'
import { beneficios, criarFaq, grupos, segmentos } from './conteudo'

// O que sustenta a landing no buscador é o conteúdo estar no HTML sem depender
// de JavaScript, e nunca citar quem processa o pagamento.
const semEspacos = (s: string) => s.replace(/\s+/g, ' ').trim()

describe('landing pré-renderizada', () => {
  it('renderiza o conteúdo inteiro no servidor', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new Error('sem API no build'))))
    const html = await renderToString(createSSRApp(App))
    const texto = semEspacos(html.replace(/<[^>]+>/g, ' '))
    expect(texto).toContain('O CRM que a sua equipe realmente usa')
    expect(html).toContain('<h1>')
    for (const id of ['segmentos', 'funcionalidades', 'organizar', 'automatizar', 'email', 'medir', 'como-funciona', 'beneficios', 'planos', 'faq', 'contato']) {
      expect(html).toContain(`id="${id}"`)
    }
    for (const s of segmentos) expect(texto).toContain(s.nome)
    for (const g of grupos) for (const i of g.itens) expect(texto).toContain(i.t)
    for (const b of beneficios) expect(texto).toContain(b.t)
    for (const { p } of criarFaq('crmia.com.br')) expect(texto).toContain(semEspacos(p))
    for (const t of ['R$ 99', 'Sem taxa de adesão', 'Mandrill', 'Maileroo', '14 dias grátis']) expect(texto).toContain(t)
  })

  it('não cita o processador de pagamento', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new Error('sem API'))))
    const html = (await renderToString(createSSRApp(App))).toLowerCase()
    expect(html).not.toContain('fix pay')
    expect(html).not.toContain('fixpay')
  })
})
