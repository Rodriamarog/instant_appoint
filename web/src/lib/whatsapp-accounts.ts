import { pb } from '@/lib/pocketbase'

/**
 * Connected WhatsApp numbers, as the app talks about them. This module is the
 * only place that knows the collection is called whatsapp_accounts and how a
 * connected Cloud API account is told apart from a retired QR one.
 *
 * Unlike calendar-events, nothing here writes: the collection's create,
 * update and delete rules are nil, so a browser cannot touch a row at all. An
 * account is only ever written by the Go handler behind
 * POST /api/whatsapp/exchange-token, which has the Meta app secret the
 * exchange needs. Reads are scoped by the list rule.
 */
export const COLLECTION = 'whatsapp_accounts'

/**
 * A connected number.
 *
 * access_token, session_id and qr_code are deliberately absent. They are
 * hidden fields: PocketBase strips them from every response and answers a
 * client filter or sort naming one with a 400, so nothing in this module may
 * mention them.
 */
export interface WhatsAppAccount {
  id: string
  phone_number: string
  phone_number_id: string
  waba_id: string
  verified_name: string
}

/**
 * The signed-in person's connected Cloud API numbers, newest first.
 *
 * Ownership is enforced by the collection's list rule, which is the only
 * check a browser cannot talk its way around; naming the user in the filter
 * as well keeps the query honest about what it expects back, and means an
 * expired session lists nothing rather than everything.
 */
export async function listConnectedAccounts(): Promise<WhatsAppAccount[]> {
  const filter = pb.filter(
    'user_id = {:userId} && account_type = "business_api" && is_active = true',
    { userId: pb.authStore.record?.id ?? '' },
  )

  return pb.collection(COLLECTION).getFullList<WhatsAppAccount>({
    filter,
    sort: '-created',
  })
}

/**
 * The number Meta assigned, preferred over the raw id when something stored
 * one. The Cloud API path never writes phone_number — only the retired QR
 * service did — so the fallback is the usual case, not the exception.
 */
export function displayNumber(account: WhatsAppAccount): string {
  return `+${account.phone_number || account.phone_number_id}`
}
