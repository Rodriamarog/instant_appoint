import { useSyncExternalStore } from 'react'

import { pb } from '@/lib/pocketbase'

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
