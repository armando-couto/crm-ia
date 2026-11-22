/**
 * Estado do formulário do gerador de listas e a conversão de e para `Filtros`
 * (contrato da prospecção). O formulário guarda os itens de catálogo inteiros
 * (para mostrar os chips com nome); a API só recebe códigos.
 */
import { api } from '../../api'
import type {
  Catalogo,
  CatalogoCnae,
  CatalogoMunicipio,
  Contagem,
  Cota,
  Filtros,
  Ordem,
  Porte
} from '../../types-prospeccao'

export type ModoAbertura = '' | 'dias' | 'intervalo'

export interface FormFiltros {
  cnaes: CatalogoCnae[]
  cnae_secundario: boolean
  ufs: string[]
  municipios: CatalogoMunicipio[]
  bairros: string[]
  ceps: string[]
  portes: Porte[]
  matriz: 'todas' | 'matriz' | 'filial'
  naturezas: number[]
  abertura_modo: ModoAbertura
  abertura_ultimos_dias: number | null
  abertura_de: string
  abertura_ate: string
  capital_min: number | null
  capital_max: number | null
  simples: 'todos' | 'sim' | 'nao'
  mei: 'todos' | 'sim' | 'nao'
  incluir_pf: boolean
  divida: 'todos' | 'com' | 'sem'
  divida_min: number | null
  divida_max: number | null
  nome: string
  com_celular: boolean
  com_telefone: boolean
  com_email: boolean
  ocultar_contador: boolean
  ordem: Ordem
}

/** Teto de empresas por lista (contrato 6.3). */
export const MAX_QUANTIDADE = 20000
/** Teto de CNPJs colados (contrato 2). */
export const MAX_CNPJS = 5000

/** Atalhos de "abertas nos últimos N dias". */
export const ATALHOS_ABERTURA: { dias: number; rotulo: string }[] = [
  { dias: 30, rotulo: '30 dias' },
  { dias: 90, rotulo: '90 dias' },
  { dias: 180, rotulo: '180 dias' },
  { dias: 365, rotulo: '1 ano' }
]

export function formVazio(): FormFiltros {
  return {
    cnaes: [],
    cnae_secundario: false,
    ufs: [],
    municipios: [],
    bairros: [],
    ceps: [],
    portes: [],
    matriz: 'todas',
    naturezas: [],
    abertura_modo: '',
    abertura_ultimos_dias: null,
    abertura_de: '',
    abertura_ate: '',
    capital_min: null,
    capital_max: null,
    simples: 'todos',
    mei: 'todos',
    incluir_pf: false,
    divida: 'todos',
    divida_min: null,
    divida_max: null,
    nome: '',
    com_celular: false,
    com_telefone: false,
    com_email: false,
    // Ligada por padrão: telefone de contador não serve para prospectar.
    ocultar_contador: true,
    ordem: 'relevancia'
  }
}

function positivo(n: number | null | undefined): n is number {
  return typeof n === 'number' && Number.isFinite(n) && n > 0
}

/** Bairro e CEP só valem com cidade ou CNAE no mesmo filtro. */
export function localFinoPermitido(form: FormFiltros): boolean {
  return form.municipios.length > 0 || form.cnaes.length > 0
}

/**
 * UFs marcadas que ficam de fora da busca porque há cidades escolhidas em
 * outros estados (com cidades, a busca vale só para as cidades).
 */
export function ufsSemCidade(form: FormFiltros): string[] {
  if (!form.municipios.length) return []
  const comCidade = new Set(form.municipios.map((m) => m.uf))
  return form.ufs.filter((uf) => !comCidade.has(uf))
}

/**
 * paraFiltros monta o `Filtros` da API só com o que realmente filtra: campo
 * vazio, `todos`/`todas` e chave desligada ficam de fora.
 */
