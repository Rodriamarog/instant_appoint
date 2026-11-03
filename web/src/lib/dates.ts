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

/**
 * The locale the zone arithmetic below formats under.
 *
 * None of it reaches the screen: the pieces are read back by name and
 * respelled from the tables above. It is pinned rather than left undefined
 * because an undefined locale is the viewer's, and a viewer's locale can
 * bring its own calendar — under ar-SA the year of a Gregorian instant
 * comes back as 1448.
 */
const ZONE_LOCALE = 'en-US'

/**
 * One formatter per zone, kept because constructing an Intl.DateTimeFormat
 * is the expensive part and a day's list asks for the same zone on every
 * row.
 */
const ZONE_FORMATTERS = new Map<string, Intl.DateTimeFormat>()

function formatterFor(timeZone: string): Intl.DateTimeFormat {
  const cached = ZONE_FORMATTERS.get(timeZone)
  if (cached) {
    return cached
  }

  let formatter: Intl.DateTimeFormat
  try {
    formatter = new Intl.DateTimeFormat(ZONE_LOCALE, {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      // h23 rather than hour12: false, which leaves midnight as "24" on some
      // engines — an appointment at the start of a day printed as the end of
      // one, and sorted there too.
      hourCycle: 'h23',
    })
  } catch {
    // An unreadable zone name throws at construction, and it arrives here
    // out of a stored row. UTC is wrong by some hours; a page that cannot
    // render at all is wrong by the whole day.
    formatter = formatterFor('UTC')
  }

  ZONE_FORMATTERS.set(timeZone, formatter)
  return formatter
}

/** The year, month, day, hour and minute of an instant, read in `timeZone`. */
function partsIn(instant: Date, timeZone: string): Record<string, string> {
  const parts: Record<string, string> = {}
  for (const { type, value } of formatterFor(timeZone).formatToParts(instant)) {
    parts[type] = value
  }

  return parts
}

/**
 * Which calendar day an instant falls on *in the clinic's zone*, as
 * YYYY-MM-DD.
 *
 * This is the one piece of date handling that may not use the browser's own
 * zone, and it is why Intl appears in a file whose header warns against
 * toLocaleDateString. The difference is the explicit timeZone: the viewer is
 * never consulted, so a receptionist checking the screen at 23:00 and a
 * dentist reading it from another country are told about the same day. The
 * string shape is chosen so two of them compare and sort as text.
 */
export function zonedDay(instant: Date, timeZone: string): string {
  const { year, month, day } = partsIn(instant, timeZone)

  return `${year}-${month}-${day}`
}

/** The time of day an instant falls at on the clinic's wall clock. */
export function zonedTime(instant: Date, timeZone: string): string {
  const { hour, minute } = partsIn(instant, timeZone)

  return `${hour}:${minute}`
}

/**
 * A YYYY-MM-DD day as a heading reads it: "martes 3 de noviembre de 2026".
 *
 * The weekday is worked out in UTC on purpose. The argument is already a
 * civil date with no zone left in it, and UTC is the only zone with no DST
 * jump that could land midnight on the day before.
 */
export function spellDay(day: string): string {
  const [year, month, date] = day.split('-').map(Number)
  const weekday = WEEKDAYS[new Date(Date.UTC(year, month - 1, date)).getUTCDay()]

  return `${weekday} ${date} de ${MONTHS[month - 1]} de ${year}`
}
