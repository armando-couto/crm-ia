// Máscara e validação de CNPJ/CPF no cliente (a validação definitiva é do backend).
// O CNPJ é texto de 14 caracteres, [0-9A-Z]{12}[0-9]{2}: use normalizarCNPJ, nunca somenteDigitos.

export function somenteDigitos(valor: string | null | undefined): string {
  return (valor ?? '').replace(/\D/g, '')
}

/**
 * normalizarCNPJ devolve o CNPJ em maiúsculas e só com [0-9A-Z] (no máximo 14
 * caracteres). O CNPJ alfanumérico vale desde julho de 2026: as 12 primeiras
 * posições aceitam letras e os 2 dígitos verificadores continuam numéricos.
 */
export function normalizarCNPJ(valor: string | null | undefined): string {
  return (valor ?? '').toUpperCase().replace(/[^0-9A-Z]/g, '').slice(0, 14)
}

const FORMATO_CNPJ = /^[0-9A-Z]{12}[0-9]{2}$/

function temLetra(valor: string | null | undefined): boolean {
  return /[A-Za-z]/.test(valor ?? '')
}

/** ehCNPJ informa se o documento tem o formato de CNPJ (14 caracteres) — 11 dígitos é CPF. */
export function ehCNPJ(valor: string | null | undefined): boolean {
  const bruto = (valor ?? '').toUpperCase().replace(/[^0-9A-Z]/g, '')
  return FORMATO_CNPJ.test(bruto)
}

export function ehCPF(valor: string | null | undefined): boolean {
  return !temLetra(valor) && somenteDigitos(valor).length === 11
}

/**
 * formatarCNPJ aplica a máscara 00.000.000/0000-00 progressivamente (serve
 * para digitação), aceitando letras nas 12 primeiras posições.
 */
export function formatarCNPJ(valor: string | null | undefined): string {
  const d = normalizarCNPJ(valor)
  let out = d.slice(0, 2)
  if (d.length > 2) out += '.' + d.slice(2, 5)
  if (d.length > 5) out += '.' + d.slice(5, 8)
  if (d.length > 8) out += '/' + d.slice(8, 12)
  if (d.length > 12) out += '-' + d.slice(12, 14)
  return out
}

/** mascaraCNPJ é o nome usado pelas telas de prospecção para formatarCNPJ. */
export const mascaraCNPJ = formatarCNPJ

/** formatarCPF aplica a máscara 000.000.000-00 progressivamente. */
export function formatarCPF(valor: string | null | undefined): string {
  const d = somenteDigitos(valor).slice(0, 11)
  let out = d.slice(0, 3)
  if (d.length > 3) out += '.' + d.slice(3, 6)
  if (d.length > 6) out += '.' + d.slice(6, 9)
  if (d.length > 9) out += '-' + d.slice(9, 11)
  return out
}

/**
 * formatarDocumento usa a máscara de CPF até 11 dígitos e a de CNPJ a partir
 * do 12º — ou desde o começo, se houver letra (só CNPJ é alfanumérico).
 */
export function formatarDocumento(valor: string | null | undefined): string {
  if (temLetra(valor)) return formatarCNPJ(valor)
  const d = somenteDigitos(valor)
  return d.length <= 11 ? formatarCPF(d) : formatarCNPJ(d)
}

function digitoVerificador(base: string, pesos: number[]): number {
  // Regra alfanumérica: o valor do caractere é o código ASCII menos 48
  // ("0" = 0 ... "9" = 9, "A" = 17 ... "Z" = 42); para dígitos dá o mesmo de antes.
  const soma = pesos.reduce((acc, peso, i) => acc + (base.charCodeAt(i) - 48) * peso, 0)
  const resto = soma % 11
  return resto < 2 ? 0 : 11 - resto
}

/**
 * cnpjValido confere formato ([0-9A-Z]{12}[0-9]{2}), sequência repetida e os
 * dois dígitos verificadores, pela regra alfanumérica.
 */
export function cnpjValido(valor: string | null | undefined): boolean {
  const d = (valor ?? '').toUpperCase().replace(/[^0-9A-Z]/g, '')
  if (!FORMATO_CNPJ.test(d)) return false
  if (/^(.)\1{13}$/.test(d)) return false
  const dv1 = digitoVerificador(d.slice(0, 12), [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
  const dv2 = digitoVerificador(d.slice(0, 13), [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
  return d[12] === String(dv1) && d[13] === String(dv2)
}

export interface CnpjsColados {
  /** Normalizados, válidos e sem repetição, na ordem em que apareceram. */
  validos: string[]
  /** Trechos que não são CNPJ válido, como foram colados. */
  invalidos: string[]
  /** Quantas vezes um CNPJ válido apareceu de novo. */
  repetidos: number
}

/**
 * separarCNPJs lê um texto colado (um CNPJ por linha, ou separados por
 * vírgula, ponto e vírgula, tabulação ou espaço) e devolve o resumo
 * "N válidos, M inválidos, K repetidos" de "Colar CNPJs".
 */
export function separarCNPJs(texto: string | null | undefined): CnpjsColados {
  const validos: string[] = []
  const invalidos: string[] = []
  const vistos = new Set<string>()
  let repetidos = 0
  for (const trecho of (texto ?? '').split(/[\s,;|]+/)) {
    if (!trecho) continue
    const d = trecho.toUpperCase().replace(/[^0-9A-Z]/g, '')
    if (!d) continue
    if (!cnpjValido(d)) {
      invalidos.push(trecho)
    } else if (vistos.has(d)) {
      repetidos++
    } else {
      vistos.add(d)
      validos.push(d)
    }
  }
  return { validos, invalidos, repetidos }
}

/** cpfValido confere tamanho, sequência repetida e os dígitos verificadores. */
export function cpfValido(valor: string | null | undefined): boolean {
  const d = somenteDigitos(valor)
  if (d.length !== 11) return false
  if (/^(\d)\1{10}$/.test(d)) return false
  const dv1 = digitoVerificador(d.slice(0, 9), [10, 9, 8, 7, 6, 5, 4, 3, 2])
  const dv2 = digitoVerificador(d.slice(0, 10), [11, 10, 9, 8, 7, 6, 5, 4, 3, 2])
  return d[9] === String(dv1) && d[10] === String(dv2)
}
