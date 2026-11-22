// Who Lead como quarta fonte da inteligência de empresas (docs/PROSPECCAO.md 7 e 10.5;
// checklist do mapa de frontend, seção 9).
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import EnrichModal from '../src/components/intel/EnrichModal.vue'
import IntelTab from '../src/components/intel/IntelTab.vue'
import SourceBadge from '../src/components/intel/SourceBadge.vue'
import { nomeProvedor } from '../src/components/intel/util'
import { useAuthStore } from '../src/stores/auth'

const company = { id: 1, name: 'ACME', cnpj: '11.222.333/0001-81', phone: '' } as any

function settingsCom(wholead?: Record<string, unknown>) {
  return {
    settings: {
      auto_receita: true,
      sobrescrever: 'vazios',
      limite_mensal_centavos: 20000,
      prazo_cache_dias: 30,
      bigdatacorp: { habilitado: false, demo: false, token_id: '', has_secret: false, datasets_padrao: [], precos_centavos: {} },
      serasa: { habilitado: false, demo: false, ambiente: 'uat', client_id: '', has_secret: false, report_name: '', optional_features: '', preco_centavos: 1500 },
      ...(wholead ? { wholead } : {})
    },
    fontes: [],
    uso_mes: { consultas: 0, custo_centavos: 0, limite_centavos: 20000 }
  }
}

type Chamada = { url: string; method: string; body: any }
const chamadas: Chamada[] = []

function stubFetch(settings: unknown) {
  chamadas.length = 0
  return vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const u = String(url)
    const method = init?.method ?? 'GET'
    chamadas.push({ url: u, method, body: init?.body ? JSON.parse(String(init.body)) : null })
    let body: unknown = {}
    if (u.includes('/settings/enrichment')) body = settings
    else if (u.includes('/enrich/preview')) body = { enrichment_ids: [1], consultas: [], perfil: null, diff: [], custo_centavos: 0 }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function admin() {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana', email: 'ana@exemplo.com.br', role: 'admin', active: true, created_at: '', updated_at: '' } as any
}

describe('Who Lead no modal de enriquecer', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    admin()
  })

  it('aparece como quarta fonte, desmarcado por padrão e desabilitado sem configuração', async () => {
    vi.stubGlobal('fetch', stubFetch(settingsCom()))
    const wrapper = mount(EnrichModal, { props: { open: true, company }, global: { stubs: { Teleport: true } } })
    await flushPromises()

    const fontes = wrapper.findAll('label.source')
    expect(fontes.length).toBe(4)
    const wl = wrapper.find('label.source[data-fonte="wholead"]')
    expect(wl.text()).toContain('Who Lead')
    expect(wl.text()).toContain('não configurado')
    const input = wl.find('input').element as HTMLInputElement
    expect(input.checked).toBe(false)
    expect(input.disabled).toBe(true)
    wrapper.unmount()
  })

  it('habilitado mostra o preço por CNPJ, soma ao custo e envia "wholead" nos provedores só quando marcado', async () => {
    vi.stubGlobal('fetch', stubFetch(settingsCom({ habilitado: true, demo: false, client_id: 'c', has_secret: true, preco_centavos: 80, lote_tamanho: 500 })))
    const wrapper = mount(EnrichModal, { props: { open: true, company }, global: { stubs: { Teleport: true } } })
    await flushPromises()

    const wl = wrapper.find('label.source[data-fonte="wholead"]')
    expect(wl.text().replace(/ /g, ' ')).toContain('≈ R$ 0,80 · por CNPJ')
    await wl.find('input').setValue(true)
    const consultar = wrapper.findAll('button').find((b) => /Consultar/.test(b.text()))!
    await consultar.trigger('click')
    await flushPromises()

    const preview = chamadas.find((c) => c.url.includes('/enrich/preview'))!
    expect(preview.body.provedores).toEqual(['receita', 'wholead'])
    wrapper.unmount()
  })

  it('em demonstração o rótulo é "demonstração"', async () => {
    vi.stubGlobal('fetch', stubFetch(settingsCom({ habilitado: true, demo: true, client_id: '', has_secret: false, preco_centavos: 80, lote_tamanho: 500 })))
    const wrapper = mount(EnrichModal, { props: { open: true, company }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.find('label.source[data-fonte="wholead"]').text()).toContain('demonstração')
    wrapper.unmount()
  })
})

describe('Who Lead na aba Inteligência', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    admin()
  })

  const data = {
    company,
    intel: {
      provedor: 'wholead',
      demo: false,
      consultado_em: '2026-11-20T12:00:00Z',
      campos: {},
      contatos: {
        telefones: [
          { numero: '85999990001', tipo: 'celular', whatsapp: true, origem: 'wholead' },
          { numero: '8530000001', tipo: 'fixo', nao_perturbe: true, origem: 'wholead' }
        ],
        emails: []
      },
      pessoas: [
        {
          nome: 'MARIA SOUZA',
          documento: '***123456**',
          papel: 'socio',
          cargo: 'Sócia-administradora',
          telefones: [{ numero: '85988887777', tipo: 'celular', whatsapp: true, nao_perturbe: true }],
          emails: ['maria@acme.com.br']
        },
        { nome: 'JOÃO LIMA', documento: '', papel: 'representante', cargo: '', telefones: [], emails: [] }
      ]
    },
    fontes: {},
    historico: [{ id: 3, provedor: 'wholead', status: 'ok', demo: false, custo_centavos: 80, datasets: null, user_name: 'Ana', created_at: '2026-11-20T12:00:00Z', aplicado_em: null }],
    uso_mes: null
  } as any

  it('marca WhatsApp e "não perturbe" nos telefones e mostra o cartão de pessoas com CPF mascarado', async () => {
    vi.stubGlobal('fetch', stubFetch(settingsCom()))
    const wrapper = mount(IntelTab, { props: { company, data, loading: false, canEnrich: true }, global: { stubs: { Teleport: true } } })
    await flushPromises()

    expect(wrapper.findAll('[data-marca="whatsapp"]').length).toBe(2)
    expect(wrapper.findAll('[data-marca="nao-perturbe"]').length).toBe(2)

    const pessoas = wrapper.find('[data-card="pessoas"]')
    expect(pessoas.exists()).toBe(true)
    const texto = pessoas.text()
    expect(texto).toContain('MARIA SOUZA')
    expect(texto).toContain('Sócio')
    expect(texto).toContain('Sócia-administradora')
    expect(texto).toContain('***123456**')
    expect(texto).toContain('(85) 98888-7777')
    expect(texto).toContain('maria@acme.com.br')
    expect(texto).toContain('Representante legal')
    expect(texto).toContain('Sem contato encontrado.')
    expect(texto).toContain('Who Lead')
    wrapper.unmount()
  })

  it('nome e pílula do provedor', () => {
    expect(nomeProvedor('wholead')).toBe('Who Lead (Plus Soluções)')
    const b = mount(SourceBadge, { props: { provedor: 'wholead', comNome: true } as any })
    expect(b.classes()).toContain('wholead')
    b.unmount()
  })
})
