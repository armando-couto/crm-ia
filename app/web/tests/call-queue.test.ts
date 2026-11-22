// Fila de ligações (10.4): modo foco, atalhos, cronômetro, rascunho de notas,
// avanço automático, pular, conflitos, reunião, estados vazios e trava.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { useAuthStore } from '../src/stores/auth'
import { chaveLocal } from '../src/tenant'
import CallQueueView from '../src/views/CallQueueView.vue'
import type { Lead, LeadCompleto, ProximoResposta } from '../src/types-prospeccao'

const rota = reactive<{ params: Record<string, string>; query: Record<string, any> }>({ params: {}, query: {} })
const push = vi.fn()
const replace = vi.fn((destino: { query?: Record<string, any> }) => {
  rota.query = { ...(destino.query ?? {}) }
  return Promise.resolve()
})

vi.mock('vue-router', () => ({
  useRoute: () => rota,
  useRouter: () => ({ push, replace })
}))

const stubs = { RouterLink: { props: ['to'], template: '<a :href="String(to)"><slot /></a>' } }

function resposta(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: () => null },
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body))
  }
}

function loginAs(permissions: Record<string, boolean> = { 'leads.view': true, 'leads.edit': true }) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana Silva', email: 'a@exemplo.com.br', role: 'seller', active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

function lead(id: number, extra: Partial<Lead> = {}): Lead {
  return {
    id,
    titulo: `Padaria ${id}`,
    company_id: 100 + id,
    company_name: `Padaria ${id} Ltda`,
    contact_id: null,
    contact_name: '',
    cnpj: '11222333000181',
    cidade: 'Fortaleza',
    uf: 'CE',
    origem: 'prospeccao',
    lista_id: 7,
    lista_nome: 'Padarias CE',
    item_id: id,
    owner_id: 1,
    owner_name: 'Ana Silva',
    estado: 'novo',
    urgencia: 'hoje',
    temperatura: '',
    etiquetas: [],
    nota: 80,
    valor_estimado: null,
    telefones: [
      { numero: '85999990001', tipo: 'celular', principal: true, pessoa: 'Maria Souza' },
      { numero: '8530000001', tipo: 'fixo' }
    ],
    emails: [],
    tentativas: 0,
    ultimo_contato_em: null,
    ultimo_resultado: '',
    proxima_acao_em: null,
    motivo_descarte: '',
    deal_id: null,
    meeting_id: null,
    convertido_em: null,
    whatsapp_optin: false,
    pessoa_fisica: false,
    em_atendimento_por: 1,
    visto: true,
    criado_em: '2026-11-20T12:00:00Z',
    atualizado_em: '2026-11-20T12:00:00Z',
    ...extra
  }
}

function completo(l: Lead, extra: Partial<LeadCompleto> = {}): LeadCompleto {
  return {
    lead: l,
    resumo: {
      frases: ['Padaria em Fortaleza/CE, aberta há 6 anos'],
      decisores: [{ nome: 'Maria Souza', cargo: 'Sócia-administradora', telefones: [] }],
      sinais: [],
      cadastro: null as any
    },
    contatos: [],
    tentativas: [],
    roteiro: { titulo: 'Abertura', corpo: 'Olá, falo com {{decisor}}? Sou {{vendedor}}.' },
    alertas: [],
    janela: { aberta: true, texto: 'segunda a sexta, 9h às 18h', reabre_em: null },
    ...extra
  }
}

const janela = { aberta: true, texto: 'segunda a sexta, 9h às 18h', reabre_em: null }
const resumo = { na_fila: 12, novos: 8, retornos_vencidos: 2, retornos_hoje: 1, feitas_hoje: 10, conectadas_hoje: 4, reunioes_hoje: 1, meta_dia: 40, janela }

let chamadas: { url: string; init: any }[] = []
/** Fila simulada: cada `proximo` entrega o seguinte. */
let fila: (ProximoResposta | (() => ProximoResposta))[] = []
let config: Record<string, unknown> = {}
let respostaResultado: (corpo: any) => ReturnType<typeof resposta> = () =>
  resposta({ lead: lead(10, { estado: 'tentando' }), call: {}, proximo_passo: { tipo: 'ligar', em: null, texto: 'Nova tentativa em 3 h' } })

