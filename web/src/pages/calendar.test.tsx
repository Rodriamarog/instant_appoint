import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { isoMillis } from '@/lib/iso-time'
import { pb } from '@/lib/pocketbase'
import { CalendarPage, VIEW_STORAGE_KEY } from '@/pages/calendar'

/**
 * The seam is the PocketBase SDK. Above it: which appointments the page asks
 * for as you navigate, what it renders, and what clicking does.
 *
 * Three zones are in play below and all three are different on purpose.
 * The process runs in Madrid, so an hour the browser would have printed is
 * seven out. The clinic is in Mexico City, which is deliberately *not*
 * DEFAULT_SETTINGS.timezone — a page that never read the settings row at
 * all would otherwise pass every expectation here by coincidence.
 */
process.env.TZ = 'Europe/Madrid'

const CLINIC_TZ = 'America/Mexico_City'

const getFullList = vi.fn()
const getFirstListItem = vi.fn()

/** Mid-November 2026, so the month window has a whole month either side. */
const today = new Date('2026-11-15T20:00:00.000Z')

function appointment(overrides: Record<string, unknown> = {}) {
  return {
    id: 'evt123',
    user_id: 'user123',
    client_name: 'Ana Ruiz',
    client_phone: '5215512345678',
    notes: 'See you at the back entrance, Ana.',
    // 12:00 on 15 November in Mexico City.
    start_time: '2026-11-15T18:00:00.000Z',
    end_time: '2026-11-15T19:00:00.000Z',
    location: 'Suite 2',
    status: 'pending',
    ...overrides,
  }
}

beforeEach(() => {
  // react-big-calendar's drag-select asks the document what is under the
  // pointer on every mousedown; jsdom has no layout and no such method.
  document.elementFromPoint = () => null

  localStorage.removeItem(VIEW_STORAGE_KEY)

  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(today)

  getFullList.mockReset().mockResolvedValue([appointment()])
  getFirstListItem.mockReset().mockResolvedValue({
    id: 'settings1',
    timing_minutes: 1440,
    is_active: true,
    template_name: '',
    template_language: 'es',
    timezone: CLINIC_TZ,
    ai_mode: 'off',
    patient_outreach: 'on',
  })
  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
    getFirstListItem,
  } as unknown as ReturnType<typeof pb.collection>)

  pb.authStore.save('not-a-real-token', {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
    business_name: 'Dev Clinic',
  })
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

/** What the calendar says it is showing. */
function toolbarLabel(): string {
  return document.querySelector('.rbc-toolbar-label')?.textContent ?? ''
}

/** The filter string of the nth query the page ran. */
function filterOfCall(index: number): string {
  return String(getFullList.mock.calls[index][0].filter)
}

/** Renders the page already on one of the two grid views. */
function renderOnGrid(view: 'week' | 'month' = 'month') {
  localStorage.setItem(VIEW_STORAGE_KEY, view)
  return render(<CalendarPage />)
}

test('Inicio opens on the day’s list', async () => {
  // The morning screen. Somebody arriving at it has not asked for a month.
  render(<CalendarPage />)

  expect(await screen.findByRole('listitem')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Hoy' }).getAttribute('aria-pressed')).toBe(
    'true',
  )
})

test('the day the list shows is the clinic’s, not the browser’s', async () => {
  // 23:00 on the 15th in Mexico City, already 06:00 on the 16th in Madrid.
  // The clinic has not gone to bed yet, so it must still be looking at the
  // 15th.
  vi.setSystemTime(new Date('2026-11-16T05:00:00.000Z'))

  render(<CalendarPage />)

  expect(await screen.findByText('domingo 15 de noviembre de 2026')).toBeDefined()
  expect(await screen.findByText('Ana Ruiz')).toBeDefined()
})

test('the clock on a row is the one the clinic configured', async () => {
  // 18:00 UTC is 12:00 in Mexico City, 19:00 in Madrid, and 10:00 in the
  // default zone. Only the first of the three comes from the settings row,
  // which is the wiring this pins.
  render(<CalendarPage />)

  expect(await screen.findByText('12:00')).toBeDefined()
})

test('a day with no appointments says so', async () => {
  getFullList.mockResolvedValue([])

  render(<CalendarPage />)

  expect(await screen.findByText(/no hay citas/i)).toBeDefined()
})

test('the chosen view survives a refresh', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  render(<CalendarPage />)

  await user.click(screen.getByRole('button', { name: 'Semana' }))
  await waitFor(() => expect(toolbarLabel()).not.toBe(''))

  // A refresh is a fresh mount against the same storage.
  cleanup()
  render(<CalendarPage />)

  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Semana' }).getAttribute('aria-pressed'),
    ).toBe('true'),
  )
  expect(toolbarLabel()).not.toBe('')
})

test('Semana and Mes each show the grid', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  render(<CalendarPage />)

  await user.click(screen.getByRole('button', { name: 'Mes' }))
  expect(screen.getByText('noviembre 2026')).toBeDefined()

  await user.click(screen.getByRole('button', { name: 'Semana' }))
  // The week view's label names days, not the bare month.
  await waitFor(() => expect(toolbarLabel()).not.toBe('noviembre 2026'))
  expect(document.querySelector('.rbc-time-view')).not.toBeNull()
})

test('the appointments in view are rendered on the grid', async () => {
  renderOnGrid()

  expect(await screen.findByText('Ana Ruiz')).toBeDefined()
})

