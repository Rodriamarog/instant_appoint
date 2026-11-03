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
    <section className="space-y-3">
      <h1 className="font-heading text-2xl font-semibold">
        Página no encontrada
      </h1>
      <p className="text-muted-foreground">
        No hay nada en esta dirección.{' '}
        <Link to={start} className="underline">
          Ir al inicio
        </Link>
        .
      </p>
    </section>
  )
}
