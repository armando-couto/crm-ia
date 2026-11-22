// Fundação da prospecção: helpers de CNPJ e telefone, cliente de API, rótulos,
// componentes de interface, composables, rotas e menu.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, effectScope, h, nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { routerKey } from 'vue-router'
import { api, ApiError, foiAbortado, setToken } from '../src/api'
import { cnpjValido, ehCNPJ, ehCPF, formatarCNPJ, formatarDocumento, mascaraCNPJ, normalizarCNPJ, separarCNPJs } from '../src/cnpj'
import * as formato from '../src/format'
import {
  agruparTelefones,
  comNonoDigito,
  formatarTelefone,
  linkTel,
  linkWhatsApp,
  normalizarTelefone,
  opcoesDiscagem,
  podeLigar,
  podeWhatsApp,
  telefonePrincipal,
  telefoneValido,
  tipoPeloNumero
} from '../src/telefone'
import {
  CONFIG_PROSPECCAO_PADRAO,
  RESULTADOS,
  estadoItemLabels,
  jobAtivo,
  statusJobLabels,
  temFiltroObrigatorio,
  type Job,
  type Telefone
} from '../src/types-prospeccao'
import { useAuthStore } from '../src/stores/auth'
import { router } from '../src/router'
import AppShell from '../src/components/AppShell.vue'
import SettingsLayout from '../src/views/SettingsLayout.vue'
import CnpjField from '../src/components/intel/CnpjField.vue'
import PaginationBar from '../src/components/ui/PaginationBar.vue'
import ProgressBar from '../src/components/ui/ProgressBar.vue'
import StepperSteps from '../src/components/ui/StepperSteps.vue'
import SideDrawer from '../src/components/ui/SideDrawer.vue'
import { useJobPolling } from '../src/composables/useJobPolling'
import { useConsultaAdiada } from '../src/composables/useConsultaAdiada'
import { useModoFoco } from '../src/composables/useModoFoco'
import { focoEmCampo, useAtalhos } from '../src/composables/useAtalhos'

function resposta(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (k: string) => headers[k] ?? null },
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body))
  }
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana Silva', email: 'a@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
  return auth
}

