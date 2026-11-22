// Configurações → Prospecção (docs/PROSPECCAO.md 8.3, 8.4 e 10.5).
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SettingsProspectingView from '../src/views/SettingsProspectingView.vue'
import { useToastStore } from '../src/stores/toast'

const config = {
  janela_inicio: 9,
  janela_fim: 18,
  sabado: false,
  max_tentativas: 6,
  intervalos_horas: [3, 24, 48, 72, 120],
  tentativas_dia: 2,
  retorno_padrao_horas: 48,
  meta_ligacoes_dia: 40,
  exigir_optin_whatsapp: true,
  incluir_sem_dono_na_fila: false,
  pipeline_id: null,
  stage_id: null,
  playbook_id: null,
  motivos_descarte: ['Sem interesse', 'Já tem fornecedor']
}

const pipelines = [
  {
    id: 7,
    name: 'Prospecção',
    position: 0,
    stages: [
      { id: 70, pipeline_id: 7, name: 'Reunião marcada', position: 0, probability: 20, is_won: false, is_lost: false },
      { id: 71, pipeline_id: 7, name: 'Ganho', position: 1, probability: 100, is_won: true, is_lost: false }
    ]
  }
]

const bloqueios = [
  { id: 1, tipo: 'telefone', valor: '85999990001', motivo: 'opt_out', canal: 'fila', observacao: '', criado_por_nome: 'Ana', criado_em: '2026-11-20T10:00:00Z' },
  { id: 2, tipo: 'cnpj', valor: '11222333000181', motivo: 'cliente', canal: '', observacao: 'já é cliente', criado_por_nome: '', criado_em: '2026-11-21T10:00:00Z' }
]

type Chamada = { url: string; method: string; body: any }
const chamadas: Chamada[] = []

function stubFetch(opts: { conflito?: boolean } = {}) {
  chamadas.length = 0
  return vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const u = String(url)
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : null
    chamadas.push({ url: u, method, body })
    let resp: unknown = []
    let status = 200
    if (u.includes('/settings/prospeccao')) resp = method === 'PUT' ? body : config
    else if (u.includes('/pipelines')) resp = pipelines
    else if (u.includes('/playbooks')) resp = [{ id: 3, name: 'Roteiro SDR', active: true }]
    else if (u.includes('/bloqueios/importar')) resp = { criados: 2, repetidos: 1, invalidos: 0 }
    else if (u.includes('/bloqueios') && method === 'POST') {
      if (opts.conflito) {
        status = 409
        resp = { error: 'já bloqueado' }
      } else resp = { id: 9, ...body }
    } else if (u.includes('/bloqueios') && method === 'DELETE') {
      status = 204
      resp = ''
    } else if (u.includes('/bloqueios')) {
      const q = new URL(u, 'http://x').searchParams.get('q') ?? ''
      const data = q ? bloqueios.filter((b) => b.valor.includes(q.replace(/\D/g, '')) || b.observacao.includes(q)) : bloqueios
      resp = { data, pagination: { page: 1, per_page: 25, total: data.length } }
    }
    return Promise.resolve({ ok: status < 400, status, text: () => Promise.resolve(status === 204 ? '' : JSON.stringify(resp)) })
  })
}

function montar() {
  return mount(SettingsProspectingView, { global: { stubs: { Teleport: true } } })
}

