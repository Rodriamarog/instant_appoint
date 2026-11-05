import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { TIMEZONE_OPTIONS, type BusinessSettings } from '@/lib/business-settings'
import { errorMessage } from '@/lib/errors'

/**
 * The business itself: its name, and the zone its day runs in.
 *
 * The zone used to sit in the reminder settings, but it decides more than a
 * reminder's wording — it is where the Agenda's day starts and ends — so it
 * belongs to the business. The name lives on the account (users.business_name),
 * the zone on the settings row; one Guardar writes whichever changed.
 */
export function BusinessForm({
  businessName,
  settings,
  onSaveName,
  onSaveSettings,
}: {
  businessName: string
  settings: BusinessSettings
  onSaveName: (name: string) => Promise<void>
  onSaveSettings: (patch: Partial<BusinessSettings>) => Promise<void>
}) {
  const [name, setName] = useState(businessName)
  const [timezone, setTimezone] = useState(settings.timezone)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => setName(businessName), [businessName])
  useEffect(() => setTimezone(settings.timezone), [settings.timezone])

  // A zone already saved that is not on the short list is kept rather than
  // silently swapped for the first one that is.
  const zones = TIMEZONE_OPTIONS.includes(timezone) ? TIMEZONE_OPTIONS : [timezone, ...TIMEZONE_OPTIONS]

  const nameChanged = name.trim() !== businessName
  const zoneChanged = timezone !== settings.timezone
  const dirty = nameChanged || zoneChanged

  const save = async () => {
    setIsSaving(true)
    setError('')
    try {
      if (nameChanged) {
        await onSaveName(name.trim())
      }
      if (zoneChanged) {
        await onSaveSettings({ timezone })
      }
      setSaved(true)
    } catch (err) {
      setError(errorMessage(err, 'No se pudieron guardar los datos del negocio.'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <Label htmlFor="business-name">Nombre del negocio</Label>
        <Input
          id="business-name"
          value={name}
          onChange={(event) => {
            setName(event.target.value)
            setSaved(false)
          }}
          placeholder="Clínica Dental Sonrisas"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="business-timezone">Zona horaria</Label>
        <select
          id="business-timezone"
          value={timezone}
          onChange={(event) => {
            setTimezone(event.target.value)
            setSaved(false)
          }}
          className="h-11 w-full rounded-[10px] border border-input bg-background px-3 text-sm"
        >
          {zones.map((zone) => (
            <option key={zone} value={zone}>
              {zone.replace('_', ' ')}
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">
          Decide dónde empieza y termina tu día en la agenda, y en qué hora se escriben la fecha
          y la hora de cada recordatorio.
        </p>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      <div className="flex items-center justify-end gap-3">
        {saved && !dirty && <span className="text-sm text-muted-foreground">Guardado</span>}
        <Button onClick={save} disabled={isSaving || !dirty}>
          {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Guardar
        </Button>
      </div>
    </div>
  )
}
