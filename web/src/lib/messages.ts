import { spellTime } from '@/lib/dates'
import { liveFeed, pollingFeed, type Feed } from '@/lib/feed'
import { pb } from '@/lib/pocketbase'

/**
 * whatsapp_messages: the one module that knows this collection's name, its
 * shape and its filters.
 *
 * It is a shared log. A patient's message, the assistant's answer and an
 * appointment reminder are all rows in it, told apart by message_type — which
 * is why the reminders page's reads live here too rather than in a second
 * module with a second name for the same collection and a second shape for
 * the same rows.
 *
 * Reading is all this module does, and it is the browser talking to
 * PocketBase directly: whatsapp_messages is owner-read, so the access rules
 * scope the thread to the person asking, and a message belonging to someone
 * else's conversation is not returned however it is asked for. Nothing here
 * writes. Every row in the collection is put there by the webhook or by the
 * reminder job, both of them server-side, and the collection is closed to
 * writes from the browser besides.
 */
export const COLLECTION = 'whatsapp_messages'

/** One message in the log. */
export interface Message {
  id: string
  user_id: string
  conversation_id: string
  direction: 'inbound' | 'outbound'
  message_content: string
  message_type: string
  message_id: string
  sent_at: string
  /** Where it went. Empty on an inbound message. */
  to_number: string
  /**
   * The appointment the row is about: the one a reminder went out for, or
   * the one a tap on its buttons moved. Empty on everything else.
   */
  event_id: string
  status: string
}

/**
 * The message_type the webhook writes when a patient taps Confirm or Cancel
 * under a reminder instead of typing a reply.
 *
 * The two read the same in a thread — the row says "Cancelar" either way —
 * and they are not the same thing. Only a tap is something this app acted
 * on, and only a tap can have failed to find the appointment it was about.
 * Spelled the same as webhook.KindTap on the Go side, which is what writes
 * it.
 */
export const TAP = 'button'

/**
 * Whether this is a tap that moved nothing.
 *
 * A tap is matched to an appointment through the reminder it answers, and
 * three things have to line up for that: the delivery has to say which
 * message it answers, that message has to be a reminder this clinic sent,
 * and the button has to be one the app can name. When they do not line up
 * the row is written with no appointment against it rather than guessed at,
 * the alternative being a cancellation applied to the wrong patient.
 *
 * An empty event_id on a tap is therefore not a missing value. It is the
 * record that the patient pressed a button and the booking did not move —
 * the one thing in a thread somebody has to go and do by hand.
 */
export function isUnmatchedTap(message: Message): boolean {
  return message.message_type === TAP && message.event_id === ''
}

/**
 * The two markers that make a row in this shared log a reminder that went
 * out. Both are in the predicate of the unique index the scheduler claims
 * against, so neither is a label: changing either changes what the database
 * enforces.
 */
const SENT_REMINDER = 'message_type = "reminder" && status = "sent"'

/** How many appointment ids go into one filter. */
const EVENT_ID_CHUNK = 50

/**
 * The reminders that have gone out, most recent first.
 *
 * Capped, because this is what the page lists and a page does not show a
 * thousand rows. Deciding whether a reminder is still *upcoming* must not
 * read this: see remindedEventIds.
 */
export async function listSentReminders(perPage = 50): Promise<Message[]> {
  const owner = pb.authStore.record?.id ?? ''

  const page = await pb.collection(COLLECTION).getList<Message>(1, perPage, {
    filter: pb.filter(`user_id = {:userId} && ${SENT_REMINDER}`, { userId: owner }),
    sort: '-sent_at',
  })

  return page.items
}

/**
 * Which of these appointments have already had their reminder sent.
 *
 * Scoped to the appointments asked about rather than to a page of history.
 * The exclusion this feeds has to be complete — a sent reminder missing from
 * it reappears as upcoming and due, and the page says a reminder is about to
 * go out that went out weeks ago — so it can be neither capped nor
 * approximated by the most recent N rows.
 *
 * The ids go in as a chain of ORs because PocketBase's filter language has no
 * IN, and in chunks because that chain becomes the query string.
 */
export async function remindedEventIds(eventIds: string[]): Promise<Set<string>> {
  const unique = [...new Set(eventIds.filter(Boolean))]
  if (unique.length === 0) {
    return new Set()
  }

  const owner = pb.authStore.record?.id ?? ''
  const chunks: string[][] = []
  for (let i = 0; i < unique.length; i += EVENT_ID_CHUNK) {
    chunks.push(unique.slice(i, i + EVENT_ID_CHUNK))
  }

  const pages = await Promise.all(
    chunks.map((chunk) => {
      const params: Record<string, string> = { userId: owner }
      const clauses = chunk.map((id, index) => {
        params[`e${index}`] = id

        return `event_id = {:e${index}}`
      })

      return pb.collection(COLLECTION).getFullList<Message>({
        filter: pb.filter(
          `user_id = {:userId} && ${SENT_REMINDER} && (${clauses.join(' || ')})`,
          params,
        ),
      })
    }),
  )

  return new Set(pages.flat().map((message) => message.event_id))
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
 * The time a message was sent, in the viewer's own zone.
 *
 * The clock comes from lib/dates rather than toLocaleTimeString, which reads
 * the *browser's* locale: a machine left on en-US printed "03:30 PM" into a
 * Spanish thread, and a thread is the one place where two timestamps have to
 * be comparable at a glance.
 */
export function formatTime(iso: string): string {
  if (!iso) {
    return ''
  }

  const instant = new Date(iso)
  if (Number.isNaN(instant.getTime())) {
    return ''
  }

  return spellTime(instant)
}
