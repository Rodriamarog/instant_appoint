'use client'

import { useState, useEffect, useMemo } from 'react'
import { Bell, BellOff, Calendar, Clock, Loader2, MessageSquare, Phone, Settings } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { pb } from '@/lib/pocketbase'

const TIMING_OPTIONS = [
  { label: '30 min',   minutes: 30 },
  { label: '1 hour',   minutes: 60 },
  { label: '2 hours',  minutes: 120 },
  { label: '12 hours', minutes: 720 },
  { label: '24 hours', minutes: 1440 },
]

interface ReminderSettings {
  id?: string
  timing_minutes: number
  is_active: boolean
}

interface CalendarEvent {
  id: string
  title: string
  client_phone?: string
  notes?: string
  start_time: string
}

interface SentMessage {
  id: string
  event_id: string
  to_number: string
  message_content: string
  sent_at: string
  status: string
}

function formatDateTime(iso: string) {
  const d = new Date(iso)
  return (
    d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' }) +
    ' at ' +
    d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  )
}

const authHeaders = () => ({
  'Content-Type': 'application/json',
  Authorization: `Bearer ${pb.authStore.token}`,
})

export function RemindersTab({ whatsappStatus }: { whatsappStatus: string | null }) {
  const [settings, setSettings] = useState<ReminderSettings>({ timing_minutes: 1440, is_active: true })
  const [events, setEvents] = useState<CalendarEvent[]>([])
  const [sentEventIds, setSentEventIds] = useState<Set<string>>(new Set())
  const [sentMessages, setSentMessages] = useState<SentMessage[]>([])
  const [isLoadingSettings, setIsLoadingSettings] = useState(true)
  const [isLoadingEvents, setIsLoadingEvents] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)

  useEffect(() => {
    loadSettings()
    loadEvents()
    loadSentReminders()
  }, [])

  const loadSettings = async () => {
    try {
      const res = await fetch('/api/reminders/settings', { headers: authHeaders() })
      if (res.ok) setSettings(await res.json())
    } catch {
      // use defaults
    } finally {
      setIsLoadingSettings(false)
    }
  }

  const loadEvents = async () => {
    try {
      const start = new Date(Date.now() - 60 * 60 * 1000).toISOString()
      const end = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString()
      const res = await fetch(
        `/api/calendar/events?start_date=${encodeURIComponent(start)}&end_date=${encodeURIComponent(end)}`,
        { headers: authHeaders() }
      )
      if (res.ok) {
        const data = await res.json()
        setEvents(data.events || [])
      }
    } catch {
      // leave empty
    } finally {
      setIsLoadingEvents(false)
    }
  }

  const loadSentReminders = async () => {
    try {
      const res = await fetch('/api/reminders/sent', { headers: authHeaders() })
      if (res.ok) {
        const data = await res.json()
        setSentEventIds(new Set(data.eventIds))
        setSentMessages(data.messages || [])
      }
    } catch {
      // leave empty
    }
  }

  const upcomingReminders = useMemo(() => {
    return events
      .filter(e => e.client_phone)
      .map(e => ({
        event: e,
        reminderAt: new Date(new Date(e.start_time).getTime() - settings.timing_minutes * 60 * 1000),
        sent: sentEventIds.has(e.id),
      }))
      .filter(r => !r.sent && new Date(r.event.start_time) > new Date(Date.now() - 60 * 60 * 1000))
      .sort((a, b) => new Date(a.event.start_time).getTime() - new Date(b.event.start_time).getTime())
  }, [events, settings.timing_minutes, sentEventIds])

  const saveSettings = async () => {
    setIsSaving(true)
    try {
      const res = await fetch('/api/reminders/settings', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({ timing_minutes: settings.timing_minutes, is_active: settings.is_active }),
      })
      if (res.ok) {
        setSettings(await res.json())
        setSettingsOpen(false)
      }
    } finally {
      setIsSaving(false)
    }
  }

  const whatsappConnected = whatsappStatus === 'connected'
  const whatsappStatusKnown = whatsappStatus !== null

  if (isLoadingSettings) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
      </div>
    )
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 mb-2">Reminders</h2>
          <p className="text-gray-600">
            Automatically send WhatsApp reminders to clients before their appointments.
          </p>
        </div>
        <button
          onClick={() => setSettingsOpen(true)}
          className="mt-1 p-2 rounded-lg text-gray-400 hover:text-gray-700 hover:bg-gray-100 transition-colors"
          aria-label="Reminder settings"
        >
          <Settings className="h-5 w-5" />
        </button>
      </div>

      {/* WhatsApp not connected warning */}
      {whatsappStatusKnown && !whatsappConnected && (
        <div className="flex items-start gap-3 p-4 bg-amber-50 border border-amber-200 rounded-lg">
          <BellOff className="h-5 w-5 text-amber-600 mt-0.5 shrink-0" />
          <p className="text-sm text-amber-800">
            <strong>WhatsApp not connected.</strong> Reminders won&apos;t send until you connect your
            WhatsApp account in the WhatsApp Setup tab.
          </p>
        </div>
      )}

      {/* Two-column layout */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        {/* Upcoming Reminders */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>Upcoming</CardTitle>
              {!isLoadingEvents && (
                <Badge variant="secondary">{upcomingReminders.length}</Badge>
              )}
            </div>
          </CardHeader>
          <CardContent>
            {isLoadingEvents ? (
              <div className="flex items-center justify-center py-10">
                <Loader2 className="h-5 w-5 animate-spin text-gray-400" />
              </div>
            ) : upcomingReminders.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-10 space-y-2 text-center">
                <div className="w-10 h-10 bg-gray-100 rounded-xl flex items-center justify-center">
                  <Bell className="h-5 w-5 text-gray-400" />
                </div>
                <p className="text-sm font-medium text-gray-700">No upcoming reminders</p>
                <p className="text-xs text-gray-400">Appointments with a phone number will appear here.</p>
              </div>
            ) : (
              <div className="space-y-2">
                {upcomingReminders.map(({ event, reminderAt, sent }) => (
                  <div
                    key={event.id}
                    className="flex items-start gap-3 p-3 border border-gray-100 rounded-lg hover:bg-gray-50 transition-colors"
                  >
                    <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center shrink-0">
                      <span className="text-xs font-bold text-gray-600">
                        {event.title.charAt(0).toUpperCase()}
                      </span>
                    </div>
                    <div className="flex-1 min-w-0 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-sm font-medium text-gray-900 truncate">{event.title}</p>
                        {sent
                          ? <Badge variant="secondary" className="text-xs shrink-0 bg-green-100 text-green-700 border-green-200">Sent</Badge>
                          : reminderAt <= new Date()
                            ? <Badge variant="secondary" className="text-xs shrink-0 bg-amber-100 text-amber-700 border-amber-200">Due</Badge>
                            : <Badge variant="outline" className="text-xs shrink-0 text-gray-500">Scheduled</Badge>
                        }
                      </div>
                      <div className="flex flex-col gap-0.5 text-xs text-gray-500">
                        <span className="flex items-center gap-1">
                          <Calendar className="h-3 w-3 shrink-0" />
                          {formatDateTime(event.start_time)}
                        </span>
                        <span className="flex items-center gap-1">
                          <Clock className="h-3 w-3 shrink-0" />
                          Reminder: {formatDateTime(reminderAt.toISOString())}
                        </span>
                        {event.client_phone && (
                          <span className="flex items-center gap-1">
                            <Phone className="h-3 w-3 shrink-0" />
                            +{event.client_phone}
                          </span>
                        )}
                      </div>
                      {event.notes && (
                        <p className="flex items-center gap-1 text-xs text-gray-400 truncate">
                          <MessageSquare className="h-3 w-3 shrink-0" />
                          {event.notes}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Sent Reminders */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>Sent</CardTitle>
              <Badge variant="secondary">{sentMessages.length}</Badge>
            </div>
          </CardHeader>
          <CardContent>
            {sentMessages.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-10 space-y-2 text-center">
                <div className="w-10 h-10 bg-gray-100 rounded-xl flex items-center justify-center">
                  <MessageSquare className="h-5 w-5 text-gray-400" />
                </div>
                <p className="text-sm font-medium text-gray-700">No reminders sent yet</p>
                <p className="text-xs text-gray-400">Sent reminders will appear here.</p>
              </div>
            ) : (
              <div className="space-y-2 max-h-96 overflow-y-auto">
                {sentMessages.map(msg => (
                  <div
                    key={msg.id}
                    className="flex items-start gap-3 p-3 border border-gray-100 rounded-lg hover:bg-gray-50 transition-colors"
                  >
                    <div className="w-8 h-8 bg-green-50 rounded-lg flex items-center justify-center shrink-0">
                      <Phone className="h-4 w-4 text-green-600" />
                    </div>
                    <div className="flex-1 min-w-0 space-y-1">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-sm font-medium text-gray-900">+{msg.to_number}</p>
                        <Badge variant="secondary" className="text-xs shrink-0 bg-green-100 text-green-700 border-green-200">Sent</Badge>
                      </div>
                      <p className="text-xs text-gray-500 line-clamp-2">{msg.message_content}</p>
                      <span className="flex items-center gap-1 text-xs text-gray-400">
                        <Clock className="h-3 w-3 shrink-0" />
                        {formatDateTime(msg.sent_at)}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Settings Dialog */}
      <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Reminder Settings</DialogTitle>
          </DialogHeader>
          <div className="space-y-5 pt-2">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-gray-700">Enable reminders</span>
              <button
                onClick={() => setSettings(s => ({ ...s, is_active: !s.is_active }))}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-black ${
                  settings.is_active ? 'bg-black' : 'bg-gray-200'
                }`}
                aria-label={settings.is_active ? 'Disable reminders' : 'Enable reminders'}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                    settings.is_active ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>

            <div className="space-y-3">
              <label className="text-sm font-medium text-gray-700">Send reminder before appointment</label>
              <div className="flex flex-wrap gap-2">
                {TIMING_OPTIONS.map(opt => (
                  <button
                    key={opt.minutes}
                    onClick={() => setSettings(s => ({ ...s, timing_minutes: opt.minutes }))}
                    className={`px-4 py-2 rounded-lg text-sm font-medium border transition-colors ${
                      settings.timing_minutes === opt.minutes
                        ? 'bg-black text-white border-black'
                        : 'bg-white text-gray-700 border-gray-200 hover:border-gray-400'
                    }`}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-1">
              <Button
                variant="outline"
                onClick={() => setSettingsOpen(false)}
                disabled={isSaving}
              >
                Cancel
              </Button>
              <Button
                onClick={saveSettings}
                disabled={isSaving}
                className="bg-black text-white hover:bg-gray-800"
              >
                {isSaving ? <><Loader2 className="h-4 w-4 animate-spin mr-2" />Saving...</> : 'Save'}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
