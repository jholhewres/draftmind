import { readFileSync } from 'node:fs'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { configDefaults } from 'vitest/config'

export default defineConfig({
  plugins: [
    react(),
    // The built app under the server's own Content-Security-Policy, for the
    // browser tests that check it (the dev server injects inline code).
    {
      name: 'server-csp',
      configurePreviewServer(server) {
        const csp = readFileSync(new URL('../api/internal/httpapi/csp.txt', import.meta.url), 'utf8').trim()
        server.middlewares.use((_req, res, next) => {
          res.setHeader('Content-Security-Policy', csp)
          next()
        })
      },
    },
  ],
  server: {
    proxy: { '/api': process.env.API_URL ?? 'http://127.0.0.1:8080' },
  },

  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    // Playwright owns e2e/.
    exclude: [...configDefaults.exclude, 'e2e/**'],
  },
})
