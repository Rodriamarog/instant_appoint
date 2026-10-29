import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, expect, test } from 'vitest'

import { App } from '@/App'
import { pb } from '@/lib/pocketbase'

/**
 * Meta's app review fetches the privacy policy and the terms of service
 * anonymously, and a URL that answers with the SPA's 404 page fails it. These
 * two paths were registered with Meta against the old frontend, so they have
 * to keep answering with the real page, signed out.
 */
afterEach(() => {
  pb.authStore.clear()
  cleanup()
})

test.each([
  ['/politica-de-privacidad', 'Privacy Policy'],
  ['/terminos-de-servicio', 'Terms of Service'],
])('%s renders without a session', (path, heading) => {
  pb.authStore.clear()

  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )

  expect(screen.getByRole('heading', { level: 1, name: heading })).toBeDefined()
  expect(screen.queryByText(/not found/i)).toBeNull()
})
