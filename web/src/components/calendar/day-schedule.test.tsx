import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, expect, test, vi } from 'vitest'

import { DaySchedule } from '@/components/calendar/day-schedule'
import type { Appointment } from '@/lib/appointments'

/**
 * The seam is the component's own props: a set of appointments, the clinic's
 * zone, and the instant it is being looked at. Everything the morning screen
 * promises — which rows, in what order, with what counts — is decided inside
 * it, so it can be asked for all of that without a server.
 *
 * The process runs in Madrid throughout. Tijuana is nine hours behind it, so
 * every expectation below is a day or a date out if anything reaches for the
 * machine's own zone instead of the clinic's.
 */
const VIEWER_TZ = process.env.TZ

const CLINIC_TZ = 'America/Tijuana'

beforeAll(() => {
  process.env.TZ = 'Europe/Madrid'
})

afterAll(() => {
  process.env.TZ = VIEWER_TZ
})

afterEach(cleanup)

/**
 * 07:00 UTC on 3 November 2026. It is 08:00 on the 3rd in Madrid and 23:00
 * on the *2nd* in Tijuana — the late-night check the issue is about.
 */
const NOW = new Date('2026-11-03T07:00:00.000Z')

function appointment(overrides: Partial<Appointment> = {}): Appointment {
  return {
    id: 'evt1',
    user_id: 'user123',
    client_name: 'Ana Ruiz',
    // 17:00 on 2 November in Tijuana.
    start_time: '2026-11-03T01:00:00.000Z',
    end_time: '2026-11-03T02:00:00.000Z',
    status: 'pending',
    language: 'es',
    ...overrides,
  }
}

/** The patient names on the list, in the order they are rendered. */
function rowNames(): string[] {
  return screen
    .getAllByRole('listitem')
    .map((row) => within(row).getByTestId('client').textContent ?? '')
}

test('the day shown is the clinic’s, even when the viewer has already turned the page', () => {
  // 23:00 in Tijuana, 08:00 the next morning in Madrid. A clinic checking
  // the screen after closing must be shown the day it just worked.
  render(<DaySchedule appointments={[]} timeZone={CLINIC_TZ} now={NOW} />)

  expect(screen.getByText('lunes 2 de noviembre de 2026')).toBeDefined()
})

test('only the appointments on the clinic’s day are listed', () => {
  const rows = [
    // 16:00 on 1 November in Tijuana — yesterday.
    appointment({ id: 'a', client_name: 'Ayer', start_time: '2026-11-02T00:00:00.000Z' }),
    appointment({ id: 'b', client_name: 'Hoy' }),
    // 16:00 on 3 November in Tijuana — tomorrow.
    appointment({ id: 'c', client_name: 'Mañana', start_time: '2026-11-04T00:00:00.000Z' }),
  ]

  render(<DaySchedule appointments={rows} timeZone={CLINIC_TZ} now={NOW} />)

  expect(rowNames()).toEqual(['Hoy'])
})

test('the list runs in time order whatever order it arrived in', () => {
  const rows = [
    appointment({ id: 'b', client_name: 'Tarde', start_time: '2026-11-03T01:00:00.000Z' }),
    appointment({ id: 'a', client_name: 'Mañana', start_time: '2026-11-02T16:00:00.000Z' }),
  ]

  render(<DaySchedule appointments={rows} timeZone={CLINIC_TZ} now={NOW} />)

  expect(rowNames()).toEqual(['Mañana', 'Tarde'])
})

test('a row carries the time on the clinic’s clock and where the patient stands', () => {
  render(
    <DaySchedule
      appointments={[appointment({ status: 'confirmed' })]}
      timeZone={CLINIC_TZ}
      now={NOW}
    />,
  )

  const row = screen.getByRole('listitem')
  // 01:00 UTC is 17:00 in Tijuana and 02:00 in Madrid.
  expect(within(row).getByText('17:00')).toBeDefined()
  expect(within(row).getByText('Ana Ruiz')).toBeDefined()
  expect(within(row).getByText('Confirmada')).toBeDefined()
})

test('the day’s confirmed, pending and cancelled are counted across the top', () => {
  // The question the screen exists to answer is how much of the day is
  // still unconfirmed, and it has to be answerable without reading the rows.
  const rows = [
    appointment({ id: 'a', status: 'confirmed' }),
    appointment({ id: 'b', status: 'confirmed' }),
    appointment({ id: 'c', status: 'pending' }),
    appointment({ id: 'd', status: 'cancelled' }),
    // Yesterday's, and it must not be counted into today's totals.
    appointment({ id: 'e', status: 'confirmed', start_time: '2026-11-02T00:00:00.000Z' }),
  ]

  render(<DaySchedule appointments={rows} timeZone={CLINIC_TZ} now={NOW} />)

  expect(screen.getByTestId('count-confirmed').textContent).toContain('2')
  expect(screen.getByTestId('count-pending').textContent).toContain('1')
  expect(screen.getByTestId('count-cancelled').textContent).toContain('1')
})

test('a day with nothing in it says so rather than rendering blank', () => {
  // An empty list and a failed load look identical on screen, and one of
  // them means somebody should go home.
  render(<DaySchedule appointments={[]} timeZone={CLINIC_TZ} now={NOW} />)

  expect(screen.getByText(/no hay citas/i)).toBeDefined()
  expect(screen.queryAllByRole('listitem')).toHaveLength(0)
})

test('a proposal and a cancellation are marked the way the grid marks them', () => {
  // The same two states the grid dims, so a glance at either view reads the
  // same. internal/appointments is also the pair no reminder goes out for.
  const rows = [
    appointment({ id: 'a', client_name: 'Propuesta', status: 'proposed' }),
    appointment({ id: 'b', client_name: 'Cancelada', status: 'cancelled' }),
  ]

  render(<DaySchedule appointments={rows} timeZone={CLINIC_TZ} now={NOW} />)

  const [proposed, cancelled] = screen.getAllByRole('listitem')
  expect(proposed.className).toContain('opacity-70')
  expect(cancelled.className).toContain('line-through')
})

test('opening a row hands back the appointment behind it', async () => {
  const user = userEvent.setup()
  const onSelect = vi.fn()
  const row = appointment()

  render(
    <DaySchedule
      appointments={[row]}
      timeZone={CLINIC_TZ}
      now={NOW}
      onSelect={onSelect}
    />,
  )

  await user.click(screen.getByRole('button', { name: /Ana Ruiz/ }))

  expect(onSelect).toHaveBeenCalledWith(row)
})
