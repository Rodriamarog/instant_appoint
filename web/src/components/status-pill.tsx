import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * How sure the clinic can be about an appointment. The landing page's day
 * view draws these two ways only, settled and not yet, and this keeps to it:
 * blue is reserved for the one answer a receptionist is looking for.
 */
export type PillTone = 'settled' | 'waiting' | 'unsure' | 'off'

const TONES: Record<PillTone, string> = {
  settled: 'bg-accent text-accent-foreground',
  waiting: 'bg-muted text-muted-foreground ring-1 ring-inset ring-border',
  unsure: 'border border-dashed border-[#cfd6e6] dark:border-[#3a4670] bg-background text-muted-foreground',
  off: 'bg-muted text-muted-foreground/80',
}

export function StatusPill({
  tone,
  children,
  className,
}: {
  tone: PillTone
  children: ReactNode
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-flex h-6 shrink-0 items-center rounded-full px-2.5 text-[11.5px] font-semibold whitespace-nowrap',
        TONES[tone],
        className,
      )}
    >
      {children}
    </span>
  )
}
