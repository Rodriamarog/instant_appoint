import PocketBase from 'pocketbase'
import { generateReply, ConversationMessage } from './ai-conversation'
import { sendWhatsAppCloudMessage } from './whatsapp-cloud-api'
import { isValidRouterDelivery } from './router-signature'

// WhatsApp webhooks reach this app only through the Neurocrow webhook router
// (hooks.neurocrow.com). The router answers Meta's GET verification, checks Meta's
// signature, and re-signs each delivery with NEUROCROW_ROUTER_SECRET.

// No AI auto-reply to a message older than this, by its own WhatsApp timestamp.
const MAX_AUTO_REPLY_AGE_MS = 15 * 60 * 1000
const PB_URL = process.env.POCKETBASE_INTERNAL_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export interface StoredMessage {
  messageId: string
  conversationId: string
  customerPhone: string
  content: string
  direction: 'inbound' | 'outbound'
  sentAt: string
}

/** Persistence used by the webhook (PocketBase in production). */
export interface WebhookStore {
  findAccount(phoneNumberId: string): Promise<{ userId: string; accessToken: string } | null>
  hasMessage(messageId: string): Promise<boolean>
  /** Returns the id of the customer's active conversation, creating it if needed. */
  upsertConversation(userId: string, customerPhone: string, phoneNumberId: string, at: string): Promise<string>
  saveMessage(userId: string, message: StoredMessage): Promise<void>
  /** All messages of a conversation, oldest first. */
  conversationHistory(conversationId: string): Promise<StoredMessage[]>
}

export interface WebhookDeps {
  secret: string
  now: () => Date
  store: WebhookStore
  generateReply: (history: ConversationMessage[], text: string) => Promise<string>
  sendMessage: (phoneNumberId: string, accessToken: string, to: string, text: string) => Promise<string>
}

interface IncomingText {
  phoneNumberId: string
  customerPhone: string
  content: string
  messageId: string
  direction: 'inbound' | 'outbound'
  timestamp?: number
}

// The parts of a Meta WhatsApp webhook payload this handler reads. Everything is
// optional because the payload is untrusted JSON.
interface MetaMessage {
  id?: string
  type?: string
  from?: string
  to?: string
  timestamp?: string
  text?: { body?: unknown }
}
interface MetaChange {
  field?: string
  value?: {
    metadata?: { phone_number_id?: unknown }
    messages?: MetaMessage[]
    message_echoes?: MetaMessage[]
  }
}

/** Every change of every entry in the payload. */
function allChanges(payload: unknown): MetaChange[] {
  const entries = (payload as { entry?: unknown } | null)?.entry
  if (!Array.isArray(entries)) return []
  return entries.flatMap(entry => (Array.isArray(entry?.changes) ? (entry.changes as MetaChange[]) : []))
}

/** Pulls every text message (inbound) and echo (outbound) out of every entry and change. */
function extractTexts(payload: unknown): IncomingText[] {
  const out: IncomingText[] = []
  for (const change of allChanges(payload)) {
    const value = change?.value
    const phoneNumberId = value?.metadata?.phone_number_id
    if (typeof phoneNumberId !== 'string' || !phoneNumberId) continue

    let items: unknown
    let direction: 'inbound' | 'outbound'
    if (change.field === 'messages') {
      items = value?.messages
      direction = 'inbound'
    } else if (change.field === 'smb_message_echoes') {
      items = value?.message_echoes
      direction = 'outbound'
    } else {
      continue
    }
    if (!Array.isArray(items)) continue

    for (const item of items as MetaMessage[]) {
      const customerPhone = direction === 'inbound' ? item?.from : item?.to
      if (item?.type !== 'text' || typeof item.text?.body !== 'string' || !item.id || !customerPhone) continue
      const ts = Number(item.timestamp)
      out.push({
        phoneNumberId,
        customerPhone,
        content: item.text.body,
        messageId: item.id,
        direction,
        timestamp: Number.isFinite(ts) && ts > 0 ? ts : undefined,
      })
    }
  }
  return out
}

async function processText(deps: WebhookDeps, msg: IncomingText) {
  const { store } = deps
  // The router delivers at least once, so the same message can arrive more than once.
  if (await store.hasMessage(msg.messageId)) {
    console.log('[webhook] duplicate message ignored:', msg.messageId)
    return
  }
  const account = await store.findAccount(msg.phoneNumberId)
  if (!account) {
    console.warn('[webhook] No whatsapp_account for phone_number_id:', msg.phoneNumberId)
    return
  }

  const now = deps.now().toISOString()
  const conversationId = await store.upsertConversation(account.userId, msg.customerPhone, msg.phoneNumberId, now)
  await store.saveMessage(account.userId, {
    messageId: msg.messageId,
    conversationId,
    customerPhone: msg.customerPhone,
    content: msg.content,
    direction: msg.direction,
    sentAt: now,
  })

  // Only generate AI reply for inbound messages (not echo captures)
  if (msg.direction !== 'inbound') return

  // A late delivery (e.g. after an outage) is stored but never answered by the bot.
  const ageMs = msg.timestamp === undefined ? Infinity : deps.now().getTime() - msg.timestamp * 1000
  if (ageMs > MAX_AUTO_REPLY_AGE_MS) {
    console.log('[webhook] message too old for auto-reply:', msg.messageId)
    return
  }

  try {
    const history = await store.conversationHistory(conversationId)
    const conversationHistory: ConversationMessage[] = history
      .filter(m => m.messageId !== msg.messageId) // exclude the message we just saved
      .map(m => ({ role: m.direction === 'inbound' ? 'user' : 'model', content: m.content }))

    const replyText = await deps.generateReply(conversationHistory, msg.content)
    const replyMessageId = await deps.sendMessage(msg.phoneNumberId, account.accessToken, msg.customerPhone, replyText)

    await store.saveMessage(account.userId, {
      messageId: replyMessageId,
      conversationId,
      customerPhone: msg.customerPhone,
      content: replyText,
      direction: 'outbound',
      sentAt: deps.now().toISOString(),
    })
    console.log(`[webhook] AI reply sent to ${msg.customerPhone}, wamid: ${replyMessageId}`)
  } catch (aiError) {
    console.error('[webhook] AI/send step failed:', aiError)
  }
}

