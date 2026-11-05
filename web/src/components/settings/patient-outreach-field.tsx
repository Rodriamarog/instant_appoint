import { messagesPatients, type PatientOutreach } from '@/lib/business-settings'

/**
 * The switch that decides whether the assistant may write to a patient who
 * has not just written in.
 *
 * It sits beside the mode switch because the two are read together and
 * neither is guessable from the other: Observador promises the assistant
 * does not answer anybody, and a clinic reading that would not expect the
 * assistant to send a message of its own the moment it writes an
 * appointment. That message is what this turns off.
 *
 * It owns no state and saves nothing — pages/assistant.tsx holds the
 * settings and does the saving, as it does for the mode.
 *
 * A switch rather than two radios: unlike the three modes, which are three
 * different promises that have to be readable side by side, this is one
 * thing that is either happening or not.
 */
export function PatientOutreachField({
  value,
  onChange,
}: {
  value: PatientOutreach
  onChange: (outreach: PatientOutreach) => void
}) {
  // Read through the same rule internal/webhook applies, so the switch shows
  // what the assistant will actually do. A row written before this column
  // existed, or holding a word this build does not know, still produces the
  // receipt — showing it as off would tell a clinic its patients hear
  // nothing while they are still being messaged.
  const on = messagesPatients(value)

  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-semibold">Mensajes que el asistente manda por su cuenta</legend>
      <p className="text-xs text-muted-foreground">
        Hoy es uno solo: cuando el asistente agenda una cita a partir de una conversación, le
        manda al paciente lo que anotó para que lo confirme o lo cancele.
      </p>

      <button
        type="button"
        role="switch"
        aria-checked={on}
        onClick={() => onChange(on ? 'off' : 'on')}
        className={`w-full cursor-pointer rounded-[12px] border p-4 text-left transition-colors ${
          on ? 'border-brand bg-accent ring-1 ring-brand' : 'border-border hover:border-brand/40 hover:bg-muted'
        }`}
      >
        <span className="block text-[15px] font-semibold">
          {on
            ? 'El asistente le pide al paciente que confirme la cita'
            : 'El asistente no le escribe al paciente por su cuenta'}
        </span>
        <span className="mt-0.5 block text-xs text-muted-foreground">
          {on
            ? 'Así te enteras de inmediato si el asistente anotó un día o una hora que el paciente no acordó.'
            : 'Las citas que el asistente agende no se le confirman a nadie. Revísalas tú en el calendario.'}
        </span>
      </button>

      {/* Said here rather than left to be discovered, because both are
          messages a patient receives and a clinic turning this off is
          entitled to know which ones keep going out. */}
      <p className="text-xs text-muted-foreground">
        Esto no apaga tus recordatorios, que siguen saliendo como los tengas configurados, ni las
        respuestas del piloto automático a los pacientes que te escriben.
      </p>
    </fieldset>
  )
}
