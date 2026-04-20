import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'

const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const WHATSAPP_SERVICE_URL = process.env.WHATSAPP_SERVICE_URL || 'http://localhost:3003'

export async function GET(request: NextRequest) {
  try {
    const authHeader = request.headers.get('Authorization')
    if (!authHeader?.startsWith('Bearer ')) {
      return NextResponse.json({ error: 'No auth token provided' }, { status: 401 })
    }

    const userPb = new PocketBase(PB_URL)
    userPb.authStore.save(authHeader.substring(7))
    try {
      await userPb.collection('users').authRefresh()
    } catch {
      return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
    }

    const userId = userPb.authStore.model?.id as string

    // Check for Cloud API account first
    try {
      const account = await userPb.collection('whatsapp_accounts').getFirstListItem(
        `user_id = "${userId}" && account_type = "business_api" && is_active = true`
      )
      return NextResponse.json({
        status: 'connected',
        connectedNumber: account.phone_number || null,
        phone_number_id: account.phone_number_id,
        accountType: 'business_api',
      })
    } catch {
      // No Cloud API account — fall through to legacy whatsapp-web.js service
    }

    // Legacy QR-based connection
    try {
      const response = await fetch(`${WHATSAPP_SERVICE_URL}/api/whatsapp/status/${userId}`)
      if (!response.ok) {
        return NextResponse.json({ status: 'not_initialized' })
      }
      return NextResponse.json(await response.json())
    } catch {
      return NextResponse.json({ status: 'not_initialized' })
    }
  } catch (error) {
    console.error('Error getting WhatsApp status:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