function job(parcial: Partial<Job> = {}): Job {
  return {
    id: 9,
    tipo: 'gerar_lista',
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

describe('CNPJ alfanumérico', () => {
  it('normaliza para maiúsculas, só [0-9A-Z] e 14 caracteres', () => {
    expect(normalizarCNPJ(' 12.abc.345/01de-35 ')).toBe('12ABC34501DE35')
    expect(normalizarCNPJ('11.222.333/0001-81999')).toBe('11222333000181')
    expect(normalizarCNPJ(null)).toBe('')
  })

  it('valida os dígitos verificadores pela regra alfanumérica (ASCII − 48)', () => {
    // Exemplo oficial da Receita Federal para o CNPJ alfanumérico.
    expect(cnpjValido('12.ABC.345/01DE-35')).toBe(true)
    expect(cnpjValido('12abc34501de35')).toBe(true)
    expect(cnpjValido('12.ABC.345/01DE-34')).toBe(false)
    // Letra nos dígitos verificadores nunca vale.
    expect(cnpjValido('12ABC34501DE3A')).toBe(false)
    // O numérico continua valendo como antes.
    expect(cnpjValido('11.222.333/0001-81')).toBe(true)
    expect(cnpjValido('11.222.333/0001-80')).toBe(false)
    expect(cnpjValido('AAAAAAAAAAAAAA')).toBe(false)
    expect(cnpjValido('00000000000000')).toBe(false)
  })

  it('aplica a máscara com letras, progressivamente', () => {
    expect(formatarCNPJ('12abc34501de35')).toBe('12.ABC.345/01DE-35')
    expect(mascaraCNPJ('12abc3')).toBe('12.ABC.3')
    expect(formatarDocumento('12abc345')).toBe('12.ABC.345')
    expect(formatarDocumento('52998224725')).toBe('529.982.247-25')
  })

  it('ehCNPJ aceita o alfanumérico e ehCPF não confunde letras com CPF', () => {
    expect(ehCNPJ('12.ABC.345/01DE-35')).toBe(true)
    expect(ehCNPJ('12ABC34501DE3X')).toBe(false)
    expect(ehCPF('529.982.247-25')).toBe(true)
    expect(ehCPF('5299822472A5')).toBe(false)
  })

  it('separarCNPJs conta válidos, inválidos e repetidos de um texto colado', () => {
    const r = separarCNPJs('11.222.333/0001-81\n12.ABC.345/01DE-35; 11222333000181,\t123 \n 11.222.333/0001-80')
    expect(r.validos).toEqual(['11222333000181', '12ABC34501DE35'])
    expect(r.repetidos).toBe(1)
    expect(r.invalidos).toEqual(['123', '11.222.333/0001-80'])
    expect(separarCNPJs('')).toEqual({ validos: [], invalidos: [], repetidos: 0 })
  })

  it('format.ts reexporta os helpers de CNPJ pedidos pelo contrato', () => {
    expect(formato.normalizarCNPJ).toBe(normalizarCNPJ)
    expect(formato.cnpjValido).toBe(cnpjValido)
    expect(formato.mascaraCNPJ('11222333000181')).toBe('11.222.333/0001-81')
  })

  it('o campo de CNPJ aceita letras e dispara a consulta com o CNPJ normalizado', async () => {
    const wrapper = mount(CnpjField, { props: { modelValue: '' }, global: { stubs: { 'router-link': true } } })
    const input = wrapper.find('input')
    await input.setValue('12abc34501de35')

    expect(wrapper.emitted('update:modelValue')!.at(-1)).toEqual(['12.ABC.345/01DE-35'])
    expect(wrapper.emitted('lookup')!.at(-1)).toEqual(['12ABC34501DE35'])
  })
})

describe('telefone', () => {
  it('normaliza tirando máscara, DDI e zero de operadora', () => {
    expect(normalizarTelefone('+55 (85) 99999-0001')).toBe('85999990001')
    expect(normalizarTelefone('0 85 3000-0001')).toBe('8530000001')
    expect(normalizarTelefone('5585999990001')).toBe('85999990001')
    expect(normalizarTelefone(null)).toBe('')
  })

  it('formata celular e fixo', () => {
    expect(formatarTelefone('85999990001')).toBe('(85) 99999-0001')
    expect(formatarTelefone('8530000001')).toBe('(85) 3000-0001')
    expect(formatarTelefone('30000001')).toBe('3000-0001')
    expect(formatarTelefone('ramal 12')).toBe('ramal 12')
  })

  it('valida e deduz o tipo pelo tamanho', () => {
    expect(telefoneValido('(85) 99999-0001')).toBe(true)
    expect(telefoneValido('8530000001')).toBe(true)
    expect(telefoneValido('85899990001')).toBe(false)
    expect(telefoneValido('8500000000')).toBe(false)
    expect(telefoneValido('999990001')).toBe(false)
    expect(tipoPeloNumero('85999990001')).toBe('celular')
    expect(tipoPeloNumero('8530000001')).toBe('fixo')
    expect(tipoPeloNumero('123')).toBe('indefinido')
  })

  it('monta os links tel: e wa.me com o DDI 55', () => {
    expect(linkTel('(85) 99999-0001')).toBe('tel:+5585999990001')
    expect(linkTel('')).toBe('')
    expect(linkWhatsApp('85999990001')).toBe('https://wa.me/5585999990001')
    expect(linkWhatsApp('85999990001', 'Olá, tudo bem?')).toBe('https://wa.me/5585999990001?text=Ol%C3%A1%2C%20tudo%20bem%3F')
    // Fixo e indefinido (10 dígitos) não têm WhatsApp.
    expect(linkWhatsApp('8530000001')).toBe('')
  })

  it('telefone indefinido oferece discar sem e com o 9', () => {
    expect(comNonoDigito('1140000001')).toBe('11940000001')
    const opcoes = opcoesDiscagem({ numero: '1140000001', tipo: 'indefinido' })
    expect(opcoes.map((o) => o.href)).toEqual(['tel:+551140000001', 'tel:+5511940000001'])
    expect(opcoesDiscagem({ numero: '85999990001', tipo: 'celular' })).toHaveLength(1)
    expect(opcoesDiscagem({ numero: '', tipo: 'fixo' })).toEqual([])
  })

  it('WhatsApp respeita opt-in, tipo, invalidez e base de demonstração', () => {
    const cel: Telefone = { numero: '85999990001', tipo: 'celular' }
    expect(podeWhatsApp(cel)).toBe(true)
    expect(podeWhatsApp(cel, { exigirOptin: true, optin: false })).toBe(false)
    expect(podeWhatsApp(cel, { exigirOptin: true, optin: true })).toBe(true)
    expect(podeWhatsApp(cel, { exigirOptin: false, optin: false })).toBe(true)
    expect(podeWhatsApp(cel, { amostra: true })).toBe(false)
    expect(podeWhatsApp({ ...cel, invalido: true })).toBe(false)
    expect(podeWhatsApp({ ...cel, whatsapp: false })).toBe(false)
    expect(podeWhatsApp({ numero: '1140000001', tipo: 'indefinido' })).toBe(false)
    expect(podeLigar(cel)).toBe(true)
    expect(podeLigar(cel, { amostra: true })).toBe(false)
    expect(podeLigar({ ...cel, invalido: true })).toBe(false)
  })

  it('agrupa por pessoa na ordem recebida, com o cargo vindo dos decisores', () => {
    const telefones: Telefone[] = [
      { numero: '85999990001', tipo: 'celular', pessoa: 'João Sá' },
      { numero: '8530000001', tipo: 'fixo' },
      { numero: '85999990002', tipo: 'celular', pessoa: 'joao sa', invalido: true },
      { numero: '85999990003', tipo: 'celular', pessoa: '' }
    ]
    const grupos = agruparTelefones(telefones, [{ nome: 'JOÃO SÁ', cargo: 'Sócio-administrador' }])

    expect(grupos.map((g) => g.pessoa)).toEqual(['João Sá', 'Empresa'])
    expect(grupos[0].cargo).toBe('Sócio-administrador')
    expect(grupos[0].telefones.map((t) => t.numero)).toEqual(['85999990001', '85999990002'])
    expect(grupos[1].telefones.map((t) => t.numero)).toEqual(['8530000001', '85999990003'])
    expect(agruparTelefones(null)).toEqual([])
  })

  it('telefonePrincipal ignora os inválidos', () => {
    expect(telefonePrincipal([{ numero: '1', tipo: 'fixo', invalido: true }, { numero: '2', tipo: 'fixo' }, { numero: '3', tipo: 'fixo', principal: true }])?.numero).toBe('3')
    expect(telefonePrincipal([{ numero: '1', tipo: 'fixo', invalido: true }])).toBeNull()
  })
})

describe('rótulos e formatos', () => {
  it('tem rótulo para todo resultado, estado e urgência', () => {
    for (const { resultado } of RESULTADOS) expect(formato.resultadoLabels[resultado]).toBeTruthy()
    expect(RESULTADOS.map((r) => r.tecla).join('')).toBe('1234567890')
    expect(formato.resultadoLabels.reuniao).toBe('Reunião marcada')
    expect(formato.estadoLeadLabels.tentando).toBe('Tentando contato')
    expect(formato.urgenciaLabels.nenhuma).toBe('Sem próxima ação')
    expect(formato.urgenciaTons).toMatchObject({ atrasado: 'red', hoje: 'green', futuro: 'gray', nenhuma: 'amber' })
    expect(estadoItemLabels.duplicado).toBe('Já no CRM')
    expect(statusJobLabels.rodando).toBe('Em andamento')
  })

  it('motivoDescarteLabel traduz os códigos do sistema e mantém o texto livre', () => {
    expect(formato.motivoDescarteLabel('sem_telefone')).toBe('Sem telefone válido')
    expect(formato.motivoDescarteLabel('opt_out')).toBe('Pediu para não ligar')
    expect(formato.motivoDescarteLabel('sem_contato')).toBe('Sem contato após as tentativas')
    expect(formato.motivoDescarteLabel('Já tem fornecedor')).toBe('Já tem fornecedor')
    expect(formato.motivoDescarteLabel('')).toBe('')
  })

  it('formata contagem conforme total_modo', () => {
    expect(formato.formatContagem({ total: 12480, total_modo: 'exato' })).toBe('12.480')
    expect(formato.formatContagem({ total: 10000, total_modo: 'teto' })).toBe('mais de 10.000')
    expect(formato.formatContagem({ total: 10000, total_modo: 'teto', total_limite_superior: 48200 })).toBe('mais de 10.000, até 48.200')
    expect(formato.formatContagem(null)).toBe('—')
  })

  it('formata centavos, duração e competência', () => {
    expect(formato.formatCentavos(8050).replace(/ /g, ' ')).toBe('R$ 80,50')
    expect(formato.formatDuracao(75)).toBe('1:15')
    expect(formato.formatDuracao(3725)).toBe('1:02:05')
    expect(formato.formatDuracao(null)).toBe('0:00')
    expect(formato.formatCompetencia('2026-11')).toBe('novembro')
    expect(formato.formatNumber(5000)).toBe('5.000')
  })

  it('urgenciaDe calcula o semáforo pelo instante', () => {
    const agora = new Date(2026, 10, 22, 15, 0, 0)
    expect(formato.urgenciaDe(null, agora)).toBe('nenhuma')
    expect(formato.urgenciaDe(new Date(2026, 10, 21, 23, 0).toISOString(), agora)).toBe('atrasado')
    expect(formato.urgenciaDe(new Date(2026, 10, 22, 9, 0).toISOString(), agora)).toBe('hoje')
    expect(formato.urgenciaDe(new Date(2026, 10, 23, 0, 0).toISOString(), agora)).toBe('futuro')
  })

  it('filtros e padrões do contrato', () => {
    expect(temFiltroObrigatorio({})).toBe(false)
    expect(temFiltroObrigatorio({ ufs: [], portes: ['me'] })).toBe(false)
    expect(temFiltroObrigatorio({ cnae_prefixos: ['56'] })).toBe(true)
    expect(temFiltroObrigatorio({ cnpjs: ['11222333000181'] })).toBe(true)
    expect(CONFIG_PROSPECCAO_PADRAO.intervalos_horas).toEqual([3, 24, 48, 72, 120])
    expect(jobAtivo(job())).toBe(true)
    expect(jobAtivo(job({ status: 'concluido' }))).toBe(false)
    expect(jobAtivo(null)).toBe(false)
  })
})

describe('cliente de API', () => {
  beforeEach(() => setToken(null))
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('ApiError guarda o corpo da resposta e o código', async () => {
    const corpo = { error: 'cota mensal de prospecção esgotada', codigo: 'cota_esgotada', cota: { mes: '2026-11', limite: 1000, usado: 1000, restante: 0 } }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(resposta(corpo, 429)))

    const erro = (await api.post('/prospeccao/listas', {}).catch((e) => e)) as ApiError

    expect(erro).toBeInstanceOf(ApiError)
    expect(erro.status).toBe(429)
    expect(erro.message).toBe('cota mensal de prospecção esgotada')
    expect(erro.data).toEqual(corpo)
    expect(erro.codigo).toBe('cota_esgotada')
    expect(erro.data.cota.restante).toBe(0)
  })

  it('lê Retry-After do cabeçalho ou de tentar_em_segundos', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(resposta({ error: 'x', codigo: 'muitas_requisicoes' }, 429, { 'Retry-After': '7' })))
    expect(((await api.get('/a').catch((e) => e)) as ApiError).retryAfter).toBe(7)

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(resposta({ error: 'x', codigo: 'muitas_requisicoes', tentar_em_segundos: 3 }, 429)))
    expect(((await api.get('/a').catch((e) => e)) as ApiError).retryAfter).toBe(3)
  })

  it('erro sem corpo JSON fica com data nula e sem código', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 502, text: () => Promise.resolve('<html>') }))
    const erro = (await api.get('/a').catch((e) => e)) as ApiError
    expect(erro.data).toBeNull()
    expect(erro.codigo).toBe('')
    expect(erro.message).toBe('erro 502')
  })

  it('repassa signal e keepalive só quando pedidos', async () => {
    const fetchMock = vi.fn().mockResolvedValue(resposta({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)
    const controle = new AbortController()

    await api.get('/leads')
    await api.post('/prospeccao/contagem', { filtros: {} }, { signal: controle.signal })
    await api.post('/leads/4/liberar', undefined, { keepalive: true })

    expect('signal' in fetchMock.mock.calls[0][1]).toBe(false)
    expect('keepalive' in fetchMock.mock.calls[0][1]).toBe(false)
    expect(fetchMock.mock.calls[1][1].signal).toBe(controle.signal)
    expect(fetchMock.mock.calls[2][1].keepalive).toBe(true)
  })

  it('chamada cancelada rejeita com AbortError, reconhecido por foiAbortado', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_u: string, o: any) =>
        new Promise((_ok, falha) => o.signal.addEventListener('abort', () => falha(new DOMException('cancelada', 'AbortError'))))
      )
    )
    const controle = new AbortController()
    const promessa = api.get('/lento', { signal: controle.signal }).catch((e) => e)
    controle.abort()

    const erro = await promessa
    expect(foiAbortado(erro)).toBe(true)
    expect(foiAbortado(new ApiError(500, 'x'))).toBe(false)
  })
})

