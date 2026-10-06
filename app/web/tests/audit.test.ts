import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import SettingsAuditView from '../src/views/SettingsAuditView.vue'
import SettingsLayout from '../src/views/SettingsLayout.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} }),
  RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
  RouterView: { template: '<div />' }
}))

const entries = [
  {
    id: 2,
    user_id: 1,
    user_name: 'Ana',
    action: 'exportar',
    entity: 'contato',
    entity_id: null,
    summary: 'exportou 320 contatos em CSV',
    ip: '10.0.0.9',
    created_at: '2026-08-30T12:00:00Z'
  },
  {
    id: 1,
    user_id: 3,
    user_name: 'Bruno',
    action: 'login_falha',
    entity: 'usuario',
    entity_id: 3,
    summary: 'senha incorreta',
    ip: '200.1.1.1',
    created_at: '2026-08-30T11:00:00Z'
  }
]

function stubFetch() {
  return vi.fn().mockImplementation((url: string) => {
    let body: unknown = {}
    if (String(url).includes('/audit')) {
      body = { data: entries, total: 2, page: 1, per_page: 50, actions: ['login', 'exportar', 'login_falha'] }
    } else if (String(url).includes('/users')) {
      body = [{ id: 1, name: 'Ana', email: 'ana@exemplo.com.br', role: 'admin', active: true }]
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

// O menu usa router-link/router-view: fora do router real precisam de stub.
const routerStubs = {
  RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
  RouterView: true
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Teste', email: 't@exemplo.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
  return auth
}

describe('SettingsAuditView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
    vi.stubGlobal('fetch', stubFetch())
  })

  it('lista os registros com o rótulo da ação', async () => {
    const wrapper = mount(SettingsAuditView)
    await flushPromises()

    const html = wrapper.html()
    expect(html).toContain('exportou 320 contatos em CSV')
    expect(html).toContain('Exportação')
    expect(html).toContain('Falha de acesso')
    expect(html).toContain('10.0.0.9')
  })

  it('mostra o total e a paginação', async () => {
    const wrapper = mount(SettingsAuditView)
    await flushPromises()

    expect(wrapper.text()).toContain('2 registro(s)')
    expect(wrapper.text()).toContain('página 1 de 1')
  })

  it('recarrega ao mudar o filtro de ação', async () => {
    const wrapper = mount(SettingsAuditView)
    await flushPromises()
    const calls = (globalThis.fetch as any).mock.calls.length

    await wrapper.findAll('select')[1].setValue('exportar')
    await flushPromises()

    const urls = (globalThis.fetch as any).mock.calls.map((c: any[]) => String(c[0]))
    expect((globalThis.fetch as any).mock.calls.length).toBeGreaterThan(calls)
    expect(urls.some((u: string) => u.includes('action=exportar'))).toBe(true)
  })
})

describe('menu de configurações', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('mostra Auditoria para quem tem a permissão', () => {
    loginAs('manager', { 'settings.audit': true })
    const wrapper = mount(SettingsLayout, { global: { stubs: routerStubs } })
    expect(wrapper.html()).toContain('Auditoria')
  })

  it('esconde Auditoria de quem não tem a permissão', () => {
    loginAs('seller', { 'contacts.view': true })
    const wrapper = mount(SettingsLayout, { global: { stubs: routerStubs } })
    expect(wrapper.html()).not.toContain('Auditoria')
  })
})
