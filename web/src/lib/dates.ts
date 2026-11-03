/**
 * The calendar's vocabulary, and the two ways this interface writes an
 * instant down.
 *
 * The words are a table rather than a locale setting for the same reason
 * internal/reminders keeps its own: a dependency and a runtime locale are a
 * large thing to carry for twenty-four words, and the viewer's own locale —
 * which is what toLocaleDateString reads — is not the clinic's. A browser
 * left on en-US was printing "Mon, Nov 3 at 3:30 PM" on a Spanish screen.
 *
 * The month is a word in every form below, and that is the point. This is a
 * border clinic: "03/11" is 3 November to the staff and 11 March to half the
 * people they treat, same digits, four months apart. The time is 24-hour for
 * the matching reason — "3:30" is twelve hours wide, "15:30" is not.
 */

/** Month names, indexed as Date#getMonth returns them. */
export const MONTHS = [
  'enero',
  'febrero',
  'marzo',
  'abril',
  'mayo',
  'junio',
  'julio',
  'agosto',
  'septiembre',
  'octubre',
  'noviembre',
  'diciembre',
]

/** The same twelve, short enough for a list row or a column header. */
export const MONTHS_SHORT = [
  'ene',
  'feb',
  'mar',
  'abr',
  'may',
  'jun',
  'jul',
  'ago',
  'sep',
  'oct',
  'nov',
  'dic',
]

/** Weekday names, indexed as Date#getDay returns them: Sunday first. */
export const WEEKDAYS = [
  'domingo',
  'lunes',
  'martes',
  'miércoles',
  'jueves',
  'viernes',
  'sábado',
]

export const WEEKDAYS_SHORT = ['dom', 'lun', 'mar', 'mié', 'jue', 'vie', 'sáb']

export const WEEKDAYS_MIN = ['do', 'lu', 'ma', 'mi', 'ju', 'vi', 'sá']

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

/** The time of day, on a 24-hour clock. */
export function spellTime(instant: Date): string {
  return `${pad(instant.getHours())}:${pad(instant.getMinutes())}`
}

/**
 * A whole instant, as a row in a list says it: "mar 3 nov 2026, 15:30".
 *
 * The year is always there. A sent reminder stays in the list after the turn
 * of the year, and a date with no year is one more thing a reader has to
 * assume.
 */
export function spellDateTime(instant: Date): string {
  const weekday = WEEKDAYS_SHORT[instant.getDay()]
  const month = MONTHS_SHORT[instant.getMonth()]

  return `${weekday} ${instant.getDate()} ${month} ${instant.getFullYear()}, ${spellTime(instant)}`
}
