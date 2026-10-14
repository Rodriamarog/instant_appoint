import { createHmac } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { createWebhookPostHandler, type StoredMessage, type WebhookStore } from '../facebook-webhook'

const SECRET = 'test-router-secret'
const NOW = new Date('2026-10-13T12:00:00Z')
const nowSec = Math.floor(NOW.getTime() / 1000)

// Signed exactly as the router does: sha256=<hex HMAC-SHA256(secret, timestamp + "." + body)>
function sign(secret: string, timestamp: number | string, body: string) {
  return 'sha256=' + createHmac('sha256', secret).update(`${timestamp}.${body}`).digest('hex')
}

function routerRequest(payload: unknown, opts: { timestamp?: number; secret?: string; signature?: string } = {}) {
  const body = JSON.stringify(payload)
  const timestamp = opts.timestamp ?? nowSec
  return new Request('http://localhost/api/webhook', {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      'x-neurocrow-timestamp': String(timestamp),
      'x-neurocrow-signature-256': opts.signature ?? sign(opts.secret ?? SECRET, timestamp, body),
      'x-neurocrow-delivery-id': 'd-1',
    },
    body,
  })
}

const PHONE_NUMBER_ID = '555000111'
const UNKNOWN_PHONE_NUMBER_ID = '1078215982040649'

// In-memory stand-in for PocketBase.
function fakeStore(accounts: Record<string, { userId: string; accessToken: string }> = {
  [PHONE_NUMBER_ID]: { userId: 'user-1', accessToken: 'token-1' },
}) {
  const messages: (StoredMessage & { userId: string })[] = []
  const conversations: { id: string; userId: string; customerPhone: string }[] = []
  const store: WebhookStore = {
    async findAccount(phoneNumberId) {
      return accounts[phoneNumberId] ?? null
    },
    async hasMessage(messageId) {
      return messages.some(m => m.messageId === messageId)
    },
    async upsertConversation(userId, customerPhone) {
      let conv = conversations.find(c => c.userId === userId && c.customerPhone === customerPhone)
      if (!conv) {
        conv = { id: `conv-${conversations.length + 1}`, userId, customerPhone }
        conversations.push(conv)
      }
      return conv.id
    },
    async saveMessage(userId, message) {
      messages.push({ ...message, userId })
    },
    async conversationHistory(conversationId) {
      return messages.filter(m => m.conversationId === conversationId)
    },
  }
  return { store, messages }
}

function textMessage(id: string, from: string, body: string, sentAt: Date = NOW) {
  return { id, from, type: 'text', text: { body }, timestamp: String(Math.floor(sentAt.getTime() / 1000)) }
}

function whatsappPayload(changes: { phoneNumberId?: string; messages?: unknown[]; field?: string; value?: unknown }[][]) {
  return {
    object: 'whatsapp_business_account',
    entry: changes.map((entryChanges, i) => ({
      id: `waba-${i}`,
      changes: entryChanges.map(c => ({
        field: c.field ?? 'messages',
        value: c.value ?? {
          messaging_product: 'whatsapp',
          metadata: { phone_number_id: c.phoneNumberId ?? PHONE_NUMBER_ID },
          messages: c.messages ?? [],
        },
      })),
    })),
  }
}

function setup(overrides: { store?: WebhookStore; secret?: string } = {}) {
  const { store, messages } = fakeStore()
  const sent: { phoneNumberId: string; to: string; text: string }[] = []
  const handler = createWebhookPostHandler({
    secret: overrides.secret ?? SECRET,
    now: () => NOW,
    store: overrides.store ?? store,
    generateReply: async (_history, text) => `reply to: ${text}`,
    sendMessage: async (phoneNumberId, _token, to, text) => {
      sent.push({ phoneNumberId, to, text })
      return `wamid.reply-${sent.length}`
    },
  })
  return { handler, messages, sent }
}

