// Gerador de listas (10.1): filtros, contagem ao vivo, prévia, segmentos,
// gerar lista, colar CNPJs e os estados da plataforma.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import ProspectingView from '../src/views/ProspectingView.vue'
import {
  deFiltros,
  formVazio,
  paraFiltros,
  resumoFiltros,
  sugerirNome,
  tetoQuantidade,
  usadosNoPerfil
} from '../src/components/prospeccao/filtros'
import type { Contagem, ResumoPlataforma } from '../src/types-prospeccao'

const push = vi.fn()
const replace = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push, replace }),
  useRoute: () => ({ params: {}, query: {} })
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

/** Resposta de erro com status. */
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

function loginGestor(permissoes: Record<string, boolean> = { 'prospecting.view': true, 'prospecting.manage': true }) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana Silva', email: 'a@exemplo.com.br', role: 'manager', active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissoes
}

function resumo(parcial: Partial<ResumoPlataforma> = {}, base: Partial<ResumoPlataforma['base']> = {}): ResumoPlataforma {
  return {
    habilitada: true,
    motivo: '',
    cota: { mes: '2026-11', limite: 5000, usado: 1240, restante: 3760 },
    base: {
      pronta: true,
      amostra: false,
      referencia_cnpj: '2026-11',
      referencia_pgfn: '2026-T3',
      versao: 'v1',
      empresas: 28700000,
      carga: { estado: 'ociosa', percentual: 0, etapa: '', erro: '' },
      ...base
    },
    ...parcial
  }
}

const contagemCE: Contagem = { total: 12480, total_modo: 'exato', parciais: { com_celular: 7910, com_email: 4120, com_divida: null } }

function respostasPadrao(r: ResumoPlataforma, extra: Responder = () => undefined): Responder {
  return (metodo, caminho, corpo) => {
    const e = extra(metodo, caminho, corpo)
    if (e !== undefined) return e
    if (caminho === '/prospeccao') return r
    if (caminho.startsWith('/prospeccao/catalogos/ufs')) return { itens: [{ uf: 'CE', nome: 'Ceará', empresas: 400000 }, { uf: 'SP', nome: 'São Paulo', empresas: 6000000 }] }
    if (caminho.startsWith('/prospeccao/catalogos/')) return { itens: [] }
    if (caminho === '/prospeccao/contagem') return contagemCE
    if (caminho.startsWith('/prospeccao/segmentos')) return []
    if (caminho.startsWith('/prospeccao/listas')) return { data: [], pagination: { page: 1, per_page: 25, total: 0 } }
    return {}
  }
}

function botaoUf(wrapper: ReturnType<typeof mount>, uf: string) {
  return wrapper.findAll('button.chip.uf').find((b) => b.text() === uf)!
}

function botao(wrapper: ReturnType<typeof mount>, texto: string) {
  return wrapper.findAll('button').find((b) => b.text().trim() === texto)!
}

function noBody<T extends Element = HTMLElement>(seletor: string): T | null {
  return document.body.querySelector<T>(seletor)
}

function botaoNoBody(texto: string): HTMLButtonElement | undefined {
  return [...document.body.querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent?.trim() === texto)
}

