import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { call } from '@/lib/api'
import { pb } from '@/lib/pocketbase'

/**
 * Three call sites wrote this out by hand, and the third left out
 * `cache: 'no-store'`. Each detail below is one of the ones that drifted.
 */

function respondWith(body: unknown, init: ResponseInit = {}) {
  const fetchMock = vi.fn((_path: string, _init?: RequestInit) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        headers: { 'Content-Type': 'application/json' },
        ...init,
      }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)

  return fetchMock
}

beforeEach(() => {
  pb.authStore.save('the-session-token', {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('every call carries the session as a bearer token and refuses a cached answer', async () => {
  const fetchMock = respondWith({ ok: true })

  await call('/api/whatsapp/templates')

  const init = fetchMock.mock.calls[0][1] ?? {}
  expect((init.headers as Record<string, string>).Authorization).toBe(
    'Bearer the-session-token',
  )
  expect((init.headers as Record<string, string>)['Content-Type']).toBe(
    'application/json',
  )
  // A stale answer to "did this send?" is worse than a slow one.
  expect(init.cache).toBe('no-store')
})

test('a refusal becomes an Error carrying the server’s own sentence', async () => {
  respondWith(
    { message: 'Message failed to send because more than 24 hours have passed' },
    { status: 500 },
  )

  await expect(call('/api/chat/send', { method: 'POST' })).rejects.toThrow(
    'Message failed to send because more than 24 hours have passed',
  )
})

test('a refusal that explains nothing falls back to the caller’s sentence', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response('<html>502</html>', { status: 502 }))),
  )

  await expect(call('/api/chat/send', {}, 'The message could not be sent.')).rejects.toThrow(
    'The message could not be sent.',
  )
})

test('the parsed body comes back as it stands', async () => {
  respondWith({ templates: [{ name: 'appointment_reminder' }] })

  expect(await call<{ templates: { name: string }[] }>('/api/whatsapp/templates')).toEqual({
    templates: [{ name: 'appointment_reminder' }],
  })
})
