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
 * send. A top-level tab of its own, beside Inicio.
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

  return (
    <div className="mx-auto max-w-3xl space-y-8">
      <div>
        <h1 className="font-heading text-3xl font-bold tracking-[-0.03em]">Asistente</h1>
        <p className="mt-1.5 text-muted-foreground">
          Quién contesta los mensajes de tus pacientes, y qué puede hacer el asistente por su
          cuenta.
        </p>
      </div>

      {!settings ? (
        <p className="text-sm text-muted-foreground" role="status">
          {error || 'Cargando la configuración de tu asistente...'}
        </p>
      ) : (
        <>
          <section aria-labelledby="modo">
            <h2 id="modo" className="mb-1 text-sm font-semibold">
              ¿Qué hace el asistente?
            </h2>
            <p className="mb-4 text-sm text-muted-foreground">
              Con los mensajes que te mandan tus pacientes por WhatsApp.
            </p>
            <AIModeField value={settings.ai_mode} onChange={(ai_mode) => change({ ai_mode })} />
          </section>

          {/* Below the mode and not above it: this is a control over something
              the assistant only does once the mode has let it write, so the
              decision it qualifies has to have been read first. */}
          <section aria-labelledby="por-su-cuenta">
            <h2 id="por-su-cuenta" className="mb-3 text-sm font-semibold">
              Mensajes que manda por su cuenta
            </h2>
            <div className="rounded-[14px] bg-background p-5 ring-1 ring-border">
              <PatientOutreachField
                value={settings.patient_outreach}
                onChange={(patient_outreach) => change({ patient_outreach })}
                assistantOff={settings.ai_mode === 'off'}
              />
            </div>
          </section>

          {error && <p className="text-sm text-destructive">{error}</p>}
        </>
      )}
    </div>
  )
}
