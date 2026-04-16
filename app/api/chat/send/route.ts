import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'
import { sendWhatsAppCloudMessage } from '@/lib/whatsapp-cloud-api'

const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function POST(request: NextRequest) {
  try {
    // Verify user auth
    const authHeader = request.headers.get('Authorization')
    if (!authHeader?.startsWith('Bearer ')) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }
    const userPb = new PocketBase(PB_URL)
    userPb.authStore.save(authHeader.substring(7))
    try {
      await userPb.collection('users').authRefresh()
    } catch {
      return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
    }
    const userId = userPb.authStore.model?.id as string

    const { conversation_id, message } = await request.json()
    if (!conversation_id || !message) {
      return NextResponse.json({ error: 'conversation_id and message required' }, { status: 400 })
    }

    const adminPb = new PocketBase(PB_URL)
    await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

    // Get conversation
    const conv = await adminPb.collection('conversations').getOne(conversation_id)
    if (conv.user_id !== userId) {
      return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
    }

    // Get WhatsApp account matching the conversation's phone number
    const account = await adminPb.collection('whatsapp_accounts').getFirstListItem(
      `phone_number_id = "${conv.phone_number_id}"`
    )

    // Send via Cloud API
    const wamid = await sendWhatsAppCloudMessage(
      account.phone_number_id as string,
      account.access_token as string,
      conv.customer_phone as string,
      message as string
    )

    // Save to DB
    const record = await adminPb.collection('whatsapp_messages').create({
      user_id: userId,
      to_number: conv.customer_phone,
      message_content: message,
      message_type: 'text',
      direction: 'outbound',
      conversation_id,
      sent_at: new Date().toISOString(),
      message_id: wamid,
    })

    return NextResponse.json(record)
  } catch (error) {
    console.error('[chat/send] error:', error)
    return NextResponse.json({ error: 'Failed to send message' }, { status: 500 })
  }
}