function api() {
  chamadas = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init: any = {}) => {
      const u = String(url)
      chamadas.push({ url: u, init })
      if (u.includes('/leads/fila/proximo')) {
        const proximo = fila.shift() ?? { lead: null, motivo_vazio: 'fila_vazia', janela }
        return Promise.resolve(resposta(typeof proximo === 'function' ? proximo() : proximo))
      }
      if (u.includes('/leads/fila/resumo')) return Promise.resolve(resposta(resumo))
      if (u.includes('/leads/listas')) return Promise.resolve(resposta([{ id: 7, nome: 'Padarias CE', abertos: 12 }]))
      if (u.includes('/settings/prospeccao')) return Promise.resolve(resposta(config))
      if (u.endsWith('/resultado')) return Promise.resolve(respostaResultado(JSON.parse(init.body)))
      if (u.endsWith('/liberar')) return Promise.resolve(resposta(undefined, 204))
      if (u.includes('/users')) return Promise.resolve(resposta([{ id: 1, name: 'Ana Silva', active: true }, { id: 2, name: 'Bruno Lima', active: true }]))
      if (u.includes('/agenda/disponibilidade')) {
        return Promise.resolve(
          resposta({
            user_id: 1,
            duracao_min: 30,
            dias: [
              {
                data: '2026-11-23',
                horarios: [
                  { inicio: '2026-11-23T10:00:00-03:00', rotulo: '10:00' },
                  { inicio: '2026-11-23T10:30:00-03:00', rotulo: '10:30' }
                ],
                ocupados: [{ inicio: '2026-11-23T09:00:00-03:00', fim: '2026-11-23T09:30:00-03:00', titulo: 'Reunião interna' }]
              }
            ]
          })
        )
      }
      if (init.method === 'PUT' && /\/leads\/\d+$/.test(u)) return Promise.resolve(resposta(lead(10, { whatsapp_optin: true })))
      return Promise.resolve(resposta({}))
    })
  )
}

const urlsDe = (trecho: string) => chamadas.filter((c) => c.url.includes(trecho))
const corpoDe = (trecho: string, i = 0) => JSON.parse(urlsDe(trecho)[i].init.body ?? 'null')

