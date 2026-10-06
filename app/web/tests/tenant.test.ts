import { describe, expect, it } from 'vitest'
import { aplicarTema, apiBase, basePath, chaveLocal, tenant } from '../src/tenant'

describe('identidade do ambiente', () => {
  it('sem injeção usa o tenant de desenvolvimento na raiz', () => {
    expect(tenant.slug).toBe('dev')
    expect(basePath()).toBe('/')
    expect(apiBase()).toBe('/api/v1')
    expect(chaveLocal('token')).toBe('crmia:dev:token')
  })

  it('aplica a cor do cliente e deriva os tons', () => {
    aplicarTema('#ff0000')
    const root = document.documentElement.style
    expect(root.getPropertyValue('--ci-purple')).toBe('#ff0000')
    expect(root.getPropertyValue('--ci-purple-rgb')).toBe('255, 0, 0')
    expect(root.getPropertyValue('--ci-purple-dark')).toMatch(/^#[0-9a-f]{6}$/)
    expect(root.getPropertyValue('--ci-purple-tint')).toMatch(/^#[0-9a-f]{6}$/)
    // Cor inválida cai no padrão em vez de quebrar o tema.
    aplicarTema('azul')
    expect(root.getPropertyValue('--ci-purple')).toBe('#6d5df6')
  })
})