describe('webhook POST from the router', () => {
  it('rejects a POST without a signature', async () => {
    const handler = setup().handler
    const res = await handler(
      new Request('http://localhost/api/webhook', { method: 'POST', body: '{"entry":[]}' })
    )
    expect(res.status).toBe(401)
  })

  it('rejects a POST signed with a different secret', async () => {
    const handler = setup().handler
    const res = await handler(routerRequest({ entry: [] }, { secret: 'someone-else' }))
    expect(res.status).toBe(401)
  })

  it('rejects a correctly signed POST whose timestamp is more than 5 minutes old', async () => {
    const handler = setup().handler
    const res = await handler(routerRequest({ entry: [] }, { timestamp: nowSec - 6 * 60 }))
    expect(res.status).toBe(401)
  })

  it('rejects a correctly signed POST whose timestamp is more than 5 minutes in the future', async () => {
    const handler = setup().handler
    const res = await handler(routerRequest({ entry: [] }, { timestamp: nowSec + 6 * 60 }))
    expect(res.status).toBe(401)
  })

  it('rejects everything when no secret is configured', async () => {
    const handler = setup({ secret: '' }).handler
    const res = await handler(routerRequest({ entry: [] }, { secret: '' }))
    expect(res.status).toBe(401)
  })

  it('accepts a correctly signed POST from 4 minutes ago', async () => {
    const handler = setup().handler
    const res = await handler(routerRequest({ entry: [] }, { timestamp: nowSec - 4 * 60 }))
    expect(res.status).toBe(200)
  })
})

describe('processing a signed WhatsApp payload', () => {
  it('stores an inbound text message and sends and stores an AI reply', async () => {
    const { handler, messages, sent } = setup()
    const res = await handler(routerRequest(whatsappPayload([[{ messages: [textMessage('wamid.1', '5215511111111', 'hola')] }]])))

    expect(res.status).toBe(200)
    expect(sent).toEqual([{ phoneNumberId: PHONE_NUMBER_ID, to: '5215511111111', text: 'reply to: hola' }])
    expect(messages.map(m => [m.messageId, m.direction, m.content])).toEqual([
      ['wamid.1', 'inbound', 'hola'],
      ['wamid.reply-1', 'outbound', 'reply to: hola'],
    ])
  })

  it('processes every message in every change of every entry', async () => {
    const { handler, messages, sent } = setup()
    const payload = whatsappPayload([
      [
        { messages: [textMessage('wamid.a1', '521000000001', 'uno'), textMessage('wamid.a2', '521000000002', 'dos')] },
        { messages: [textMessage('wamid.b1', '521000000003', 'tres')] },
      ],
      [{ messages: [textMessage('wamid.c1', '521000000004', 'cuatro')] }],
    ])
    const res = await handler(routerRequest(payload))

    expect(res.status).toBe(200)
    expect(messages.filter(m => m.direction === 'inbound').map(m => m.messageId)).toEqual([
      'wamid.a1',
      'wamid.a2',
      'wamid.b1',
      'wamid.c1',
    ])
    expect(sent.map(s => s.to)).toEqual(['521000000001', '521000000002', '521000000003', '521000000004'])
  })

  it('stores outbound echoes from the WhatsApp Business app without replying', async () => {
    const { handler, messages, sent } = setup()
    const payload = whatsappPayload([
      [
        {
          field: 'smb_message_echoes',
          value: {
            metadata: { phone_number_id: PHONE_NUMBER_ID },
            message_echoes: [{ id: 'wamid.echo1', to: '521000000009', type: 'text', text: { body: 'ya voy' }, timestamp: String(nowSec) }],
          },
        },
      ],
    ])
    await handler(routerRequest(payload))

    expect(messages.map(m => [m.messageId, m.direction, m.customerPhone])).toEqual([['wamid.echo1', 'outbound', '521000000009']])
    expect(sent).toEqual([])
  })

  it('ignores a redelivered message it has already stored (same WhatsApp message id)', async () => {
    const { handler, messages, sent } = setup()
    const payload = whatsappPayload([[{ messages: [textMessage('wamid.dup', '5215511111111', 'hola')] }]])

    expect((await handler(routerRequest(payload))).status).toBe(200)
    expect((await handler(routerRequest(payload))).status).toBe(200)

    expect(messages.filter(m => m.messageId === 'wamid.dup')).toHaveLength(1)
    expect(sent).toHaveLength(1)
  })

  it('handles the same message id twice within one payload only once', async () => {
    const { handler, messages, sent } = setup()
    const msg = textMessage('wamid.twice', '5215511111111', 'hola')
    await handler(routerRequest(whatsappPayload([[{ messages: [msg] }], [{ messages: [msg] }]])))

    expect(messages.filter(m => m.messageId === 'wamid.twice')).toHaveLength(1)
    expect(sent).toHaveLength(1)
  })

  it('stores a message sent more than 15 minutes ago but does not auto-reply to it', async () => {
    const { handler, messages, sent } = setup()
    const sentAt = new Date(NOW.getTime() - 16 * 60 * 1000)
    const res = await handler(routerRequest(whatsappPayload([[{ messages: [textMessage('wamid.late', '5215511111111', 'hola?', sentAt)] }]])))

    expect(res.status).toBe(200)
    expect(messages.map(m => m.messageId)).toEqual(['wamid.late'])
    expect(sent).toEqual([])
  })

  it('still auto-replies to a message sent 14 minutes ago', async () => {
    const { handler, sent } = setup()
    const sentAt = new Date(NOW.getTime() - 14 * 60 * 1000)
    await handler(routerRequest(whatsappPayload([[{ messages: [textMessage('wamid.recent', '5215511111111', 'hola', sentAt)] }]])))

    expect(sent).toHaveLength(1)
  })

  it('does not auto-reply to a message with no timestamp', async () => {
    const { handler, messages, sent } = setup()
    const noTimestamp = { ...textMessage('wamid.nots', '5215511111111', 'hola'), timestamp: undefined }
    await handler(routerRequest(whatsappPayload([[{ messages: [noTimestamp] }]])))

    expect(messages.map(m => m.messageId)).toEqual(['wamid.nots'])
    expect(sent).toEqual([])
  })
})

