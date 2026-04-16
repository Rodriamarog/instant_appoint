import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'

const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function GET(request: NextRequest) {
  try {
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

    const { searchParams } = new URL(request.url)
    const conversationId = searchParams.get('conversation_id')
    if (!conversationId) {
      return NextResponse.json({ error: 'conversation_id required' }, { status: 400 })
    }

    const adminPb = new PocketBase(PB_URL)
    await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

    // Verify conversation belongs to user
    const conv = await adminPb.collection('conversations').getOne(conversationId)
    if (conv.user_id !== userId) {
      return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
    }

    const records = await adminPb.collection('whatsapp_messages').getFullList({
      filter: `conversation_id = "${conversationId}"`,
      sort: 'sent_at',
    })

    return NextResponse.json({ messages: records })
  } catch (error) {
    console.error('[chat/messages] error:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
