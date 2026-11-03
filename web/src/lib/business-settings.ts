import { pb } from '@/lib/pocketbase'

/**
 * One business's settings: how far ahead its reminders go out, whether they
 * go out at all, the approved template they are written in, and the zone the
 * business keeps time in.
 *
 * This module is the only place that knows the collection is called
 * business_settings, that there is exactly one row per person, and what to
 * show somebody who has never opened the settings dialog.
 *
 * There is no server-side handler behind it: the SPA reaches PocketBase
 * directly and the collection's rules are the whole of the access control.
 * That is why a create stamps user_id from the session rather than taking it
 * as an argument — the create rule compares the submitted user_id against the
 * caller and runs before any hook, so a value from anywhere else is a 400.
 */
export const COLLECTION = 'business_settings'

/**
 * How much a clinic has let the assistant do with its patients' messages.
 *
 * The three words are a storage format: they are what the column holds, what
 * migrations/1793232008_ai_mode.go writes, and what internal/webhook compares
 * against to decide whether a patient hears from a bot. Renaming one here
 * without renaming it there leaves every stored row meaning nothing.
 */
export type AIMode = 'off' | 'observer' | 'autopilot'

/**
 * The three positions, in the order they give the assistant more rope, with
 * the plain-language explanation each one is offered under.
 *
 * The explanation is part of the option rather than prose beside the control
 * because the difference between "no responde" and "no responde todavía" is
 * the whole decision somebody is making here, and it is not guessable from a
 * three-word label.
 */
export const AI_MODE_OPTIONS: { mode: AIMode; label: string; help: string }[] = [
  {
    mode: 'off',
    label: 'Apagado',
    help: 'El asistente no responde ni escribe nada. Los mensajes de tus pacientes se siguen guardando y los ves en Chat.',
  },
  {
    mode: 'observer',
    label: 'Observador',
    help: 'El asistente no responde. Solo observa las conversaciones que atiende tu clínica.',
  },
  {
    mode: 'autopilot',
    label: 'Piloto automático',
    help: 'El asistente responde solo, por WhatsApp, a los mensajes recientes de tus pacientes.',
  },
]

/** One business's settings. */
export interface BusinessSettings {
  /** Absent until the first save. */
  id?: string
  /** How many minutes before the appointment the reminder goes out. */
  timing_minutes: number
  is_active: boolean
  /** The approved Meta template, or '' while none has been chosen. */
  template_name: string
  template_language: string
  /** An IANA zone name; the date and time in a reminder are spelled in it. */
  timezone: string
  /** What the assistant is allowed to do with inbound messages. */
  ai_mode: AIMode
}

/**
 * What somebody sees before they have saved anything.
 *
 * The same values are declared in migrations/1793232003_reminder_settings.go,
 * where the language and the zone are create-time field defaults, so a row
 * written by anything that skips them matches what the page would have shown.
 */
export const DEFAULT_SETTINGS: BusinessSettings = {
  timing_minutes: 1440,
  is_active: true,
  template_name: '',
  template_language: 'es',
  timezone: 'America/Tijuana',
  // Off, and not because nothing better is known: a clinic that has never
  // opened this page has not agreed to anything its patients will be told,
  // and switching the assistant on deliberately is the moment it takes
  // responsibility for what the assistant says. The same value is the column
  // default and the backfill in migrations/1793232008_ai_mode.go, so a clinic
  // that predates the switch and one that registered today are in the same
  // position.
  ai_mode: 'off',
}

/** The lead times the dialog offers, longest-used first in the old UI order. */
export const TIMING_OPTIONS = [
  { label: '30 min', minutes: 30 },
  { label: '1 hora', minutes: 60 },
  { label: '2 horas', minutes: 120 },
  { label: '12 horas', minutes: 720 },
  { label: '24 horas', minutes: 1440 },
]

/**
 * The zones the dialog offers.
 *
 * A reminder names a date and a time, and the appointment behind it is a UTC
 * instant, so the zone is what decides which hour the customer is told. The
 * list is short on purpose: the full IANA set is some six hundred names, and
 * a business sends from one place. A zone already saved that is not on this
 * list is kept and offered alongside them.
 */
export const TIMEZONE_OPTIONS = [
  'America/Tijuana',
  'America/Hermosillo',
  'America/Chihuahua',
  'America/Mazatlan',
  'America/Mexico_City',
  'America/Merida',
  'America/Cancun',
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
]

/** True for the SDK error that means "the query matched nothing". */
function isNotFound(error: unknown): boolean {
  return (error as { status?: number } | null)?.status === 404
}

/** The signed-in person's row, or null if they have never saved one. */
async function findOwn(): Promise<BusinessSettings | null> {
  const owner = pb.authStore.record?.id ?? ''

  try {
    return await pb
      .collection(COLLECTION)
      .getFirstListItem<BusinessSettings>(
        // The list rule already scopes this to the caller; naming the owner
        // as well keeps the query honest about what it expects back, and is
        // what makes a duplicate row impossible to reach rather than merely
        // unlikely — the collection's unique index is the other half.
        pb.filter('user_id = {:userId}', { userId: owner }),
      )
  } catch (error) {
    if (isNotFound(error)) {
      return null
    }
    throw error
  }
}

/**
 * The signed-in person's settings, falling back to DEFAULT_SETTINGS when they
 * have never saved any.
 *
 * Only "nothing saved yet" produces the default. Any other failure is thrown:
 * showing the default after, say, an expired session would present somebody
 * else's configuration back to them as their own, and the next save would
 * overwrite the real row with it.
 */
export async function loadBusinessSettings(): Promise<BusinessSettings> {
  return (await findOwn()) ?? DEFAULT_SETTINGS
}

/**
 * Writes the settings, creating the row on the first save and editing it
 * after.
 *
 * The row is looked up rather than trusted from `settings.id`, so a second tab
 * that created one in the meantime is edited rather than collided with. If two
 * saves still race, the collection's unique index refuses the second create
 * loudly instead of leaving two rows for one person.
 */
export async function saveBusinessSettings(
  settings: BusinessSettings,
): Promise<BusinessSettings> {
  const {
    timing_minutes,
    is_active,
    template_name,
    template_language,
    timezone,
    ai_mode,
  } = settings

  // user_id is deliberately absent: ownership is not something an edit may
  // change, and the update rule refuses the attempt regardless.
  const body = {
    timing_minutes,
    is_active,
    template_name,
    template_language,
    timezone,
    ai_mode,
  }

  const existing = await findOwn()
  if (existing?.id) {
    return pb.collection(COLLECTION).update<BusinessSettings>(existing.id, body)
  }

  const owner = pb.authStore.record?.id
  if (!owner) {
    throw new Error('Tu sesión expiró. Inicia sesión otra vez para guardar esto.')
  }

  return pb.collection(COLLECTION).create<BusinessSettings>({
    ...body,
    user_id: owner,
  })
}
