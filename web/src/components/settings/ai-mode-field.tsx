import { AI_MODE_OPTIONS, type AIMode } from '@/lib/business-settings'

/**
 * The switch that decides whether a clinic's patients are answered by the
 * assistant, and how much it is allowed to do when they are.
 *
 * It owns no state and saves nothing: the Configuración page it sits on —
 * pages/assistant.tsx — holds the settings and does the saving. That is what
 * let it move there out of the reminder settings dialog it was first written
 * in, as one line deleted and the same line written elsewhere, with no
 * behaviour travelling with it.
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
      <legend className="font-heading text-lg font-bold tracking-tight">El asistente de IA</legend>
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
              className={`w-full cursor-pointer rounded-[12px] border p-4 text-left transition-colors ${
                chosen
                  ? 'border-brand bg-accent ring-1 ring-brand'
                  : 'border-border hover:border-brand/40 hover:bg-muted'
              }`}
            >
              <span className="block text-[15px] font-semibold">{option.label}</span>
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
