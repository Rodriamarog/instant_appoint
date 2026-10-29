import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  // Same mapping as tsconfig's "@/*". Spelled out because tsconfig excludes the
  // test files (they're kept out of next build).
  resolve: { alias: { '@': fileURLToPath(new URL('.', import.meta.url)) } },
  test: {
    environment: 'node',
    // The Vite SPA under web/ has its own vitest config and its own
    // node_modules. Without this, the root runner picks up web/src tests and
    // fails on packages only web/ installs. Goes away with this file in #10.
    exclude: ['**/node_modules/**', '**/dist/**', 'web/**'],
  },
})
