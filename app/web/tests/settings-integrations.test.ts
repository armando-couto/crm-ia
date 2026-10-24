import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SettingsIntegrationsView from '../src/views/SettingsIntegrationsView.vue'
import { useToastStore } from '../src/stores/toast'

const settings = {
  auto_receita: true,
  sobrescrever: 'vazios',
  limite_mensal_centavos: 20000,
  prazo_cache_dias: 30,
  bigdatacorp: {
    habilitado: true,
    demo: false,
    token_id: 'tok-123',
    has_secret: true,
    datasets_padrao: ['basic_data', 'registration_data', 'activity_indicators', 'relationships'],
    precos_centavos: { basic_data: 3, registration_data: 14, activity_indicators: 6, relationships: 4, phones_extended: 6, emails_extended: 6, addresses_extended: 6, domains_extended: 6, processes: 8, kyc: 6, media_profile_and_exposure: 10 }
  },
  serasa: { habilitado: true, demo: true, ambiente: 'uat', client_id: 'cid', has_secret: true, report_name: 'RELATORIO_AVANCADO_PJ_PME', optional_features: '', preco_centavos: 1500 }
}

const mesAtual = new Date().toISOString().slice(0, 7)

const usage = {
  meses: [
    { mes: mesAtual, consultas: 12, custo_centavos: 340, por_provedor: { receita: { consultas: 9, custo_centavos: 0 }, bigdatacorp: { consultas: 3, custo_centavos: 340 } } },
    { mes: '2026-09', consultas: 40, custo_centavos: 1200, por_provedor: {} }
  ]
}

type Chamada = { url: string; method: string; body: any }
const chamadas: Chamada[] = []

function stubFetch() {
  chamadas.length = 0
  return vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const u = String(url)
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : null
    chamadas.push({ url: u, method, body })
    let resp: unknown = {}
    if (u.includes('/settings/enrichment/usage')) resp = usage
    else if (u.includes('/settings/enrichment/test')) resp = { ok: true, mensagem: 'conexão ok', custo_centavos: 3, demo: false }
    else if (u.includes('/settings/enrichment') && method === 'PUT') resp = { settings: { ...settings, ...body, bigdatacorp: { ...settings.bigdatacorp, ...body.bigdatacorp, has_secret: true } }, fontes: [], uso_mes: { consultas: 12, custo_centavos: 340, limite_centavos: 20000 } }
    else if (u.includes('/settings/enrichment')) resp = { settings, fontes: [], uso_mes: { consultas: 12, custo_centavos: 340, limite_centavos: 20000 } }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(resp)) })
  })
}

function texto(wrapper: { text(): string }) {
  return wrapper.text().replace(/ /g, ' ')
}

describe('SettingsIntegrationsView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('salva sem reenviar os segredos quando os campos ficam em branco', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsIntegrationsView)
    await flushPromises()

    const tokenInput = wrapper.find('[data-source="bigdatacorp"] input[type="password"]')
    expect(tokenInput.attributes('placeholder')).toBe('•••••• (mantida se vazio)')
    expect(wrapper.find('[data-source="bigdatacorp"]').text()).toContain('guardado')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const put = chamadas.find((c) => c.method === 'PUT' && c.url.includes('/settings/enrichment'))
    expect(put).toBeDefined()
    expect(put!.body.bigdatacorp.access_token).toBeUndefined()
    expect(put!.body.serasa.client_secret).toBeUndefined()
    expect(put!.body.bigdatacorp.token_id).toBe('tok-123')
    expect(put!.body.bigdatacorp.datasets_padrao).toEqual(['basic_data', 'registration_data', 'activity_indicators', 'relationships'])
    expect(put!.body.limite_mensal_centavos).toBe(20000)
    expect(useToastStore().toasts.map((t) => t.message)).toContain('Integrações de dados salvas')
    wrapper.unmount()
  })

  it('envia o segredo em claro quando digitado e limpa o campo depois', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsIntegrationsView)
    await flushPromises()

    const tokenInput = wrapper.find('[data-source="bigdatacorp"] input[type="password"]')
    await tokenInput.setValue('novo-token')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const put = chamadas.find((c) => c.method === 'PUT')
    expect(put!.body.bigdatacorp.access_token).toBe('novo-token')
    expect(put!.body.serasa.client_secret).toBeUndefined()
    expect((wrapper.find('[data-source="bigdatacorp"] input[type="password"]').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })

  it('mostra o total dos conjuntos padrão, limite em reais e o uso do mês por fonte', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsIntegrationsView)
    await flushPromises()

    expect(texto(wrapper)).toContain('≈ R$ 0,27 por consulta · 4 conjuntos')

    // Marca mais um conjunto: o total acompanha.
    await wrapper.find('input[data-dataset="phones_extended"]').setValue(true)
    expect(texto(wrapper)).toContain('≈ R$ 0,33 por consulta · 5 conjuntos')

    // Limite mensal em R$ ↔ centavos.
    const limite = wrapper.findAll('[data-source="regras"] input').find((i) => (i.element as HTMLInputElement).value === '200,00')
    expect(limite).toBeDefined()
    await limite!.setValue('350,50')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(chamadas.find((c) => c.method === 'PUT')!.body.limite_mensal_centavos).toBe(35050)

    const uso = wrapper.find('.usage')
    expect(texto(uso)).toContain('R$ 3,40')
    expect(texto(uso)).toContain('12 consultas')
    expect(texto(uso)).toContain('BigDataCorp')
    expect(texto(uso)).toContain('set/2026')
    expect(wrapper.find('[data-source="serasa"]').text()).toContain('demonstração')
    wrapper.unmount()
  })

  it('botão Testar chama o endpoint de teste com o CNPJ limpo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsIntegrationsView)
    await flushPromises()

    const card = wrapper.find('[data-source="bigdatacorp"]')
    await card.find('.test input').setValue('11.222.333/0001-81')
    await card.find('.test button').trigger('click')
    await flushPromises()

    const teste = chamadas.find((c) => c.url.includes('/settings/enrichment/test'))
    expect(teste?.body).toEqual({ provedor: 'bigdatacorp', cnpj: '11222333000181' })
    expect(card.text()).toContain('conexão ok')
    wrapper.unmount()
  })
})
