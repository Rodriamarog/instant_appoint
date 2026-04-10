'use client'

import { useState, useEffect } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Loader2 } from 'lucide-react'
import { pb } from '@/lib/pocketbase'

export interface CalendarEvent {
  id: string
  title: string
  client_phone?: string
  notes?: string
  start_time: string
  end_time: string
}

interface EventDialogProps {
  open: boolean
  mode: 'create' | 'edit'
  event: CalendarEvent | null
  onClose: () => void
  onSaved: () => void
}

const COUNTRY_CODES = [
  { label: '+1',  value: '1',   flag: '🇺🇸' },
  { label: '+52', value: '521', flag: '🇲🇽' },
]

export function EventDialog({ open, mode, event, onClose, onSaved }: EventDialogProps) {
  const [clientName, setClientName] = useState('')
  const [countryCode, setCountryCode] = useState('1')
  const [phoneNumber, setPhoneNumber] = useState('')
  const [reminderMessage, setReminderMessage] = useState('')
  const [startDate, setStartDate] = useState('')
  const [startTime, setStartTime] = useState('')
  const [endDate, setEndDate] = useState('')
  const [endTime, setEndTime] = useState('')
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Function to generate default reminder message based on country code
  const generateReminderMessage = (clientName: string, startDate: string, startTime: string, countryCode: string) => {
    const businessName = (typeof window !== 'undefined' && pb.authStore.model?.business_name) || 'our clinic'

    // If we have date and time, always use them even without client name
    if (startDate && startTime) {
      const appointmentDate = new Date(`${startDate}T${startTime}`)

      if (countryCode === '521') {
        // Spanish for Mexican clients
        const timeStr = appointmentDate.toLocaleTimeString('es-MX', {
          hour: 'numeric',
          minute: '2-digit',
          hour12: true
        })

        const dateStr = appointmentDate.toLocaleDateString('es-MX', {
          weekday: 'long',
          day: 'numeric',
          month: 'long'
        })

        return clientName
          ? `Hola ${clientName}, solo un recordatorio sobre su cita de mañana ${dateStr} a las ${timeStr} en ${businessName}.`
          : `Hola, solo un recordatorio sobre su cita de mañana ${dateStr} a las ${timeStr} en ${businessName}.`
      } else {
        // English for US clients
        const timeStr = appointmentDate.toLocaleTimeString('en-US', {
          hour: 'numeric',
          minute: '2-digit',
          hour12: true
        })

        const dateStr = appointmentDate.toLocaleDateString('en-US', {
          weekday: 'long',
          month: 'long',
          day: 'numeric'
        })

        return clientName
          ? `Hello ${clientName}, just a reminder about your appointment tomorrow ${dateStr} at ${timeStr} at ${businessName}.`
          : `Hello, just a reminder about your appointment tomorrow ${dateStr} at ${timeStr} at ${businessName}.`
      }
    }

    // Fallback if no date/time available
    return countryCode === '521'
      ? `Hola, solo un recordatorio sobre su cita de mañana en ${businessName}.`
      : `Hello, just a reminder about your appointment tomorrow at ${businessName}.`
  }

  useEffect(() => {
    if (event) {
      const eventClientName = event.title || ''
      setClientName(eventClientName)
      // Parse stored phone back into country code + local number
      const stored = event.client_phone || ''
      const mx = COUNTRY_CODES.find(c => c.value === '521')!
      const us = COUNTRY_CODES.find(c => c.value === '1')!
      if (stored.startsWith('521')) {
        setCountryCode('521')
        setPhoneNumber(stored.slice(3))
      } else if (stored.startsWith('1')) {
        setCountryCode('1')
        setPhoneNumber(stored.slice(1))
      } else {
        setCountryCode('1')
        setPhoneNumber(stored)
      }
      const start = new Date(event.start_time)
      const eventStartDate = start.toISOString().split('T')[0]
      const eventStartTime = start.toTimeString().slice(0, 5)
      setStartDate(eventStartDate)
      setStartTime(eventStartTime)
      const end = new Date(event.end_time)
      setEndDate(end.toISOString().split('T')[0])
      setEndTime(end.toTimeString().slice(0, 5))

      // Set reminder message - use existing notes or generate new one
      const eventCountryCode = stored.startsWith('521') ? '521' : '1'
      setReminderMessage(event.notes || generateReminderMessage(eventClientName, eventStartDate, eventStartTime, eventCountryCode))
    } else {
      setClientName('')
      setCountryCode('1')
      setPhoneNumber('')
      const now = new Date()
      const defaultStartDate = now.toISOString().split('T')[0]
      const defaultStartTime = now.toTimeString().slice(0, 5)
      setStartDate(defaultStartDate)
      setStartTime(defaultStartTime)
      const later = new Date(now.getTime() + 3600000)
      setEndDate(later.toISOString().split('T')[0])
      setEndTime(later.toTimeString().slice(0, 5))
      setReminderMessage('Hello, just a reminder about your appointment!')
    }
    setError(null)
  }, [event, open])

  // Update reminder message when client name or appointment details change
  useEffect(() => {
    if (startDate && startTime) {
      setReminderMessage(generateReminderMessage(clientName, startDate, startTime, countryCode))
    }
  }, [clientName, startDate, startTime, countryCode])

  const authHeaders = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${pb.authStore.token}`,
  }

  const handleSave = async () => {
    setError(null)
    setSaving(true)
    try {
      if (!clientName.trim()) {
        setError('Client name is required')
        setSaving(false)
        return
      }
      const start_time = new Date(`${startDate}T${startTime}`).toISOString()
      const end_time = new Date(`${endDate}T${endTime}`).toISOString()
      if (new Date(end_time) <= new Date(start_time)) {
        setError('End time must be after start time')
        setSaving(false)
        return
      }

      const client_phone = phoneNumber.trim() ? `${countryCode}${phoneNumber.replace(/\D/g, '')}` : ''

      const body = {
        title: clientName,
        client_phone,
        notes: reminderMessage,
        start_time,
        end_time,
      }

      const response = mode === 'create'
        ? await fetch('/api/calendar/events', { method: 'POST', headers: authHeaders, body: JSON.stringify(body) })
        : await fetch(`/api/calendar/events/${event?.id}`, { method: 'PATCH', headers: authHeaders, body: JSON.stringify(body) })

      if (!response.ok) {
        const data = await response.json()
        throw new Error(data.error || 'Failed to save appointment')
      }
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error saving appointment')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Delete this appointment?')) return
    setError(null)
    setDeleting(true)
    try {
      const response = await fetch(`/api/calendar/events/${event?.id}`, {
        method: 'DELETE',
        headers: authHeaders,
      })
      if (!response.ok) {
        const data = await response.json()
        throw new Error(data.error || 'Failed to delete appointment')
      }
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error deleting appointment')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>{mode === 'create' ? 'New Appointment' : 'Edit Appointment'}</DialogTitle>
          <DialogDescription>
            {mode === 'create' ? 'Schedule a new appointment' : 'Edit appointment details'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-2">
            <Label htmlFor="clientName">Client Name *</Label>
            <Input id="clientName" value={clientName} onChange={(e) => setClientName(e.target.value)} placeholder="e.g. John Smith" />
          </div>

          <div className="space-y-2">
            <Label htmlFor="phone">Phone Number</Label>
            <div className="flex gap-2">
              <select
                value={countryCode}
                onChange={(e) => setCountryCode(e.target.value)}
                className="border border-gray-300 rounded-md px-2 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-black"
              >
                {COUNTRY_CODES.map((c) => (
                  <option key={c.value} value={c.value}>{c.flag} {c.label}</option>
                ))}
              </select>
              <Input
                id="phone"
                type="text"
                placeholder="5512345678"
                value={phoneNumber}
                onChange={(e) => setPhoneNumber(e.target.value)}
                className="flex-1"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="start-date">Start Date *</Label>
              <Input id="start-date" type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="start-time">Start Time *</Label>
              <Input id="start-time" type="time" value={startTime} onChange={(e) => setStartTime(e.target.value)} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="end-date">End Date *</Label>
              <Input id="end-date" type="date" value={endDate} onChange={(e) => setEndDate(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="end-time">End Time *</Label>
              <Input id="end-time" type="time" value={endTime} onChange={(e) => setEndTime(e.target.value)} />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="reminderMessage">Reminder Message</Label>
            <Textarea
              id="reminderMessage"
              value={reminderMessage}
              onChange={(e) => setReminderMessage(e.target.value)}
              placeholder="e.g. Hello, just a reminder about your appointment tomorrow!"
              rows={3}
            />
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          {mode === 'edit' && (
            <Button type="button" variant="destructive" onClick={handleDelete} disabled={deleting || saving} className="sm:mr-auto">
              {deleting ? <><Loader2 className="h-4 w-4 animate-spin mr-2" />Deleting...</> : 'Delete'}
            </Button>
          )}
          <Button type="button" variant="outline" onClick={onClose} disabled={saving || deleting}>Cancel</Button>
          <Button type="button" onClick={handleSave} disabled={saving || deleting} className="bg-black text-white hover:bg-gray-800">
            {saving ? <><Loader2 className="h-4 w-4 animate-spin mr-2" />Saving...</> : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
