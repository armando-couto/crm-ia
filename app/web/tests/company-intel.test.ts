import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CompanyDetailView from '../src/views/CompanyDetailView.vue'
import CompaniesView from '../src/views/CompaniesView.vue'
import { useAuthStore } from '../src/stores/auth'
import { useToastStore } from '../src/stores/toast'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: { id: '1' }, query: {} })
}))

// ===== Fixtures (formato do contrato em docs/FUNIL-E-INTELIGENCIA.md) =====
const company = {
  id: 1,
  name: 'ACME',
  domain: 'acme.com.br',
  phone: '',
  industry: 'Desenvolvimento de programas',
  city: 'Fortaleza',
  state: 'CE',
  owner_id: null,
  owner_name: '',
  contacts_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  is_target: false,
  target_tier: 2,
  target_notes: '',
  target_since: null,
  ec_number: '',
  economic_group: '',
  cnpj: '11.222.333/0001-81',
  accredited_at: null,
  representative: '',
  instagram: '',
  products: [],
  machines_count: 0,
  is_client: false,
  anticipation_mode: '',
  validator: false,
  do_not_disturb: false,
  legal_name: 'ACME VENDAS LTDA',
  registration_status: 'ATIVA',
  registration_status_at: '2015-03-10',
  founded_at: '2015-03-10',
  company_size: 'ME',
  cnae_code: '6201-5/01',
  cnae_description: 'Desenvolvimento de programas',
  enriched_at: '2026-10-24T12:00:00Z'
}

const perfil = {
  provedor: 'receita',
  demo: false,
  consultado_em: '2026-10-24T12:00:00Z',
  campos: {
    legal_name: 'ACME VENDAS LTDA',
    trade_name: 'ACME',
    name: 'ACME',
    cnae_code: '6201-5/01',
    cnae_description: 'Desenvolvimento de programas',
    legal_nature: 'Sociedade Empresária Limitada',
    founded_at: '2015-03-10',
    capital: 100000,
    company_size: 'ME',
    registration_status: 'ATIVA',
    registration_status_at: '2015-03-10',
    simples: true,
    mei: false,
    street: 'Rua X',
    street_number: '100',
    neighborhood: 'Centro',
    zip: '60000000',
    city: 'Fortaleza',
    state: 'CE',
    phone: '(85) 3333-4444',
    email: 'contato@acme.com.br',
    website: 'https://acme.com.br',
    domain: 'acme.com.br'
  },
  socios: [{ nome: 'FULANO DE TAL', documento: '***123456**', qualificacao: 'Sócio-Administrador', desde: '2015-03-10', tipo: 'PF' }],
  cnaes_secundarios: [{ codigo: '6202-3/00', descricao: 'Desenvolvimento de programas customizáveis' }],
  indicadores: { nivel_atividade: 0.82, tem_dominio_ativo: true, tem_ssl: true, tem_email_corporativo: true, funcionarios_faixa: '10 a 49', faturamento_faixa: 'R$ 1M a 5M' },
  contatos: {
    telefones: [{ numero: '+5585999990000', tipo: 'mobile', principal: true, validado: true }],
    emails: [{ email: 'contato@acme.com.br', tipo: 'corporativo', validado: true }]
  },
  presenca_digital: { dominios: [{ dominio: 'acme.com.br', ativo: true, ssl: true }], redes: { instagram: 'acme' } },
  risco: { score: 720, faixa: 'B', nivel: 'baixo', pendencias_qtd: 0, pendencias_valor: 0, protestos_qtd: 0, cheques_sem_fundo: 0, acoes_qtd: 0, consultas_90d: 3, limite_sugerido: 50000, observacao: '' },
  processos: { total: 2, ativos: 1, como_reu: 1, valor_total: 35000, ultimos: [{ numero: '0001234-56.2026.8.06.0001', tipo: 'Cível', tribunal: 'TJCE', status: 'Ativo', polo: 'Réu', valor: 20000, data: '2026-01-10' }] },
  conformidade: { pep: false, sancionado: false, listas: [] },
  midia: { exposicao: 'baixa', noticias: [{ titulo: 'ACME lança produto', url: 'https://noticias.exemplo/1', sentimento: 'positivo', data: '2026-09-01' }] },
  sinais: [
    { tipo: 'situacao', nivel: 'ok', texto: 'Situação cadastral ativa desde 2015' },
    { tipo: 'risco', nivel: 'atencao', texto: '3 consultas nos últimos 90 dias' }
  ],
  custo_centavos: 27
}

