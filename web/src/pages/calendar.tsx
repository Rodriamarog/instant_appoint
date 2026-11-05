import moment from 'moment'
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Calendar,
  momentLocalizer,
  type ToolbarProps,
  type View,
} from 'react-big-calendar'
import 'react-big-calendar/lib/css/react-big-calendar.css'

import { DaySchedule } from '@/components/calendar/day-schedule'
import { EventDialog } from '@/components/calendar/event-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  DEFAULT_REMINDER_LANGUAGE,
  listAppointmentsBetween,
  statusLabel,
  statusTone,
  type Appointment,
} from '@/lib/appointments'
import {
  DEFAULT_SETTINGS,
  loadBusinessSettings,
} from '@/lib/business-settings'
import {
  MONTHS,
  MONTHS_SHORT,
  WEEKDAYS,
  WEEKDAYS_MIN,
  WEEKDAYS_SHORT,
} from '@/lib/dates'
import { errorMessage } from '@/lib/errors'
import { isoMillis } from '@/lib/iso-time'
import { cn } from '@/lib/utils'

/**
 * The grid writes its own dates, and it writes them in whatever language
 * moment is set to.
 *
 * Hence a locale defined here out of the app's own word tables rather than an
 * import of moment/locale/es: the words are already written down once for the
 * rest of the interface, and the alternative is two lists of month names that
 * can disagree. LT is what react-big-calendar formats every clock face with,
 * so pinning it to HH:mm is what keeps the time gutter off a 12-hour clock
 * the rest of this app does not use.
 */
const LOCALE = 'es-instantappoint'

moment.updateLocale(LOCALE, {
  months: MONTHS,
  monthsShort: MONTHS_SHORT,
  weekdays: WEEKDAYS,
  weekdaysShort: WEEKDAYS_SHORT,
  weekdaysMin: WEEKDAYS_MIN,
  // Monday, as a Spanish-language calendar is read.
  week: { dow: 1, doy: 4 },
  longDateFormat: {
    LT: 'HH:mm',
    LTS: 'HH:mm:ss',
    L: 'DD/MM/YYYY',
    LL: 'D [de] MMMM [de] YYYY',
    LLL: 'D [de] MMMM [de] YYYY HH:mm',
    LLLL: 'dddd, D [de] MMMM [de] YYYY HH:mm',
  },
})

// Stated rather than left to updateLocale's own side effect, which is what
// currently selects it: the grid reads the global locale at render time, and
// nothing else in this app sets one.
moment.locale(LOCALE)

/**
 * The toolbar's own words. These are the library's, not the dates': no locale
 * reaches them, so an untranslated calendar keeps an English Today button
 * above perfectly Spanish dates.
 */
const MESSAGES = {
  date: 'Fecha',
  time: 'Hora',
  event: 'Cita',
  allDay: 'Todo el día',
  week: 'Semana',
  work_week: 'Semana laboral',
  day: 'Día',
  month: 'Mes',
  previous: 'Anterior',
  next: 'Siguiente',
  yesterday: 'Ayer',
  tomorrow: 'Mañana',
  today: 'Hoy',
  agenda: 'Agenda',
  noEventsInRange: 'No hay citas en estas fechas.',
  showMore: (total: number) => `+${total} más`,
}

const localizer = momentLocalizer(moment)

/**
 * What react-big-calendar wants: two Dates, plus the row behind them.
 *
 * title is the library's own accessor name rather than this app's â€” it is
 * what the grid prints on a slot. Renaming it to client_name along with the
 * column would leave every appointment in the month view blank.
 */
interface SlotEvent {
  id: string
  title: string
  start: Date
  end: Date
  resource: Appointment
}

/**
 * A month view shows days either side of the month itself in its first and
 * last rows, and an appointment in one of those still has to appear. A week
 * of slack covers every view's overhang.
 */
const WINDOW_SLACK_DAYS = 7

/**
 * The two states that are not a booking somebody is expected at: a proposal
 * the assistant is unsure about, and an appointment that is not happening.
 *
 * They are the two the grid has to call out, and they are the same two no
 * reminder goes out for â€” internal/appointments decides that, and a slot
 * that looked ordinary while being silently skipped is the mismatch this is
 * here to stop.
 */