describe('SettingsProspectingView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useRealTimers()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('carrega a configuração e salva com intervalos lidos do texto e motivo novo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montar()
    await flushPromises()

    expect((wrapper.find('#intervalos').element as HTMLInputElement).value).toBe('3, 24, 48, 72, 120')
    expect(wrapper.findAll('[data-motivo]').length).toBe(2)

    await wrapper.find('#intervalos').setValue('2; 12 24h')
    await wrapper.find('[data-campo="novo-motivo"]').setValue('Sem orçamento')
    await wrapper.find('[data-campo="novo-motivo"]').trigger('keydown', { key: 'Enter' })
    await wrapper.find('#funil').setValue('7')
    await wrapper.find('#etapa').setValue('70')
    await wrapper.find('[data-bloco="config"]').trigger('submit')
    await flushPromises()

    const put = chamadas.find((c) => c.method === 'PUT')!
    expect(put.url).toContain('/settings/prospeccao')
    expect(put.body.intervalos_horas).toEqual([2, 12, 24])
    expect(put.body.motivos_descarte).toEqual(['Sem interesse', 'Já tem fornecedor', 'Sem orçamento'])
    expect(put.body.pipeline_id).toBe(7)
    expect(put.body.stage_id).toBe(70)
    expect(useToastStore().toasts.map((t) => t.message)).toContain('Configurações de prospecção salvas')
    wrapper.unmount()
  })

  it('não salva com janela invertida ou intervalos inválidos', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montar()
    await flushPromises()

    await wrapper.find('#janela-inicio').setValue('18')
    await wrapper.find('#janela-fim').setValue('9')
    await wrapper.find('#intervalos').setValue('abc')
    await wrapper.find('[data-bloco="config"]').trigger('submit')
    await flushPromises()

    expect(chamadas.some((c) => c.method === 'PUT')).toBe(false)
    const erros = wrapper.find('.erros').text()
    expect(erros).toContain('O início da janela precisa ser antes do fim.')
    expect(erros).toContain('Informe os intervalos entre tentativas')
    wrapper.unmount()
  })

  it('lista os bloqueios formatados e busca com atraso', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montar()
    await flushPromises()

    const linhas = wrapper.findAll('[data-bloqueio]')
    expect(linhas.length).toBe(2)
    expect(linhas[0].text()).toContain('(85) 99999-0001')
    expect(linhas[0].text()).toContain('Pediu para não ser contatado')
    expect(linhas[1].text()).toContain('11.222.333/0001-81')
    expect(linhas[1].text()).toContain('Sistema')

    await wrapper.find('input[type="search"]').setValue('99999')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()
    const busca = chamadas.filter((c) => c.method === 'GET' && c.url.includes('/bloqueios?')).at(-1)!
    expect(busca.url).toContain('q=99999')
    expect(wrapper.findAll('[data-bloqueio]').length).toBe(1)
    vi.useRealTimers()
    wrapper.unmount()
  })

  it('adiciona um bloqueio normalizando o valor e recusa valor inválido', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montar()
    await flushPromises()

    await wrapper.find('[data-acao="adicionar"]').trigger('click')
    await wrapper.find('#novo-valor').setValue('123')
    await wrapper.find('#form-bloqueio').trigger('submit')
    await flushPromises()
    expect(wrapper.find('#form-bloqueio').text()).toContain('Telefone inválido')
    expect(chamadas.some((c) => c.method === 'POST')).toBe(false)

    await wrapper.find('#novo-valor').setValue('+55 (85) 98888-7777')
    await wrapper.find('#novo-motivo').setValue('pedido_titular')
    await wrapper.find('#novo-obs').setValue('pediu por e-mail')
    await wrapper.find('#form-bloqueio').trigger('submit')
    await flushPromises()

    const post = chamadas.find((c) => c.method === 'POST')!
    expect(post.url).toContain('/bloqueios')
    expect(post.body).toEqual({ tipo: 'telefone', valor: '85988887777', motivo: 'pedido_titular', observacao: 'pediu por e-mail' })
    expect(wrapper.find('#form-bloqueio').exists()).toBe(false)
    expect(useToastStore().toasts.map((t) => t.message)).toContain('Bloqueio adicionado')
    wrapper.unmount()
  })

  it('avisa quando o valor já está bloqueado (409)', async () => {
    vi.stubGlobal('fetch', stubFetch({ conflito: true }))
    const wrapper = montar()
    await flushPromises()
    await wrapper.find('[data-acao="adicionar"]').trigger('click')
    await wrapper.find('#novo-tipo').setValue('email')
    await wrapper.find('#novo-valor').setValue('Pessoa@Empresa.com.br')
    await wrapper.find('#form-bloqueio').trigger('submit')
    await flushPromises()
    expect(chamadas.find((c) => c.method === 'POST')!.body.valor).toBe('pessoa@empresa.com.br')
    expect(wrapper.find('#form-bloqueio').text()).toContain('Esse valor já está bloqueado.')
    wrapper.unmount()
  })

  it('importa colando: conta válidos, inválidos e repetidos e envia só os válidos', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montar()
    await flushPromises()

    await wrapper.find('[data-acao="importar"]').trigger('click')
    await wrapper.find('#imp-texto').setValue('(85) 99999-0001\n85 3000-0001\n85999990001\n123\n')
    expect(wrapper.find('[data-resumo]').text().replace(/\s+/g, ' ')).toBe('2 válidos · 1 inválido · 1 repetido')

    await wrapper.find('#form-importar').trigger('submit')
    await flushPromises()
    const post = chamadas.find((c) => c.method === 'POST' && c.url.includes('/bloqueios/importar'))!
    expect(post.body).toEqual({ tipo: 'telefone', motivo: 'lista_externa', valores: ['85999990001', '8530000001'] })
    // Repetidos e inválidos do navegador somam aos do servidor.
    expect(wrapper.find('[data-resultado]').text()).toContain('2 criados · 2 já existiam · 1 inválido')
    wrapper.unmount()
  })

  it('recusa importar mais de 5.000 valores', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montar()
    await flushPromises()
    await wrapper.find('[data-acao="importar"]').trigger('click')
    await wrapper.find('#imp-tipo').setValue('email')
    const texto = Array.from({ length: 5001 }, (_, i) => `p${i}@ex.com.br`).join('\n')
    await wrapper.find('#imp-texto').setValue(texto)
    expect(wrapper.find('#form-importar').text()).toContain('São mais de 5.000 valores')
    const botao = wrapper.findAll('button[type="submit"]').find((b) => b.attributes('form') === 'form-importar')!
    expect(botao.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('remove um bloqueio depois de confirmar', async () => {
    vi.stubGlobal('fetch', stubFetch())
    vi.stubGlobal('confirm', vi.fn(() => true))
    const wrapper = montar()
    await flushPromises()

    await wrapper.findAll('[data-bloqueio]')[1].find('button').trigger('click')
    await flushPromises()
    const del = chamadas.find((c) => c.method === 'DELETE')!
    expect(del.url).toContain('/bloqueios/2')
    expect(useToastStore().toasts.map((t) => t.message)).toContain('Bloqueio removido')
    wrapper.unmount()
  })
})
