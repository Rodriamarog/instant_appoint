import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import {
  listMessages,
  listUnmatchedTaps,
  listSentReminders,
  remindedEventIds,
  type Message,
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

function message(id: string, sentAt: string): Message {
  return {
    id,
    user_id: 'user123',
    conversation_id: 'conv1',
    direction: 'outbound',
    message_content: id,
    message_type: 'text',
    message_id: 'wamid.' + id,
    sent_at: sentAt,
    to_number: '5215512345678',
    event_id: '',
    status: 'sent',
  }
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
 * The sent column on the reminders page. Capped on purpose — nobody reads a
 * thousand rows — which is exactly why deciding what is still *upcoming*
 * must not read it. See remindedEventIds.
 */
test('the sent reminders are one page of the signed-in person’s reminder rows, newest first', async () => {
  const getList = vi.fn((_page: number, _perPage: number, _options?: ListOptions) =>
    Promise.resolve({ items: [] as Message[] }),
  )
  vi.spyOn(pb, 'collection').mockReturnValue({
    getList,
  } as unknown as ReturnType<typeof pb.collection>)

  await listSentReminders()

  const [page, perPage, options] = getList.mock.calls[0]
  expect([page, perPage]).toEqual([1, 50])
  expect(options?.sort).toBe('-sent_at')
  expect(options?.filter).toContain('message_type = "reminder"')
  expect(options?.filter).toContain('status = "sent"')
  expect(options?.filter).toContain("user_id = 'user123'")
})

/**
 * A 404 used to be read as "none", because whatsapp_messages did not exist
 * when this was written. It does now, so a failure is a failure and the page
 * must be allowed to say so rather than show an empty column.
 */
test('a failure reading the sent reminders is not passed off as "none"', async () => {
  for (const status of [404, 500]) {
    vi.spyOn(pb, 'collection').mockReturnValue({
      getList: () =>
        Promise.reject(Object.assign(new Error('nope'), { status })),
    } as unknown as ReturnType<typeof pb.collection>)

    await expect(listSentReminders()).rejects.toThrow()
  }
})

test('the already-sent question is asked about given appointments, and only those', async () => {
  const getFullList = stubList([
    { ...message('m1', '2026-10-29T12:00:00.000Z'), event_id: 'evtA' },
  ])

  const reminded = await remindedEventIds(['evtA', 'evtB'])

  expect(reminded).toEqual(new Set(['evtA']))
  const filter = getFullList.mock.calls[0][0]?.filter ?? ''
  expect(filter).toContain("event_id = 'evtA'")
  expect(filter).toContain("event_id = 'evtB'")
  expect(filter).toContain('message_type = "reminder"')
})

/**
 * PocketBase has no IN, so the ids go in as a chain of ORs — and that chain
 * becomes the query string. Enough appointments and it has to be split, which
 * must not change the answer.
 */
test('a long list of appointments is asked for in several queries, and the answers join up', async () => {
  const ids = Array.from({ length: 120 }, (_, index) => `evt${index}`)
  const getFullList = vi.fn((options?: ListOptions) =>
    Promise.resolve(
      ids
        .filter((id) => (options?.filter ?? '').includes(`'${id}'`))
        .map((id) => ({ ...message(id, '2026-10-29T12:00:00.000Z'), event_id: id })),
    ),
  )
  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
  } as unknown as ReturnType<typeof pb.collection>)

  const reminded = await remindedEventIds(ids)

  expect(getFullList.mock.calls.length).toBeGreaterThan(1)
  expect(reminded.size).toBe(120)
})

test('asking about no appointments asks the database nothing', async () => {
  const getFullList = stubList([])

  expect(await remindedEventIds([])).toEqual(new Set())
  expect(getFullList).not.toHaveBeenCalled()
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
