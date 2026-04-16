import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'

const APP_ID = process.env.META_APP_ID!
const APP_SECRET = process.env.META_APP_SECRET!
const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function POST(request: NextRequest) {
  try {
    // Verify user auth
    const authHeader = request.headers.get('Authorization')
    if (!authHeader?.startsWith('Bearer ')) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    // Use a fresh PB instance with user token to verify identity
    const userPb = new PocketBase(PB_URL)
    const userToken = authHeader.substring(7)
    try {
      userPb.authStore.save(userToken)
      await userPb.collection('users').authRefresh()
    } catch {
      return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
    }

    const userId = userPb.authStore.model?.id
    if (!userId) {
      return NextResponse.json({ error: 'Could not determine user ID' }, { status: 401 })
    }

    const body = await request.json()
    const { code, waba_id, phone_number_id } = body

    console.log('[exchange-token] received:', { userId, code: code?.slice(0, 20) + '...', waba_id, phone_number_id })

    if (!code || !waba_id || !phone_number_id) {
      console.error('[exchange-token] missing fields:', { code: !!code, waba_id, phone_number_id })
      return NextResponse.json({ error: 'Missing code, waba_id or phone_number_id' }, { status: 400 })
    }

    // Exchange code for access token server-side
    const tokenRes = await fetch(
      `https://graph.facebook.com/v25.0/oauth/access_token?client_id=${APP_ID}&client_secret=${APP_SECRET}&code=${code}`,
      { method: 'GET' }
    )
    const tokenData = await tokenRes.json()

    if (!tokenRes.ok || tokenData.error) {
      console.error('[exchange-token] token exchange failed:', tokenData)
      return NextResponse.json({ error: 'Failed to exchange token', details: tokenData }, { status: 500 })
    }

    const accessToken = tokenData.access_token
    console.log('[exchange-token] token exchange success, saving to PocketBase...')

    // Use admin credentials for writes (collection rules are admin-only)
    const adminPb = new PocketBase(PB_URL)
    await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

    const record = { account_type: 'business_api', waba_id, phone_number_id, access_token: accessToken, status: 'connected', is_active: true }

    try {
      const existing = await adminPb.collection('whatsapp_accounts').getFirstListItem(`user_id = "${userId}"`)
      await adminPb.collection('whatsapp_accounts').update(existing.id, record)
      console.log('[exchange-token] updated existing record', existing.id)
    } catch {
      await adminPb.collection('whatsapp_accounts').create({ ...record, user_id: userId, session_id: `business_${userId}` })
      console.log('[exchange-token] created new record')
    }

    return NextResponse.json({ success: true })
  } catch (error) {
    console.error('[exchange-token] error:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
