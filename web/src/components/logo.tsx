import { cn } from '@/lib/utils'

/**
 * The wordmark, the same one the landing page uses: InstantAppoint in the
 * display face, with Appoint in the brand blue.
 */
export function Logo({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'font-heading text-[19px] font-extrabold tracking-[-0.035em] text-foreground',
        className,
      )}
    >
      Instant<b className="font-[inherit] text-brand">Appoint</b>
    </span>
  )
}
