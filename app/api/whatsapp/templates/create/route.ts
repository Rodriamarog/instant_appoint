import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'

const PB_URL = process.env.POCKETBASE_INTERNAL_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function POST(request: NextRequest) {
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

  const { name, category, language, body } = await request.json()
  if (!name || !category || !language || !body) {
    return NextResponse.json({ error: 'name, category, language, and body are required' }, { status: 400 })
  }

  const adminPb = new PocketBase(PB_URL)
  await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

  let account
  try {
    account = await adminPb.collection('whatsapp_accounts').getFirstListItem(
      `user_id = "${userId}" && account_type = "business_api" && is_active = true && waba_id = "506463685879575"`
    )
  } catch {
    return NextResponse.json({ error: 'No connected WhatsApp account found' }, { status: 404 })
  }

  const res = await fetch(`https://graph.facebook.com/v21.0/${account.waba_id}/message_templates`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${account.access_token}`,
    },
    body: JSON.stringify({
      name,
      category,
      language,
      components: [{ type: 'BODY', text: body }],
    }),
  })

  const data = await res.json()
  if (!res.ok || data.error) {
    return NextResponse.json({ error: data.error?.message ?? 'Failed to create template', details: data.error }, { status: 500 })
  }

  return NextResponse.json({ success: true, template: data })
}