export function paraFiltros(form: FormFiltros): Filtros {
  const f: Filtros = {}

  const subclasses = form.cnaes.filter((c) => c.nivel === 'subclasse').map((c) => Number(c.codigo))
  const prefixos = form.cnaes.filter((c) => c.nivel !== 'subclasse').map((c) => c.codigo)
  if (subclasses.length) f.cnaes = subclasses
  if (prefixos.length) f.cnae_prefixos = prefixos
  if (form.cnae_secundario && form.cnaes.length) f.cnae_secundario = true

  // Com cidades escolhidas a busca vale só para elas; a UF serve de recorte do seletor.
  if (form.municipios.length) f.municipios = form.municipios.map((m) => m.codigo)
  else if (form.ufs.length) f.ufs = [...form.ufs]

  if (localFinoPermitido(form)) {
    if (form.bairros.length) f.bairros = [...form.bairros]
    if (form.ceps.length) f.ceps = [...form.ceps]
  }

  if (form.portes.length) f.portes = [...form.portes]
  if (form.matriz !== 'todas') f.matriz = form.matriz
  if (form.naturezas.length) f.naturezas = [...form.naturezas]

  if (form.abertura_modo === 'dias' && positivo(form.abertura_ultimos_dias)) {
    f.abertura_ultimos_dias = Math.round(form.abertura_ultimos_dias)
  } else if (form.abertura_modo === 'intervalo') {
    if (form.abertura_de) f.abertura_de = form.abertura_de
    if (form.abertura_ate) f.abertura_ate = form.abertura_ate
  }

  if (positivo(form.capital_min)) f.capital_min = form.capital_min
  if (positivo(form.capital_max)) f.capital_max = form.capital_max
  if (form.simples !== 'todos') f.simples = form.simples

  if (form.incluir_pf) f.incluir_pf = true
  // `mei: 'sim'` só vale com `incluir_pf` (422 sem isso).
  if (form.mei === 'nao' || (form.mei === 'sim' && form.incluir_pf)) f.mei = form.mei

  if (form.divida !== 'todos') f.divida = form.divida
  if (form.divida !== 'sem') {
    if (positivo(form.divida_min)) f.divida_min = form.divida_min
    if (positivo(form.divida_max)) f.divida_max = form.divida_max
  }

  const nome = form.nome.trim()
  if (nome.length >= 3) f.nome = nome

  if (form.com_celular) f.com_celular = true
  if (form.com_telefone) f.com_telefone = true
  if (form.com_email) f.com_email = true
  if (form.ocultar_contador) f.ocultar_contador = true
  return f
}

/** Rótulos dos chips: os filtros guardados só têm códigos. */
export interface RotulosCatalogo {
  cnaes?: CatalogoCnae[]
  municipios?: CatalogoMunicipio[]
}

function nivelPeloCodigo(codigo: string): CatalogoCnae['nivel'] {
  if (codigo.length <= 2) return 'divisao'
  if (codigo.length === 3) return 'grupo'
  if (codigo.length <= 5) return 'classe'
  return 'subclasse'
}

/**
 * deFiltros reconstrói o formulário a partir de um `Filtros` guardado
 * (segmento salvo ou lista). Código sem rótulo no catálogo vira chip com o
 * próprio código, para nunca sumir da tela.
 */
export function deFiltros(f: Filtros | null | undefined, ordem: Ordem = 'relevancia', rotulos: RotulosCatalogo = {}): FormFiltros {
  const form = formVazio()
  const filtros = f ?? {}
  form.ordem = ordem

  const porCodigoCnae = new Map((rotulos.cnaes ?? []).map((c) => [String(c.codigo), c]))
  const cnae = (codigo: string, subclasse: boolean): CatalogoCnae => {
    const achado = porCodigoCnae.get(codigo)
    if (achado && (achado.nivel === 'subclasse') === subclasse) return achado
    return { codigo, descricao: achado?.descricao ?? '', nivel: subclasse ? 'subclasse' : nivelPeloCodigo(codigo) }
  }
  form.cnaes = [
    ...(filtros.cnae_prefixos ?? []).map((p) => cnae(String(p), false)),
    ...(filtros.cnaes ?? []).map((c) => cnae(String(c).padStart(7, '0'), true))
  ]
  form.cnae_secundario = !!filtros.cnae_secundario

  const porCodigoMun = new Map((rotulos.municipios ?? []).map((m) => [m.codigo, m]))
  form.municipios = (filtros.municipios ?? []).map(
    (codigo) => porCodigoMun.get(codigo) ?? { codigo, nome: `Cidade ${codigo}`, uf: '', empresas: 0 }
  )
  const ufs = new Set<string>(filtros.ufs ?? [])
  for (const m of form.municipios) if (m.uf) ufs.add(m.uf)
  form.ufs = [...ufs]

  form.bairros = [...(filtros.bairros ?? [])]
  form.ceps = [...(filtros.ceps ?? [])]
  form.portes = [...(filtros.portes ?? [])]
  form.matriz = filtros.matriz ?? 'todas'
  form.naturezas = [...(filtros.naturezas ?? [])]

  if (positivo(filtros.abertura_ultimos_dias)) {
    form.abertura_modo = 'dias'
    form.abertura_ultimos_dias = filtros.abertura_ultimos_dias
  } else if (filtros.abertura_de || filtros.abertura_ate) {
    form.abertura_modo = 'intervalo'
    form.abertura_de = filtros.abertura_de ?? ''
    form.abertura_ate = filtros.abertura_ate ?? ''
  }

  form.capital_min = filtros.capital_min ?? null
  form.capital_max = filtros.capital_max ?? null
  form.simples = filtros.simples ?? 'todos'
  form.mei = filtros.mei ?? 'todos'
  form.incluir_pf = !!filtros.incluir_pf
  form.divida = filtros.divida ?? 'todos'
  form.divida_min = filtros.divida_min ?? null
  form.divida_max = filtros.divida_max ?? null
  form.nome = filtros.nome ?? ''
  form.com_celular = !!filtros.com_celular
  form.com_telefone = !!filtros.com_telefone
  form.com_email = !!filtros.com_email
  form.ocultar_contador = !!filtros.ocultar_contador
  return form
}