describe('componentes de interface', () => {
  it('PaginationBar mostra o resumo, as páginas com saltos e emite a troca', async () => {
    const wrapper = mount(PaginationBar, { props: { page: 5, perPage: 25, total: 1300, rotulo: 'leads' } })

    expect(wrapper.find('.resumo').text()).toBe('101–125 de 1.300 leads')
    expect(wrapper.findAll('.pg:not(.anterior):not(.proxima)').map((b) => b.text())).toEqual(['1', '4', '5', '6', '52'])
    expect(wrapper.findAll('.salto')).toHaveLength(2)
    expect(wrapper.find('.pg.atual').text()).toBe('5')

    await wrapper.find('.pg.proxima').trigger('click')
    await wrapper.find('.pg.anterior').trigger('click')
    await wrapper.find('.pg.atual').trigger('click')
    expect(wrapper.emitted('update:page')).toEqual([[6], [4]])
  })

  it('PaginationBar trava as pontas, lida com lista vazia e troca o tamanho da página', async () => {
    const vazia = mount(PaginationBar, { props: { page: 1, perPage: 25, total: 0 } })
    expect(vazia.find('.resumo').text()).toBe('0–0 de 0')
    expect(vazia.find('.pg.anterior').attributes('disabled')).toBeDefined()
    expect(vazia.find('.pg.proxima').attributes('disabled')).toBeDefined()
    expect(vazia.find('select').exists()).toBe(false)

    const wrapper = mount(PaginationBar, { props: { page: 2, perPage: 25, total: 40, perPageOptions: [25, 50, 100] } })
    expect(wrapper.find('.resumo').text()).toBe('26–40 de 40')
    expect(wrapper.find('.pg.proxima').attributes('disabled')).toBeDefined()
    await wrapper.find('select').setValue('50')
    expect(wrapper.emitted('update:perPage')).toEqual([[50]])
  })

  it('ProgressBar calcula o percentual, limita em 100 e aceita modo indeterminado', () => {
    const wrapper = mount(ProgressBar, { props: { valor: 1240, total: 5000, rotulo: 'Leads usados em novembro' } })
    expect(wrapper.text()).toContain('Leads usados em novembro')
    expect(wrapper.text()).toContain('1.240 de 5.000 (25%)')
    expect(wrapper.find('[role="progressbar"]').attributes('aria-valuenow')).toBe('25')
    expect(wrapper.find('.barra').attributes('style')).toContain('width: 25%')

    const cheio = mount(ProgressBar, { props: { valor: 80, total: 40, tom: 'green' } })
    expect(cheio.find('.barra').attributes('style')).toContain('width: 100%')
    expect(cheio.classes()).toContain('green')

    const pendente = mount(ProgressBar, { props: { valor: 0, total: 0, indeterminado: true } })
    expect(pendente.find('.barra').classes()).toContain('indeterminada')
    expect(pendente.find('.numeros').exists()).toBe(false)
  })

  it('StepperSteps marca feito e atual e só deixa voltar', async () => {
    const wrapper = mount(StepperSteps, { props: { passos: ['O que criar', 'Distribuição', 'Confirmar'], atual: 1 } })
    const passos = wrapper.findAll('.passo')

    expect(passos[0].classes()).toContain('feito')
    expect(passos[1].classes()).toContain('atual')
    expect(passos[1].attributes('aria-current')).toBe('step')
    expect(passos[0].find('.marca').text()).toBe('✓')
    expect(passos[2].find('.marca').text()).toBe('3')

    await passos[2].find('.marca').trigger('click')
    await passos[0].find('.marca').trigger('click')
    expect(wrapper.emitted('update:atual')).toEqual([[0]])
  })

  it('SideDrawer abre no body, fecha pelo botão, pelo fundo e por Esc, e solta o ouvinte ao desmontar', async () => {
    const fechar = vi.fn()
    const wrapper = mount(SideDrawer, {
      props: { open: false, title: 'Padaria Central', onClose: fechar },
      slots: { default: '<p class="conteudo">ficha</p>', footer: '<button class="acao">Salvar</button>' },
      attachTo: document.body
    })
    expect(document.querySelector('.drawer')).toBeNull()

    await wrapper.setProps({ open: true })
    const gaveta = document.querySelector('.drawer')!
    expect(gaveta.getAttribute('aria-modal')).toBe('false')
    expect(gaveta.textContent).toContain('Padaria Central')
    expect(document.querySelector('.conteudo')).not.toBeNull()
    expect(document.querySelector('.drawer-footer .acao')).not.toBeNull()

    ;(document.querySelector('.drawer-close') as HTMLElement).click()
    document.querySelector('.drawer-fundo')!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    expect(fechar).toHaveBeenCalledTimes(3)

    // Com um modal aberto por cima, o Esc é do modal.
    const modal = document.createElement('div')
    modal.setAttribute('role', 'dialog')
    modal.setAttribute('aria-modal', 'true')
    document.body.appendChild(modal)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(fechar).toHaveBeenCalledTimes(3)
    modal.remove()

    wrapper.unmount()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(fechar).toHaveBeenCalledTimes(3)
    expect(document.querySelector('.drawer')).toBeNull()
  })
})

