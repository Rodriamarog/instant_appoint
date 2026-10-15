// WhatsApp Embedded Signup helpers shared by the signup button and the token exchange.

export type SignupResult =
  | { kind: 'finish'; wabaId: string; phoneNumberId?: string }
  | { kind: 'cancel' }
  | { kind: 'error'; message: string }

// A normal signup ends with FINISH. Onboarding a number that stays on the WhatsApp
// Business app (coexistence) ends with FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING instead.
const FINISH_EVENTS = new Set(['FINISH', 'FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING'])

/** Reads a postMessage from Meta's signup popup. Null for anything that isn't one. */
export function parseSignupMessage(raw: unknown): SignupResult | null {
  if (typeof raw !== 'string') return null
  let msg: { type?: unknown; event?: unknown; data?: Record<string, unknown> }
  try {
    msg = JSON.parse(raw)
  } catch {
    return null
  }
  if (msg?.type !== 'WA_EMBEDDED_SIGNUP' || typeof msg.event !== 'string') return null

  if (FINISH_EVENTS.has(msg.event)) {
    const wabaId = msg.data?.waba_id
    if (typeof wabaId !== 'string' || !wabaId) return null
    const phoneNumberId = msg.data?.phone_number_id
    return { kind: 'finish', wabaId, phoneNumberId: typeof phoneNumberId === 'string' && phoneNumberId ? phoneNumberId : undefined }
  }
  if (msg.event === 'CANCEL') return { kind: 'cancel' }
  if (msg.event === 'ERROR') {
    const message = msg.data?.error_message
    return { kind: 'error', message: typeof message === 'string' ? message : 'Unknown error from Facebook' }
  }
  return null
}

/**
 * The signup's phone number id, or, when Meta didn't send one, the WABA's only number.
 * Null when it can't be told which number was onboarded.
 */
export async function resolvePhoneNumberId(
  wabaId: string,
  phoneNumberId: string | undefined,
  accessToken: string,
  fetchFn: typeof fetch = fetch,
): Promise<string | null> {
  if (phoneNumberId) return phoneNumberId
  const res = await fetchFn(`https://graph.facebook.com/v25.0/${wabaId}/phone_numbers?fields=id`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) return null
  const body = (await res.json()) as { data?: { id?: unknown }[] }
  const ids = (body.data ?? []).map(n => n.id).filter((id): id is string => typeof id === 'string')
  return ids.length === 1 ? ids[0] : null
}
