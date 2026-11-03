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
import { spellDateTime } from '@/lib/dates'
import { errorMessage } from '@/lib/errors'
import {
  actorLabel,
  createAppointment,
  deleteAppointment,
  languageForCountryCode,
  REMINDER_LANGUAGES,
  setAppointmentStatus,
  statusLabel,
  updateAppointment,
  type Appointment,
  type AppointmentStatus,
} from '@/lib/appointments'
import {
  localDateInput,
  localInputsToIso,
  localTimeInput,
} from '@/lib/iso-time'

interface EventDialogProps {
  open: boolean
  mode: 'create' | 'edit'
  /** The appointment being edited, or the blank slot that was selected. */
  event: Appointment | null
  onClose: () => void
  /** Called after a successful save or delete, so the calendar can reload. */
  onSaved: () => void
}

const COUNTRY_CODES = [
  { label: '+1', value: '1', flag: '🇺🇸' },
  { label: '+52', value: '521', flag: '🇲🇽' },
]

/**
 * Mexico, because the clinic is in Tijuana and most of its patients dial
 * from this side of the line.
 *
 * It used to be +1, which was wrong twice over once the reminder language
 * started following the prefix: it put the wrong country on most new
 * bookings, and it defaulted their reminders to English — reversing, with
 * nobody deciding anything, the language every appointment on the calendar
 * already went out in.
 */
const DEFAULT_COUNTRY_CODE = '521'

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

/**
 * Where the appointment stands and how it got there, as one sentence:
 * "Cancelada por el paciente · lun 2 nov 2026, 09:30".
 *
 * Each half is dropped rather than faked when the row does not carry it. An
 * appointment migrated in from before this existed has a status and no
 * author, and inventing one would answer the only question this line is
 * here for with a guess.
 */