const intel = {
  company,
  intel: perfil,
  fontes: {
    legal_name: { provedor: 'receita', fetched_at: '2026-10-24T12:00:00Z' },
    phone: { provedor: 'receita', fetched_at: '2026-10-24T12:00:00Z' }
  },
  historico: [
    { id: 10, provedor: 'serasa', status: 'ok', demo: true, custo_centavos: 0, datasets: null, user_name: 'Ana', created_at: '2026-10-24T13:00:00Z', aplicado_em: '2026-10-24T13:01:00Z' },
    { id: 9, provedor: 'receita', status: 'ok', demo: false, custo_centavos: 0, datasets: null, user_name: 'Ana', created_at: '2026-10-24T12:00:00Z', aplicado_em: '2026-10-24T12:01:00Z' }
  ],
  uso_mes: { consultas: 12, custo_centavos: 340, limite_centavos: 20000 }
}

const settings = {
  settings: {
    auto_receita: true,
    sobrescrever: 'vazios',
    limite_mensal_centavos: 20000,
    prazo_cache_dias: 30,
    bigdatacorp: {
      habilitado: true,
      demo: false,
      token_id: 'tok',
      has_secret: true,
      datasets_padrao: ['basic_data', 'registration_data', 'activity_indicators', 'relationships'],
      precos_centavos: { basic_data: 3, registration_data: 14, activity_indicators: 6, relationships: 4, phones_extended: 6 }
    },
    serasa: { habilitado: false, demo: false, ambiente: 'uat', client_id: '', has_secret: false, report_name: 'RELATORIO_AVANCADO_PJ_PME', optional_features: '', preco_centavos: 1500 }
  },
  fontes: [],
  uso_mes: { consultas: 12, custo_centavos: 340, limite_centavos: 20000 }
}

const preview = {
  enrichment_ids: [11],
  consultas: [
    { provedor: 'receita', status: 'ok', mensagem: '', demo: false, custo_centavos: 0, cache: true },
    { provedor: 'bigdatacorp', status: 'erro', mensagem: 'BigDataCorp não configurado. Peça ao administrador para ativar em Configurações → Integrações.', demo: false, custo_centavos: 0, cache: false }
  ],
  perfil,
  diff: [
    { campo: 'legal_name', label: 'Razão social', atual: '', proposto: 'ACME VENDAS LTDA', provedor: 'receita', sugerido: true },
    { campo: 'phone', label: 'Telefone', atual: '', proposto: '(85) 3333-4444', provedor: 'receita', sugerido: true },
    { campo: 'city', label: 'Cidade', atual: 'Fortaleza', proposto: 'FORTALEZA', provedor: 'receita', sugerido: false }
  ],
  custo_centavos: 0
}

const consultaCnpj = {
  perfil,
  existente: null
}

type Chamada = { url: string; method: string; body: any }
const chamadas: Chamada[] = []

