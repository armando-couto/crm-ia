import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import SetupWizardView from '../src/views/SetupWizardView.vue'
import { setToken } from '../src/api'
import { useAuthStore } from '../src/stores/auth'

const status = {
  completed: false,
  template: '',
  steps: [],
  workspace: { name: 'Minha Empresa', segment: '', color: '#6d5df6', logo_url: '', website: '', timezone: 'America/Sao_Paulo' },
  email: null,
  sending: { daily_limit: 500, window_start: 8, window_end: 19, weekdays_only: true, paused: false, signature: '', track_opens: true, track_clicks: true, timezone: 'America/Sao_Paulo' },
  users_active: 1,
  users_max: 5,
  tenant: { slug: 'minhaempresa', name: 'Minha Empresa', color: '#6d5df6', logo_url: '', version: 'dev', base_path: '/minhaempresa', app_url: '', plan_enabled: false, setup_done: false }
}

const templates = [
  { codigo: 'vendas_b2b', nome: 'Vendas B2B', descricao: 'Para equipes B2B', icone: 'b', segmentos: ['Software'], destaques: ['Funil com 6 etapas'], pipelines: 1, etapas: 7, emails: 4, cadencias: 2, campos: 5 },
  { codigo: 'em_branco', nome: 'Começar do zero', descricao: 'Só o essencial', icone: 's', segmentos: [], destaques: ['Funil simples'], pipelines: 1, etapas: 5, emails: 2, cadencias: 0, campos: 0 }
]

function mockApi() {
  const calls: { url: string; method: string; body?: any }[] = []
  const fetchMock = vi.fn(async (url: string, options: any = {}) => {
    const method = options.method || 'GET'
    const body = options.body ? JSON.parse(options.body) : undefined
    calls.push({ url, method, body })
    const json = (data: unknown, status = 200) => ({ ok: status < 300, status, text: () => Promise.resolve(JSON.stringify(data)) })
    if (url.endsWith('/setup') && method === 'GET') return json(status)
    if (url.endsWith('/setup/templates')) return json(templates)
    if (url.includes('/setup/templates/')) return json({ resumo: templates[0], pipelines: [{ nome: 'Funil de vendas', etapas: ['Lead novo', 'Qualificado'] }], emails: ['Primeiro contato'], cadencias: [], campos: [] })
    if (url.endsWith('/settings/email')) return json({ settings: null, presets: { mandrill_api: { nome: 'Mandrill', host: '', port: 0, dica: 'chave' } }, active: false, platform_default: false, inbound_webhook: '' })
    if (url.endsWith('/setup/template')) return json({ pipelines: 1, etapas: 7, emails: 4, cadencias: 2, propriedades: 5, snippets: 3, ignorados: 0 })
    if (url.endsWith('/setup/step')) return json({ ...status, steps: ['template'] })
    return json({})
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

describe('SetupWizardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    setToken('tok')
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Admin', email: 'a@exemplo.com.br', role: 'admin', active: true, created_at: '', updated_at: '' } as any
  })
  afterEach(() => {
    vi.restoreAllMocks()
    setToken(null)
  })

  it('lista os modelos do catálogo e aplica o escolhido', async () => {
    const calls = mockApi()
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', name: 'dashboard', component: { template: '<div />' } }] })
    const wrapper = mount(SetupWizardView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Vendas B2B')
    expect(wrapper.text()).toContain('Começar do zero')

    await wrapper.find('[data-template="vendas_b2b"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Lead novo → Qualificado')

    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    const aplicar = calls.find((c) => c.url.endsWith('/setup/template') && c.method === 'POST')
    expect(aplicar?.body).toEqual({ template: 'vendas_b2b' })
    const passo = calls.find((c) => c.url.endsWith('/setup/step'))
    expect(passo?.body).toEqual({ step: 'template' })
    // Avançou para o passo da empresa com o nome já preenchido.
    expect(wrapper.text()).toContain('Como a sua equipe vai ver o CRM')
    expect((wrapper.find('input[required]').element as HTMLInputElement).value).toBe('Minha Empresa')
  })
})
