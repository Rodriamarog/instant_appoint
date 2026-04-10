import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

async function authenticate(request: NextRequest) {
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
  return pb.authStore.model?.id as string | null
}

export async function GET(request: NextRequest) {
  const userId = await authenticate(request)
  if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  try {
    const record = await pb.collection('reminder_settings').getFirstListItem(
      `user_id = "${userId}"`
    )
    return NextResponse.json({
      id: record.id,
      timing_minutes: record.timing_minutes,
      is_active: record.is_active,
    })
  } catch {
    // No settings yet — return defaults
    return NextResponse.json({ timing_minutes: 1440, is_active: true })
  }
}

export async function POST(request: NextRequest) {
  const userId = await authenticate(request)
  if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

  const { timing_minutes, is_active } = await request.json()

  const data = { user_id: userId, timing_minutes, is_active }

  try {
    let record
    try {
      const existing = await pb.collection('reminder_settings').getFirstListItem(
        `user_id = "${userId}"`
      )
      record = await pb.collection('reminder_settings').update(existing.id, data)
    } catch {
      record = await pb.collection('reminder_settings').create(data)
    }

    return NextResponse.json({
      id: record.id,
      timing_minutes: record.timing_minutes,
      is_active: record.is_active,
    })
  } catch (error) {
    console.error('Error saving reminder settings:', error)
    return NextResponse.json({ error: 'Failed to save settings' }, { status: 500 })
  }
}
