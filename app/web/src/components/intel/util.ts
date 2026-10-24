// Auxiliares compartilhados pelos componentes de inteligência de empresas.
import { formatMoney } from '../../format'
import type { PerfilCampos, Provedor } from '../../types-intel'

export const nomesProvedores: Record<string, string> = {
  receita: 'Receita Federal',
  cep: 'BrasilAPI CEP',
  bigdatacorp: 'BigDataCorp',
  serasa: 'Serasa Experian'
}

export function nomeProvedor(codigo: Provedor | string | undefined | null): string {
  if (!codigo) return '—'
  return nomesProvedores[codigo] ?? codigo
}

/** Rótulos dos conjuntos BigDataCorp (o catálogo da API pode sobrepor). */
export const nomesDatasets: Record<string, string> = {
  basic_data: 'Dados básicos',
  registration_data: 'Dados cadastrais',
  activity_indicators: 'Indicadores de atividade',
  relationships: 'Sócios e relacionamentos',
  phones_extended: 'Telefones',
  emails_extended: 'E-mails',
  addresses_extended: 'Endereços',
  domains_extended: 'Domínios e presença digital',
  processes: 'Processos judiciais',
  kyc: 'KYC e sanções',
  media_profile_and_exposure: 'Mídia e exposição'
}

export const datasetsSugeridos = ['basic_data', 'registration_data', 'activity_indicators', 'relationships']

export const rotulosCampos: Record<string, string> = {
  name: 'Nome',
  legal_name: 'Razão social',
  trade_name: 'Nome fantasia',
  cnae_code: 'CNAE',
  cnae_description: 'Descrição do CNAE',
  industry: 'Segmento',
  legal_nature: 'Natureza jurídica',
  founded_at: 'Abertura',
  capital: 'Capital social',
  company_size: 'Porte',
  registration_status: 'Situação cadastral',
  registration_status_at: 'Data da situação',
  simples: 'Simples Nacional',
  mei: 'MEI',
  street: 'Logradouro',
  street_number: 'Número',
  complement: 'Complemento',
  neighborhood: 'Bairro',
  zip: 'CEP',
  city: 'Cidade',
  state: 'UF',
  phone: 'Telefone',
  email: 'E-mail',
  website: 'Site',
  domain: 'Domínio',
  employees_range: 'Funcionários',
  revenue_range: 'Faturamento'
}

export function rotuloCampo(campo: string): string {
  return rotulosCampos[campo] ?? campo
}

/** centavos formata um valor em centavos como moeda. */
export function centavos(valor: number | null | undefined): string {
  return formatMoney((valor ?? 0) / 100)
}

/**
 * dataCurta formata datas sem deslocar o dia: "2015-03-10" vira 10/03/2015
 * (new Date de uma data pura cai no fuso UTC e voltaria um dia no Brasil).
 */
export function dataCurta(valor: string | null | undefined): string {
  if (!valor) return '—'
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(valor)
  if (m && valor.length === 10) return `${m[3]}/${m[2]}/${m[1]}`
  const d = new Date(valor)
  if (isNaN(d.getTime())) return valor
  return d.toLocaleDateString('pt-BR')
}

/** valorLegivel transforma um valor cru do diff em texto para a tabela. */
export function valorLegivel(campo: string, valor: unknown): string {
  if (valor === null || valor === undefined || valor === '') return ''
  if (typeof valor === 'boolean') return valor ? 'Sim' : 'Não'
  if (campo === 'capital' && typeof valor === 'number') return formatMoney(valor)
  if ((campo === 'founded_at' || campo === 'registration_status_at') && typeof valor === 'string') return dataCurta(valor)
  if (campo === 'zip' && typeof valor === 'string' && /^\d{8}$/.test(valor)) return `${valor.slice(0, 5)}-${valor.slice(5)}`
  if (Array.isArray(valor)) return valor.join(', ')
  if (typeof valor === 'object') return JSON.stringify(valor)
  return String(valor)
}

export function formatarCEP(zip: string | null | undefined): string {
  if (!zip) return ''
  const d = zip.replace(/\D/g, '')
  return d.length === 8 ? `${d.slice(0, 5)}-${d.slice(5)}` : zip
}

/** enderecoCompleto monta a linha "Rua X, 100 — sala 2 · Centro · Fortaleza/CE · 60000-000". */
export function enderecoCompleto(c: {
  street?: string | null
  street_number?: string | null
  complement?: string | null
  neighborhood?: string | null
  city?: string | null
  state?: string | null
  zip?: string | null
}): string {
  const partes: string[] = []
  const rua = [c.street, c.street_number].filter(Boolean).join(', ')
  if (rua) partes.push(c.complement ? `${rua} — ${c.complement}` : rua)
  if (c.neighborhood) partes.push(c.neighborhood)
  const cidade = c.city ? `${c.city}${c.state ? '/' + c.state : ''}` : c.state || ''
  if (cidade) partes.push(cidade)
  if (c.zip) partes.push(formatarCEP(c.zip))
  return partes.join(' · ')
}

/** Campos do formulário que o pré-preenchimento pela Receita pode tocar. */
export const camposPreenchiveis = [
  'name',
  'legal_name',
  'domain',
  'phone',
  'email',
  'city',
  'state',
  'industry',
  'street',
  'street_number',
  'complement',
  'neighborhood',
  'zip',
  'website'
] as const

function vazio(v: unknown): boolean {
  return v === null || v === undefined || (typeof v === 'string' && v.trim() === '')
}

/**
 * preencherVazios copia para o formulário só os campos ainda vazios, a partir
 * dos campos do perfil da Receita. Devolve a lista do que foi preenchido.
 */
export function preencherVazios(form: Record<string, any>, campos: PerfilCampos | undefined | null): string[] {
  if (!campos) return []
  const propostos: Record<string, unknown> = {
    name: campos.trade_name || campos.legal_name || campos.name,
    legal_name: campos.legal_name,
    domain: campos.domain,
    phone: campos.phone,
    email: campos.email,
    city: campos.city,
    state: campos.state,
    industry: campos.cnae_description || campos.industry,
    street: campos.street,
    street_number: campos.street_number,
    complement: campos.complement,
    neighborhood: campos.neighborhood,
    zip: campos.zip,
    website: campos.website
  }
  const preenchidos: string[] = []
  for (const campo of camposPreenchiveis) {
    const proposto = propostos[campo]
    if (vazio(proposto) || !vazio(form[campo])) continue
    form[campo] = proposto
    preenchidos.push(campo)
  }
  return preenchidos
}
