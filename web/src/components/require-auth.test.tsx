import { cleanup, render, screen } from '@testing-library/react'
import { act } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, expect, test } from 'vitest'

import { RequireAuth } from '@/components/require-auth'
import { pb } from '@/lib/pocketbase'

/**
 * The auth store only looks at the `exp` claim, so a syntactically valid
 * unexpired token is enough to stand in for a real login here.
 */
function tokenExpiringIn(seconds: number) {
  const encode = (value: object) =>
    btoa(JSON.stringify(value)).replace(/=+$/, '')

  return [
    encode({ alg: 'HS256', typ: 'JWT' }),
    encode({
      id: 'user123',
      collectionId: '_pb_users_auth_',
      exp: Math.floor(Date.now() / 1000) + seconds,
    }),
    'not-a-real-signature',
  ].join('.')
}

const signedInRecord = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
  verified: true,
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/login" element={<p>the login page</p>} />
        <Route element={<RequireAuth />}>
          <Route path="/calendar" element={<p>the calendar</p>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  )
}

afterEach(() => {
  cleanup()
  pb.authStore.clear()
})

test('a logged-out visit to an app page lands on the login page', () => {
  pb.authStore.clear()

  renderAt('/calendar')

  expect(screen.getByText('the login page')).toBeDefined()
  expect(screen.queryByText('the calendar')).toBeNull()
})

test('a logged-in visit to an app page renders the page', () => {
  pb.authStore.save(tokenExpiringIn(3600), signedInRecord)

  renderAt('/calendar')

  expect(screen.getByText('the calendar')).toBeDefined()
})

test('an expired session is treated as logged out', () => {
  // A token left in localStorage long enough to expire must not get past the
  // guard just because something is stored.
  pb.authStore.save(tokenExpiringIn(-60), signedInRecord)

  renderAt('/calendar')

  expect(screen.getByText('the login page')).toBeDefined()
})

test('clearing the session while on an app page redirects to login', () => {
  pb.authStore.save(tokenExpiringIn(3600), signedInRecord)

  renderAt('/calendar')
  expect(screen.getByText('the calendar')).toBeDefined()

  // This is what Sign Out does. The guard has to notice without a reload.
  act(() => {
    pb.authStore.clear()
  })

  expect(screen.getByText('the login page')).toBeDefined()
})
