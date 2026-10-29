import type { RecordModel } from 'pocketbase'

import { pb } from '@/lib/pocketbase'

/**
 * How a page keeps a list up to date without knowing how that happens.
 *
 * This is the seam the chat page reads through. The page opens a feed, is
 * handed a list, and is handed a fresh one whenever there may be a new one; it
 * never learns whether that came from a live connection or from asking again
 * on a timer. Swapping the two is a one-line change in the module that owns
 * the collection, not a change to the page.
 *
 * The seam exists because the live path is only proven locally. A long-lived
 * server-sent-events stream is exactly what an idle timeout on a proxy in
 * front of the app will cut, and when it is cut the symptom is a thread that
 * quietly stops updating rather than an error anyone sees. Keeping the polling
 * implementation written, tested and one line away is what makes that
 * recoverable.
 *
 * Both implementations re-read the whole list rather than patching it from the
 * event, which is what makes them interchangeable: the ordering, the filtering
 * and the access rules are all decided by the server either way, so neither
 * can drift from the other.
 */

/**
 * Opens a feed. It hands the current items to onItems, keeps doing so, and
 * returns the function that closes it.
 *
 * onProblem is called instead when a load fails, so a page can say so rather
 * than showing an empty list that looks like "nothing here".
 */
export type Feed<T> = (
  onItems: (items: T[]) => void,
  onProblem: (error: unknown) => void,
) => () => void

/** What a feed needs in order to fetch the list it hands over. */
interface Source<T> {
  load: () => Promise<T[]>
}

/**
 * A feed driven by PocketBase's realtime connection.
 *
 * `concerns` narrows which events are worth a reload — an open thread does not
 * care about a message in a conversation nobody is looking at. Events for
 * records the signed-in person cannot see never arrive in the first place:
 * subscriptions evaluate the collection's list rule, the same rule the load
 * goes through.
 *
 * That rule evaluation is also why this watches the session. PocketBase binds
 * a subscription to the token it was submitted with, and the JS SDK does not
 * resubmit when the token changes — so after a token refresh the connection
 * stays open and simply stops delivering. Resubscribing under the new token is
 * the difference between a thread that keeps working and one that silently
 * stops; the reload that follows covers whatever arrived in the gap.
 */
export function liveFeed<T>(
  source: Source<T> & {
    collection: string
    concerns?: (record: RecordModel) => boolean
  },
): Feed<T> {
  return (onItems, onProblem) => {
    let closed = false
    let unsubscribe: (() => Promise<void>) | null = null

    // Loads overlap — an event can arrive while the last one is still in
    // flight — and nothing guarantees they come back in the order they were
    // asked for. Without this a slow older answer overwrites a newer list and
    // the thread appears to lose its most recent message.
    let newestLoad = 0

    const reload = () => {
      const thisLoad = ++newestLoad

      source
        .load()
        .then((items) => {
          if (!closed && thisLoad === newestLoad) {
            onItems(items)
          }
        })
        .catch((error: unknown) => {
          if (!closed && thisLoad === newestLoad) {
            onProblem(error)
          }
        })
    }

    const open = () => {
      pb.collection(source.collection)
        .subscribe('*', (event: { record: RecordModel }) => {
          if (source.concerns && !source.concerns(event.record)) {
            return
          }

          reload()
        })
        .then((stop) => {
          if (closed) {
            void stop()

            return
          }

          unsubscribe = stop
          // Anything that arrived between the load above and the moment the
          // subscription was accepted would otherwise be missed.
          reload()
        })
        .catch((error: unknown) => {
          if (!closed) {
            onProblem(error)
          }
        })
    }

    reload()
    open()

    const stopWatchingSession = pb.authStore.onChange((token) => {
      if (closed) {
        return
      }

      const stale = unsubscribe
      unsubscribe = null

      // The old subscription has to be gone before the new one is asked for,
      // and this await is load-bearing. The SDK batches subscription changes
      // made in one tick and sends nothing when the batch nets out to the
      // same set — so dropping and re-adding together is seen as no change,
      // no request is sent, and the new listener is never attached to the
      // stream. The page then looks subscribed and receives nothing. Measured
      // against pocketbase 0.26 in a browser, not reasoned about.
      Promise.resolve(stale?.())
        .catch(() => {
          // A failed unsubscribe still leaves the old listener detached from
          // this feed; reopening is the recovery either way.
        })
        .then(() => {
          // A cleared store is a sign-out. The guard is already taking the
          // page away, and a subscription without a session would be refused.
          if (!closed && token) {
            open()
          }
        })
    })

    return () => {
      closed = true
      stopWatchingSession()
      void unsubscribe?.()
      unsubscribe = null
    }
  }
}

/**
 * A feed that asks again on a timer.
 *
 * This is what the chat page did before, at 5 seconds for the conversation
 * list and 3 for the open thread. It is kept as the fallback for a deployment
 * where the live connection cannot be held open.
 */
export function pollingFeed<T>(source: Source<T> & { everyMs: number }): Feed<T> {
  return (onItems, onProblem) => {
    let closed = false
    let newestLoad = 0

    const reload = () => {
      const thisLoad = ++newestLoad

      source
        .load()
        .then((items) => {
          if (!closed && thisLoad === newestLoad) {
            onItems(items)
          }
        })
        .catch((error: unknown) => {
          if (!closed && thisLoad === newestLoad) {
            onProblem(error)
          }
        })
    }

    reload()
    const timer = setInterval(reload, source.everyMs)

    return () => {
      closed = true
      clearInterval(timer)
    }
  }
}
