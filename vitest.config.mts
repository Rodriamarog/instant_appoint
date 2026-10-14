import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  // Same mapping as tsconfig's "@/*". Spelled out because tsconfig excludes the
  // test files (they're kept out of next build).
  resolve: { alias: { '@': fileURLToPath(new URL('.', import.meta.url)) } },
  test: {
    environment: 'node',
  },
})
