import { Navigate, Outlet } from 'react-router'

import { useIsSignedIn } from '@/lib/auth'

/**
 * The layout route every app page sits under. Without a session it renders the
 * login page instead of the page that was asked for.
 *
 * The redirect is rendered rather than fired from an effect, so the protected
 * page never mounts and never flashes its contents at a logged-out visitor.
 * `replace` keeps the protected URL out of the history, so Back from the login
 * page does not bounce straight into the guard again.
 */
export function RequireAuth() {
  const signedIn = useIsSignedIn()

  if (!signedIn) {
    return <Navigate to="/login" replace />
  }

  return <Outlet />
}
