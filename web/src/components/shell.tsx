import { Link, Outlet, useLocation, useNavigate } from 'react-router'

import { signOut } from '@/lib/auth'

const links = [
  { to: '/calendar', label: 'Calendar' },
  { to: '/chat', label: 'Chat' },
  { to: '/whatsapp', label: 'WhatsApp' },
  { to: '/reminders', label: 'Reminders' },
  { to: '/templates', label: 'Templates' },
]

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
        <nav className="mx-auto flex max-w-5xl items-center gap-6 px-6 py-4 text-sm">
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
          <button
            type="button"
            onClick={handleSignOut}
            className="ml-auto rounded-lg bg-black px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
          >
            Sign Out
          </button>
        </nav>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-10">
        <Outlet />
      </main>
    </div>
  )
}
