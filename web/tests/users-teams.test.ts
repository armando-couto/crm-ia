import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SettingsUsersView from '../src/views/SettingsUsersView.vue'

const users = [
  {
    id: 1,
    name: 'Eliseu Becco',
    email: 'eliseu@fixpay.com.br',
    role: 'admin',
    active: true,
    team_id: 1,
    team_name: 'Comercial - Closers',
    created_at: new Date().toISOString(),
    updated_at: ''
  },
  {
    id: 2,
    name: 'Fabio Militao',
    email: 'fabio@fixpay.com.br',
    role: 'vendedor',
    active: true,
    team_id: 1,
    team_name: 'Comercial - Closers',
    created_at: new Date().toISOString(),
    updated_at: ''
  }
]

const teams = [
  { id: 1, name: 'Comercial - Closers', members_count: 2, created_at: '' },
  { id: 2, name: 'Customer Success (CS)', members_count: 0, created_at: '' }
]

function stubFetch() {
  return vi.fn().mockImplementation((url: string, options?: any) => {
    const u = String(url)
    let body: unknown = {}
    if (u.includes('/users') && (!options?.method || options.method === 'GET')) {
      body = users
    } else if (u.includes('/teams') && (!options?.method || options.method === 'GET')) {
      body = teams
    }
    return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
  })
}

describe('SettingsUsersView (usuários e equipes)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  function build() {
    return mount(SettingsUsersView, {
      global: { stubs: { Teleport: true } }
    })
  }

  it('lista usuários com a coluna de equipe principal', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    expect(wrapper.text()).toContain('Eliseu Becco')
    expect(wrapper.text()).toContain('Comercial - Closers')
    expect(wrapper.text()).toContain('2 usuário(s) ativo(s) · 2 equipe(s)')
    wrapper.unmount()
  })

  it('aba Equipes mostra os membros por iniciais', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = build()
    await flushPromises()

    await wrapper.findAll('.tab')[1].trigger('click')
    expect(wrapper.text()).toContain('Customer Success (CS)')

    const closersRow = wrapper.findAll('tbody tr').find((r) => r.text().includes('Comercial - Closers'))!
    const avatars = closersRow.findAll('.member-avatar').map((a) => a.text())
    expect(avatars).toEqual(['EB', 'FM'])

    const csRow = wrapper.findAll('tbody tr').find((r) => r.text().includes('Customer Success'))!
    expect(csRow.text()).toContain('nenhum membro')
    wrapper.unmount()
  })

  it('modal de criação tem o campo Equipe e envia team_id', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = build()
    await flushPromises()

    await wrapper.find('.page-head .btn-primary').trigger('click')
    expect(wrapper.text()).toContain('Criar um novo usuário')
    expect(wrapper.text()).toContain('Equipe')
    expect(wrapper.text()).toContain('receberá um convite por e-mail')

    await wrapper.find('input[type="text"], input:not([type])').setValue('Nova Pessoa')
    await wrapper.find('input[type="email"]').setValue('nova@fixpay.com.br')
    const selects = wrapper.findAll('.field select')
    await selects[1].setValue('1') // equipe Comercial - Closers
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const postCall = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    expect(postCall).toBeTruthy()
    expect(JSON.parse(postCall![1].body).team_id).toBe(1)
    wrapper.unmount()
  })

  it('reenvia a senha do usuário depois de confirmar', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))
    const wrapper = build()
    await flushPromises()

    const row = wrapper.findAll('tbody tr').find((r) => r.text().includes('Fabio Militao'))!
    const botao = row.findAll('button').find((b) => b.text() === 'Reenviar senha')!
    expect(botao).toBeTruthy()
    await botao.trigger('click')
    await flushPromises()

    const call = fetchMock.mock.calls.find((c: any[]) => String(c[0]).includes('/resend-invite'))
    expect(call).toBeTruthy()
    expect(String(call![0])).toBe('/api/v1/users/2/resend-invite')
    expect(call![1].method).toBe('POST')
    wrapper.unmount()
  })

  it('não reenvia se o usuário cancelar a confirmação', async () => {
    const fetchMock = stubFetch()
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false))
    const wrapper = build()
    await flushPromises()

    const row = wrapper.findAll('tbody tr').find((r) => r.text().includes('Fabio Militao'))!
    await row.findAll('button').find((b) => b.text() === 'Reenviar senha')!.trigger('click')
    await flushPromises()

    expect(fetchMock.mock.calls.some((c: any[]) => String(c[0]).includes('/resend-invite'))).toBe(false)
    wrapper.unmount()
  })
})
