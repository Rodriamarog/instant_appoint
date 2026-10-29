import { pb } from '@/lib/pocketbase'

/**
 * Whether this person has a WhatsApp number that can actually deliver a
 * message.
 *
 * This is the replacement for `GET /api/whatsapp/status`, which the reminders
 * page read a `status` key off that the route never returned — so the amber
 * "WhatsApp not connected" banner showed permanently, for everybody,
 * including fully connected Cloud API accounts. Asking the collection
 * directly removes the shape mismatch rather than papering over it.
 *
 * web/src/pages/whatsapp.tsx runs the same query inline to list the numbers
 * themselves; it is a candidate to adopt this module once the parallel
 * tickets are merged.
 */
export const COLLECTION = 'whatsapp_accounts'

/** The only kind of account that can send anything today. */
export const BUSINESS_API = 'business_api'

export async function isWhatsAppConnected(): Promise<boolean> {
  const owner = pb.authStore.record?.id ?? ''

  try {
    // The list rule already scopes this to the caller; naming the owner as
    // well keeps the query honest about what it expects back.
    //
    // Nothing here may mention access_token, qr_code or session_id: they are
    // hidden fields, and PocketBase answers a filter naming one with a 400,
    // which would come back out of here as a false "not connected".
    await pb.collection(COLLECTION).getFirstListItem(
      pb.filter(
        `user_id = {:userId} && account_type = "${BUSINESS_API}" && is_active = true`,
        { userId: owner },
      ),
    )

    return true
  } catch (error) {
    if ((error as { status?: number } | null)?.status === 404) {
      return false
    }
    throw error
  }
}
