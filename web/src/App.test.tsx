import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

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
  vi.restoreAllMocks()
  cleanup()
})

test.each([
  ['/politica-de-privacidad', 'Política de privacidad'],
  ['/terminos-de-servicio', 'Términos de servicio'],
])('%s renders without a session', (path, heading) => {
  pb.authStore.clear()

  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )

  expect(screen.getByRole('heading', { level: 1, name: heading })).toBeDefined()
  expect(screen.queryByText(/no encontrada/i)).toBeNull()
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
    ['Iniciar sesión', '/login'],
    ['Crear cuenta', '/register'],
    ['Política de privacidad', '/politica-de-privacidad'],
    ['Términos de servicio', '/terminos-de-servicio'],
  ]) {
    expect(screen.getByRole('link', { name }).getAttribute('href')).toBe(href)
  }
})

/**
 * Everything below is the signed-in route table: which URLs exist now that
 * five tabs are two, and what the ones that stopped existing do.
 *
 * The seam is the PocketBase SDK and `fetch`, the two edges every page under
 * the shell loads through. Stubbed empty, so each page renders its own
 * nothing-here state and a test can tell them apart by what they say. It
 * misses what the pages do with real data — their own test files cover that.
 */
const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
  verified: true,
}

/**
 * The auth store only reads the `exp` claim, so a syntactically valid
 * unexpired token stands in for a real login — the same stand-in
 * require-auth.test.tsx uses.
 */
function freshToken() {
  const encode = (value: object) =>
    btoa(JSON.stringify(value)).replace(/=+$/, '')

  return [
    encode({ alg: 'HS256', typ: 'JWT' }),
    encode({
      id: signedInUser.id,
      collectionId: signedInUser.collectionId,
      exp: Math.floor(Date.now() / 1000) + 3600,
    }),
    'not-a-real-signature',
  ].join('.')
}

function openSignedIn(path: string) {
  pb.authStore.save(freshToken(), signedInUser as never)

  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  // react-big-calendar's drag-select asks the document what is under the
  // pointer; jsdom has no layout and no such method.
  document.elementFromPoint = () => null

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList: () => Promise.resolve([]),
    getFirstListItem: () =>
      Promise.reject(Object.assign(new Error('not found'), { status: 404 })),
    authWithPassword: () => {
      pb.authStore.save(freshToken(), signedInUser as never)

      return Promise.resolve({})
    },
  } as unknown as ReturnType<typeof pb.collection>)

  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response('{"templates":[]}', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
})

/** Signing in lands here, so this is the one path that has to be right. */
test('Inicio is the appointments page', async () => {
  openSignedIn('/inicio')

  expect(await screen.findByRole('button', { name: 'Hoy' })).toBeDefined()
})

/**
 * Nothing the five tabs reached may be lost. Each of these is a page that
 * used to have a tab of its own and now lives under Configuración.
 */
test.each([
  ['/configuracion/asistente', /^Asistente de IA/],
  ['/configuracion/whatsapp', /^Número de WhatsApp/],
  ['/configuracion/recordatorios', /^Recordatorios/],
  ['/configuracion/plantillas', /^Plantillas/],
])('%s is still reachable, opened', async (path, name) => {
  openSignedIn(path)

  const row = await screen.findByRole('button', { name })
  await waitFor(() => expect(row.getAttribute('aria-expanded')).toBe('true'))
  expect(screen.queryByText(/no encontrada/i)).toBeNull()
})

test('/configuracion lists every setting, none of them opened', async () => {
  openSignedIn('/configuracion')

  for (const name of [/^Asistente de IA/, /^Número de WhatsApp/, /^Recordatorios/, /^Plantillas/]) {
    const row = await screen.findByRole('button', { name })
    expect(row.getAttribute('aria-expanded')).toBe('false')
  }
})

/**
 * The five old paths are in people's history and in their bookmarks. Four of
 * them still name a page, so they take you to it rather than to a 404.
 */
test.each([
  ['/calendar', 'Inicio'],
  ['/whatsapp', 'Configuración'],
  ['/reminders', 'Configuración'],
  ['/templates', 'Configuración'],
])('the old %s link lands under %s', async (path, tab) => {
  openSignedIn(path)

  const bar = await screen.findByRole('navigation', { name: 'Principal' })
  expect(
    within(bar).getByRole('link', { name: tab }).getAttribute('aria-current'),
  ).toBe('page')
})

/**
 * The fifth names a page this ticket deleted. A deep link to it has to say
 * so and offer a way on, which is the client-side 404 — the Go server answers
 * every extensionless path with the shell, so nothing upstream ever sees it.
 */
test('a deep link to the deleted chat page says there is nothing there', async () => {
  openSignedIn('/chat')

  expect(
    await screen.findByRole('heading', { name: /no encontrada/i }),
  ).toBeDefined()

  // And the way out goes where a signed-in person belongs, not to the
  // signed-out landing page.
  await waitFor(() =>
    expect(screen.getByRole('link').getAttribute('href')).toBe('/inicio'),
  )
})

/**
 * The last of the five tabs to go was the one sign-in used to land on. Where
 * a person arrives is the app's answer to "what is this for", so it is
 * asserted through the route table rather than against the navigate call.
 */
test('signing in lands on Inicio', async () => {
  pb.authStore.clear()

  render(
    <MemoryRouter initialEntries={['/login']}>
      <App />
    </MemoryRouter>,
  )

  await userEvent.type(screen.getByLabelText('Correo electrónico'), 'dev@example.test')
  await userEvent.type(screen.getByLabelText('Contraseña'), 'devpassword123')
  await userEvent.click(screen.getByRole('button', { name: 'Iniciar sesión' }))

  expect(await screen.findByRole('button', { name: 'Hoy' })).toBeDefined()
})
