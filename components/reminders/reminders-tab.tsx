'use client'

import { useState, useEffect, useMemo } from 'react'
import { Bell, BellOff, Calendar, Clock, Loader2, MessageSquare, Phone } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { pb } from '@/lib/pocketbase'

const TIMING_OPTIONS = [
  { label: '30 min',  minutes: 30 },
  { label: '1 hour',  minutes: 60 },
  { label: '2 hours', minutes: 120 },
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

export function RemindersTab({ whatsappStatus }: { whatsappStatus: string }) {
  const [settings, setSettings] = useState<ReminderSettings>({ timing_minutes: 1440, is_active: true })
  const [events, setEvents] = useState<CalendarEvent[]>([])
  const [isLoadingSettings, setIsLoadingSettings] = useState(true)
  const [isLoadingEvents, setIsLoadingEvents] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [saveSuccess, setSaveSuccess] = useState(false)

  useEffect(() => {
    loadSettings()
    loadEvents()
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
      const start = new Date().toISOString()
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

  // Derive upcoming reminders from events + current timing setting (no extra fetch needed)
  const upcomingReminders = useMemo(() => {
    const now = new Date()
    return events
      .filter(e => e.client_phone)
      .map(e => ({
        event: e,
        reminderAt: new Date(new Date(e.start_time).getTime() - settings.timing_minutes * 60 * 1000),
      }))
      .filter(r => r.reminderAt > now)
      .sort((a, b) => new Date(a.event.start_time).getTime() - new Date(b.event.start_time).getTime())
  }, [events, settings.timing_minutes])

  const saveSettings = async () => {
    setIsSaving(true)
    setSaveSuccess(false)
    try {
      const res = await fetch('/api/reminders/settings', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({ timing_minutes: settings.timing_minutes, is_active: settings.is_active }),
      })
      if (res.ok) {
        setSettings(await res.json())
        setSaveSuccess(true)
        setTimeout(() => setSaveSuccess(false), 2000)
      }
    } finally {
      setIsSaving(false)
    }
  }

  const whatsappConnected = whatsappStatus === 'connected'

  if (isLoadingSettings) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <div>
        <h2 className="text-2xl font-bold text-gray-900 mb-2">Reminders</h2>
        <p className="text-gray-600">
          Automatically send WhatsApp reminders to clients before their appointments.
        </p>
      </div>

      {/* WhatsApp not connected warning */}
      {!whatsappConnected && (
        <div className="flex items-start gap-3 p-4 bg-amber-50 border border-amber-200 rounded-lg">
          <BellOff className="h-5 w-5 text-amber-600 mt-0.5 shrink-0" />
          <p className="text-sm text-amber-800">
            <strong>WhatsApp not connected.</strong> Reminders won&apos;t send until you connect your
            WhatsApp account in the WhatsApp Setup tab.
          </p>
        </div>
      )}

      {/* Settings card */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Settings</CardTitle>
            {/* Toggle */}
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
        </CardHeader>
        <CardContent className="space-y-5">
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

          <div className="flex items-center justify-between pt-1">
            <p className="text-sm text-gray-500">
              {settings.is_active
                ? `Reminders will be sent ${TIMING_OPTIONS.find(o => o.minutes === settings.timing_minutes)?.label ?? ''} before each appointment.`
                : 'Reminders are currently disabled.'}
            </p>
            <Button
              onClick={saveSettings}
              disabled={isSaving}
              className="bg-black text-white hover:bg-gray-800 shrink-0 ml-4"
            >
              {isSaving ? (
                <><Loader2 className="h-4 w-4 animate-spin mr-2" />Saving...</>
              ) : saveSuccess ? (
                'Saved!'
              ) : (
                'Save'
              )}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Upcoming reminders */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Upcoming Reminders</CardTitle>
            {!isLoadingEvents && (
              <Badge variant="secondary">
                {upcomingReminders.length} scheduled
              </Badge>
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
              <p className="text-xs text-gray-400">
                Appointments with a phone number will appear here.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {upcomingReminders.map(({ event, reminderAt }) => (
                <div
                  key={event.id}
                  className="flex items-start gap-4 p-4 border border-gray-100 rounded-lg hover:bg-gray-50 transition-colors"
                >
                  {/* Avatar */}
                  <div className="w-9 h-9 bg-gray-100 rounded-lg flex items-center justify-center shrink-0">
                    <span className="text-sm font-bold text-gray-600">
                      {event.title.charAt(0).toUpperCase()}
                    </span>
                  </div>

                  {/* Content */}
                  <div className="flex-1 min-w-0 space-y-1.5">
                    <div className="flex items-center justify-between gap-2">
                      <p className="font-medium text-gray-900 truncate">{event.title}</p>
                      <Badge variant="outline" className="text-xs shrink-0 text-gray-500">
                        Scheduled
                      </Badge>
                    </div>

                    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500">
                      <span className="flex items-center gap-1">
                        <Calendar className="h-3 w-3" />
                        {formatDateTime(event.start_time)}
                      </span>
                      <span className="flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        Reminder: {formatDateTime(reminderAt.toISOString())}
                      </span>
                      {event.client_phone && (
                        <span className="flex items-center gap-1">
                          <Phone className="h-3 w-3" />
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
    </div>
  )
}
