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

/**
 * `/` is the public root, and #11's Meta app review fetches it signed out. It
 * sat as ticket #1's developer placeholder — "The Go server is serving this
 * single-page app from its own binary" — until this.
 */
test('the root serves a real landing page, signed out', () => {
  pb.authStore.clear()

  render(
    <MemoryRouter initialEntries={['/']}>
      <App />
    </MemoryRouter>,
  )

  expect(screen.getByRole('heading', { level: 1 })).toBeDefined()
  expect(screen.queryByText(/single-page app from its own binary/i)).toBeNull()

  // The two ways in, and the two pages the review also wants reachable.
  for (const [name, href] of [
    ['Sign In', '/login'],
    ['Create Account', '/register'],
    ['Política de privacidad', '/politica-de-privacidad'],
    ['Términos de servicio', '/terminos-de-servicio'],
  ]) {
    expect(screen.getByRole('link', { name }).getAttribute('href')).toBe(href)
  }
})
