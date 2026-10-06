import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import PublicBookingView from '../src/views/PublicBookingView.vue'
import SettingsBookingView from '../src/views/SettingsBookingView.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ params: { slug: 'ana' }, query: {} })
}))

const definition = {
  slug: 'ana',
  title: 'Agende uma conversa',
  description: 'Vamos falar sobre maquininhas.',
  location: 'Google Meet',
  duration_min: 30,
  host_name: 'Ana Silva',
  days: [
    { date: '2026-09-07', slots: ['09:00', '09:30', '10:00'] },
    { date: '2026-09-08', slots: ['14:00'] }
  ]
}

function stubPublicFetch(bookResponse: any = { message: 'Agendamento confirmado!' }, ok = true) {
  return vi.fn().mockImplementation((_url: string, options?: any) => {
    if (options?.method === 'POST') {
      return Promise.resolve({
        ok,
        status: ok ? 200 : 409,
        json: () => Promise.resolve(bookResponse)
      })
    }
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(definition) })
  })
}

describe('PublicBookingView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('mostra os dias e os horários do primeiro dia', async () => {
    vi.stubGlobal('fetch', stubPublicFetch())
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    expect(wrapper.text()).toContain('Ana Silva')
    expect(wrapper.text()).toContain('30 minutos')
    expect(wrapper.findAll('.day')).toHaveLength(2)
    expect(wrapper.findAll('.slot')).toHaveLength(3)
  })

  it('troca os horários ao mudar de dia', async () => {
    vi.stubGlobal('fetch', stubPublicFetch())
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    await wrapper.findAll('.day')[1].trigger('click')
    expect(wrapper.findAll('.slot')).toHaveLength(1)
    expect(wrapper.findAll('.slot')[0].text()).toBe('14:00')
  })

  it('só pede os dados depois de escolher o horário', async () => {
    vi.stubGlobal('fetch', stubPublicFetch())
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    expect(wrapper.find('form').exists()).toBe(false)
    await wrapper.findAll('.slot')[0].trigger('click')
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('envia data, hora e dados do cliente', async () => {
    const fetchMock = stubPublicFetch()
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    await wrapper.findAll('.slot')[1].trigger('click')
    await wrapper.find('#name').setValue('João Cliente')
    await wrapper.find('#email').setValue('joao@cliente.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const post = fetchMock.mock.calls.find((c: any[]) => c[1]?.method === 'POST')
    const body = JSON.parse(post![1].body)
    expect(body).toMatchObject({
      name: 'João Cliente',
      email: 'joao@cliente.com',
      date: '2026-09-07',
      time: '09:30',
      _gotcha: ''
    })
    expect(wrapper.text()).toContain('Agendamento confirmado!')
  })

  it('avisa quando o horário foi ocupado no meio do caminho', async () => {
    vi.stubGlobal('fetch', stubPublicFetch({ error: 'esse horário acabou de ser ocupado' }, false))
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    await wrapper.findAll('.slot')[0].trigger('click')
    await wrapper.find('#name').setValue('João')
    await wrapper.find('#email').setValue('joao@cliente.com')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('acabou de ser ocupado')
  })

  it('avisa quando não há horário livre', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ ...definition, days: [] })
      })
    )
    const wrapper = mount(PublicBookingView)
    await flushPromises()

    expect(wrapper.text()).toContain('Nenhum horário livre')
  })
})

const page = {
  id: 1,
  user_id: 1,
  slug: 'ana',
  title: 'Agende uma conversa',
  description: '',
  location: 'Google Meet',
  duration_min: 30,
  buffer_min: 0,
  days_ahead: 14,
  notice_hours: 4,
  weekly_hours: { '1': [{ start: '09:00', end: '18:00' }] },
  active: true,
  bookings: 0,
  created_at: '',
  updated_at: ''
}

describe('SettingsBookingView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    auth.user = { id: 1, name: 'Ana', email: 'a@exemplo.com.br', role: 'admin' } as any
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, options?: any) => {
        const body = options?.method === 'PUT' ? page : { page, exists: true, base_url: 'https://crmia.com.br/suaempresa' }
        return Promise.resolve({ ok: true, status: 200, text: () => Promise.resolve(JSON.stringify(body)) })
      })
    )
  })

  it('monta o link público a partir do slug', async () => {
    const wrapper = mount(SettingsBookingView)
    await flushPromises()

    const input = wrapper.find('.link-card input').element as HTMLInputElement
    expect(input.value).toBe('https://crmia.com.br/suaempresa/agendar/ana')
  })

  it('mostra os dias abertos e os fechados', async () => {
    const wrapper = mount(SettingsBookingView)
    await flushPromises()

    // Segunda aberta com uma faixa; os outros dias aparecem como fechados.
    expect(wrapper.findAll('.window')).toHaveLength(1)
    expect(wrapper.findAll('.closed').length).toBeGreaterThan(0)
  })

  it('abre e fecha um dia da semana', async () => {
    const wrapper = mount(SettingsBookingView)
    await flushPromises()

    const vm = wrapper.vm as any
    vm.toggleDay('2')
    await flushPromises()
    expect(vm.page.weekly_hours['2']).toHaveLength(1)

    vm.toggleDay('2')
    await flushPromises()
    expect(vm.page.weekly_hours['2']).toBeUndefined()
  })
})
