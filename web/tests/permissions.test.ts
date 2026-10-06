import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import SettingsPermissionsView from '../src/views/SettingsPermissionsView.vue'
import AppShell from '../src/components/AppShell.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: {}, query: {} })
}))

const catalog = [
  { key: 'contacts.view', label: 'Ver contatos', group: 'Contatos' },
  { key: 'contacts.delete', label: 'Excluir contatos', group: 'Contatos' },
  { key: 'settings.users', label: 'Gerenciar usuários e equipes', group: 'Configurações' }
]

const matrix = {
  seller: { 'contacts.view': true, 'contacts.delete': false, 'settings.users': false },
  manager: { 'contacts.view': true, 'contacts.delete': true, 'settings.users': false },
  admin: { 'contacts.view': true, 'contacts.delete': true, 'settings.users': true }
}

function stubFetch() {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    let body: unknown = {}
    if (String(url).includes('/permissions')) {
      body =
        options?.method === 'PUT'
          ? { matrix }
          : { catalog, roles: ['seller', 'manager', 'admin'], labels: { seller: 'Seller', manager: 'Manager', admin: 'Admin' }, matrix }
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

function loginAs(role: string, permissions: Record<string, boolean> = {}) {
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Teste', email: 't@fixpay.com.br', role, active: true, created_at: '', updated_at: '' } as any
  auth.permissions = permissions
  return auth
}

describe('SettingsPermissionsView (matriz)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    loginAs('admin')
  })

  it('renderiza a matriz agrupada com os três perfis', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsPermissionsView)
    await flushPromises()

    const headers = wrapper.findAll('thead th').map((h) => h.text())
    expect(headers[1]).toContain('Seller')
    expect(headers[2]).toContain('Manager')
    expect(headers[3]).toContain('Admin')
    expect(wrapper.text()).toContain('Contatos')
    expect(wrapper.text()).toContain('Excluir contatos')
    wrapper.unmount()
  })

  it('coluna do Admin fica travada em acesso total', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(SettingsPermissionsView)
    await flushPromises()

    const row = wrapper.findAll('.perm-row')[1] // Excluir contatos
    const boxes = row.findAll('input[type="checkbox"]')
    expect((boxes[0].element as HTMLInputElement).checked).toBe(false) // seller
    expect((boxes[1].element as HTMLInputElement).checked).toBe(true) // manager
    expect((boxes[2].element as HTMLInputElement).checked).toBe(true) // admin
    expect(boxes[2].attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('salva apenas os perfis editáveis via PUT', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SettingsPermissionsView)
    await flushPromises()

    // Marca "Excluir contatos" para o seller e salva.
    const row = wrapper.findAll('.perm-row')[1]
    await row.findAll('input[type="checkbox"]')[0].trigger('change')
    await wrapper.find('.page-head .btn-primary').trigger('click')
    await flushPromises()

    const puts = fetchMock.mock.calls.filter((c: any[]) => c[1]?.method === 'PUT')
    const roles = puts.map((c: any[]) => JSON.parse(c[1].body).role)
    expect(roles).toEqual(['seller', 'manager']) // admin não é enviado
    expect(JSON.parse(puts[0][1].body).permissions['contacts.delete']).toBe(true)
    wrapper.unmount()
  })
})

describe('Menu lateral respeita as permissões', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('esconde itens sem permissão para o Seller', async () => {
    loginAs('seller', { 'contacts.view': true, 'deals.view': true, 'dashboard.view': true })
    const wrapper = mount(AppShell, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, GlobalSearch: true } }
    })

    const text = wrapper.text()
    expect(text).toContain('Contatos')
    expect(text).toContain('Negócios')
    expect(text).not.toContain('Empresas')
    expect(text).not.toContain('Caixa de entrada')
    wrapper.unmount()
  })

  it('Admin enxerga tudo mesmo sem permissões carregadas', async () => {
    loginAs('admin', {})
    const wrapper = mount(AppShell, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, GlobalSearch: true } }
    })

    const text = wrapper.text()
    expect(text).toContain('Contatos')
    expect(text).toContain('Empresas')
    expect(text).toContain('Caixa de entrada')
    expect(text).toContain('Manuais')
    wrapper.unmount()
  })
})