describe('numbers without an account', () => {
  it('ignores their messages and still processes the other numbers in the payload', async () => {
    const { handler, messages, sent } = setup()
    const payload = whatsappPayload([
      [{ phoneNumberId: UNKNOWN_PHONE_NUMBER_ID, messages: [textMessage('wamid.unknown', '521000000001', 'cita')] }],
      [{ messages: [textMessage('wamid.ia', '521000000002', 'hola')] }],
    ])
    const res = await handler(routerRequest(payload))

    expect(res.status).toBe(200)
    expect(messages.filter(m => m.direction === 'inbound').map(m => m.messageId)).toEqual(['wamid.ia'])
    expect(sent.map(s => s.phoneNumberId)).toEqual([PHONE_NUMBER_ID])
  })
})

describe('failures', () => {
  it('returns 500 when storing fails, so the router retries the delivery', async () => {
    const { store } = fakeStore()
    const { handler } = setup({
      store: { ...store, saveMessage: async () => { throw new Error('PocketBase down') } },
    })
    const res = await handler(routerRequest(whatsappPayload([[{ messages: [textMessage('wamid.f', '521000000001', 'hola')] }]])))
    expect(res.status).toBe(500)
  })

  it('still returns 200 when the AI reply fails (the message is stored)', async () => {
    const { store, messages } = fakeStore()
    const handler = createWebhookPostHandler({
      secret: SECRET,
      now: () => NOW,
      store,
      generateReply: async () => { throw new Error('Gemini down') },
      sendMessage: async () => 'never',
    })
    const res = await handler(routerRequest(whatsappPayload([[{ messages: [textMessage('wamid.g', '521000000001', 'hola')] }]])))
    expect(res.status).toBe(200)
    expect(messages.map(m => m.messageId)).toEqual(['wamid.g'])
  })
})

describe('webhook routes', () => {
  it.each(['@/app/api/webhook/route', '@/app/webhook/route'])('%s no longer answers Meta GET verification', async path => {
    const route = await import(path)
    expect(route.GET).toBeUndefined()
    expect(typeof route.POST).toBe('function')
  })
})

