/**
 * The browser half of the stored-timestamp contract. internal/isotime is the
 * Go half, and the two must agree byte for byte.
 *
 * Every application timestamp lives in a PocketBase *text* column, so the
 * ordering and the range filters the calendar runs are byte-by-byte string
 * comparisons made by SQLite. They only mean "chronological" while every
 * writer emits the same fixed-width shape:
 *
 *     YYYY-MM-DDTHH:mm:ss.sssZ
 *
 * which is exactly what Date.prototype.toISOString() is specified to produce:
 * 24 characters, always UTC, always a literal Z, always three fractional
 * digits. This module exists so no call site reaches for a formatter that
 * nearly matches — a trimmed fraction or a +00:00 offset sorts wrongly and
 * nothing complains until the wrong appointments come back.
 */

/** The stored timestamp for an instant. */
export function isoMillis(instant: Date): string {
  const millis = instant.getTime()
  if (!Number.isFinite(millis)) {
    throw new Error('cannot format an invalid Date as a timestamp')
  }

  return instant.toISOString()
}

/** The stored timestamp for right now. */
export function isoMillisNow(): string {
  return isoMillis(new Date())
}

/**
 * Turns the pair of values a <input type="date"> and <input type="time">
 * hold into one stored timestamp.
 *
 * `new Date('2026-11-02T09:30')` — a date-time with no offset — is parsed as
 * *local* time, which is what the person typing it meant. The stored value is
 * the UTC instant that names.
 */
export function localInputsToIso(date: string, time: string): string {
  const instant = new Date(`${date}T${time}`)
  if (Number.isNaN(instant.getTime())) {
    throw new Error(`"${date} ${time}" is not a date and time`)
  }

  return isoMillis(instant)
}

/** The <input type="date"> value for an instant, in the viewer's own zone. */
export function localDateInput(instant: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0')

  return `${instant.getFullYear()}-${pad(instant.getMonth() + 1)}-${pad(instant.getDate())}`
}

/** The <input type="time"> value for an instant, in the viewer's own zone. */
export function localTimeInput(instant: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0')

  return `${pad(instant.getHours())}:${pad(instant.getMinutes())}`
}
