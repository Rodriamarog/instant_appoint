import { spellTime } from '@/lib/dates'
import { isoMillis } from '@/lib/iso-time'
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
 *
 * A tap carries the appointment it moved in event_id, and an empty one is
 * not a missing value: three things have to line up for a tap to be matched
 * — the delivery has to say which message it answers, that message has to be
 * a reminder this clinic sent, and the button has to be one the app can name
 * — and when they do not, the row is written with no appointment rather than
 * guessed at, the alternative being a cancellation applied to the wrong
 * patient.
 */
export const TAP = 'button'

/**
 * How far back an unmatched tap is still worth putting in front of somebody.
 *
 * Nothing marks one of these as dealt with — there is no column for it and
 * no screen to tick it off on — so an unbounded list would only ever grow
 * into a warning nobody reads. A week is longer than any lead time the
 * settings dialog offers, so a tap that drops off the end is one answering a
 * reminder for an appointment that has already happened.
 */
export const UNMATCHED_TAP_DAYS = 7

/**
 * The taps that changed nothing, most recent first.
 *
 * Each one is a patient who pressed Confirm or Cancel and whose appointment
 * did not move. Nothing else in the app says so: the booking looks exactly
 * as it did before the reminder went out, which is why this is asked for
 * rather than left in the log for somebody to find.
 *
 * The lower bound is compared against a text column, so it has to be
 * written in the same fixed-width shape the stored values are in.
 */
export async function listUnmatchedTaps(now = new Date()): Promise<Message[]> {
  const owner = pb.authStore.record?.id ?? ''
  const since = new Date(now.getTime() - UNMATCHED_TAP_DAYS * 24 * 60 * 60 * 1000)

  return pb.collection(COLLECTION).getFullList<Message>({
    filter: pb.filter(
      `user_id = {:userId} && message_type = "${TAP}" && event_id = "" && sent_at >= {:since}`,
      { userId: owner, since: isoMillis(since) },
    ),
    sort: '-sent_at',
  })
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