describe('useJobPolling', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('consulta a cada 2 s, avisa a cada mudança e para quando o job termina', async () => {
    const respostas = [job({ feitos: 10 }), job({ feitos: 60 }), job({ feitos: 100, status: 'concluido' })]
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(resposta(respostas.shift())))
    vi.stubGlobal('fetch', fetchMock)
    const aoMudar = vi.fn()
    const aoTerminar = vi.fn()
    const escopo = effectScope()
    const polling = escopo.run(() => useJobPolling({ aoMudar, aoTerminar }))!

    polling.acompanhar(job({ feitos: 10 }))
    expect(polling.ativo.value).toBe(true)
    expect(polling.percentual.value).toBe(10)
    expect(fetchMock).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(2000)
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/v1/prospeccao/jobs/9')
    // Mesmo `feitos`: nada mudou, nada a recarregar.
    expect(aoMudar).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(2000)
    expect(aoMudar).toHaveBeenCalledTimes(1)
    expect(polling.percentual.value).toBe(60)

    await vi.advanceTimersByTimeAsync(2000)
    expect(aoMudar).toHaveBeenCalledTimes(2)
    expect(aoTerminar).toHaveBeenCalledTimes(1)
    expect(aoTerminar.mock.calls[0][0].status).toBe('concluido')
    expect(polling.ativo.value).toBe(false)
    expect(polling.consultando.value).toBe(false)

    await vi.advanceTimersByTimeAsync(10000)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    escopo.stop()
  })

  it('limpa o intervalo e cancela a consulta em voo ao desmontar', async () => {
    let sinal: AbortSignal | undefined
    const fetchMock = vi.fn().mockImplementation((_u: string, o: any) => {
      sinal = o.signal
      return new Promise(() => {})
    })
    vi.stubGlobal('fetch', fetchMock)
    let polling!: ReturnType<typeof useJobPolling>
    const Tela = defineComponent({
      setup() {
        polling = useJobPolling()
        return () => h('div')
      }
    })
    const wrapper = mount(Tela)

    polling.acompanhar(job())
    await vi.advanceTimersByTimeAsync(2000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sinal!.aborted).toBe(false)

    wrapper.unmount()
    expect(sinal!.aborted).toBe(true)
    expect(polling.consultando.value).toBe(false)
    await vi.advanceTimersByTimeAsync(10000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('segue tentando depois de falha de rede e desiste em 404', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error('rede caiu'))
      .mockResolvedValueOnce(resposta(job({ feitos: 20 })))
      .mockResolvedValueOnce(resposta({ error: 'registro não encontrado' }, 404))
    vi.stubGlobal('fetch', fetchMock)
    const escopo = effectScope()
    const polling = escopo.run(() => useJobPolling({ intervalo: 500 }))!

    polling.acompanhar(job())
    await vi.advanceTimersByTimeAsync(500)
    expect(polling.erro.value).toBe('rede caiu')
    expect(polling.consultando.value).toBe(true)

    await vi.advanceTimersByTimeAsync(500)
    expect(polling.erro.value).toBe('')
    expect(polling.job.value!.feitos).toBe(20)

    await vi.advanceTimersByTimeAsync(500)
    expect(polling.erro.value).toBe('registro não encontrado')
    expect(polling.consultando.value).toBe(false)
    await vi.advanceTimersByTimeAsync(5000)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    escopo.stop()
  })

  it('aceita só o id, guarda job já terminado sem consultar e cancela pelo endpoint', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string, o: any) => {
      if (o.method === 'POST') return Promise.resolve(resposta(job({ status: 'cancelado', mensagem: 'cancelado pelo usuário' })))
      return Promise.resolve(resposta(job({ feitos: 30 })))
    })
    vi.stubGlobal('fetch', fetchMock)
    const aoTerminar = vi.fn()
    const escopo = effectScope()
    const polling = escopo.run(() => useJobPolling({ aoTerminar }))!

    polling.acompanhar(job({ status: 'falhou', mensagem: 'saldo esgotado' }))
    expect(polling.ativo.value).toBe(false)
    expect(polling.consultando.value).toBe(false)
    expect(polling.job.value!.mensagem).toBe('saldo esgotado')

    polling.acompanhar(9)
    await vi.advanceTimersByTimeAsync(0)
    expect(polling.job.value!.feitos).toBe(30)
    expect(polling.ativo.value).toBe(true)

    const cancelado = await polling.cancelar()
    expect(String(fetchMock.mock.calls.at(-1)![0])).toBe('/api/v1/prospeccao/jobs/9/cancelar')
    expect(cancelado!.status).toBe('cancelado')
    expect(polling.ativo.value).toBe(false)
    expect(aoTerminar).toHaveBeenCalledTimes(1)
    const chamadas = fetchMock.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(fetchMock).toHaveBeenCalledTimes(chamadas)
    escopo.stop()
  })
})

