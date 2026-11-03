import { AI_MODE_OPTIONS, type AIMode } from '@/lib/business-settings'

/**
 * The switch that decides whether a clinic's patients are answered by the
 * assistant, and how much it is allowed to do when they are.
 *
 * It is a component of its own, and not part of the reminder settings dialog
 * it currently appears in, because it does not belong there: #21 builds the
 * Configuración page the issue names, and this is meant to be moved into it
 * by deleting one line from that dialog and writing the same line there. It
 * owns no state and saves nothing, so the move carries no behaviour with it.
 *
 * Radio buttons rather than a select, because the three positions are not
 * interchangeable settings of one kind: each one is a different promise to a
 * patient, and all three have to be readable at once for the choice between
 * them to be an informed one.
 */
export function AIModeField({
  value,
  onChange,
}: {
  value: AIMode
  onChange: (mode: AIMode) => void
}) {
  return (
    <fieldset className="space-y-3">
      <legend className="text-sm font-medium">El asistente de IA</legend>
      <p className="text-xs text-muted-foreground">
        Qué puede hacer el asistente con los mensajes que te mandan tus pacientes.
      </p>

      <div role="radiogroup" aria-label="El asistente de IA" className="space-y-2">
        {AI_MODE_OPTIONS.map((option) => {
          // Compared rather than defaulted, so a value this build has no
          // button for — a position added later, a row edited by hand —
          // leaves every button unchecked instead of reporting a state the
          // clinic is not in.
          const chosen = option.mode === value

          return (
            <button
              key={option.mode}
              type="button"
              role="radio"
              aria-checked={chosen}
              onClick={() => onChange(option.mode)}
              className={`w-full rounded-lg border p-3 text-left transition-colors ${
                chosen
                  ? 'border-primary bg-primary/5'
                  : 'border-border hover:border-foreground/40'
              }`}
            >
              <span className="block text-sm font-medium">{option.label}</span>
              <span className="mt-0.5 block text-xs text-muted-foreground">
                {option.help}
              </span>
            </button>
          )
        })}
      </div>
    </fieldset>
  )
}
