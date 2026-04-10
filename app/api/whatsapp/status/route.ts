import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

const WHATSAPP_SERVICE_URL = process.env.WHATSAPP_SERVICE_URL || 'http://localhost:3003'

export async function GET(request: NextRequest) {
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

    // Get status from WhatsApp service
    const response = await fetch(`${WHATSAPP_SERVICE_URL}/api/whatsapp/status/${userId}`)

    if (!response.ok) {
      if (response.status === 404) {
        return NextResponse.json({
          status: 'not_initialized',
          message: 'WhatsApp session not found'
        })
      }

      const errorData = await response.json()
      return NextResponse.json(
        { error: 'Failed to get WhatsApp status', details: errorData },
        { status: 500 }
      )
    }

    const statusData = await response.json()

    // Update PocketBase record with latest status
    try {
      const whatsappAccount = await pb.collection('whatsapp_accounts').getFirstListItem(
        `user_id = "${userId}"`
      )

      await pb.collection('whatsapp_accounts').update(whatsappAccount.id, {
        status: statusData.status,
        phone_number: statusData.connectedNumber || whatsappAccount.phone_number,
        last_seen: new Date().toISOString()
      })
    } catch (error) {
      // Record might not exist, that's okay
      console.log('WhatsApp account record not found in PocketBase')
    }

    return NextResponse.json(statusData)
  } catch (error) {
    console.error('Error getting WhatsApp status:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}