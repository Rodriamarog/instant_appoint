import { Link, Outlet, useLocation } from 'react-router'

const links = [
  { to: '/', label: 'Home' },
  { to: '/calendar', label: 'Calendar' },
]

/**
 * The layout every app route renders inside. The real navigation and the auth
 * guard arrive with the page ports.
 */
export function Shell() {
  const { pathname } = useLocation()

  return (
    <div className="flex min-h-screen flex-col">
      <header className="border-b">
        <nav className="mx-auto flex max-w-5xl gap-6 px-6 py-4 text-sm">
          <span className="font-heading font-semibold">InstantAppoint</span>
          {links.map(({ to, label }) => (
            <Link
              key={to}
              to={to}
              className={
                pathname === to ? 'text-foreground' : 'text-muted-foreground'
              }
            >
              {label}
            </Link>
          ))}
        </nav>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-10">
        <Outlet />
      </main>
    </div>
  )
}
