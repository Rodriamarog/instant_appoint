import { Link } from 'react-router'

import { Button } from '@/components/ui/button'

/**
 * The public landing page, served at `/`.
 *
 * This is the first thing anyone sees signed out — Meta's app review
 * included, which fetches `/` without a session — so it carries the welcome,
 * the two ways in, and the two legal pages that review also wants reachable.
 *
 * It is a port of the splash this app shipped with, not a copy of it: the
 * welcome and the two routes are the same, the next/image logo beside them is
 * not. `public/next.svg` went with the rest of Next, and this app was never
 * Next.js as far as a visitor was concerned.
 */
export function HomePage() {
  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-center px-6 py-20">
        <p className="font-heading text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          InstantAppoint
        </p>

        <h1 className="font-heading mt-4 max-w-lg text-4xl font-semibold tracking-tight">
          Appointments, and the WhatsApp reminders that go with them.
        </h1>

        <p className="mt-5 max-w-xl text-lg leading-8 text-muted-foreground">
          A calendar, a connected WhatsApp Business number, and a reminder
          before every appointment — in one place. Sign in, or create an
          account to get started.
        </p>

        <div className="mt-10 flex flex-col gap-3 sm:flex-row">
          <Button asChild size="lg">
            <Link to="/login">Sign In</Link>
          </Button>
          <Button asChild size="lg" variant="outline">
            <Link to="/register">Create Account</Link>
          </Button>
        </div>
      </main>

      <footer className="border-t border-border">
        <div className="mx-auto flex w-full max-w-3xl flex-wrap gap-x-6 gap-y-2 px-6 py-6 text-sm text-muted-foreground">
          <Link to="/politica-de-privacidad" className="underline hover:text-foreground">
            Política de privacidad
          </Link>
          <Link to="/terminos-de-servicio" className="underline hover:text-foreground">
            Términos de servicio
          </Link>
        </div>
      </footer>
    </div>
  )
}
