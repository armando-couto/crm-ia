import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// A landing é estática e pré-renderizada no build. Em dev, /painel é
// repassado ao painel local para a seção de planos e o formulário de leads.
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5174,
    proxy: { '/painel': { target: process.env.PAINEL_ALVO || 'http://localhost:7894', changeOrigin: true } }
  },
  build: { outDir: 'dist', sourcemap: false },
  test: { environment: 'jsdom', globals: true }
} as any)
