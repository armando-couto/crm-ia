import { computed, onScopeDispose, ref, type ComputedRef, type Ref } from 'vue'
import { api, ApiError, foiAbortado } from '../api'
import { jobAtivo, type Job } from '../types-prospeccao'

export interface JobPollingOptions {
  /** Intervalo entre consultas, em ms (o contrato pede 2 s). */
  intervalo?: number
  /**
   * Chamado a cada mudança de `feitos`, `erros` ou `status` (e uma vez ao
   * terminar): é a hora de recarregar a lista e a página atual de itens.
   */
  aoMudar?: (job: Job) => void
  /** Chamado uma vez quando o job sai de `pendente`/`rodando`. */
  aoTerminar?: (job: Job) => void
}

export interface JobPolling {
  /** Último estado conhecido do job acompanhado (ou `null`). */
  job: Ref<Job | null>
  /** Há job `pendente` ou `rodando`: a tela desabilita as ações. */
  ativo: ComputedRef<boolean>
  /** O intervalo está rodando. */
  consultando: Ref<boolean>
  /** Última falha de consulta (rede); a consulta continua tentando. */
  erro: Ref<string>
  /** 0 a 100, pelo `feitos` de `total`. */
  percentual: ComputedRef<number>
  /** Começa (ou troca) o acompanhamento. Job já terminado só é guardado. */
  acompanhar: (job: Job | number) => void
  /** Para de consultar sem mexer no job. */
  parar: () => void
  /** POST /prospeccao/jobs/{id}/cancelar e segue acompanhando até o worker parar. */
  cancelar: () => Promise<Job | null>
}

/**
 * Acompanha um job da prospecção consultando GET /prospeccao/jobs/{id}.
 *
 * - Uma consulta por vez (o próximo tique só é agendado depois da resposta).
 * - Para sozinho quando o job termina e quando o componente (ou o escopo de
 *   efeito) é desmontado, cancelando a consulta em voo.
 * - Sobrevive a recarregar a página: chame `acompanhar(job)` com o job ativo
 *   que vier em `GET /prospeccao/listas/{id}`.jobs.
 */
export function useJobPolling(opcoes: JobPollingOptions = {}): JobPolling {
  const intervalo = opcoes.intervalo ?? 2000
  const job = ref<Job | null>(null)
  const consultando = ref(false)
  const erro = ref('')

  let timer: ReturnType<typeof setTimeout> | null = null
  let controle: AbortController | null = null
  // Cada acompanhar()/parar() abre uma geração nova; resposta de geração antiga é descartada.
  let geracao = 0

  const ativo = computed(() => jobAtivo(job.value))
  const percentual = computed(() => {
    const j = job.value
    if (!j || j.total <= 0) return 0
    return Math.min(100, Math.max(0, Math.round((j.feitos / j.total) * 100)))
  })

  function limpar() {
    if (timer) clearTimeout(timer)
    timer = null
    controle?.abort()
    controle = null
  }

  function parar() {
    geracao++
    limpar()
    consultando.value = false
  }

  function receber(novo: Job) {
    const antes = job.value
    job.value = novo
    const mudou = !antes || antes.id !== novo.id || antes.feitos !== novo.feitos || antes.erros !== novo.erros || antes.status !== novo.status
    if (mudou) opcoes.aoMudar?.(novo)
  }

  function agendar(id: number, minha: number) {
    timer = setTimeout(() => consultar(id, minha), intervalo)
  }

  async function consultar(id: number, minha: number) {
    timer = null
    controle = new AbortController()
    try {
      const novo = await api.get<Job>(`/prospeccao/jobs/${id}`, { signal: controle.signal })
      if (minha !== geracao) return
      erro.value = ''
      receber(novo)
      if (!jobAtivo(novo)) {
        consultando.value = false
        opcoes.aoTerminar?.(novo)
        return
      }
    } catch (e: any) {
      if (minha !== geracao || foiAbortado(e)) return
      // Job apagado (lista excluída) ou sessão caída: não adianta insistir.
      if (e instanceof ApiError && (e.status === 404 || e.status === 401 || e.status === 403)) {
        erro.value = e.message
        consultando.value = false
        return
      }
      erro.value = e?.message || 'não foi possível consultar o andamento'
    }
    if (minha === geracao) agendar(id, minha)
  }

  function acompanhar(alvo: Job | number) {
    parar()
    const minha = geracao
    if (typeof alvo === 'number') {
      consultando.value = true
      void consultar(alvo, minha)
      return
    }
    job.value = alvo
    if (!jobAtivo(alvo)) return
    consultando.value = true
    agendar(alvo.id, minha)
  }

  async function cancelar(): Promise<Job | null> {
    const atual = job.value
    if (!atual) return null
    const novo = await api.post<Job>(`/prospeccao/jobs/${atual.id}/cancelar`)
    if (job.value?.id !== novo.id) return novo
    receber(novo)
    if (!jobAtivo(novo)) {
      parar()
      opcoes.aoTerminar?.(novo)
    }
    return novo
  }

  onScopeDispose(parar)

  return { job, ativo, consultando, erro, percentual, acompanhar, parar, cancelar }
}
