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
    const records = await pb.collection('whatsapp_messages').getFullList({
      filter: `user_id = "${userId}" && message_type = "reminder" && status = "sent"`,
    })

    const eventIds = records.map(r => r.event_id).filter(Boolean)
    return NextResponse.json({ eventIds })
  } catch {
    return NextResponse.json({ eventIds: [] })
  }
}
