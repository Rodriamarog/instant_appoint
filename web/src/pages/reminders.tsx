import {
  Bell,
  BellOff,
  Calendar,
  Clock,
  Loader2,
  MessageSquare,
  Phone,
  Settings,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { ReminderSettingsDialog } from '@/components/reminders/settings-dialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  DEFAULT_SETTINGS,
  loadReminderSettings,
  saveReminderSettings,
  type ReminderSettings,
} from '@/lib/reminder-settings'
import {
  listSentReminders,
  listUpcomingReminders,
  type SentReminder,
  type UpcomingReminder,
} from '@/lib/reminders'
import { isWhatsAppConnected } from '@/lib/whatsapp-connection'

/** A stored instant, in the viewer's own zone and words. */
function formatDateTime(instant: Date) {
  return (
    instant.toLocaleDateString(undefined, {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
    }) +
    ' at ' +
    instant.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  )
}

function Pill({ tone, children }: { tone: 'due' | 'scheduled' | 'sent'; children: string }) {
  const tones = {
    due: 'bg-amber-100 text-amber-800 ring-amber-200',
    scheduled: 'text-muted-foreground ring-border',
    sent: 'bg-green-100 text-green-800 ring-green-200',
  }

  return (
    <span
      className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ${tones[tone]}`}
    >
      {children}
    </span>
  )
}

function Count({ children }: { children: number }) {
  return (
    <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium">{children}</span>
  )
}

function Empty({ icon: Icon, title, hint }: { icon: typeof Bell; title: string; hint: string }) {
  return (
    <div className="flex flex-col items-center justify-center space-y-2 py-10 text-center">
      <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-muted">
        <Icon className="h-5 w-5 text-muted-foreground" />
      </div>
      <p className="text-sm font-medium">{title}</p>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  )
}

/**
 * The reminders page: what goes out and when, what is coming, what has gone.
 *
 * Nothing here is a stored "reminder" until it has been sent — the upcoming
 * column is derived from the appointments themselves, which is why changing
 * the lead time changes it immediately. The scheduler that turns one into a
 * message is #9, so the sent column stays empty until then.
 */
export function RemindersPage() {
  const [settings, setSettings] = useState<ReminderSettings | null>(null)
  const [upcoming, setUpcoming] = useState<UpcomingReminder[] | null>(null)
  const [sent, setSent] = useState<SentReminder[]>([])
  // null while the answer is still unknown: an amber warning shown during a
  // round trip is the bug this page is here to stop repeating.
  const [connected, setConnected] = useState<boolean | null>(null)
  const [error, setError] = useState('')

  const [dialogOpen, setDialogOpen] = useState(false)

  useEffect(() => {
    loadReminderSettings()
      .then(setSettings)
      .catch(() => {
        // The page is still usable: the defaults are what an unsaved account
        // would have shown anyway. Saving from here would overwrite a row
        // that may exist, so say so rather than pretend.
        setSettings(DEFAULT_SETTINGS)
        setError('Could not load your reminder settings.')
      })
  }, [])

  useEffect(() => {
    isWhatsAppConnected()
      .then(setConnected)
      .catch(() => setConnected(false))
  }, [])

  useEffect(() => {
    listSentReminders()
      .then(setSent)
      .catch(() => setSent([]))
  }, [])

  const leadTime = settings?.timing_minutes
  useEffect(() => {
    if (leadTime === undefined) {
      return
    }

    let current = true
    listUpcomingReminders(leadTime)
      .then((rows) => {
        if (current) {
          setUpcoming(rows)
        }
      })
      .catch(() => {
        if (current) {
          setUpcoming([])
          setError('Could not load your upcoming reminders.')
        }
      })

    return () => {
      current = false
    }
  }, [leadTime])

  const save = useCallback(async (next: ReminderSettings) => {
    setSettings(await saveReminderSettings(next))
    setError('')
  }, [])

  if (!settings) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <section className="space-y-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="font-heading text-2xl font-semibold">Reminders</h1>
          <p className="text-muted-foreground">
            A WhatsApp reminder goes out{' '}
            {settings.is_active
              ? 'before every appointment that has a phone number.'
              : 'before every appointment — once you switch reminders back on.'}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          aria-label="Reminder settings"
          className="mt-1 rounded-lg p-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <Settings className="h-5 w-5" />
        </button>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {connected === false && (
        <div className="flex items-start gap-3 rounded-lg bg-amber-50 p-4 ring-1 ring-amber-200">
          <BellOff className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" />
          <p className="text-sm text-amber-800">
            <strong>WhatsApp not connected.</strong> Reminders will not send until you
            connect a WhatsApp Business number on the WhatsApp page.
          </p>
        </div>
      )}

      {!settings.is_active && (
        <div className="flex items-start gap-3 rounded-lg bg-muted p-4 ring-1 ring-border">
          <BellOff className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
          <p className="text-sm">
            <strong>Reminders are switched off.</strong> Nothing below will be sent.
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Upcoming</CardTitle>
            {upcoming !== null && <Count>{upcoming.length}</Count>}
          </CardHeader>
          <CardContent>
            {upcoming === null ? (
              <div className="flex items-center justify-center py-10">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : upcoming.length === 0 ? (
              <Empty
                icon={Bell}
                title="No upcoming reminders"
                hint="Appointments with a phone number will appear here."
              />
            ) : (
              <ul className="space-y-2">
                {upcoming.map(({ event, reminderAt, isDue }) => (
                  <li
                    key={event.id}
                    className="flex items-start gap-3 rounded-lg p-3 ring-1 ring-border"
                  >
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-muted">
                      <span className="text-xs font-bold">
                        {event.title.charAt(0).toUpperCase()}
                      </span>
                    </div>
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <p className="truncate text-sm font-medium">{event.title}</p>
                        {isDue ? (
                          <Pill tone="due">Due</Pill>
                        ) : (
                          <Pill tone="scheduled">Scheduled</Pill>
                        )}
                      </div>
                      <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
                        <span className="flex items-center gap-1">
                          <Calendar className="h-3 w-3 shrink-0" />
                          {formatDateTime(new Date(event.start_time))}
                        </span>
                        <span className="flex items-center gap-1">
                          <Clock className="h-3 w-3 shrink-0" />
                          Reminder: {formatDateTime(reminderAt)}
                        </span>
                        <span className="flex items-center gap-1">
                          <Phone className="h-3 w-3 shrink-0" />+{event.client_phone}
                        </span>
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Sent</CardTitle>
            <Count>{sent.length}</Count>
          </CardHeader>
          <CardContent>
            {sent.length === 0 ? (
              <Empty
                icon={MessageSquare}
                title="No reminders sent yet"
                hint="Sent reminders will appear here."
              />
            ) : (
              <ul className="max-h-96 space-y-2 overflow-y-auto">
                {sent.map((message) => (
                  <li
                    key={message.id}
                    className="flex items-start gap-3 rounded-lg p-3 ring-1 ring-border"
                  >
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-green-50">
                      <Phone className="h-4 w-4 text-green-600" />
                    </div>
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-sm font-medium">+{message.to_number}</p>
                        <Pill tone="sent">Sent</Pill>
                      </div>
                      <p className="line-clamp-2 text-xs text-muted-foreground">
                        {message.message_content}
                      </p>
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Clock className="h-3 w-3 shrink-0" />
                        {formatDateTime(new Date(message.sent_at))}
                      </span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      <ReminderSettingsDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        settings={settings}
        onSave={save}
      />
    </section>
  )
}
