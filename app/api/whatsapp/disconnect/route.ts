import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

const WHATSAPP_SERVICE_URL = process.env.WHATSAPP_SERVICE_URL || 'http://localhost:3003'

export async function POST(request: NextRequest) {
  try {
    // Check authentication via Authorization header
    const authHeader = request.headers.get('Authorization')

    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return NextResponse.json({ error: 'No auth token provided' }, { status: 401 })
    }

    const token = authHeader.substring(7) // Remove 'Bearer ' prefix

    try {
      pb.authStore.save(token)
      await pb.collection('users').authRefresh()
    } catch (error) {
      console.error('Error validating auth token:', error)
      return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
    }

    if (!pb.authStore.isValid) {
      return NextResponse.json({ error: 'Invalid session' }, { status: 401 })
    }

    const userId = pb.authStore.model?.id
    if (!userId) {
      return NextResponse.json({ error: 'User ID not found' }, { status: 401 })
    }

    // Disconnect from WhatsApp service
    const response = await fetch(`${WHATSAPP_SERVICE_URL}/api/whatsapp/disconnect`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ userId })
    })

    if (!response.ok) {
      const errorData = await response.json()
      return NextResponse.json(
        { error: 'Failed to disconnect WhatsApp session', details: errorData },
        { status: 500 }
      )
    }

    // Update PocketBase record
    try {
      const whatsappAccount = await pb.collection('whatsapp_accounts').getFirstListItem(
        `user_id = "${userId}"`
      )

      await pb.collection('whatsapp_accounts').update(whatsappAccount.id, {
        status: 'disconnected',
        qr_code: '',
        is_active: false,
        disconnected_at: new Date().toISOString()
      })
    } catch (error) {
      console.log('WhatsApp account record not found in PocketBase')
    }

    return NextResponse.json({
      success: true,
      message: 'WhatsApp session disconnected successfully'
    })
  } catch (error) {
    console.error('Error disconnecting WhatsApp:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}