describe('useConsultaAdiada', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('espera o usuário parar e só consulta o último pedido', async () => {
    const consultar = vi.fn().mockImplementation((n: number) => Promise.resolve(n * 10))
    const escopo = effectScope()
    const c = escopo.run(() => useConsultaAdiada<number, number>(consultar))!

    c.pedir(1)
    await vi.advanceTimersByTimeAsync(300)
    c.pedir(2)
    await vi.advanceTimersByTimeAsync(399)
    expect(consultar).not.toHaveBeenCalled()
    expect(c.carregando.value).toBe(true)

    await vi.advanceTimersByTimeAsync(1)
    expect(consultar).toHaveBeenCalledTimes(1)
    expect(c.dados.value).toBe(20)
    expect(c.carregando.value).toBe(false)
    escopo.stop()
  })

  it('descarta a resposta atrasada e cancela a consulta anterior', async () => {
    const pendentes: { resolver: (v: string) => void; signal: AbortSignal }[] = []
    const consultar = vi.fn().mockImplementation(
      (_a: string, signal: AbortSignal) => new Promise<string>((resolver) => pendentes.push({ resolver, signal }))
    )
    const escopo = effectScope()
    const c = escopo.run(() => useConsultaAdiada<string, string>(consultar, { atraso: 100 }))!

    c.pedir('CE')
    await vi.advanceTimersByTimeAsync(100)
    c.pedir('CE+SP')
    await vi.advanceTimersByTimeAsync(100)
    expect(pendentes).toHaveLength(2)
    expect(pendentes[0].signal.aborted).toBe(true)

    // A segunda responde primeiro; a primeira chega atrasada e é ignorada.
    pendentes[1].resolver('12.480')
    await flushPromises()
    pendentes[0].resolver('900')
    await flushPromises()
    expect(c.dados.value).toBe('12.480')
    escopo.stop()
  })

  it('em 429 de muitas requisições mantém o último número e tenta de novo, sem erro', async () => {
    const aoErro = vi.fn()
    const consultar = vi
      .fn()
      .mockResolvedValueOnce(100)
      .mockRejectedValueOnce(new ApiError(429, 'muitas consultas', { codigo: 'muitas_requisicoes' }, 3))
      .mockResolvedValueOnce(250)
    const escopo = effectScope()
    const c = escopo.run(() => useConsultaAdiada<number, number>(consultar, { atraso: 0, aoErro }))!

    await c.agora(1)
    expect(c.dados.value).toBe(100)

    await c.agora(2)
    expect(c.dados.value).toBe(100)
    expect(c.erro.value).toBe('')
    expect(aoErro).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(3000)
    expect(consultar).toHaveBeenCalledTimes(3)
    expect(c.dados.value).toBe(250)
    escopo.stop()
  })

  it('erro de verdade aparece; limpar zera; desmontar cancela o que está agendado', async () => {
    const aoErro = vi.fn()
    const consultar = vi.fn().mockRejectedValue(new ApiError(429, 'cota mensal de prospecção esgotada', { codigo: 'cota_esgotada' }))
    const escopo = effectScope()
    const c = escopo.run(() => useConsultaAdiada<number, number>(consultar, { aoErro }))!

    await c.agora(1)
    expect(c.erro.value).toBe('cota mensal de prospecção esgotada')
    expect(aoErro).toHaveBeenCalledTimes(1)
    expect(c.carregando.value).toBe(false)

    c.limpar()
    expect(c.erro.value).toBe('')
    expect(c.dados.value).toBeNull()

    c.pedir(2)
    escopo.stop()
    await vi.advanceTimersByTimeAsync(1000)
    expect(consultar).toHaveBeenCalledTimes(1)
  })
})

