import { useEffect, useState } from 'react'

import { AIModeField } from '@/components/settings/ai-mode-field'
import { PatientOutreachField } from '@/components/settings/patient-outreach-field'
import {
  loadBusinessSettings,
  saveBusinessSettings,
  type BusinessSettings,
} from '@/lib/business-settings'
import { errorMessage } from '@/lib/errors'

/**
 * What the assistant is allowed to do with the messages a clinic's patients
 * send, and the first thing Configuración opens on.
 *
 * It is first because it is the decision with a patient on the other end of
 * it: the reminder settings and the templates decide how a message is worded,
 * this decides whether a person or a model writes it.
 *
 * Unlike every other setting in here there is no Save button. The control is
 * one choice out of three with nothing else on the page to coordinate with,
 * and a switch that needs confirming is a switch somebody leaves half-thrown.
 */
export function AssistantPage() {
  // null until the row is read, so no button is shown as chosen before it is
  // known which one is — the one state this page must never fake, because it
  // would be read as "the assistant is off" by a clinic whose assistant is on.
  const [settings, setSettings] = useState<BusinessSettings | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    loadBusinessSettings()
      .then(setSettings)
      .catch((err: unknown) =>
        setError(errorMessage(err, 'No se pudo leer la configuración de tu asistente.')),
      )
  }, [])

  // One saver for both controls rather than one each. They write two
  // columns of the same row through the same single-row save, so two
  // handlers would be two copies of the move-first-and-put-back rule, and
  // the copy that drifted would be the one that leaves a switch showing a
  // change nothing wrote.
  const change = async (patch: Partial<BusinessSettings>) => {
    if (!settings) {
      return
    }

    const before = settings
    // Moved first and put back on failure. The alternative is a control that
    // does not move until a round trip finishes, which reads as a dead
    // button and gets clicked again.
    setSettings({ ...settings, ...patch })
    setError('')

    try {
      setSettings(await saveBusinessSettings({ ...before, ...patch }))
    } catch (err) {
      setSettings(before)
      setError(errorMessage(err, 'No se pudo guardar el cambio. Inténtalo de nuevo.'))
    }
  }

  if (!settings) {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        {error || 'Cargando la configuración de tu asistente...'}
      </p>
    )
  }

  return (
    <section className="max-w-xl space-y-6">
      <AIModeField value={settings.ai_mode} onChange={(ai_mode) => change({ ai_mode })} />

      {/* Below the mode and not above it: this is a control over something
          the assistant only does once the mode has let it write, so the
          decision it qualifies has to have been read first. */}
      <PatientOutreachField
        value={settings.patient_outreach}
        onChange={(patient_outreach) => change({ patient_outreach })}
      />

      {error && <p className="text-sm text-destructive">{error}</p>}
    </section>
  )
}
