// Detalhe da lista de prospecção (10.2): progresso do job que sobrevive a
// recarregar, seleção além da página, filtros, ficha, enriquecer com
// confirmação de custo, assistente de envio ao CRM, descartar e exportar.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { api } from '../src/api'
import { useAuthStore } from '../src/stores/auth'
import ProspectListDetailView from '../src/views/ProspectListDetailView.vue'
import {
  alternarItem,
  alternarPagina,
  MAX_SELECAO_IDS,
  paraSelecao,
  quantidadeSelecionada,
  selecaoVazia,
  selecionarTodos
} from '../src/components/prospeccao/selecao'
import type { ContadoresLista, EnriquecerEstimativa, Item, Job, Lista } from '../src/types-prospeccao'

const rota = reactive({ params: { id: '3' } as Record<string, string>, query: {} as Record<string, string> })
const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
  useRoute: () => rota
}))

const stubs = { RouterLink: { props: ['to'], template: '<a><slot /></a>' } }

function resposta(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: () => null },
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body))
  }
}

interface Chamada {
  metodo: string
  caminho: string
  corpo: any
}
type Responder = (metodo: string, caminho: string, corpo: any) => unknown
let chamadas: Chamada[] = []
let responder: Responder = () => ({})

function erro(status: number, body: unknown) {
  return { __status: status, body }
}

function instalarFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit = {}) => {
      const caminho = String(url).replace(/^.*\/api\/v1/, '')
      const metodo = init.method ?? 'GET'
      const corpo = typeof init.body === 'string' ? JSON.parse(init.body) : undefined
      chamadas.push({ metodo, caminho, corpo })
      const r: any = await responder(metodo, caminho, corpo)
      if (r && typeof r === 'object' && '__status' in r) return resposta(r.body, r.__status)
      return resposta(r)
    })
  )
}

function contadores(parcial: Partial<ContadoresLista> = {}): ContadoresLista {
  return {
    total: 120,
    novo: 100,
    enriquecido: 15,
    sem_resultado: 0,
    erro: 0,
    enviado: 5,
    descartado: 0,
    duplicado: 0,
    bloqueado: 0,
    com_celular: 80,
    com_email: 40,
    com_decisor: 12,
    com_whatsapp: 10,
    pessoa_fisica: 0,
    custo_centavos: 1200,
    ...parcial
  }
}

function lista(parcial: Partial<Lista> = {}): Lista {
  return {
    id: 3,
    nome: 'Restaurantes de Fortaleza',
    descricao: '',
    origem: 'base',
    filtros: { municipios: [1389] },
    ordem: 'relevancia',
    quantidade_pedida: 120,
    status: 'pronta',
    mensagem: '',
    versao_base: 'v1',
    incluir_pf: false,
    playbook_id: null,
    pipeline_id: null,
    stage_id: null,
    criado_por: 1,
    criado_por_nome: 'Ana Silva',
    criado_em: '2026-11-20T10:00:00Z',
    atualizado_em: '',
    contadores: contadores(),
    ...parcial
  }
}

function item(id: number, parcial: Partial<Item> = {}): Item {
  return {
    id,
    lista_id: 3,
    cnpj: '11222333000181',
    razao_social: `EMPRESA ${id} LTDA`,
    nome_fantasia: `Empresa ${id}`,
    uf: 'CE',
    municipio: 'FORTALEZA',
    cnae_codigo: 5611201,
    cnae_descricao: 'Restaurantes',
    porte: 'me',
    pessoa_fisica: false,
    estado: 'novo',
    nota: 72,
    nota_motivos: [
      { pontos: 35, texto: 'celular próprio' },
      { pontos: 10, texto: 'e-mail corporativo' }
    ],
    tem_celular: true,
    tem_email: true,
    tem_decisor: false,
    tem_whatsapp: false,
    tem_divida: false,
    contato_compartilhado: false,
    telefone_principal: '85999990001',
    enriquecido_em: null,
    enriquecimento_provedor: '',
    custo_centavos: 0,
    erro: '',
    company_id: null,
    lead_id: null,
    enviado_em: null,
    criado_em: '',
    ...parcial
  }
}

