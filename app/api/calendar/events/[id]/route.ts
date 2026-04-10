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

export async function PATCH(request: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  try {
    const userId = await authenticate(request)
    if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

    const resolvedParams = await params
    const body = await request.json()
    const { title, client_phone, notes, start_time, end_time } = body

    const record = await pb.collection('calendar_events').update(resolvedParams.id, {
      title,
      client_phone,
      notes,
      start_time,
      end_time,
    })

    return NextResponse.json(record)
  } catch (error) {
    console.error('Error updating calendar event:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}

export async function DELETE(request: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  try {
    const userId = await authenticate(request)
    if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

    const resolvedParams = await params
    await pb.collection('calendar_events').delete(resolvedParams.id)

    return NextResponse.json({ success: true })
  } catch (error) {
    console.error('Error deleting calendar event:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
