// Leads de prospecção (10.3): filas, chips com contadores, semáforo, ações em
// massa e o painel contextual com J, K e Esc.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { useAuthStore } from '../src/stores/auth'
import LeadsInboxView from '../src/views/LeadsInboxView.vue'
import type { Lead, LeadCompleto } from '../src/types-prospeccao'

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

const stubs = {
  RouterLink: { props: ['to'], template: '<a :href="String(to)"><slot /></a>' }
}

// A gaveta e os modais vão para o body (Teleport): procure lá.
const corpo = () => document.body
const noBody = (sel: string) => corpo().querySelector<HTMLElement>(sel)

const montados: { unmount: () => void }[] = []
function montar() {
  const w = mount(LeadsInboxView, { global: { stubs }, attachTo: document.body })
  montados.push(w)
  return w
}

function resposta(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: () => null },
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body))
  }
}

function loginAs(permissions: Record<string, boolean>) {
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
    urgencia: 'atrasado',
    temperatura: '',
    etiquetas: [],
    nota: 80,
    valor_estimado: null,
    telefones: [{ numero: '85999990001', tipo: 'celular', principal: true }],
    emails: [],
    tentativas: 0,
    ultimo_contato_em: null,
    ultimo_resultado: '',
    proxima_acao_em: '2026-11-20T12:00:00Z',
    motivo_descarte: '',
    deal_id: null,
    meeting_id: null,
    convertido_em: null,
    whatsapp_optin: false,
    pessoa_fisica: false,
    em_atendimento_por: null,
    visto: true,
    criado_em: '2026-11-20T12:00:00Z',
    atualizado_em: '2026-11-20T12:00:00Z',
    ...extra
  }
}

function completo(l: Lead): LeadCompleto {
  return {
    lead: l,
    resumo: { frases: ['Padaria em Fortaleza/CE'], decisores: [], sinais: [], cadastro: null as any },
    contatos: [],
    tentativas: [],
    roteiro: null,
    alertas: [],
    janela: { aberta: true, texto: 'segunda a sexta, 9h às 18h', reabre_em: null }
  }
}

const contadores = { meus: 3, sem_dono: 5, todos: 8, novo: 2, tentando: 1, retornar: 0, convertido: 4, descartado: 1, atrasados: 2, hoje: 1 }
const leads = [lead(1), lead(2, { urgencia: 'hoje', estado: 'tentando' }), lead(3, { urgencia: 'nenhuma' })]

let chamadas: { url: string; init: any }[] = []

function api(extra: (url: string, init: any) => unknown = () => undefined) {
  chamadas = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init: any = {}) => {
      chamadas.push({ url: String(url), init })
      const u = String(url)
      const proprio = extra(u, init)
      if (proprio !== undefined) return Promise.resolve(proprio)
      if (u.includes('/leads/bulk')) return Promise.resolve(resposta({ afetados: 2 }))
      if (u.includes('/leads/listas')) return Promise.resolve(resposta([{ id: 7, nome: 'Padarias CE', abertos: 3 }]))
      if (u.includes('/settings/prospeccao')) return Promise.resolve(resposta({}))
      if (u.includes('/users')) return Promise.resolve(resposta([{ id: 1, name: 'Ana Silva', active: true }, { id: 2, name: 'Bruno', active: true }]))
      const m = u.match(/\/leads\/(\d+)$/)
      if (m) return Promise.resolve(resposta(completo(leads.find((l) => l.id === Number(m[1])) ?? lead(Number(m[1])))))
      if (u.includes('/leads?')) return Promise.resolve(resposta({ data: leads, pagination: { page: 1, per_page: 25, total: 3 }, contadores }))
      return Promise.resolve(resposta({}))
    })
  )
}

const urlsDe = (trecho: string) => chamadas.filter((c) => c.url.includes(trecho))

function tecla(key: string) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key }))
}

