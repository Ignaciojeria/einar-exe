import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { tanstackRouter } from '@tanstack/router-plugin/vite'

// vite.config.ts
//
// Plugins:
//   - tanstackRouter: escanea src/routes/ y genera src/routeTree.gen.ts
//                     en watch mode. Esa generación es lo que da el
//                     type-safety end-to-end (params, search, links).
//                     IMPORTANTE: debe ir ANTES del plugin de React.
//   - react: JSX + Fast Refresh.
//
// build.outDir = ../internal/web/dist (el binario Go lo embebe via go:embed).
//
// server.proxy: dev con Vite (`npm run dev`) → API/auth/SPA-pages se
// reenvían a Caddy (puerto 8000), que reparte a app/casdoor.
export default defineConfig({
  plugins: [
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    react(),
  ],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api':    'http://localhost:8000',
      '/auth':   'http://localhost:8000',
      '/signup': 'http://localhost:8000',
      '/t':      'http://localhost:8000',
    },
  },
})
