import { writeFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'

import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import type { Plugin } from 'vite'
import { defineConfig } from 'vitest/config'

/** Where `go run ./cmd/server serve` listens by default. */
const goServer = 'http://127.0.0.1:8090'

/**
 * web/dist is gitignored apart from a committed .gitkeep, which is what keeps
 * the directory present so `//go:embed all:dist` compiles in a fresh clone.
 * Vite's emptyOutDir wipes it on every build, so put it straight back.
 */
function keepDistTracked(): Plugin {
  return {
    name: 'instant-appoint:keep-dist-tracked',
    apply: 'build',
    closeBundle() {
      writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '')
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepDistTracked()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    // The guard and the auth pages are DOM behaviour, so the suite needs a
    // document and a localStorage for the PocketBase auth store.
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
  },
  server: {
    port: 5173,
    // The SPA talks to PocketBase same-origin in production, so the dev server
    // has to forward both the API and the admin UI to the Go binary.
    proxy: {
      '/api': { target: goServer, changeOrigin: true },
      '/_/': { target: goServer, changeOrigin: true },
    },
  },
})
