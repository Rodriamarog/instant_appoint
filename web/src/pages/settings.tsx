import { Link, Outlet, useLocation } from 'react-router'

/**
 * Everything a clinic sets up once and then leaves alone.
 *
 * The three pages beneath this used to be three of the five top-level tabs,
 * which put a Meta template review one click from the day's appointments.
 * None of them is daily work, so they are one click further away and the top
 * bar is left saying what this app is for.
 */
const sections = [
  { to: '/configuracion', label: 'Asistente' },
  { to: '/configuracion/whatsapp', label: 'WhatsApp' },
  { to: '/configuracion/recordatorios', label: 'Recordatorios' },
  { to: '/configuracion/plantillas', label: 'Plantillas' },
]

export function SettingsPage() {
  const { pathname } = useLocation()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-3xl font-bold tracking-[-0.03em]">
          Configuración
        </h1>
        <p className="mt-1.5 text-muted-foreground">
          Lo que decides una vez y después dejas en paz.
        </p>
      </div>

      <nav
        aria-label="Configuración"
        className="-mx-1 flex gap-1 overflow-x-auto border-b px-1 pb-3 text-sm"
      >
        {sections.map(({ to, label }) => {
          // Exact, unlike the top bar: these are siblings, and the first of
          // them is the index route, whose path is a prefix of all the rest.
          const current = pathname.replace(/\/$/, '') === to

          return (
            <Link
              key={to}
              to={to}
              aria-current={current ? 'page' : undefined}
              className={`shrink-0 rounded-lg px-3 py-2 font-medium transition-colors ${
                current
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              {label}
            </Link>
          )
        })}
      </nav>

      <Outlet />
    </div>
  )
}
