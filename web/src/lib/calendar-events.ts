import { isoMillis } from '@/lib/iso-time'
import { pb } from '@/lib/pocketbase'

/**
 * Appointments, as the app talks about them. This module is the only place
 * that knows the collection is called calendar_events, how the month window
 * is expressed as a filter, and that a create has to carry its owner.
 *
 * There is no server-side handler behind it: the SPA reaches PocketBase
 * directly and the collection's access rules are what keep one person's
 * appointments away from another's. That is why createAppointment stamps
 * user_id from the session rather than taking it as an argument — the create
 * rule compares the submitted user_id against the caller, so a value from
 * anywhere else is rejected with a 400 anyway.
 */
export const COLLECTION = 'calendar_events'

/** An appointment row. */
export interface CalendarEvent {
  id: string
  user_id: string
  title: string
  client_phone?: string
  notes?: string
  start_time: string
  end_time: string
  location?: string
}

/** Everything the dialog can set. The id and the owner are not its business. */
export type AppointmentDraft = Omit<CalendarEvent, 'id' | 'user_id'>

/**
 * Every appointment that overlaps the window, oldest first.
 *
 * The bounds are compared against two *text* columns, so they have to be
 * written in the same fixed-width shape as the stored values — hence
 * isoMillis rather than anything the caller happens to have to hand.
 */
export async function listAppointmentsBetween(
  from: Date,
  to: Date,
): Promise<CalendarEvent[]> {
  const filter = pb.filter('end_time >= {:from} && start_time <= {:to}', {
    from: isoMillis(from),
    to: isoMillis(to),
  })

  return pb.collection(COLLECTION).getFullList<CalendarEvent>({
    filter,
    sort: 'start_time',
  })
}

/** Writes a new appointment, owned by whoever is signed in. */
export async function createAppointment(
  draft: AppointmentDraft,
): Promise<CalendarEvent> {
  const owner = pb.authStore.record?.id
  if (!owner) {
    throw new Error('Your session has expired. Sign in again to save this.')
  }

  return pb.collection(COLLECTION).create<CalendarEvent>({
    ...draft,
    user_id: owner,
  })
}

/**
 * Replaces the contents of an existing appointment.
 *
 * user_id is deliberately not sent: ownership is not something an edit is
 * allowed to change, and the update rule refuses the attempt regardless.
 */
export async function updateAppointment(
  id: string,
  draft: AppointmentDraft,
): Promise<CalendarEvent> {
  return pb.collection(COLLECTION).update<CalendarEvent>(id, draft)
}

/** Removes an appointment. */
export async function deleteAppointment(id: string): Promise<void> {
  await pb.collection(COLLECTION).delete(id)
}
