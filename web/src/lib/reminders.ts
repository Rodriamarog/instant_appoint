import { listAppointmentsBetween, type CalendarEvent } from '@/lib/calendar-events'
import { pb } from '@/lib/pocketbase'

/**
 * Reminders, as the reminders page talks about them.
 *
 * A reminder is not a stored thing until it has been sent: an upcoming one is
 * derived, on the spot, from an appointment that has somewhere to send to and
 * has not been reminded about yet. This module owns that derivation, and the
 * read of the sent ones.
 *
 * The scheduler that actually sends them is #9. Everything here is a view.
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

/** A reminder that has already been sent. */
export interface SentReminder {
  id: string
  event_id: string
  to_number: string
  message_content: string
  sent_at: string
  status: string
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

  const [appointments, sent] = await Promise.all([
    listAppointmentsBetween(from, to),
    listSentReminders(),
  ])

  const alreadyReminded = new Set(sent.map((message) => message.event_id))

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

/**
 * The reminders that have gone out, most recent first.
 *
 * whatsapp_messages is a shared log — chat writes to it too — so the two
 * markers that make a row a *reminder* are part of the query rather than
 * assumed. #9 is what starts writing them; until then this is empty, and the
 * collection itself does not exist until #6 creates it, which is why a 404 is
 * read as "none" rather than as a failure.
 */
export async function listSentReminders(): Promise<SentReminder[]> {
  const owner = pb.authStore.record?.id ?? ''

  try {
    const page = await pb.collection('whatsapp_messages').getList<SentReminder>(1, 50, {
      filter: pb.filter(
        'user_id = {:userId} && message_type = "reminder" && status = "sent"',
        { userId: owner },
      ),
      sort: '-sent_at',
    })

    return page.items
  } catch (error) {
    if ((error as { status?: number } | null)?.status === 404) {
      return []
    }
    throw error
  }
}
