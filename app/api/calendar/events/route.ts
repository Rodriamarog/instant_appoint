import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

function getUserIdFromRequest(request: NextRequest): string | null {
  const authHeader = request.headers.get('Authorization')
  if (!authHeader?.startsWith('Bearer ')) return null
  const token = authHeader.substring(7)
  try {
    const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString())
    return payload.id ?? null
  } catch {
    return null
  }
}

export async function GET(request: NextRequest) {
  try {
    const userId = getUserIdFromRequest(request)
    if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

    const { searchParams } = new URL(request.url)
    const startDate = searchParams.get('start_date')
    const endDate = searchParams.get('end_date')

    let filter = `user_id = "${userId}"`
    if (startDate) filter += ` && end_time >= "${startDate}"`
    if (endDate) filter += ` && start_time <= "${endDate}"`

    const records = await pb.collection('calendar_events').getFullList({
      filter,
      sort: 'start_time',
    })

    return NextResponse.json({ events: records })
  } catch (error) {
    console.error('Error fetching calendar events:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}

export async function POST(request: NextRequest) {
  try {
    const userId = getUserIdFromRequest(request)
    if (!userId) return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })

    const body = await request.json()
    const { title, client_phone, notes, start_time, end_time, location } = body

    if (!title || !start_time || !end_time) {
      return NextResponse.json({ error: 'title, start_time, and end_time are required' }, { status: 400 })
    }

    const record = await pb.collection('calendar_events').create({
      user_id: userId,
      title,
      client_phone: client_phone || '',
      notes: notes || '',
      start_time,
      end_time,
      location: location || '',
    })

    return NextResponse.json(record)
  } catch (error) {
    console.error('Error creating calendar event:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
