import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { isoMillis } from '@/lib/iso-time'
import { pb } from '@/lib/pocketbase'
import { CalendarPage } from '@/pages/calendar'

/**
 * The seam is the PocketBase SDK. Above it: which appointments the page asks
 * for as you navigate, what it renders, and what clicking does.
 */
const getFullList = vi.fn()

/** Mid-November 2026, so the month window has a whole month either side. */
const today = new Date(2026, 10, 15, 12, 0, 0)

function appointment(overrides: Record<string, unknown> = {}) {
  return {
    id: 'evt123',
    user_id: 'user123',
    title: 'Ana Ruiz',
    client_phone: '5215512345678',
    notes: 'See you at the back entrance, Ana.',
    start_time: isoMillis(new Date(2026, 10, 12, 10, 0)),
    end_time: isoMillis(new Date(2026, 10, 12, 11, 0)),
    location: 'Suite 2',
    ...overrides,
  }
}

beforeEach(() => {
  // react-big-calendar's drag-select asks the document what is under the
  // pointer on every mousedown; jsdom has no layout and no such method.
  document.elementFromPoint = () => null

  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(today)

  getFullList.mockReset().mockResolvedValue([appointment()])
  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
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

test('the appointments in view are rendered', async () => {
  render(<CalendarPage />)

  expect(await screen.findByText('Ana Ruiz')).toBeDefined()
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
  render(<CalendarPage />)

  await waitFor(() => expect(getFullList).toHaveBeenCalledTimes(1))

  await user.click(screen.getByRole('button', { name: 'Next' }))

  await waitFor(() => expect(getFullList).toHaveBeenCalledTimes(2))
  expect(filterOfCall(1)).toContain(isoMillis(new Date(2026, 10, 24, 0, 0, 0, 0)))
})

test('month, week and day views each render and can be navigated', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  render(<CalendarPage />)

  await waitFor(() => expect(getFullList).toHaveBeenCalled())
  expect(screen.getByText('November 2026')).toBeDefined()

  await user.click(screen.getByRole('button', { name: 'Week' }))
  expect(screen.getByRole('button', { name: 'Week' }).className).toContain('rbc-active')

  await user.click(screen.getByRole('button', { name: 'Day' }))
  expect(screen.getByRole('button', { name: 'Day' }).className).toContain('rbc-active')

  // Day view navigates a day at a time. The month window already covers the
  // next day, so nothing is refetched — but the day on show has to move.
  const shown = toolbarLabel()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await waitFor(() => expect(toolbarLabel()).not.toBe(shown))

  await user.click(screen.getByRole('button', { name: 'Month' }))
  expect(screen.getByRole('button', { name: 'Month' }).className).toContain('rbc-active')
  expect(screen.getByText('November 2026')).toBeDefined()
})

test('clicking an appointment opens it for editing', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  render(<CalendarPage />)

  await user.click(await screen.findByText('Ana Ruiz'))

  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Edit Appointment')).toBeDefined()
  expect((within(dialog).getByLabelText(/client name/i) as HTMLInputElement).value).toBe(
    'Ana Ruiz',
  )
  // Its saved reminder, not a freshly generated one.
  expect(
    (within(dialog).getByLabelText(/reminder message/i) as HTMLTextAreaElement).value,
  ).toBe('See you at the back entrance, Ana.')
})

test('saving from the dialog reloads the appointments', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  render(<CalendarPage />)

  await user.click(await screen.findByText('Ana Ruiz'))
  await screen.findByRole('dialog')

  const before = getFullList.mock.calls.length
  const update = vi.fn().mockResolvedValue(appointment())
  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
    update,
  } as unknown as ReturnType<typeof pb.collection>)

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  await waitFor(() => expect(getFullList.mock.calls.length).toBeGreaterThan(before))
})

test('a failed load is reported rather than leaving an empty calendar', async () => {
  getFullList.mockRejectedValue(new Error('Something went wrong.'))

  render(<CalendarPage />)

  expect((await screen.findByRole('alert')).textContent).toMatch(/something went wrong/i)
})

// Selecting a slot by dragging is not covered here: react-big-calendar's
// selection reads real geometry (getBoundingClientRect, elementFromPoint),
// which jsdom does not have, so a click in this environment never resolves to
// a cell. It is exercised against a real browser instead.
