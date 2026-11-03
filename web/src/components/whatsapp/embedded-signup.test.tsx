import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { WhatsAppEmbeddedSignup } from '@/components/whatsapp/embedded-signup'

// The SDK reports status 'unknown' for a popup the person closed and for one
// that collapsed alike, which is why nothing here reads it: what separates the
// two is the message Meta posts, not the login response.
type LoginResponse = { authResponse?: { code: string } | null }
type LoginCallback = (response: LoginResponse) => void

const FAILED =
  'La conexión no terminó, así que no se conectó ningún número. Inténtalo de nuevo.'

/**
 * Stands in for the Facebook SDK, handing the test the callback FB.login was
 * given so it can answer the way the real SDK would.
 */
function stubFacebookSDK() {
  const callbacks: LoginCallback[] = []

  window.FB = {
    init: () => {},
    login: (cb: LoginCallback) => {
      callbacks.push(cb)
    },
  } as unknown as typeof window.FB

  return callbacks
}

/** A message as Meta's popup posts it, from the origin the component trusts. */
function postFromMeta(event: string, data: Record<string, unknown> = {}) {
  act(() => {
    window.dispatchEvent(
      new MessageEvent('message', {
        origin: 'https://www.facebook.com',
        data: JSON.stringify({ type: 'WA_EMBEDDED_SIGNUP', event, data }),
      }),
    )
  })
}

function connect() {
  fireEvent.click(screen.getByRole('button'))
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  delete (window as { FB?: unknown }).FB
})

test('a signup that ends without a code says the attempt failed', () => {
  // What the SDK reports when it could not follow the popup to the end. The
  // person did nothing wrong and needs to know the attempt is dead, rather
  // than watch the button quietly return to its resting state.
  const logins = stubFacebookSDK()
  render(<WhatsAppEmbeddedSignup onConnected={() => {}} />)

  connect()
  act(() => logins[0]({ authResponse: null }))

  expect(screen.getByText(FAILED)).toBeDefined()
})

test('a signup the person cancelled is not reported as a failure', () => {
  // Meta posts CANCEL when the popup is dismissed on purpose. That is a
  // non-event: saying anything would be telling someone off for changing
  // their mind.
  const logins = stubFacebookSDK()
  render(<WhatsAppEmbeddedSignup onConnected={() => {}} />)

  connect()
  postFromMeta('CANCEL')
  act(() => logins[0]({ authResponse: null }))

  expect(screen.queryByText(FAILED)).toBeNull()
})

test('an error Meta explained keeps its own words', () => {
  // The generic message is the fallback for silence. When Meta said what
  // went wrong, overwriting it with the vaguer line would throw away the
  // only part the person can act on.
  const logins = stubFacebookSDK()
  render(<WhatsAppEmbeddedSignup onConnected={() => {}} />)

  connect()
  postFromMeta('ERROR', { error_message: 'This number is already registered.' })
  act(() => logins[0]({ authResponse: null }))

  expect(screen.getByText('This number is already registered.')).toBeDefined()
  expect(screen.queryByText(FAILED)).toBeNull()
})

test('every ending leaves a button the person can press again', () => {
  const logins = stubFacebookSDK()
  render(<WhatsAppEmbeddedSignup onConnected={() => {}} />)

  // A dead end the person cannot retry from strands them on the page with
  // no way forward but a reload. Pressing it again is what clears the last
  // ending, so the passes below run against one mounted component.
  for (const ending of ['CANCEL', 'ERROR'] as const) {
    connect()
    postFromMeta(ending, { error_message: 'Something Meta said.' })
    act(() => logins[logins.length - 1]({ authResponse: null }))

    expect(screen.getByRole('button').hasAttribute('disabled')).toBe(false)
  }

  // And the silent ending, which is the one this ticket is about.
  connect()
  act(() => logins[logins.length - 1]({ authResponse: null }))

  expect(screen.getByText(FAILED)).toBeDefined()
  expect(screen.getByRole('button').hasAttribute('disabled')).toBe(false)
})
