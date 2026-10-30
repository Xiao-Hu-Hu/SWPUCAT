import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    port: 3000,
    host: '0.0.0.0',
    // Docker Desktop on Windows may not forward bind-mount file events to
    // chokidar reliably. The compose setup enables polling so edits cannot
    // leave Vite serving a stale in-memory module transform.
    watch: {
      usePolling: process.env.CHOKIDAR_USEPOLLING === 'true'
    },
    proxy: {
      '/api': {
        target: 'http://server:8080',
        changeOrigin: true
      }
    }
  }
})
