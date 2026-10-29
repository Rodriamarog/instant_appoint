import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { liveFeed, pollingFeed, type Feed } from '@/lib/feed'
import { pb } from '@/lib/pocketbase'

/**
 * A feed is the seam the chat page reads through. Both implementations have to
 * behave the same way from the outside, because swapping one for the other is
 * the whole point of the seam existing.
 */

type Subscriber = (event: { action: string; record: Record<string, unknown> }) => void

/**
 * Stands in for a collection's realtime channel, and remembers how many times
 * it has been subscribed to — which is what the token-refresh case is about.
 */
function stubRealtime() {
  const subscribers: Subscriber[] = []
  const unsubscribed: number[] = []

  const subscribe = vi.fn((_topic: string, callback: Subscriber) => {
    const index = subscribers.push(callback) - 1

    return Promise.resolve(() => {
      unsubscribed.push(index)
      subscribers[index] = () => {}

      return Promise.resolve()
    })
  })

  vi.spyOn(pb, 'collection').mockReturnValue({
    subscribe,
  } as unknown as ReturnType<typeof pb.collection>)

  return {
    subscribe,
    unsubscribed,
    /** Delivers one event to every live subscriber. */
    deliver(record: Record<string, unknown> = {}) {
      for (const subscriber of [...subscribers]) {
        subscriber({ action: 'create', record })
      }
    },
  }
}

/**
 * Opens a feed and makes sure it is closed even when an assertion fails
 * partway. A feed left open outlives its test: it stays registered for session
 * changes, and the next test's sign-in wakes it up again.
 */
const opened: (() => void)[] = []

function open<T>(
  feed: Feed<T>,
  onItems: (items: T[]) => void = () => {},
  onProblem: (error: unknown) => void = () => {},
) {
  const close = feed(onItems, onProblem)
  opened.push(close)

  return close
}

function signIn(token: string) {
  pb.authStore.save(token, {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
  })
}

beforeEach(() => {
  signIn('the-first-token')
})