function stubFetch(extra: (u: string, method: string) => unknown | undefined = () => undefined) {
  chamadas.length = 0
  return vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const u = String(url)
    const method = init?.method ?? 'GET'
    chamadas.push({ url: u, method, body: init?.body ? JSON.parse(String(init.body)) : null })
    let body: unknown = extra(u, method)
    if (body === undefined) {
      if (u.includes('/companies/1/intel')) body = intel
      else if (u.includes('/companies/1/enrich/preview')) body = preview
      else if (u.includes('/companies/1/enrich/apply')) body = { company: { ...company, legal_name: 'ACME VENDAS LTDA' }, aplicados: ['legal_name'] }
      else if (u.includes('/companies/enrich/bulk')) body = { processados: 1, enriquecidos: 1, sem_cnpj: 0, erros: [] }
      else if (u.includes('/settings/enrichment')) body = settings
      else if (u.includes('/cnpj/')) body = consultaCnpj
      else if (u.includes('/companies/1')) body = company
      else if (u.includes('/companies?')) body = { data: [company], pagination: { page: 1, per_page: 25, total: 1 } }
      else if (u.includes('/contacts?') || u.includes('/deals') || u.includes('/tickets') || u.includes('/tasks') || u.includes('/meetings'))
        body = { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
      else if (u.includes('/activities')) body = { data: [] }
      else body = []
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function admin() {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana', email: 'ana@exemplo.com.br', role: 'admin', active: true, created_at: '', updated_at: '' } as any
}

const stubs = { 'router-link': { template: '<a><slot /></a>' }, Teleport: true }

function botao(wrapper: ReturnType<typeof mount>, texto: string) {
  const b = wrapper.findAll('button').find((x) => x.text().trim().startsWith(texto))
  if (!b) throw new Error(`botão "${texto}" não encontrado`)
  return b
}

describe('Ficha da empresa — Inteligência', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    admin()
  })

  it('mostra a aba Inteligência com sinais, cartões e histórico a partir do payload mockado', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompanyDetailView, { global: { stubs } })
    await flushPromises()

    // Empresa já enriquecida: o intel é carregado uma vez e dá o ✦ em "Sobre essa empresa".
    expect(chamadas.filter((c) => c.url.includes('/companies/1/intel')).length).toBe(1)
    const badge = wrapper.find('.about .source')
    expect(badge.exists()).toBe(true)
    expect(badge.attributes('title')).toBe('Receita Federal · 24/10/2026')
    expect(wrapper.text()).toContain('Razão social')
    expect(wrapper.text()).toContain('Situação cadastral')

    const tabs = wrapper.findAll('.center-tabs > button').map((b) => b.text())
    expect(tabs[2]).toContain('Inteligência')
    await wrapper.findAll('.center-tabs > button')[2].trigger('click')
    await flushPromises()

    // Uma chamada só, reaproveitada pela aba.
    expect(chamadas.filter((c) => c.url.includes('/companies/1/intel')).length).toBe(1)

    const chips = wrapper.findAll('.signals .chip')
    expect(chips.length).toBe(2)
    expect(chips[0].classes()).toContain('ok')
    expect(chips[0].text()).toContain('Situação cadastral ativa desde 2015')
    expect(chips[1].classes()).toContain('atencao')

    for (const card of ['cadastro', 'endereco', 'contatos', 'socios', 'atividade', 'presenca', 'risco', 'processos', 'conformidade', 'midia']) {
      expect(wrapper.find(`[data-card="${card}"]`).exists(), `cartão ${card}`).toBe(true)
    }
    expect(wrapper.text()).toContain('ACME VENDAS LTDA')
    expect(wrapper.text()).toContain('FULANO DE TAL')
    expect(wrapper.text()).toContain('Sócio-Administrador')
    expect(wrapper.find('[data-card="risco"]').text()).toContain('720')
    expect(wrapper.find('[data-card="risco"]').text()).toContain('Faixa B')
    expect(wrapper.find('[data-card="endereco"] a').attributes('href')).toContain('google.com/maps')
    expect(wrapper.find('[data-card="cadastro"]').text()).toContain('Fonte · Receita Federal · 24/10/2026')
    expect(wrapper.find('[data-card="risco"]').text()).toContain('Serasa Experian')
    // Serasa em demo no histórico → faixa de demonstração.
    expect(wrapper.find('.intel .demo-banner').exists()).toBe(true)
    expect(wrapper.text()).toContain('Histórico de consultas')
    expect(wrapper.text()).toContain('Remover dados de inteligência')
    wrapper.unmount()
  })

  it('modal de enriquecimento mostra o diff e envia só os campos marcados', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompanyDetailView, { global: { stubs } })
    await flushPromises()

    const enrich = wrapper.find('.quick-actions button.enrich')
    expect(enrich.exists()).toBe(true)
    expect(enrich.attributes('disabled')).toBeUndefined()
    await enrich.trigger('click')
    await flushPromises()

    // Passo 1: fontes com custo estimado vindo das configurações.
    expect(wrapper.text()).toContain('Receita Federal')
    expect(wrapper.text()).toContain('grátis')
    expect(wrapper.text().replace(/ /g, ' ')).toContain('≈ R$ 0,27 · 4 conjuntos')
    expect(wrapper.text()).toContain('Uso neste mês')

    await botao(wrapper, 'Consultar').trigger('click')
    await flushPromises()

    const previewCall = chamadas.find((c) => c.url.includes('/enrich/preview'))
    expect(previewCall?.body.provedores).toEqual(['receita'])

    // Passo 2: erro da fonte inline, diff com sugeridos marcados.
    expect(wrapper.text()).toContain('BigDataCorp não configurado')
    const rows = wrapper.findAll('table.diff tbody tr')
    expect(rows.length).toBe(3)
    expect(rows[0].text()).toContain('Razão social')
    expect(rows[0].text()).toContain('ACME VENDAS LTDA')
    const checks = wrapper.findAll('table.diff input[type="checkbox"]')
    expect(checks.map((c) => (c.element as HTMLInputElement).checked)).toEqual([true, true, false])

    await wrapper.find('input[data-campo="phone"]').setValue(false)
    await botao(wrapper, 'Aplicar 1 campo').trigger('click')
    await flushPromises()

    const applyCall = chamadas.find((c) => c.url.includes('/enrich/apply'))
    expect(applyCall?.body).toEqual({ enrichment_ids: [11], campos: ['legal_name'] })

    // Passo 3 e efeitos no pai.
    expect(wrapper.text()).toContain('1 campo atualizado pela Receita Federal')
    expect(useToastStore().toasts.map((t) => t.message)).toContain('1 campo atualizado pelo enriquecimento')
    wrapper.unmount()
  })

  it('"Marcar todos" e "Marcar só vazios" ajustam a seleção do diff', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompanyDetailView, { global: { stubs } })
    await flushPromises()
    await wrapper.find('.quick-actions button.enrich').trigger('click')
    await flushPromises()
    await botao(wrapper, 'Consultar').trigger('click')
    await flushPromises()

    await botao(wrapper, 'Marcar todos').trigger('click')
    expect(wrapper.findAll('table.diff input[type="checkbox"]').every((c) => (c.element as HTMLInputElement).checked)).toBe(true)
    await botao(wrapper, 'Marcar só vazios').trigger('click')
    expect(wrapper.findAll('table.diff input[type="checkbox"]').map((c) => (c.element as HTMLInputElement).checked)).toEqual([true, true, false])
    wrapper.unmount()
  })

  it('sem CNPJ válido a ação Enriquecer fica desabilitada com dica', async () => {
    vi.stubGlobal('fetch', stubFetch((u) => (u.includes('/companies/1') && !u.includes('/intel') ? { ...company, cnpj: '', enriched_at: null } : undefined)))
    const wrapper = mount(CompanyDetailView, { global: { stubs } })
    await flushPromises()
    const enrich = wrapper.find('.quick-actions button.enrich')
    expect(enrich.attributes('disabled')).toBeDefined()
    expect(enrich.attributes('title')).toBe('Informe o CNPJ para enriquecer')
    expect(chamadas.some((c) => c.url.includes('/companies/1/intel'))).toBe(false)
    wrapper.unmount()
  })
})

