import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { CalendarEvent } from '@/lib/calendar-events'
import { pb } from '@/lib/pocketbase'
import {
  HORIZON_DAYS,
  listSentReminders,
  listUpcomingReminders,
  upcomingWindow,
} from '@/lib/reminders'

/**
 * The seam is the PocketBase SDK, stubbed per collection. What is under test
 * is everything above it: which appointments count as having a reminder
 * coming, how the chosen lead time moves that boundary, and which ones drop
 * off because a reminder already went out.
 */
const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
}

/** Fixed so every expectation below is an arithmetic fact, not a race. */
const NOW = new Date('2026-11-02T12:00:00.000Z')

let appointments: CalendarEvent[] = []
let sentMessages: unknown[] = []
let sentFailure: unknown = null
/** Every filter string the appointment query was given. */
let appointmentFilters: string[] = []

function appointment(overrides: Partial<CalendarEvent>): CalendarEvent {
  return {
    id: 'evt1',
    user_id: signedInUser.id,
    title: 'Ana Ruiz',
    client_phone: '5215512345678',
    notes: 'See you tomorrow.',
    start_time: '2026-11-03T17:00:00.000Z',
    end_time: '2026-11-03T18:00:00.000Z',
    ...overrides,
  }
}

beforeEach(() => {
  appointments = []
  sentMessages = []
  sentFailure = null
  appointmentFilters = []

  vi.spyOn(pb, 'collection').mockImplementation((name: string) => {
    if (name === 'calendar_events') {
      return {
        getFullList: (options: { filter?: string }) => {
          appointmentFilters.push(options?.filter ?? '')
          return Promise.resolve(appointments)
        },
      } as unknown as ReturnType<typeof pb.collection>
    }

    return {
      getList: () =>
        sentFailure
          ? Promise.reject(sentFailure)
          : Promise.resolve({ items: sentMessages }),
    } as unknown as ReturnType<typeof pb.collection>
  })

  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('the window starts now: an appointment already under way needs no reminder', () => {
  expect(upcomingWindow(1440, NOW).from).toEqual(NOW)
})

test('the window reaches a lead time past the horizon, so it holds 30 days of reminders', () => {
  // The list is of reminders, not appointments. A reminder going out on the
  // last day of the horizon belongs to an appointment one lead time later,
  // which is why the far edge moves when the lead time does.
  expect(upcomingWindow(1440, NOW).to).toEqual(new Date('2026-12-03T12:00:00.000Z'))
  expect(upcomingWindow(30, NOW).to).toEqual(new Date('2026-12-02T12:30:00.000Z'))
  expect(HORIZON_DAYS).toBe(30)
})

test('the chosen lead time reaches the database, in the stored timestamp shape', async () => {
  await listUpcomingReminders(30, NOW)

  expect(appointmentFilters).toHaveLength(1)
  // Byte-exact: start_time is a text column and the bound is compared
  // lexicographically, so a trimmed fraction would silently change the window.
  expect(appointmentFilters[0]).toContain('2026-11-02T12:00:00.000Z')
  expect(appointmentFilters[0]).toContain('2026-12-02T12:30:00.000Z')
})

test('an appointment with no phone number gets no reminder', async () => {
  appointments = [
    appointment({ id: 'withPhone' }),
    appointment({ id: 'noPhone', client_phone: '' }),
    appointment({ id: 'phoneMissing', client_phone: undefined }),
  ]

  const upcoming = await listUpcomingReminders(1440, NOW)

  expect(upcoming.map((r) => r.event.id)).toEqual(['withPhone'])
})

test('a reminder that already went out drops off the list', async () => {
  appointments = [appointment({ id: 'reminded' }), appointment({ id: 'notYet' })]
  sentMessages = [
    { id: 'msg1', event_id: 'reminded', to_number: '5215512345678', sent_at: '2026-11-02T11:00:00.000Z' },
  ]

  const upcoming = await listUpcomingReminders(1440, NOW)

  expect(upcoming.map((r) => r.event.id)).toEqual(['notYet'])
})

test('each entry says when its reminder goes out, a lead time before the appointment', async () => {
  appointments = [appointment({ start_time: '2026-11-03T17:00:00.000Z' })]

  const [withADay] = await listUpcomingReminders(1440, NOW)
  expect(withADay.reminderAt).toEqual(new Date('2026-11-02T17:00:00.000Z'))

  const [withTwoHours] = await listUpcomingReminders(120, NOW)
  expect(withTwoHours.reminderAt).toEqual(new Date('2026-11-03T15:00:00.000Z'))
})

test('a reminder whose moment has passed is due, not merely scheduled', async () => {
  appointments = [
    appointment({ id: 'soon', start_time: '2026-11-02T13:00:00.000Z' }),
    appointment({ id: 'later', start_time: '2026-11-04T13:00:00.000Z' }),
  ]

  const upcoming = await listUpcomingReminders(1440, NOW)

  expect(upcoming.map((r) => [r.event.id, r.isDue])).toEqual([
    ['soon', true],
    ['later', false],
  ])
})

test('the soonest reminder is listed first', async () => {
  appointments = [
    appointment({ id: 'third', start_time: '2026-11-20T09:00:00.000Z' }),
    appointment({ id: 'first', start_time: '2026-11-03T09:00:00.000Z' }),
    appointment({ id: 'second', start_time: '2026-11-09T09:00:00.000Z' }),
  ]

  const upcoming = await listUpcomingReminders(1440, NOW)

  expect(upcoming.map((r) => r.event.id)).toEqual(['first', 'second', 'third'])
})

test('nothing has been sent until the scheduler lands, and that is not an error', async () => {
  // whatsapp_messages is created by #6 and filled by #9. Until then the
  // query 404s on a collection that is not there yet, and the page has to
  // show an empty Sent column rather than fall over.
  sentFailure = Object.assign(new Error('Missing or invalid collection context.'), {
    status: 404,
  })

  await expect(listSentReminders()).resolves.toEqual([])
  await expect(listUpcomingReminders(1440, NOW)).resolves.toEqual([])
})

test('a real failure reading the sent reminders is not passed off as "none"', async () => {
  sentFailure = Object.assign(new Error('Something went wrong'), { status: 500 })

  await expect(listSentReminders()).rejects.toThrow()
})
