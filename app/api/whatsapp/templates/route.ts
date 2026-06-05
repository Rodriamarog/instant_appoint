import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'

const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function GET(request: NextRequest) {
  const authHeader = request.headers.get('Authorization')
  if (!authHeader?.startsWith('Bearer ')) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }

  const userPb = new PocketBase(PB_URL)
  userPb.authStore.save(authHeader.substring(7))
  try {
    await userPb.collection('users').authRefresh()
  } catch {
    return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
  }
  const userId = userPb.authStore.model?.id as string

  const adminPb = new PocketBase(PB_URL)
  await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

  let account
  try {
    account = await adminPb.collection('whatsapp_accounts').getFirstListItem(
      `user_id = "${userId}" && account_type = "business_api" && is_active = true`
    )
  } catch {
    console.error('[templates] no account found for userId:', userId)
    return NextResponse.json({ error: 'No connected WhatsApp account found' }, { status: 404 })
  }

  console.log('[templates] fetching for waba_id:', account.waba_id)

  const res = await fetch(
    `https://graph.facebook.com/v21.0/${account.waba_id}/message_templates?fields=name,status,category,language,components&limit=100`,
    { headers: { Authorization: `Bearer ${account.access_token}` } }
  )
  const data = await res.json()
  console.log('[templates] raw response:', JSON.stringify(data).slice(0, 500))

  if (!res.ok || data.error) {
    console.error('[templates] Meta API error:', JSON.stringify(data.error ?? data))
    return NextResponse.json({ error: 'Failed to fetch templates', details: data.error }, { status: 500 })
  }

  console.log('[templates] returned', data.data?.length ?? 0, 'templates')
  return NextResponse.json({ templates: data.data ?? [] })
}