describe('filtros do formulário', () => {
  it('converte o formulário em Filtros só com o que filtra', () => {
    const form = formVazio()
    expect(paraFiltros(form)).toEqual({ ocultar_contador: true })

    form.cnaes = [
      { codigo: '5611201', descricao: 'Restaurantes', nivel: 'subclasse' },
      { codigo: '47', descricao: 'Comércio varejista', nivel: 'divisao' }
    ]
    form.ufs = ['CE', 'SP']
    form.municipios = [{ codigo: 1389, nome: 'Fortaleza', uf: 'CE', empresas: 1 }]
    form.bairros = ['ALDEOTA']
    form.mei = 'sim' // sem incluir_pf não vai (422 no serviço)
    form.nome = 'ab' // menos de 3 letras não vai
    form.capital_min = 0
    const f = paraFiltros(form)
    expect(f.cnaes).toEqual([5611201])
    expect(f.cnae_prefixos).toEqual(['47'])
    // Com cidade, a busca vale só para ela.
    expect(f.municipios).toEqual([1389])
    expect(f.ufs).toBeUndefined()
    expect(f.bairros).toEqual(['ALDEOTA'])
    expect(f.mei).toBeUndefined()
    expect(f.nome).toBeUndefined()
    expect(f.capital_min).toBeUndefined()

    form.incluir_pf = true
    expect(paraFiltros(form)).toMatchObject({ mei: 'sim', incluir_pf: true })
  })

  it('bairro e CEP só valem com cidade ou CNAE', () => {
    const form = formVazio()
    form.ufs = ['CE']
    form.ceps = ['60160']
    expect(paraFiltros(form).ceps).toBeUndefined()
    form.cnaes = [{ codigo: '56', descricao: '', nivel: 'divisao' }]
    expect(paraFiltros(form).ceps).toEqual(['60160'])
  })

  it('reconstrói o formulário de um segmento salvo com os rótulos do catálogo', () => {
    const form = deFiltros(
      { cnaes: [5611201], cnae_prefixos: ['47'], municipios: [1389, 7107], abertura_ultimos_dias: 90, portes: ['me'] },
      'recentes',
      { cnaes: [{ codigo: '5611201', descricao: 'Restaurantes', nivel: 'subclasse' }], municipios: [{ codigo: 1389, nome: 'Fortaleza', uf: 'CE', empresas: 10 }] }
    )
    expect(form.ordem).toBe('recentes')
    expect(form.cnaes.map((c) => [c.codigo, c.nivel, c.descricao])).toEqual([
      ['47', 'divisao', ''],
      ['5611201', 'subclasse', 'Restaurantes']
    ])
    expect(form.municipios[0].nome).toBe('Fortaleza')
    // Sem rótulo, o chip fica com o código.
    expect(form.municipios[1].nome).toBe('Cidade 7107')
    expect(form.ufs).toEqual(['CE'])
    expect(form.abertura_modo).toBe('dias')
    expect(usadosNoPerfil(form)).toBe(2)
    expect(paraFiltros(form)).toMatchObject({ cnaes: [5611201], cnae_prefixos: ['47'], municipios: [1389, 7107], abertura_ultimos_dias: 90 })
  })

  it('limita a quantidade ao total exato e à cota; sem total exato avisa que pode sair menor', () => {
    const cota = { mes: '2026-11', limite: 5000, usado: 1240, restante: 3760 }
    expect(tetoQuantidade({ total: 800, total_modo: 'exato', parciais: null }, cota)).toEqual({ maximo: 800, podeSairMenor: false })
    expect(tetoQuantidade({ total: 10000, total_modo: 'teto', parciais: null }, cota)).toEqual({ maximo: 3760, podeSairMenor: true })
    expect(tetoQuantidade({ total: 90000, total_modo: 'exato', parciais: null }, { ...cota, restante: 50000 }).maximo).toBe(20000)
    expect(tetoQuantidade(contagemCE, { ...cota, restante: 0 }).maximo).toBe(0)
  })

  it('sugere nome e resume os filtros', () => {
    const form = formVazio()
    form.cnaes = [{ codigo: '5611201', descricao: 'Restaurantes', nivel: 'subclasse' }]
    form.municipios = [{ codigo: 1389, nome: 'Fortaleza', uf: 'CE', empresas: 1 }]
    expect(sugerirNome(form)).toBe('Restaurantes · Fortaleza/CE')
    expect(resumoFiltros({ ufs: ['CE'], cnaes: [5611201], com_celular: true })).toBe('CNAE 5611-2/01 · CE · com celular')
  })
})

