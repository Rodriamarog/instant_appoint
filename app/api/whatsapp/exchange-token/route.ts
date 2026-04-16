import { NextRequest, NextResponse } from 'next/server'
import { pb } from '@/lib/pocketbase'

const APP_ID = process.env.META_APP_ID!
const APP_SECRET = process.env.META_APP_SECRET!

export async function POST(request: NextRequest) {
  try {
    const authHeader = request.headers.get('Authorization')
    if (!authHeader?.startsWith('Bearer ')) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    const token = authHeader.substring(7)
    try {
      pb.authStore.save(token)
      await pb.collection('users').authRefresh()
    } catch {
      return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
    }

    if (!pb.authStore.isValid) {
      return NextResponse.json({ error: 'Invalid session' }, { status: 401 })
    }

    const userId = pb.authStore.model?.id
    const { code, waba_id, phone_number_id } = await request.json()

    if (!code || !waba_id || !phone_number_id) {
      return NextResponse.json({ error: 'Missing code, waba_id or phone_number_id' }, { status: 400 })
    }

    // Exchange code for access token server-side
    const tokenRes = await fetch(
      `https://graph.facebook.com/v25.0/oauth/access_token?client_id=${APP_ID}&client_secret=${APP_SECRET}&code=${code}`,
      { method: 'GET' }
    )
    const tokenData = await tokenRes.json()

    if (!tokenRes.ok || tokenData.error) {
      console.error('Token exchange failed:', tokenData)
      return NextResponse.json({ error: 'Failed to exchange token', details: tokenData }, { status: 500 })
    }

    const accessToken = tokenData.access_token

    // Upsert whatsapp_accounts record for this user
    try {
      const existing = await pb.collection('whatsapp_accounts').getFirstListItem(`user_id = "${userId}"`)
      await pb.collection('whatsapp_accounts').update(existing.id, {
        account_type: 'business_api',
        waba_id,
        phone_number_id,
        access_token: accessToken,
        status: 'connected',
        is_active: true,
      })
    } catch {
      await pb.collection('whatsapp_accounts').create({
        user_id: userId,
        session_id: `business_${userId}`,
        account_type: 'business_api',
        waba_id,
        phone_number_id,
        access_token: accessToken,
        status: 'connected',
        is_active: true,
      })
    }

    return NextResponse.json({ success: true })
  } catch (error) {
    console.error('Error exchanging token:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}
