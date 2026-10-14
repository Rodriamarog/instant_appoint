import { createHmac, timingSafeEqual } from 'node:crypto'

// Headers set by the Neurocrow webhook router (hooks.neurocrow.com) on every delivery.
export const SIGNATURE_HEADER = 'x-neurocrow-signature-256'
export const TIMESTAMP_HEADER = 'x-neurocrow-timestamp'
export const DELIVERY_ID_HEADER = 'x-neurocrow-delivery-id'

const MAX_SKEW_SECONDS = 5 * 60

/**
 * Checks a router delivery: the signature header must be
 * "sha256=<hex HMAC-SHA256(secret, timestamp + "." + rawBody)>" and the
 * timestamp (unix seconds) must be within 5 minutes of now.
 */
export function isValidRouterDelivery(
  secret: string,
  rawBody: string,
  headers: Headers,
  now: Date
): boolean {
  if (!secret) return false
  const signature = headers.get(SIGNATURE_HEADER)
  const timestamp = headers.get(TIMESTAMP_HEADER)
  if (!signature?.startsWith('sha256=') || !timestamp || !/^\d+$/.test(timestamp)) return false

  if (Math.abs(Math.floor(now.getTime() / 1000) - Number(timestamp)) > MAX_SKEW_SECONDS) return false

  const expected = createHmac('sha256', secret).update(`${timestamp}.${rawBody}`).digest()
  const got = Buffer.from(signature.slice('sha256='.length), 'hex')
  return got.length === expected.length && timingSafeEqual(got, expected)
}