describe('tela Gerar listas', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginGestor()
    chamadas = []
    push.mockReset()
    instalarFetch()
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('mostra cota, referência da base e pede um filtro obrigatório antes de contar', async () => {
    responder = respostasPadrao(resumo())
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('1.240 de 5.000 leads usados em novembro')
    expect(wrapper.text()).toContain('Receita 2026-11 · PGFN 2026-T3')
    expect(wrapper.text()).toContain('Escolha um segmento (CNAE) ou um lugar (UF ou cidade)')
    expect(wrapper.text()).toContain('Nova lista')
    expect(wrapper.text()).toContain('Listas de prospecção')
    expect(wrapper.text()).toContain('Segmentos salvos')
    expect(chamadas.some((c) => c.caminho === '/prospeccao/contagem')).toBe(false)
    expect(botao(wrapper, 'Ver prévia').attributes('disabled')).toBeDefined()
    expect(botao(wrapper, 'Gerar lista').attributes('disabled')).toBeDefined()
    expect(botao(wrapper, 'Salvar segmento').attributes('disabled')).toBeDefined()
    // Blocos 3 e 4 começam recolhidos; bloco 4 tem "ocultar contador" ligado por padrão.
    expect(wrapper.text()).toContain('1 em uso')
    expect(wrapper.find('#ff-ordem').exists()).toBe(false)
  })

  it('conta ao vivo 400 ms depois da última alteração e mostra as parciais', async () => {
    vi.useFakeTimers()
    responder = respostasPadrao(resumo())
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()

    await botaoUf(wrapper, 'CE').trigger('click')
    await vi.advanceTimersByTimeAsync(399)
    expect(chamadas.filter((c) => c.caminho === '/prospeccao/contagem')).toHaveLength(0)
    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()

    const contagens = chamadas.filter((c) => c.caminho === '/prospeccao/contagem')
    expect(contagens).toHaveLength(1)
    expect(contagens[0].corpo).toEqual({ filtros: { ufs: ['CE'], ocultar_contador: true } })
    const rodape = wrapper.find('.rodape-contagem').text()
    expect(rodape).toContain('12.480 empresas')
    expect(rodape).toContain('7.910 com celular')
    expect(rodape).toContain('4.120 com e-mail')
    // Parcial nula é omitida.
    expect(rodape).not.toContain('dívida')
    expect(botao(wrapper, 'Gerar lista').attributes('disabled')).toBeUndefined()
  })

  it('descarta a resposta atrasada da contagem', async () => {
    vi.useFakeTimers()
    const pendentes: { corpo: any; resolver: (v: unknown) => void }[] = []
    responder = respostasPadrao(resumo(), (_m, caminho, corpo) => {
      if (caminho === '/prospeccao/contagem') return new Promise((resolver) => pendentes.push({ corpo, resolver }))
      return undefined
    })
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()

    await botaoUf(wrapper, 'CE').trigger('click')
    await vi.advanceTimersByTimeAsync(400)
    await botaoUf(wrapper, 'SP').trigger('click')
    await vi.advanceTimersByTimeAsync(400)
    expect(pendentes).toHaveLength(2)
    expect(pendentes[1].corpo.filtros.ufs).toEqual(['CE', 'SP'])

    pendentes[1].resolver({ total: 10000, total_modo: 'teto', total_limite_superior: 48200, parciais: null })
    await flushPromises()
    pendentes[0].resolver({ total: 900, total_modo: 'exato', parciais: null })
    await flushPromises()

    const rodape = wrapper.find('.rodape-contagem').text()
    expect(rodape).toContain('mais de 10.000, até 48.200 empresas')
    expect(rodape).not.toContain('900')
  })

  it('gera a lista com nome, quantidade limitada e exclusões, e abre o detalhe', async () => {
    vi.useFakeTimers()
    responder = respostasPadrao(resumo(), (metodo, caminho) => {
      if (metodo === 'POST' && caminho === '/prospeccao/listas') return { lista: { id: 42, nome: 'CE' }, job: { id: 7, status: 'pendente' } }
      if (caminho === '/prospeccao/contagem') return { total: 800, total_modo: 'exato', parciais: null }
      return undefined
    })
    const wrapper = mount(ProspectingView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await botaoUf(wrapper, 'CE').trigger('click')
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()

    await botao(wrapper, 'Gerar lista').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('Esta lista usa')
    expect(document.body.textContent).toContain('dos seus')
    expect(document.body.textContent).toContain('3.760')
    const quantidade = noBody<HTMLInputElement>('#gl-quantidade')!
    expect(quantidade.max).toBe('800')
    quantidade.value = '300'
    quantidade.dispatchEvent(new Event('input'))
    const pularCrm = [...document.body.querySelectorAll<HTMLInputElement>('.exclusoes input')][1]
    pularCrm.checked = false
    pularCrm.dispatchEvent(new Event('change'))
    await flushPromises()
    noBody<HTMLFormElement>('#form-gerar-lista')!.dispatchEvent(new Event('submit'))
    await flushPromises()

    const criar = chamadas.find((c) => c.metodo === 'POST' && c.caminho === '/prospeccao/listas')!
    expect(criar.corpo).toMatchObject({
      nome: 'CE',
      origem: 'base',
      filtros: { ufs: ['CE'], ocultar_contador: true },
      ordem: 'relevancia',
      quantidade: 300,
      excluir_no_crm: true,
      excluir_em_listas: false
    })
    expect(push).toHaveBeenCalledWith({ name: 'prospect-list', params: { id: '42' } })
    wrapper.unmount()
  })

  it('mostra a prévia sem contatos e avisa quando a ordem pedida não foi aplicada', async () => {
    vi.useFakeTimers()
    responder = respostasPadrao(resumo(), (_m, caminho) => {
      if (caminho === '/prospeccao/previa') {
        return {
          ordem_aplicada: 'relevancia',
          itens: [
            {
              cnpj: '11222333000181',
              razao_social: 'PADARIA EXEMPLO LTDA',
              nome_fantasia: 'Pão Bom',
              cnae: { codigo: 1091102, descricao: 'Padaria' },
              porte: 'me',
              abertura: '2020-01-10',
              capital: 10000,
              municipio: 'FORTALEZA',
              uf: 'CE',
              pessoa_fisica: false,
              mei: false,
              tem_celular: true,
              tem_telefone: true,
              tem_email: false,
              tem_divida: true,
              socios_qtd: 2,
              qualidade: 60
            }
          ]
        }
      }
      return undefined
    })
    const wrapper = mount(ProspectingView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await botaoUf(wrapper, 'CE').trigger('click')
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()

    // Escolhe outra ordem no bloco 4 e abre a prévia.
    await wrapper.findAll('button.bloco-head')[1].trigger('click')
    await wrapper.find('#ff-ordem').setValue('recentes')
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()
    await botao(wrapper, 'Ver prévia').trigger('click')
    await flushPromises()

    const previa = chamadas.find((c) => c.caminho === '/prospeccao/previa')!
    expect(previa.corpo.ordem).toBe('recentes')
    const texto = document.body.textContent ?? ''
    expect(texto).toContain('Pão Bom')
    expect(texto).toContain('11.222.333/0001-81')
    expect(texto).toContain('Muitas empresas para esta ordenação; mostrando os melhores contatos')
    wrapper.unmount()
  })

  it('carrega um segmento salvo com os rótulos e conta com segmento_id', async () => {
    vi.useFakeTimers()
    const segmento = {
      id: 5,
      nome: 'Restaurantes de Fortaleza',
      filtros: { cnaes: [5611201], municipios: [1389] },
      ordem: 'relevancia',
      compartilhado: true,
      ultima_contagem: 1200,
      ultima_contagem_em: '2026-11-01T10:00:00Z',
      criado_por: 1,
      criado_por_nome: 'Ana Silva',
      criado_em: '',
      atualizado_em: ''
    }
    responder = respostasPadrao(resumo(), (_m, caminho) => {
      if (caminho === '/prospeccao/segmentos') return [segmento]
      if (caminho.startsWith('/prospeccao/catalogos/cnaes?codigos=')) return { itens: [{ codigo: '5611201', descricao: 'Restaurantes e similares', nivel: 'subclasse' }] }
      if (caminho.startsWith('/prospeccao/catalogos/municipios?codigos=')) return { itens: [{ codigo: 1389, nome: 'Fortaleza', uf: 'CE', empresas: 10 }] }
      return undefined
    })
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()

    await botao(wrapper, 'Segmentos salvos').trigger('click')
    expect(wrapper.text()).toContain('Restaurantes de Fortaleza')
    expect(wrapper.text()).toContain('1.200')
    await botao(wrapper, 'Usar').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()

    expect(chamadas.some((c) => c.caminho === '/prospeccao/catalogos/cnaes?codigos=5611201')).toBe(true)
    expect(wrapper.text()).toContain('Restaurantes e similares')
    expect(wrapper.text()).toContain('Fortaleza/CE')
    expect(wrapper.text()).toContain('Segmento: Restaurantes de Fortaleza')
    const contagem = chamadas.filter((c) => c.caminho === '/prospeccao/contagem').pop()!
    expect(contagem.corpo.segmento_id).toBe(5)
    expect(contagem.corpo.filtros).toMatchObject({ cnaes: [5611201], municipios: [1389] })

    // Mexer no filtro deixa de atualizar o segmento (desmarcar o CE tira Fortaleza).
    await botaoUf(wrapper, 'CE').trigger('click')
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()
    expect(chamadas.filter((c) => c.caminho === '/prospeccao/contagem').pop()!.corpo.segmento_id).toBeUndefined()
    expect(wrapper.text()).toContain('(filtros alterados)')
  })

  it('MEI = sim liga a inclusão de pessoa física, com o aviso da LGPD', async () => {
    responder = respostasPadrao(resumo())
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()
    await wrapper.findAll('button.bloco-head')[0].trigger('click')
    expect(wrapper.text()).toContain('a LGPD se aplica ao contato')
    await wrapper.find('#ff-mei').setValue('sim')
    await flushPromises()
    const pf = wrapper.find('.chave.pf input').element as HTMLInputElement
    expect(pf.checked).toBe(true)
  })

  it('cola CNPJs, mostra válidos, inválidos e repetidos e cria lista de origem cnpjs', async () => {
    responder = respostasPadrao(resumo(), (metodo, caminho) => {
      if (metodo === 'POST' && caminho === '/prospeccao/listas') return { lista: { id: 9, nome: 'Feira' }, job: { id: 1 } }
      return undefined
    })
    const wrapper = mount(ProspectingView, { global: { stubs }, attachTo: document.body })
    await flushPromises()
    await botao(wrapper, 'Colar CNPJs').trigger('click')
    await flushPromises()

    const texto = noBody<HTMLTextAreaElement>('#cc-texto')!
    texto.value = '11.222.333/0001-81\n11222333000181\n12345678000100'
    texto.dispatchEvent(new Event('input'))
    const nome = noBody<HTMLInputElement>('#cc-nome')!
    nome.value = 'Feira'
    nome.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(document.body.textContent).toContain('1 válidos, 1 inválidos, 1 repetidos')

    noBody<HTMLFormElement>('#form-colar-cnpjs')!.dispatchEvent(new Event('submit'))
    await flushPromises()
    const criar = chamadas.find((c) => c.metodo === 'POST' && c.caminho === '/prospeccao/listas')!
    expect(criar.corpo).toMatchObject({ nome: 'Feira', origem: 'cnpjs', filtros: { cnpjs: ['11222333000181'] }, quantidade: 1 })
    expect(push).toHaveBeenCalledWith({ name: 'prospect-list', params: { id: '9' } })
    wrapper.unmount()
  })

  it('com a plataforma desabilitada mostra só o motivo', async () => {
    responder = respostasPadrao(resumo({ habilitada: false, motivo: 'seu plano não inclui prospecção' }))
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('seu plano não inclui prospecção')
    expect(wrapper.text()).not.toContain('Segmentos salvos')
    expect(wrapper.find('.filtros').exists()).toBe(false)
  })

  it('com a base em carga desabilita o formulário e confere de novo a cada 30 s', async () => {
    vi.useFakeTimers()
    let r = resumo({}, { pronta: false, carga: { estado: 'baixando', percentual: 37, etapa: 'Estabelecimentos3', erro: '' } })
    responder = (m, c, corpo) => respostasPadrao(r)(m, c, corpo)
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Base em carga, 37%')
    expect((wrapper.find('fieldset.filtros').element as HTMLFieldSetElement).disabled).toBe(true)
    expect(chamadas.filter((c) => c.caminho === '/prospeccao')).toHaveLength(1)

    r = resumo()
    await vi.advanceTimersByTimeAsync(30000)
    await flushPromises()
    expect(chamadas.filter((c) => c.caminho === '/prospeccao')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('Base em carga')
    expect((wrapper.find('fieldset.filtros').element as HTMLFieldSetElement).disabled).toBe(false)
  })

  it('base pronta em atualização só avisa; base de demonstração mostra a faixa', async () => {
    responder = respostasPadrao(resumo({}, { amostra: true, carga: { estado: 'baixando', percentual: 12, etapa: '', erro: '' } }))
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('Dados de demonstração')
    expect(wrapper.text()).toContain('Atualização em andamento')
    expect((wrapper.find('fieldset.filtros').element as HTMLFieldSetElement).disabled).toBe(false)
  })

  it('cota esgotada trava "Gerar lista" mas deixa contar', async () => {
    vi.useFakeTimers()
    responder = respostasPadrao(resumo({ cota: { mes: '2026-11', limite: 1000, usado: 1000, restante: 0 } }))
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()
    await botaoUf(wrapper, 'CE').trigger('click')
    await vi.advanceTimersByTimeAsync(400)
    await flushPromises()
    expect(wrapper.text()).toContain('Cota do mês esgotada')
    expect(wrapper.text()).toContain('12.480 empresas')
    const gerar = botao(wrapper, 'Cota esgotada')
    expect(gerar.attributes('disabled')).toBeDefined()
  })

  it('quem só vê não tem os botões de gerar, salvar e colar', async () => {
    loginGestor({ 'prospecting.view': true })
    responder = respostasPadrao(resumo())
    const wrapper = mount(ProspectingView, { global: { stubs } })
    await flushPromises()
    expect(botao(wrapper, 'Gerar lista')).toBeUndefined()
    expect(botao(wrapper, 'Salvar segmento')).toBeUndefined()
    expect(botao(wrapper, 'Colar CNPJs')).toBeUndefined()
    expect(botao(wrapper, 'Ver prévia')).toBeDefined()
  })
})
