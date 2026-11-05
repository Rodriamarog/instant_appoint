import { ChevronDown, type LucideIcon } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router'

import { cn } from '@/lib/utils'

/**
 * The building blocks Configuración and Integraciones share: a small heading
 * over one bordered card, and inside it rows that open in place.
 */
export function SettingsGroup({
  heading,
  children,
}: {
  heading: string
  children: ReactNode
}) {
  const id = `grupo-${heading.toLowerCase().replace(/\s+/g, '-')}`

  return (
    <section aria-labelledby={id}>
      <h2 id={id} className="mb-3 text-sm font-semibold text-foreground">
        {heading}
      </h2>
      <div className="overflow-hidden rounded-[14px] bg-background ring-1 ring-border">
        {children}
      </div>
    </section>
  )
}

export function SettingsRow({
  id,
  icon: Icon,
  title,
  subtitle,
  trailing,
  open,
  onToggle,
  disabled = false,
  children,
}: {
  id: string
  icon: LucideIcon
  title: string
  subtitle: string
  trailing?: ReactNode
  open: boolean
  onToggle: () => void
  /** A row for something not available yet: shown, never opened. */
  disabled?: boolean
  children?: ReactNode
}) {
  const panel = `${id}-panel`

  return (
    <div id={id} className="scroll-mt-24 border-b border-border last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        disabled={disabled}
        aria-expanded={disabled ? undefined : open}
        aria-controls={disabled ? undefined : panel}
        className={cn(
          'flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors',
          disabled ? 'cursor-default' : 'cursor-pointer hover:bg-muted/60',
        )}
      >
        <span
          className={cn(
            'grid size-8 shrink-0 place-items-center rounded-[9px] bg-muted text-muted-foreground',
            disabled && 'opacity-60',
          )}
        >
          <Icon className="size-4" />
        </span>
        <span className={cn('min-w-0 flex-1', disabled && 'opacity-60')}>
          <span className="block text-sm font-medium text-foreground">{title}</span>
          <span className="block truncate text-xs text-muted-foreground">{subtitle}</span>
        </span>
        {trailing}
        {!disabled && (
          <ChevronDown
            className={cn(
              'size-4 shrink-0 text-muted-foreground transition-transform',
              open && 'rotate-180',
            )}
          />
        )}
      </button>
      {open && !disabled && (
        <div id={panel} className="border-t border-border px-4 pb-5 pt-4">
          {children}
        </div>
      )}
    </div>
  )
}

/**
 * Which rows are open, seeded from the path: /integraciones/plantillas opens
 * the Plantillas row and scrolls to it, whether arrived at by bookmark or by
 * a link inside another row.
 */
export function useOpenRows(base: string, ids: string[]) {
  const { pathname } = useLocation()
  const [open, setOpen] = useState<Set<string>>(() => new Set())
  const key = ids.join('|')

  useEffect(() => {
    const rest = pathname.replace(/\/$/, '').slice(base.length + 1)
    if (!key.split('|').includes(rest)) {
      return
    }
    setOpen((current) => new Set(current).add(rest))
    document.getElementById(rest)?.scrollIntoView?.({ block: 'start' })
  }, [pathname, base, key])

  const toggle = (id: string) =>
    setOpen((current) => {
      const next = new Set(current)
      if (!next.delete(id)) {
        next.add(id)
      }
      return next
    })

  return { isOpen: (id: string) => open.has(id), toggle }
}