describe('modo foco e atalhos', () => {
  it('useModoFoco põe a classe no body enquanto a tela está montada', () => {
    const Tela = defineComponent({
      setup() {
        useModoFoco()
        return () => h('div')
      }
    })
    const wrapper = mount(Tela)
    expect(document.body.classList.contains('modo-foco')).toBe(true)
    wrapper.unmount()
    expect(document.body.classList.contains('modo-foco')).toBe(false)
  })

  it('a regra do modo foco mora em styles.css, não no chunk de uma view', async () => {
    // @ts-ignore: o projeto não instala os tipos do Node; o Vitest roda nele.
    const { readFileSync } = await import('node:fs')
    const css: string = readFileSync('src/styles.css', 'utf8')
    expect(css).toMatch(/body\.modo-foco \.sidebar,\s*body\.modo-foco \.topbar[^{]*\{\s*display: none !important;/)
  })

  it('useAtalhos ignora campo de digitação, modal aberto e modificadores', async () => {
    const um = vi.fn()
    const sair = vi.fn()
    let ativo = true
    const Tela = defineComponent({
      setup() {
        useAtalhos({ '1': um, escape: sair }, { ativo: () => ativo, sempre: ['escape'] })
        return () => h('div', [h('input', { class: 'notas' })])
      }
    })
    const wrapper = mount(Tela, { attachTo: document.body })
    const tecla = (key: string, alvo: EventTarget = window, extra: KeyboardEventInit = {}) =>
      alvo.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...extra }))

    tecla('1')
    expect(um).toHaveBeenCalledTimes(1)

    // Foco no campo de notas: o "1" é texto.
    const campo = wrapper.find('.notas').element
    tecla('1', campo)
    expect(um).toHaveBeenCalledTimes(1)
    expect(focoEmCampo(campo)).toBe(true)
    // Tecla marcada como "sempre" vale mesmo no campo.
    tecla('Escape', campo)
    expect(sair).toHaveBeenCalledTimes(1)

    tecla('1', window, { ctrlKey: true })
    expect(um).toHaveBeenCalledTimes(1)

    const modal = document.createElement('div')
    modal.setAttribute('role', 'dialog')
    modal.setAttribute('aria-modal', 'true')
    document.body.appendChild(modal)
    tecla('1')
    expect(um).toHaveBeenCalledTimes(1)
    modal.remove()

    ativo = false
    tecla('1')
    expect(um).toHaveBeenCalledTimes(1)
    ativo = true

    wrapper.unmount()
    tecla('1')
    expect(um).toHaveBeenCalledTimes(1)
  })
})

