import {
  statusLabel,
  statusTone,
  type Appointment,
  type AppointmentStatus,
} from '@/lib/appointments'
import { spellDay, zonedDay, zonedTime } from '@/lib/dates'
import { cn } from '@/lib/utils'

interface DayScheduleProps {
  /** Everything the page has loaded; the day is picked out of it here. */
  appointments: Appointment[]
  /** The clinic's IANA zone, out of its business settings. */
  timeZone: string
  /** Now. A prop so a test can stand anywhere in the day. */
  now: Date
  onSelect?: (appointment: Appointment) => void
}

/**
 * The three totals across the top, in the order a morning is read: what is
 * settled, what still needs chasing, what has fallen away.
 *
 * Proposed is deliberately not among them. It is the assistant saying it is
 * unsure, not a fourth kind of appointment, and a count of uncertainty
 * beside three counts of fact invites it to be read as one.
 */
const COUNTED: { status: AppointmentStatus; label: string }[] = [
  { status: 'confirmed', label: 'Confirmadas' },
  { status: 'pending', label: 'Pendientes' },
  { status: 'cancelled', label: 'Canceladas' },
]

/**
 * Today's appointments, in time order, each saying whether the patient has
 * confirmed.
 *
 * A list rather than the time grid, because the question this screen is
 * opened to answer is who has not confirmed, and that is read down a column
 * of words far faster than it is picked out of a wall of blocks.
 */
export function DaySchedule({
  appointments,
  timeZone,
  now,
  onSelect,
}: DayScheduleProps) {
  // The clinic's day, never the viewer's: a receptionist looking at this at
  // 23:00 is still working today, and a dentist reading it from abroad is
  // not entitled to a different list.
  const today = zonedDay(now, timeZone)

  const rows = appointments
    .filter((row) => zonedDay(new Date(row.start_time), timeZone) === today)
    // Sorted here rather than trusted from the caller. The stored timestamps
    // are fixed-width UTC text, so comparing them as strings is comparing
    // them as instants, whatever zone they are shown in.
    .sort((a, b) => a.start_time.localeCompare(b.start_time))

  return (
    <section className="space-y-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="font-heading text-xl font-semibold first-letter:uppercase">
          {spellDay(today)}
        </h2>
      </div>

      <dl className="grid grid-cols-3 gap-3">
        {COUNTED.map(({ status, label }) => (
          <div
            key={status}
            data-testid={`count-${status}`}
            className="rounded-lg border px-4 py-3"
          >
            <dd className="text-2xl font-semibold tabular-nums">
              {rows.filter((row) => row.status === status).length}
            </dd>
            <dt className="text-sm text-muted-foreground">{label}</dt>
          </div>
        ))}
      </dl>

      {rows.length === 0 ? (
        // Said out loud, because a blank panel and a load that quietly
        // failed look the same, and one of the two means go home.
        <p className="rounded-lg border border-dashed px-4 py-10 text-center text-sm text-muted-foreground">
          No hay citas para este día.
        </p>
      ) : (
        <ul className="divide-y rounded-lg border">
          {rows.map((row) => (
            <li
              key={row.id}
              className={cn('flex items-center', statusTone(row.status))}
            >
              <button
                type="button"
                onClick={() => onSelect?.(row)}
                className="flex w-full items-center gap-4 px-4 py-3 text-left transition-colors hover:bg-muted/50"
              >
                <span className="w-14 shrink-0 font-mono text-sm tabular-nums">
                  {zonedTime(new Date(row.start_time), timeZone)}
                </span>
                <span data-testid="client" className="flex-1 truncate">
                  {row.client_name}
                </span>
                <span className="shrink-0 text-sm text-muted-foreground">
                  {statusLabel(row.status)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
