import moment from 'moment'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Calendar, momentLocalizer, type View } from 'react-big-calendar'
import 'react-big-calendar/lib/css/react-big-calendar.css'

import { EventDialog } from '@/components/calendar/event-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  listAppointmentsBetween,
  type CalendarEvent,
} from '@/lib/calendar-events'

const localizer = momentLocalizer(moment)

/** What react-big-calendar wants: two Dates, plus the row behind them. */
interface SlotEvent {
  id: string
  title: string
  start: Date
  end: Date
  resource: CalendarEvent
}

/**
 * A month view shows days either side of the month itself in its first and
 * last rows, and an appointment in one of those still has to appear. A week
 * of slack covers every view's overhang.
 */
const WINDOW_SLACK_DAYS = 7

function windowFor(date: Date): { from: Date; to: Date } {
  return {
    from: moment(date).startOf('month').subtract(WINDOW_SLACK_DAYS, 'days').toDate(),
    to: moment(date).endOf('month').add(WINDOW_SLACK_DAYS, 'days').toDate(),
  }
}

export function CalendarPage() {
  const [appointments, setAppointments] = useState<SlotEvent[]>([])
  const [date, setDate] = useState(() => new Date())
  const [view, setView] = useState<View>('month')
  const [selected, setSelected] = useState<CalendarEvent | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create')
  const [error, setError] = useState<string | null>(null)

  // The window is keyed off the month rather than the exact day, so moving
  // between days or weeks inside one month does not refetch what is already
  // on screen.
  const monthKey = moment(date).format('YYYY-MM')

  const load = useCallback(async () => {
    const { from, to } = windowFor(moment(monthKey, 'YYYY-MM').toDate())

    try {
      const rows = await listAppointmentsBetween(from, to)
      setAppointments(
        rows.map((row) => ({
          id: row.id,
          title: row.title,
          start: new Date(row.start_time),
          end: new Date(row.end_time),
          resource: row,
        })),
      )
      setError(null)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Could not load your appointments.',
      )
    }
  }, [monthKey])

  useEffect(() => {
    void load()
  }, [load])

  const views = useMemo<View[]>(() => ['month', 'week', 'day'], [])

  return (
    <div className="space-y-4">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div style={{ height: 'calc(100vh - 220px)', minHeight: '32rem' }}>
        <Calendar
          localizer={localizer}
          events={appointments}
          startAccessor="start"
          endAccessor="end"
          view={view}
          views={views}
          onView={setView}
          date={date}
          onNavigate={setDate}
          onSelectSlot={(slot) => {
            // A blank appointment covering whatever was dragged out. The
            // dialog fills in the rest.
            setSelected({
              id: '',
              user_id: '',
              title: '',
              start_time: slot.start.toISOString(),
              end_time: slot.end.toISOString(),
            })
            setDialogMode('create')
            setDialogOpen(true)
          }}
          onSelectEvent={(event) => {
            setSelected(event.resource)
            setDialogMode('edit')
            setDialogOpen(true)
          }}
          selectable
          popup
          step={30}
          showMultiDayTimes
          style={{ height: '100%' }}
        />
      </div>

      <EventDialog
        open={dialogOpen}
        mode={dialogMode}
        event={selected}
        onClose={() => setDialogOpen(false)}
        onSaved={() => {
          setDialogOpen(false)
          void load()
        }}
      />
    </div>
  )
}
