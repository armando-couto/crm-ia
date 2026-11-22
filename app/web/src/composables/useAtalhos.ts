import { onBeforeUnmount, onMounted } from 'vue'

export type MapaAtalhos = Record<string, (event: KeyboardEvent) => void>

export interface AtalhosOptions {
  /** Devolve `false` para suspender os atalhos (tela carregando, salvando). */
  ativo?: () => boolean
  /** Teclas que valem mesmo com modal aberto ou foco em campo (padrão: nenhuma). */
  sempre?: string[]
}

/** O foco está num campo de digitação: tecla é texto, não atalho. */
export function focoEmCampo(alvo: EventTarget | null = document.activeElement): boolean {
  const el = alvo as HTMLElement | null
  if (!el || typeof el.tagName !== 'string') return false
  const tag = el.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || el.isContentEditable === true
}

/** Há um ModalDialog aberto (a gaveta SideDrawer não conta: é `aria-modal="false"`). */
export function modalAberto(): boolean {
  return !!document.querySelector('[role="dialog"][aria-modal="true"]')
}

/**
 * Atalhos de teclado de uma tela. As chaves do mapa são a tecla em minúsculas
 * (`'1'`, `'j'`, `'s'`, `'escape'`). São ignorados com o foco em `input`,
 * `textarea` ou `select`, com modal aberto e com Ctrl, Alt ou Cmd pressionados.
 *
 *   useAtalhos({ j: proximo, k: anterior, escape: fechar })
 */
export function useAtalhos(mapa: MapaAtalhos, opcoes: AtalhosOptions = {}): void {
  const sempre = new Set((opcoes.sempre ?? []).map((t) => t.toLowerCase()))

  function onTecla(event: KeyboardEvent) {
    if (event.ctrlKey || event.metaKey || event.altKey || event.repeat) return
    const tecla = (event.key || '').toLowerCase()
    const acao = mapa[tecla]
    if (!acao) return
    if (opcoes.ativo && !opcoes.ativo()) return
    if (!sempre.has(tecla) && (focoEmCampo(event.target) || modalAberto())) return
    event.preventDefault()
    acao(event)
  }

  onMounted(() => window.addEventListener('keydown', onTecla))
  onBeforeUnmount(() => window.removeEventListener('keydown', onTecla))
}
