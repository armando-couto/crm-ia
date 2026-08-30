import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// O proxy /api aponta para o backend Go local durante o desenvolvimento.
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:9000',
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
