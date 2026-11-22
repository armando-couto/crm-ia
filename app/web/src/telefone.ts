// Telefones da prospecção: formatação e links de discagem e de WhatsApp.
//
// Contrato (docs/PROSPECCAO.md + adendo): `numero` vem só com dígitos e com
// DDD. Celular = 11 dígitos; fixo e indefinido = 10 dígitos (os 8 da Receita,
// sem o 9). O DDI 55 é sempre montado aqui, no cliente.
import type { Telefone, TipoTelefone } from './types-prospeccao'

type Entrada = string | null | undefined

/**
 * normalizarTelefone deixa só os dígitos com DDD: tira máscara, o DDI 55 e o
 * zero de operadora ("0xx85..."). Não inventa o nono dígito.
 */
export function normalizarTelefone(valor: Entrada): string {
  let d = (valor ?? '').replace(/\D/g, '')
  d = d.replace(/^0+/, '')
  if ((d.length === 12 || d.length === 13) && d.startsWith('55')) d = d.slice(2)
  return d
}

/** telefoneValido aceita 10 (fixo ou indefinido) ou 11 dígitos (celular) com DDD plausível. */
export function telefoneValido(valor: Entrada): boolean {
  const d = normalizarTelefone(valor)
  if (d.length !== 10 && d.length !== 11) return false
  if (d[0] === '0' || d[1] === '0') return false
  if (d.length === 11 && d[2] !== '9') return false
  return !/^(\d)\1+$/.test(d.slice(2))
}

/** tipoPeloNumero deduz o tipo pelo tamanho quando o backend não informou. */
export function tipoPeloNumero(valor: Entrada): TipoTelefone {
  const d = normalizarTelefone(valor)
  if (d.length === 11) return 'celular'
  if (d.length === 10) return 'fixo'
  return 'indefinido'
}

/** formatarTelefone: "85999990001" → "(85) 99999-0001"; "8530000001" → "(85) 3000-0001". */
export function formatarTelefone(valor: Entrada): string {
  const d = normalizarTelefone(valor)
  if (d.length === 11) return `(${d.slice(0, 2)}) ${d.slice(2, 7)}-${d.slice(7)}`
  if (d.length === 10) return `(${d.slice(0, 2)}) ${d.slice(2, 6)}-${d.slice(6)}`
  if (d.length === 9) return `${d.slice(0, 5)}-${d.slice(5)}`
  if (d.length === 8) return `${d.slice(0, 4)}-${d.slice(4)}`
  return (valor ?? '').trim()
}

/** comNonoDigito põe o 9 depois do DDD de um número de 10 dígitos (telefone "indefinido"). */
export function comNonoDigito(valor: Entrada): string {
  const d = normalizarTelefone(valor)
  return d.length === 10 ? `${d.slice(0, 2)}9${d.slice(2)}` : d
}

/** linkTel monta o `tel:` com DDI: "tel:+5585999990001". Vazio se não há número. */
export function linkTel(valor: Entrada): string {
  const d = normalizarTelefone(valor)
  return d ? `tel:+55${d}` : ''
}

/**
 * linkWhatsApp monta "https://wa.me/55<numero>" (com `?text=` opcional). Só
 * celular tem WhatsApp: fora de 11 dígitos devolve vazio.
 */
export function linkWhatsApp(valor: Entrada, texto?: string): string {
  const d = normalizarTelefone(valor)
  if (d.length !== 11) return ''
  const url = `https://wa.me/55${d}`
  return texto ? `${url}?text=${encodeURIComponent(texto)}` : url
}

export interface OpcaoDiscagem {
  /** Dígitos com DDD a discar (em `ResultadoRequest.numero` vai sempre o `numero` original do telefone). */
  numero: string
  rotulo: string
  href: string
}

/**
 * opcoesDiscagem devolve os modos de discar um telefone. Para o tipo
 * `indefinido` (DDD 11 começando por 4 ou 5) são dois: sem e com o 9.
 */
export function opcoesDiscagem(tel: Pick<Telefone, 'numero' | 'tipo'>): OpcaoDiscagem[] {
  const d = normalizarTelefone(tel.numero)
  if (!d) return []
  const base: OpcaoDiscagem = { numero: d, rotulo: formatarTelefone(d), href: linkTel(d) }
  if (tel.tipo !== 'indefinido' || d.length !== 10) return [base]
  const com9 = comNonoDigito(d)
  return [base, { numero: com9, rotulo: formatarTelefone(com9), href: linkTel(com9) }]
}

export interface RegrasWhatsApp {
  /** `ConfigProspeccao.exigir_optin_whatsapp`. */
  exigirOptin?: boolean
  /** `Lead.whatsapp_optin`. */
  optin?: boolean
  /** `ResumoPlataforma.base.amostra`: dados de demonstração não ligam nem abrem WhatsApp. */
  amostra?: boolean
}

/** podeLigar: telefone marcado como inválido ou base de demonstração não disca. */
export function podeLigar(tel: Pick<Telefone, 'numero' | 'invalido'>, regras: Pick<RegrasWhatsApp, 'amostra'> = {}): boolean {
  return !tel.invalido && !regras.amostra && !!normalizarTelefone(tel.numero)
}

/**
 * podeWhatsApp aplica as regras do contrato: só celular válido, nunca para
 * `indefinido`, nem com `whatsapp: false` informado pelo enriquecimento, nem
 * em base de demonstração; e exige o opt-in quando a configuração pede.
 */
export function podeWhatsApp(tel: Pick<Telefone, 'numero' | 'tipo' | 'invalido' | 'whatsapp'>, regras: RegrasWhatsApp = {}): boolean {
  if (tel.invalido || regras.amostra) return false
  if (tel.tipo !== 'celular' || tel.whatsapp === false) return false
  if (!linkWhatsApp(tel.numero)) return false
  return !regras.exigirOptin || regras.optin === true
}

export interface GrupoTelefones {
  /** Nome da pessoa; vazio vira "Empresa". */
  pessoa: string
  /** Cargo vindo de `resumo.decisores` pelo nome (vazio se não achou). */
  cargo: string
  telefones: Telefone[]
}

const semAcento = (s: string) =>
  s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .trim()
    .toLowerCase()

/**
 * agruparTelefones monta a coluna "Ligar": só `lead.telefones`, na ordem
 * recebida, agrupados por `pessoa` (vazio = "Empresa"), com o cargo buscado
 * em `resumo.decisores` pelo nome. A ordem dos grupos é a da primeira aparição.
 */
export function agruparTelefones(
  telefones: Telefone[] | null | undefined,
  decisores: { nome: string; cargo: string }[] | null | undefined = []
): GrupoTelefones[] {
  const cargos = new Map<string, string>()
  for (const d of decisores ?? []) cargos.set(semAcento(d.nome), d.cargo)
  const grupos: GrupoTelefones[] = []
  const indice = new Map<string, GrupoTelefones>()
  for (const tel of telefones ?? []) {
    const chave = semAcento(tel.pessoa ?? '')
    let grupo = indice.get(chave)
    if (!grupo) {
      grupo = { pessoa: tel.pessoa?.trim() || 'Empresa', cargo: cargos.get(chave) ?? '', telefones: [] }
      indice.set(chave, grupo)
      grupos.push(grupo)
    }
    grupo.telefones.push(tel)
  }
  return grupos
}

/** telefonePrincipal: o marcado `principal`, senão o primeiro que não é inválido. */
export function telefonePrincipal(telefones: Telefone[] | null | undefined): Telefone | null {
  const lista = (telefones ?? []).filter((t) => !t.invalido)
  return lista.find((t) => t.principal) ?? lista[0] ?? null
}
