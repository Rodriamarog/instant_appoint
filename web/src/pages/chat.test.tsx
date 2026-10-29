import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router'

import type { Conversation } from '@/lib/conversations'
import type { Feed } from '@/lib/feed'
import { pollingFeed } from '@/lib/feed'
import type { Message, MessageFeed } from '@/lib/messages'
import { pb } from '@/lib/pocketbase'
import { ChatPage } from '@/pages/chat'

/**
 * The page is driven through its two feeds. That is the seam: these tests hand
 * it a feed the same way a deployment would hand it the polling one, and the
 * page cannot tell the difference — which is what makes the swap a change of
 * argument rather than a rewrite.
 */

const quiet: Conversation = {
  id: 'conv_quiet',
  user_id: 'user123',
  customer_phone: '5216641111111',
  status: 'active',
  last_message_at: '2026-10-28T09:00:00.000Z',
  phone_number_id: '200000000000002',
}

const loud: Conversation = {
  id: 'conv_loud',
  user_id: 'user123',
  customer_phone: '5216642222222',
  status: 'active',
  last_message_at: '2026-10-29T18:30:00.000Z',
  phone_number_id: '200000000000002',
}

function message(
  id: string,
  direction: Message['direction'],
  content: string,
  sentAt: string,
  conversationId = 'conv_loud',
): Message {
  return {
    id,
    user_id: 'user123',
    conversation_id: conversationId,
    direction,
    message_content: content,
    message_type: 'text',
    message_id: 'wamid.' + id,
    sent_at: sentAt,
    to_number: '5215512345678',
    event_id: '',
    status: 'sent',
  }
}

const thread = [
  message('m1', 'inbound', 'Can I move my appointment?', '2026-10-29T18:29:00.000Z'),
  message('m2', 'outbound', 'Of course — when suits?', '2026-10-29T18:30:00.000Z'),
]

/**
 * A feed a test can push into, standing in for whatever is keeping the list
 * current. The page is given the same shape either way.
 */
function controllable<T>(initial: T[]) {
  let push: ((items: T[]) => void) | null = null
  let closes = 0

  const feed: Feed<T> = (onItems) => {
    push = onItems
    onItems(initial)

    return () => {
      closes += 1
      push = null
    }
  }

  return {
    feed,
    deliver(items: T[]) {
      push?.(items)
    },
    get closes() {
      return closes
    },
  }
}

function renderChat(props: {
  conversations?: Feed<Conversation>
  messages?: MessageFeed
}) {
  return render(
    <MemoryRouter>
      <ChatPage {...props} />
    </MemoryRouter>,
  )
}

/** A feed factory that answers every conversation with the same thread. */
function staticMessages(messages: Message[]): MessageFeed {
  return () => (onItems) => {
    onItems(messages)

    return () => {}
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
  cleanup()
  pb.authStore.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

test('conversations are listed in the order the feed hands them over', async () => {
  const conversations = controllable<Conversation>([loud, quiet])
  renderChat({ conversations: conversations.feed, messages: staticMessages([]) })

  const listed = await screen.findAllByRole('button', { name: /\+52166/ })

  expect(listed.map((button) => button.textContent)).toEqual([
    expect.stringContaining('+5216642222222'),
    expect.stringContaining('+5216641111111'),
  ])
})

test('selecting a conversation shows its thread, inbound and outbound apart', async () => {
  const conversations = controllable<Conversation>([loud, quiet])
  renderChat({
    conversations: conversations.feed,
    messages: staticMessages(thread),
  })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))

  const inbound = await screen.findByText('Can I move my appointment?')
  const outbound = await screen.findByText('Of course — when suits?')

  expect(inbound.closest('li')?.dataset.direction).toBe('inbound')
  expect(outbound.closest('li')?.dataset.direction).toBe('outbound')

  // And the person being spoken to is named above the thread.
  expect(screen.getAllByText('+5216642222222').length).toBeGreaterThan(1)
})

test('a message inserted elsewhere appears in the open thread', async () => {
  const conversations = controllable<Conversation>([loud])
  const messages = controllable<Message>(thread)

  renderChat({ conversations: conversations.feed, messages: () => messages.feed })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))
  expect(await screen.findByText('Can I move my appointment?')).toBeDefined()
  expect(screen.queryByText('Tuesday at ten, please.')).toBeNull()

  // What the feed does when the webhook stores a customer's reply.
  messages.deliver([
    ...thread,
    message('m3', 'inbound', 'Tuesday at ten, please.', '2026-10-29T18:31:00.000Z'),
  ])

  const arrived = await screen.findByText('Tuesday at ten, please.')
  expect(arrived.closest('li')?.dataset.direction).toBe('inbound')
})