function tecla(key: string, alvo: EventTarget = window) {
  alvo.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

const montados: { unmount: () => void }[] = []
function montar() {
  const w = mount(CallQueueView, { global: { stubs }, attachTo: document.body })
  montados.push(w)
  return w
}

describe('CallQueueView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs()
    rota.query = {}
    push.mockClear()
    replace.mockClear()
    config = {}
    fila = [
      { lead: completo(lead(10)), motivo_vazio: '', janela },
      { lead: completo(lead(11, { titulo: 'Mercado 11' })), motivo_vazio: '', janela }
    ]
    respostaResultado = () =>
      resposta({ lead: lead(10, { estado: 'tentando' }), call: {}, proximo_passo: { tipo: 'ligar', em: null, texto: 'Nova tentativa em 3 h' } })
    localStorage.clear()
    api()
  })

  afterEach(() => {
    while (montados.length) montados.pop()!.unmount()
    document.body.innerHTML = ''
    document.body.className = ''
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('entra em modo foco, pede o próximo lead e mostra as três colunas', async () => {
    const wrapper = montar()
    await flushPromises()

    expect(document.body.classList.contains('modo-foco')).toBe(true)
    expect(urlsDe('/leads/fila/proximo').length).toBe(1)
    expect(wrapper.find('.quem').text()).toContain('Padaria em Fortaleza/CE')
    expect(wrapper.find('.ligar').text()).toContain('(85) 99999-0001')
    expect(wrapper.find('.ligar').text()).toContain('Sócia-administradora')
    expect(wrapper.find('.historico').text()).toContain('Primeira ligação')
    // Roteiro com as variáveis preenchidas.
    expect(wrapper.find('.roteiro').text()).toContain('Olá, falo com Maria Souza? Sou Ana Silva.')
    // Barra do dia: feitas de meta.
    expect(wrapper.find('.barra').text()).toContain('10 de 40 ligações hoje')
    // Dez botões de resultado com a tecla.
    expect(wrapper.findAll('.resultado-btn').length).toBe(10)
    expect(wrapper.find('[data-resultado="nao_ligar_mais"] .tecla').text()).toBe('0')
  })

  it('lê a lista da URL e manda no proximo', async () => {
    rota.query = { lista: '7' }
    montar()
    await flushPromises()
    expect(corpoDe('/leads/fila/proximo')).toEqual({ lista_id: 7 })
  })

  it('atalho 6 grava "não atendeu" com o número principal e avança sozinho', async () => {
    const wrapper = montar()
    await flushPromises()

    tecla('6')
    await flushPromises()

    const corpo = corpoDe('/leads/10/resultado')
    expect(corpo.resultado).toBe('nao_atendeu')
    expect(corpo.numero).toBe('85999990001')
    expect(corpo.duracao_segundos).toBe(0)
    expect(corpo.chave).toMatch(/^[0-9a-f-]{36}$/)
    expect(urlsDe('/leads/fila/proximo').length).toBe(2)
    expect(wrapper.find('.coluna.quem h2').text()).toBe('Mercado 11')
  })

  it('atalhos são ignorados com o foco nas notas', async () => {
    const wrapper = montar()
    await flushPromises()

    const notas = wrapper.find('#fila-notas').element as HTMLTextAreaElement
    notas.focus()
    tecla('6', notas)
    await flushPromises()
    expect(urlsDe('/resultado').length).toBe(0)
  })

  it('cronômetro começa ao clicar em ligar e vai no resultado', async () => {
    const wrapper = montar()
    await flushPromises()
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })

    const ligar = wrapper.find('.telefone[data-numero="8530000001"] a.ligar')
    expect(ligar.attributes('href')).toBe('tel:+558530000001')
    // O jsdom não navega para tel:.
    ligar.element.addEventListener('click', (e) => e.preventDefault())
    await ligar.trigger('click')
    vi.advanceTimersByTime(75_000)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.cronometro .tempo').text()).toBe('1:15')

    vi.useRealTimers()
    tecla('7')
    await flushPromises()
    const corpo = corpoDe('/leads/10/resultado')
    expect(corpo.resultado).toBe('caixa_postal')
    expect(corpo.numero).toBe('8530000001')
    expect(corpo.duracao_segundos).toBe(75)
  })

  it('rascunho de notas fica no navegador e some depois do resultado', async () => {
    const wrapper = montar()
    await flushPromises()

    await wrapper.find('#fila-notas').setValue('Falar com a Maria depois das 14h')
    const chave = chaveLocal('fila_notas_10')
    expect(localStorage.getItem(chave)).toBe('Falar com a Maria depois das 14h')

    tecla('8')
    await flushPromises()
    expect(corpoDe('/leads/10/resultado').notas).toBe('Falar com a Maria depois das 14h')
    expect(localStorage.getItem(chave)).toBeNull()
  })

  it('recupera o rascunho ao voltar ao mesmo lead', async () => {
    localStorage.setItem(chaveLocal('fila_notas_10'), 'anotação antiga')
    const wrapper = montar()
    await flushPromises()
    expect((wrapper.find('#fila-notas').element as HTMLTextAreaElement).value).toBe('anotação antiga')
  })

  it('"Retornar" pede a data antes de gravar', async () => {
    const wrapper = montar()
    await flushPromises()

    tecla('3')
    await flushPromises()
    expect(wrapper.find('[data-complemento="retornar"]').exists()).toBe(true)
    await wrapper.find('[data-complemento="retornar"]').trigger('submit')
    await flushPromises()
    expect(urlsDe('/resultado').length).toBe(0)
    expect(wrapper.text()).toContain('Informe a data e a hora do retorno.')

    await wrapper.findAll('[data-complemento="retornar"] .atalhos .chip')[2].trigger('click')
    await wrapper.find('[data-complemento="retornar"]').trigger('submit')
    await flushPromises()
    const corpo = corpoDe('/leads/10/resultado')
    expect(corpo.resultado).toBe('retornar')
    expect(new Date(corpo.retornar_em).getTime()).toBeGreaterThan(Date.now())
  })

  it('"Sem interesse" pede o motivo da configuração', async () => {
    config = { motivos_descarte: ['Já tem fornecedor', 'Sem orçamento'] }
    const wrapper = montar()
    await flushPromises()

    tecla('4')
    await flushPromises()
    const chips = wrapper.findAll('[data-complemento="sem_interesse"] .chip')
    expect(chips.map((c) => c.text())).toEqual(['Já tem fornecedor', 'Sem orçamento', 'Outro'])
    await chips[1].trigger('click')
    await wrapper.find('[data-complemento="sem_interesse"]').trigger('submit')
    await flushPromises()
    expect(corpoDe('/leads/10/resultado')).toMatchObject({ resultado: 'sem_interesse', motivo: 'Sem orçamento' })
  })

  it('reunião: escolhe vendedor e horário livre, e oferece "Marcar mesmo assim" no 409', async () => {
    let tentativa = 0
    respostaResultado = (corpo) => {
      tentativa++
      if (tentativa === 1) return resposta({ error: 'horário ocupado', codigo: 'agenda_ocupada' }, 409)
      return resposta({ lead: lead(10, { estado: 'convertido' }), call: {}, proximo_passo: { tipo: 'reuniao', em: corpo.reuniao.inicio, texto: 'Reunião marcada' } })
    }
    const wrapper = montar()
    await flushPromises()

    tecla('2')
    await flushPromises()
    expect(urlsDe('/agenda/disponibilidade?user_id=1').length).toBeGreaterThan(0)
    expect(wrapper.find('.slot-picker').text()).toContain('Reunião interna')

    await wrapper.findAll('.horario')[1].trigger('click')
    await wrapper.find('[data-complemento="reuniao"]').trigger('submit')
    await flushPromises()
    expect(corpoDe('/leads/10/resultado').reuniao).toMatchObject({ inicio: '2026-11-23T10:30:00-03:00', user_id: 1, duracao_min: 30 })

    // 409 agenda_ocupada: mantém o formulário e oferece forçar.
    const forcar = wrapper.find('.forcar')
    expect(forcar.exists()).toBe(true)
    await forcar.trigger('click')
    await flushPromises()
    expect(corpoDe('/leads/10/resultado', 1).reuniao).toMatchObject({ inicio: '2026-11-23T10:30:00-03:00', forcar: true })
    expect(urlsDe('/leads/fila/proximo').length).toBe(2)
  })

  it('reunião para outro vendedor lê a agenda dele', async () => {
    const wrapper = montar()
    await flushPromises()
    tecla('2')
    await flushPromises()
    await wrapper.find('#reuniao-vendedor').setValue('2')
    await flushPromises()
    expect(urlsDe('/agenda/disponibilidade?user_id=2').length).toBe(1)
  })

  it('S pula o lead e manda os pulados da sessão', async () => {
    fila.push({ lead: completo(lead(12)), motivo_vazio: '', janela })
    montar()
    await flushPromises()

    tecla('s')
    await flushPromises()
    expect(corpoDe('/leads/fila/proximo', 1)).toEqual({ pular_ids: [10] })
    tecla('s')
    await flushPromises()
    expect(corpoDe('/leads/fila/proximo', 2)).toEqual({ pular_ids: [10, 11] })
  })

  it('423 (lead com outra pessoa) avisa e busca o próximo', async () => {
    respostaResultado = () => resposta({ error: 'lead em atendimento por Bruno', em_atendimento_por: 2, nome: 'Bruno' }, 423)
    const wrapper = montar()
    await flushPromises()

    tecla('6')
    await flushPromises()
    expect(urlsDe('/leads/fila/proximo').length).toBe(2)
    expect(wrapper.find('.coluna.quem h2').text()).toBe('Mercado 11')
  })

  it('WhatsApp desabilitado sem opt-in quando a configuração exige; a caixa registra o opt-in', async () => {
    const wrapper = montar()
    await flushPromises()

    const whats = wrapper.find('.telefone[data-numero="85999990001"] .whatsapp')
    expect(whats.element.tagName).toBe('BUTTON')
    expect(whats.attributes('disabled')).toBeDefined()

    await wrapper.find('.optin input').trigger('change')
    await flushPromises()
    expect(corpoDe('/leads/10')).toEqual({ whatsapp_optin: true })
    const link = wrapper.find('.telefone[data-numero="85999990001"] a.whatsapp')
    expect(link.attributes('href')).toBe('https://wa.me/5585999990001')
  })

  it('WhatsApp liberado quando a configuração não exige opt-in', async () => {
    config = { exigir_optin_whatsapp: false }
    const wrapper = montar()
    await flushPromises()
    expect(wrapper.find('.telefone[data-numero="85999990001"] a.whatsapp').exists()).toBe(true)
    // Fixo não tem WhatsApp.
    expect(wrapper.find('.telefone[data-numero="8530000001"] .whatsapp').exists()).toBe(false)
  })

  it('fila vazia: "Fila em dia"', async () => {
    fila = [{ lead: null, motivo_vazio: 'fila_vazia', janela }]
    const wrapper = montar()
    await flushPromises()
    expect(wrapper.find('.fila-em-dia').text()).toContain('Fila em dia')
  })

  it('fora da janela: avisa quando reabre', async () => {
    const amanha = new Date()
    amanha.setDate(amanha.getDate() + 1)
    amanha.setHours(9, 0, 0, 0)
    fila = [{ lead: null, motivo_vazio: 'fora_da_janela', janela: { aberta: false, texto: 'segunda a sexta, 9h às 18h', reabre_em: amanha.toISOString() } }]
    const wrapper = montar()
    await flushPromises()
    const texto = wrapper.find('.fora-da-janela').text()
    expect(texto).toContain('Fora da janela de contato')
    expect(texto).toContain('reabre amanhã às 9h')
  })

  it('ao sair solta a trava (keepalive) e tira o modo foco', async () => {
    const wrapper = montar()
    await flushPromises()

    montados.pop()
    wrapper.unmount()
    const liberar = urlsDe('/leads/10/liberar')
    expect(liberar.length).toBe(1)
    expect(liberar[0].init.keepalive).toBe(true)
    expect(document.body.classList.contains('modo-foco')).toBe(false)
  })

  it('Esc sai da fila para /leads', async () => {
    montar()
    await flushPromises()
    tecla('Escape')
    expect(push).toHaveBeenCalledWith('/leads')
  })
})
