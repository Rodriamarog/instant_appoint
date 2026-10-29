import { pb } from '@/lib/pocketbase'

/**
 * The one way this app calls its own Go routes.
 *
 * Everything the browser cannot do for itself goes through a handler: a send
 * needs the connected account's access token, which the page must never hold.
 * Those routes all answer in PocketBase's {message, status, data} shape, they
 * all authenticate with the session token as a bearer, and they must all be
 * uncached — three details that were written out by hand at each call site
 * until one of them quietly left out `cache: 'no-store'`.
 */

/**
 * Calls one route and turns a refusal into an Error carrying the server's own
 * sentence.
 *
 * The server's message is already written for a person — it quotes Meta's
 * explanation where Meta gave one — so it is passed through rather than
 * replaced. `fallback` covers a response with no message at all, including
 * one that is not JSON.
 */
export async function call<T>(
  path: string,
  init?: RequestInit,
  fallback = 'The request failed. Please try again.',
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    // A stale answer to "did this send?" is worse than a slow one, and a
    // bearer token in the request does not stop a proxy caching the reply.
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${pb.authStore.token}`,
      ...init?.headers,
    },
  })

  const body: unknown = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error((body as { message?: string } | null)?.message ?? fallback)
  }

  return body as T
}
