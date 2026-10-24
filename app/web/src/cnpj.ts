// Máscara e validação de CNPJ/CPF no cliente (a validação definitiva é do backend).

export function somenteDigitos(valor: string | null | undefined): string {
  return (valor ?? '').replace(/\D/g, '')
}

/** ehCNPJ informa se o documento tem 14 dígitos (CNPJ) — 11 dígitos é CPF. */
export function ehCNPJ(valor: string | null | undefined): boolean {
  return somenteDigitos(valor).length === 14
}

export function ehCPF(valor: string | null | undefined): boolean {
  return somenteDigitos(valor).length === 11
}

/** formatarCNPJ aplica a máscara 00.000.000/0000-00 progressivamente (serve para digitação). */
export function formatarCNPJ(valor: string | null | undefined): string {
  const d = somenteDigitos(valor).slice(0, 14)
  let out = d.slice(0, 2)
  if (d.length > 2) out += '.' + d.slice(2, 5)
  if (d.length > 5) out += '.' + d.slice(5, 8)
  if (d.length > 8) out += '/' + d.slice(8, 12)
  if (d.length > 12) out += '-' + d.slice(12, 14)
  return out
}

/** formatarCPF aplica a máscara 000.000.000-00 progressivamente. */
export function formatarCPF(valor: string | null | undefined): string {
  const d = somenteDigitos(valor).slice(0, 11)
  let out = d.slice(0, 3)
  if (d.length > 3) out += '.' + d.slice(3, 6)
  if (d.length > 6) out += '.' + d.slice(6, 9)
  if (d.length > 9) out += '-' + d.slice(9, 11)
  return out
}

/** formatarDocumento usa a máscara de CPF até 11 dígitos e a de CNPJ a partir do 12º. */
export function formatarDocumento(valor: string | null | undefined): string {
  const d = somenteDigitos(valor)
  return d.length <= 11 ? formatarCPF(d) : formatarCNPJ(d)
}

function digitoVerificador(base: string, pesos: number[]): number {
  const soma = pesos.reduce((acc, peso, i) => acc + Number(base[i]) * peso, 0)
  const resto = soma % 11
  return resto < 2 ? 0 : 11 - resto
}

/** cnpjValido confere tamanho, sequência repetida e os dois dígitos verificadores. */
export function cnpjValido(valor: string | null | undefined): boolean {
  const d = somenteDigitos(valor)
  if (d.length !== 14) return false
  if (/^(\d)\1{13}$/.test(d)) return false
  const dv1 = digitoVerificador(d.slice(0, 12), [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
  const dv2 = digitoVerificador(d.slice(0, 13), [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
  return d[12] === String(dv1) && d[13] === String(dv2)
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
