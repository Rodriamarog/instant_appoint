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

    const { to, message } = await request.json()

    if (!to || !message) {
      return NextResponse.json(
        { error: 'Recipient phone number and message are required' },
        { status: 400 }
      )
    }

    // Send message via WhatsApp service
    const response = await fetch(`${WHATSAPP_SERVICE_URL}/api/whatsapp/send-message`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ userId, to, message })
    })

    if (!response.ok) {
      const errorData = await response.json()
      return NextResponse.json(
        { error: 'Failed to send WhatsApp message', details: errorData },
        { status: 500 }
      )
    }

    const messageData = await response.json()

    // Log message in PocketBase
    try {
      const whatsappAccount = await pb.collection('whatsapp_accounts').getFirstListItem(
        `user_id = "${userId}"`
      )

      await pb.collection('whatsapp_messages').create({
        user_id: userId,
        whatsapp_account_id: whatsappAccount.id,
        to_number: to,
        message_content: message,
        message_type: 'text',
        status: 'sent',
        sent_at: new Date().toISOString(),
        message_id: messageData.messageId
      })
    } catch (error) {
      console.error('Error logging message to PocketBase:', error)
      // Don't fail the request if logging fails
    }

    return NextResponse.json({
      success: true,
      messageId: messageData.messageId,
      to: messageData.to,
      message: messageData.message
    })
  } catch (error) {
    console.error('Error sending WhatsApp message:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}