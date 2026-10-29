import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { CalendarEvent } from '@/lib/calendar-events'
import { pb } from '@/lib/pocketbase'
import { HORIZON_DAYS, listUpcomingReminders, upcomingWindow } from '@/lib/reminders'

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
let sentMessages: { event_id: string }[] = []
/** Every filter string the appointment query was given. */
let appointmentFilters: string[] = []
/** Every filter string the message log was asked for. */
let messageFilters: string[] = []
/** How many rows one page of the message log returns, at most. */
let messagePageSize = Infinity

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
  appointmentFilters = []
  messageFilters = []
  messagePageSize = Infinity

  vi.spyOn(pb, 'collection').mockImplementation((name: string) => {
    if (name === 'calendar_events') {
      return {
        getFullList: (options: { filter?: string }) => {
          appointmentFilters.push(options?.filter ?? '')
          return Promise.resolve(appointments)
        },
      } as unknown as ReturnType<typeof pb.collection>
    }

    // The message log answers only what the filter asked about, and never
    // more than one page of it — which is the whole of what this is here to
    // check.
    return {
      getFullList: (options: { filter?: string }) => {
        const filter = options?.filter ?? ''
        messageFilters.push(filter)

        return Promise.resolve(
          sentMessages
            .filter((message) => filter.includes(`'${message.event_id}'`))
            .slice(0, messagePageSize),
        )
      },
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
  sentMessages = [{ event_id: 'reminded' }]

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

/**
 * The bug this replaces: the exclusion set was built from one 50-row page of
 * sent reminders. Past fifty, the oldest fell out of that page and every
 * appointment they belonged to came back as upcoming — and, being in the
 * past, as due. A reminder sent months ago was announced as about to go out,
 * for ever.
 */
test('a reminder that went out long ago stays off the list, however many have been sent since', async () => {
  const many = Array.from({ length: 140 }, (_, index) => `evt${index}`)
  appointments = many.map((id) => appointment({ id }))
  sentMessages = many.map((event_id) => ({ event_id }))
  // Whatever one page holds, the answer cannot depend on it.
  messagePageSize = 50

  expect(await listUpcomingReminders(1440, NOW)).toEqual([])
})

test('the already-sent query asks about the appointments in the window, not about history', async () => {
  appointments = [appointment({ id: 'evtA' }), appointment({ id: 'evtB' })]

  await listUpcomingReminders(1440, NOW)

  expect(messageFilters.join(' ')).toContain("'evtA'")
  expect(messageFilters.join(' ')).toContain("'evtB'")
})

test('no appointments in the window means the message log is not asked at all', async () => {
  expect(await listUpcomingReminders(1440, NOW)).toEqual([])
  expect(messageFilters).toEqual([])
})
