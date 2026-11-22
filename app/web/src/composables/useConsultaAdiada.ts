import { onScopeDispose, ref, type Ref } from 'vue'
import { ApiError, foiAbortado } from '../api'

export interface ConsultaAdiadaOptions<T> {
  /** Espera depois da última alteração antes de consultar (contagem ao vivo: 400 ms). */
  atraso?: number
  /**
   * 429 `muitas_requisicoes`: mantém o último resultado e tenta de novo
   * depois de `Retry-After`, sem erro na tela. Padrão: ligado.
   */
  repetirEmLimite?: boolean
  aoErro?: (e: ApiError | Error) => void
  aoResultado?: (dados: T) => void
}

export interface ConsultaAdiada<A, T> {
  /** Último resultado válido (fica na tela enquanto a nova consulta roda). */
  dados: Ref<T | null>
  carregando: Ref<boolean>
  erro: Ref<string>
  /** Agenda a consulta com o atraso; chamadas seguidas reiniciam a espera. */
  pedir: (args: A) => void
  /** Consulta já, sem esperar. */
  agora: (args: A) => Promise<void>
  /** Cancela o que estiver agendado ou em voo e limpa o resultado. */
  limpar: () => void
}

/**
 * Consulta "ao vivo" enquanto o usuário mexe num formulário: espera o usuário
 * parar, cancela a consulta anterior (AbortSignal) e descarta resposta
 * atrasada — só o resultado do pedido mais recente chega em `dados`.
 *
 *   const contagem = useConsultaAdiada((f: Filtros, signal) =>
 *     api.post<Contagem>('/prospeccao/contagem', { filtros: f }, { signal }))
 *   watch(filtros, (f) => (temFiltroObrigatorio(f) ? contagem.pedir(f) : contagem.limpar()), { deep: true })
 */
export function useConsultaAdiada<A, T>(
  consultar: (args: A, signal: AbortSignal) => Promise<T>,
  opcoes: ConsultaAdiadaOptions<T> = {}
): ConsultaAdiada<A, T> {
  const atraso = opcoes.atraso ?? 400
  const repetir = opcoes.repetirEmLimite ?? true
  const dados = ref<T | null>(null) as Ref<T | null>
  const carregando = ref(false)
  const erro = ref('')

  let timer: ReturnType<typeof setTimeout> | null = null
  let controle: AbortController | null = null
  let pedido = 0

  function cancelar() {
    pedido++
    if (timer) clearTimeout(timer)
    timer = null
    controle?.abort()
    controle = null
  }

  async function executar(args: A, meu: number) {
    timer = null
    controle = new AbortController()
    carregando.value = true
    try {
      const resp = await consultar(args, controle.signal)
      if (meu !== pedido) return
      dados.value = resp
      erro.value = ''
      carregando.value = false
      opcoes.aoResultado?.(resp)
    } catch (e: any) {
      if (meu !== pedido || foiAbortado(e)) return
      if (repetir && e instanceof ApiError && e.status === 429 && e.codigo === 'muitas_requisicoes') {
        const espera = Math.min(60, Math.max(1, e.retryAfter ?? 2)) * 1000
        timer = setTimeout(() => executar(args, meu), espera)
        return
      }
      carregando.value = false
      erro.value = e?.message || 'erro na consulta'
      opcoes.aoErro?.(e)
    }
  }

  function pedir(args: A) {
    cancelar()
    const meu = pedido
    carregando.value = true
    timer = setTimeout(() => executar(args, meu), atraso)
  }

  function agora(args: A): Promise<void> {
    cancelar()
    return executar(args, pedido)
  }

  function limpar() {
    cancelar()
    dados.value = null
    erro.value = ''
    carregando.value = false
  }

  onScopeDispose(cancelar)

  return { dados, carregando, erro, pedir, agora, limpar }
}
