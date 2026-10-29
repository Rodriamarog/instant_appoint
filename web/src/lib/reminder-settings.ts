import { pb } from '@/lib/pocketbase'

/**
 * How a person's reminders are configured: how far ahead they go out, whether
 * they go out at all, and the approved template and zone they are written in.
 *
 * This module is the only place that knows the collection is called
 * reminder_settings, that there is exactly one row per person, and what to
 * show somebody who has never opened the settings dialog.
 *
 * There is no server-side handler behind it: the SPA reaches PocketBase
 * directly and the collection's rules are the whole of the access control.
 * That is why a create stamps user_id from the session rather than taking it
 * as an argument — the create rule compares the submitted user_id against the
 * caller and runs before any hook, so a value from anywhere else is a 400.
 */
export const COLLECTION = 'reminder_settings'

/** One person's reminder configuration. */
export interface ReminderSettings {
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
}

/**
 * What somebody sees before they have saved anything.
 *
 * The same values are declared in migrations/1793232003_reminder_settings.go,
 * where the language and the zone are create-time field defaults, so a row
 * written by anything that skips them matches what the page would have shown.
 */
export const DEFAULT_SETTINGS: ReminderSettings = {
  timing_minutes: 1440,
  is_active: true,
  template_name: '',
  template_language: 'es',
  timezone: 'America/Tijuana',
}

/** The lead times the dialog offers, longest-used first in the old UI order. */
export const TIMING_OPTIONS = [
  { label: '30 min', minutes: 30 },
  { label: '1 hour', minutes: 60 },
  { label: '2 hours', minutes: 120 },
  { label: '12 hours', minutes: 720 },
  { label: '24 hours', minutes: 1440 },
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
async function findOwn(): Promise<ReminderSettings | null> {
  const owner = pb.authStore.record?.id ?? ''

  try {
    return await pb
      .collection(COLLECTION)
      .getFirstListItem<ReminderSettings>(
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
export async function loadReminderSettings(): Promise<ReminderSettings> {
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
export async function saveReminderSettings(
  settings: ReminderSettings,
): Promise<ReminderSettings> {
  const { timing_minutes, is_active, template_name, template_language, timezone } =
    settings

  // user_id is deliberately absent: ownership is not something an edit may
  // change, and the update rule refuses the attempt regardless.
  const body = {
    timing_minutes,
    is_active,
    template_name,
    template_language,
    timezone,
  }

  const existing = await findOwn()
  if (existing?.id) {
    return pb.collection(COLLECTION).update<ReminderSettings>(existing.id, body)
  }

  const owner = pb.authStore.record?.id
  if (!owner) {
    throw new Error('Your session has expired. Sign in again to save this.')
  }

  return pb.collection(COLLECTION).create<ReminderSettings>({
    ...body,
    user_id: owner,
  })
}