function statusLine(event: Appointment): string {
  const who = actorLabel(event.status_set_by)
  const moment = new Date(event.status_set_at ?? '')
  // An unreadable timestamp is dropped rather than printed as "Invalid
  // Date": the sentence still answers who, which is the half that matters.
  const when = Number.isNaN(moment.getTime()) ? '' : spellDateTime(moment)

  return [
    [statusLabel(event.status), who && `por ${who}`].filter(Boolean).join(' '),
    when,
  ]
    .filter(Boolean)
    .join(' · ')
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
  const [language, setLanguage] = useState(() =>
    languageForCountryCode(DEFAULT_COUNTRY_CODE),
  )
  // Whether the language on screen is somebody's decision or still the guess
  // made from the dialling prefix. Only the guess is allowed to move when
  // the prefix changes: re-deriving it unconditionally would make an
  // override impossible to keep, which is the failure the issue calls
  // "inferred and locked".
  const [languageChosen, setLanguageChosen] = useState(false)
  const [location, setLocation] = useState('')
  const [notes, setNotes] = useState('')
  const [startDate, setStartDate] = useState('')
  const [startTime, setStartTime] = useState('')
  const [endDate, setEndDate] = useState('')
  const [endTime, setEndTime] = useState('')
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [settingStatus, setSettingStatus] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setError(null)

    if (!event) {
      const now = new Date()
      const later = new Date(now.getTime() + 60 * 60 * 1000)
      setClientName('')
      setCountryCode(DEFAULT_COUNTRY_CODE)
      setPhoneNumber('')
      setLanguage(languageForCountryCode(DEFAULT_COUNTRY_CODE))
      setLanguageChosen(false)
      setLocation('')
      setStartDate(localDateInput(now))
      setStartTime(localTimeInput(now))
      setEndDate(localDateInput(later))
      setEndTime(localTimeInput(later))
      setNotes('')

      return
    }

    const { countryCode: code, digits } = splitPhone(event.client_phone || '')
    setClientName(event.client_name || '')
    setCountryCode(code)
    setPhoneNumber(digits)
    // A stored language is a decision already recorded — by staff here, or
    // by the migration copying across what the business was already sending
    // in — so it outranks the prefix and the prefix no longer moves it.
    //
    // A blank slot dragged out on the calendar carries no decisions, only
    // the times: whatever language is on it is this dialog's own guess, and
    // the prefix is still free to move it. event.id is how the rest of this
    // file tells a stored appointment from a slot.
    const stored = Boolean(event.id && event.language)
    setLanguage(stored ? event.language : languageForCountryCode(code))
    setLanguageChosen(stored)
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
      setError('El nombre del cliente es obligatorio')
      return
    }

    let start_time: string
    let end_time: string
    try {
      start_time = localInputsToIso(startDate, startTime)
      end_time = localInputsToIso(endDate, endTime)
    } catch {
      setError('Hacen falta la fecha y la hora de inicio y de fin')
      return
    }

    if (new Date(end_time) <= new Date(start_time)) {
      setError('La hora de fin debe ser posterior a la de inicio')
      return
    }

    const draft = {
      client_name: clientName,
      client_phone: phoneNumber.trim()
        ? `${countryCode}${phoneNumber.replace(/\D/g, '')}`
        : '',
      notes,
      location,
      start_time,
      end_time,
      language,
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
      setError(errorMessage(err, 'No se pudo guardar la cita'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!event?.id) return
    if (!confirm('¿Eliminar esta cita?')) return

    setError(null)
    setDeleting(true)
    try {
      await deleteAppointment(event.id)
      onSaved()
    } catch (err) {
      setError(errorMessage(err, 'No se pudo eliminar la cita'))
    } finally {
      setDeleting(false)
    }
  }

  /**
   * Deciding something about the appointment, which is not the same act as
   * editing it: nothing from the form is sent. A button that posted the
   * whole form would save whatever half-typed changes were on screen,
   * without anybody having pressed Save.
   */
  const handleStatus = async (status: AppointmentStatus) => {
    if (!event?.id) return

    setError(null)
    setSettingStatus(true)
    try {
      await setAppointmentStatus(event.id, status)
      onSaved()
    } catch (err) {
      setError(errorMessage(err, 'No se pudo cambiar el estado de la cita'))
    } finally {
      setSettingStatus(false)
    }
  }

  /** Changing the country re-guesses the language, unless somebody chose it. */
  const handleCountryCode = (next: string) => {
    setCountryCode(next)
    if (!languageChosen) {
      setLanguage(languageForCountryCode(next))
    }
  }

  const busy = saving || deleting || settingStatus

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>
            {mode === 'create' ? 'Nueva cita' : 'Editar cita'}
          </DialogTitle>
          <DialogDescription>
            {mode === 'create'
              ? 'Agenda una cita nueva'
              : 'Cambia los datos de la cita'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {/*
            Only once the appointment exists. A status set against a blank
            id would be written to nothing, and "pending" is what saving it
            will produce anyway.
          */}
          {mode === 'edit' && event?.id && (
            <div className="space-y-2">
              <Label>Estado</Label>
              <div className="space-y-3 rounded-lg bg-muted/50 p-3 ring-1 ring-border">
                <p className="text-sm">{statusLine(event)}</p>
                <div className="flex flex-wrap gap-2">
                  {/*
                    Only the moves that change something. Proposed is not
                    offered at all: it is the assistant saying it is unsure,
                    and a person at the desk is never unsure in that way.
                  */}
                  {event.status !== 'confirmed' && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => handleStatus('confirmed')}
                      disabled={busy}
                    >
                      Marcar confirmada
                    </Button>
                  )}
                  {event.status !== 'cancelled' && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => handleStatus('cancelled')}
                      disabled={busy}
                    >
                      Marcar cancelada
                    </Button>
                  )}
                </div>
              </div>
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="client-name">Nombre del cliente *</Label>
            <Input
              id="client-name"
              value={clientName}
              onChange={(e) => setClientName(e.target.value)}
              placeholder="p. ej. Ana Ruiz"
            />
          </div>

          <div className="grid grid-cols-[auto_1fr] gap-2">
            <div className="space-y-2">
              <Label htmlFor="country-code">Código de país</Label>
              <select
                id="country-code"
                value={countryCode}
                onChange={(e) => handleCountryCode(e.target.value)}
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
              <Label htmlFor="phone">Número de teléfono</Label>
              <Input
                id="phone"
                type="text"
                placeholder="5512345678"
                value={phoneNumber}
                onChange={(e) => setPhoneNumber(e.target.value)}
              />
            </div>
          </div>

          {/*
            Under the phone fields because that is where its default comes
            from, and visible rather than hidden behind an "advanced" toggle:
            a reminder in a language the patient does not read is not an edge
            case on this list, it is half of it.
          */}
          <div className="space-y-2">
            <Label htmlFor="reminder-language">Idioma del recordatorio</Label>
            <select
              id="reminder-language"
              value={language}
              onChange={(e) => {
                setLanguage(e.target.value)
                setLanguageChosen(true)
              }}
              className="h-9 w-full rounded-3xl border border-transparent bg-input/50 px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30"
            >
              {REMINDER_LANGUAGES.map((option) => (
                <option key={option.code} value={option.code}>
                  {option.label}
                </option>
              ))}
            </select>
            <p className="text-xs text-muted-foreground">
              En el que se le escribe al cliente. Lo adivinamos por la lada;
              cámbialo si no es el suyo.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="start-date">Fecha de inicio *</Label>
              <Input
                id="start-date"
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="start-time">Hora de inicio *</Label>
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
              <Label htmlFor="end-date">Fecha de fin *</Label>
              <Input
                id="end-date"
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="end-time">Hora de fin *</Label>
              <Input
                id="end-time"
                type="time"
                value={endTime}
                onChange={(e) => setEndTime(e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="location">Lugar</Label>
            <Input
              id="location"
              value={location}
              onChange={(e) => setLocation(e.target.value)}
              placeholder="p. ej. Consultorio 2"
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
            <Label htmlFor="notes">Notas</Label>
            <Textarea
              id="notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="p. ej. Alérgico al látex"
              rows={3}
            />
            <p className="text-xs text-muted-foreground">
              Solo para tu equipo. El cliente nunca lo ve.
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
                  Eliminando...
                </>
              ) : (
                'Eliminar'
              )}
            </Button>
          )}
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button type="button" onClick={handleSave} disabled={busy}>
            {saving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Guardando...
              </>
            ) : (
              'Guardar'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
