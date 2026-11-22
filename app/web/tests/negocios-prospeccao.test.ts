// Negócios com a prospecção (docs/PROSPECCAO.md seção 9 e 10.5 + adendo):
// semáforo de próxima atividade, filtro "Sem próxima atividade" e o diálogo
// único de motivo de perda no quadro e na ficha.
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../src/stores/auth'
import DealsBoardView from '../src/views/DealsBoardView.vue'
import DealDetailView from '../src/views/DealDetailView.vue'
import LostReasonDialog from '../src/components/deals/LostReasonDialog.vue'
import NextActivityLight from '../src/components/deals/NextActivityLight.vue'

const { routeMock } = vi.hoisted(() => ({ routeMock: { params: {} as Record<string, string> } }))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => routeMock
}))

const pipelines = [
  {
    id: 1,
    name: 'Pipeline de Vendas',
    position: 0,
    stages: [
      { id: 1, pipeline_id: 1, name: 'Novo Lead', position: 0, probability: 10, is_won: false, is_lost: false },
      { id: 2, pipeline_id: 1, name: 'Negociação', position: 1, probability: 80, is_won: false, is_lost: false },
      { id: 3, pipeline_id: 1, name: 'Ganho', position: 2, probability: 100, is_won: true, is_lost: false },
      { id: 4, pipeline_id: 1, name: 'Perdido', position: 3, probability: 0, is_won: false, is_lost: true }
    ]
  }
]

function diasAPartirDeHoje(dias: number, hora = 12): string {
  const d = new Date()
  d.setDate(d.getDate() + dias)
  d.setHours(hora, 0, 0, 0)
  return d.toISOString()
}

function dealBase(over: { id: number; name: string } & Record<string, unknown>) {
  return {
    amount: 1000,
    currency: 'BRL',
    pipeline_id: 1,
    stage_id: 1,
    stage_name: 'Novo Lead',
    contact_id: null,
    company_id: null,
    owner_id: 1,
    status: 'aberto',
    temperature: '',
    close_date: null,
    position: 0,
    closed_at: null,
    last_activity_at: null,
    created_at: '',
    updated_at: '',
    proxima_atividade_tipo: 'ligacao',
    ...over
  }
}

const deals = [
  dealBase({ id: 1, name: 'Atrasado SA', proxima_atividade_em: diasAPartirDeHoje(-2) }),
  dealBase({ id: 2, name: 'Hoje Ltda', proxima_atividade_em: diasAPartirDeHoje(0, 15) }),
  dealBase({ id: 3, name: 'Futuro ME', proxima_atividade_em: diasAPartirDeHoje(5) }),
  dealBase({ id: 4, name: 'Parado EPP', stage_id: 2, stage_name: 'Negociação', proxima_atividade_em: null, proxima_atividade_tipo: '' })
]

type Chamada = { url: string; method: string; body: any }
const chamadas: Chamada[] = []

function stubFetch(opts: { motivos?: unknown; falhaStage?: boolean } = {}) {
  chamadas.length = 0
  return vi.fn().mockImplementation((url: string, init?: RequestInit) => {
    const u = String(url)
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : null
    chamadas.push({ url: u, method, body })
    let resp: unknown = []
    let status = 200
    if (u.includes('/settings/motivos-perda')) resp = opts.motivos ?? { motivos: ['Preço', 'Sem orçamento', 'Outro'], exigir: true }
    else if (method === 'PATCH' && /\/deals\/\d+\/(close|stage)/.test(u)) {
      const id = Number(u.match(/\/deals\/(\d+)\//)![1])
      const d = deals.find((x) => x.id === id)!
      if (opts.falhaStage && u.includes('/stage')) {
        status = 422
        resp = { error: 'informe o motivo da perda', codigo: 'motivo_obrigatorio' }
      } else {
        resp = {
          ...d,
          status: body?.won === true ? 'ganho' : body?.stage_id === 1 || body?.stage_id === 2 ? 'aberto' : 'perdido',
          stage_id: body?.stage_id ?? 4,
          lost_reason: body?.motivo ?? '',
          lost_note: body?.observacao ?? ''
        }
      }
    } else if (u.includes('/pipelines')) resp = pipelines
    else if (u.includes('/deals/board')) resp = { data: deals }
    else if (u.includes('/deals/1')) resp = { ...deals[0], name: 'Negócio da ficha' }
    else if (u.includes('/tasks') || u.includes('/meetings') || u.includes('/activities')) resp = { data: [], pagination: { page: 1, per_page: 50, total: 0 } }
    else if (u.includes('/contacts?') || u.includes('/companies?')) resp = { data: [], pagination: { page: 1, per_page: 100, total: 0 } }
    return Promise.resolve({ ok: status < 400, status, text: () => Promise.resolve(JSON.stringify(resp)) })
  })
}

function montarQuadro() {
  return mount(DealsBoardView, {
    global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true } }
  })
}

