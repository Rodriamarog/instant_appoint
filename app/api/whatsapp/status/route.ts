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

    // Legacy QR-based status
    let legacyStatus = { status: 'not_initialized', connectedNumber: null as string | null }
    try {
      const response = await fetch(`${WHATSAPP_SERVICE_URL}/api/whatsapp/status/${userId}`)
      if (response.ok) {
        legacyStatus = await response.json()
      }
    } catch {
      // service not running
    }

    // Cloud API accounts
    let cloudAccounts: { id: string; phone_number: string; phone_number_id: string; waba_id: string }[] = []
    try {
      const records = await userPb.collection('whatsapp_accounts').getFullList({
        filter: `user_id = "${userId}" && account_type = "business_api" && is_active = true`,
      })
      cloudAccounts = records.map(r => ({
        id: r.id,
        phone_number: r.phone_number as string,
        phone_number_id: r.phone_number_id as string,
        waba_id: r.waba_id as string,
        verified_name: r.verified_name as string,
      }))
    } catch {
      // no records
    }

    return NextResponse.json({ legacy: legacyStatus, cloudAccounts })
  } catch (error) {
    console.error('Error getting WhatsApp status:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
