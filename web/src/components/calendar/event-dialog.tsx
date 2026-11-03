import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { errorMessage } from '@/lib/errors'
import {
  createAppointment,
  deleteAppointment,
  updateAppointment,
  type CalendarEvent,
} from '@/lib/calendar-events'
import {
  localDateInput,
  localInputsToIso,
  localTimeInput,
} from '@/lib/iso-time'

interface EventDialogProps {
  open: boolean
  mode: 'create' | 'edit'
  /** The appointment being edited, or the blank slot that was selected. */
  event: CalendarEvent | null
  onClose: () => void
  /** Called after a successful save or delete, so the calendar can reload. */
  onSaved: () => void
}

const COUNTRY_CODES = [
  { label: '+1', value: '1', flag: '🇺🇸' },
  { label: '+52', value: '521', flag: '🇲🇽' },
]

const DEFAULT_COUNTRY_CODE = '1'

/** Splits a stored phone number back into the two fields that produced it. */
function splitPhone(stored: string): { countryCode: string; digits: string } {
  for (const { value } of [...COUNTRY_CODES].sort(
    (a, b) => b.value.length - a.value.length,
  )) {
    if (stored.startsWith(value)) {
      return { countryCode: value, digits: stored.slice(value.length) }
    }
  }

  return { countryCode: DEFAULT_COUNTRY_CODE, digits: stored }
}

export function EventDialog({
  open,
  mode,
  event,
  onClose,
  onSaved,
}: EventDialogProps) {
  const [clientName, setClientName] = useState('')
  const [countryCode, setCountryCode] = useState(DEFAULT_COUNTRY_CODE)
  const [phoneNumber, setPhoneNumber] = useState('')
  const [location, setLocation] = useState('')
  const [notes, setNotes] = useState('')
  const [startDate, setStartDate] = useState('')
  const [startTime, setStartTime] = useState('')
  const [endDate, setEndDate] = useState('')
  const [endTime, setEndTime] = useState('')
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setError(null)

    if (!event) {
      const now = new Date()
      const later = new Date(now.getTime() + 60 * 60 * 1000)
      setClientName('')
      setCountryCode(DEFAULT_COUNTRY_CODE)
      setPhoneNumber('')
      setLocation('')
      setStartDate(localDateInput(now))
      setStartTime(localTimeInput(now))
      setEndDate(localDateInput(later))
      setEndTime(localTimeInput(later))
      setNotes('')

      return
    }

    const { countryCode: code, digits } = splitPhone(event.client_phone || '')
    setClientName(event.title || '')
    setCountryCode(code)
    setPhoneNumber(digits)
    setLocation(event.location || '')

    // Both halves of each timestamp come from the same local clock. Reading
    // the date in UTC and the time locally used to shift an evening
    // appointment by a day as soon as it was reopened.
    const start = new Date(event.start_time)
    const end = new Date(event.end_time)
    setStartDate(localDateInput(start))
    setStartTime(localTimeInput(start))
    setEndDate(localDateInput(end))
    setEndTime(localTimeInput(end))

    setNotes(event.notes || '')
  }, [event, open])

  const handleSave = async () => {
    setError(null)

    if (!clientName.trim()) {
      setError('Client name is required')
      return
    }

    let start_time: string
    let end_time: string
    try {
      start_time = localInputsToIso(startDate, startTime)
      end_time = localInputsToIso(endDate, endTime)
    } catch {
      setError('Both the start and end date and time are required')
      return
    }

    if (new Date(end_time) <= new Date(start_time)) {
      setError('End time must be after start time')
      return
    }

    const draft = {
      title: clientName,
      client_phone: phoneNumber.trim()
        ? `${countryCode}${phoneNumber.replace(/\D/g, '')}`
        : '',
      notes,
      location,
      start_time,
      end_time,
    }

    setSaving(true)
    try {
      if (mode === 'create' || !event?.id) {
        await createAppointment(draft)
      } else {
        await updateAppointment(event.id, draft)
      }
      onSaved()
    } catch (err) {
      setError(errorMessage(err, 'Error saving appointment'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!event?.id) return
    if (!confirm('Delete this appointment?')) return

    setError(null)
    setDeleting(true)
    try {
      await deleteAppointment(event.id)
      onSaved()
    } catch (err) {
      setError(errorMessage(err, 'Error deleting appointment'))
    } finally {
      setDeleting(false)
    }
  }

  const busy = saving || deleting

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>
            {mode === 'create' ? 'New Appointment' : 'Edit Appointment'}
          </DialogTitle>
          <DialogDescription>
            {mode === 'create'
              ? 'Schedule a new appointment'
              : 'Edit appointment details'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-2">
            <Label htmlFor="client-name">Client Name *</Label>
            <Input
              id="client-name"
              value={clientName}
              onChange={(e) => setClientName(e.target.value)}
              placeholder="e.g. Ana Ruiz"
            />
          </div>

          <div className="grid grid-cols-[auto_1fr] gap-2">
            <div className="space-y-2">
              <Label htmlFor="country-code">Country code</Label>
              <select
                id="country-code"
                value={countryCode}
                onChange={(e) => setCountryCode(e.target.value)}
                className="h-9 rounded-3xl border border-transparent bg-input/50 px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30"
              >
                {COUNTRY_CODES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.flag} {c.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="phone">Phone Number</Label>
              <Input
                id="phone"
                type="text"
                placeholder="5512345678"
                value={phoneNumber}
                onChange={(e) => setPhoneNumber(e.target.value)}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="start-date">Start Date *</Label>
              <Input
                id="start-date"
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="start-time">Start Time *</Label>
              <Input
                id="start-time"
                type="time"
                value={startTime}
                onChange={(e) => setStartTime(e.target.value)}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="end-date">End Date *</Label>
              <Input
                id="end-date"
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="end-time">End Time *</Label>
              <Input
                id="end-time"
                type="time"
                value={endTime}
                onChange={(e) => setEndTime(e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="location">Location</Label>
            <Input
              id="location"
              value={location}
              onChange={(e) => setLocation(e.target.value)}
              placeholder="e.g. Suite 2"
            />
          </div>

          {/*
            This used to be offered as the reminder the client would receive.
            It never was one: a reminder leaves well outside Meta's 24-hour
            window, where only an approved template is accepted, so the words
            typed here could not be delivered and the reminder path never read
            them. Saying plainly who reads it is what stops someone writing a
            message to a client in it again.
          */}
          <div className="space-y-2">
            <Label htmlFor="notes">Notes</Label>
            <Textarea
              id="notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="e.g. Allergic to latex"
              rows={3}
            />
            <p className="text-xs text-muted-foreground">
              For your team only. The client never sees this.
            </p>
          </div>
        </div>

        <DialogFooter className="gap-2">
          {mode === 'edit' && event?.id && (
            <Button
              type="button"
              variant="destructive"
              onClick={handleDelete}
              disabled={busy}
              className="sm:mr-auto"
            >
              {deleting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deleting...
                </>
              ) : (
                'Delete'
              )}
            </Button>
          )}
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="button" onClick={handleSave} disabled={busy}>
            {saving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              'Save'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
