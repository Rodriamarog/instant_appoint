import { isoMillis } from '@/lib/iso-time'
import { pb } from '@/lib/pocketbase'

/**
 * Appointments, as the app talks about them. This module is the only place
 * that knows the collection is called appointments, how the month window
 * is expressed as a filter, and that a create has to carry its owner.
 *
 * There is no server-side handler behind it: the SPA reaches PocketBase
 * directly and the collection's access rules are what keep one person's
 * appointments away from another's. That is why createAppointment stamps
 * user_id from the session rather than taking it as an argument — the create
 * rule compares the submitted user_id against the caller, so a value from
 * anywhere else is rejected with a 400 anyway.
 */
export const COLLECTION = 'appointments'

/**
 * Where an appointment stands. The four are a closed set and they are the
 * same four strings as internal/appointments on the Go side, which is what
 * the reminder job reads. A spelling that drifts from it fails nothing: it
 * quietly reminds about a cancelled appointment, or stops reminding about a
 * real one.
 */
export type AppointmentStatus = 'proposed' | 'pending' | 'confirmed' | 'cancelled'

/**
 * Who did something to an appointment. Staff work in this app, the patient
 * taps a button in WhatsApp, the assistant acts on a conversation it read.
 */
export type Actor = 'staff' | 'patient' | 'assistant'

/** An appointment row. */
export interface Appointment {
  id: string
  user_id: string
  client_name: string
  client_phone?: string
  notes?: string
  start_time: string
  end_time: string
  location?: string

  status: AppointmentStatus

  /**
   * How it reached that status, and when.
   *
   * Typed as a plain string rather than as Actor because an unset text
   * column comes back as '': a row written by something that did not record
   * itself has to be displayable, not a value the UI can be sure of.
   */
  status_set_by?: string
  status_set_at?: string

  /** Who booked it in the first place. Same '' caveat as status_set_by. */
  created_by?: string

  /**
   * The conversation it came out of, when it came out of one. Empty for an
   * appointment somebody booked at the desk.
   */
  conversation_id?: string
}

/**
 * What the dialog's form can set: the details of the appointment itself.
 *
 * Status and provenance are deliberately not in it. They are not fields
 * anybody types — they are recorded by the act that changes them, which is
 * what setAppointmentStatus is for. A draft carrying them would let a
 * corrected phone number restamp who last confirmed the appointment, and the
 * one question the screen exists to answer would stop being answerable.
 */
export type AppointmentDraft = Pick<
  Appointment,
  'client_name' | 'client_phone' | 'notes' | 'start_time' | 'end_time' | 'location'
>

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
): Promise<Appointment[]> {
  const filter = pb.filter('end_time >= {:from} && start_time <= {:to}', {
    from: isoMillis(from),
    to: isoMillis(to),
  })

  return pb.collection(COLLECTION).getFullList<Appointment>({
    filter,
    sort: 'start_time',
  })
}

/** This app is the one staff use. Nothing else writes through it. */
const STAFF: Actor = 'staff'

/**
 * How each status is named on screen. Feminine, because the thing being
 * described is "la cita".
 */
const STATUS_LABELS: Record<AppointmentStatus, string> = {
  proposed: 'Propuesta',
  pending: 'Pendiente',
  confirmed: 'Confirmada',
  cancelled: 'Cancelada',
}

/**
 * The status in words.
 *
 * An unrecognised value is shown as itself rather than hidden: a row written
 * by something this build has not heard of is exactly the row somebody needs
 * to see, and a blank where the status should be reads as "no status".
 */
export function statusLabel(status: AppointmentStatus): string {
  return STATUS_LABELS[status] ?? status
}

/** Who did it, as the sentence "… por el paciente" needs them. */
const ACTOR_LABELS: Record<Actor, string> = {
  staff: 'el personal',
  patient: 'el paciente',
  assistant: 'el asistente',
}

/**
 * Who did it, or null when the row does not say.
 *
 * Null rather than a guess: claiming the patient cancelled an appointment
 * nothing recorded the author of is worse than admitting nobody knows.
 */
export function actorLabel(actor: string | undefined): string | null {
  return ACTOR_LABELS[actor as Actor] ?? null
}

/**
 * Writes a new appointment, owned by whoever is signed in.
 *
 * It starts pending, booked by staff: this app has no other kind of user, so
 * the provenance is known here rather than asked of every caller. Pending
 * and not confirmed, because nobody has asked the patient anything yet — a
 * new booking that called itself confirmed would put a clinic's whole day on
 * the screen as settled.
 */
export async function createAppointment(
  draft: AppointmentDraft,
): Promise<Appointment> {
  const owner = pb.authStore.record?.id
  if (!owner) {
    throw new Error('Your session has expired. Sign in again to save this.')
  }

  return pb.collection(COLLECTION).create<Appointment>({
    ...draft,
    user_id: owner,
    status: 'pending',
    created_by: STAFF,
    status_set_by: STAFF,
    status_set_at: isoMillis(new Date()),
  })
}

/**
 * Moves an appointment to a new status, recording that staff did it and when.
 *
 * Separate from updateAppointment because the two are different acts. This
 * one is somebody deciding something about the appointment, and it is the
 * only place in the SPA that writes the provenance of a status — the webhook
 * that handles a patient's own Confirm or Cancel writes the same three
 * fields on the Go side, attributed to the patient.
 */
export async function setAppointmentStatus(
  id: string,
  status: AppointmentStatus,
): Promise<Appointment> {
  return pb.collection(COLLECTION).update<Appointment>(id, {
    status,
    status_set_by: STAFF,
    status_set_at: isoMillis(new Date()),
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
): Promise<Appointment> {
  return pb.collection(COLLECTION).update<Appointment>(id, draft)
}

/** Removes an appointment. */
export async function deleteAppointment(id: string): Promise<void> {
  await pb.collection(COLLECTION).delete(id)
}
