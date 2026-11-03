import { Link } from 'react-router'

/**
 * The server answers every extensionless path with the app shell, so an
 * unknown URL lands here rather than on a server 404.
 */
export function NotFoundPage() {
  return (
    <section className="space-y-3">
      <h1 className="font-heading text-2xl font-semibold">
        Página no encontrada
      </h1>
      <p className="text-muted-foreground">
        No hay nada en esta dirección.{' '}
        <Link to="/" className="underline">
          Ir al inicio
        </Link>
        .
      </p>
    </section>
  )
}
