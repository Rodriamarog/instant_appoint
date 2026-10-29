import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { listConversations } from '@/lib/conversations'
import {
  listMessages,
  sendReply,
  withMessage,
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
 * The list rule is what scopes conversations to their owner, so the query does
 * not repeat it — but the sort is the page's, and it has to be the descending
 * one or the list reads oldest first.
 */
test('conversations are asked for most recent activity first', async () => {
  const getFullList = stubList()

  await listConversations()

  expect(pb.collection).toHaveBeenCalledWith('conversations')
  const [options] = getFullList.mock.calls[0] ?? []
  expect(options?.sort).toBe('-last_message_at')
})

test('sending a reply posts it to the server with the session token', async () => {
  const stored = message('msg1', '2026-10-29T12:00:00.000Z')
  const fetchStub = vi.fn((_url: string, _init?: RequestInit) =>
    Promise.resolve({ ok: true, json: () => Promise.resolve(stored) } as Response),
  )
  vi.stubGlobal('fetch', fetchStub)

  const sent = await sendReply('conv1', 'On my way.')

  expect(sent).toEqual(stored)
  const [url, init] = fetchStub.mock.calls[0] ?? []
  expect(url).toBe('/api/chat/send')
  expect(init?.method).toBe('POST')
  expect((init?.headers as Record<string, string>).Authorization).toBe(
    'Bearer the-session-token',
  )
  expect(JSON.parse(init?.body as string)).toEqual({
    conversation_id: 'conv1',
    message: 'On my way.',
  })
})

test('a refused send surfaces the sentence the server wrote', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: false,
        json: () =>
          Promise.resolve({
            message: 'WhatsApp rejected the message: more than 24 hours have passed.',
            status: 500,
          }),
      } as Response),
    ),
  )

  await expect(sendReply('conv1', 'Still there?')).rejects.toThrow(
    'more than 24 hours have passed',
  )
})

test('a send that fails without a readable answer still says something', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: false,
        json: () => Promise.reject(new Error('not json')),
      } as unknown as Response),
    ),
  )

  await expect(sendReply('conv1', 'Hello?')).rejects.toThrow(
    'could not be sent',
  )
})

/**
 * A sent reply arrives twice: once as the send's own answer, once over the
 * live connection. They are the same row, so the thread must end up with one
 * of it.
 */
test('a message merged into a thread lands in order and never twice', () => {
  const thread = [
    message('a', '2026-10-29T12:00:00.000Z'),
    message('c', '2026-10-29T12:00:02.000Z'),
  ]

  const withNew = withMessage(thread, message('b', '2026-10-29T12:00:01.000Z'))
  expect(withNew.map((item) => item.id)).toEqual(['a', 'b', 'c'])

  const again = withMessage(withNew, message('b', '2026-10-29T12:00:01.000Z'))
  expect(again.map((item) => item.id)).toEqual(['a', 'b', 'c'])
})