describe('rotas, menu e permissões', () => {
  const stubs = { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' }, RouterView: true, GlobalSearch: true }

  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('registra as cinco rotas novas com nome e permissão do contrato', () => {
    const esperado: Record<string, [string, string]> = {
      prospecting: ['/prospeccao', 'prospecting.view'],
      'prospect-list': ['/prospeccao/listas/:id', 'prospecting.view'],
      leads: ['/leads', 'leads.view'],
      'call-queue': ['/fila-de-ligacoes', 'leads.edit'],
      'settings-prospecting': ['/configuracoes/prospeccao', 'prospecting.manage']
    }
    for (const [nome, [caminho, permissao]] of Object.entries(esperado)) {
      const rota = router.getRoutes().find((r) => r.name === nome)
      expect(rota, nome).toBeTruthy()
      expect(rota!.path).toBe(caminho)
      expect(rota!.meta.permission).toBe(permissao)
    }
    expect(router.resolve('/prospeccao/listas/12').params.id).toBe('12')
    expect(router.resolve('/fila-de-ligacoes?lista=4').query.lista).toBe('4')
  })

  it('gestor vê a seção Prospecção com os três links', () => {
    loginAs('manager', { 'prospecting.view': true, 'leads.view': true, 'leads.edit': true })
    const wrapper = mount(AppShell, { global: { stubs, provide: { [routerKey as symbol]: router } } })

    expect(wrapper.findAll('.nav-section').map((s) => s.text())).toContain('Prospecção')
    const links = wrapper.findAll('nav a').map((a) => [a.attributes('href'), a.find('.label').text()])
    expect(links).toContainEqual(['/prospeccao', 'Gerar listas'])
    expect(links).toContainEqual(['/leads', 'Leads de prospecção'])
    expect(links).toContainEqual(['/fila-de-ligacoes', 'Fila de ligações'])
  })

  it('vendedor (leads.view e leads.edit) vê a seção sem "Gerar listas"', () => {
    loginAs('seller', { 'leads.view': true, 'leads.edit': true })
    const wrapper = mount(AppShell, { global: { stubs, provide: { [routerKey as symbol]: router } } })
    const hrefs = wrapper.findAll('nav a').map((a) => a.attributes('href'))

    expect(wrapper.findAll('.nav-section').map((s) => s.text())).toContain('Prospecção')
    expect(hrefs).not.toContain('/prospeccao')
    expect(hrefs).toContain('/leads')
    expect(hrefs).toContain('/fila-de-ligacoes')
  })

  it('quem só vê leads não tem a fila; sem nenhuma permissão a seção some', () => {
    loginAs('seller', { 'leads.view': true })
    let wrapper = mount(AppShell, { global: { stubs, provide: { [routerKey as symbol]: router } } })
    let hrefs = wrapper.findAll('nav a').map((a) => a.attributes('href'))
    expect(hrefs).toContain('/leads')
    expect(hrefs).not.toContain('/fila-de-ligacoes')

    loginAs('seller', { 'contacts.view': true })
    wrapper = mount(AppShell, { global: { stubs, provide: { [routerKey as symbol]: router } } })
    hrefs = wrapper.findAll('nav a').map((a) => a.attributes('href'))
    expect(wrapper.findAll('.nav-section').map((s) => s.text())).not.toContain('Prospecção')
    expect(hrefs).not.toContain('/leads')
  })

  it('prospecting.manage entra em canManage e abre o item do menu de configurações', async () => {
    const auth = loginAs('manager', { 'prospecting.manage': true })
    expect(auth.canManage).toBe(true)
    const wrapper = mount(SettingsLayout, { global: { stubs } })
    await nextTick()
    const link = wrapper.findAll('a').find((a) => a.attributes('href') === '/configuracoes/prospeccao')
    expect(link?.text()).toBe('Prospecção')

    const semAcesso = loginAs('seller', { 'leads.view': true, 'leads.edit': true })
    expect(semAcesso.canManage).toBe(false)
    const outro = mount(SettingsLayout, { global: { stubs } })
    expect(outro.findAll('a').some((a) => a.attributes('href') === '/configuracoes/prospeccao')).toBe(false)
  })

  it('as telas provisórias montam', async () => {
    for (const nome of ['prospecting', 'prospect-list', 'leads', 'call-queue', 'settings-prospecting']) {
      const rota = router.getRoutes().find((r) => r.name === nome)!
      const modulo: any = await (rota.components!.default as any)()
      const wrapper = mount(modulo.default)
      expect(wrapper.find('h1').exists()).toBe(true)
    }
  })
})
