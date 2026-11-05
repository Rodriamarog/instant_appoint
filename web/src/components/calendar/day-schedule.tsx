import {
  statusLabel,
  statusTone,
  type Appointment,
  type AppointmentStatus,
} from '@/lib/appointments'
import { StatusPill, type PillTone } from '@/components/status-pill'
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
  /**
   * Controls that belong on the heading's line — the page's view switcher —
   * so they share the date's row instead of spending one of their own.
   */
  actions?: React.ReactNode
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

/** Each status as a pill, in the landing page's day view's two colours. */
const TONE: Record<AppointmentStatus, PillTone> = {
  confirmed: 'settled',
  pending: 'waiting',
  proposed: 'unsure',
  cancelled: 'off',
}

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
  actions,
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
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-heading text-2xl font-bold tracking-[-0.03em] first-letter:uppercase">
          {spellDay(today)}
        </h2>
        {actions}
      </div>

      <dl className="grid grid-cols-3 gap-3">
        {COUNTED.map(({ status, label }) => (
          <div
            key={status}
            data-testid={`count-${status}`}
            className={cn(
              'flex flex-col-reverse rounded-[14px] px-4 py-3.5 ring-1 ring-border',
              status === 'confirmed' ? 'bg-accent ring-brand/15' : 'bg-background',
            )}
          >
            <dt className="text-sm text-muted-foreground">{label}</dt>
            <dd
              className={cn(
                'font-heading text-3xl font-bold tracking-tight tabular-nums',
                status === 'confirmed' && 'text-brand',
              )}
            >
              {rows.filter((row) => row.status === status).length}
            </dd>
          </div>
        ))}
      </dl>

      {rows.length === 0 ? (
        // Said out loud, because a blank panel and a load that quietly
        // failed look the same, and one of the two means go home.
        <p className="rounded-[14px] border border-dashed border-[#cfd6e6] dark:border-[#3a4670] bg-muted/50 px-4 py-12 text-center text-sm text-muted-foreground">
          No hay citas para este día.
        </p>
      ) : (
        <ul className="divide-y overflow-hidden rounded-[14px] bg-background ring-1 ring-border">
          {rows.map((row) => (
            <li
              key={row.id}
              className={cn('flex items-center', statusTone(row.status))}
            >
              <button
                type="button"
                onClick={() => onSelect?.(row)}
                className="flex w-full cursor-pointer items-center gap-4 px-4 py-3.5 text-left transition-colors hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
              >
                <span className="w-14 shrink-0 text-sm font-medium text-muted-foreground tabular-nums">
                  {zonedTime(new Date(row.start_time), timeZone)}
                </span>
                <span
                  data-testid="client"
                  className="flex-1 truncate text-[15px] font-semibold"
                >
                  {row.client_name}
                </span>
                <StatusPill tone={TONE[row.status]}>
                  {statusLabel(row.status)}
                </StatusPill>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