describe('Lista de empresas — CNPJ e ação em massa', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    admin()
  })

  it('formulário de nova empresa pré-preenche pelo CNPJ e mostra o chip da Receita', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompaniesView, { global: { stubs } })
    await flushPromises()

    await botao(wrapper, '+ Adicionar empresa').trigger('click')
    await flushPromises()

    const cnpjInput = wrapper.find('.cnpj-field input')
    await cnpjInput.setValue('11222333000181')
    await flushPromises()

    expect((cnpjInput.element as HTMLInputElement).value).toBe('11.222.333/0001-81')
    expect(chamadas.some((c) => c.url.endsWith('/cnpj/11222333000181'))).toBe(true)
    expect(wrapper.text()).toContain('Preenchido pela Receita Federal — revise antes de salvar')

    const inputs = wrapper.findAll('form input')
    const valores = inputs.map((i) => (i.element as HTMLInputElement).value)
    expect(valores).toContain('ACME')
    expect(valores).toContain('ACME VENDAS LTDA')
    expect(valores).toContain('(85) 3333-4444')
    expect(valores).toContain('Desenvolvimento de programas')
    expect(valores).toContain('Rua X')
    wrapper.unmount()
  })

  it('CNPJ inválido mostra erro e não consulta; CNPJ já cadastrado avisa com link', async () => {
    vi.stubGlobal(
      'fetch',
      stubFetch((u) => (u.includes('/cnpj/') ? { perfil, existente: { id: 7, name: 'ACME (já cadastrada)' } } : undefined))
    )
    const wrapper = mount(CompaniesView, { global: { stubs } })
    await flushPromises()
    await botao(wrapper, '+ Adicionar empresa').trigger('click')

    const cnpjInput = wrapper.find('.cnpj-field input')
    await cnpjInput.setValue('11222333000180')
    await flushPromises()
    expect(wrapper.text()).toContain('CNPJ inválido')
    expect(chamadas.some((c) => c.url.includes('/cnpj/'))).toBe(false)

    // O stub de Teleport do test-utils recria o conteúdo do modal a cada render; no app o <input> é o mesmo nó.
    await wrapper.find('.cnpj-field input').setValue('11222333000181')
    await flushPromises()
    expect(wrapper.find('.cnpj-field .exists').text()).toContain('Já existe a empresa ACME (já cadastrada)')
    wrapper.unmount()
  })

  it('seleção na lista dispara "Enriquecer pela Receita" em massa com resumo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(CompaniesView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.find('.bulk-bar').exists()).toBe(false)
    await wrapper.find('tbody .select-col input').setValue(true)
    expect(wrapper.find('.bulk-bar').text()).toContain('1 selecionada')

    await botao(wrapper, '✦ Enriquecer pela Receita').trigger('click')
    await flushPromises()
    await botao(wrapper, '✦ Enriquecer 1').trigger('click')
    await flushPromises()

    const bulk = chamadas.find((c) => c.url.includes('/companies/enrich/bulk'))
    expect(bulk?.body).toEqual({ ids: [1], provedores: ['receita'] })
    expect(wrapper.find('.bulk-summary').text().replace(/\s+/g, ' ')).toBe('1 processadas · 1 enriquecidas · 0 sem CNPJ')
    wrapper.unmount()
  })
})
