import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'https://localhost:8443',
        changeOrigin: true,
        secure: false,
      },
      '/ws': {
        target: 'wss://localhost:8443',
        ws: true,
        changeOrigin: true,
        secure: false,
      },
      '/static': {
        target: 'https://localhost:8443',
        changeOrigin: true,
        secure: false,
      },
    },
  },
})
