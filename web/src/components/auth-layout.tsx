import { ArrowLeft } from 'lucide-react'
import type { CSSProperties, ReactNode } from 'react'
import { Link } from 'react-router'

import { Logo } from '@/components/logo'
import { StatusPill } from '@/components/status-pill'

/**
 * The frame both ways in (Iniciar sesión, Crear cuenta) sit inside.
 *
 * The form takes the left half and nothing competes with it. The right half,
 * from a laptop up, repeats the landing page's promise and its day view, so
 * someone arriving from `/` sees the same product they were just sold rather
 * than a generic grey login box. On a phone that half is dropped entirely:
 * there it would only push the form below the fold.
 */
export function AuthLayout({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle: ReactNode
  children: ReactNode
}) {
  return (
    <div className="grid min-h-screen bg-background lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)]">
      <div className="flex flex-col px-4 py-5 sm:px-10 sm:py-7">
        <header className="flex items-center justify-between gap-4">
          <Link to="/" aria-label="InstantAppoint, página principal">
            <Logo />
          </Link>
          <Link
            to="/"
            className="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            <ArrowLeft className="size-4" />
            Volver
          </Link>
        </header>

        <main className="flex flex-1 items-center justify-center py-12">
          <div className="w-full max-w-[400px] motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-3 motion-safe:duration-700">
            <h1 className="font-heading text-[34px] leading-[1.05] font-bold tracking-[-0.035em] sm:text-[40px]">
              {title}
            </h1>
            <p className="mt-3 text-[15px] leading-relaxed text-muted-foreground">
              {subtitle}
            </p>
            <div className="mt-8">{children}</div>
          </div>
        </main>

        <footer className="flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground">
          <span>© {new Date().getFullYear()} InstantAppoint</span>
          <Link to="/politica-de-privacidad" className="hover:text-foreground">
            Privacidad
          </Link>
          <Link to="/terminos-de-servicio" className="hover:text-foreground">
            Términos
          </Link>
        </footer>
      </div>

      <Showcase />
    </div>
  )
}

/** The fields' shared spacing, so both forms read down the same rhythm. */
export function AuthField({ children }: { children: ReactNode }) {
  return <div className="space-y-2">{children}</div>
}

/* ------------------------------------------------------------------ */

const DAY = [
  { time: '09:00', name: 'Ana López', what: 'Limpieza', confirmed: true },
  { time: '10:00', name: 'Carlos Ruiz', what: 'Revisión', confirmed: true },
  { time: '11:30', name: 'María Fernández', what: 'Primera consulta', confirmed: false },
  { time: '12:30', name: 'Diego Torres', what: 'Ortodoncia', confirmed: true },
]

/**
 * The landing page's closing line and a slice of its day view. Decorative:
 * the names are invented, so it is hidden from assistive technology rather
 * than read out as somebody's schedule.
 */
function Showcase() {
  const confirmed = DAY.filter((d) => d.confirmed).length

  return (
    <aside
      aria-hidden
      className="relative m-3 hidden overflow-hidden rounded-[22px] bg-muted lg:flex lg:flex-col lg:justify-center"
    >
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(80%_60%_at_50%_0%,var(--accent)_0%,transparent_70%)]" />

      <div className="relative mx-auto w-full max-w-[520px] px-10 py-16">
        <p className="font-heading text-[40px] leading-[1.04] font-bold tracking-[-0.04em] xl:text-[48px]">
          <span className="block">Sigue trabajando como siempre.</span>
          <span className="block text-brand">Del resto nos encargamos.</span>
        </p>

        <div className="mt-12 overflow-hidden rounded-[14px] bg-background shadow-[0_0_0_1px_var(--border),0_40px_70px_-45px_rgba(11,18,51,0.35)]">
          <div className="flex h-9 items-center gap-1.5 border-b bg-muted px-3.5">
            <i className="size-2.5 rounded-full bg-border" />
            <i className="size-2.5 rounded-full bg-border" />
            <i className="size-2.5 rounded-full bg-border" />
          </div>
          <div className="space-y-4 p-5">
            <div className="flex items-baseline justify-between gap-2">
              <span className="font-heading text-lg font-bold tracking-tight">Hoy</span>
              <span className="text-sm text-muted-foreground">
                {confirmed} de {DAY.length} confirmadas
              </span>
            </div>
            <ul className="divide-y rounded-[10px] ring-1 ring-border">
              {DAY.map((row, i) => (
                <li
                  key={row.time}
                  style={{ animationDelay: `${300 + i * 120}ms` } as CSSProperties}
                  className="flex items-center gap-4 px-4 py-3 motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-2 motion-safe:duration-700 motion-safe:fill-mode-both"
                >
                  <span className="w-12 shrink-0 text-sm font-medium text-muted-foreground tabular-nums">
                    {row.time}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[15px] font-semibold">{row.name}</span>
                    <span className="block truncate text-xs text-muted-foreground">{row.what}</span>
                  </span>
                  <StatusPill tone={row.confirmed ? 'settled' : 'waiting'}>
                    {row.confirmed ? 'Confirmada' : 'Por confirmar'}
                  </StatusPill>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </aside>
  )
}