function cartao(wrapper: ReturnType<typeof montarQuadro>, nome: string) {
  return wrapper.findAll('.deal-card').find((c) => c.text().includes(nome))!
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  routeMock.params = {}
  const auth = useAuthStore()
  auth.user = { id: 1, name: 'Admin', email: 'a@b.c', role: 'admin', active: true, created_at: '', updated_at: '' } as any
})

describe('NextActivityLight', () => {
  it('pinta vermelho, verde, cinza e amarelo conforme a próxima atividade', () => {
    const casos: [string | null, string, string][] = [
      [diasAPartirDeHoje(-1), 'atrasado', 'red'],
      [diasAPartirDeHoje(0, 18), 'hoje', 'green'],
      [diasAPartirDeHoje(3), 'futuro', 'gray'],
      [null, 'nenhuma', 'amber']
    ]
    for (const [em, urgencia, cor] of casos) {
      const w = mount(NextActivityLight, { props: { em, tipo: 'ligacao' } })
      expect(w.attributes('data-urgencia')).toBe(urgencia)
      expect(w.find('.semaforo').classes()).toContain(cor)
      w.unmount()
    }
    const vazio = mount(NextActivityLight, { props: { em: null } })
    expect(vazio.text()).toBe('Sem próxima atividade')
  })
})

describe('Quadro de negócios: semáforo e filtro', () => {
  it('mostra o semáforo em cada cartão e filtra os sem próxima atividade', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montarQuadro()
    await flushPromises()

    expect(cartao(wrapper, 'Atrasado SA').find('[data-urgencia]').attributes('data-urgencia')).toBe('atrasado')
    expect(cartao(wrapper, 'Hoje Ltda').find('[data-urgencia]').attributes('data-urgencia')).toBe('hoje')
    expect(cartao(wrapper, 'Futuro ME').find('[data-urgencia]').attributes('data-urgencia')).toBe('futuro')
    expect(cartao(wrapper, 'Parado EPP').find('[data-urgencia]').attributes('data-urgencia')).toBe('nenhuma')

    const filtro = wrapper.find('.no-next-btn')
    expect(filtro.text()).toContain('Sem próxima atividade')
    expect(filtro.find('.no-next-count').text()).toBe('1')
    await filtro.trigger('click')

    const nomes = wrapper.findAll('.deal-card').map((c) => c.find('strong').text())
    expect(nomes).toEqual(['Parado EPP'])
    expect(filtro.attributes('aria-pressed')).toBe('true')
    wrapper.unmount()
  })
})

