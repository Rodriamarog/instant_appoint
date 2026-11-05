import { Check, Eye, PowerOff, Sparkles, X, type LucideIcon } from 'lucide-react'

import { AI_MODE_OPTIONS, type AIMode } from '@/lib/business-settings'
import { cn } from '@/lib/utils'

/**
 * The switch that decides whether a clinic's patients are answered by the
 * assistant, and how much it is allowed to do when they are.
 *
 * It owns no state and saves nothing: the Asistente page it sits on —
 * pages/assistant.tsx — holds the settings and does the saving.
 *
 * Three cards side by side rather than a stacked list of paragraphs. Each
 * position is a different promise to a patient, so the differences have to
 * be readable at a glance: every card carries the same short checklist, and
 * the comparison is made down a row of ticks and crosses rather than by
 * reading three paragraphs. The chosen position's full wording sits under
 * the cards, where it reads as "this is what you have switched on".
 */
const LOOK: Record<
  AIMode,
  { icon: LucideIcon; tagline: string; answers: boolean; books: boolean }
> = {
  off: { icon: PowerOff, tagline: 'Todo lo atiendes tú', answers: false, books: false },
  observer: {
    icon: Eye,
    tagline: 'Tú contestas, él agenda',
    answers: false,
    books: true,
  },
  autopilot: {
    icon: Sparkles,
    tagline: 'Contesta y agenda por ti',
    answers: true,
    books: true,
  },
}

export function AIModeField({
  value,
  onChange,
}: {
  value: AIMode
  onChange: (mode: AIMode) => void
}) {
  // Compared rather than defaulted, so a value this build has no card for —
  // a position added later, a row edited by hand — leaves every card
  // unchecked instead of reporting a state the clinic is not in.
  const chosen = AI_MODE_OPTIONS.find((option) => option.mode === value)

  return (
    <fieldset className="space-y-4">
      <legend className="sr-only">El asistente de IA</legend>

      <div
        role="radiogroup"
        aria-label="El asistente de IA"
        className="grid gap-3 sm:grid-cols-3"
      >
        {AI_MODE_OPTIONS.map((option) => {
          const look = LOOK[option.mode]
          const Icon = look.icon
          const on = option === chosen

          return (
            <button
              key={option.mode}
              type="button"
              role="radio"
              aria-checked={on}
              onClick={() => onChange(option.mode)}
              className={cn(
                'group relative flex cursor-pointer flex-col rounded-[14px] border bg-background p-4 text-left transition-all',
                on
                  ? 'border-brand bg-accent/60 shadow-[0_0_0_1px_var(--brand)]'
                  : 'border-border hover:border-brand/40 hover:bg-muted/50',
              )}
            >
              <span className="flex items-center justify-between">
                <span
                  className={cn(
                    'grid size-9 place-items-center rounded-[10px] transition-colors',
                    on ? 'bg-brand text-brand-foreground' : 'bg-muted text-muted-foreground',
                  )}
                >
                  <Icon className="size-[18px]" />
                </span>
                <span
                  aria-hidden
                  className={cn(
                    'grid size-5 place-items-center rounded-full border-2 transition-colors',
                    on ? 'border-brand' : 'border-border group-hover:border-brand/40',
                  )}
                >
                  {on && <span className="size-2.5 rounded-full bg-brand" />}
                </span>
              </span>

              <span className="mt-4 block text-[15px] font-semibold">{option.label}</span>
              <span className="mt-0.5 block text-xs text-muted-foreground">{look.tagline}</span>

              <span className="mt-4 block space-y-1.5 border-t pt-3 text-xs">
                <Can yes={look.answers}>Contesta a tus pacientes</Can>
                <Can yes={look.books}>Agenda las citas acordadas</Can>
                <Can yes>Guarda las conversaciones</Can>
              </span>
            </button>
          )
        })}
      </div>

      {chosen && (
        <p
          aria-live="polite"
          className="rounded-[12px] bg-muted px-4 py-3 text-sm leading-relaxed text-muted-foreground"
        >
          <span className="font-semibold text-foreground">{chosen.label}:</span>{' '}
          {chosen.help}
        </p>
      )}
    </fieldset>
  )
}

function Can({ yes, children }: { yes: boolean; children: React.ReactNode }) {
  const Mark = yes ? Check : X

  return (
    <span className={cn('flex items-center gap-2', yes ? 'text-foreground' : 'text-muted-foreground')}>
      <Mark
        aria-label={yes ? 'Sí' : 'No'}
        className={cn('size-3.5 shrink-0', yes ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground/60')}
      />
      <span className={cn(!yes && 'line-through decoration-muted-foreground/40')}>{children}</span>
    </span>
  )
}