function labelFor(status: Appointment['status']): string | null {
  return status === 'proposed' || status === 'cancelled' ? statusLabel(status) : null
}

/**
 * What a slot prints: the client's name, and the word for its state when it
 * has one worth reading at a glance.
 */
function SlotLabel({ event }: { event: SlotEvent }) {
  const label = labelFor(event.resource.status)

  return (
    <span className="flex items-center gap-1">
      <span className="truncate">{event.title}</span>
      {label && (
        <span className="shrink-0 rounded-full bg-black/20 px-1.5 text-[10px] font-medium">
          {label}
        </span>
      )}
    </span>
  )
}

/** Dimmed, and struck through once it is not happening at all. */
function slotStyle(event: SlotEvent): { className: string } {
  return { className: statusTone(event.resource.status) }
}

function windowFor(date: Date): { from: Date; to: Date } {
  return {
    from: moment(date).startOf('month').subtract(WINDOW_SLACK_DAYS, 'days').toDate(),
    to: moment(date).endOf('month').add(WINDOW_SLACK_DAYS, 'days').toDate(),
  }
}

/**
 * The three things this page can be: the day being worked through, and the
 * two grids that answer where there is room.
 */
type Pane = 'day' | 'week' | 'month'

/**
 * Where the choice is kept.
 *
 * It is remembered because it is a habit rather than a navigation. A clinic
 * that plans from the month view would otherwise be handed the day's list
 * again on every refresh, and the surest way to stop somebody using a
 * control is to undo their last use of it.
 */
export const VIEW_STORAGE_KEY = 'inicio:vista'

const PANES: { pane: Pane; label: string }[] = [
  { pane: 'day', label: 'Hoy' },
  { pane: 'week', label: 'Semana' },
  { pane: 'month', label: 'Mes' },
]

/**
 * The remembered choice, defaulting to the day.
 *
 * Checked against the list rather than cast, so a key left behind by an
 * older build â€” or edited by hand â€” opens the morning screen rather than
 * putting the grid into a view it has no renderer for.
 */
function storedPane(): Pane {
  const stored = localStorage.getItem(VIEW_STORAGE_KEY)

  return PANES.some(({ pane }) => pane === stored) ? (stored as Pane) : 'day'
}

/**
 * The grid's navigation, without the view switcher it ships with.
 *
 * react-big-calendar's own toolbar offers Mes, Semana and Día. Left in
 * place it would be a second control for the choice the segmented control
 * above already makes, and its Día button would hand back the day grid this
 * page deliberately replaced with a list. Its today button is named for the
 * span it jumps to so that it does not read as a second Hoy.
 */
function GridToolbar({ label, view, onNavigate }: ToolbarProps<SlotEvent, object>) {
  return (
    <div className="rbc-toolbar">
      <span className="rbc-btn-group">
        <button type="button" onClick={() => onNavigate('PREV')}>
          Anterior
        </button>
        <button type="button" onClick={() => onNavigate('TODAY')}>
          {view === 'week' ? 'Esta semana' : 'Este mes'}
        </button>
        <button type="button" onClick={() => onNavigate('NEXT')}>
          Siguiente
        </button>
      </span>
      <span className="rbc-toolbar-label">{label}</span>
    </div>
  )
}

