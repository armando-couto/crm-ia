import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'

// Em `npm run dev` a API não serve o index.html, então os placeholders do
// tenant ficariam crus. O plugin troca por um ambiente de desenvolvimento na raiz.
const tenantDeDev = (): Plugin => ({
  name: 'tenant-de-dev',
  apply: 'serve',
  transformIndexHtml: (html) =>
    html
      .replace(/__TENANT_BASE__/g, '/')
      .replace(/__TENANT_NOME__/g, 'CRM IA (dev)')
      .replace(/__TEMA_COR_PRIMARIA__/g, '#6d5df6')
      .replace(
        /__TENANT_JSON__/g,
        JSON.stringify({ slug: 'dev', name: 'CRM IA (dev)', color: '#6d5df6', logo_url: '', version: 'dev', base_path: '', app_url: 'http://localhost:5173', plan_enabled: false, setup_done: true })
      )
})

// O proxy /api aponta para o backend Go local durante o desenvolvimento.
export default defineConfig({
  plugins: [vue(), tenantDeDev()],
  // base relativa: o mesmo build é servido em /<slug>/ de cada cliente; o
  // <base href> injetado pela API resolve as rotas profundas.
  base: './',
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 700
  },
  test: {
    environment: 'jsdom',
    globals: true
  }
} as any)
