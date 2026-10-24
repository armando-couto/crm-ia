import { describe, expect, it } from 'vitest'
import { cnpjValido, cpfValido, ehCNPJ, ehCPF, formatarCNPJ, formatarCPF, formatarDocumento, somenteDigitos } from '../src/cnpj'
import { preencherVazios, valorLegivel, dataCurta, enderecoCompleto } from '../src/components/intel/util'

describe('cnpj', () => {
  it('somenteDigitos remove máscara e espaços', () => {
    expect(somenteDigitos(' 11.222.333/0001-81 ')).toBe('11222333000181')
    expect(somenteDigitos(null)).toBe('')
  })

  it('formatarCNPJ aplica a máscara progressivamente', () => {
    expect(formatarCNPJ('11')).toBe('11')
    expect(formatarCNPJ('112223')).toBe('11.222.3')
    expect(formatarCNPJ('11222333000181')).toBe('11.222.333/0001-81')
    expect(formatarCNPJ('11222333000181999')).toBe('11.222.333/0001-81')
  })

  it('formatarCPF e formatarDocumento alternam pela quantidade de dígitos', () => {
    expect(formatarCPF('52998224725')).toBe('529.982.247-25')
    expect(formatarDocumento('52998224725')).toBe('529.982.247-25')
    expect(formatarDocumento('112223330001')).toBe('11.222.333/0001')
  })

  it('cnpjValido confere os dígitos verificadores', () => {
    expect(cnpjValido('11.222.333/0001-81')).toBe(true)
    expect(cnpjValido('00.000.000/0001-91')).toBe(true)
    expect(cnpjValido('11.222.333/0001-80')).toBe(false)
    expect(cnpjValido('11111111111111')).toBe(false)
    expect(cnpjValido('1122233300018')).toBe(false)
    expect(cnpjValido('')).toBe(false)
  })

  it('cpfValido confere os dígitos verificadores', () => {
    expect(cpfValido('529.982.247-25')).toBe(true)
    expect(cpfValido('529.982.247-24')).toBe(false)
    expect(cpfValido('00000000000')).toBe(false)
  })

  it('ehCNPJ e ehCPF distinguem pelo tamanho', () => {
    expect(ehCNPJ('11.222.333/0001-81')).toBe(true)
    expect(ehCNPJ('529.982.247-25')).toBe(false)
    expect(ehCPF('529.982.247-25')).toBe(true)
  })
})

describe('util de inteligência', () => {
  it('preencherVazios só toca campos vazios e mapeia fantasia → nome e CNAE → segmento', () => {
    const form: Record<string, any> = { name: '', legal_name: '', city: 'Sobral', industry: '', phone: '', zip: '' }
    const preenchidos = preencherVazios(form, {
      trade_name: 'ACME',
      legal_name: 'ACME VENDAS LTDA',
      city: 'Fortaleza',
      cnae_description: 'Desenvolvimento de programas',
      phone: '(85) 3333-4444',
      zip: '60000000'
    })
    expect(form.name).toBe('ACME')
    expect(form.legal_name).toBe('ACME VENDAS LTDA')
    expect(form.city).toBe('Sobral')
    expect(form.industry).toBe('Desenvolvimento de programas')
    expect(preenchidos).toEqual(['name', 'legal_name', 'phone', 'industry', 'zip'])
  })

  it('valorLegivel e dataCurta formatam sem deslocar o dia', () => {
    expect(dataCurta('2015-03-10')).toBe('10/03/2015')
    expect(valorLegivel('founded_at', '2015-03-10')).toBe('10/03/2015')
    expect(valorLegivel('simples', true)).toBe('Sim')
    expect(valorLegivel('zip', '60000000')).toBe('60000-000')
    expect(valorLegivel('phone', '')).toBe('')
  })

  it('enderecoCompleto monta a linha do endereço', () => {
    expect(enderecoCompleto({ street: 'Rua X', street_number: '100', neighborhood: 'Centro', city: 'Fortaleza', state: 'CE', zip: '60000000' })).toBe(
      'Rua X, 100 · Centro · Fortaleza/CE · 60000-000'
    )
    expect(enderecoCompleto({})).toBe('')
  })
})
