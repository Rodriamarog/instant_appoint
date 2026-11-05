import { useSyncExternalStore } from 'react'

import { pb } from '@/lib/pocketbase'

/**
 * The session, and the four things that start or end one.
 *
 * users is a collection like any other, so this module owns its name and its
 * calls for the same reason every other lib module owns one: a page that
 * reaches for pb.collection('users') itself is a second place that has to be
 * found when any of this changes.
 */

/** The collection the session belongs to. */
const USERS = 'users'

/** The account fields this app keeps beyond PocketBase's own. */
export interface Registration {
  email: string
  password: string
  name: string
  businessName: string
}

/**
 * The PocketBase auth store is the session. It loads itself from localStorage
 * when the module is first imported, which is what makes a session survive a
 * page refresh, and it notifies subscribers on every login, refresh and clear.
 */
function subscribe(onStoreChange: () => void) {
  return pb.authStore.onChange(onStoreChange)
}

/**
 * Whether a usable session exists, re-rendering whenever that changes. The
 * check includes the token's expiry, so a stale token left in localStorage
 * reads as logged out.
 */
export function useIsSignedIn(): boolean {
  return useSyncExternalStore(subscribe, () => pb.authStore.isValid)
}

/** Ends the session. The guard reacts on its own; no reload is needed. */
export function signOut() {
  pb.authStore.clear()
}

/**
 * Signs in, and refuses an account that has not verified its email.
 *
 * Mail delivery is not configured yet, so an account that registered through
 * the form is never verified. PocketBase itself does not enforce that —
 * authWithPassword succeeds — so the session is cleared again here. Letting
 * an unverified account in would make the pending verification work a no-op.
 */
export async function signIn(email: string, password: string): Promise<void> {
  await pb.collection(USERS).authWithPassword(email, password)

  const user = pb.authStore.record
  if (user && !user.verified) {
    pb.authStore.clear()
    throw new Error(
      'Verifica tu correo electrónico antes de iniciar sesión. Busca en tu correo el enlace de verificación.',
    )
  }
}

/**
 * Creates an account, and says whether the verification email went out.
 *
 * Asking for the email is a separate, best-effort step: without configured
 * mail delivery it fails, and the account exists either way, so a failure
 * here must not read as a failed signup.
 */
export async function register(
  details: Registration,
): Promise<{ verificationSent: boolean }> {
  await pb.collection(USERS).create({
    email: details.email,
    password: details.password,
    passwordConfirm: details.password,
    name: details.name,
    business_name: details.businessName,
    emailVisibility: true,
  })

  try {
    await requestVerification(details.email)
  } catch {
    return { verificationSent: false }
  }

  return { verificationSent: true }
}

/** Asks for another verification email. */
export async function requestVerification(email: string): Promise<void> {
  await pb.collection(USERS).requestVerification(email)
}

type CurrentUser = { id: string; email: string; business_name: string }

// The auth store hands back a freshly parsed record on every read, so the
// snapshot is cached and only replaced when what it says changes —
// useSyncExternalStore re-renders forever otherwise.
let cachedUser: CurrentUser | null = null

function currentUserSnapshot(): CurrentUser | null {
  const record = pb.authStore.record
  if (!record) {
    cachedUser = null
    return null
  }
  const next = {
    id: String(record.id),
    email: String(record.email ?? ''),
    business_name: String(record.business_name ?? ''),
  }
  if (
    !cachedUser ||
    cachedUser.id !== next.id ||
    cachedUser.email !== next.email ||
    cachedUser.business_name !== next.business_name
  ) {
    cachedUser = next
  }
  return cachedUser
}

/** The signed-in account, re-rendering whenever the session changes. */
export function useCurrentUser(): CurrentUser | null {
  return useSyncExternalStore(subscribe, currentUserSnapshot)
}

/** Renames the business on the signed-in account. */
export async function updateBusinessName(businessName: string): Promise<void> {
  const id = pb.authStore.record?.id
  if (!id) {
    throw new Error('No hay una sesión iniciada.')
  }
  // PocketBase refreshes the auth store's copy of the record itself when the
  // record it updates is the signed-in one.
  await pb.collection(USERS).update(id, { business_name: businessName })
}

/**
 * Changes the password. PocketBase invalidates every session of the account
 * when it does, this one included, so it signs straight back in with the
 * new password rather than dropping the person at the login page.
 */
export async function changePassword(current: string, next: string): Promise<void> {
  const record = pb.authStore.record
  if (!record) {
    throw new Error('No hay una sesión iniciada.')
  }
  await pb.collection(USERS).update(record.id, {
    oldPassword: current,
    password: next,
    passwordConfirm: next,
  })
  await pb.collection(USERS).authWithPassword(String(record.email), next)
}
