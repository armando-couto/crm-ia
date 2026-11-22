// Atalho "Sua fila de ligações: N leads" no espaço de trabalho (10.5).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import SalesWorkspaceView from '../src/views/SalesWorkspaceView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const stubs = { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } }

const workspace = {
  overdue_tasks: [{ id: 1, title: 'Ligar para o cliente', subtitle: 'ligacao', due: '2026-08-20T10:00:00Z' }],
  today_tasks: [],
  today_meetings: [],
  stale_deals: [],
  closing_soon: [],
  untouched_leads: [],
  open_deals: 0,
  open_amount: 0,
  won_month: 0,
  goal_month: 0,
  tasks_pending: 1,
  meetings_week: 0,
  target_accounts: 0
}

function resposta(body: unknown, status = 200) {
  return { ok: status < 300, status, headers: { get: () => null }, text: () => Promise.resolve(JSON.stringify(body)) }
}

function loginAs(permissions: Record<string, boolean>) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Ana Silva', email: 'a@exemplo.com.br', role: 'seller', active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
}

function api(fila: () => ReturnType<typeof resposta>) {
  const f = vi.fn((url: string) => Promise.resolve(String(url).includes('/leads/fila/resumo') ? fila() : resposta(workspace)))
  vi.stubGlobal('fetch', f)
  return f
}

describe('atalho da fila no espaço de trabalho', () => {
  beforeEach(() => setActivePinia(createPinia()))
  afterEach(() => vi.unstubAllGlobals())

  it('mostra "Sua fila de ligações: N leads" acima das filas, sem entrar na contagem', async () => {
    loginAs({ 'leads.edit': true, 'tasks.edit': true })
    api(() => resposta({ na_fila: 12, retornos_vencidos: 2, feitas_hoje: 5, meta_dia: 40 }))
    const wrapper = mount(SalesWorkspaceView, { global: { stubs } })
    await flushPromises()

    const atalho = wrapper.find('.fila-atalho')
    expect(atalho.text()).toContain('Sua fila de ligações: 12 leads')
    expect(atalho.attributes('href')).toBe('/fila-de-ligacoes')
    expect(wrapper.text()).toContain('1 item(ns) na sua fila')
    // A fila antiga ganhou o nome novo.
    expect(wrapper.text()).toContain('Contatos novos sem atividade')
    // Nenhuma aba nova.
    expect(wrapper.findAll('.ws-tabs button').length).toBe(6)
  })

  it('tolera resposta sem as chaves', async () => {
    loginAs({ 'leads.edit': true })
    api(() => resposta({}))
    const wrapper = mount(SalesWorkspaceView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('.fila-atalho').text()).toContain('Sua fila de ligações: 0 leads')
  })

  it('some quando a rota falha', async () => {
    loginAs({ 'leads.edit': true })
    api(() => resposta({ error: 'falhou' }, 500))
    const wrapper = mount(SalesWorkspaceView, { global: { stubs } })
    await flushPromises()
    expect(wrapper.find('.fila-atalho').exists()).toBe(false)
    expect(wrapper.text()).toContain('Tarefas atrasadas')
  })

  it('sem leads.edit não consulta a fila', async () => {
    loginAs({ 'tasks.edit': true })
    const f = api(() => resposta({ na_fila: 3 }))
    const wrapper = mount(SalesWorkspaceView, { global: { stubs } })
    await flushPromises()
    expect(f.mock.calls.some((c) => String(c[0]).includes('/leads/fila/resumo'))).toBe(false)
    expect(wrapper.find('.fila-atalho').exists()).toBe(false)
  })
})
