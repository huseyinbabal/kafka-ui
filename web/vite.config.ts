import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    proxy: {
      '/api': { target: process.env.KAFKAUI_API ?? 'http://127.0.0.1:18080', changeOrigin: false },
    },
  },
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 1500 },
})
