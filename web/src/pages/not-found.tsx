import { Link } from 'react-router'

import { useIsSignedIn } from '@/lib/auth'

/**
 * The server answers every extensionless path with the app shell, so an
 * unknown URL lands here rather than on a server 404.
 *
 * Where "the start" is depends on who is reading: a signed-in person who
 * followed a stale bookmark wants their own day back, and the landing page
 * would offer them a sign-in button they do not need.
 */
export function NotFoundPage() {
  const start = useIsSignedIn() ? '/inicio' : '/'

  return (
    <section className="mx-auto max-w-xl space-y-3 px-4 py-24 text-center">
      <p className="text-sm font-semibold text-brand">404</p>
      <h1 className="font-heading text-4xl font-bold tracking-[-0.035em]">
        Página no encontrada
      </h1>
      <p className="text-muted-foreground">
        No hay nada en esta dirección.{' '}
        <Link to={start} className="font-semibold text-brand hover:underline">
          Ir al inicio
        </Link>
        .
      </p>
    </section>
  )
}
