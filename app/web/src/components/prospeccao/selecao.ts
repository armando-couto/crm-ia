/**
 * Seleção de itens da lista de prospecção além da página: marcar linhas
 * soltas (`ids`, até 5.000) ou "selecionar todos os N" que casam com o
 * filtro (`todos: true`), podendo desmarcar linhas depois (`exceto`).
 */
import type { FiltroItens, Selecao } from '../../types-prospeccao'

/** Teto de `ids` e de `exceto` (contrato 6.3). */
export const MAX_SELECAO_IDS = 5000

export interface EstadoSelecao {
  todos: boolean
  /** Sem `todos`: as linhas marcadas. Com `todos`: as desmarcadas. */
  ids: Set<number>
}

export function selecaoVazia(): EstadoSelecao {
  return { todos: false, ids: new Set() }
}

export function estaMarcado(s: EstadoSelecao, id: number): boolean {
  return s.todos ? !s.ids.has(id) : s.ids.has(id)
}

/** Marca ou desmarca uma linha; devolve `false` quando bate no teto. */
export function alternarItem(s: EstadoSelecao, id: number, marcar = !estaMarcado(s, id)): boolean {
  const incluirNoConjunto = s.todos ? !marcar : marcar
  if (incluirNoConjunto) {
    if (s.ids.size >= MAX_SELECAO_IDS && !s.ids.has(id)) return false
    s.ids.add(id)
  } else {
    s.ids.delete(id)
  }
  return true
}

/** Marca ou desmarca as linhas da página; devolve `false` quando bate no teto. */
export function alternarPagina(s: EstadoSelecao, ids: number[], marcar: boolean): boolean {
  let ok = true
  for (const id of ids) ok = alternarItem(s, id, marcar) && ok
  return ok
}

export function selecionarTodos(s: EstadoSelecao): void {
  s.todos = true
  s.ids.clear()
}

export function limparSelecao(s: EstadoSelecao): void {
  s.todos = false
  s.ids.clear()
}

/** Quantos itens estão selecionados, dado o total que casa com o filtro. */
export function quantidadeSelecionada(s: EstadoSelecao, totalFiltrado: number): number {
  return s.todos ? Math.max(0, totalFiltrado - s.ids.size) : s.ids.size
}

/** Todas as linhas da página estão marcadas (e há linhas). */
export function paginaInteiraMarcada(s: EstadoSelecao, ids: number[]): boolean {
  return ids.length > 0 && ids.every((id) => estaMarcado(s, id))
}

/** Algumas, mas não todas, as linhas da página estão marcadas. */
export function paginaParcial(s: EstadoSelecao, ids: number[]): boolean {
  const marcadas = ids.filter((id) => estaMarcado(s, id)).length
  return marcadas > 0 && marcadas < ids.length
}

/** Tira do filtro o que não filtra, para não mandar `q: ''` nem `false`. */
export function filtroLimpo(f: FiltroItens): FiltroItens | undefined {
  const saida: FiltroItens = {}
  if (f.estado) saida.estado = f.estado
  if (f.q && f.q.trim()) saida.q = f.q.trim()
  if (f.com_celular) saida.com_celular = true
  if (f.com_decisor) saida.com_decisor = true
  if (f.nota_min && f.nota_min > 0) saida.nota_min = f.nota_min
  return Object.keys(saida).length ? saida : undefined
}

/** Monta o `Selecao` do contrato a partir do estado da tela. */
export function paraSelecao(s: EstadoSelecao, filtro: FiltroItens): Selecao {
  if (s.todos) {
    const sel: Selecao = { todos: true }
    const f = filtroLimpo(filtro)
    if (f) sel.filtro = f
    if (s.ids.size) sel.exceto = [...s.ids]
    return sel
  }
  return { ids: [...s.ids] }
}