describe('Motivo de perda no quadro', () => {
  it('botão ✕ abre o diálogo; sem motivo não envia; com motivo manda close com motivo e observação', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montarQuadro()
    await flushPromises()

    await cartao(wrapper, 'Futuro ME').find('.deal-actions .lose').trigger('click')
    await flushPromises()
    const dialogo = wrapper.find('[data-dialogo="perda"]')
    expect(dialogo.exists()).toBe(true)
    expect(dialogo.text()).toContain('Futuro ME')
    expect(wrapper.findAll('input[name="motivo-perda"]').map((i) => (i.element as HTMLInputElement).value)).toEqual(['Preço', 'Sem orçamento', 'Outro'])

    await dialogo.trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Escolha o motivo da perda.')
    expect(chamadas.some((c) => c.method === 'PATCH')).toBe(false)

    await wrapper.findAll('input[name="motivo-perda"]')[1].setValue(true)
    await wrapper.find('#perda-observacao').setValue('  cliente congelou o orçamento ')
    await wrapper.find('[data-dialogo="perda"]').trigger('submit')
    await flushPromises()

    const patch = chamadas.find((c) => c.method === 'PATCH')!
    expect(patch.url).toContain('/deals/3/close')
    expect(patch.body).toEqual({ won: false, motivo: 'Sem orçamento', observacao: 'cliente congelou o orçamento' })
    expect(wrapper.find('[data-dialogo="perda"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('ganho continua mandando só { won: true }, sem abrir o diálogo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montarQuadro()
    await flushPromises()
    await cartao(wrapper, 'Hoje Ltda').find('.deal-actions .win').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-dialogo="perda"]').exists()).toBe(false)
    expect(chamadas.find((c) => c.method === 'PATCH')!.body).toEqual({ won: true })
    wrapper.unmount()
  })

  it('soltar o cartão numa etapa de perda abre o diálogo; cancelar devolve o cartão sem chamar a API', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montarQuadro()
    await flushPromises()

    await cartao(wrapper, 'Hoje Ltda').trigger('dragstart', { dataTransfer: { setData: vi.fn(), effectAllowed: '' } })
    const perdido = wrapper.find('.column.lost')
    await perdido.trigger('drop', { dataTransfer: { getData: () => '2' } })
    await flushPromises()

    expect(wrapper.find('[data-dialogo="perda"]').exists()).toBe(true)
    await wrapper.find('[data-acao="cancelar"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-dialogo="perda"]').exists()).toBe(false)
    expect(chamadas.some((c) => c.method === 'PATCH')).toBe(false)
    const novoLead = wrapper.findAll('.column').find((c) => c.text().includes('Novo Lead'))!
    expect(novoLead.text()).toContain('Hoje Ltda')
    wrapper.unmount()
  })

  it('confirmar no arraste manda PATCH /stage com o motivo', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = montarQuadro()
    await flushPromises()

    await cartao(wrapper, 'Hoje Ltda').trigger('dragstart', { dataTransfer: { setData: vi.fn(), effectAllowed: '' } })
    const perdido = wrapper.find('.column.lost')
    await perdido.trigger('drop', { dataTransfer: { getData: () => '2' } })
    await flushPromises()

    await wrapper.findAll('input[name="motivo-perda"]')[0].setValue(true)
    await wrapper.find('[data-dialogo="perda"]').trigger('submit')
    await flushPromises()

    const patch = chamadas.find((c) => c.method === 'PATCH')!
    expect(patch.url).toContain('/deals/2/stage')
    expect(patch.body).toMatchObject({ stage_id: 4, motivo: 'Preço' })
    expect(wrapper.find('[data-dialogo="perda"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

describe('LostReasonDialog', () => {
  it('com exigir desligado aceita confirmar sem motivo', async () => {
    vi.stubGlobal('fetch', stubFetch({ motivos: { motivos: ['Preço'], exigir: false } }))
    const wrapper = mount(LostReasonDialog, { props: { open: true, nome: 'X' }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('(opcional)')
    await wrapper.find('[data-dialogo="perda"]').trigger('submit')
    expect(wrapper.emitted('confirm')?.[0]).toEqual([{}])
    wrapper.unmount()
  })

  it('sem resposta da configuração usa os motivos padrão do contrato', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500, text: () => Promise.resolve('{"error":"x"}') }))
    const wrapper = mount(LostReasonDialog, { props: { open: true }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.findAll('input[name="motivo-perda"]').length).toBe(7)
    wrapper.unmount()
  })
})

describe('Motivo de perda na ficha do negócio', () => {
  it('"Perdido" abre o diálogo e manda close com o motivo; a ficha mostra o motivo gravado', async () => {
    routeMock.params = { id: '1' }
    vi.stubGlobal('fetch', stubFetch())
    const wrapper = mount(DealDetailView, {
      global: { stubs: { 'router-link': { template: '<a><slot /></a>' }, Teleport: true, CustomProperties: true, AttachmentsPanel: true } }
    })
    await flushPromises()

    const perdido = wrapper.findAll('.close-actions button').find((b) => b.text().includes('Perdido'))!
    await perdido.trigger('click')
    await flushPromises()
    expect(chamadas.some((c) => c.method === 'PATCH')).toBe(false)
    expect(wrapper.find('[data-dialogo="perda"]').text()).toContain('Negócio da ficha')

    await wrapper.findAll('input[name="motivo-perda"]')[0].setValue(true)
    await wrapper.find('[data-dialogo="perda"]').trigger('submit')
    await flushPromises()

    const patch = chamadas.find((c) => c.method === 'PATCH')!
    expect(patch.url).toContain('/deals/1/close')
    expect(patch.body).toEqual({ won: false, motivo: 'Preço' })
    expect(wrapper.find('[data-dialogo="perda"]').exists()).toBe(false)
    expect(wrapper.find('[data-campo="motivo-perda"]').text()).toBe('Preço')
    wrapper.unmount()
  })
})
