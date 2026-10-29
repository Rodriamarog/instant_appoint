import { liveFeed, pollingFeed, type Feed } from '@/lib/feed'
import { pb } from '@/lib/pocketbase'

/**
 * The messages in a thread, and the one way to add to it.
 *
 * Reading is the browser talking to PocketBase directly: whatsapp_messages is
 * owner-read, so the access rules scope the thread to the person asking, and
 * a message belonging to someone else's conversation is not returned however
 * it is asked for.
 *
 * Sending is not. The collection is closed to writes from the browser, and the
 * reply itself goes out through Meta with the connected account's access token
 * — a credential the page must never hold. Both of those make the send a Go
 * route, which is why this is the one module here that posts to the server.
 */
export const COLLECTION = 'whatsapp_messages'

/** One message in a thread. */
export interface Message {
  id: string
  user_id: string
  conversation_id: string
  direction: 'inbound' | 'outbound'
  message_content: string
  message_type: string
  message_id: string
  sent_at: string
}

/**
 * A conversation's messages, oldest first.
 *
 * sent_at is a text column, so this is a string sort; the stored format is
 * fixed-width precisely so that it matches chronological order.
 */
export async function listMessages(conversationId: string): Promise<Message[]> {
  return pb.collection(COLLECTION).getFullList<Message>({
    filter: pb.filter('conversation_id = {:conversationId}', { conversationId }),
    sort: 'sent_at',
  })
}

/**
 * How the page is kept up to date with one thread.
 *
 * A feed is opened per conversation, so this is a factory rather than a feed.
 * Either implementation satisfies it, which is the seam: the page is handed
 * one of these and never learns which.
 */
export type MessageFeed = (conversationId: string) => Feed<Message>

/** The open thread, kept current over the live connection. */
export const liveMessages: MessageFeed = (conversationId) =>
  liveFeed({
    collection: COLLECTION,
    load: () => listMessages(conversationId),
    // Every message this person can see arrives on the connection, including
    // ones in threads nobody is looking at. Only the open one is worth a
    // reload.
    concerns: (record) => record.conversation_id === conversationId,
  })

/**
 * The same thread, asked for again every few seconds.
 *
 * The page polled at three seconds before. Kept for the same reason as the
 * conversation list's: the live path is proven locally and not through a
 * tunnel, and this is what the page falls back to without being rewritten.
 */
export function polledMessages(everyMs = 3000): MessageFeed {
  return (conversationId) =>
    pollingFeed({ load: () => listMessages(conversationId), everyMs })
}

/**
 * Sends a reply and returns the message that was stored.
 *
 * The server answers in PocketBase's {message, status, data} shape, and its
 * message is written for a person — it quotes WhatsApp's own explanation where
 * there was one, which is what says that a customer has to write first before
 * the business can.
 */
export async function sendReply(
  conversationId: string,
  text: string,
): Promise<Message> {
  const response = await fetch('/api/chat/send', {
    method: 'POST',
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${pb.authStore.token}`,
    },
    body: JSON.stringify({ conversation_id: conversationId, message: text }),
  })

  const body: unknown = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(
      (body as { message?: string } | null)?.message ??
        'The message could not be sent. Please try again.',
    )
  }

  return body as Message
}

/**
 * Puts a message into a thread, replacing any copy of it already there.
 *
 * A sent reply arrives twice — once as the send's own answer, once over the
 * live connection — and the two are the same row. Merging by id means the
 * reply shows the moment it is sent without appearing twice when the
 * connection catches up.
 */
export function withMessage(thread: Message[], message: Message): Message[] {
  const merged = thread.filter((existing) => existing.id !== message.id)
  merged.push(message)

  return merged.sort((a, b) => a.sent_at.localeCompare(b.sent_at))
}

/** The time a message was sent, in the viewer's own zone. */
export function formatTime(iso: string): string {
  if (!iso) {
    return ''
  }

  const instant = new Date(iso)
  if (Number.isNaN(instant.getTime())) {
    return ''
  }

  return instant.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