export function CalendarPage() {
  const [rows, setRows] = useState<Appointment[]>([])
  const [date, setDate] = useState(() => new Date())
  const [pane, setPane] = useState<Pane>(storedPane)
  const [selected, setSelected] = useState<Appointment | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create')
  const [error, setError] = useState<string | null>(null)

  /**
   * The zone the clinic keeps, which is what decides where its day begins
   * and ends. Until the settings row arrives the documented default stands
   * in â€” the same value a clinic that has never opened the settings page is
   * already running under.
   */
  const [timeZone, setTimeZone] = useState(DEFAULT_SETTINGS.timezone)

  // Captured once, so a re-render partway through the evening cannot slide
  // the list onto a different day under somebody reading it.
  const [now] = useState(() => new Date())

  useEffect(() => {
    let abandoned = false

    void loadBusinessSettings()
      .then((settings) => {
        if (!abandoned && settings.timezone) {
          setTimeZone(settings.timezone)
        }
      })
      // The zone decides which hours are printed, not whether a day can be
      // shown at all. A settings row that will not load is a reason to fall
      // back to the default, not to leave a clinic with a blank morning.
      .catch(() => {})

    return () => {
      abandoned = true
    }
  }, [])

  // The window is keyed off the month rather than the exact day, so moving
  // between days or weeks inside one month does not refetch what is already
  // on screen. It is measured in the browser's own zone, which is safe here
  // and nowhere else on this page: a week of slack either side swallows any
  // offset between the viewer and the clinic many times over.
  const monthKey = moment(date).format('YYYY-MM')

  const load = useCallback(async () => {
    const { from, to } = windowFor(moment(monthKey, 'YYYY-MM').toDate())

    try {
      setRows(await listAppointmentsBetween(from, to))
      setError(null)
    } catch (err) {
      setError(
        errorMessage(err, 'No se pudieron cargar tus citas.'),
      )
    }
  }, [monthKey])

  useEffect(() => {
    void load()
  }, [load])

  const events = useMemo<SlotEvent[]>(
    () =>
      rows.map((row) => ({
        id: row.id,
        title: row.client_name,
        start: new Date(row.start_time),
        end: new Date(row.end_time),
        resource: row,
      })),
    [rows],
  )

  const views = useMemo<View[]>(() => ['week', 'month'], [])
  const slotComponents = useMemo(
    () => ({ event: SlotLabel, toolbar: GridToolbar }),
    [],
  )

  const choosePane = (next: Pane) => {
    setPane(next)
    localStorage.setItem(VIEW_STORAGE_KEY, next)
  }

  const openForEditing = (appointment: Appointment) => {
    setSelected(appointment)
    setDialogMode('edit')
    setDialogOpen(true)
  }

  return (
    <div className="space-y-4">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div
        role="group"
        aria-label="Vista"
        className="inline-flex rounded-[10px] bg-muted p-1 ring-1 ring-border"
      >
        {PANES.map(({ pane: option, label }) => (
          <button
            key={option}
            type="button"
            aria-pressed={pane === option}
            onClick={() => choosePane(option)}
            className={cn(
              'cursor-pointer rounded-[7px] px-3.5 py-1.5 text-sm font-medium transition-colors',
              pane === option
                ? 'bg-background text-foreground shadow-[0_1px_2px_rgba(11,18,51,0.08),0_0_0_1px_var(--border)]'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {pane === 'day' ? (
        <DaySchedule
          appointments={rows}
          timeZone={timeZone}
          now={now}
          onSelect={openForEditing}
        />
      ) : (
        <div style={{ height: 'calc(100vh - 260px)', minHeight: '32rem' }}>
          <Calendar
            localizer={localizer}
            messages={MESSAGES}
            culture={LOCALE}
            events={events}
            startAccessor="start"
            endAccessor="end"
            view={pane}
            views={views}
            onView={(next) => choosePane(next as Pane)}
            components={slotComponents}
            eventPropGetter={slotStyle}
            date={date}
            onNavigate={setDate}
            onSelectSlot={(slot) => {
              // A blank appointment covering whatever was dragged out. The
              // dialog fills in the rest.
              setSelected({
                id: '',
                user_id: '',
                client_name: '',
                // What saving it will produce. Nothing is stored yet, so the
                // dialog shows no status for it either.
                status: 'pending',
                // Likewise a placeholder and not a decision: the dialog
                // re-guesses it from the dialling prefix until somebody
                // chooses one.
                language: DEFAULT_REMINDER_LANGUAGE,
                // Through isoMillis like every other writer, not
                // toISOString directly. The two agree today; the point of the
                // module is that no call site decides that for itself, because
                // the whole system sorts these as text.
                start_time: isoMillis(slot.start),
                end_time: isoMillis(slot.end),
              })
              setDialogMode('create')
              setDialogOpen(true)
            }}
            onSelectEvent={(event) => openForEditing(event.resource)}
            selectable
            popup
            step={30}
            showMultiDayTimes
            style={{ height: '100%' }}
          />
        </div>
      )}

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