/**
 * Busca no catálogo os nomes dos CNAEs e das cidades de um `Filtros` guardado
 * (`catalogos/cnaes?codigos=` e `catalogos/municipios?codigos=`). Falha de
 * catálogo não impede o uso do segmento: os chips saem só com o código.
 */
export async function carregarRotulos(f: Filtros | null | undefined): Promise<RotulosCatalogo> {
  const codigosCnae = [...(f?.cnae_prefixos ?? []).map(String), ...(f?.cnaes ?? []).map((c) => String(c).padStart(7, '0'))]
  const codigosMun = f?.municipios ?? []
  const rotulos: RotulosCatalogo = {}
  const lotes = <T>(lista: T[]): T[][] => {
    const saida: T[][] = []
    for (let i = 0; i < lista.length; i += 200) saida.push(lista.slice(i, i + 200))
    return saida
  }
  await Promise.all([
    (async () => {
      if (!codigosCnae.length) return
      try {
        const partes = await Promise.all(
          lotes(codigosCnae).map((l) => api.get<Catalogo<CatalogoCnae>>(`/prospeccao/catalogos/cnaes?codigos=${l.join(',')}`))
        )
        rotulos.cnaes = partes.flatMap((p) => p?.itens ?? []).map((c) => ({ ...c, codigo: String(c.codigo) }))
      } catch {
        /* chips só com o código */
      }
    })(),
    (async () => {
      if (!codigosMun.length) return
      try {
        const partes = await Promise.all(
          lotes(codigosMun).map((l) => api.get<Catalogo<CatalogoMunicipio>>(`/prospeccao/catalogos/municipios?codigos=${l.join(',')}`))
        )
        rotulos.municipios = partes.flatMap((p) => p?.itens ?? [])
      } catch {
        /* chips só com o código */
      }
    })()
  ])
  return rotulos
}

/** Código de CNAE como o vendedor conhece: 5611-2/01, 56.11-2, 56.1, 56. */
export function formatarCnae(codigo: string | number): string {
  const c = String(codigo)
  if (c.length === 7) return `${c.slice(0, 4)}-${c[4]}/${c.slice(5)}`
  if (c.length === 5) return `${c.slice(0, 2)}.${c.slice(2, 4)}-${c[4]}`
  if (c.length === 4) return `${c.slice(0, 2)}.${c.slice(2)}`
  if (c.length === 3) return `${c.slice(0, 2)}.${c[2]}`
  return c
}

export interface TetoQuantidade {
  /** Maior quantidade que dá para pedir agora (0 = não dá para gerar). */
  maximo: number
  /** O total real é desconhecido: "a lista pode sair menor que o pedido". */
  podeSairMenor: boolean
}

/**
 * Teto da quantidade da nova lista: `min(20.000, cota.restante, total)` com
 * contagem exata; sem total exato, `min(20.000, cota.restante)` com aviso.
 */
