import { Link, Outlet, useLocation, useNavigate } from 'react-router'

import { signOut } from '@/lib/auth'

/**
 * The whole of the signed-in navigation.
 *
 * Two tabs, because only one of the five this replaced was daily work:
 * connecting a number happens once, choosing a template is a setup act with a
 * Meta review attached, and the reminder settings are decided and then left
 * alone. All three sit under Configuración now.
 */
const links = [
  { to: '/inicio', label: 'Inicio' },
  { to: '/configuracion', label: 'Configuración' },
]

/**
 * True while the person is on `to` or on a page beneath it.
 *
 * The separator is part of the comparison on purpose: a bare prefix test
 * would light /inicio up for an /inicio-anything added later, which is a
 * different page. An exact match is not enough either — Configuración holds
 * four sub-pages, and the tab went dark on every one of them.
 */
function isCurrent(pathname: string, to: string): boolean {
  return pathname === to || pathname.startsWith(to + '/')
}

/**
 * The layout every signed-in page renders inside. It assumes a session: the
 * RequireAuth route above it never renders it without one.
 */
export function Shell() {
  const { pathname } = useLocation()
  const navigate = useNavigate()

  const handleSignOut = () => {
    signOut()
    navigate('/login', { replace: true })
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="border-b">
        <nav
          aria-label="Principal"
          className="mx-auto flex max-w-5xl items-center gap-6 px-6 py-4 text-sm"
        >
          <span className="font-heading font-semibold">InstantAppoint</span>
          {links.map(({ to, label }) => (
            <Link
              key={to}
              to={to}
              aria-current={isCurrent(pathname, to) ? 'page' : undefined}
              className={
                isCurrent(pathname, to)
                  ? 'text-foreground'
                  : 'text-muted-foreground'
              }
            >
              {label}
            </Link>
          ))}
          <button
            type="button"
            onClick={handleSignOut}
            className="ml-auto rounded-lg bg-black px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
          >
            Cerrar sesión
          </button>
        </nav>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-10">
        <Outlet />
      </main>
    </div>
  )
}
