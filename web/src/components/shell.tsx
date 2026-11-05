import { LogOut, Moon, Sun } from 'lucide-react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'

import { Logo } from '@/components/logo'
import { signOut } from '@/lib/auth'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

/**
 * The whole of the signed-in navigation.
 *
 * Inicio is the daily work. Asistente gets a tab of its own because what the
 * assistant may do with a patient's messages is the product's central
 * decision, not one more setting. Connecting a number, the templates and the
 * reminder settings are decided once and left alone, so they sit under
 * Configuración.
 */
const links = [
  { to: '/inicio', label: 'Inicio' },
  { to: '/asistente', label: 'Asistente' },
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
  const { theme, toggle } = useTheme()

  const handleSignOut = () => {
    signOut()
    navigate('/login', { replace: true })
  }

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur-sm supports-[backdrop-filter]:bg-background/85">
        <nav
          aria-label="Principal"
          className="mx-auto flex h-16 max-w-5xl items-center gap-2 px-4 text-sm sm:gap-6 sm:px-6"
        >
          <Logo className="mr-2 shrink-0 text-[17px] sm:text-[19px]" />
          <div className="flex items-center gap-1">
            {links.map(({ to, label }) => (
              <Link
                key={to}
                to={to}
                aria-current={isCurrent(pathname, to) ? 'page' : undefined}
                className={cn(
                  'rounded-lg px-3 py-2 font-medium transition-colors',
                  isCurrent(pathname, to)
                    ? 'bg-muted text-foreground'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {label}
              </Link>
            ))}
          </div>
          <button
            type="button"
            onClick={toggle}
            aria-label={theme === 'dark' ? 'Cambiar a modo claro' : 'Cambiar a modo oscuro'}
            title={theme === 'dark' ? 'Modo claro' : 'Modo oscuro'}
            className="ml-auto grid size-9 cursor-pointer place-items-center rounded-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            {theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </button>
          <button
            type="button"
            onClick={handleSignOut}
            className="inline-flex cursor-pointer items-center gap-2 rounded-[10px] px-3 py-2 font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <LogOut className="size-4" />
            <span className="sr-only sm:not-sr-only">Cerrar sesión</span>
          </button>
        </nav>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8 sm:px-6 sm:py-10">
        <Outlet />
      </main>
    </div>
  )
}
