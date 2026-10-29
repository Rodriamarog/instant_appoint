import { Link } from 'react-router'

/**
 * The server answers every extensionless path with the app shell, so an
 * unknown URL lands here rather than on a server 404.
 */
export function NotFoundPage() {
  return (
    <section className="space-y-3">
      <h1 className="font-heading text-2xl font-semibold">Page not found</h1>
      <p className="text-muted-foreground">
        There is nothing at this address.{' '}
        <Link to="/" className="underline">
          Go home
        </Link>
        .
      </p>
    </section>
  )
}
