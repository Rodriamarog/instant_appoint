import { listAppointmentsBetween, type CalendarEvent } from '@/lib/calendar-events'
import { remindedEventIds } from '@/lib/messages'

/**
 * Reminders, as the reminders page talks about them.
 *
 * A reminder is not a stored thing until it has been sent: an upcoming one is
 * derived, on the spot, from an appointment that has somewhere to send to and
 * has not been reminded about yet. This module owns that derivation.
 *
 * The scheduler that actually sends them is the Go cron. Everything here is a
 * view, and the reads of the message log behind it belong to lib/messages —
 * this module owns the derivation, not the collection.
 */

/**
 * How far ahead the list looks, measured in *reminders* rather than
 * appointments — see upcomingWindow.
 */
export const HORIZON_DAYS = 30

const MINUTE = 60 * 1000
const DAY = 24 * 60 * MINUTE

/** An appointment with a reminder still to go out. */
export interface UpcomingReminder {
  event: CalendarEvent
  /** When the reminder is due to be sent: a lead time before the start. */
  reminderAt: Date
  /** True once that moment has passed and the reminder has still not gone. */
  isDue: boolean
}

/**
 * The span of appointment start times whose reminders fall in the next
 * HORIZON_DAYS.
 *
 * The near edge is now: an appointment that has already begun is past being
 * reminded about, which is a deliberate narrowing of the page this replaces —
 * it kept showing appointments for an hour after they started.
 *
 * The far edge moves with the lead time, because the list is of reminders. A
 * reminder going out on the last day of the horizon belongs to an appointment
 * one lead time further out, so a 24-hour lead reaches a day deeper into the
 * calendar than a 30-minute one.
 */
export function upcomingWindow(
  leadTimeMinutes: number,
  now: Date,
): { from: Date; to: Date } {
  return {
    from: now,
    to: new Date(now.getTime() + HORIZON_DAYS * DAY + leadTimeMinutes * MINUTE),
  }
}

/**
 * The reminders still to go out, soonest first.
 *
 * `now` is a parameter rather than read here so the caller — and the tests —
 * decide what "now" is for a whole render.
 */
export async function listUpcomingReminders(
  leadTimeMinutes: number,
  now: Date = new Date(),
): Promise<UpcomingReminder[]> {
  const { from, to } = upcomingWindow(leadTimeMinutes, now)

  const appointments = await listAppointmentsBetween(from, to)

  // Asked about exactly these appointments, and not read off the page of sent
  // reminders the page displays. That list is capped at what a person will
  // look at; this one decides whether a reminder still has to go out, and a
  // cap on it means the 51st-oldest sent reminder reappears as upcoming and
  // due for ever after.
  const alreadyReminded = await remindedEventIds(appointments.map((event) => event.id))

  return appointments
    .filter((event) => Boolean(event.client_phone) && !alreadyReminded.has(event.id))
    .map((event) => {
      const reminderAt = new Date(
        new Date(event.start_time).getTime() - leadTimeMinutes * MINUTE,
      )

      return { event, reminderAt, isDue: reminderAt <= now }
    })
    .sort((a, b) => a.reminderAt.getTime() - b.reminderAt.getTime())
}
