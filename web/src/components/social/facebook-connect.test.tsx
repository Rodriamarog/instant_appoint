import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

const config = vi.hoisted(() => ({ id: 'CONFIG-MESSAGING' }))

vi.mock('@/lib/embedded-signup', () => ({
  FB_APP_ID: 'APP',
  get MESSAGING_CONFIG_ID() {
    return config.id
  },
}))

vi.mock('@/lib/social-accounts', () => ({
  connectWithCode: vi.fn(),
  choosePage: vi.fn(),
}))

import { FacebookConnect } from '@/components/social/facebook-connect'
import { choosePage, connectWithCode } from '@/lib/social-accounts'

type LoginCallback = (response: { authResponse?: { code: string } }) => void

let logins: { cb: LoginCallback; opts: Record<string, unknown> }[]

beforeEach(() => {
  config.id = 'CONFIG-MESSAGING'
  logins = []
  window.FB = {
    init: () => {},
    login: (cb: LoginCallback, opts: Record<string, unknown>) => {
      logins.push({ cb, opts })
    },
  } as unknown as typeof window.FB
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  delete (window as { FB?: unknown }).FB
})

test('logs in with the messaging configuration and asks for a code', () => {
  render(<FacebookConnect onConnected={() => {}} />)

  fireEvent.click(screen.getByRole('button', { name: /Conectar con Facebook/ }))

  expect(logins).toHaveLength(1)
  expect(logins[0].opts).toMatchObject({
    config_id: 'CONFIG-MESSAGING',
    response_type: 'code',
    override_default_response_type: true,
  })
})

test('without a configuration it asks for the messaging permissions by name', () => {
  config.id = ''
  render(<FacebookConnect onConnected={() => {}} />)

  fireEvent.click(screen.getByRole('button', { name: /Conectar con Facebook/ }))

  expect(logins[0].opts).not.toHaveProperty('config_id')
  expect(logins[0].opts).toMatchObject({ response_type: 'code' })
  expect(String(logins[0].opts.scope).split(',')).toEqual(
    expect.arrayContaining(['pages_messaging', 'instagram_manage_messages', 'pages_manage_metadata']),
  )
})

test('a single Page connects straight away', async () => {
  const connected = { page_name: 'Clínica Sol', instagram_username: 'clinicasol', has_instagram: true }
  vi.mocked(connectWithCode).mockResolvedValue({ connected })
  const onConnected = vi.fn()
  render(<FacebookConnect onConnected={onConnected} />)

  fireEvent.click(screen.getByRole('button', { name: /Conectar con Facebook/ }))
  await act(async () => logins[0].cb({ authResponse: { code: 'CODE' } }))

  expect(connectWithCode).toHaveBeenCalledWith('CODE')
  expect(onConnected).toHaveBeenCalledWith(connected)
})

test('several Pages are offered, and the chosen one is connected', async () => {
  vi.mocked(connectWithCode).mockResolvedValue({
    pages: [
      { id: 'P1', name: 'Clínica Sol', instagram_username: 'clinicasol' },
      { id: 'P2', name: 'Clínica Luna' },
    ],
  })
  vi.mocked(choosePage).mockResolvedValue({
    connected: { page_name: 'Clínica Luna', has_instagram: false },
  })
  const onConnected = vi.fn()
  render(<FacebookConnect onConnected={onConnected} />)

  fireEvent.click(screen.getByRole('button', { name: /Conectar con Facebook/ }))
  await act(async () => logins[0].cb({ authResponse: { code: 'CODE' } }))

  expect(screen.getByText('@clinicasol')).toBeTruthy()
  await act(async () => fireEvent.click(screen.getByRole('button', { name: /Clínica Luna/ })))

  expect(choosePage).toHaveBeenCalledWith('P2')
  expect(onConnected).toHaveBeenCalledWith({ page_name: 'Clínica Luna', has_instagram: false })
})

test('a closed popup goes back to the button without an error', async () => {
  render(<FacebookConnect onConnected={() => {}} />)

  fireEvent.click(screen.getByRole('button', { name: /Conectar con Facebook/ }))
  await act(async () => logins[0].cb({}))

  expect(connectWithCode).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: /Conectar con Facebook/ })).toBeTruthy()
})

test('the server’s refusal is shown', async () => {
  vi.mocked(connectWithCode).mockRejectedValue(
    new Error('Esa página ya está conectada a otra cuenta de InstantAppoint.'),
  )
  render(<FacebookConnect onConnected={() => {}} />)

  fireEvent.click(screen.getByRole('button', { name: /Conectar con Facebook/ }))
  await act(async () => logins[0].cb({ authResponse: { code: 'CODE' } }))

  expect(screen.getByText(/ya está conectada a otra cuenta/)).toBeTruthy()
})
