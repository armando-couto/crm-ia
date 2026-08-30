import { describe, expect, it } from 'vitest'
import { formatDate, formatMoney, initials, lifecycleLabels, relativeDate } from '../src/format'

describe('formatMoney', () => {
  it('formata valores em BRL', () => {
    // Intl usa espaço não separável entre R$ e o número.
    expect(formatMoney(1234.5).replace(/ /g, ' ')).toBe('R$ 1.234,50')
  })

  it('trata nulos como zero', () => {
    expect(formatMoney(null).replace(/ /g, ' ')).toBe('R$ 0,00')
    expect(formatMoney(undefined).replace(/ /g, ' ')).toBe('R$ 0,00')
  })
})

describe('formatDate', () => {
  it('formata datas ISO', () => {
    expect(formatDate('2026-03-15T12:00:00Z')).toMatch(/15\/03\/2026/)
  })

  it('retorna travessão para vazio ou inválido', () => {
    expect(formatDate(null)).toBe('—')
    expect(formatDate('abc')).toBe('—')
  })
})

describe('relativeDate', () => {
  it('retorna "hoje" para agora', () => {
    expect(relativeDate(new Date().toISOString())).toBe('hoje')
  })

  it('retorna "ontem" para 1 dia atrás', () => {
    const yesterday = new Date(Date.now() - 25 * 3600000).toISOString()
    expect(relativeDate(yesterday)).toBe('ontem')
  })
})

describe('initials', () => {
  it('extrai as iniciais do nome', () => {
    expect(initials('Armando Couto')).toBe('AC')
    expect(initials('Ana Beatriz da Silva')).toBe('AS')
    expect(initials('Ana')).toBe('A')
  })

  it('retorna ? para vazio', () => {
    expect(initials('')).toBe('?')
    expect(initials(null)).toBe('?')
  })
})

describe('lifecycleLabels', () => {
  it('cobre todos os estágios do backend', () => {
    for (const key of ['lead', 'mql', 'sql', 'oportunidade', 'cliente', 'perdido']) {
      expect(lifecycleLabels[key]).toBeTruthy()
    }
  })
})