export function createWebhookPostHandler(deps: WebhookDeps) {
  // Message ids being processed right now, so two concurrent deliveries of the
  // same message can't both pass the hasMessage check.
  const inFlight = new Set<string>()

  return async function POST(request: Request): Promise<Response> {
    const rawBody = await request.text()
    if (!isValidRouterDelivery(deps.secret, rawBody, request.headers, deps.now())) {
      return Response.json({ error: 'invalid signature' }, { status: 401 })
    }

    let payload: unknown
    try {
      payload = JSON.parse(rawBody)
    } catch {
      return Response.json({ error: 'invalid JSON' }, { status: 400 })
    }

    try {
      for (const msg of extractTexts(payload)) {
        if (inFlight.has(msg.messageId)) continue
        inFlight.add(msg.messageId)
        try {
          await processText(deps, msg)
        } finally {
          inFlight.delete(msg.messageId)
        }
      }
    } catch (error) {
      // Non-2xx makes the router retry; dedupe makes the retry safe.
      console.error('[webhook] error:', error)
      return Response.json({ error: 'processing failed' }, { status: 500 })
    }
    return Response.json({ status: 'ok' }, { status: 200 })
  }
}

function isNotFound(err: unknown) {
  return (err as { status?: number })?.status === 404
}

function pocketBaseStore(): WebhookStore {
  const client = new PocketBase(PB_URL)
  client.autoCancellation(false) // concurrent webhook requests share this client
  // Re-authenticate whenever the superuser token is missing or expired.
  const pb = async () => {
    if (!client.authStore.isValid) {
      await client.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)
    }
    return client
  }

  return {
    async findAccount(phoneNumberId) {
      const client = await pb()
      try {
        const account = await client
          .collection('whatsapp_accounts')
          .getFirstListItem(client.filter('phone_number_id = {:phoneNumberId}', { phoneNumberId }))
        return { userId: account.user_id as string, accessToken: account.access_token as string }
      } catch (err) {
        if (isNotFound(err)) return null
        throw err
      }
    },
    async hasMessage(messageId) {
      const client = await pb()
      const res = await client
        .collection('whatsapp_messages')
        .getList(1, 1, { filter: client.filter('message_id = {:messageId}', { messageId }) })
      return res.totalItems > 0
    },
    async upsertConversation(userId, customerPhone, phoneNumberId, at) {
      const client = await pb()
      try {
        const conv = await client
          .collection('conversations')
          .getFirstListItem(
            client.filter('user_id = {:userId} && customer_phone = {:customerPhone} && status = "active"', {
              userId,
              customerPhone,
            })
          )
        await client.collection('conversations').update(conv.id, { last_message_at: at, phone_number_id: phoneNumberId })
        return conv.id
      } catch (err) {
        if (!isNotFound(err)) throw err
        const conv = await client.collection('conversations').create({
          user_id: userId,
          customer_phone: customerPhone,
          status: 'active',
          last_message_at: at,
          phone_number_id: phoneNumberId,
        })
        return conv.id
      }
    },
    async saveMessage(userId, m) {
      await (await pb()).collection('whatsapp_messages').create({
        user_id: userId,
        to_number: m.customerPhone,
        message_content: m.content,
        message_type: 'text',
        direction: m.direction,
        conversation_id: m.conversationId,
        sent_at: m.sentAt,
        message_id: m.messageId,
      })
    },
    async conversationHistory(conversationId) {
      const client = await pb()
      const records = await client.collection('whatsapp_messages').getFullList({
        filter: client.filter('conversation_id = {:conversationId}', { conversationId }),
        sort: 'sent_at',
      })
      return records.map(r => ({
        messageId: r.message_id as string,
        conversationId: r.conversation_id as string,
        customerPhone: r.to_number as string,
        content: r.message_content as string,
        direction: r.direction as 'inbound' | 'outbound',
        sentAt: r.sent_at as string,
      }))
    },
  }
}

let handler: ((request: Request) => Promise<Response>) | null = null

export function handleWebhookPost(request: Request) {
  // Built on first request so env vars are read at runtime, not at build time.
  handler ??= createWebhookPostHandler({
    secret: process.env.NEUROCROW_ROUTER_SECRET ?? '',
    now: () => new Date(),
    store: pocketBaseStore(),
    generateReply,
    sendMessage: sendWhatsAppCloudMessage,
  })
  return handler(request)
}
