// Auxiliares das telas de leads e da fila de ligações.
import { api } from '../../api'
import { CONFIG_PROSPECCAO_PADRAO, type ConfigProspeccao, type Lead, type ResumoPlataforma } from '../../types-prospeccao'

/** UUID da chave de idempotência do resultado (`ResultadoRequest.chave`). */
export function novaChave(): string {
  const c = globalThis.crypto as Crypto | undefined
  if (c && typeof c.randomUUID === 'function') return c.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (ch) => {
    const r = Math.floor(Math.random() * 16)
    return (ch === 'x' ? r : (r & 0x3) | 0x8).toString(16)
  })
}

const dois = (n: number) => String(n).padStart(2, '0')

/** Data local no formato do `<input type="datetime-local">`: "2026-11-23T09:00". */
export function paraCampoDataHora(d: Date): string {
  return `${d.getFullYear()}-${dois(d.getMonth() + 1)}-${dois(d.getDate())}T${dois(d.getHours())}:${dois(d.getMinutes())}`
}

/** Valor do `datetime-local` (hora local) → RFC 3339 em UTC. Vazio ou inválido devolve ''. */
export function campoParaIso(valor: string | null | undefined): string {
  if (!valor) return ''
  const d = new Date(valor)
  return isNaN(d.getTime()) ? '' : d.toISOString()
}

/** RFC 3339 → valor do `datetime-local` na hora local. */
export function isoParaCampo(valor: string | null | undefined): string {
  if (!valor) return ''
  const d = new Date(valor)
  return isNaN(d.getTime()) ? '' : paraCampoDataHora(d)
}

export interface AtalhoRetorno {
  rotulo: string
  valor: string
}

/** Atalhos de data para "Retornar": poupam digitação no meio da ligação. */
export function atalhosRetorno(agora: Date = new Date(), inicioJanela = 9): AtalhoRetorno[] {
  const em = (horas: number) => new Date(agora.getTime() + horas * 3600000)
  const diaAs = (dias: number, hora: number) => {
    const d = new Date(agora.getFullYear(), agora.getMonth(), agora.getDate() + dias, hora, 0, 0)
    return d
  }
  return [
    { rotulo: 'Em 1 hora', valor: paraCampoDataHora(em(1)) },
    { rotulo: 'Em 3 horas', valor: paraCampoDataHora(em(3)) },
    { rotulo: `Amanhã, ${inicioJanela}h`, valor: paraCampoDataHora(diaAs(1, inicioJanela)) },
    { rotulo: 'Em 2 dias', valor: paraCampoDataHora(diaAs(2, inicioJanela)) },
    { rotulo: 'Em 1 semana', valor: paraCampoDataHora(diaAs(7, inicioJanela)) }
  ]
}

/** "segunda-feira, 23/11" a partir de "2026-11-23" (sem deslocar o fuso). */
export function rotuloDia(data: string): string {
  const [a, m, d] = data.split('-').map(Number)
  if (!a || !m || !d) return data
  const dia = new Date(a, m - 1, d)
  const semana = dia.toLocaleDateString('pt-BR', { weekday: 'short' }).replace('.', '')
  return `${semana}, ${dois(d)}/${dois(m)}`
}

/** "14:30" de um instante RFC 3339, na hora local. */
export function horaDe(valor: string | null | undefined): string {
  if (!valor) return ''
  const d = new Date(valor)
  return isNaN(d.getTime()) ? '' : `${dois(d.getHours())}:${dois(d.getMinutes())}`
}

/** Texto do estado "fora da janela": "reabre hoje às 9h" / "reabre segunda-feira às 9h". */
export function textoReabre(reabreEm: string | null | undefined, agora: Date = new Date()): string {
  if (!reabreEm) return ''
  const d = new Date(reabreEm)
  if (isNaN(d.getTime())) return ''
  const hora = d.getMinutes() ? `${d.getHours()}h${dois(d.getMinutes())}` : `${d.getHours()}h`
  const mesmoDia = d.toDateString() === agora.toDateString()
  const amanha = new Date(agora.getFullYear(), agora.getMonth(), agora.getDate() + 1).toDateString() === d.toDateString()
  if (mesmoDia) return `reabre às ${hora}`
  if (amanha) return `reabre amanhã às ${hora}`
  return `reabre ${d.toLocaleDateString('pt-BR', { weekday: 'long' })} às ${hora}`
}

/** Configurações da prospecção; em falha valem os padrões do contrato. */
export async function carregarConfig(): Promise<ConfigProspeccao> {
  try {
    const c = await api.get<Partial<ConfigProspeccao> | null>('/settings/prospeccao')
    return { ...CONFIG_PROSPECCAO_PADRAO, ...(c ?? {}), motivos_descarte: c?.motivos_descarte?.length ? c.motivos_descarte : CONFIG_PROSPECCAO_PADRAO.motivos_descarte }
  } catch {
    return { ...CONFIG_PROSPECCAO_PADRAO }
  }
}

/**
 * A base de empresas é de demonstração? Só quem tem `prospecting.view` lê
 * `GET /prospeccao`; para os demais vale o sinal do próprio lead (os e-mails
 * fictícios da amostra terminam em `@exemplo.invalid`).
 */
export async function baseDeDemonstracao(podeVerPlataforma: boolean): Promise<boolean> {
  if (!podeVerPlataforma) return false
  try {
    const r = await api.get<ResumoPlataforma | null>('/prospeccao')
    return r?.base?.amostra === true
  } catch {
    return false
  }
}

export function leadDeDemonstracao(lead: Pick<Lead, 'emails'> | null | undefined): boolean {
  return (lead?.emails ?? []).some((e) => e.toLowerCase().endsWith('@exemplo.invalid'))
}

/** Copia para a área de transferência, com alternativa para navegador sem a API. */
export async function copiarTexto(texto: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(texto)
      return true
    }
  } catch {
    /* cai na alternativa */
  }
  try {
    const campo = document.createElement('textarea')
    campo.value = texto
    campo.style.position = 'fixed'
    campo.style.opacity = '0'
    document.body.appendChild(campo)
    campo.select()
    const ok = document.execCommand('copy')
    campo.remove()
    return ok
  } catch {
    return false
  }
}