test('an appointment the assistant only proposed is marked as such on the grid', async () => {
  // A proposal is not a commitment, and a month view is read at a glance.
  // One that looked like every other slot is a clinic holding an
  // appointment nobody ever agreed to.
  getFullList.mockResolvedValue([appointment({ status: 'proposed' })])

  renderOnGrid()

  expect(await screen.findByText('Ana Ruiz')).toBeDefined()
  expect(screen.getByText('Propuesta')).toBeDefined()
})

test('a cancelled appointment is marked as such on the grid', async () => {
  // Cancelling leaves the booking in place — that is the point of it — so
  // the slot stays on the calendar and has to say that nobody is coming.
  getFullList.mockResolvedValue([appointment({ status: 'cancelled' })])

  renderOnGrid()

  expect(await screen.findByText('Cancelada')).toBeDefined()
})

test('an ordinary booking carries no label at all on the grid', async () => {
  // Pending is what nearly every appointment is. Labelling it would make
  // the two labels that matter invisible among the noise.
  renderOnGrid()

  await screen.findByText('Ana Ruiz')
  expect(screen.queryByText('Pendiente')).toBeNull()
})

test('the query covers the displayed month with a week of slack either side', async () => {
  render(<CalendarPage />)

  await waitFor(() => expect(getFullList).toHaveBeenCalled())

  // A month view shows days from the neighbouring months in its first and
  // last rows, so the window has to reach past the month itself.
  const filter = filterOfCall(0)
  expect(filter).toContain(isoMillis(new Date(2026, 9, 25, 0, 0, 0, 0)))
  expect(filter).toContain(isoMillis(new Date(2026, 11, 7, 23, 59, 59, 999)))
})

test('both window bounds are written in the millisecond-precision UTC format', async () => {
  // The columns are text and the comparison is byte by byte, so a bound in
  // any other shape silently returns the wrong appointments.
  render(<CalendarPage />)

  await waitFor(() => expect(getFullList).toHaveBeenCalled())

  const timestamps = filterOfCall(0).match(/\d{4}-\d{2}-\d{2}T[\d:.]+Z/g) ?? []
  expect(timestamps).toHaveLength(2)
  for (const timestamp of timestamps) {
    expect(timestamp).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/)
  }
})

test('the appointments are asked for oldest first', async () => {
  render(<CalendarPage />)

  await waitFor(() => expect(getFullList).toHaveBeenCalled())
  expect(getFullList.mock.calls[0][0].sort).toBe('start_time')
})

test('moving to the next month asks for that month instead', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderOnGrid()

  await waitFor(() => expect(getFullList).toHaveBeenCalledTimes(1))

  await user.click(screen.getByRole('button', { name: 'Siguiente' }))

  await waitFor(() => expect(getFullList).toHaveBeenCalledTimes(2))
  expect(filterOfCall(1)).toContain(isoMillis(new Date(2026, 10, 24, 0, 0, 0, 0)))
})

test('clicking an appointment on the grid opens it for editing', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderOnGrid()

  await user.click(await screen.findByText('Ana Ruiz'))

  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Editar cita')).toBeDefined()
  expect(
    (within(dialog).getByLabelText(/nombre del cliente/i) as HTMLInputElement).value,
  ).toBe('Ana Ruiz')
  // Opening an appointment is how its notes are read, so the stored text has
  // to be on screen rather than only in the row.
  expect(
    (within(dialog).getByLabelText(/notas/i) as HTMLTextAreaElement).value,
  ).toBe('See you at the back entrance, Ana.')
})

test('clicking a row of the day’s list opens the same dialog', async () => {
  // The list is where the day is worked through, so confirming somebody
  // from it has to be one click away — the same click the grid takes.
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  render(<CalendarPage />)

  await user.click(await screen.findByRole('button', { name: /Ana Ruiz/ }))

  expect(within(await screen.findByRole('dialog')).getByText('Editar cita')).toBeDefined()
})

test('saving from the dialog reloads the appointments', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderOnGrid()

  await user.click(await screen.findByText('Ana Ruiz'))
  await screen.findByRole('dialog')

  const before = getFullList.mock.calls.length
  const update = vi.fn().mockResolvedValue(appointment())
  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
    getFirstListItem,
    update,
  } as unknown as ReturnType<typeof pb.collection>)

  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  await waitFor(() => expect(getFullList.mock.calls.length).toBeGreaterThan(before))
})

test('a failed load is reported rather than leaving an empty calendar', async () => {
  getFullList.mockRejectedValue(new Error('Something went wrong.'))

  render(<CalendarPage />)

  expect((await screen.findByRole('alert')).textContent).toMatch(/something went wrong/i)
})

test('settings that cannot be read leave the day on screen rather than blanking it', async () => {
  // The day's list is the point of the screen. Losing the settings row is a
  // reason to show a clinic the default zone, not to show it nothing.
  getFirstListItem.mockRejectedValue(new Error('nope'))

  render(<CalendarPage />)

  expect(await screen.findByText('Ana Ruiz')).toBeDefined()
})

// Selecting a slot by dragging is not covered here: react-big-calendar's
// selection reads real geometry (getBoundingClientRect, elementFromPoint),
// which jsdom does not have, so a click in this environment never resolves to
// a cell. It is exercised against a real browser instead.
