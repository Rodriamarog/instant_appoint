import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { Appointment } from '@/lib/appointments'
import { pb } from '@/lib/pocketbase'
import { RemindersPage } from '@/pages/reminders'

/**
 * The seam is the PocketBase SDK, stubbed per collection — the same one the
 * page's own modules sit on. Everything above it is the behaviour under test:
 * what the two columns show, whether the connection banner is telling the
 * truth, and that changing the lead time is written down and taken account of.
 */
const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
}

const NOW = new Date('2026-11-02T12:00:00.000Z')

const notFound = () =>
  Object.assign(new Error('The requested resource wasn’t found.'), { status: 404 })

let appointments: Appointment[] = []
let storedSettings: Record<string, unknown> | null = null
let connectedAccount: Record<string, unknown> | null = null

const createSettings = vi.fn()
const updateSettings = vi.fn()

function appointment(overrides: Partial<Appointment>): Appointment {
  return {
    id: 'evt1',
    user_id: signedInUser.id,
    client_name: 'Ana Ruiz',
    client_phone: '5215512345678',
    notes: 'See you tomorrow.',
    start_time: '2026-11-03T17:00:00.000Z',
    end_time: '2026-11-03T18:00:00.000Z',
    status: 'pending',
    language: 'es',
    ...overrides,
  }
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(NOW)

  appointments = []
  storedSettings = null
  connectedAccount = null
  createSettings.mockReset().mockImplementation((body) => {
    storedSettings = { id: 'set123', ...body }
    return Promise.resolve(storedSettings)
  })
  updateSettings.mockReset().mockImplementation((id, body) => {
    storedSettings = { ...storedSettings, id, ...body }
    return Promise.resolve(storedSettings)
  })

  vi.spyOn(pb, 'collection').mockImplementation((name: string) => {
    const stub = {
      appointments: {
        getFullList: () => Promise.resolve(appointments),
      },
      whatsapp_messages: {
        // Two reads now: the sent column is a page of history, and the
        // already-reminded set is asked for by appointment id.
        getList: () => Promise.resolve({ items: [] }),
        getFullList: () => Promise.resolve([]),
      },
      whatsapp_accounts: {
        getFullList: () => Promise.resolve(connectedAccount ? [connectedAccount] : []),
      },
      business_settings: {
        getFirstListItem: () =>
          storedSettings ? Promise.resolve(storedSettings) : Promise.reject(notFound()),
        create: createSettings,
        update: updateSettings,
      },
    }[name]

    return stub as unknown as ReturnType<typeof pb.collection>
  })

  // The settings dialog asks the server for the account's approved
  // templates. Stubbed empty so these tests stay about the page.
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ templates: [] }),
      } as Response),
    ),
  )

  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('a connected number means no "not connected" warning', async () => {
  // The page this replaces read a key the endpoint never returned, so this
  // banner was shown to everybody, permanently, connected or not.
  connectedAccount = { id: 'acc1', phone_number_id: '200000000000002' }

  render(<RemindersPage />)

  expect(await screen.findByText('Próximos')).toBeDefined()
  await waitFor(() =>
    expect(screen.queryByText(/WhatsApp no está conectado/)).toBeNull(),
  )
})

test('somebody with no connected number is told reminders will not send', async () => {
  render(<RemindersPage />)

  expect(await screen.findByText(/WhatsApp no está conectado/)).toBeDefined()
})

test('the sent column says so while nothing has been sent', async () => {
  render(<RemindersPage />)

  expect(
    await screen.findByText('Aún no se ha enviado ningún recordatorio'),
  ).toBeDefined()
})

test('an appointment with a phone number is listed as a reminder to come', async () => {
  appointments = [appointment({})]

  render(<RemindersPage />)

  expect(await screen.findByText('Ana Ruiz')).toBeDefined()
  expect(screen.getByText('+5215512345678')).toBeDefined()
})

test('nothing to remind about reads as such rather than as an empty box', async () => {
  render(<RemindersPage />)

  expect(await screen.findByText('No hay recordatorios próximos')).toBeDefined()
})

test('a new lead time is written down and changes what is due', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  // 29 hours out: with the default day's notice its reminder is still five
  // hours away, with two hours' notice it is a long way off.
  appointments = [appointment({ start_time: '2026-11-03T17:00:00.000Z' })]

  render(<RemindersPage />)

  expect(await screen.findByText('Programado')).toBeDefined()

  await user.click(screen.getByRole('button', { name: /ajustes de recordatorios/i }))
  await user.click(await screen.findByRole('button', { name: '2 horas' }))
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(createSettings).toHaveBeenCalledTimes(1))
  expect(createSettings.mock.calls[0][0]).toMatchObject({
    user_id: signedInUser.id,
    timing_minutes: 120,
  })

  // Reopening shows what was saved, not the default it started from.
  await user.click(screen.getByRole('button', { name: /ajustes de recordatorios/i }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: '2 horas' }).getAttribute('aria-pressed')).toBe(
      'true',
    ),
  )
})

test('reminders can be switched off and the choice is saved', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

  render(<RemindersPage />)

  await user.click(
    await screen.findByRole('button', { name: /ajustes de recordatorios/i }),
  )
  await user.click(
    await screen.findByRole('switch', { name: /apagar los recordatorios/i }),
  )
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(createSettings).toHaveBeenCalledTimes(1))
  expect(createSettings.mock.calls[0][0]).toMatchObject({ is_active: false })
})

test('a saved lead time is what the page comes back to', async () => {
  storedSettings = {
    id: 'set123',
    user_id: signedInUser.id,
    timing_minutes: 30,
    is_active: true,
    template_name: '',
    template_language: 'es',
    timezone: 'America/Tijuana',
  }
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

  render(<RemindersPage />)

  await user.click(
    await screen.findByRole('button', { name: /ajustes de recordatorios/i }),
  )

  await waitFor(() =>
    expect(screen.getByRole('button', { name: '30 min' }).getAttribute('aria-pressed')).toBe(
      'true',
    ),
  )
})

test('a reminder whose moment has passed is marked due', async () => {
  appointments = [appointment({ start_time: '2026-11-02T18:00:00.000Z' })]

  render(<RemindersPage />)

  expect(await screen.findByText('Por enviar')).toBeDefined()
})