afterEach(() => {
  for (const close of opened.splice(0)) {
    close()
  }
  pb.authStore.clear()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

test('a live feed hands over what it loaded, and loads again on an event', async () => {
  const realtime = stubRealtime()
  let rows = ['one']
  const seen: string[][] = []

  open(
    liveFeed<string>({
      collection: 'whatsapp_messages',
      load: () => Promise.resolve([...rows]),
    }),
    (items) => seen.push(items),
  )

  await vi.waitFor(() => expect(seen.at(-1)).toEqual(['one']))

  rows = ['one', 'two']
  realtime.deliver()

  await vi.waitFor(() => expect(seen.at(-1)).toEqual(['one', 'two']))
})

test('a live feed ignores events about something else', async () => {
  const realtime = stubRealtime()
  const load = vi.fn(() => Promise.resolve(['a message']))

  open(
    liveFeed<string>({
      collection: 'whatsapp_messages',
      load,
      concerns: (record) => record.conversation_id === 'open_one',
    }),
  )

  await vi.waitFor(() => expect(realtime.subscribe).toHaveBeenCalledTimes(1))
  await vi.waitFor(() => expect(load).toHaveBeenCalled())
  const settled = load.mock.calls.length

  realtime.deliver({ conversation_id: 'a_different_one' })
  await Promise.resolve()
  expect(load).toHaveBeenCalledTimes(settled)

  realtime.deliver({ conversation_id: 'open_one' })
  await vi.waitFor(() => expect(load).toHaveBeenCalledTimes(settled + 1))
})

/**
 * The one that fails silently. PocketBase evaluates the collection's list rule
 * for every subscriber, against the token the subscription was submitted with,
 * and the JS SDK does not resubmit when the session token changes. A feed that
 * does not resubscribe keeps its connection open and simply stops being told
 * anything.
 */
test('a live feed resubscribes when the session token is refreshed', async () => {
  const realtime = stubRealtime()
  const seen: string[][] = []

  open(
    liveFeed<string>({
      collection: 'whatsapp_messages',
      load: () => Promise.resolve(['a message']),
    }),
    (items) => seen.push(items),
  )

  await vi.waitFor(() => expect(realtime.subscribe).toHaveBeenCalledTimes(1))
  await vi.waitFor(() => expect(seen.length).toBeGreaterThan(0))
  const before = seen.length

  // What pb.collection('users').authRefresh() does when it answers.
  signIn('the-refreshed-token')

  await vi.waitFor(() => expect(realtime.subscribe).toHaveBeenCalledTimes(2))
  // The stale subscription is dropped rather than left open alongside.
  await vi.waitFor(() => expect(realtime.unsubscribed).toEqual([0]))
  // And the gap is covered: whatever arrived while unsubscribed is picked up
  // by the reload the new subscription triggers.
  await vi.waitFor(() => expect(seen.length).toBeGreaterThan(before))

  // The new subscription is a working one, not just an extra call.
  realtime.deliver()
  await vi.waitFor(() => expect(seen.at(-1)).toEqual(['a message']))
})

test('a live feed signed out does not resubscribe', async () => {
  const realtime = stubRealtime()

  open(
    liveFeed<string>({
      collection: 'whatsapp_messages',
      load: () => Promise.resolve([]),
    }),
  )

  await vi.waitFor(() => expect(realtime.subscribe).toHaveBeenCalledTimes(1))

  pb.authStore.clear()

  await vi.waitFor(() => expect(realtime.unsubscribed).toEqual([0]))
  expect(realtime.subscribe).toHaveBeenCalledTimes(1)
})

test('closing a live feed unsubscribes and stops handing anything over', async () => {
  const realtime = stubRealtime()
  const seen: string[][] = []

  const close = open(
    liveFeed<string>({
      collection: 'whatsapp_messages',
      load: () => Promise.resolve(['a message']),
    }),
    (items) => seen.push(items),
  )

  await vi.waitFor(() => expect(realtime.subscribe).toHaveBeenCalledTimes(1))
  await vi.waitFor(() => expect(seen.length).toBeGreaterThan(0))

  close()
  await vi.waitFor(() => expect(realtime.unsubscribed).toEqual([0]))

  const before = seen.length
  realtime.deliver()
  signIn('another-token')

  await Promise.resolve()
  expect(seen.length).toBe(before)
  expect(realtime.subscribe).toHaveBeenCalledTimes(1)
})

test('a live feed reports a failure instead of silently showing nothing', async () => {
  stubRealtime()
  const problems: unknown[] = []

  open(
    liveFeed<string>({
      collection: 'whatsapp_messages',
      load: () => Promise.reject(new Error('the server said no')),
    }),
    () => {},
    (error) => problems.push(error),
  )

  await vi.waitFor(() => expect(problems.length).toBeGreaterThan(0))
  expect((problems[0] as Error).message).toBe('the server said no')
})

/**
 * The fallback. It is kept working and tested because the live connection is
 * only proven locally — a tunnel's idle timeout can cut a long-lived stream —
 * and the page has to be swappable onto this without being rewritten.
 */
test('a polling feed loads straight away and then on every interval', async () => {
  vi.useFakeTimers()
  const seen: string[][] = []
  const load = vi.fn(() => Promise.resolve(['a message']))

  const close = open(pollingFeed<string>({ load, everyMs: 3000 }), (items) =>
    seen.push(items),
  )

  await vi.advanceTimersByTimeAsync(0)
  expect(seen).toEqual([['a message']])

  await vi.advanceTimersByTimeAsync(3000)
  await vi.advanceTimersByTimeAsync(3000)
  expect(load).toHaveBeenCalledTimes(3)

  close()
  await vi.advanceTimersByTimeAsync(9000)
  expect(load).toHaveBeenCalledTimes(3)
})
