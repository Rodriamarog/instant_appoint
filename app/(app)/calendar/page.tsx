'use client'

import { useState, useEffect, useCallback } from 'react'
import dynamic from 'next/dynamic'
import { momentLocalizer } from 'react-big-calendar'
import type { View, CalendarProps } from 'react-big-calendar'
import moment from 'moment'
import 'react-big-calendar/lib/css/react-big-calendar.css'
import { Loader2 } from 'lucide-react'
import { pb } from '@/lib/pocketbase'
import { EventDialog } from '@/components/calendar/event-dialog'

const localizer = momentLocalizer(moment)

interface CalendarEvent {
  id: string
  title: string
  client_phone?: string
  notes?: string
  start_time: string
  end_time: string
  location?: string
}

interface BigCalendarEvent {
  id: string
  title: string
  start: Date
  end: Date
  resource: CalendarEvent
}

const BigCalendar = dynamic<CalendarProps<BigCalendarEvent, CalendarEvent>>(
  () => import('react-big-calendar').then((mod) => mod.Calendar as any),
  {
    loading: () => (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin" />
      </div>
    ),
    ssr: false,
  }
)

export default function CalendarPage() {
  const [calendarEvents, setCalendarEvents] = useState<BigCalendarEvent[]>([])
  const [calendarDate, setCalendarDate] = useState(new Date())
  const [calendarView, setCalendarView] = useState<View>('month')
  const [selectedEvent, setSelectedEvent] = useState<CalendarEvent | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create')

  const loadCalendarEvents = useCallback(async () => {
    const startDate = moment(calendarDate).startOf('month').subtract(7, 'days').toISOString()
    const endDate = moment(calendarDate).endOf('month').add(7, 'days').toISOString()
    try {
      const response = await fetch(
        `/api/calendar/events?start_date=${encodeURIComponent(startDate)}&end_date=${encodeURIComponent(endDate)}`,
        { headers: { Authorization: `Bearer ${pb.authStore.token}` } }
      )
      if (!response.ok) {
        const err = await response.json().catch(() => ({}))
        console.error('Failed to load calendar events:', response.status, err)
        return
      }
      const data = await response.json()
      const converted: BigCalendarEvent[] = (data.events || []).map((e: CalendarEvent) => ({
        id: e.id,
        title: e.title,
        start: new Date(e.start_time),
        end: new Date(e.end_time),
        resource: e,
      }))
      setCalendarEvents(converted)
    } catch (err) {
      console.error('Error loading calendar events:', err)
    }
  }, [calendarDate])

  useEffect(() => {
    loadCalendarEvents()
  }, [loadCalendarEvents])

  return (
    <div style={{ height: 'calc(100vh - 120px)' }}>
      <BigCalendar
        localizer={localizer}
        events={calendarEvents}
        startAccessor="start"
        endAccessor="end"
        view={calendarView}
        onView={setCalendarView}
        date={calendarDate}
        onNavigate={setCalendarDate}
        onSelectSlot={(slotInfo) => {
          setSelectedEvent({
            id: '',
            title: '',
            start_time: slotInfo.start.toISOString(),
            end_time: slotInfo.end.toISOString(),
          })
          setDialogMode('create')
          setDialogOpen(true)
        }}
        onSelectEvent={(event) => {
          setSelectedEvent(event.resource)
          setDialogMode('edit')
          setDialogOpen(true)
        }}
        selectable
        popup
        step={30}
        showMultiDayTimes
        style={{ height: '100%' }}
      />
      <EventDialog
        open={dialogOpen}
        mode={dialogMode}
        event={selectedEvent}
        onClose={() => setDialogOpen(false)}
        onSaved={() => {
          setDialogOpen(false)
          loadCalendarEvents()
        }}
      />
    </div>
  )
}
