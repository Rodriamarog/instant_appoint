import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

const WHATSAPP_SERVICE_URL = process.env.WHATSAPP_SERVICE_URL || 'http://localhost:3003'

export async function POST(request: NextRequest) {
  try {
    // Check authentication via Authorization header
    const authHeader = request.headers.get('Authorization')
    console.log('Auth header exists:', !!authHeader)

    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return NextResponse.json({ error: 'No auth token provided' }, { status: 401 })
    }

    const token = authHeader.substring(7) // Remove 'Bearer ' prefix
    console.log('Token length:', token.length)

    try {
      // Set the auth token in PocketBase and verify it
      pb.authStore.save(token)

      // Validate the token by fetching user info
      await pb.collection('users').authRefresh()
    } catch (error) {
      console.error('Error validating auth token:', error)
      return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
    }

    console.log('Auth store valid:', pb.authStore.isValid)
    console.log('Auth store model:', pb.authStore.model?.id)

    if (!pb.authStore.isValid) {
      return NextResponse.json({ error: 'Invalid session' }, { status: 401 })
    }

    const userId = pb.authStore.model?.id
    console.log('=== NEXT.JS API DEBUG ===')
    console.log('Auth store model:', pb.authStore.model)
    console.log('User ID extracted:', userId)
    console.log('User ID type:', typeof userId)
    console.log('========================')

    if (!userId) {
      return NextResponse.json({ error: 'User ID not found' }, { status: 401 })
    }

    // Initialize session with WhatsApp service
    console.log('=== SENDING TO WHATSAPP SERVICE ===')
    console.log('URL:', `${WHATSAPP_SERVICE_URL}/api/whatsapp/init-session`)
    console.log('Payload:', JSON.stringify({ userId }))
    console.log('====================================')

    const response = await fetch(`${WHATSAPP_SERVICE_URL}/api/whatsapp/init-session`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ userId })
    })

    if (!response.ok) {
      const errorData = await response.json()
      return NextResponse.json(
        { error: 'Failed to initialize WhatsApp session', details: errorData },
        { status: 500 }
      )
    }

    // The service returns { status: 'initializing' } immediately.
    // QR code and connection updates arrive via Socket.IO — not in this HTTP response.

    return NextResponse.json({ success: true, status: 'initializing' })
  } catch (error) {
    console.error('Error initializing WhatsApp session:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}