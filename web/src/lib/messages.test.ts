import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import {
  listMessages,
  listUnmatchedTaps,
} from '@/lib/messages'
import { pb } from '@/lib/pocketbase'

/** Records the options a list query was issued with. */
type ListOptions = { filter?: string; sort?: string }

function stubList(items: unknown[] = []) {
  const getFullList = vi.fn((_options?: ListOptions) => Promise.resolve(items))

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
  } as unknown as ReturnType<typeof pb.collection>)

  return getFullList
}

beforeEach(() => {
  pb.authStore.save('the-session-token', {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
  })
})

afterEach(() => {
  pb.authStore.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

test('a thread is asked for by conversation, oldest first', async () => {
  const getFullList = stubList()

  await listMessages('conv1')

  expect(pb.collection).toHaveBeenCalledWith('whatsapp_messages')
  const [options] = getFullList.mock.calls[0] ?? []
  expect(options?.sort).toBe('sent_at')
  expect(options?.filter).toContain('conv1')
  expect(options?.filter).toContain('conversation_id')
})

/**
 * A conversation id goes into a filter string, and a filter string is a query
 * language. pb.filter binds it rather than pasting it in.
 */
test('a conversation id cannot smuggle a clause into the filter', async () => {
  const getFullList = stubList()

  await listMessages("conv1' || user_id != '")

  const [options] = getFullList.mock.calls[0] ?? []
  const filter = options?.filter
  // The quotes that would have closed the literal are escaped, so the whole
  // thing stays one value and the || is text rather than an operator.
  expect(filter).toBe("conversation_id = 'conv1\\' || user_id != \\''")
})

/**
 * The taps that changed nothing, asked for as a bounded list: this person's,
 * taps only, the ones carrying no appointment, and only recent ones.
 *
 * The window is what keeps the warning readable. Nothing marks one of these
 * as dealt with, so without it the page would accumulate every tap that ever
 * missed and become a banner nobody reads.
 */
test('the unmatched taps asked for are this person’s, recent, and newest first', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-11-02T12:00:00.000Z'))

  const getFullList = stubList([])
  await listUnmatchedTaps()

  const options = getFullList.mock.calls[0][0]
  expect(options?.filter).toContain("user_id = 'user123'")
  expect(options?.filter).toContain('message_type = "button"')
  expect(options?.filter).toContain('event_id = ""')
  expect(options?.filter).toContain("sent_at >= '2026-10-26T12:00:00.000Z'")
  expect(options?.sort).toBe('-sent_at')

  vi.useRealTimers()
})
