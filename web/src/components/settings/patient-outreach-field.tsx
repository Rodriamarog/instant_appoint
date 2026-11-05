import { Info } from 'lucide-react'

import { messagesPatients, type PatientOutreach } from '@/lib/business-settings'
import { cn } from '@/lib/utils'

/**
 * The switch that decides whether the assistant may write to a patient who
 * has not just written in. Today that is one message: when it books an
 * appointment out of a conversation, it sends the patient what it wrote down
 * so they can confirm or cancel it.
 *
 * It owns no state and saves nothing — pages/assistant.tsx holds the
 * settings and does the saving, as it does for the mode.
 *
 * A real toggle on the right of a settings row, rather than a card that
 * changes its wording: a card that rewrites itself when clicked reads as a
 * selected option, not as something that is on or off.
 */
export function PatientOutreachField({
  value,
  onChange,
  assistantOff = false,
}: {
  value: PatientOutreach
  onChange: (outreach: PatientOutreach) => void
  /** The mode is Apagado, so nothing is sent whatever this says. */
  assistantOff?: boolean
}) {
  // Read through the same rule internal/webhook applies, so the switch shows
  // what the assistant will actually do. A row written before this column
  // existed, or holding a word this build does not know, still produces the
  // receipt — showing it as off would tell a clinic its patients hear
  // nothing while they are still being messaged.
  const on = messagesPatients(value)

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-6">
        <div className="min-w-0">
          <p id="outreach-title" className="text-[15px] font-semibold">
            Pedir confirmación al agendar
          </p>
          <p id="outreach-help" className="mt-1 text-sm leading-relaxed text-muted-foreground">
            Cuando el asistente agenda una cita a partir de una conversación, le manda al
            paciente lo que anotó para que lo confirme o lo cancele. Así te enteras de inmediato
            si anotó un día o una hora que el paciente no acordó.
          </p>
        </div>

        <button
          type="button"
          role="switch"
          aria-checked={on}
          aria-labelledby="outreach-title"
          aria-describedby="outreach-help"
          onClick={() => onChange(on ? 'off' : 'on')}
          className={cn(
            'relative mt-0.5 inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand',
            on ? 'bg-brand' : 'bg-border',
          )}
        >
          <span
            aria-hidden
            className={cn(
              'inline-block size-5 rounded-full bg-white shadow-sm transition-transform',
              on ? 'translate-x-[22px]' : 'translate-x-0.5',
            )}
          />
        </button>
      </div>

      {assistantOff && (
        <p className="text-xs font-medium text-amber-700 dark:text-amber-400">
          El asistente está apagado, así que por ahora no se manda nada. Esto aplica en cuanto
          lo enciendas.
        </p>
      )}

      {/* Said here rather than left to be discovered, because both are
          messages a patient receives and a clinic turning this off is
          entitled to know which ones keep going out. */}
      <p className="flex gap-2 border-t pt-3 text-xs leading-relaxed text-muted-foreground">
        <Info className="mt-px size-3.5 shrink-0" />
        <span>
          Esto no apaga tus recordatorios, que siguen saliendo como los tengas configurados, ni
          las respuestas del piloto automático a los pacientes que te escriben.
        </span>
      </p>
    </div>
  )
}
