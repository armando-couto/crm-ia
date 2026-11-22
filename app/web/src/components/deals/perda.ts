// Apoio do diálogo de motivo de perda (docs/PROSPECCAO.md, seção 9).
import { ApiError } from '../../api'

/** Padrão do contrato, usado enquanto GET /settings/motivos-perda não responde. */
export const MOTIVOS_PERDA_PADRAO = ['Preço', 'Sem orçamento', 'Escolheu concorrente', 'Sem interesse', 'Sem resposta', 'Momento errado', 'Outro']

/** O servidor recusou a perda por falta de motivo (422 `motivo_obrigatorio`). */
export function faltouMotivo(e: unknown): boolean {
  return e instanceof ApiError && (e.codigo === 'motivo_obrigatorio' || (e.status === 422 && /motivo/i.test(e.message)))
}
