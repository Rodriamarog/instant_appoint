/**
 * Reading Meta's Embedded Signup popup.
 *
 * This is the only part of the signup that stays in the browser: Meta reports
 * the result by posting a message to the window that opened the popup, so
 * nothing on the server can see it. Everything the result leads to — the token
 * exchange, the phone-number lookup, the webhook subscription — happens on the
 * server, because it needs the Meta app secret.
 */

export type SignupResult =
  | { kind: 'finish'; wabaId: string; phoneNumberId?: string }
  | { kind: 'cancel' }
  | { kind: 'error'; message: string }

/**
 * A normal signup ends with FINISH. A business that keeps using the WhatsApp
 * Business app on their phone — Meta calls it coexistence, and it is the
 * common case for a small business — ends with the second event instead, and
 * usually without a phone number id.
 */
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
    // Without a WABA there is nothing the server can act on, so this is not a
    // finish worth reporting.
    if (typeof wabaId !== 'string' || !wabaId) return null

    const phoneNumberId = msg.data?.phone_number_id

    return {
      kind: 'finish',
      wabaId,
      phoneNumberId:
        typeof phoneNumberId === 'string' && phoneNumberId ? phoneNumberId : undefined,
    }
  }

  if (msg.event === 'CANCEL') return { kind: 'cancel' }

  if (msg.event === 'ERROR') {
    const message = msg.data?.error_message
    return {
      kind: 'error',
      message: typeof message === 'string' ? message : 'Unknown error from Facebook',
    }
  }

  return null
}
