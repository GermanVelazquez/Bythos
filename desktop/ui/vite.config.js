import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// vite.config.js — El PORTERO entre Vite y Go.
// Solo existe para desarrollo (pnpm dev).
// En producción (pnpm build) este archivo no hace nada: el dist/ lo sirve Go.
//
// ¿Qué problema resuelve? Sin esto, tu React en :5173 pediría
// http://localhost:8080/api/... y el navegador lo bloquearía por CORS
// o tendrías que hardcodear URLs. Con el proxy, React pide /api/...
// y Vite lo reenvía a Go en secreto. El navegador cree que es mismo origen.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173, // ¿Por qué 5173? Es el default de Vite y NO choca con :8080 de Go
    proxy: {
      // Todo lo que empiece con /api va al cerebro Go
      '/api': 'http://localhost:8080',
    },
  },
})