describe('LeadsInboxView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    rota.query = {}
    push.mockClear()
    replace.mockClear()
  })

  afterEach(() => {
    while (montados.length) montados.pop()!.unmount()
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
  })

  it('vendedor vê as filas Meus e Sem dono com contadores, sem a aba Todos nem Atribuir', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    api()
    const wrapper = montar()
    await flushPromises()

    const filas = wrapper.findAll('.fila').map((b) => b.text())
    expect(filas).toEqual(['Meus 3', 'Sem dono 5'])
    expect(urlsDe('/leads?')[0].url).toContain('fila=meus')
    expect(urlsDe('/leads?')[0].url).toContain('sort=urgencia')
    // Sem leads.assign não busca usuários.
    expect(urlsDe('/users').length).toBe(0)

    await wrapper.findAll('tbody input[type="checkbox"]')[0].setValue(true)
    expect(wrapper.find('.bulk-bar').exists()).toBe(true)
    expect(wrapper.find('select[aria-label="Atribuir"]').exists()).toBe(false)
  })

  it('gestor vê a aba Todos e o atribuir em massa', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true, 'leads.assign': true })
    api()
    const wrapper = montar()
    await flushPromises()

    expect(wrapper.findAll('.fila').map((b) => b.text())).toEqual(['Meus 3', 'Sem dono 5', 'Todos 8'])
    await wrapper.find('[data-fila="todos"]').trigger('click')
    await flushPromises()
    expect(urlsDe('/leads?').at(-1)!.url).toContain('fila=todos')

    await wrapper.findAll('tbody input[type="checkbox"]')[0].setValue(true)
    await wrapper.findAll('tbody input[type="checkbox"]')[2].setValue(true)
    await wrapper.find('select[aria-label="Atribuir"]').setValue('2')
    await flushPromises()
    const bulk = urlsDe('/leads/bulk')[0]
    expect(JSON.parse(bulk.init.body)).toEqual({ ids: [1, 3], acao: 'dono', owner_id: 2 })
  })

  it('chips de estado e de urgência mostram contadores e filtram', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    api()
    const wrapper = montar()
    await flushPromises()

    expect(wrapper.find('[data-estado="tentando"]').text()).toContain('1')
    expect(wrapper.find('[data-estado="convertido"]').text()).toContain('4')
    expect(wrapper.find('button[data-urgencia="atrasado"]').text()).toContain('2')

    await wrapper.find('[data-estado="tentando"]').trigger('click')
    await flushPromises()
    expect(urlsDe('/leads?').at(-1)!.url).toContain('estado=tentando')

    await wrapper.find('button[data-urgencia="hoje"]').trigger('click')
    await flushPromises()
    expect(urlsDe('/leads?').at(-1)!.url).toContain('urgencia=hoje')
  })

  it('semáforo de urgência: vermelho, verde e amarelo', async () => {
    loginAs({ 'leads.view': true })
    api()
    const wrapper = montar()
    await flushPromises()

    const sinais = wrapper.findAll('tbody .semaforo')
    expect(sinais.map((s) => s.classes().find((c) => c !== 'semaforo'))).toEqual(['red', 'green', 'amber'])
    // Sem leads.edit não há seleção nem "Começar a ligar".
    expect(wrapper.find('tbody input[type="checkbox"]').exists()).toBe(false)
    expect(wrapper.find('.comecar').exists()).toBe(false)
  })

  it('painel contextual: clicar abre, J e K navegam, Esc fecha', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    api()
    const wrapper = montar()
    await flushPromises()

    await wrapper.find('tbody tr[data-lead="1"]').trigger('click')
    await flushPromises()
    expect(rota.query.lead).toBe('1')
    expect(urlsDe('/leads/1').length).toBe(1)
    expect(noBody('.lead-painel')).not.toBeNull()

    tecla('j')
    await flushPromises()
    expect(rota.query.lead).toBe('2')
    expect(urlsDe('/leads/2').length).toBe(1)

    tecla('j')
    await flushPromises()
    expect(rota.query.lead).toBe('3')
    // No fim da página, J não sai do último.
    tecla('j')
    await flushPromises()
    expect(rota.query.lead).toBe('3')

    tecla('k')
    await flushPromises()
    expect(rota.query.lead).toBe('2')

    tecla('Escape')
    await flushPromises()
    expect(rota.query.lead).toBeUndefined()
    expect(noBody('.lead-painel')).toBeNull()
  })

  it('abre direto pelo endereço /leads?lead=<id>', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    rota.query = { lead: '2' }
    api()
    const wrapper = montar()
    await flushPromises()
    expect(urlsDe('/leads/2').length).toBe(1)
    expect(noBody('.lead-painel')!.textContent).toContain('Tentando contato')
    expect(wrapper.exists()).toBe(true)
  })

  it('descartar em massa exige o motivo', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    api()
    const wrapper = montar()
    await flushPromises()

    await wrapper.findAll('tbody input[type="checkbox"]')[1].setValue(true)
    await wrapper.find('.massa-descartar').trigger('click')
    await flushPromises()
    const form = noBody('#massa-motivo')!.closest('form')!
    form.dispatchEvent(new Event('submit'))
    await flushPromises()
    expect(urlsDe('/leads/bulk').length).toBe(0)

    const select = noBody('#massa-motivo') as HTMLSelectElement
    select.value = 'Sem orçamento'
    select.dispatchEvent(new Event('change'))
    form.dispatchEvent(new Event('submit'))
    await flushPromises()
    expect(JSON.parse(urlsDe('/leads/bulk')[0].init.body)).toEqual({ ids: [2], acao: 'descartar', motivo: 'Sem orçamento' })
  })

  it('"Começar a ligar" leva a fila com a lista filtrada', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    api()
    const wrapper = montar()
    await flushPromises()

    await wrapper.find('select[aria-label="Lista"]').setValue('7')
    await flushPromises()
    expect(urlsDe('/leads?').at(-1)!.url).toContain('lista_id=7')
    await wrapper.find('.comecar').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/fila-de-ligacoes', query: { lista: '7' } })
  })

  it('registrar resultado pelo painel usa a mesma rota da fila', async () => {
    loginAs({ 'leads.view': true, 'leads.edit': true })
    rota.query = { lead: '1' }
    api((u) =>
      u.endsWith('/leads/1/resultado')
        ? resposta({ lead: lead(1, { estado: 'tentando' }), call: {}, proximo_passo: { tipo: 'ligar', em: null, texto: 'Nova tentativa amanhã' } })
        : undefined
    )
    const wrapper = montar()
    await flushPromises()

    noBody('.lead-painel [data-resultado="nao_atendeu"]')!.click()
    await flushPromises()
    expect(wrapper.exists()).toBe(true)
    const post = urlsDe('/leads/1/resultado')[0]
    const corpo = JSON.parse(post.init.body)
    expect(corpo.resultado).toBe('nao_atendeu')
    expect(corpo.numero).toBe('85999990001')
    expect(typeof corpo.chave).toBe('string')
  })
})
