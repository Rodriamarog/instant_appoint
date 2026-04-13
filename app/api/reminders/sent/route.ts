import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

async function authenticate(request: NextRequest): Promise<string | null> {
  const authHeader = request.headers.get('Authorization')
  if (!authHeader?.startsWith('Bearer ')) return null
  const token = authHeader.substring(7)
  try {
    pb.authStore.save(token)
    await pb.collection('users').authRefresh()
  } catch {
    return null
  }
  if (!pb.authStore.isValid) return null
  return pb.authStore.model?.id ?? null
}

export async function GET(request: NextRequest) {
  const userId = await authenticate(request)
  if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  try {
    const records = await pb.collection('whatsapp_messages').getList(1, 50, {
      filter: `user_id = "${userId}" && message_type = "reminder" && status = "sent"`,
      sort: '-sent_at',
    }).then(r => r.items)

    const eventIds = records.map(r => r.event_id).filter(Boolean)
    const messages = records.map(r => ({
      id: r.id,
      event_id: r.event_id,
      to_number: r.to_number,
      message_content: r.message_content,
      sent_at: r.sent_at,
      status: r.status,
    }))
    return NextResponse.json({ eventIds, messages })
  } catch {
    return NextResponse.json({ eventIds: [] })
  }
}
