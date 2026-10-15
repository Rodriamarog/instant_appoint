import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'
import { resolvePhoneNumberId } from '@/lib/embedded-signup'

const APP_ID = process.env.META_APP_ID!
const APP_SECRET = process.env.META_APP_SECRET!
const PB_URL = process.env.POCKETBASE_INTERNAL_URL || 'http://127.0.0.1:8090'
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
    const { code, waba_id } = body

    console.log('[exchange-token] received:', { userId, waba_id, phone_number_id: body.phone_number_id })

    // phone_number_id may be missing: a WhatsApp Business app (coexistence) signup
    // doesn't always send it. It's looked up from the WABA after the token exchange.
    if (!code || !waba_id) {
      console.error('[exchange-token] missing fields:', { code: !!code, waba_id })
      return NextResponse.json({ error: 'Missing code or waba_id' }, { status: 400 })
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

    const phone_number_id = await resolvePhoneNumberId(waba_id, body.phone_number_id || undefined, accessToken)
    if (!phone_number_id) {
      console.error('[exchange-token] could not tell which phone number was onboarded:', { waba_id })
      return NextResponse.json({ error: 'Could not determine the WhatsApp phone number' }, { status: 400 })
    }

    // Subscribe app to this WABA's webhook events so inbound messages are delivered
    const subRes = await fetch(
      `https://graph.facebook.com/v25.0/${waba_id}/subscribed_apps`,
      { method: 'POST', headers: { Authorization: `Bearer ${accessToken}` } }
    )
    const subData = await subRes.json()
    if (!subData.success) {
      console.warn('[exchange-token] WABA webhook subscription failed:', subData)
    } else {
      console.log('[exchange-token] WABA webhook subscription confirmed')
    }

    // Use admin credentials for writes (collection rules are admin-only)
    const adminPb = new PocketBase(PB_URL)
    await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

    // Fetch verified_name for the phone number
    let verified_name = ''
    try {
      const pnRes = await fetch(`https://graph.facebook.com/v21.0/${phone_number_id}?fields=verified_name,display_phone_number&access_token=${accessToken}`)
      const pnData = await pnRes.json()
      verified_name = pnData.verified_name ?? ''
    } catch { /* non-fatal */ }

    const record = { account_type: 'business_api', waba_id, phone_number_id, access_token: accessToken, status: 'connected', is_active: true, verified_name }

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