function job(parcial: Partial<Job> = {}): Job {
  return {
    id: 9,
    tipo: 'enriquecer',
    lista_id: 3,
    status: 'rodando',
    total: 100,
    feitos: 10,
    erros: 0,
    custo_centavos: 0,
    mensagem: '',
    resultado: null,
    criado_por: 1,
    criado_em: '',
    iniciado_em: null,
    finalizado_em: null,
    ...parcial
  }
}

const pagina = [item(1), item(2), item(3)]

function respostasPadrao(extra: Responder = () => undefined, jobs: Job[] = []): Responder {
  return (metodo, caminho, corpo) => {
    const e = extra(metodo, caminho, corpo)
    if (e !== undefined) return e
    if (caminho === '/prospeccao') return { habilitada: true, motivo: '', cota: {}, base: { pronta: true, amostra: false, carga: {} } }
    if (caminho === '/prospeccao/listas/3') return { lista: lista(), jobs }
    if (caminho.startsWith('/prospeccao/listas/3/itens?')) return { data: pagina, pagination: { page: 1, per_page: 50, total: 120 } }
    if (caminho === '/users') return [
      { id: 1, name: 'Ana Silva', role: 'manager', active: true },
      { id: 2, name: 'Bruno', role: 'seller', active: true },
      { id: 3, name: 'Carla', role: 'seller', active: true }
    ]
    return {}
  }
}

function loginGestor(permissoes: Record<string, boolean> = { 'prospecting.view': true, 'prospecting.manage': true }) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana Silva', email: 'a@exemplo.com.br', role: 'manager', active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissoes
}

function botao(wrapper: ReturnType<typeof mount>, texto: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === texto)
}
function botaoNoBody(texto: string): HTMLButtonElement | undefined {
  return [...document.body.querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent?.trim() === texto)
}
/** Texto da página com espaço comum no lugar do não separável (moeda). */
function textoBody(): string {
  return (document.body.textContent ?? '').replace(/\u00a0/g, ' ')
}
function marcar(el: HTMLInputElement, valor = true) {
  el.checked = valor
  el.dispatchEvent(new Event('change'))
}

describe('seleção além da página', () => {
  it('marca linhas soltas até 5.000 e vira "todos" com exceções', () => {
    const s = reactive(selecaoVazia())
    alternarPagina(s, [1, 2, 3], true)
    expect(quantidadeSelecionada(s, 120)).toBe(3)
    expect(paraSelecao(s, { q: '' })).toEqual({ ids: [1, 2, 3] })

    selecionarTodos(s)
    alternarItem(s, 2)
    expect(quantidadeSelecionada(s, 120)).toBe(119)
    expect(paraSelecao(s, { estado: 'novo', q: ' pizza ', com_celular: false, nota_min: 0 })).toEqual({
      todos: true,
      filtro: { estado: 'novo', q: 'pizza' },
      exceto: [2]
    })

    const cheia = reactive(selecaoVazia())
    for (let i = 0; i < MAX_SELECAO_IDS; i++) cheia.ids.add(i)
    expect(alternarItem(cheia, 999999)).toBe(false)
    expect(cheia.ids.size).toBe(MAX_SELECAO_IDS)
  })
})