test('a reply is sent and shown in the thread', async () => {
  const conversations = controllable<Conversation>([loud])
  const messages = controllable<Message>(thread)

  const sent = message('m9', 'outbound', 'Tuesday at ten it is.', '2026-10-29T18:32:00.000Z')
  const fetchStub = vi.fn((_url: string, _init?: RequestInit) =>
    Promise.resolve({ ok: true, json: () => Promise.resolve(sent) } as Response),
  )
  vi.stubGlobal('fetch', fetchStub)

  renderChat({ conversations: conversations.feed, messages: () => messages.feed })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))
  await userEvent.type(screen.getByLabelText('Message'), 'Tuesday at ten it is.')
  await userEvent.click(screen.getByRole('button', { name: /send/i }))

  const shown = await screen.findByText('Tuesday at ten it is.')
  expect(shown.closest('li')?.dataset.direction).toBe('outbound')

  const [url, init] = fetchStub.mock.calls[0] ?? []
  expect(url).toBe('/api/chat/send')
  expect(JSON.parse(init?.body as string)).toEqual({
    conversation_id: 'conv_loud',
    message: 'Tuesday at ten it is.',
  })

  // The box is cleared, and the reply the connection echoes back does not
  // double it up.
  expect((screen.getByLabelText('Message') as HTMLInputElement).value).toBe('')
  messages.deliver([...thread, sent])
  await waitFor(() =>
    expect(screen.getAllByText('Tuesday at ten it is.')).toHaveLength(1),
  )
})

test('a refused send says why and keeps what was typed', async () => {
  const conversations = controllable<Conversation>([loud])
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: false,
        json: () =>
          Promise.resolve({ message: 'WhatsApp rejected the message: too late.' }),
      } as Response),
    ),
  )

  renderChat({
    conversations: conversations.feed,
    messages: staticMessages(thread),
  })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))
  await userEvent.type(screen.getByLabelText('Message'), 'Still there?')
  await userEvent.click(screen.getByRole('button', { name: /send/i }))

  expect(await screen.findByRole('alert')).toHaveProperty(
    'textContent',
    'WhatsApp rejected the message: too late.',
  )
  expect((screen.getByLabelText('Message') as HTMLInputElement).value).toBe(
    'Still there?',
  )
})

test('switching conversations does not leave the old thread on screen', async () => {
  const conversations = controllable<Conversation>([loud, quiet])
  const byConversation: MessageFeed = (conversationId) => (onItems) => {
    onItems(
      conversationId === 'conv_loud'
        ? thread
        : [message('q1', 'inbound', 'A different customer entirely', '2026-10-28T09:00:00.000Z', 'conv_quiet')],
    )

    return () => {}
  }

  renderChat({ conversations: conversations.feed, messages: byConversation })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))
  expect(await screen.findByText('Can I move my appointment?')).toBeDefined()

  await userEvent.click(screen.getByRole('button', { name: /5216641111111/ }))
  expect(await screen.findByText('A different customer entirely')).toBeDefined()
  expect(screen.queryByText('Can I move my appointment?')).toBeNull()
})

test('closing the page closes its feeds', async () => {
  const conversations = controllable<Conversation>([loud])
  const messages = controllable<Message>(thread)

  const view = renderChat({
    conversations: conversations.feed,
    messages: () => messages.feed,
  })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))
  await screen.findByText('Can I move my appointment?')

  view.unmount()

  expect(conversations.closes).toBe(1)
  expect(messages.closes).toBe(1)
})

/**
 * The acceptance criterion the seam exists for. The page is given the polling
 * implementation instead of the live one and behaves identically, with no
 * change to the page — which is what "could be swapped back to polling without
 * reworking the page" means in practice.
 */
test('the page works unchanged on the polling feed', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })

  let rows = [...thread]
  const messages: MessageFeed = (conversationId) =>
    pollingFeed({
      load: () =>
        Promise.resolve(rows.filter((item) => item.conversation_id === conversationId)),
      everyMs: 3000,
    })

  renderChat({
    conversations: pollingFeed({ load: () => Promise.resolve([loud]), everyMs: 5000 }),
    messages,
  })

  await userEvent.click(await screen.findByRole('button', { name: /5216642222222/ }))
  expect(await screen.findByText('Can I move my appointment?')).toBeDefined()

  // Inserted by something other than this page, exactly as the live case.
  rows = [
    ...thread,
    message('m3', 'inbound', 'Tuesday at ten, please.', '2026-10-29T18:31:00.000Z'),
  ]
  expect(screen.queryByText('Tuesday at ten, please.')).toBeNull()

  await vi.advanceTimersByTimeAsync(3000)

  const arrived = await screen.findByText('Tuesday at ten, please.')
  expect(arrived.closest('li')?.dataset.direction).toBe('inbound')
})
