import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'

import { useTheme, type ThemePreference } from '@/lib/theme'
import { cn } from '@/lib/utils'

const OPTIONS: { value: ThemePreference; label: string; icon: LucideIcon }[] = [
  { value: 'light', label: 'Claro', icon: Sun },
  { value: 'dark', label: 'Oscuro', icon: Moon },
  { value: 'system', label: 'Sistema', icon: Monitor },
]

export function ThemeField() {
  const { preference, setPreference } = useTheme()

  return (
    <div role="radiogroup" aria-label="Tema" className="grid grid-cols-3 gap-3">
      {OPTIONS.map(({ value, label, icon: Icon }) => {
        const on = preference === value
        return (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => setPreference(value)}
            className={cn(
              'flex cursor-pointer flex-col items-center gap-2 rounded-[12px] border p-4 text-sm font-medium transition-colors',
              on
                ? 'border-brand bg-accent text-accent-foreground ring-1 ring-brand'
                : 'border-border text-muted-foreground hover:border-brand/40 hover:text-foreground',
            )}
          >
            <Icon className="size-5" />
            {label}
          </button>
        )
      })}
    </div>
  )
}