export function tetoQuantidade(contagem: Contagem | null | undefined, cota: Cota | null | undefined): TetoQuantidade {
  const restante = Math.max(0, cota?.restante ?? 0)
  if (contagem && contagem.total_modo === 'exato') {
    return { maximo: Math.max(0, Math.min(MAX_QUANTIDADE, restante, contagem.total)), podeSairMenor: false }
  }
  return { maximo: Math.min(MAX_QUANTIDADE, restante), podeSairMenor: true }
}

/** Sugestão de nome para a lista: "Restaurantes · Fortaleza/CE". */
export function sugerirNome(form: FormFiltros): string {
  const partes: string[] = []
  const cnae = form.cnaes[0]
  if (cnae) {
    const descricao = cnae.descricao || `CNAE ${formatarCnae(cnae.codigo)}`
    partes.push(form.cnaes.length > 1 ? `${descricao} +${form.cnaes.length - 1}` : descricao)
  }
  if (form.municipios.length) {
    const m = form.municipios[0]
    const nome = m.uf ? `${m.nome}/${m.uf}` : m.nome
    partes.push(form.municipios.length > 1 ? `${nome} +${form.municipios.length - 1}` : nome)
  } else if (form.ufs.length) {
    partes.push(form.ufs.slice(0, 4).join(', ') + (form.ufs.length > 4 ? ` +${form.ufs.length - 4}` : ''))
  }
  const nome = partes.join(' · ')
  return nome.length > 90 ? `${nome.slice(0, 89)}…` : nome
}

/** Quantos filtros opcionais do bloco estão em uso (aparece no bloco recolhido). */
export function usadosNoPerfil(form: FormFiltros): number {
  let n = 0
  if (form.portes.length) n++
  if (form.matriz !== 'todas') n++
  if (form.naturezas.length) n++
  if (form.abertura_modo === 'dias' ? positivo(form.abertura_ultimos_dias) : form.abertura_modo === 'intervalo' && (form.abertura_de || form.abertura_ate)) n++
  if (positivo(form.capital_min) || positivo(form.capital_max)) n++
  if (form.simples !== 'todos') n++
  if (form.mei !== 'todos') n++
  if (form.incluir_pf) n++
  if (form.divida !== 'todos' || positivo(form.divida_min) || positivo(form.divida_max)) n++
  if (form.nome.trim().length >= 3) n++
  return n
}

export function usadosNaQualidade(form: FormFiltros): number {
  let n = 0
  if (form.com_celular) n++
  if (form.com_telefone) n++
  if (form.com_email) n++
  if (form.ocultar_contador) n++
  if (form.ordem !== 'relevancia') n++
  return n
}

/**
 * Resumo legível de um `Filtros` guardado (aba de segmentos), sem consultar
 * catálogo: "2 CNAEs · CE · 3 cidades · com celular".
 */
export function resumoFiltros(f: Filtros | null | undefined): string {
  if (!f) return ''
  const partes: string[] = []
  const nCnae = (f.cnaes?.length ?? 0) + (f.cnae_prefixos?.length ?? 0)
  if (f.cnpjs?.length) partes.push(`${f.cnpjs.length} CNPJs colados`)
  if (nCnae) partes.push(nCnae === 1 ? `CNAE ${formatarCnae(String(f.cnae_prefixos?.[0] ?? f.cnaes?.[0] ?? ''))}` : `${nCnae} CNAEs`)
  if (f.municipios?.length) partes.push(f.municipios.length === 1 ? '1 cidade' : `${f.municipios.length} cidades`)
  else if (f.ufs?.length) partes.push(f.ufs.length > 5 ? `${f.ufs.length} estados` : f.ufs.join(', '))
  if (f.portes?.length) partes.push(`${f.portes.length} porte${f.portes.length > 1 ? 's' : ''}`)
  if (f.abertura_ultimos_dias) partes.push(`abertas há até ${f.abertura_ultimos_dias} dias`)
  else if (f.abertura_de || f.abertura_ate) partes.push('abertura em intervalo')
  if (f.divida === 'com') partes.push('com dívida ativa')
  if (f.divida === 'sem') partes.push('sem dívida ativa')
  if (f.com_celular) partes.push('com celular')
  if (f.com_email) partes.push('com e-mail')
  if (f.incluir_pf) partes.push('inclui pessoa física')
  return partes.join(' · ')
}
