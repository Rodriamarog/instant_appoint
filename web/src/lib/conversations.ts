import { liveFeed, pollingFeed, type Feed } from '@/lib/feed'
import { pb } from '@/lib/pocketbase'

/**
 * Customer threads, as the chat page talks about them. This module is the only
 * place that knows the collection is called conversations and how "most recent
 * activity first" is expressed.
 *
 * There is no server-side handler in front of it. The collection is owner-read
 * — list and view are `user_id = @request.auth.id` — so the access rules are
 * what scope the list to the person asking, and the query below deliberately
 * does not repeat that as a filter: a filter the browser sends is a filter the
 * browser can change, and adding one would suggest it was doing the work.
 *
 * Writes are closed to the browser entirely, which is why replying goes
 * through the Go route in lib/messages.ts rather than through a create here.
 */
export const COLLECTION = 'conversations'

/** One customer's thread. */
export interface Conversation {
  id: string
  user_id: string
  customer_phone: string
  status: string
  last_message_at: string
  phone_number_id: string
}

/**
 * Every conversation this person has, most recent activity first.
 *
 * last_message_at is a text column, so this sort is a byte-by-byte string
 * comparison. It only means "most recent" while every writer emits the same
 * fixed-width shape — see lib/iso-time.ts, and internal/isotime on the other
 * side.
 */
export async function listConversations(): Promise<Conversation[]> {
  return pb.collection(COLLECTION).getFullList<Conversation>({
    sort: '-last_message_at',
  })
}

/** The conversation list, kept current over the live connection. */
export const liveConversations: Feed<Conversation> = liveFeed({
  collection: COLLECTION,
  load: listConversations,
})

/**
 * The same list, asked for again every few seconds.
 *
 * The page took this route before, at five seconds. It is kept — written and
 * tested — because the live connection is only proven locally: a proxy in
 * front of the app can cut a long-lived stream on an idle timeout, and the
 * symptom is a list that quietly stops moving. Swapping is changing which of
 * these two the page is given.
 */
export function polledConversations(everyMs = 5000): Feed<Conversation> {
  return pollingFeed({ load: listConversations, everyMs })
}

/** The customer's number as a person reads it. */
export function formatPhone(phone: string): string {
  return phone ? `+${phone}` : 'Unknown number'
}