describe('tela da lista de prospecção', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginGestor()
    chamadas = []
    push.mockReset()
    rota.params = { id: '3' }
    instalarFetch()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('mostra contadores, itens com nota, motivos e selos', async () => {
    responder = respostasPadrao((_m, caminho) => {
      if (caminho.startsWith('/prospeccao/listas/3/itens?'))
        return {
          data: [item(1, { pessoa_fisica: true, tem_divida: true, tem_whatsapp: true, contato_compartilhado: true })],
          pagination: { page: 1, per_page: 50, total: 1 }
        }
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('h1').text()).toBe('Restaurantes de Fortaleza')
    const cont = wrapper.find('.contadores').text()
    expect(cont).toContain('120')
    expect(cont).toContain('com celular')
    expect(cont).toContain('com decisor')
    expect(cont).toContain('enriquecidas')
    expect(cont).toContain('enviadas ao CRM')
    const linha = wrapper.find('tbody tr')
    expect(linha.text()).toContain('Empresa 1')
    expect(linha.text()).toContain('(85) 99999-0001')
    for (const selo of ['PF', 'dívida', 'WhatsApp', 'contador']) expect(linha.text()).toContain(selo)

    await wrapper.find('.nota').trigger('mouseenter')
    expect(wrapper.find('.motivos').text()).toContain('+35 celular próprio')
  })

  it('retoma o job ativo ao recarregar, consulta a cada 2 s, recarrega ao mudar e libera as ações no fim', async () => {
    vi.useFakeTimers()
    let estadoJob = job({ feitos: 10 })
    responder = respostasPadrao((_m, caminho) => {
      if (caminho === '/prospeccao/jobs/9') return estadoJob
      if (caminho === '/prospeccao/listas/3') return { lista: lista(), jobs: [estadoJob] }
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Enriquecendo')
    expect(wrapper.text()).toContain('10 de 100')
    // Com job ativo, as ações ficam travadas mesmo com seleção.
    await wrapper.find('tbody input[type="checkbox"]').setValue(true)
    expect(botao(wrapper, 'Enriquecer')!.attributes('disabled')).toBeDefined()
    expect(botao(wrapper, 'Enviar ao CRM')!.attributes('disabled')).toBeDefined()

    const listasAntes = chamadas.filter((c) => c.caminho === '/prospeccao/listas/3').length
    estadoJob = job({ feitos: 60 })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(chamadas.filter((c) => c.caminho === '/prospeccao/jobs/9')).toHaveLength(1)
    expect(chamadas.filter((c) => c.caminho === '/prospeccao/listas/3').length).toBe(listasAntes + 1)
    expect(wrapper.text()).toContain('60 de 100')

    estadoJob = job({ feitos: 100, status: 'concluido', custo_centavos: 8000 })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.text()).toContain('Enriquecimento: concluído')
    expect(botao(wrapper, 'Enriquecer')!.attributes('disabled')).toBeUndefined()

    await vi.advanceTimersByTimeAsync(6000)
    expect(chamadas.filter((c) => c.caminho === '/prospeccao/jobs/9')).toHaveLength(2)
    wrapper.unmount()
  })

  it('cancela o job em andamento', async () => {
    responder = respostasPadrao((metodo, caminho) => {
      if (metodo === 'POST' && caminho === '/prospeccao/jobs/9/cancelar') return job({ status: 'cancelado', mensagem: 'cancelado por Ana' })
      return undefined
    }, [job()])
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()
    await botao(wrapper, 'Cancelar')!.trigger('click')
    await flushPromises()
    expect(chamadas.some((c) => c.caminho === '/prospeccao/jobs/9/cancelar')).toBe(true)
    expect(wrapper.text()).toContain('cancelado por Ana')
    wrapper.unmount()
  })

  it('seleciona todos os N além da página, desmarca um e descarta com exceto e filtro', async () => {
    responder = respostasPadrao((metodo, caminho) => {
      if (metodo === 'POST' && caminho === '/prospeccao/listas/3/itens/descartar') return { afetados: 119 }
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()

    await wrapper.find('input[type="checkbox"][aria-label="Selecionar a página"]').setValue(true)
    expect(wrapper.find('.selecao-info').text()).toContain('3 selecionadas')
    await botao(wrapper, 'Selecionar todas as 120')!.trigger('click')
    expect(wrapper.find('.selecao-info').text()).toContain('Todas as 120')

    const linhas = wrapper.findAll('tbody input[type="checkbox"]')
    await linhas[1].setValue(false)
    expect(wrapper.find('.selecao-info').text()).toContain('Todas as 119')
    expect(wrapper.find('.selecao-info').text()).toContain('menos 1')

    await botao(wrapper, 'Descartar')!.trigger('click')
    await flushPromises()
    const descartar = chamadas.find((c) => c.caminho === '/prospeccao/listas/3/itens/descartar')!
    expect(descartar.corpo).toEqual({ selecao: { todos: true, exceto: [2] } })
    expect(wrapper.find('.selecao-info').text()).not.toContain('selecionadas')
  })

  it('filtra pelo estado e pela nota, ordena e recomeça a seleção', async () => {
    vi.useFakeTimers()
    responder = respostasPadrao()
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()

    await wrapper.find('tbody input[type="checkbox"]').setValue(true)
    const enriquecido = wrapper.findAll('.estados .chip').find((c) => c.text().startsWith('Enriquecido'))!
    await enriquecido.trigger('click')
    await flushPromises()
    await wrapper.find('select[aria-label="Nota mínima"]').setValue('50')
    await flushPromises()
    await wrapper.find('input[aria-label="Buscar empresa"]').setValue('pizza')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    await wrapper.findAll('th.ordenavel').find((t) => t.text().startsWith('Cidade'))!.trigger('click')
    await flushPromises()

    const ultima = chamadas.filter((c) => c.caminho.startsWith('/prospeccao/listas/3/itens?')).pop()!
    const q = new URLSearchParams(ultima.caminho.split('?')[1])
    expect(q.get('estado')).toBe('enriquecido')
    expect(q.get('nota_min')).toBe('50')
    expect(q.get('q')).toBe('pizza')
    expect(q.get('sort')).toBe('municipio')
    expect(q.get('dir')).toBe('asc')
    expect(q.get('page')).toBe('1')
    expect(wrapper.find('.selecao-info').text()).not.toContain('selecionada')

    // Com a seleção "todos", o filtro vai junto.
    await botao(wrapper, 'Selecionar todas as 120')!.trigger('click')
    await botao(wrapper, 'Descartar')!.trigger('click')
    await flushPromises()
    const descartar = chamadas.find((c) => c.caminho === '/prospeccao/listas/3/itens/descartar')!
    expect(descartar.corpo.selecao).toEqual({ todos: true, filtro: { estado: 'enriquecido', q: 'pizza', nota_min: 50 } })
  })

  it('abre a ficha do item na gaveta com contatos, origem, sócios e dívida', async () => {
    responder = respostasPadrao((_m, caminho) => {
      if (caminho === '/prospeccao/itens/1')
        return {
          ...item(1),
          dados: {
            cnpj: '11222333000181',
            matriz: true,
            razao_social: 'EMPRESA 1 LTDA',
            nome_fantasia: 'Empresa 1',
            natureza: { codigo: 2062, descricao: 'Sociedade Empresária Limitada' },
            pessoa_fisica: false,
            porte: 'me',
            capital: 50000,
            abertura: '2019-03-01',
            situacao: 'ATIVA',
            situacao_desde: null,
            simples: true,
            mei: false,
            cnae: { codigo: 5611201, descricao: 'Restaurantes e similares' },
            cnaes_secundarios: [],
            endereco: { tipo_logradouro: 'RUA', logradouro: 'DAS FLORES', numero: '10', complemento: '', bairro: 'ALDEOTA', cep: '60160000', municipio: 'FORTALEZA', municipio_codigo: 1389, uf: 'CE' },
            telefones: [{ numero: '8530000001', tipo: 'fixo', origem: 'receita' }],
            email: 'contato@exemplo.invalid',
            email_tipo: 'corporativo',
            socios: [{ nome: 'MARIA EXEMPLO', qualificacao: 'Sócio-Administrador', desde: '2019-03-01', tipo: 'pf', documento: '***123456**' }],
            divida: { total: 15000, inscricoes: 2, em_cobranca: 10000, regular: 5000, tipos: ['geral'], ajuizada: false, mais_antiga: '2021-01-01', referencia: '2026-T3' },
            qualidade: 60,
            referencia: '2026-11'
          },
          enriquecimento: {
            provedor: 'wholead',
            demo: true,
            consultado_em: '2026-11-21T10:00:00Z',
            telefones: [{ numero: '85999990001', tipo: 'celular', whatsapp: true, origem: 'wholead' }],
            emails: [],
            pessoas: [{ nome: 'Maria Exemplo', documento: '***123456**', papel: 'socio', cargo: 'Sócia', telefones: [{ numero: '85999990002', tipo: 'celular' }], emails: [] }],
            funcionarios_faixa: '1 a 9',
            faturamento_faixa: '',
            site: '',
            observacao: ''
          }
        }
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await wrapper.find('tbody tr').trigger('click')
    await flushPromises()

    const gaveta = document.body.querySelector('.drawer')!.textContent ?? ''
    expect(gaveta).toContain('(85) 99999-0001')
    expect(gaveta).toContain('(85) 3000-0001')
    expect(gaveta).toContain('Who Lead')
    expect(gaveta).toContain('Receita Federal')
    expect(gaveta).toContain('MARIA EXEMPLO')
    expect(gaveta).toContain('Maria Exemplo')
    expect(gaveta).toContain('demonstração')
    expect(gaveta).toContain('Referência 2026-T3')
    expect(gaveta).toContain('***123456**')
    wrapper.unmount()
  })

  it('enriquece só depois de confirmar o custo e pede nova confirmação se o custo mudou', async () => {
    const estimativa = (custo: number): EnriquecerEstimativa => ({
      selecionados: 3,
      a_enriquecer: 3,
      ja_enriquecidos: 0,
      provedor: { codigo: 'wholead', nome: 'Who Lead (Plus Soluções)', pronto: true, demo: false, motivo: '' },
      custo_unitario_centavos: 80,
      custo_estimado_centavos: custo,
      uso_mes: { custo_centavos: 1200, limite_centavos: 50000 },
      saldo: { plano: 'Pré-pago', franquia_total: null, franquia_restante: null }
    })
    let tentativas = 0
    responder = respostasPadrao((metodo, caminho, corpo) => {
      if (caminho === '/prospeccao/listas/3/enriquecer/estimativa') return estimativa(240)
      if (metodo === 'POST' && caminho === '/prospeccao/listas/3/enriquecer') {
        tentativas++
        if (corpo.custo_confirmado_centavos < 300) return erro(409, { error: 'o custo mudou', codigo: 'custo_mudou', estimativa: estimativa(300) })
        return { job: job({ id: 11, status: 'pendente', feitos: 0 }) }
      }
      if (caminho === '/prospeccao/jobs/11') return job({ id: 11, status: 'pendente', feitos: 0 })
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await wrapper.find('input[aria-label="Selecionar a página"]').setValue(true)
    await botao(wrapper, 'Enriquecer')!.trigger('click')
    await flushPromises()

    expect(chamadas.find((c) => c.caminho === '/prospeccao/listas/3/enriquecer/estimativa')!.corpo).toEqual({
      selecao: { ids: [1, 2, 3] },
      reprocessar: false
    })
    const texto = textoBody()
    expect(texto).toContain('Who Lead (Plus Soluções)')
    expect(texto).toContain('R$ 2,40')
    expect(texto).toContain('R$ 12,00')
    expect(texto).toContain('R$ 500,00')
    expect(texto).toContain('Pré-pago')

    // Sem a confirmação do custo, o botão não envia.
    const enriquecer = () => botaoNoBody('Enriquecer 3')!
    expect(enriquecer().disabled).toBe(true)
    marcar(document.body.querySelector<HTMLInputElement>('.confirmar input')!)
    await flushPromises()
    expect(enriquecer().disabled).toBe(false)
    enriquecer().click()
    await flushPromises()

    expect(tentativas).toBe(1)
    expect(textoBody()).toContain('O custo mudou desde a estimativa')
    expect(textoBody()).toContain('R$ 3,00')
    expect(enriquecer().disabled).toBe(true)
    marcar(document.body.querySelector<HTMLInputElement>('.confirmar input')!)
    await flushPromises()
    enriquecer().click()
    await flushPromises()

    const envios = chamadas.filter((c) => c.metodo === 'POST' && c.caminho === '/prospeccao/listas/3/enriquecer')
    expect(envios[1].corpo).toEqual({ selecao: { ids: [1, 2, 3] }, reprocessar: false, custo_confirmado_centavos: 300 })
    // Fecha o modal e passa a acompanhar o job novo.
    expect(document.body.textContent).not.toContain('Enriquecer contatos')
    expect(wrapper.text()).toContain('Enriquecendo')
    wrapper.unmount()
  })

  it('provedor sem configuração bloqueia o enriquecimento com o motivo', async () => {
    responder = respostasPadrao((_m, caminho) => {
      if (caminho === '/prospeccao/listas/3/enriquecer/estimativa')
        return {
          selecionados: 3,
          a_enriquecer: 3,
          ja_enriquecidos: 0,
          provedor: { codigo: 'wholead', nome: 'Who Lead (Plus Soluções)', pronto: false, demo: false, motivo: 'credencial não configurada' },
          custo_unitario_centavos: 80,
          custo_estimado_centavos: 240,
          uso_mes: { custo_centavos: 0, limite_centavos: 0 },
          saldo: null
        }
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await wrapper.find('input[aria-label="Selecionar a página"]').setValue(true)
    await botao(wrapper, 'Enriquecer')!.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('credencial não configurada')
    expect(botaoNoBody('Enriquecer 3')!.disabled).toBe(true)
    wrapper.unmount()
  })

  it('envia ao CRM em três passos: opções, distribuição em rodízio e confirmação com a prévia', async () => {
    responder = respostasPadrao((metodo, caminho) => {
      if (caminho === '/prospeccao/listas/3/enviar/previa')
        return {
          selecionados: 120,
          aptos: 110,
          duplicados_no_crm: 6,
          bloqueados: 2,
          ja_enviados: 2,
          descartados: 0,
          pessoa_fisica: 0,
          sem_telefone: 4,
          por_usuario: [
            { user_id: 2, nome: 'Bruno', quantidade: 55 },
            { user_id: 3, nome: 'Carla', quantidade: 55 }
          ]
        }
      if (metodo === 'POST' && caminho === '/prospeccao/listas/3/enviar') return { job: job({ id: 12, tipo: 'enviar_crm', status: 'pendente', feitos: 0 }) }
      if (caminho === '/prospeccao/jobs/12') return job({ id: 12, tipo: 'enviar_crm', status: 'pendente', feitos: 0 })
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await wrapper.find('input[aria-label="Selecionar a página"]').setValue(true)
    await botao(wrapper, 'Selecionar todas as 120')!.trigger('click')
    await botao(wrapper, 'Enviar ao CRM')!.trigger('click')
    await flushPromises()

    // Passo 1: vincular duplicados e não criar contatos.
    const radios = [...document.body.querySelectorAll<HTMLInputElement>('input[type="radio"]')]
    marcar(radios.find((r) => r.value === 'vincular')!)
    const criarContatos = [...document.body.querySelectorAll<HTMLInputElement>('.passo input[type="checkbox"]')][0]
    marcar(criarContatos, false)
    await flushPromises()
    botaoNoBody('Continuar')!.click()
    await flushPromises()

    // Passo 2: rodízio entre os dois vendedores (padrão), 20 por dia.
    marcar([...document.body.querySelectorAll<HTMLInputElement>('input[type="radio"]')].find((r) => r.value === 'rodizio')!)
    await flushPromises()
    expect(document.body.textContent).toContain('≈ 60 leads por pessoa')
    const porDia = [...document.body.querySelectorAll<HTMLInputElement>('.passo input[type="checkbox"]')].pop()!
    marcar(porDia)
    await flushPromises()
    expect(document.body.textContent).toContain('cerca de 3 dias úteis')
    botaoNoBody('Continuar')!.click()
    await flushPromises()

    // Passo 3: resumo do servidor.
    const previa = chamadas.find((c) => c.caminho === '/prospeccao/listas/3/enviar/previa')!
    expect(previa.corpo).toEqual({
      selecao: { todos: true },
      duplicados: 'vincular',
      criar_contatos: false,
      distribuicao: { modo: 'rodizio', usuarios: [2, 3], por_dia: 20 }
    })
    const texto = document.body.textContent ?? ''
    expect(texto).toContain('110')
    expect(texto).toContain('6 já estão no CRM')
    expect(texto).toContain('o lead fica ligado à empresa existente')
    expect(texto).toContain('2 bloqueadas')
    expect(texto).toContain('4 sem telefone')
    expect(texto).toContain('Carla')

    botaoNoBody('Enviar 110 ao CRM')!.click()
    await flushPromises()
    const enviar = chamadas.find((c) => c.metodo === 'POST' && c.caminho === '/prospeccao/listas/3/enviar')!
    expect(enviar.corpo).toEqual(previa.corpo)
    expect(wrapper.text()).toContain('Enviando ao CRM')
    wrapper.unmount()
  })

  it('ação com job ativo no servidor (409 job_ativo) passa a acompanhar o job', async () => {
    responder = respostasPadrao((metodo, caminho) => {
      if (metodo === 'POST' && caminho === '/prospeccao/listas/3/itens/descartar')
        return erro(409, { error: 'há um trabalho em andamento', codigo: 'job_ativo', job: job({ id: 20, tipo: 'enviar_crm' }) })
      if (caminho === '/prospeccao/jobs/20') return job({ id: 20, tipo: 'enviar_crm' })
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()
    await wrapper.find('tbody input[type="checkbox"]').setValue(true)
    await botao(wrapper, 'Descartar')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Enviando ao CRM')
    expect(botao(wrapper, 'Enviar ao CRM')!.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('exporta o CSV pela rota autenticada e troca de lista quando o id da rota muda', async () => {
    const download = vi.spyOn(api, 'download').mockResolvedValue()
    responder = respostasPadrao((_m, caminho) => {
      if (caminho === '/prospeccao/listas/4') return { lista: lista({ id: 4, nome: 'Padarias' }), jobs: [] }
      if (caminho.startsWith('/prospeccao/listas/4/itens?')) return { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
      return undefined
    })
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()
    await botao(wrapper, 'Exportar CSV')!.trigger('click')
    expect(download).toHaveBeenCalledWith('/prospeccao/listas/3/exportar', 'restaurantes-de-fortaleza.csv')

    rota.params = { id: '4' }
    await flushPromises()
    expect(wrapper.find('h1').text()).toBe('Padarias')
    expect(wrapper.text()).toContain('Lista vazia')
  })

  it('quem só vê a prospecção não tem as ações de escrita', async () => {
    loginGestor({ 'prospecting.view': true })
    responder = respostasPadrao()
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()
    expect(botao(wrapper, 'Enriquecer')).toBeUndefined()
    expect(botao(wrapper, 'Enviar ao CRM')).toBeUndefined()
    expect(botao(wrapper, 'Excluir')).toBeUndefined()
    expect(botao(wrapper, 'Exportar CSV')).toBeDefined()
  })

  it('lista inexistente mostra o aviso', async () => {
    responder = respostasPadrao((_m, caminho) => (caminho === '/prospeccao/listas/3' ? erro(404, { error: 'lista não encontrada' }) : undefined))
    const wrapper = mount(ProspectListDetailView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('Lista não encontrada')
  })
})
