import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'

const VERIFY_TOKEN = 'neurocrow_webhook_verification'
const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function handleWebhookGet(request: NextRequest) {
  const { searchParams } = new URL(request.url)

  const mode = searchParams.get('hub.mode')
  const token = searchParams.get('hub.verify_token')
  const challenge = searchParams.get('hub.challenge')

  if (mode === 'subscribe' && token === VERIFY_TOKEN) {
    console.log('Facebook webhook verified')
    return new NextResponse(challenge, { status: 200 })
  }

  return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
}

export async function handleWebhookPost(request: NextRequest) {
  // Always return 200 fast — Meta requires quick ack
  try {
    const body = await request.json()

    const change = body?.entry?.[0]?.changes?.[0]
    if (!change) return NextResponse.json({ status: 'ok' }, { status: 200 })

    const field = change.field as string
    const value = change.value

    let customerPhone: string
    let messageText: string
    let messageId: string
    let direction: 'inbound' | 'outbound'

    if (field === 'messages') {
      // Inbound: customer → business
      if (!value?.messages?.length) return NextResponse.json({ status: 'ok' }, { status: 200 })
      const msg = value.messages[0]
      if (msg.type !== 'text') return NextResponse.json({ status: 'ok' }, { status: 200 })
      customerPhone = msg.from
      messageText = msg.text.body
      messageId = msg.id
      direction = 'inbound'
    } else if (field === 'smb_message_echoes') {
      // Outbound echo: business app → customer
      if (!value?.message_echoes?.length) return NextResponse.json({ status: 'ok' }, { status: 200 })
      const echo = value.message_echoes[0]
      if (echo.type !== 'text') return NextResponse.json({ status: 'ok' }, { status: 200 })
      customerPhone = echo.to
      messageText = echo.text.body
      messageId = echo.id
      direction = 'outbound'
    } else {
      return NextResponse.json({ status: 'ok' }, { status: 200 })
    }

    const phoneNumberId = value?.metadata?.phone_number_id as string
    if (!phoneNumberId) return NextResponse.json({ status: 'ok' }, { status: 200 })

    // Admin PB auth
    const adminPb = new PocketBase(PB_URL)
    await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

    // Resolve which business user owns this phone number
    let account
    try {
      account = await adminPb.collection('whatsapp_accounts').getFirstListItem(
        `phone_number_id = "${phoneNumberId}"`
      )
    } catch {
      console.warn('[webhook] No whatsapp_account found for phone_number_id:', phoneNumberId)
      return NextResponse.json({ status: 'ok' }, { status: 200 })
    }

    const userId = account.user_id as string
    const now = new Date().toISOString()

    // Upsert conversation
    let conv
    try {
      conv = await adminPb.collection('conversations').getFirstListItem(
        `user_id = "${userId}" && customer_phone = "${customerPhone}" && status = "active"`
      )
      await adminPb.collection('conversations').update(conv.id, { last_message_at: now })
    } catch {
      conv = await adminPb.collection('conversations').create({
        user_id: userId,
        customer_phone: customerPhone,
        status: 'active',
        last_message_at: now,
      })
    }

    // Save message
    await adminPb.collection('whatsapp_messages').create({
      user_id: userId,
      to_number: customerPhone,
      message_content: messageText,
      message_type: 'text',
      direction,
      conversation_id: conv.id,
      sent_at: now,
      message_id: messageId,
    })

    // TODO: AI analysis goes here (future step)

    return NextResponse.json({ status: 'ok' }, { status: 200 })
  } catch (error) {
    console.error('[webhook] Error processing event:', error)
    return NextResponse.json({ status: 'ok' }, { status: 200 })
  }
}
